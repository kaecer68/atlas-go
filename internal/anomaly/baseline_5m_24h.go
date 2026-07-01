package anomaly

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

// Baseline5m24hDetector compares the recent 5-minute call rate against a
// 24-hour baseline. It is a T1.2 stub: the real rolling-window z-score math
// will land in T1.3 behind the same Detector interface.
type Baseline5m24hDetector struct {
	cfg    Config
	zscore ZScoreFunc
	now    func() time.Time
}

// NewBaseline5m24hDetector creates a detector with the given configuration.
func NewBaseline5m24hDetector(cfg Config) *Baseline5m24hDetector {
	return &Baseline5m24hDetector{
		cfg:    cfg,
		zscore: DefaultZScoreFunc(),
		now:    time.Now,
	}
}

// Name returns the detector identifier.
func (d *Baseline5m24hDetector) Name() string {
	return "baseline_5m_24h"
}

// Detect computes a single anomaly when the short-window call rate deviates
// from the long-window baseline.
func (d *Baseline5m24hDetector) Detect(ctx context.Context, entries []AuditEntryV2) ([]Anomaly, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("baseline_5m_24h detect: %w", err)
	}

	now := d.now()
	currentCutoff := now.Add(-time.Duration(d.cfg.CurrentWindowMin) * time.Minute)
	baselineCutoff := now.Add(-time.Duration(d.cfg.BaselineWindowMin) * time.Minute)

	currentCount := 0
	numBuckets := d.cfg.BaselineWindowMin / d.cfg.CurrentWindowMin
	if numBuckets <= 0 {
		numBuckets = 1
	}
	baselineCounts := make([]int, numBuckets)

	for _, e := range entries {
		ts := parseTS(e.TS)
		if ts.IsZero() {
			continue
		}
		if ts.After(now) {
			continue
		}
		if ts.After(currentCutoff) {
			currentCount++
			continue
		}
		if ts.Before(baselineCutoff) {
			continue
		}

		idx := int(ts.Sub(baselineCutoff) / (time.Duration(d.cfg.CurrentWindowMin) * time.Minute))
		if idx < 0 {
			idx = 0
		}
		if idx >= numBuckets {
			idx = numBuckets - 1
		}
		baselineCounts[idx]++
	}

	if len(baselineCounts) < d.cfg.MinBaselineSamples {
		return nil, nil
	}

	baseline := Baseline{
		WindowMin: d.cfg.BaselineWindowMin,
		Median:    medianInt(baselineCounts),
		StdDev:    stdDevInt(baselineCounts),
		SampleN:   len(baselineCounts),
	}
	current := Current{
		WindowMin: d.cfg.CurrentWindowMin,
		Median:    float64(currentCount),
		SampleN:   1,
	}

	// Guard against a degenerate baseline where every bucket has the same count.
	if baseline.StdDev == 0 {
		baseline.StdDev = 1
	}

	score := d.zscore(baseline, current)
	if score <= d.cfg.ZScoreThreshold {
		return nil, nil
	}

	return []Anomaly{
		{
			Type:       d.Name(),
			Score:      score,
			DetectedAt: now,
			Baseline:   baseline,
			Current:    current,
		},
	}, nil
}

func medianInt(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]int, len(vals))
	copy(sorted, vals)
	sort.Ints(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return float64(sorted[n/2])
	}
	return float64(sorted[n/2-1]+sorted[n/2]) / 2.0
}

func stdDevInt(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += float64(v)
	}
	mean := sum / float64(len(vals))
	var variance float64
	for _, v := range vals {
		diff := float64(v) - mean
		variance += diff * diff
	}
	variance /= float64(len(vals))
	return math.Sqrt(variance)
}
