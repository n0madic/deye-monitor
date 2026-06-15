package deye

import "time"

// Reading holds one decoded snapshot of the inverter state.
type Reading struct {
	Time          time.Time          // when this snapshot was taken
	Serial        string             // inverter serial read from the device (empty if lookup failed)
	Model         string             // model label derived from the device (e.g. "Deye 12kW 3P")
	Values        map[string]float64 // metric key -> decoded value
	States        map[string]string  // decoded enum fields ("device_state", "work_mode")
	Heartbeats    int                // total heartbeats seen so far
	HeartbeatNow  bool               // a heartbeat arrived during this cycle
	LastHeartbeat time.Time          // when the most recent heartbeat arrived
}

// DeviceInfo is the static identity read from the device-info register block.
// It is read once and cached by the Client.
type DeviceInfo struct {
	Serial      string  // inverter serial (registers 3-7, ASCII)
	RatedPowerW float64 // device rated power in watts (registers 0x14-0x15)
	Phases      int     // number of AC phases (register 0x16, low nibble)
	Model       string  // derived label, e.g. "Deye 12kW 3P" ("" if rated power unknown)
}

// Get returns the value for key; ok=false if the metric was missing.
func (r *Reading) Get(key string) (float64, bool) {
	v, ok := r.Values[key]
	return v, ok
}

// State returns the decoded enum label for key (e.g. "device_state"), or "" if
// absent.
func (r *Reading) State(key string) string {
	return r.States[key]
}

// PVTotal sums the four PV-string power readings.
func (r *Reading) PVTotal() float64 {
	var s float64
	for _, k := range []string{"pv1_p", "pv2_p", "pv3_p", "pv4_p"} {
		s += r.Values[k]
	}
	return s
}
