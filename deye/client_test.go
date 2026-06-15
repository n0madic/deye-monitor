package deye

import "testing"

// TestOnUnsolicited verifies the native solarman hook classifies heartbeat
// frames (control code 0x4710) and ignores other unsolicited frames.
func TestOnUnsolicited(t *testing.T) {
	c := New(Config{IP: "1.2.3.4", Serial: 1})
	c.onUnsolicited(heartbeatControlCode, nil) // heartbeat
	c.onUnsolicited(heartbeatControlCode, nil) // heartbeat
	c.onUnsolicited(0x4210, nil)               // data report — not a heartbeat

	if c.Heartbeats() != 2 {
		t.Fatalf("Heartbeats = %d, want 2", c.Heartbeats())
	}
	if !c.heartbeatThisCycle {
		t.Fatalf("heartbeatThisCycle should be set after a heartbeat")
	}
	if c.lastHeartbeat.IsZero() {
		t.Fatalf("lastHeartbeat should be timestamped after a heartbeat")
	}
}

// TestNewDefaults verifies the zero-value Config falls back to documented
// defaults.
func TestNewDefaults(t *testing.T) {
	c := New(Config{IP: "10.0.0.1", Serial: 42})
	if c.addr != "10.0.0.1:8899" {
		t.Fatalf("addr = %q, want 10.0.0.1:8899", c.addr)
	}
	if c.timeout != defaultTimeout {
		t.Fatalf("timeout = %v, want %v", c.timeout, defaultTimeout)
	}
	if c.attempts != defaultAttempts {
		t.Fatalf("attempts = %d, want %d", c.attempts, defaultAttempts)
	}
	if got := c.Heartbeats(); got != 0 {
		t.Fatalf("fresh client Heartbeats = %d, want 0", got)
	}
}

// TestNewOverrides verifies explicit Config fields win over the defaults.
func TestNewOverrides(t *testing.T) {
	c := New(Config{IP: "1.2.3.4", Serial: 7, Port: 502, Attempts: 2})
	if c.addr != "1.2.3.4:502" {
		t.Fatalf("addr = %q, want 1.2.3.4:502", c.addr)
	}
	if c.attempts != 2 {
		t.Fatalf("attempts = %d, want 2", c.attempts)
	}
}
