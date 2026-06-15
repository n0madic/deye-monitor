package main

import (
	"image"
	"math"

	ui "github.com/gizak/termui/v3"
)

// flowDiagram is a custom termui widget that draws a power-flow diagram in the
// style of the Solarman web UI: an inverter in the centre with PV, grid,
// battery and load nodes in the four corners, connected by elbow lines whose
// arrowheads show the direction of power flow.
type flowDiagram struct {
	ui.Block
	pv      float64 // PV total power (W, >=0)
	gridP   float64 // grid power (W, >0 import, <0 export)
	batP    float64 // battery power (W, <0 charging, >0 discharging)
	loadP   float64 // load power (W, >=0)
	soc     float64 // battery state of charge (%)
	hasData bool
}

func newFlowDiagram() *flowDiagram {
	f := &flowDiagram{Block: *ui.NewBlock()}
	f.Title = "Power Flow"
	return f
}

// set sets a styled rune, clipped to the widget's inner rectangle.
func (f *flowDiagram) set(buf *ui.Buffer, x, y int, r rune, st ui.Style) {
	if x < f.Inner.Min.X || x >= f.Inner.Max.X || y < f.Inner.Min.Y || y >= f.Inner.Max.Y {
		return
	}
	buf.SetCell(ui.NewCell(r, st), image.Pt(x, y))
}

// str writes a string rune-by-rune (width-1 runes only), clipped to Inner.
func (f *flowDiagram) str(buf *ui.Buffer, x, y int, s string, st ui.Style) {
	for i, r := range []rune(s) {
		f.set(buf, x+i, y, r, st)
	}
}

func (f *flowDiagram) hline(buf *ui.Buffer, x0, x1, y int, st ui.Style) {
	for x := x0; x <= x1; x++ {
		f.set(buf, x, y, '─', st)
	}
}

func (f *flowDiagram) vline(buf *ui.Buffer, x, y0, y1 int, st ui.Style) {
	for y := y0; y <= y1; y++ {
		f.set(buf, x, y, '│', st)
	}
}

func rlen(s string) int { return len([]rune(s)) }

// wfmt formats watts as a compact "<n>W" string.
func wfmt(watts float64) string { return num(watts) + "W" }

// Draw renders the diagram. It is dispatched by termui with the widget's lock
// held; all geometry is derived from f.Inner so it adapts to the panel size.
func (f *flowDiagram) Draw(buf *ui.Buffer) {
	f.Block.Draw(buf)
	in := f.Inner

	yellow := ui.NewStyle(ui.ColorYellow)
	magenta := ui.NewStyle(ui.ColorMagenta)
	green := ui.NewStyle(ui.ColorGreen)
	red := ui.NewStyle(ui.ColorRed)
	gray := ui.NewStyle(ui.ColorWhite)
	invSt := ui.NewStyle(ui.ColorCyan, ui.ColorClear, ui.ModifierBold)

	// Node styles, value strings and arrowheads by power direction.
	gridSt, gridArrow, gridTag := gray, '─', ""
	switch {
	case f.gridP > 0:
		gridSt, gridArrow, gridTag = red, '◀', " import" // into inverter
	case f.gridP < 0:
		gridSt, gridArrow, gridTag = green, '▶', " export" // out to grid
	}
	batSt, batArrow, batTag := gray, '─', ""
	switch {
	case f.batP < 0:
		batSt, batArrow, batTag = green, '◀', " charging" // out to battery
	case f.batP > 0:
		batSt, batArrow, batTag = yellow, '▶', " discharging" // into inverter
	}

	pvL1, pvVal := "PV", wfmt(f.pv)
	gridL1, gridVal := "GRID", wfmt(math.Abs(f.gridP))+gridTag
	batL1, batVal := "BATTERY "+num(f.soc)+"%", wfmt(math.Abs(f.batP))+batTag
	loadL1, loadVal := "LOAD", wfmt(f.loadP)

	maxLeft := maxInt(rlen(pvL1), rlen(pvVal), rlen(batL1), rlen(batVal))
	maxRight := maxInt(rlen(gridL1), rlen(gridVal), rlen(loadL1), rlen(loadVal))

	const invLabel = "INVERTER"
	iw := rlen(invLabel)

	left, top := in.Min.X, in.Min.Y
	right, bottom := in.Max.X-1, in.Max.Y-1
	cx := (in.Min.X + in.Max.X) / 2
	cy := (in.Min.Y + in.Max.Y) / 2
	bxL := cx - iw/2 - 1
	bxR := bxL + iw + 1
	railL := left + maxLeft + 2
	railR := right - maxRight - 2

	feasible := f.hasData &&
		railL <= bxL-2 && railR >= bxR+2 &&
		cy-2 >= top+1 && cy+2 <= bottom-1
	if !feasible {
		f.drawFallback(buf, pvVal, gridVal, batVal, loadVal,
			gridArrow, batArrow, yellow, magenta, gridSt, batSt)
		return
	}

	// Inverter box (3 tall), centred and sized to its label.
	f.set(buf, bxL, cy-1, '┌', invSt)
	f.hline(buf, bxL+1, bxR-1, cy-1, invSt)
	f.set(buf, bxR, cy-1, '┐', invSt)
	f.set(buf, bxL, cy, '│', invSt)
	f.str(buf, bxL+1, cy, invLabel, invSt)
	f.set(buf, bxR, cy, '│', invSt)
	f.set(buf, bxL, cy+1, '└', invSt)
	f.hline(buf, bxL+1, bxR-1, cy+1, invSt)
	f.set(buf, bxR, cy+1, '┘', invSt)

	// PV — top-left, into the inverter's top-left.
	f.str(buf, left, top, pvL1, yellow)
	f.str(buf, left, top+1, pvVal, yellow)
	f.hline(buf, left+rlen(pvL1)+1, railL-1, top, yellow)
	f.set(buf, railL, top, '┐', yellow)
	f.vline(buf, railL, top+1, cy-2, yellow)
	f.set(buf, railL, cy-1, '└', yellow)
	f.hline(buf, railL+1, bxL-2, cy-1, yellow)
	f.set(buf, bxL-1, cy-1, '▶', yellow)

	// GRID — top-right, into/out of the inverter's top-right.
	gs := right - rlen(gridL1) + 1
	vs := right - rlen(gridVal) + 1
	f.str(buf, gs, top, gridL1, gridSt)
	f.str(buf, vs, top+1, gridVal, gridSt)
	f.set(buf, railR, top, '┌', gridSt)
	f.hline(buf, railR+1, gs-2, top, gridSt)
	f.vline(buf, railR, top+1, cy-2, gridSt)
	f.set(buf, railR, cy-1, '┘', gridSt)
	f.hline(buf, bxR+2, railR-1, cy-1, gridSt)
	f.set(buf, bxR+1, cy-1, gridArrow, gridSt)

	// BAT — bottom-left, into/out of the inverter's bottom-left.
	f.str(buf, left, bottom-1, batL1, batSt)
	f.str(buf, left, bottom, batVal, batSt)
	f.hline(buf, left+rlen(batL1)+1, railL-1, bottom-1, batSt)
	f.set(buf, railL, bottom-1, '┘', batSt)
	f.vline(buf, railL, cy+2, bottom-2, batSt)
	f.set(buf, railL, cy+1, '┌', batSt)
	f.hline(buf, railL+1, bxL-2, cy+1, batSt)
	f.set(buf, bxL-1, cy+1, batArrow, batSt)

	// LOAD — bottom-right, out of the inverter's bottom-right.
	ls := right - rlen(loadL1) + 1
	lvs := right - rlen(loadVal) + 1
	f.str(buf, ls, bottom-1, loadL1, magenta)
	f.str(buf, lvs, bottom, loadVal, magenta)
	f.set(buf, railR, bottom-1, '└', magenta)
	f.hline(buf, railR+1, ls-2, bottom-1, magenta)
	f.vline(buf, railR, cy+2, bottom-2, magenta)
	f.set(buf, railR, cy+1, '┐', magenta)
	f.hline(buf, bxR+2, railR-1, cy+1, magenta)
	f.set(buf, bxR+1, cy+1, '▶', magenta)
}

// drawFallback renders a compact textual flow list when the panel is too small
// for the diagram.
func (f *flowDiagram) drawFallback(buf *ui.Buffer, pvVal, gridVal, batVal, loadVal string,
	gridArrow, batArrow rune, yellow, magenta, gridSt, batSt ui.Style) {
	y := f.Inner.Min.Y
	x := f.Inner.Min.X
	rows := []struct {
		label, val string
		arrow      rune
		st         ui.Style
	}{
		{"PV", pvVal, '▶', yellow},
		{"GRID", gridVal, gridArrow, gridSt},
		{"BATTERY", batVal, batArrow, batSt},
		{"LOAD", loadVal, '▶', magenta},
	}
	for i, r := range rows {
		f.str(buf, x, y+i, r.label, r.st)
		f.str(buf, x+9, y+i, string(r.arrow)+" "+r.val, r.st)
	}
}

func maxInt(vals ...int) int {
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}
