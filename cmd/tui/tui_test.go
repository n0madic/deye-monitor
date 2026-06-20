package main

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/n0madic/deye-monitor/deye"

	ui "github.com/gizak/termui/v3"
)

// bufferText flattens a rendered termui buffer into newline-separated text so
// tests can assert on the drawn glyphs.
func bufferText(buf *ui.Buffer) string {
	r := buf.Rectangle
	var b strings.Builder
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := buf.GetCell(image.Pt(x, y))
			if c.Rune == 0 {
				b.WriteRune(' ')
			} else {
				b.WriteRune(c.Rune)
			}
		}
		b.WriteRune('\n')
	}
	return b.String()
}

// sampleReading builds a synthetic snapshot for the widget-update tests.
func sampleReading() *deye.Reading {
	return &deye.Reading{
		Time:   time.Now(),
		Serial: "2508064166",
		States: map[string]string{"device_state": "Normal", "work_mode": "Zero Export To Load"},
		Values: map[string]float64{
			"pv1_p": 245, "pv2_p": 249, "pv3_p": 0, "pv4_p": 0,
			"bat_soc": 80, "bat_power": -2120, "bat_temp": 29,
			"grid_power": 11, "grid_freq": 49.95,
			"load_power":   420,
			"e_today_pv":   12.7,
			"e_today_load": 38.1,
		},
	}
}

// TestTUIUpdate exercises the widget-update path without ui.Init() (which needs
// a real terminal). Building the grid and setting widget fields is pure data.
func TestTUIUpdate(t *testing.T) {
	tu := newTUI("192.168.50.171", "Deye SUN-12K", 5*time.Second)
	tu.update(sampleReading())

	if tu.gauge.Percent != 80 {
		t.Fatalf("gauge.Percent = %d, want 80", tu.gauge.Percent)
	}
	if tu.gauge.BarColor != ui.ColorGreen {
		t.Fatalf("SOC 80%% should be green, got color %v", tu.gauge.BarColor)
	}
	if !strings.Contains(tu.gauge.Label, "chg") {
		t.Fatalf("negative bat_power should label as charging, got %q", tu.gauge.Label)
	}

	// Flow diagram values: PV 494 W, grid +11 (import), bat -2120 (charge), load 420.
	if tu.flow.pv != 494 || tu.flow.gridP != 11 || tu.flow.batP != -2120 || tu.flow.loadP != 420 {
		t.Fatalf("flow values not set correctly: %+v", tu.flow)
	}
	// Render the diagram into an off-screen buffer and check the drawn glyphs.
	tu.flow.SetRect(0, 0, 60, 16)
	buf := ui.NewBuffer(tu.flow.GetRect())
	tu.flow.Draw(buf)
	txt := bufferText(buf)
	if !strings.Contains(txt, "INV") {
		t.Fatalf("diagram should draw the inverter box, got:\n%s", txt)
	}
	for _, node := range []string{"PV", "GRID", "BATTERY", "LOAD"} {
		if !strings.Contains(txt, node) {
			t.Fatalf("diagram should label node %q, got:\n%s", node, txt)
		}
	}
	if !strings.Contains(txt, "▶") {
		t.Fatalf("diagram should draw flow arrows, got:\n%s", txt)
	}

	if !strings.Contains(tu.keyvals.Text, "Today PV") {
		t.Fatalf("keyvals should include today PV, got %q", tu.keyvals.Text)
	}

	if len(tu.slPV.Data) != 1 || tu.slPV.Data[0] != 494 {
		t.Fatalf("PV sparkline should have one sample of 494, got %v", tu.slPV.Data)
	}
	// Grid/battery sparklines store absolute magnitudes.
	if len(tu.slBat.Data) != 1 || tu.slBat.Data[0] != 2120 {
		t.Fatalf("battery sparkline should store abs power 2120, got %v", tu.slBat.Data)
	}
	if tu.slBat.MaxVal < 2120 {
		t.Fatalf("sparkline MaxVal must cover the data, got %v", tu.slBat.MaxVal)
	}
	// Title carries the min/max over the history period.
	if !strings.Contains(tu.slPV.Title, "↓") || !strings.Contains(tu.slPV.Title, "↑") {
		t.Fatalf("sparkline title should show min/max markers, got %q", tu.slPV.Title)
	}

	if len(tu.details.Rows) != 7 {
		t.Fatalf("details should have 7 rows, got %d", len(tu.details.Rows))
	}

	// A second sample should accumulate in the ring buffers.
	tu.update(sampleReading())
	if len(tu.slPV.Data) != 2 {
		t.Fatalf("second update should append to history, got len %d", len(tu.slPV.Data))
	}
}

// TestTUIHeaderHeartbeat verifies the header surfaces the heartbeat indicator.
func TestTUIHeaderHeartbeat(t *testing.T) {
	tu := newTUI("10.0.0.1", "Deye", time.Second)
	r := sampleReading()
	r.Heartbeats = 5
	r.HeartbeatNow = true
	r.LastHeartbeat = time.Now()
	tu.update(r)
	if !strings.Contains(tu.header.Text, "♥") {
		t.Fatalf("header should show filled ♥ on a live heartbeat, got %q", tu.header.Text)
	}
	if !strings.Contains(tu.header.Text, "×5") {
		t.Fatalf("header should show heartbeat count ×5, got %q", tu.header.Text)
	}
}

// TestTUIPauseStatus verifies the pause flag shows in the header status line.
func TestTUIPauseStatus(t *testing.T) {
	tu := newTUI("10.0.0.1", "Deye", time.Second)
	tu.update(sampleReading())
	if strings.Contains(tu.header.Text, "PAUSED") {
		t.Fatalf("header should not show PAUSED before pausing")
	}
	tu.paused = true
	tu.renderHeader()
	if !strings.Contains(tu.header.Text, "PAUSED") {
		t.Fatalf("header should show PAUSED after pausing, got %q", tu.header.Text)
	}
}
