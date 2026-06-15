package deye

import "testing"

// TestDecode verifies the Solarman decoding semantics on representative values.
func TestDecode(t *testing.T) {
	regs := map[int]uint16{
		586: 1290,  // battery temp: (1290-1000)*0.1 = 29.0
		587: 5433,  // battery voltage: 5433*0.01 = 54.33
		590: 63416, // battery power signed16: 63416-65536 = -2120
		609: 5004,  // grid freq: 5004*0.01 = 50.04
		676: 4729,  // PV1 voltage: 4729*0.1 = 472.9
		534: 22149, // total PV low word
		535: 0,     // total PV high word -> 22149*0.1 = 2214.9
	}
	check := func(key string, want float64) {
		t.Helper()
		var m Metric
		for _, mm := range Metrics {
			if mm.Key == key {
				m = mm
				break
			}
		}
		got, ok := decode(m, regs)
		if !ok {
			t.Fatalf("decode(%s) not ok", key)
		}
		if got != want {
			t.Fatalf("decode(%s) = %v, want %v", key, got, want)
		}
	}
	check("bat_temp", 29.0)
	check("bat_v", 54.33)
	check("bat_power", -2120)
	check("grid_freq", 50.04)
	check("pv1_v", 472.9)
	check("e_total_pv", 2214.9)
}

// TestDecodeMissingRegister verifies a missing register yields ok=false.
func TestDecodeMissingRegister(t *testing.T) {
	var m Metric
	for _, mm := range Metrics {
		if mm.Key == "e_total_pv" {
			m = mm
			break
		}
	}
	// Only the low word is present; the 32-bit decode must report not-ok.
	if _, ok := decode(m, map[int]uint16{534: 22149}); ok {
		t.Fatalf("decode with a missing high word should be not-ok")
	}
}

func TestDecodeDeviceInfo(t *testing.T) {
	// Rated power 12000 W -> raw 120000 (rule 4, scale 0.1), low word first.
	const rated = 120000
	regs := map[int]uint16{
		3:    uint16('2')<<8 | uint16('5'),
		4:    uint16('0')<<8 | uint16('8'),
		5:    uint16('0')<<8 | uint16('6'),
		6:    uint16('4')<<8 | uint16('1'),
		7:    uint16('6')<<8 | uint16('6'),
		0x14: uint16(rated & 0xFFFF),
		0x15: uint16(rated >> 16),
		0x16: 0x0203, // phases = low nibble = 3
	}
	d := decodeDeviceInfo(regs)
	if d.Serial != "2508064166" {
		t.Fatalf("Serial = %q, want 2508064166", d.Serial)
	}
	if d.RatedPowerW != 12000 {
		t.Fatalf("RatedPowerW = %v, want 12000", d.RatedPowerW)
	}
	if d.Phases != 3 {
		t.Fatalf("Phases = %d, want 3", d.Phases)
	}
	if d.Model != "Deye 12kW 3P" {
		t.Fatalf("Model = %q, want %q", d.Model, "Deye 12kW 3P")
	}
}

func TestDeriveModelUnknown(t *testing.T) {
	if got := deriveModel(0, 0); got != "" {
		t.Fatalf("deriveModel(0,0) = %q, want empty", got)
	}
	if got := deriveModel(12500, 3); got != "Deye 12.5kW 3P" {
		t.Fatalf("deriveModel(12500,3) = %q, want %q", got, "Deye 12.5kW 3P")
	}
}

func TestDecodeSerial(t *testing.T) {
	// "2508064166" packed two ASCII chars per register, regs 3-7.
	regs := map[int]uint16{
		3: uint16('2')<<8 | uint16('5'),
		4: uint16('0')<<8 | uint16('8'),
		5: uint16('0')<<8 | uint16('6'),
		6: uint16('4')<<8 | uint16('1'),
		7: uint16('6')<<8 | uint16('6'),
	}
	if got := decodeSerial(regs); got != "2508064166" {
		t.Fatalf("decodeSerial = %q, want %q", got, "2508064166")
	}
}
