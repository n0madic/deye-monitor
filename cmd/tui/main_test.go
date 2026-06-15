package main

import (
	"strings"
	"testing"
	"time"

	"deye-monitor/deye"
)

func TestHeartbeatTag(t *testing.T) {
	if got := heartbeatTag(&deye.Reading{Heartbeats: 0}); got != "" {
		t.Fatalf("no heartbeats should render empty tag, got %q", got)
	}

	fresh := heartbeatTag(&deye.Reading{Heartbeats: 3, HeartbeatNow: true, LastHeartbeat: time.Now()})
	if !strings.Contains(fresh, "♥") {
		t.Fatalf("fresh heartbeat should show filled ♥, got %q", fresh)
	}
	if !strings.Contains(fresh, "×3") {
		t.Fatalf("tag should include the count ×3, got %q", fresh)
	}

	idle := heartbeatTag(&deye.Reading{Heartbeats: 3, HeartbeatNow: false, LastHeartbeat: time.Now()})
	if !strings.Contains(idle, "♡") {
		t.Fatalf("idle state should show hollow ♡, got %q", idle)
	}
}

// TestSeriesPush verifies the ring buffer keeps only the last cap samples in
// chronological order.
func TestSeriesPush(t *testing.T) {
	s := newSeries(3)
	for _, v := range []float64{1, 2, 3, 4, 5} {
		s.push(v)
	}
	if len(s.data) != 3 {
		t.Fatalf("len = %d, want 3 (cap)", len(s.data))
	}
	want := []float64{3, 4, 5}
	for i, v := range want {
		if s.data[i] != v {
			t.Fatalf("data[%d] = %v, want %v (got %v)", i, s.data[i], v, s.data)
		}
	}
}

func TestSeriesUnderCap(t *testing.T) {
	s := newSeries(5)
	s.push(10)
	s.push(20)
	if len(s.data) != 2 {
		t.Fatalf("len = %d, want 2", len(s.data))
	}
	if s.data[0] != 10 || s.data[1] != 20 {
		t.Fatalf("data = %v, want [10 20]", s.data)
	}
}

func TestAdjustInterval(t *testing.T) {
	cases := []struct {
		name string
		cur  time.Duration
		dir  int
		want time.Duration
	}{
		{"lengthen 5s->10s", 5 * time.Second, +1, 10 * time.Second},
		{"shorten 5s->3s", 5 * time.Second, -1, 3 * time.Second},
		{"clamp at min", time.Second, -1, time.Second},
		{"clamp at max", time.Minute, +1, time.Minute},
		{"snap odd value up", 4 * time.Second, +1, 5 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := adjustInterval(c.cur, c.dir); got != c.want {
				t.Fatalf("adjustInterval(%v, %d) = %v, want %v", c.cur, c.dir, got, c.want)
			}
		})
	}
}
