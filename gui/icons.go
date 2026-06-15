package gui

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

// Node icons, embedded so the binary is self-contained on every platform.

//go:embed assets/pv.svg
var pvSVG []byte

//go:embed assets/grid.svg
var gridSVG []byte

//go:embed assets/battery.svg
var batterySVG []byte

//go:embed assets/load.svg
var loadSVG []byte

//go:embed assets/inverter.svg
var inverterSVG []byte

var (
	iconPV       = fyne.NewStaticResource("pv.svg", pvSVG)
	iconGrid     = fyne.NewStaticResource("grid.svg", gridSVG)
	iconBattery  = fyne.NewStaticResource("battery.svg", batterySVG)
	iconLoad     = fyne.NewStaticResource("load.svg", loadSVG)
	iconInverter = fyne.NewStaticResource("inverter.svg", inverterSVG)
)
