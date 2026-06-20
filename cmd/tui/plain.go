package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/n0madic/deye-monitor/deye"
)

// ANSI colors; cleared when output is not a terminal or for plain pipes.
var (
	cReset   = "\033[0m"
	cBold    = "\033[1m"
	cDim     = "\033[2m"
	cRed     = "\033[31m"
	cGreen   = "\033[32m"
	cYellow  = "\033[33m"
	cBlue    = "\033[34m"
	cMagenta = "\033[35m"
	cCyan    = "\033[36m"
)

func disableColors() {
	cReset, cBold, cDim = "", "", ""
	cRed, cGreen, cYellow, cBlue, cMagenta, cCyan = "", "", "", "", "", ""
}

// runOnce prints a single text dashboard snapshot then returns.
func runOnce(c *deye.Client, ip, model string) {
	if !isatty(os.Stdout) {
		disableColors()
	}
	r, err := c.Snapshot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(renderDashboard(r, ip, model))
}

// runPlainLoop runs the text dashboard on an interval. Used for -plain and when
// stdout is not a TTY (the termui TUI needs a real terminal). When piped, it
// drops the colors and the screen-clear so the output stays parseable.
func runPlainLoop(c *deye.Client, ip, model string, interval time.Duration) {
	tty := isatty(os.Stdout)
	if !tty {
		disableColors()
	}
	for {
		r, err := c.Snapshot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cycle failed:", err)
			time.Sleep(min(interval, 3*time.Second))
			continue
		}
		if tty {
			fmt.Print("\033[2J\033[H")
		}
		fmt.Println(renderDashboard(r, ip, model))
		if tty {
			fmt.Printf("\n%srefresh %s · Ctrl-C to quit%s\n", cDim, interval.String(), cReset)
		}
		time.Sleep(interval)
	}
}

// num formats a float with the minimal decimal representation (1319, 54.33, 12.2).
func num(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// fw right-aligns "value+unit" in a fixed width.
func fw(v float64, ok bool, unit string, width int) string {
	if !ok {
		return fmt.Sprintf("%*s", width, "n/a")
	}
	return fmt.Sprintf("%*s", width, num(v)+unit)
}

func fwk(r *deye.Reading, key string, width int) string {
	v, ok := r.Get(key)
	return fw(v, ok, "", width)
}

func line() string {
	return "──────────────────────────────────────────────────────────────────"
}

func renderDashboard(r *deye.Reading, ip, model string) string {
	model = displayModel(model, r)
	g := r.Get
	batP := r.Values["bat_power"]
	gridP := r.Values["grid_power"]
	loadP := r.Values["load_power"]

	batDir := "idle"
	switch {
	case batP < 0:
		batDir = cGreen + "charging" + cReset
	case batP > 0:
		batDir = cYellow + "discharging" + cReset
	}
	gridDir := "balanced"
	switch {
	case gridP > 0:
		gridDir = cRed + "import" + cReset
	case gridP < 0:
		gridDir = cGreen + "export" + cReset
	}

	pv1v, ok1 := g("pv1_v")
	pv1i, _ := g("pv1_i")
	pv1p, _ := g("pv1_p")
	pv2v, ok2 := g("pv2_v")
	pv2i, _ := g("pv2_i")
	pv2p, _ := g("pv2_p")
	pv3p, ok3 := g("pv3_p")
	pv4p, ok4 := g("pv4_p")
	bsoc, oks := g("bat_soc")
	bv, okv := g("bat_v")
	bt, okt := g("bat_temp")
	g1, okg1 := g("grid_l1_v")
	g2, okg2 := g("grid_l2_v")
	g3, okg3 := g("grid_l3_v")
	gf, okgf := g("grid_freq")
	gl1, _ := g("grid_l1_p")
	gl2, _ := g("grid_l2_p")
	gl3, _ := g("grid_l3_p")
	ll1, _ := g("load_l1_p")
	ll2, _ := g("load_l2_p")
	ll3, _ := g("load_l3_p")
	lf, oklf := g("load_freq")
	tdc, oktdc := g("temp_dc")
	tac, oktac := g("temp_ac")

	out := ""
	add := func(s string) { out += s + "\n" }

	add(fmt.Sprintf("%s%s%s%s  ·  %s  ·  SN %s%s", cBold, cCyan, model, cReset, ip, r.Serial, heartbeatTag(r)))
	add(fmt.Sprintf("%s%s%s   State: %s%s%s   Work mode: %s",
		cDim, r.Time.Format("2006-01-02 15:04:05"), cReset, cBold, r.State("device_state"), cReset, r.State("work_mode")))
	add(line())

	add(fmt.Sprintf("%s☀ SOLAR%s   PV1 %s %s %s   PV2 %s %s %s",
		cYellow, cReset,
		fw(pv1v, ok1, "V", 7), fw(pv1i, ok1, "A", 6), fw(pv1p, ok1, "W", 7),
		fw(pv2v, ok2, "V", 7), fw(pv2i, ok2, "A", 6), fw(pv2p, ok2, "W", 7)))
	add(fmt.Sprintf("           PV3 %s   PV4 %s          %sΣ %s W%s",
		fw(pv3p, ok3, "W", 7), fw(pv4p, ok4, "W", 7), cBold, num(r.PVTotal()), cReset))

	add(fmt.Sprintf("%s⚡ BATTERY%s SOC %s%s%s  %s  %s %s  %s",
		cGreen, cReset, cBold, fw(bsoc, oks, "%", 4), cReset,
		fw(bv, okv, "V", 8), fw(math.Abs(batP), true, "W", 7), batDir, fw(bt, okt, "°C", 7)))

	add(fmt.Sprintf("%s🔌 GRID%s    L1 %s  L2 %s  L3 %s  %s",
		cBlue, cReset, fw(g1, okg1, "V", 7), fw(g2, okg2, "V", 7), fw(g3, okg3, "V", 7), fw(gf, okgf, "Hz", 8)))
	add(fmt.Sprintf("           %s %s   (L1 %s L2 %s L3 %s)",
		fw(math.Abs(gridP), true, "W", 7), gridDir,
		fw(gl1, true, "W", 6), fw(gl2, true, "W", 6), fw(gl3, true, "W", 6)))

	add(fmt.Sprintf("%s🏠 LOAD%s    %s%s%s   (L1 %s L2 %s L3 %s)  %s",
		cMagenta, cReset, cBold, fw(loadP, true, "W", 7), cReset,
		fw(ll1, true, "W", 6), fw(ll2, true, "W", 6), fw(ll3, true, "W", 6), fw(lf, oklf, "Hz", 8)))

	add(fmt.Sprintf("%s🌡 TEMP    DC %s   AC %s   BAT %s%s",
		cDim, fw(tdc, oktdc, "°C", 7), fw(tac, oktac, "°C", 7), fw(bt, okt, "°C", 7), cReset))

	add(line())
	add(fmt.Sprintf("%sTODAY%s  PV %s  Load %s  Chg %s  Dis %s  Imp %s  Exp %s  kWh",
		cBold, cReset,
		fwk(r, "e_today_pv", 5), fwk(r, "e_today_load", 5), fwk(r, "e_today_chg", 5),
		fwk(r, "e_today_dis", 5), fwk(r, "e_today_imp", 5), fwk(r, "e_today_exp", 5)))
	add(fmt.Sprintf("%sTOTAL%s  PV %s  Load %s  Chg %s  Dis %s  kWh",
		cBold, cReset,
		fwk(r, "e_total_pv", 7), fwk(r, "e_total_load", 7), fwk(r, "e_total_chg", 7), fwk(r, "e_total_dis", 7)))
	return out
}

// heartbeatTag renders the logger heartbeat indicator: a filled ♥ pulses on the
// cycle a heartbeat lands, otherwise a dim hollow ♡ with the running count and
// time since the last one.
func heartbeatTag(r *deye.Reading) string {
	if r.Heartbeats == 0 {
		return ""
	}
	var heart string
	if r.HeartbeatNow {
		heart = cBold + cRed + "♥" + cReset
	} else {
		heart = cDim + "♡" + cReset
	}
	ago := ""
	if !r.LastHeartbeat.IsZero() {
		ago = fmt.Sprintf(" %ds ago", int(time.Since(r.LastHeartbeat).Seconds()))
	}
	return fmt.Sprintf("  %s%s ×%d%s%s", heart, cDim, r.Heartbeats, ago, cReset)
}
