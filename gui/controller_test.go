package gui

import (
	"errors"
	"sync"
	"testing"
	"time"

	"deye-monitor/deye"
)

// fakeSource is a scripted dataSource for controller tests. Each Snapshot call
// returns the next queued (reading, error) pair.
type fakeSource struct {
	mu      sync.Mutex
	steps   []step
	i       int
	closed  bool
	closeFn func()
}

type step struct {
	r   *deye.Reading
	err error
}

func (f *fakeSource) Snapshot() (*deye.Reading, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.i >= len(f.steps) {
		return nil, errors.New("exhausted")
	}
	s := f.steps[f.i]
	f.i++
	return s.r, s.err
}

func (f *fakeSource) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	if f.closeFn != nil {
		f.closeFn()
	}
	return nil
}

func mkReading(pv, load, grid, bat, soc float64) *deye.Reading {
	return &deye.Reading{
		Time: time.Unix(0, 0),
		Values: map[string]float64{
			"pv1_p":      pv,
			"load_power": load,
			"grid_power": grid,
			"bat_power":  bat,
			"bat_soc":    soc,
		},
		States: map[string]string{},
	}
}

// newTestController builds a controller with synchronous dispatch so onUpdate
// runs inline (no Fyne event loop needed).
func newTestController(src dataSource) *controller {
	c := newController(src, time.Second)
	c.dispatch = func(fn func()) { fn() }
	return c
}

func TestControllerPollStoresLatestAndHistory(t *testing.T) {
	t.Parallel()
	want := mkReading(1500, 800, -200, -300, 75)
	fs := &fakeSource{steps: []step{{r: want}}}
	c := newTestController(fs)

	var gotR *deye.Reading
	var gotErr error
	calls := 0
	c.onUpdate = func(r *deye.Reading, err error) { gotR, gotErr, calls = r, err, calls+1 }

	c.poll()

	if calls != 1 {
		t.Fatalf("onUpdate called %d times, want 1", calls)
	}
	if gotErr != nil {
		t.Fatalf("onUpdate err = %v, want nil", gotErr)
	}
	if gotR != want {
		t.Fatalf("onUpdate reading = %p, want %p", gotR, want)
	}
	if c.latest.Load() != want {
		t.Error("latest pointer not stored")
	}
	if got := c.history("pv"); len(got) != 1 || got[0] != 1500 {
		t.Errorf("pv history = %v, want [1500]", got)
	}
	if got := c.history("grid"); len(got) != 1 || got[0] != 200 { // abs value
		t.Errorf("grid history = %v, want [200] (abs)", got)
	}
	if got := c.history("bat"); len(got) != 1 || got[0] != 300 { // abs value
		t.Errorf("bat history = %v, want [300] (abs)", got)
	}
}

func TestControllerErrorKeepsLastGood(t *testing.T) {
	t.Parallel()
	good := mkReading(1000, 500, 0, 0, 50)
	fs := &fakeSource{steps: []step{
		{r: good},
		{err: errors.New("read timeout")},
	}}
	c := newTestController(fs)

	var lastErr error
	var lastR *deye.Reading
	c.onUpdate = func(r *deye.Reading, err error) { lastR, lastErr = r, err }

	c.poll() // good
	c.poll() // error

	if lastErr == nil {
		t.Fatal("expected error on second poll")
	}
	if lastR != nil {
		t.Errorf("on error onUpdate reading = %p, want nil", lastR)
	}
	if c.latest.Load() != good {
		t.Error("latest must retain the last good reading after an error")
	}
	// History must not gain a sample from the failed poll.
	if got := c.history("pv"); len(got) != 1 {
		t.Errorf("pv history len = %d, want 1 (error poll skipped)", len(got))
	}
}

func TestControllerHistoryAccumulates(t *testing.T) {
	t.Parallel()
	fs := &fakeSource{steps: []step{
		{r: mkReading(100, 0, 0, 0, 0)},
		{r: mkReading(200, 0, 0, 0, 0)},
		{r: mkReading(300, 0, 0, 0, 0)},
	}}
	c := newTestController(fs)
	c.onUpdate = func(*deye.Reading, error) {}

	c.poll()
	c.poll()
	c.poll()

	if got := c.history("pv"); len(got) != 3 || got[0] != 100 || got[2] != 300 {
		t.Errorf("pv history = %v, want [100 200 300]", got)
	}
}

func TestControllerStartAndClose(t *testing.T) {
	t.Parallel()
	done := make(chan struct{}, 4)
	fs := &fakeSource{steps: []step{
		{r: mkReading(1, 0, 0, 0, 0)},
		{r: mkReading(2, 0, 0, 0, 0)},
	}}
	c := newTestController(fs)
	c.onUpdate = func(r *deye.Reading, err error) {
		select {
		case done <- struct{}{}:
		default:
		}
	}

	c.start()
	select {
	case <-done: // immediate first poll fired
	case <-time.After(2 * time.Second):
		t.Fatal("controller did not poll after start")
	}
	if err := c.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !fs.closed {
		t.Error("close must close the source")
	}
}

func TestControllerPauseSkipsTicks(t *testing.T) {
	t.Parallel()
	c := newTestController(&fakeSource{})
	c.setPaused(true)
	if !c.isPaused() {
		t.Error("setPaused(true) not reflected by isPaused")
	}
	c.setPaused(false)
	if c.isPaused() {
		t.Error("setPaused(false) not reflected by isPaused")
	}
}
