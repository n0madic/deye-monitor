package gui

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"

	"github.com/n0madic/deye-monitor/deye"
)

// historyLen is the number of samples each history ring buffer retains.
const historyLen = 120

// histKeys are the four series the history charts plot, in display order.
var histKeys = []string{"pv", "load", "grid", "bat"}

// dataSource is the minimal surface the controller needs from a Deye client.
// *deye.Client satisfies it; tests supply a fake. Declared next to its consumer
// per Go convention.
type dataSource interface {
	Snapshot() (*deye.Reading, error)
	Close() error
}

// controller owns the single polling goroutine that talks to the (not
// concurrency-safe) Deye client. The UI reads the latest reading via an
// atomic pointer and the history buffers under a mutex; only the poller writes
// them. UI repaints are marshalled onto the Fyne main thread via dispatch.
type controller struct {
	mu       sync.Mutex
	src      dataSource
	hist     map[string]*series
	interval time.Duration

	latest atomic.Pointer[deye.Reading]
	paused atomic.Bool

	// onUpdate is invoked (via dispatch, i.e. on the UI thread) after every
	// poll. On success r is the fresh reading and err is nil; on failure r is
	// nil and err is set — the last good reading remains available via latest.
	onUpdate func(r *deye.Reading, err error)

	// dispatch marshals a function onto the UI thread. Defaults to fyne.Do;
	// tests replace it with a synchronous call.
	dispatch func(func())

	refresh chan struct{} // request an immediate poll
	stop    chan struct{} // ask the loop to exit
	done    chan struct{} // closed when the loop has exited
}

// newController builds a controller over src with the given poll interval and
// empty history buffers.
func newController(src dataSource, interval time.Duration) *controller {
	c := &controller{
		src:      src,
		interval: interval,
		hist:     make(map[string]*series, len(histKeys)),
		dispatch: fyne.Do,
	}
	for _, k := range histKeys {
		c.hist[k] = newSeries(historyLen)
	}
	return c
}

// start launches the polling loop in a background goroutine. It returns
// immediately; the loop polls once right away and then on every interval tick.
func (c *controller) start() {
	c.refresh = make(chan struct{}, 1)
	c.stop = make(chan struct{})
	c.done = make(chan struct{})
	go c.loop()
}

func (c *controller) loop() {
	defer close(c.done)
	c.poll() // immediate first read
	ticker := time.NewTicker(c.currentInterval())
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-c.refresh:
			c.poll()
		case <-ticker.C:
			if !c.paused.Load() {
				c.poll()
			}
		}
	}
}

func (c *controller) currentInterval() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.interval
}

// poll performs one blocking Snapshot, stores it, appends history, and fires the
// UI callback. On error it preserves the last good reading and reports the error.
func (c *controller) poll() {
	c.mu.Lock()
	src := c.src
	c.mu.Unlock()

	r, err := src.Snapshot()
	if err == nil && r != nil {
		c.latest.Store(r)
		c.pushHistory(r)
	}
	if c.onUpdate != nil && c.dispatch != nil {
		cb := c.onUpdate
		c.dispatch(func() { cb(r, err) })
	}
}

func (c *controller) pushHistory(r *deye.Reading) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hist["pv"].push(r.PVTotal())
	c.hist["load"].push(math.Abs(r.Values["load_power"]))
	c.hist["grid"].push(math.Abs(r.Values["grid_power"]))
	c.hist["bat"].push(math.Abs(r.Values["bat_power"]))
}

// history returns a defensive copy of the named series' samples.
func (c *controller) history(key string) []float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.hist[key]
	if s == nil {
		return nil
	}
	return s.snapshot()
}

// setPaused enables or disables polling on interval ticks. An in-flight poll is
// allowed to finish; ticks are skipped while paused.
func (c *controller) setPaused(p bool) { c.paused.Store(p) }

// isPaused reports whether interval polling is currently suspended.
func (c *controller) isPaused() bool { return c.paused.Load() }

// pollNow requests an immediate out-of-cycle poll (e.g. a manual refresh button).
func (c *controller) pollNow() {
	if c.refresh != nil {
		select {
		case c.refresh <- struct{}{}:
		default:
		}
	}
}

// stopLoop signals the loop to exit and waits for it to finish. Safe to call
// when no loop is running.
func (c *controller) stopLoop() {
	if c.stop == nil {
		return
	}
	close(c.stop)
	<-c.done
	c.stop = nil
}

// close stops polling and closes the underlying source.
func (c *controller) close() error {
	c.stopLoop()
	c.mu.Lock()
	src := c.src
	c.mu.Unlock()
	if src != nil {
		return src.Close()
	}
	return nil
}
