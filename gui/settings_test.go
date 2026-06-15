package gui

import "testing"

// fakePrefs is an in-memory prefStore for tests.
type fakePrefs struct {
	str map[string]string
	in  map[string]int
}

func newFakePrefs() *fakePrefs {
	return &fakePrefs{str: map[string]string{}, in: map[string]int{}}
}

func (p *fakePrefs) String(k string) string { return p.str[k] }
func (p *fakePrefs) StringWithFallback(k, fb string) string {
	if v, ok := p.str[k]; ok {
		return v
	}
	return fb
}
func (p *fakePrefs) SetString(k, v string) { p.str[k] = v }
func (p *fakePrefs) Int(k string) int      { return p.in[k] }
func (p *fakePrefs) IntWithFallback(k string, fb int) int {
	if v, ok := p.in[k]; ok {
		return v
	}
	return fb
}
func (p *fakePrefs) SetInt(k string, v int) { p.in[k] = v }

func TestLoadSettingsDefaults(t *testing.T) {
	t.Parallel()
	s := loadSettings(newFakePrefs())
	if s.IP != "" {
		t.Errorf("IP = %q, want empty", s.IP)
	}
	if s.Serial != 0 {
		t.Errorf("Serial = %d, want 0", s.Serial)
	}
	if s.Port != defaultPort {
		t.Errorf("Port = %d, want %d", s.Port, defaultPort)
	}
	if s.IntervalSec != defaultIntervalSec {
		t.Errorf("IntervalSec = %d, want %d", s.IntervalSec, defaultIntervalSec)
	}
	if s.valid() {
		t.Error("settings without IP must be invalid")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	t.Parallel()
	p := newFakePrefs()
	in := settings{
		IP:            "192.168.50.171",
		Serial:        3566613625,
		Port:          8899,
		IntervalSec:   3,
		HTTPUser:      "admin",
		HTTPPass:      "secret",
		ModelOverride: "Deye 12kW 3P",
	}
	in.save(p)
	out := loadSettings(p)
	if out != in {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", out, in)
	}
	if !out.valid() {
		t.Error("settings with IP must be valid")
	}
}

func TestSettingsSerialZeroIsAutoDiscover(t *testing.T) {
	t.Parallel()
	p := newFakePrefs()
	settings{IP: "10.0.0.2", Serial: 0}.save(p)
	if got := p.str[keySerial]; got != "" {
		t.Errorf("serial 0 must persist as empty string, got %q", got)
	}
	if out := loadSettings(p); out.Serial != 0 {
		t.Errorf("reloaded Serial = %d, want 0", out.Serial)
	}
}

func TestSettingsNormalizeClampsInterval(t *testing.T) {
	t.Parallel()
	if got := (settings{IP: "x", IntervalSec: 9000}).normalized().IntervalSec; got != maxIntervalSec {
		t.Errorf("interval clamp high = %d, want %d", got, maxIntervalSec)
	}
	if got := (settings{IP: "x", IntervalSec: -5}).normalized().IntervalSec; got != minIntervalSec {
		t.Errorf("interval clamp low = %d, want %d", got, minIntervalSec)
	}
	if got := (settings{IP: "x"}).normalized().IntervalSec; got != defaultIntervalSec {
		t.Errorf("zero interval default = %d, want %d", got, defaultIntervalSec)
	}
}

func TestSettingsInterval(t *testing.T) {
	t.Parallel()
	if got := (settings{IntervalSec: 5}).interval().Seconds(); got != 5 {
		t.Errorf("interval() = %vs, want 5s", got)
	}
}
