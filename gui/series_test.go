package gui

import (
	"slices"
	"testing"
)

func TestSeriesPushEviction(t *testing.T) {
	t.Parallel()
	s := newSeries(3)
	for _, v := range []float64{1, 2, 3, 4, 5} {
		s.push(v)
	}
	if got, want := s.snapshot(), []float64{3, 4, 5}; !slices.Equal(got, want) {
		t.Errorf("snapshot = %v, want %v", got, want)
	}
}

func TestSeriesSnapshotIsCopy(t *testing.T) {
	t.Parallel()
	s := newSeries(3)
	s.push(1)
	s.push(2)
	snap := s.snapshot()
	snap[0] = 99
	if s.data[0] == 99 {
		t.Error("snapshot must return a defensive copy, not the backing slice")
	}
}

func TestNewSeriesClampsCapacity(t *testing.T) {
	t.Parallel()
	s := newSeries(0)
	s.push(1)
	s.push(2)
	if got := len(s.data); got != 1 {
		t.Errorf("cap clamped to 1: len = %d, want 1", got)
	}
}

func TestStats(t *testing.T) {
	t.Parallel()
	mn, mx, latest := stats([]float64{4, 1, 7, 3})
	if mn != 1 || mx != 7 || latest != 3 {
		t.Errorf("stats = (%v,%v,%v), want (1,7,3)", mn, mx, latest)
	}
	if mn, mx, latest := stats(nil); mn != 0 || mx != 0 || latest != 0 {
		t.Errorf("stats(nil) = (%v,%v,%v), want (0,0,0)", mn, mx, latest)
	}
}
