package deye

import (
	"testing"
	"time"
)

// feed observes a SOC ramp: count samples, every step apart, starting at
// startSOC and rising by socPerStep each step.
func feed(e *ChargeEstimator, start time.Time, step time.Duration, count int, startSOC, socPerStep float64) {
	for i := range count {
		e.Observe(start.Add(time.Duration(i)*step), startSOC+float64(i)*socPerStep)
	}
}

func TestChargeETACharging(t *testing.T) {
	t.Parallel()
	e := NewChargeEstimator(30 * time.Minute)
	base := time.Unix(1_700_000_000, 0)
	// 50% -> charging at 1%/min (0.5% per 30s step) for 10 minutes.
	feed(e, base, 30*time.Second, 21, 50, 0.5)

	rate, ok := e.RatePerHour()
	if !ok {
		t.Fatal("RatePerHour not available after a clear ramp")
	}
	if rate < 59 || rate > 61 {
		t.Fatalf("rate = %.2f %%/h, want ~60", rate)
	}

	d, ok := e.TimeToFull()
	if !ok {
		t.Fatal("TimeToFull not available while charging")
	}
	// Last sample is 60%; at 1%/min, 40% remain -> ~40 min.
	if d < 39*time.Minute || d > 41*time.Minute {
		t.Fatalf("TimeToFull = %s, want ~40m", d)
	}
}

func TestChargeETADischargingNotAvailable(t *testing.T) {
	t.Parallel()
	e := NewChargeEstimator(30 * time.Minute)
	base := time.Unix(1_700_000_000, 0)
	feed(e, base, 30*time.Second, 21, 80, -0.5) // discharging

	if _, ok := e.TimeToFull(); ok {
		t.Fatal("TimeToFull must be unavailable while discharging")
	}
	rate, ok := e.RatePerHour()
	if !ok || rate >= 0 {
		t.Fatalf("rate = %.2f (ok=%v), want a negative rate while discharging", rate, ok)
	}
}

func TestChargeETANotEnoughSignal(t *testing.T) {
	t.Parallel()
	e := NewChargeEstimator(30 * time.Minute)
	base := time.Unix(1_700_000_000, 0)

	// Too few samples.
	e.Observe(base, 50)
	e.Observe(base.Add(30*time.Second), 51)
	if _, ok := e.TimeToFull(); ok {
		t.Fatal("ETA must be unavailable with fewer than the minimum samples")
	}

	// Enough samples but spanning less than the minimum time window.
	e.Reset()
	feed(e, base, 5*time.Second, 5, 50, 0.2) // spans only 20s < 60s
	if _, ok := e.TimeToFull(); ok {
		t.Fatal("ETA must be unavailable below the minimum time span")
	}
}

func TestChargeETAAlreadyFull(t *testing.T) {
	t.Parallel()
	e := NewChargeEstimator(30 * time.Minute)
	base := time.Unix(1_700_000_000, 0)
	// Rising and already at 100%.
	feed(e, base, 30*time.Second, 21, 90, 0.5) // ends at 100
	if _, ok := e.TimeToFull(); ok {
		t.Fatal("ETA must be unavailable once the battery is full")
	}
}

func TestChargeObserveIgnoresStaleAndGaps(t *testing.T) {
	t.Parallel()
	e := NewChargeEstimator(10 * time.Minute)
	base := time.Unix(1_700_000_000, 0)

	// Out-of-order / duplicate timestamps are dropped.
	e.Observe(base, 50)
	e.Observe(base, 60)                   // duplicate ts: ignored
	e.Observe(base.Add(-time.Minute), 70) // earlier ts: ignored
	if n := len(e.samples); n != 1 {
		t.Fatalf("stale samples not ignored: have %d, want 1", n)
	}

	// A gap longer than the window discards the pre-gap history.
	feed(e, base, 30*time.Second, 5, 50, 0.5) // a few in-window points
	e.Observe(base.Add(time.Hour), 80)        // big jump
	if n := len(e.samples); n != 1 {
		t.Fatalf("gap did not reset history: have %d samples, want 1", n)
	}
}

func TestFormatETA(t *testing.T) {
	t.Parallel()
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "<1m"},
		{45 * time.Minute, "45m"},
		{83 * time.Minute, "1h23m"},
		{2 * time.Hour, "2h00m"},
		{89 * time.Second, "1m"},
	}
	for _, c := range cases {
		if got := FormatETA(c.d); got != c.want {
			t.Errorf("FormatETA(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}
