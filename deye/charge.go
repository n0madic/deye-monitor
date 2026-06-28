package deye

import (
	"fmt"
	"time"
)

// Charge-estimator tuning. The device never reports its battery capacity, so the
// time-to-full is derived purely from the observed state-of-charge (SOC) trend.
const (
	defaultChargeWindow = 10 * time.Minute // sliding window the SOC rate is fitted over
	minChargeSamples    = 3                // need at least this many points to fit a line
	minChargeSpan       = 60 * time.Second // ...spread over at least this much time
	maxChargeETA        = 48 * time.Hour   // estimates beyond this are treated as noise
	fullSOC             = 100.0            // a full battery, in percent
)

// socSample is one timestamped state-of-charge observation.
type socSample struct {
	t   time.Time
	soc float64
}

// ChargeEstimator projects the time remaining until the battery finishes
// charging. It records the SOC over a sliding time window and fits the rate of
// change by least squares, so it needs no battery-capacity configuration: the
// inverter never reports its pack size, but the SOC trend alone yields an ETA.
//
// A ChargeEstimator is not safe for concurrent use; callers serialise Observe
// and the query methods on a single goroutine (the polling loop).
type ChargeEstimator struct {
	window  time.Duration
	samples []socSample
}

// NewChargeEstimator returns an estimator that fits the SOC trend over the given
// sliding window. A non-positive window selects the default (10 minutes).
func NewChargeEstimator(window time.Duration) *ChargeEstimator {
	if window <= 0 {
		window = defaultChargeWindow
	}
	return &ChargeEstimator{window: window}
}

// Observe records a SOC reading (percent) taken at time t. Samples that are out
// of order or repeat the previous timestamp are ignored so the fit stays
// well-conditioned. A gap longer than the window (e.g. after a sleep or
// reconnect) is treated as a discontinuity and clears the history first, so the
// trend is never fitted across a stale jump.
func (e *ChargeEstimator) Observe(t time.Time, soc float64) {
	if n := len(e.samples); n > 0 {
		last := e.samples[n-1]
		if !t.After(last.t) {
			return
		}
		if t.Sub(last.t) > e.window {
			e.samples = e.samples[:0]
		}
	}
	e.samples = append(e.samples, socSample{t: t, soc: soc})
	e.prune(t)
}

// prune drops samples that have aged out of the window relative to now.
func (e *ChargeEstimator) prune(now time.Time) {
	cutoff := now.Add(-e.window)
	i := 0
	for i < len(e.samples) && e.samples[i].t.Before(cutoff) {
		i++
	}
	if i > 0 {
		e.samples = e.samples[i:]
	}
}

// Reset clears the observation history, e.g. after a reconnect.
func (e *ChargeEstimator) Reset() { e.samples = e.samples[:0] }

// RatePerHour returns the fitted SOC rate of change in percentage points per
// hour (positive while charging, negative while discharging) and whether enough
// signal exists to report it.
func (e *ChargeEstimator) RatePerHour() (float64, bool) {
	slope, ok := e.slopePerSec()
	if !ok {
		return 0, false
	}
	return slope * 3600, true
}

// ETA returns the projected time until the battery reaches targetSOC and whether
// an estimate is currently available. It reports ok=false when the SOC is not
// moving toward the target, the target is already reached, there is not yet
// enough signal to fit a trend, or the projection exceeds a sane upper bound.
func (e *ChargeEstimator) ETA(targetSOC float64) (time.Duration, bool) {
	slope, ok := e.slopePerSec()
	if !ok || slope <= 0 {
		return 0, false
	}
	remaining := targetSOC - e.samples[len(e.samples)-1].soc
	if remaining <= 0 {
		return 0, false
	}
	d := time.Duration(remaining / slope * float64(time.Second))
	if d > maxChargeETA {
		return 0, false
	}
	return d, true
}

// TimeToFull is ETA to a full (100%) battery.
func (e *ChargeEstimator) TimeToFull() (time.Duration, bool) { return e.ETA(fullSOC) }

// slopePerSec fits SOC (percent) against time (seconds) by least squares and
// returns the slope in percent per second. ok=false when there are too few
// samples or they do not span enough time to fit a stable line.
func (e *ChargeEstimator) slopePerSec() (float64, bool) {
	n := len(e.samples)
	if n < minChargeSamples {
		return 0, false
	}
	t0 := e.samples[0].t
	if e.samples[n-1].t.Sub(t0) < minChargeSpan {
		return 0, false
	}
	var sx, sy, sxx, sxy float64
	for _, s := range e.samples {
		x := s.t.Sub(t0).Seconds()
		y := s.soc
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	fn := float64(n)
	denom := fn*sxx - sx*sx
	if denom == 0 {
		return 0, false
	}
	return (fn*sxy - sx*sy) / denom, true
}

// FormatETA renders a charge-time duration as a compact label: "1h23m", "45m",
// or "<1m" for sub-minute estimates. Durations are rounded to the nearest minute.
func FormatETA(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	d = d.Round(time.Minute)
	h := d / time.Hour
	m := (d % time.Hour) / time.Minute
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
