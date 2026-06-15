package gui

import (
	"fmt"
	"image/color"
	"math"
)

// Power-flow palette, in the spirit of the Deye Cloud flow diagram. These are
// the semantic colours used across the power-flow widget, history charts and
// detail accents.
var (
	colYellow = color.NRGBA{R: 0xF5, G: 0xA6, B: 0x23, A: 0xFF} // PV / battery discharge
	colGreen  = color.NRGBA{R: 0x2E, G: 0xCC, B: 0x71, A: 0xFF} // grid export / battery charge
	colRed    = color.NRGBA{R: 0xE7, G: 0x4C, B: 0x3C, A: 0xFF} // grid import / low SOC
	colBlue   = color.NRGBA{R: 0x34, G: 0x98, B: 0xDB, A: 0xFF} // load
	colGray   = color.NRGBA{R: 0x9E, G: 0x9E, B: 0x9E, A: 0xFF} // idle / no flow
)

// flowDir is the direction of power on a node relative to the central inverter.
type flowDir int

const (
	flowIdle flowDir = iota // no significant flow
	flowIn                  // power flowing into the inverter (node -> inverter)
	flowOut                 // power flowing out of the inverter (inverter -> node)
)

// nodeView is the resolved presentation of one power-flow node: what to label
// it, the magnitude in kW, an optional detail tag, the flow direction and the
// colours that encode its state.
type nodeView struct {
	label    string      // node name, e.g. "GRID"
	value    string      // magnitude, e.g. "1.23 kW"
	detail   string      // optional tag, e.g. "import" / "65%"
	dir      flowDir     // arrow direction
	col      color.Color // flow colour for the connector line/arrow
	titleCol color.Color // label colour (battery encodes SOC health here)
}

// kW formats a power value as a Deye-Cloud-style magnitude string: the absolute
// value in kilowatts with two decimals (e.g. -2700 W -> "2.70 kW"). The sign is
// conveyed separately by direction and colour, never by the number.
func kW(watts float64) string {
	return fmt.Sprintf("%.2f kW", math.Abs(watts)/1000)
}

// socColor maps a battery state-of-charge percentage to a status colour:
// green >=50, yellow >=20, red below 20.
func socColor(soc float64) color.Color {
	switch {
	case soc >= 50:
		return colGreen
	case soc >= 20:
		return colYellow
	default:
		return colRed
	}
}

// pvView resolves the PV (solar) node. PV power always flows into the inverter.
func pvView(pvTotal float64) nodeView {
	return nodeView{label: "PV", value: kW(pvTotal), dir: flowIn, col: colYellow, titleCol: colYellow}
}

// gridView resolves the grid node from grid_power: positive imports from the
// grid (into the inverter, red), negative exports to the grid (out, green),
// zero is idle (gray).
func gridView(gridP float64) nodeView {
	var v nodeView
	switch {
	case gridP > 0:
		v = nodeView{label: "GRID", value: kW(gridP), detail: "import", dir: flowIn, col: colRed}
	case gridP < 0:
		v = nodeView{label: "GRID", value: kW(gridP), detail: "export", dir: flowOut, col: colGreen}
	default:
		v = nodeView{label: "GRID", value: kW(0), detail: "idle", dir: flowIdle, col: colGray}
	}
	v.titleCol = v.col
	return v
}

// batteryView resolves the battery node from bat_power and bat_soc: negative is
// charging (inverter -> battery, out, green), positive is discharging (battery
// -> inverter, in, yellow), zero is idle. The detail carries the SOC percentage,
// and the label colour encodes SOC health (green/yellow/red by threshold).
func batteryView(batP, soc float64) nodeView {
	v := nodeView{label: "BATTERY", value: kW(batP), detail: fmt.Sprintf("%.0f%%", soc), titleCol: socColor(soc)}
	switch {
	case batP < 0:
		v.dir, v.col = flowOut, colGreen // charging
	case batP > 0:
		v.dir, v.col = flowIn, colYellow // discharging
	default:
		v.dir, v.col = flowIdle, colGray
	}
	return v
}

// loadView resolves the load node. Load power always flows out of the inverter.
func loadView(loadP float64) nodeView {
	return nodeView{label: "LOAD", value: kW(loadP), dir: flowOut, col: colBlue, titleCol: colBlue}
}
