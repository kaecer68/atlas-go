package janus

import (
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/prism"
)

// CohortPerformanceTracker maintains a rolling history of cohort snapshots
// and computes windowed aggregates for JANUS weighting.
type CohortPerformanceTracker struct {
	history     map[prism.RegimeType][]CohortSnapshot
	maxHistory  int
	mu          sync.RWMutex
}

// NewCohortPerformanceTracker creates a tracker with a bounded history per cohort.
// maxHistory should comfortably cover the longest window (e.g., 90 days for 60-day window).
func NewCohortPerformanceTracker(maxHistory int) *CohortPerformanceTracker {
	if maxHistory <= 0 {
		maxHistory = 90
	}
	return &CohortPerformanceTracker{
		history:    make(map[prism.RegimeType][]CohortSnapshot),
		maxHistory: maxHistory,
	}
}

// RecordSnapshot appends a new performance snapshot for the given cohort.
func (t *CohortPerformanceTracker) RecordSnapshot(snapshot CohortSnapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()

	list := t.history[snapshot.Regime]
	list = append(list, snapshot)
	if len(list) > t.maxHistory {
		list = list[len(list)-t.maxHistory:]
	}
	t.history[snapshot.Regime] = list
}

// GetPerformance returns rolling-window aggregates for every tracked cohort.
func (t *CohortPerformanceTracker) GetPerformance() map[prism.RegimeType]*CohortPerformance {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[prism.RegimeType]*CohortPerformance)
	for regime, snaps := range t.history {
		result[regime] = &CohortPerformance{
			Regime:      regime,
			ShortWindow: t.aggregate(snaps, 5),
			MedWindow:   t.aggregate(snaps, 20),
			LongWindow:  t.aggregate(snaps, 60),
			LastUpdated: time.Now(),
		}
	}
	return result
}

// GetCohortPerformance returns performance for a single cohort.
func (t *CohortPerformanceTracker) GetCohortPerformance(regime prism.RegimeType) *CohortPerformance {
	t.mu.RLock()
	defer t.mu.RUnlock()

	snaps, ok := t.history[regime]
	if !ok || len(snaps) == 0 {
		return nil
	}

	return &CohortPerformance{
		Regime:      regime,
		ShortWindow: t.aggregate(snaps, 5),
		MedWindow:   t.aggregate(snaps, 20),
		LongWindow:  t.aggregate(snaps, 60),
		LastUpdated: time.Now(),
	}
}

// aggregate computes windowed averages over the last n snapshots.
// n maps to trading days; we treat each snapshot as one trading-day observation.
func (t *CohortPerformanceTracker) aggregate(snaps []CohortSnapshot, n int) *WindowPerformance {
	if len(snaps) == 0 {
		return nil
	}
	if n > len(snaps) {
		n = len(snaps)
	}

	windowSnaps := snaps[len(snaps)-n:]
	var sumSharpe, sumHitRate, sumReturn float64
	for _, s := range windowSnaps {
		sumSharpe += s.SharpeRatio
		sumHitRate += s.HitRate
		sumReturn += s.TotalReturn
	}

	count := float64(len(windowSnaps))
	return &WindowPerformance{
		Window:       windowFromDays(n),
		SharpeRatio:  sumSharpe / count,
		HitRate:      sumHitRate / count,
		TotalReturn:  sumReturn / count,
		Observations: len(windowSnaps),
	}
}

func windowFromDays(n int) PerformanceWindow {
	switch {
	case n <= 7:
		return WindowShort
	case n <= 30:
		return WindowMedium
	default:
		return WindowLong
	}
}
