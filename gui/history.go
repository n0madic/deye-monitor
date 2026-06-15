package gui

import (
	"fmt"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// chart is a minimal line chart over a series, drawn with canvas primitives. It
// plots watt samples but labels min/max/latest in kW.
type chart struct {
	widget.BaseWidget
	title string
	col   color.Color
	data  []float64
	note  string // direction tag, e.g. "import" / "charging"
}

func newChart(title string, col color.Color) *chart {
	c := &chart{title: title, col: col}
	c.ExtendBaseWidget(c)
	return c
}

// setData replaces the plotted samples and the direction note, then repaints.
func (c *chart) setData(data []float64, note string) {
	c.data = data
	c.note = note
	c.Refresh()
}

func (c *chart) CreateRenderer() fyne.WidgetRenderer {
	r := &chartRenderer{
		c:     c,
		bg:    canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground)),
		title: newText(c.title, c.col, true, fyne.TextAlignLeading),
		stats: newText("", theme.Color(theme.ColorNamePlaceHolder), false, fyne.TextAlignTrailing),
	}
	r.bg.CornerRadius = 4
	r.Refresh()
	return r
}

type chartRenderer struct {
	c       *chart
	bg      *canvas.Rectangle
	title   *canvas.Text
	stats   *canvas.Text
	lines   []*canvas.Line
	objects []fyne.CanvasObject
}

func (r *chartRenderer) Refresh() {
	title := r.c.title
	if r.c.note != "" {
		title = fmt.Sprintf("%s — %s", r.c.title, r.c.note)
	}
	r.title.Text = title
	r.title.Color = r.c.col

	mn, mx, latest := stats(r.c.data)
	r.stats.Text = fmt.Sprintf("%.2f kW   ↓%.2f ↑%.2f kW", latest/1000, mn/1000, mx/1000)

	r.rebuild(r.c.Size())
	canvas.Refresh(r.c)
}

func (r *chartRenderer) Layout(size fyne.Size) { r.rebuild(size) }

// rebuild positions the chrome and regenerates the polyline for the plot area.
func (r *chartRenderer) rebuild(size fyne.Size) {
	pad := theme.Padding()
	r.bg.Resize(size)
	r.bg.Move(fyne.NewPos(0, 0))

	lineH := r.title.MinSize().Height
	r.title.Resize(fyne.NewSize(size.Width/2, lineH))
	r.title.Move(fyne.NewPos(pad, pad))
	r.stats.Resize(fyne.NewSize(size.Width-size.Width/2-2*pad, lineH))
	r.stats.Move(fyne.NewPos(size.Width/2+pad, pad))

	top := pad + lineH + pad
	plot := fyne.NewPos(pad, top)
	pw := size.Width - 2*pad
	ph := size.Height - top - pad

	r.lines = r.lines[:0]
	data := r.c.data
	if pw > 1 && ph > 1 && len(data) >= 2 {
		mn, mx, _ := stats(data)
		span := mx - mn
		if span < 1 {
			span = 1
		}
		n := len(data)
		yOf := func(v float64) float32 {
			frac := (v - mn) / span
			return plot.Y + ph - float32(frac)*ph
		}
		xOf := func(i int) float32 {
			return plot.X + pw*float32(i)/float32(n-1)
		}
		prevX, prevY := xOf(0), yOf(data[0])
		for i := 1; i < n; i++ {
			x, y := xOf(i), yOf(data[i])
			ln := canvas.NewLine(r.c.col)
			ln.StrokeWidth = 1.6
			ln.Position1 = fyne.NewPos(prevX, prevY)
			ln.Position2 = fyne.NewPos(x, y)
			r.lines = append(r.lines, ln)
			prevX, prevY = x, y
		}
	}

	r.objects = make([]fyne.CanvasObject, 0, len(r.lines)+3)
	r.objects = append(r.objects, r.bg, r.title, r.stats)
	for _, ln := range r.lines {
		r.objects = append(r.objects, ln)
	}
}

func (r *chartRenderer) MinSize() fyne.Size           { return fyne.NewSize(260, 92) }
func (r *chartRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *chartRenderer) Destroy()                     {}

// stats returns the min, max and latest of data; zeros for an empty series.
func stats(data []float64) (mn, mx, latest float64) {
	if len(data) == 0 {
		return 0, 0, 0
	}
	mn, mx = data[0], data[0]
	for _, v := range data {
		mn = math.Min(mn, v)
		mx = math.Max(mx, v)
	}
	return mn, mx, data[len(data)-1]
}

// historyView is the History tab: one mini line chart per power series, refreshed
// from the controller's ring buffers.
type historyView struct {
	root   fyne.CanvasObject
	charts map[string]*chart
}

func newHistoryView() *historyView {
	hv := &historyView{charts: make(map[string]*chart, len(histKeys))}
	hv.charts["pv"] = newChart("PV", colYellow)
	hv.charts["load"] = newChart("Load", colBlue)
	hv.charts["grid"] = newChart("Grid", colRed)
	hv.charts["bat"] = newChart("Battery", colGreen)
	hv.root = container.NewVScroll(container.NewGridWithRows(4,
		hv.charts["pv"], hv.charts["load"], hv.charts["grid"], hv.charts["bat"],
	))
	return hv
}

// refresh pulls the latest ring-buffer snapshots from the controller and annotates
// the grid/battery charts with the current flow direction.
func (hv *historyView) refresh(c *controller) {
	hv.charts["pv"].setData(c.history("pv"), "")
	hv.charts["load"].setData(c.history("load"), "")

	gridNote, batNote := "", ""
	if r := c.latest.Load(); r != nil {
		switch {
		case r.Values["grid_power"] > 0:
			gridNote = "import"
		case r.Values["grid_power"] < 0:
			gridNote = "export"
		}
		switch {
		case r.Values["bat_power"] < 0:
			batNote = "charging"
		case r.Values["bat_power"] > 0:
			batNote = "discharging"
		}
	}
	hv.charts["grid"].setData(c.history("grid"), gridNote)
	hv.charts["bat"].setData(c.history("bat"), batNote)
}
