package anomaly

import (
	"context"
	"fmt"
	"time"
)

// PerToolErrorDetector compares the recent error count per tool against a
// 24-hour baseline. T1.2 uses stub z-score math; T1.3 will replace the
// statistics behind the same Detector interface.
type PerToolErrorDetector struct {
	cfg    Config
	zscore ZScoreFunc
	now    func() time.Time
}

// NewPerToolErrorDetector creates a detector with the given configuration.
func NewPerToolErrorDetector(cfg Config) *PerToolErrorDetector {
	return &PerToolErrorDetector{
		cfg:    cfg,
		zscore: DefaultZScoreFunc(),
		now:    time.Now,
	}
}

// Name returns the detector identifier.
func (d *PerToolErrorDetector) Name() string {
	return "per_tool_error"
}

// Detect computes anomalies for each tool whose recent error count deviates
// from its long-window baseline.
func (d *PerToolErrorDetector) Detect(ctx context.Context, entries []AuditEntryV2) ([]Anomaly, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("per_tool_error detect: %w", err)
	}

	now := d.now()
	currentCutoff := now.Add(-time.Duration(d.cfg.CurrentWindowMin) * time.Minute)
	baselineCutoff := now.Add(-time.Duration(d.cfg.BaselineWindowMin) * time.Minute)
	numBuckets := d.cfg.BaselineWindowMin / d.cfg.CurrentWindowMin
	if numBuckets <= 0 {
		numBuckets = 1
	}

	type toolStats struct {
		current int
		base    []int
	}
	stats := make(map[string]*toolStats)

	for _, e := range entries {
		ts := parseTS(e.TS)
		if ts.IsZero() {
			continue
		}
		if ts.After(now) {
			continue
		}
		if e.Status != "error" {
			continue
		}

		s, ok := stats[e.Tool]
		if !ok {
			s = &toolStats{base: make([]int, numBuckets)}
			stats[e.Tool] = s
		}

		if ts.After(currentCutoff) {
			s.current++
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
		s.base[idx]++
	}

	var anomalies []Anomaly
	for tool, s := range stats {
		if len(s.base) < d.cfg.MinBaselineSamples {
			continue
		}
		baseline := Baseline{
			WindowMin: d.cfg.BaselineWindowMin,
			Median:    medianInt(s.base),
			StdDev:    stdDevInt(s.base),
			SampleN:   len(s.base),
		}
		current := Current{
			WindowMin: d.cfg.CurrentWindowMin,
			Median:    float64(s.current),
			SampleN:   1,
		}
		if baseline.StdDev == 0 {
			baseline.StdDev = 1
		}
		score := d.zscore(baseline, current)
		if score <= d.cfg.ZScoreThreshold {
			continue
		}
		anomalies = append(anomalies, Anomaly{
			Type:       d.Name(),
			Tool:       tool,
			Score:      score,
			DetectedAt: now,
			Baseline:   baseline,
			Current:    current,
		})
	}

	return anomalies, nil
}
