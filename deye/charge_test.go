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

// TestChargeETAIntegerSOC mirrors the real device: SOC is reported as whole
// percent, so the trend has to be fitted across several 1% steps. A realistic
// ~8%/h charge ticks once every ~7.5 min; over a 45-minute window that is ~6
// steps, enough for a stable rate and a sane time-to-full.
func TestChargeETAIntegerSOC(t *testing.T) {
	t.Parallel()
	e := NewChargeEstimator(0) // default 45-minute window
	base := time.Unix(1_700_000_000, 0)

	// 1% steps 7.5 min apart starting at 70% => ~8%/h. Poll every 30s so each
	// step is held across many samples, exactly like the live poller.
	soc := 70.0
	last := base
	for i := range 91 { // 91 * 30s = 45 min
		ts := base.Add(time.Duration(i) * 30 * time.Second)
		if ts.Sub(last) >= 450*time.Second && soc < 100 {
			soc++
			last = ts
		}
		e.Observe(ts, soc)
	}

	rate, ok := e.RatePerHour()
	if !ok {
		t.Fatal("RatePerHour not available after a full window of integer-SOC steps")
	}
	if rate < 6 || rate > 10 {
		t.Fatalf("rate = %.2f %%/h, want ~8", rate)
	}

	d, ok := e.TimeToFull()
	if !ok {
		t.Fatal("TimeToFull not available while charging")
	}
	// ~76% with ~24% remaining at ~8%/h => roughly 3 hours, not the minutes a
	// short window would have produced from the most recent single step.
	if d < 2*time.Hour || d > 4*time.Hour {
		t.Fatalf("TimeToFull = %s, want ~3h", d)
	}
}

// TestChargeETAIgnoresSingleStep is the regression guard for the warm-up
// artifact: a lone +1% quantization step within the window must not yield an
// ETA, however the samples are spaced.
func TestChargeETAIgnoresSingleStep(t *testing.T) {
	t.Parallel()
	e := NewChargeEstimator(0)
	base := time.Unix(1_700_000_000, 0)

	// Flat at 70% for 5 min, then a single +1% tick, then flat at 71%. Net
	// movement is only 1% (< minChargeDelta), so no estimate yet.
	for i := range 10 {
		e.Observe(base.Add(time.Duration(i)*30*time.Second), 70)
	}
	for i := 10; i < 20; i++ {
		e.Observe(base.Add(time.Duration(i)*30*time.Second), 71)
	}
	if _, ok := e.TimeToFull(); ok {
		t.Fatal("a single 1% step must not produce a time-to-full estimate")
	}
	if _, ok := e.RatePerHour(); ok {
		t.Fatal("a single 1% step must not produce a rate")
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
