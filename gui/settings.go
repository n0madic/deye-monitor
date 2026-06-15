package gui

import (
	"strconv"
	"time"
)

// Preference keys and defaults. Stored cross-platform via fyne.Preferences.
const (
	keyIP       = "ip"
	keySerial   = "serial"
	keyPort     = "port"
	keyInterval = "interval_sec"
	keyHTTPUser = "http_user"
	keyHTTPPass = "http_pass"
	keyModel    = "model_override"

	defaultPort        = 8899
	defaultIntervalSec = 5
	minIntervalSec     = 1
	maxIntervalSec     = 300
)

// prefStore is the subset of fyne.Preferences the settings layer uses. Declared
// next to its consumer so a fake can back the unit tests. *fyne.Preferences
// (the value returned by app.Preferences()) satisfies it.
type prefStore interface {
	String(key string) string
	StringWithFallback(key, fallback string) string
	SetString(key, value string)
	Int(key string) int
	IntWithFallback(key string, fallback int) int
	SetInt(key string, value int)
}

// settings is the user-configurable connection configuration.
type settings struct {
	IP            string
	Serial        uint32 // 0 means auto-discover from the logger web UI
	Port          int
	IntervalSec   int
	HTTPUser      string
	HTTPPass      string
	ModelOverride string
}

// loadSettings reads the persisted settings, applying defaults for unset fields.
func loadSettings(p prefStore) settings {
	s := settings{
		IP:            p.String(keyIP),
		Port:          p.IntWithFallback(keyPort, defaultPort),
		IntervalSec:   p.IntWithFallback(keyInterval, defaultIntervalSec),
		HTTPUser:      p.String(keyHTTPUser),
		HTTPPass:      p.String(keyHTTPPass),
		ModelOverride: p.String(keyModel),
	}
	if v := p.String(keySerial); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			s.Serial = uint32(n)
		}
	}
	return s.normalized()
}

// save persists the settings. The serial is stored as a decimal string to avoid
// platform int-width concerns; an empty string means auto-discover.
func (s settings) save(p prefStore) {
	s = s.normalized()
	p.SetString(keyIP, s.IP)
	if s.Serial == 0 {
		p.SetString(keySerial, "")
	} else {
		p.SetString(keySerial, strconv.FormatUint(uint64(s.Serial), 10))
	}
	p.SetInt(keyPort, s.Port)
	p.SetInt(keyInterval, s.IntervalSec)
	p.SetString(keyHTTPUser, s.HTTPUser)
	p.SetString(keyHTTPPass, s.HTTPPass)
	p.SetString(keyModel, s.ModelOverride)
}

// normalized clamps numeric fields into valid ranges, applying defaults for
// zero values.
func (s settings) normalized() settings {
	if s.Port == 0 {
		s.Port = defaultPort
	}
	if s.IntervalSec == 0 {
		s.IntervalSec = defaultIntervalSec
	}
	if s.IntervalSec < minIntervalSec {
		s.IntervalSec = minIntervalSec
	}
	if s.IntervalSec > maxIntervalSec {
		s.IntervalSec = maxIntervalSec
	}
	return s
}

// valid reports whether the settings are usable for a connection (an IP is the
// only hard requirement; the serial can be auto-discovered).
func (s settings) valid() bool {
	return s.IP != ""
}

// interval is the poll cadence as a Duration.
func (s settings) interval() time.Duration {
	return time.Duration(s.IntervalSec) * time.Second
}
