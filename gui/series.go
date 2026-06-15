package gui

// series is a fixed-capacity ring buffer of recent float samples, used to back
// the history charts. It mirrors the sparkline series from the TUI.
type series struct {
	data []float64
	cap  int
}

func newSeries(capacity int) *series {
	if capacity < 1 {
		capacity = 1
	}
	return &series{cap: capacity}
}

// push appends v, dropping the oldest sample once cap is exceeded.
func (s *series) push(v float64) {
	s.data = append(s.data, v)
	if len(s.data) > s.cap {
		s.data = s.data[len(s.data)-s.cap:]
	}
}

// snapshot returns a defensive copy of the current samples, oldest first.
func (s *series) snapshot() []float64 {
	out := make([]float64, len(s.data))
	copy(out, s.data)
	return out
}
