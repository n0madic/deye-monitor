package gui

import (
	"errors"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"github.com/n0madic/deye-monitor/deye"
)

// These tests render the custom-drawn widgets and views through Fyne's headless
// test driver (no GL, no display). They exercise CreateRenderer/Layout/Refresh
// so layout-math or nil-pointer regressions surface without a real device.

func TestPowerFlowRenders(t *testing.T) {
	test.NewApp()
	pf := newPowerFlow()
	w := test.NewWindow(pf)
	defer w.Close()
	w.Resize(fyne.NewSize(540, 480))

	pf.Refresh()                                         // no-data state
	pf.setReading(mkReading(2500, 1200, -300, -800, 76)) // charge + export
	w.Resize(fyne.NewSize(360, 600))                     // force re-layout
	pf.setReading(mkReading(0, 900, 1500, 600, 18))      // import + discharge, low SOC

	// Charge ETA renders as its own line in the battery node.
	pf.setReading(mkReading(2500, 1200, -300, -800, 76))
	pf.setChargeETA("1h23m")
	rr := pf.CreateRenderer().(*powerFlowRenderer)
	if got := rr.batETA.Text; got != "full in 1h23m" {
		t.Fatalf("battery node ETA = %q, want %q", got, "full in 1h23m")
	}
	pf.setChargeETA("") // cleared when not charging
	rr = pf.CreateRenderer().(*powerFlowRenderer)
	if got := rr.batETA.Text; got != "" {
		t.Fatalf("battery node ETA should clear, got %q", got)
	}
}

func TestChartRenders(t *testing.T) {
	test.NewApp()
	c := newChart("PV", colYellow)
	w := test.NewWindow(c)
	defer w.Close()
	w.Resize(fyne.NewSize(400, 120))

	c.setData(nil, "")                          // empty: no polyline
	c.setData([]float64{0, 100, 250, 80}, "")   // small series
	c.setData([]float64{500, 500, 500}, "flat") // degenerate (min==max)
}

func TestDetailsAndHeaderRender(t *testing.T) {
	test.NewApp()
	d := newDetailsView()
	h := newHeaderView()
	w := test.NewWindow(container.NewVBox(h.root, d.root))
	defer w.Close()
	w.Resize(fyne.NewSize(540, 700))

	r := mkReading(2500, 1200, -300, -800, 76)
	r.Serial = "2306123456"
	r.Model = "Deye 12kW 3P"
	r.States["device_state"] = "Normal"
	r.States["work_mode"] = "Zero Export To CT"
	d.update(r)
	d.setChargeETA("1h23m") // charging estimate
	if got := d.eta.Text; got != "1h23m" {
		t.Fatalf("battery time-to-full = %q, want %q", got, "1h23m")
	}
	d.setChargeETA("") // not charging / no estimate -> dash
	if got := d.eta.Text; got != "—" {
		t.Fatalf("empty ETA should clear to dash, got %q", got)
	}
	h.update(r, "")
	h.setStale(true, errors.New("read timeout"))
	h.setStale(false, nil)
}

func TestHistoryViewRenders(t *testing.T) {
	test.NewApp()
	hv := newHistoryView()
	w := test.NewWindow(hv.root)
	defer w.Close()
	w.Resize(fyne.NewSize(540, 600))

	c := newTestController(&fakeSource{steps: []step{
		{r: mkReading(1000, 800, -200, -300, 75)},
		{r: mkReading(1200, 850, 400, 500, 60)},
	}})
	c.onUpdate = func(*deye.Reading, error) {}
	c.poll()
	c.poll()
	hv.refresh(c)
}
