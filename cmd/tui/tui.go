package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/n0madic/deye-monitor/deye"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// historyLen is the initial sparkline capacity, used until the first render
// learns the panel width (see updateSparklines, which grows it to fill).
const historyLen = 120

// series is a ring buffer of recent samples for a sparkline. Its capacity is
// retuned to the panel width each cycle so the history fills the full width.
type series struct {
	data []float64
	cap  int
}

func newSeries(capacity int) *series {
	return &series{cap: capacity}
}

// push appends v, dropping the oldest sample once cap is exceeded.
func (s *series) push(v float64) {
	s.data = append(s.data, v)
	if len(s.data) > s.cap {
		s.data = s.data[len(s.data)-s.cap:]
	}
}

// intervalSteps are the refresh intervals the +/- keys cycle through.
var intervalSteps = []time.Duration{
	time.Second, 2 * time.Second, 3 * time.Second, 5 * time.Second,
	10 * time.Second, 15 * time.Second, 30 * time.Second, time.Minute,
}

// adjustInterval moves cur one step along intervalSteps. dir>0 lengthens the
// interval, dir<0 shortens it; the result is clamped to the available steps.
func adjustInterval(cur time.Duration, dir int) time.Duration {
	idx := 0
	for i, s := range intervalSteps {
		if s <= cur {
			idx = i
		}
	}
	idx += dir
	if idx < 0 {
		idx = 0
	}
	if idx >= len(intervalSteps) {
		idx = len(intervalSteps) - 1
	}
	return intervalSteps[idx]
}

// tui owns the termui widgets, the power history ring buffers, and the live UI
// state (last reading, pause flag, current interval, transient error message).
type tui struct {
	ip       string
	model    string
	interval time.Duration
	paused   bool
	last     *deye.Reading
	errMsg   string

	grid    *ui.Grid
	header  *widgets.Paragraph
	gauge   *widgets.Gauge
	flow    *flowDiagram
	keyvals *widgets.Paragraph
	spark   *widgets.SparklineGroup
	slPV    *widgets.Sparkline
	slLoad  *widgets.Sparkline
	slGrid  *widgets.Sparkline
	slBat   *widgets.Sparkline
	details *widgets.Table

	pvSeries   *series
	loadSeries *series
	gridSeries *series
	batSeries  *series

	charge *deye.ChargeEstimator
}

func newTUI(ip, model string, interval time.Duration) *tui {
	t := &tui{
		ip:         ip,
		model:      model,
		interval:   interval,
		pvSeries:   newSeries(historyLen),
		loadSeries: newSeries(historyLen),
		gridSeries: newSeries(historyLen),
		batSeries:  newSeries(historyLen),
		charge:     deye.NewChargeEstimator(0),
	}

	t.header = widgets.NewParagraph()
	t.header.Title = "Deye Monitor"
	t.header.Text = "connecting…"

	t.gauge = widgets.NewGauge()
	t.gauge.Title = "Battery SOC"
	t.gauge.Percent = 0
	t.gauge.BarColor = ui.ColorGreen
	t.gauge.Label = "n/a"

	t.flow = newFlowDiagram()

	t.keyvals = widgets.NewParagraph()
	t.keyvals.Title = "Key Values"
	t.keyvals.Text = ""

	t.slPV = widgets.NewSparkline()
	t.slPV.LineColor = ui.ColorYellow
	t.slLoad = widgets.NewSparkline()
	t.slLoad.LineColor = ui.ColorMagenta
	t.slGrid = widgets.NewSparkline()
	t.slGrid.LineColor = ui.ColorRed
	t.slBat = widgets.NewSparkline()
	t.slBat.LineColor = ui.ColorGreen
	t.spark = widgets.NewSparklineGroup(t.slPV, t.slLoad, t.slGrid, t.slBat)
	t.spark.Title = "Power History"

	t.details = widgets.NewTable()
	t.details.Title = "Details"
	t.details.RowSeparator = false
	t.details.FillRow = false
	t.details.TextStyle = ui.NewStyle(ui.ColorWhite)
	t.details.Rows = [][]string{{"", "", "", ""}}

	t.grid = ui.NewGrid()
	t.grid.Set(
		ui.NewRow(0.10, t.header),
		ui.NewRow(0.38, // power-flow diagram, with gauge + key-values stacked beside it
			ui.NewCol(0.58, t.flow),
			ui.NewCol(0.42,
				ui.NewRow(0.5, t.gauge),
				ui.NewRow(0.5, t.keyvals),
			),
		),
		ui.NewRow(0.34, t.spark),
		ui.NewRow(0.18, t.details),
	)

	t.renderHeader()
	return t
}

// kwNum formats watts as a bare kW number (e.g. "2.70").
func kwNum(watts float64) string {
	return strconv.FormatFloat(watts/1000, 'f', 2, 64)
}

// kw formats watts as a signed-magnitude kW string (e.g. "2.70 kW").
func kw(watts float64) string {
	return kwNum(watts) + " kW"
}

// renderHeader rebuilds the header text from the last reading plus the live UI
// state (interval, pause, error). Safe to call before the first reading.
func (t *tui) renderHeader() {
	status := fmt.Sprintf("refresh %s", t.interval)
	if t.paused {
		status = "[PAUSED](fg:yellow,mod:bold)  " + status
	}
	keys := "[q]uit  [p]ause  [+/-] interval"
	model := displayModel(t.model, t.last)

	if t.last == nil {
		t.header.Text = fmt.Sprintf("%s\nconnecting to %s…\n%s   %s", model, t.ip, status, keys)
		if t.errMsg != "" {
			t.header.Text = fmt.Sprintf("%s\n[%s](fg:red)\n%s   %s", model, t.errMsg, status, keys)
		}
		return
	}

	r := t.last
	hb := ""
	if r.Heartbeats > 0 {
		ago := ""
		if !r.LastHeartbeat.IsZero() {
			ago = fmt.Sprintf(" %ds ago", int(time.Since(r.LastHeartbeat).Seconds()))
		}
		heart := "♡"
		color := "fg:red"
		if r.HeartbeatNow {
			heart = "♥"
			color = "fg:red,mod:bold"
		}
		hb = fmt.Sprintf("  [%s ×%d%s](%s)", heart, r.Heartbeats, ago, color)
	}

	errLine := ""
	if t.errMsg != "" {
		errLine = fmt.Sprintf("   [%s](fg:red)", t.errMsg)
	}

	t.header.Text = fmt.Sprintf(
		"[%s](fg:cyan,mod:bold)   SN %s   %s%s\nState: [%s](mod:bold)   Work mode: %s   %s%s\n%s   %s",
		model, r.Serial, t.ip, hb,
		r.State("device_state"), r.State("work_mode"), r.Time.Format("15:04:05"), errLine,
		status, keys,
	)
}

// update refreshes every widget and the history buffers from a fresh reading.
func (t *tui) update(r *deye.Reading) {
	t.last = r
	t.errMsg = ""
	if soc, ok := r.Get("bat_soc"); ok {
		t.charge.Observe(r.Time, soc)
	}
	t.renderHeader()
	t.updateGauge(r)
	t.updateFlow(r)
	t.updateKeyVals(r)
	t.updateSparklines(r)
	t.updateDetails(r)
}

func (t *tui) updateGauge(r *deye.Reading) {
	soc, ok := r.Get("bat_soc")
	if !ok {
		t.gauge.Percent = 0
		t.gauge.Label = "n/a"
		return
	}
	pct := int(soc)
	pct = max(0, min(pct, 100))
	t.gauge.Percent = pct

	switch {
	case pct >= 50:
		t.gauge.BarColor = ui.ColorGreen
	case pct >= 20:
		t.gauge.BarColor = ui.ColorYellow
	default:
		t.gauge.BarColor = ui.ColorRed
	}

	batP := r.Values["bat_power"]
	dir := "idle"
	switch {
	case batP < 0:
		dir = "chg"
	case batP > 0:
		dir = "dis"
	}
	label := fmt.Sprintf("%d%%  %s %s", pct, kw(math.Abs(batP)), dir)
	if bt, okt := r.Get("bat_temp"); okt {
		label += fmt.Sprintf("  %s°C", num(bt))
	}
	// While charging, append the estimated time until the battery is full.
	if batP < 0 {
		if d, ok := t.charge.TimeToFull(); ok {
			label += fmt.Sprintf("  full in %s", deye.FormatETA(d))
		}
	}
	t.gauge.Label = label
}

func (t *tui) updateFlow(r *deye.Reading) {
	t.flow.pv = r.PVTotal()
	t.flow.loadP = r.Values["load_power"]
	t.flow.batP = r.Values["bat_power"]
	t.flow.gridP = r.Values["grid_power"]
	t.flow.soc, _ = r.Get("bat_soc")
	t.flow.hasData = true
}

func (t *tui) updateKeyVals(r *deye.Reading) {
	pv := r.PVTotal()
	loadP := r.Values["load_power"]
	batP := r.Values["bat_power"]
	gridP := r.Values["grid_power"]

	gridArrow := "="
	if gridP > 0 {
		gridArrow = "▲"
	} else if gridP < 0 {
		gridArrow = "▼"
	}
	batArrow := "="
	if batP < 0 {
		batArrow = "▼" // charging
	} else if batP > 0 {
		batArrow = "▲" // discharging
	}

	row := func(label, val string) string { return fmt.Sprintf("%-11s%9s", label, val) }

	lines := []string{
		row("PV total", num(pv)+" W"),
		row("Load", num(loadP)+" W"),
		row("Grid "+gridArrow, num(math.Abs(gridP))+" W"),
		row("Battery "+batArrow, num(math.Abs(batP))+" W"),
	}
	if gf, ok := r.Get("grid_freq"); ok {
		lines = append(lines, row("Grid freq", num(gf)+" Hz"))
	}
	if v, ok := r.Get("e_today_pv"); ok {
		lines = append(lines, row("Today PV", num(v)+" kWh"))
	}
	if v, ok := r.Get("e_today_load"); ok {
		lines = append(lines, row("Today load", num(v)+" kWh"))
	}
	t.keyvals.Text = strings.Join(lines, "\n")
}

func (t *tui) updateSparklines(r *deye.Reading) {
	// Match the ring-buffer capacity to the panel width so the history can grow
	// to fill the whole width instead of stalling at the fixed initial cap.
	if w := t.spark.Inner.Dx(); w > 1 {
		for _, s := range []*series{t.pvSeries, t.loadSeries, t.gridSeries, t.batSeries} {
			s.cap = w
		}
	}

	t.pvSeries.push(r.PVTotal())
	t.loadSeries.push(math.Abs(r.Values["load_power"]))
	t.gridSeries.push(math.Abs(r.Values["grid_power"]))
	t.batSeries.push(math.Abs(r.Values["bat_power"]))

	gridP := r.Values["grid_power"]
	gridSign := "→ import"
	if gridP < 0 {
		gridSign = "← export"
	}
	batP := r.Values["bat_power"]
	batSign := "charging"
	if batP > 0 {
		batSign = "discharging"
	}

	setSpark(t.slPV, t.pvSeries, fmt.Sprintf("PV  %s", kw(r.PVTotal())))
	setSpark(t.slLoad, t.loadSeries, fmt.Sprintf("Load  %s", kw(r.Values["load_power"])))
	setSpark(t.slGrid, t.gridSeries, fmt.Sprintf("Grid %s  %s", gridSign, kw(math.Abs(gridP))))
	setSpark(t.slBat, t.batSeries, fmt.Sprintf("Bat %s  %s", batSign, kw(math.Abs(batP))))
}

// setSpark binds a series to a sparkline, appends the min/max over the history
// buffer to the title, and pins MaxVal so all-zero data does not divide by zero
// in the widget's Draw.
func setSpark(sl *widgets.Sparkline, s *series, title string) {
	sl.Data = s.data
	if len(s.data) == 0 {
		sl.Title = title
		sl.MaxVal = 1
		return
	}
	minv, maxv := s.data[0], s.data[0]
	for _, v := range s.data {
		minv = math.Min(minv, v)
		maxv = math.Max(maxv, v)
	}
	sl.Title = fmt.Sprintf("%s   ↓%s ↑%s kW", title, kwNum(minv), kwNum(maxv))
	sl.MaxVal = math.Max(maxv, 1)
}

func (t *tui) updateDetails(r *deye.Reading) {
	t.details.Rows = [][]string{
		{"PV1", vstr(r, "pv1_v", "V"), vstr(r, "pv1_i", "A"), vstr(r, "pv1_p", "W")},
		{"PV2", vstr(r, "pv2_v", "V"), vstr(r, "pv2_i", "A"), vstr(r, "pv2_p", "W")},
		{"Grid V", "L1 " + vstr(r, "grid_l1_v", ""), "L2 " + vstr(r, "grid_l2_v", ""), "L3 " + vstr(r, "grid_l3_v", "")},
		{"Load V", "L1 " + vstr(r, "load_l1_v", ""), "L2 " + vstr(r, "load_l2_v", ""), "L3 " + vstr(r, "load_l3_v", "")},
		{"Temp", "DC " + vstr(r, "temp_dc", "°C"), "AC " + vstr(r, "temp_ac", "°C"), "BAT " + vstr(r, "bat_temp", "°C")},
		{"Today kWh", "PV " + vstr(r, "e_today_pv", ""), "Load " + vstr(r, "e_today_load", ""), "Imp " + vstr(r, "e_today_imp", "") + " Exp " + vstr(r, "e_today_exp", "")},
		{"Total kWh", "PV " + vstr(r, "e_total_pv", ""), "Load " + vstr(r, "e_total_load", ""), "Chg " + vstr(r, "e_total_chg", "") + " Dis " + vstr(r, "e_total_dis", "")},
	}
}

// vstr renders a metric value with its unit, or "n/a" if absent.
func vstr(r *deye.Reading, key, unit string) string {
	v, ok := r.Get(key)
	if !ok {
		return "n/a"
	}
	return num(v) + unit
}

// showError surfaces a read failure in the header without dropping the last
// good reading from the rest of the widgets.
func (t *tui) showError(err error) {
	t.errMsg = "error: " + err.Error()
	t.renderHeader()
}

// runTUI runs the interactive termui dashboard. If termui cannot grab the
// terminal (e.g. no real TTY), it falls back to the text loop.
func runTUI(client *deye.Client, ip, model string, interval time.Duration) {
	if err := ui.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "termui init failed, falling back to text mode:", err)
		runPlainLoop(client, ip, model, interval)
		return
	}
	defer ui.Close()

	t := newTUI(ip, model, interval)
	w, h := ui.TerminalDimensions()
	t.grid.SetRect(0, 0, w, h)
	ui.Render(t.grid)

	snapCh := make(chan *deye.Reading, 1)
	errCh := make(chan error, 1)
	poll := func() {
		go func() {
			r, err := client.Snapshot()
			if err != nil {
				errCh <- err
				return
			}
			snapCh <- r
		}()
	}
	poll() // immediate first read

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	uiEvents := ui.PollEvents()

	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			case "p":
				t.paused = !t.paused
				t.renderHeader()
				ui.Render(t.grid)
			case "+", "=":
				interval = adjustInterval(interval, +1)
				t.interval = interval
				ticker.Reset(interval)
				t.renderHeader()
				ui.Render(t.grid)
			case "-", "_":
				interval = adjustInterval(interval, -1)
				t.interval = interval
				ticker.Reset(interval)
				t.renderHeader()
				ui.Render(t.grid)
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				t.grid.SetRect(0, 0, payload.Width, payload.Height)
				ui.Clear()
				ui.Render(t.grid)
			}
		case <-ticker.C:
			if !t.paused {
				poll()
			}
		case r := <-snapCh:
			t.update(r)
			ui.Render(t.grid)
		case err := <-errCh:
			t.showError(err)
			ui.Render(t.grid)
		}
	}
}
