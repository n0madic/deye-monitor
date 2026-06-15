// Package deye is a reusable, CLI-independent client for reading real-time
// statistics from a Deye SUN-12K-SG05LP3 inverter through its LSW-5 WiFi data
// logger using the Solarman V5 protocol (Modbus over TCP, port 8899). No cloud
// servers involved.
//
// It uses the same validated register map as deye_monitor.py (a curated subset
// of davidrapan/ha-solarman deye_p3.yaml) and decodes values with identical
// semantics. Typical use:
//
//	c := deye.New(deye.Config{IP: "192.168.50.171", Serial: 3566613625})
//	defer c.Close()
//	r, err := c.Snapshot()
//	if err != nil {
//		log.Fatal(err)
//	}
//	soc, _ := r.Get("bat_soc")
package deye

import (
	"math"
	"strconv"
)

// Metric describes how to read and decode one value.
//
// Solarman decoding semantics:
//
//	value = (raw - offset) * scale
//	  - 32-bit values: registers listed low-word-first
//	  - rule 2 / rule 4 -> signed (two's complement over the full width)
//	  - a list scale in the source map -> first element is used here
type Metric struct {
	Key       string
	Label     string
	Registers []int
	Rule      int
	Scale     float64
	Offset    int
	Unit      string
}

// Metrics is the curated register map read each cycle. Keys are stable and used
// as the lookup keys in Reading.Values / Reading.States.
var Metrics = []Metric{
	// status
	{Key: "device_state", Label: "State", Registers: []int{500}, Rule: 1, Scale: 1},
	{Key: "work_mode", Label: "Work mode", Registers: []int{142}, Rule: 1, Scale: 1},
	// solar / PV
	{Key: "pv1_v", Label: "PV1 voltage", Registers: []int{676}, Rule: 1, Scale: 0.1, Unit: "V"},
	{Key: "pv1_i", Label: "PV1 current", Registers: []int{677}, Rule: 1, Scale: 0.1, Unit: "A"},
	{Key: "pv1_p", Label: "PV1 power", Registers: []int{672}, Rule: 1, Scale: 1, Unit: "W"},
	{Key: "pv2_v", Label: "PV2 voltage", Registers: []int{678}, Rule: 1, Scale: 0.1, Unit: "V"},
	{Key: "pv2_i", Label: "PV2 current", Registers: []int{679}, Rule: 1, Scale: 0.1, Unit: "A"},
	{Key: "pv2_p", Label: "PV2 power", Registers: []int{673}, Rule: 1, Scale: 1, Unit: "W"},
	{Key: "pv3_p", Label: "PV3 power", Registers: []int{674}, Rule: 1, Scale: 1, Unit: "W"},
	{Key: "pv4_p", Label: "PV4 power", Registers: []int{675}, Rule: 1, Scale: 1, Unit: "W"},
	// battery
	{Key: "bat_soc", Label: "Battery SOC", Registers: []int{588}, Rule: 1, Scale: 1, Unit: "%"},
	{Key: "bat_v", Label: "Battery voltage", Registers: []int{587}, Rule: 1, Scale: 0.01, Unit: "V"},
	{Key: "bat_power", Label: "Battery power", Registers: []int{590}, Rule: 2, Scale: 1, Unit: "W"},
	{Key: "bat_temp", Label: "Battery temp", Registers: []int{586}, Rule: 1, Scale: 0.1, Offset: 1000, Unit: "°C"},
	// grid (CT / external meter)
	{Key: "grid_l1_v", Label: "Grid L1 voltage", Registers: []int{598}, Rule: 1, Scale: 0.1, Unit: "V"},
	{Key: "grid_l2_v", Label: "Grid L2 voltage", Registers: []int{599}, Rule: 1, Scale: 0.1, Unit: "V"},
	{Key: "grid_l3_v", Label: "Grid L3 voltage", Registers: []int{600}, Rule: 1, Scale: 0.1, Unit: "V"},
	{Key: "grid_freq", Label: "Grid frequency", Registers: []int{609}, Rule: 1, Scale: 0.01, Unit: "Hz"},
	{Key: "grid_l1_p", Label: "Grid L1 power", Registers: []int{622, 687}, Rule: 4, Scale: 1, Unit: "W"},
	{Key: "grid_l2_p", Label: "Grid L2 power", Registers: []int{623, 688}, Rule: 4, Scale: 1, Unit: "W"},
	{Key: "grid_l3_p", Label: "Grid L3 power", Registers: []int{624, 689}, Rule: 4, Scale: 1, Unit: "W"},
	{Key: "grid_power", Label: "Grid power", Registers: []int{625, 690}, Rule: 4, Scale: 1, Unit: "W"},
	// load
	{Key: "load_l1_v", Label: "Load L1 voltage", Registers: []int{644}, Rule: 1, Scale: 0.1, Unit: "V"},
	{Key: "load_l2_v", Label: "Load L2 voltage", Registers: []int{645}, Rule: 1, Scale: 0.1, Unit: "V"},
	{Key: "load_l3_v", Label: "Load L3 voltage", Registers: []int{646}, Rule: 1, Scale: 0.1, Unit: "V"},
	{Key: "load_freq", Label: "Load frequency", Registers: []int{655}, Rule: 1, Scale: 0.01, Unit: "Hz"},
	{Key: "load_l1_p", Label: "Load L1 power", Registers: []int{650, 656}, Rule: 4, Scale: 1, Unit: "W"},
	{Key: "load_l2_p", Label: "Load L2 power", Registers: []int{651, 657}, Rule: 4, Scale: 1, Unit: "W"},
	{Key: "load_l3_p", Label: "Load L3 power", Registers: []int{652, 658}, Rule: 4, Scale: 1, Unit: "W"},
	{Key: "load_power", Label: "Load power", Registers: []int{653, 659}, Rule: 4, Scale: 1, Unit: "W"},
	// temperatures
	{Key: "temp_dc", Label: "DC/heatsink temp", Registers: []int{540}, Rule: 2, Scale: 0.1, Offset: 1000, Unit: "°C"},
	{Key: "temp_ac", Label: "AC/IGBT temp", Registers: []int{541}, Rule: 2, Scale: 0.1, Offset: 1000, Unit: "°C"},
	// energy: today
	{Key: "e_today_pv", Label: "Today PV", Registers: []int{529}, Rule: 1, Scale: 0.1, Unit: "kWh"},
	{Key: "e_today_load", Label: "Today load", Registers: []int{526}, Rule: 1, Scale: 0.1, Unit: "kWh"},
	{Key: "e_today_chg", Label: "Today battery charge", Registers: []int{514}, Rule: 1, Scale: 0.1, Unit: "kWh"},
	{Key: "e_today_dis", Label: "Today battery discharge", Registers: []int{515}, Rule: 1, Scale: 0.1, Unit: "kWh"},
	{Key: "e_today_imp", Label: "Today grid import", Registers: []int{520}, Rule: 1, Scale: 0.1, Unit: "kWh"},
	{Key: "e_today_exp", Label: "Today grid export", Registers: []int{521}, Rule: 1, Scale: 0.1, Unit: "kWh"},
	// energy: total
	{Key: "e_total_pv", Label: "Total PV", Registers: []int{534, 535}, Rule: 3, Scale: 0.1, Unit: "kWh"},
	{Key: "e_total_load", Label: "Total load", Registers: []int{527, 528}, Rule: 3, Scale: 0.1, Unit: "kWh"},
	{Key: "e_total_chg", Label: "Total battery charge", Registers: []int{516, 517}, Rule: 3, Scale: 0.1, Unit: "kWh"},
	{Key: "e_total_dis", Label: "Total battery discharge", Registers: []int{518, 519}, Rule: 3, Scale: 0.1, Unit: "kWh"},
	{Key: "e_total_imp", Label: "Total grid import", Registers: []int{522, 523}, Rule: 3, Scale: 0.1, Unit: "kWh"},
	{Key: "e_total_exp", Label: "Total grid export", Registers: []int{524, 525}, Rule: 3, Scale: 0.1, Unit: "kWh"},
}

// DeviceState and WorkMode map the raw enum codes (device_state / work_mode) to
// human-readable labels.
var (
	DeviceState = map[int]string{0: "Standby", 1: "Self-check", 2: "Normal", 3: "Alarm", 4: "Fault"}
	WorkMode    = map[int]string{0: "Selling First", 1: "Zero Export To Load", 2: "Zero Export To CT"}
)

// liveBlocks are contiguous (start,count) Modbus reads covering every register
// in Metrics.
var liveBlocks = [][2]int{
	{142, 1},   // work mode
	{500, 42},  // state(500), energy counters(514-535), temps(540-541)
	{586, 105}, // battery / grid / load / PV (586-690)
}

// decode applies the Solarman semantics; ok=false if a register is missing.
func decode(m Metric, regs map[int]uint16) (val float64, ok bool) {
	var raw uint64
	for i, r := range m.Registers {
		v, present := regs[r]
		if !present {
			return 0, false
		}
		raw |= uint64(v) << (16 * i)
	}
	bits := 16 * len(m.Registers)
	var signed int64
	if m.Rule == 2 || m.Rule == 4 {
		if raw >= (uint64(1) << (bits - 1)) {
			signed = int64(raw) - (int64(1) << bits)
		} else {
			signed = int64(raw)
		}
	} else {
		signed = int64(raw)
	}
	val = (float64(signed) - float64(m.Offset)) * m.Scale
	// Round to 3 decimals to strip float64 representation noise
	// (e.g. 4729 * 0.1 -> 472.90000000000003 -> 472.9).
	return math.Round(val*1000) / 1000, true
}

// decodeSerial extracts the logger serial from the device-info block (regs 3-7).
func decodeSerial(regs map[int]uint16) string {
	b := make([]byte, 0, 10)
	for a := 3; a <= 7; a++ {
		v, ok := regs[a]
		if !ok {
			continue
		}
		b = append(b, byte(v>>8), byte(v&0xFF))
	}
	// trim spaces / NULs
	end := len(b)
	for end > 0 && (b[end-1] == ' ' || b[end-1] == 0) {
		end--
	}
	return string(b[:end])
}

// lookup resolves an enum code to its label, falling back to the raw number.
func lookup(m map[int]string, k int) string {
	if s, ok := m[k]; ok {
		return s
	}
	return strconv.Itoa(k)
}

// ratedPowerMetric decodes Device Rated Power (registers 0x14-0x15, rule 4,
// scale 0.1) into watts.
var ratedPowerMetric = Metric{Registers: []int{0x14, 0x15}, Rule: 4, Scale: 0.1}

// decodeDeviceInfo extracts the static device identity from the device-info
// block (registers 0-24): serial, rated power, phase count, and a derived
// model label.
func decodeDeviceInfo(regs map[int]uint16) DeviceInfo {
	d := DeviceInfo{Serial: decodeSerial(regs)}
	if w, ok := decode(ratedPowerMetric, regs); ok {
		d.RatedPowerW = w
	}
	if v, ok := regs[0x16]; ok {
		d.Phases = int(v & 0x000F)
	}
	d.Model = deriveModel(d.RatedPowerW, d.Phases)
	return d
}

// deriveModel builds a model label from the rated power and phase count, e.g.
// "Deye 12kW 3P". It returns "" when the rated power is unknown.
func deriveModel(ratedW float64, phases int) string {
	if ratedW <= 0 {
		return ""
	}
	model := "Deye " + strconv.FormatFloat(ratedW/1000, 'f', -1, 64) + "kW"
	if phases > 0 {
		model += " " + strconv.Itoa(phases) + "P"
	}
	return model
}
