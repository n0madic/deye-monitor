// Command gui is the cross-platform (desktop + mobile) graphical monitor
// for a Deye SUN-12K-SG05LP3 inverter. It is a thin entry point over the gui
// package, which renders the live data read by the deye-monitor/deye core.
//
//	go run ./cmd/gui     # run on the desktop (same Wi-Fi as the logger)
//
// Packaging (from this directory):
//
//	fyne package -os darwin  -icon Icon.png
//	fyne package -os android -app-id com.deye.monitor -icon Icon.png
package main

import "github.com/n0madic/deye-monitor/gui"

func main() {
	gui.Main()
}
