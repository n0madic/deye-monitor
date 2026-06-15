package gui

import (
	"image/color"
	"testing"
)

func TestKW(t *testing.T) {
	t.Parallel()
	cases := []struct {
		watts float64
		want  string
	}{
		{0, "0.00 kW"},
		{2700, "2.70 kW"},
		{-2700, "2.70 kW"}, // magnitude only; sign carried by direction/colour
		{1234, "1.23 kW"},
		{-500, "0.50 kW"},
	}
	for _, c := range cases {
		if got := kW(c.watts); got != c.want {
			t.Errorf("kW(%v) = %q, want %q", c.watts, got, c.want)
		}
	}
}

func TestSOCColor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		soc  float64
		want color.Color
	}{
		{100, colGreen},
		{50, colGreen},
		{49.9, colYellow},
		{20, colYellow},
		{19.9, colRed},
		{0, colRed},
	}
	for _, c := range cases {
		if got := socColor(c.soc); got != c.want {
			t.Errorf("socColor(%v) = %v, want %v", c.soc, got, c.want)
		}
	}
}

func TestNodeViews(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		got      nodeView
		label    string
		value    string
		detail   string
		dir      flowDir
		col      color.Color
		titleCol color.Color
	}{
		{"pv", pvView(1500), "PV", "1.50 kW", "", flowIn, colYellow, colYellow},
		{"grid import", gridView(1000), "GRID", "1.00 kW", "import", flowIn, colRed, colRed},
		{"grid export", gridView(-800), "GRID", "0.80 kW", "export", flowOut, colGreen, colGreen},
		{"grid idle", gridView(0), "GRID", "0.00 kW", "idle", flowIdle, colGray, colGray},
		// Battery: flow colour tracks charge/discharge; title colour tracks SOC health.
		{"bat charge high SOC", batteryView(-900, 80), "BATTERY", "0.90 kW", "80%", flowOut, colGreen, colGreen},
		{"bat discharge mid SOC", batteryView(600, 35), "BATTERY", "0.60 kW", "35%", flowIn, colYellow, colYellow},
		{"bat discharge low SOC", batteryView(600, 12), "BATTERY", "0.60 kW", "12%", flowIn, colYellow, colRed},
		{"bat idle low SOC", batteryView(0, 12), "BATTERY", "0.00 kW", "12%", flowIdle, colGray, colRed},
		{"load", loadView(1200), "LOAD", "1.20 kW", "", flowOut, colBlue, colBlue},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.got.label != c.label {
				t.Errorf("label = %q, want %q", c.got.label, c.label)
			}
			if c.got.value != c.value {
				t.Errorf("value = %q, want %q", c.got.value, c.value)
			}
			if c.got.detail != c.detail {
				t.Errorf("detail = %q, want %q", c.got.detail, c.detail)
			}
			if c.got.dir != c.dir {
				t.Errorf("dir = %v, want %v", c.got.dir, c.dir)
			}
			if c.got.col != c.col {
				t.Errorf("col = %v, want %v", c.got.col, c.col)
			}
			if c.got.titleCol != c.titleCol {
				t.Errorf("titleCol = %v, want %v", c.got.titleCol, c.titleCol)
			}
		})
	}
}
