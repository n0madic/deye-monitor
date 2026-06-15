package deye

import (
	"fmt"
	"maps"
	"time"

	"github.com/snowirbis/solarman"
)

// Defaults applied by New for unset Config fields.
const (
	defaultPort     = 8899
	defaultTimeout  = 8 * time.Second
	defaultAttempts = 2
)

// heartbeatControlCode is the Solarman V5 control code the logger uses for its
// unsolicited keepalive frames. The library delivers such frames to our
// OnUnsolicited hook rather than returning them as read errors.
const heartbeatControlCode uint16 = 0x4710

// identityBlock is the device-info register block that carries the serial.
var identityBlock = [][2]int{{0, 25}}

// Config configures a Client. Zero-valued fields fall back to sensible defaults.
type Config struct {
	IP       string
	Serial   uint32
	Port     int           // default 8899
	Timeout  time.Duration // default 8s
	Attempts int           // retry attempts per read, default 2
}

// Client wraps the solarman logger with reconnect-on-error and tracks the
// logger's unsolicited heartbeat frames. A Client is not safe for concurrent
// use; serialize calls if sharing one across goroutines.
type Client struct {
	addr     string
	serial   uint32
	timeout  time.Duration
	attempts int

	inv                *solarman.InverterLogger
	heartbeats         int         // total heartbeat frames seen this session
	lastHeartbeat      time.Time   // when the most recent heartbeat arrived
	heartbeatThisCycle bool        // a heartbeat arrived during the current Snapshot
	device             *DeviceInfo // cached device identity, looked up once
}

// New creates a Client from cfg, applying defaults for unset fields.
func New(cfg Config) *Client {
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.Attempts == 0 {
		cfg.Attempts = defaultAttempts
	}
	return &Client{
		addr:     fmt.Sprintf("%s:%d", cfg.IP, cfg.Port),
		serial:   cfg.Serial,
		timeout:  cfg.Timeout,
		attempts: cfg.Attempts,
	}
}

func (c *Client) connect() *solarman.InverterLogger {
	if c.inv == nil {
		// the library's timeout granularity is whole seconds; floor at 1
		secs := max(int(c.timeout.Seconds()), 1)
		c.inv = solarman.Init(c.addr, c.serial, secs)
		c.inv.SetMeta(0xA5, 0x15, 0x4510, 0x1510) // standard Solarman V5 framing
		c.inv.OnUnsolicited = c.onUnsolicited
	}
	return c.inv
}

// onUnsolicited is the hook the solarman library calls for every frame whose
// control code is not the expected response (heartbeats, data reports). It runs
// synchronously inside Read, in the same goroutine, so it needs no locking.
func (c *Client) onUnsolicited(controlCode uint16, _ []byte) {
	if controlCode == heartbeatControlCode {
		c.heartbeats++
		c.lastHeartbeat = time.Now()
		c.heartbeatThisCycle = true
	}
}

// Close closes the underlying connection. The Client stays usable afterwards:
// the next read reconnects.
func (c *Client) Close() error {
	if c.inv == nil {
		return nil
	}
	err := c.inv.Close()
	c.inv = nil
	return err
}

func (c *Client) readBlocks(blocks [][2]int) (map[int]uint16, error) {
	inv := c.connect()
	out := make(map[int]uint16)
	for _, b := range blocks {
		data, err := inv.Read(b[0], b[1])
		if err != nil {
			_ = c.Close() // force reconnect next time
			return nil, fmt.Errorf("read %d+%d: %w", b[0], b[1], err)
		}
		maps.Copy(out, data)
	}
	return out, nil
}

// readRetry reads blocks, retrying with a fresh reconnect on an I/O error. The
// library now matches responses to requests and skips heartbeat/stale frames
// internally, so any error returned here is a genuine read failure; a reconnect
// on the next attempt may still recover from a transient network hiccup.
func (c *Client) readRetry(blocks [][2]int, attempts int) (map[int]uint16, error) {
	var err error
	for range attempts {
		var regs map[int]uint16
		if regs, err = c.readBlocks(blocks); err == nil {
			return regs, nil
		}
		// readBlocks already closed the connection; the next attempt reconnects.
		time.Sleep(150 * time.Millisecond)
	}
	return nil, err
}

// Device reads the static device identity (serial, rated power, phases, derived
// model) once and caches it. On a read failure it returns the error and caches
// nothing, so a later call retries.
func (c *Client) Device() (DeviceInfo, error) {
	if c.device != nil {
		return *c.device, nil
	}
	regs, err := c.readRetry(identityBlock, 3)
	if err != nil {
		return DeviceInfo{}, err
	}
	d := decodeDeviceInfo(regs)
	c.device = &d
	return d, nil
}

// Identity returns the inverter serial, reading and caching the device info on
// first use. It is a convenience wrapper around Device.
func (c *Client) Identity() (string, error) {
	d, err := c.Device()
	return d.Serial, err
}

// Snapshot reads the live register blocks and decodes them into a Reading. The
// device identity (serial, model) is looked up once and cached; a failed lookup
// is non-fatal — the Reading is returned with an empty Serial/Model.
func (c *Client) Snapshot() (*Reading, error) {
	c.heartbeatThisCycle = false
	dev, _ := c.Device() // best-effort: a Reading is still useful without it
	regs, err := c.readRetry(liveBlocks, c.attempts)
	if err != nil {
		return nil, err
	}
	r := &Reading{
		Time:          time.Now(),
		Serial:        dev.Serial,
		Model:         dev.Model,
		Values:        make(map[string]float64),
		States:        make(map[string]string),
		Heartbeats:    c.heartbeats,
		HeartbeatNow:  c.heartbeatThisCycle,
		LastHeartbeat: c.lastHeartbeat,
	}
	for _, m := range Metrics {
		v, ok := decode(m, regs)
		if !ok {
			continue
		}
		switch m.Key {
		case "device_state":
			r.States["device_state"] = lookup(DeviceState, int(v))
		case "work_mode":
			r.States["work_mode"] = lookup(WorkMode, int(v))
		default:
			r.Values[m.Key] = v
		}
	}
	return r, nil
}

// Heartbeats returns the total number of logger heartbeat frames seen so far.
func (c *Client) Heartbeats() int {
	return c.heartbeats
}
