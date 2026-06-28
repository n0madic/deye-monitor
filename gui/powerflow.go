package gui

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/n0madic/deye-monitor/deye"
)

// powerFlow is a custom widget that draws the Deye-Cloud-style power-flow
// diagram: a central inverter with PV, grid, battery and load nodes in the four
// corners, joined by colour-coded connectors whose arrows show flow direction.
type powerFlow struct {
	widget.BaseWidget

	pv      float64 // PV total power (W, >=0)
	gridP   float64 // grid power (W, >0 import, <0 export)
	batP    float64 // battery power (W, <0 charging, >0 discharging)
	loadP   float64 // load power (W, >=0)
	soc     float64 // battery state of charge (%)
	etaText string  // formatted time-to-full while charging ("" if unavailable)
	hasData bool
}

func newPowerFlow() *powerFlow {
	p := &powerFlow{}
	p.ExtendBaseWidget(p)
	return p
}

// setReading copies the flow-relevant fields out of a reading and repaints. A
// nil reading is ignored so the last good frame stays on screen.
func (p *powerFlow) setReading(r *deye.Reading) {
	if r == nil {
		return
	}
	p.pv = r.PVTotal()
	p.gridP = r.Values["grid_power"]
	p.batP = r.Values["bat_power"]
	p.loadP = r.Values["load_power"]
	p.soc, _ = r.Get("bat_soc")
	p.hasData = true
	p.Refresh()
}

// setChargeETA updates the battery node's time-to-full label. An empty string
// clears it (battery not charging or no estimate yet) and repaints.
func (p *powerFlow) setChargeETA(text string) {
	if p.etaText == text {
		return
	}
	p.etaText = text
	p.Refresh()
}

// fractional centre of each corner node within the widget, indexed pv/grid/bat/load.
var nodeCenters = [4][2]float32{
	{0.17, 0.28}, // PV   — top-left
	{0.83, 0.28}, // GRID — top-right
	{0.17, 0.72}, // BAT  — bottom-left
	{0.83, 0.72}, // LOAD — bottom-right
}

// batteryNode indexes the battery corner within nodeCenters / the node arrays.
const batteryNode = 2

type flowNodeObjs struct {
	img    *canvas.Image
	title  *canvas.Text
	detail *canvas.Text // optional second line, e.g. "100%" for battery
	value  *canvas.Text
	line   *canvas.Line
	arrow1 *canvas.Line // chevron barb
	arrow2 *canvas.Line // chevron barb
}

type powerFlowRenderer struct {
	w        *powerFlow
	bg       *canvas.Rectangle
	invImg   *canvas.Image
	invTitle *canvas.Text
	nodes    [4]*flowNodeObjs
	batETA   *canvas.Text // battery time-to-full, drawn below the battery node's value
	objects  []fyne.CanvasObject
}

func (p *powerFlow) CreateRenderer() fyne.WidgetRenderer {
	icons := [4]fyne.Resource{iconPV, iconGrid, iconBattery, iconLoad}

	r := &powerFlowRenderer{
		w:        p,
		bg:       canvas.NewRectangle(color.Transparent),
		invImg:   newIcon(iconInverter),
		invTitle: newText("INVERTER", theme.Color(theme.ColorNameForeground), false, fyne.TextAlignCenter),
		batETA:   newText("", colGreen, false, fyne.TextAlignCenter),
	}
	r.objects = []fyne.CanvasObject{r.bg}
	for i := range r.nodes {
		n := &flowNodeObjs{
			img:    newIcon(icons[i]),
			title:  newText("", theme.Color(theme.ColorNameForeground), true, fyne.TextAlignCenter),
			detail: newText("", theme.Color(theme.ColorNameForeground), false, fyne.TextAlignCenter),
			value:  newText("", theme.Color(theme.ColorNameForeground), false, fyne.TextAlignCenter),
			line:   canvas.NewLine(color.Transparent),
			arrow1: canvas.NewLine(color.Transparent),
			arrow2: canvas.NewLine(color.Transparent),
		}
		n.line.StrokeWidth = 3
		r.nodes[i] = n
		// connector first so icons/text paint on top of it
		r.objects = append(r.objects, n.line, n.arrow1, n.arrow2, n.img, n.title, n.detail, n.value)
	}
	r.objects = append(r.objects, r.invImg, r.invTitle, r.batETA)
	r.Refresh()
	return r
}

// newIcon builds a contained, scalable image from an embedded SVG resource.
func newIcon(res fyne.Resource) *canvas.Image {
	img := canvas.NewImageFromResource(res)
	img.FillMode = canvas.ImageFillContain
	return img
}

func newText(s string, col color.Color, bold bool, align fyne.TextAlign) *canvas.Text {
	t := canvas.NewText(s, col)
	t.TextStyle = fyne.TextStyle{Bold: bold}
	t.Alignment = align
	t.TextSize = theme.TextSize()
	return t
}

func (r *powerFlowRenderer) views() [4]nodeView {
	w := r.w
	return [4]nodeView{
		pvView(w.pv),
		gridView(w.gridP),
		batteryView(w.batP, w.soc),
		loadView(w.loadP),
	}
}

// Refresh re-resolves every node from the widget's data, recolours the
// connectors and labels, then re-lays-out so arrow directions track the data.
func (r *powerFlowRenderer) Refresh() {
	views := r.views()
	for i, v := range views {
		n := r.nodes[i]
		n.title.Text = v.label
		n.title.Color = v.titleCol
		n.detail.Text = v.detail
		n.detail.Color = v.titleCol
		if r.w.hasData {
			n.value.Text = v.value
		} else {
			n.value.Text = "—"
		}
		n.value.Color = theme.Color(theme.ColorNameForeground)

		if r.w.hasData && v.dir != flowIdle {
			n.line.StrokeColor = v.col
			n.arrow1.StrokeColor = v.col
			n.arrow2.StrokeColor = v.col
		} else {
			n.line.StrokeColor = colGray
			n.arrow1.StrokeColor = color.Transparent
			n.arrow2.StrokeColor = color.Transparent
		}
	}
	r.invTitle.Color = theme.Color(theme.ColorNameForeground)

	if r.w.hasData && r.w.etaText != "" {
		r.batETA.Text = "full in " + r.w.etaText
	} else {
		r.batETA.Text = ""
	}

	r.Layout(r.w.Size())
	canvas.Refresh(r.w)
}

func (r *powerFlowRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.bg.Move(fyne.NewPos(0, 0))

	iconSize := clampF(minF(size.Width, size.Height)*0.16, 36, 88)
	invSize := iconSize * 1.3
	cx, cy := size.Width/2, size.Height/2

	// Inverter, centred, with its label just below. The label band is kept to the
	// text's own width (not a wide centred band) and dropped below the icon so it
	// clears the four connectors fanning out from the inverter.
	placeCentered(r.invImg, cx, cy-iconSize*0.18, invSize, invSize)
	placeText(r.invTitle, cx, cy+invSize*0.62, r.invTitle.MinSize().Width)

	views := r.views()
	for i, n := range r.nodes {
		nx := size.Width * nodeCenters[i][0]
		ny := size.Height * nodeCenters[i][1]
		band := size.Width * 0.42

		placeCentered(n.img, nx, ny, iconSize, iconSize)
		if n.detail.Text != "" {
			placeText(n.title, nx, ny-iconSize*0.5-theme.TextSize()*2.6, band)
			placeText(n.detail, nx, ny-iconSize*0.5-theme.TextSize()*1.4, band)
		} else {
			placeText(n.title, nx, ny-iconSize*0.5-theme.TextSize()*1.4, band)
			n.detail.Move(fyne.NewPos(0, -100))
			n.detail.Resize(fyne.NewSize(0, 0))
		}
		placeText(n.value, nx, ny+iconSize*0.5+theme.TextSize()*0.3, band)

		// The battery charge ETA sits one line below the value, in the outward
		// corner — away from the connector arrow that fans toward the inverter.
		if i == batteryNode {
			placeText(r.batETA, nx, ny+iconSize*0.5+theme.TextSize()*1.6, band)
		}

		// Connector from the node's icon edge to the inverter's icon edge.
		dx, dy := cx-nx, cy-ny
		dist := float32(math.Hypot(float64(dx), float64(dy)))
		if dist < 1 {
			dist = 1
		}
		ux, uy := dx/dist, dy/dist
		sx, sy := nx+ux*iconSize*0.62, ny+uy*iconSize*0.62
		ex, ey := cx-ux*invSize*0.62, cy-uy*invSize*0.62
		n.line.Position1 = fyne.NewPos(sx, sy)
		n.line.Position2 = fyne.NewPos(ex, ey)

		// Chevron arrowhead at the connector midpoint, pointing with the flow.
		// flowIn points toward the inverter (centre), flowOut points to the node.
		v := views[i]
		px, py := ux, uy
		if v.dir == flowOut {
			px, py = -ux, -uy
		}
		barb := iconSize * 0.34
		sw := float32(math.Max(3, float64(iconSize)*0.07))
		n.arrow1.StrokeWidth, n.arrow2.StrokeWidth = sw, sw
		mx, my := (sx+ex)/2, (sy+ey)/2
		tipX, tipY := mx+px*barb*0.5, my+py*barb*0.5
		setChevron(n.arrow1, n.arrow2, tipX, tipY, px, py, barb)
	}
}

// setChevron points a two-barb arrowhead at (tipX, tipY) along the unit vector
// (dirX, dirY): each barb runs back from the tip, opened symmetrically.
func setChevron(l1, l2 *canvas.Line, tipX, tipY, dirX, dirY, barbLen float32) {
	const theta = 0.62 // ~35° half-angle between the shaft and each barb
	c, s := float32(math.Cos(theta)), float32(math.Sin(theta))
	bx, by := -dirX, -dirY // barbs run opposite the pointing direction
	tip := fyne.NewPos(tipX, tipY)
	l1.Position1, l1.Position2 = tip, fyne.NewPos(tipX+(bx*c-by*s)*barbLen, tipY+(bx*s+by*c)*barbLen)
	l2.Position1, l2.Position2 = tip, fyne.NewPos(tipX+(bx*c+by*s)*barbLen, tipY+(-bx*s+by*c)*barbLen)
}

func (r *powerFlowRenderer) MinSize() fyne.Size           { return fyne.NewSize(300, 260) }
func (r *powerFlowRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *powerFlowRenderer) Destroy()                     {}

// placeCentered positions and sizes a movable/resizable object so its centre
// lands on (cx, cy).
func placeCentered(o fyne.CanvasObject, cx, cy, w, h float32) {
	o.Resize(fyne.NewSize(w, h))
	o.Move(fyne.NewPos(cx-w/2, cy-h/2))
}

// placeText centres a (centre-aligned) text object's band of the given width on
// cx, with its top at cy minus half a line.
func placeText(t *canvas.Text, cx, cy, band float32) {
	t.Resize(fyne.NewSize(band, t.MinSize().Height))
	t.Move(fyne.NewPos(cx-band/2, cy))
}

func minF(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func clampF(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
