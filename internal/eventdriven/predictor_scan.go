package eventdriven

import (
	"context"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// Scan theme consumption knobs. Values chosen so a single
// critical-confidence scan cannot outweigh the calendar-event signal,
// but a cluster of high-severity scans within the lookback window can
// tilt the day in a meaningful direction.
const (
	scanThemeLoadLimit       = 50
	scanThemeMinConfidence   = 0.5
	scanThemeLookback        = 24 * time.Hour
	scanThemeTiltDampener    = 0.5
	scanThemeMaxDriversKept  = 5
)

// Bullish / bearish theme keyword tables. Lower-case substring match.
// Themes that match neither table are treated as neutral observations
// (they contribute to confidence via the W3b narrative path, not
// direction here — this keeps W3b and W4 paths orthogonal).
var (
	bullishScanThemeKeywords = []string{
		"rally", "growth", "bull", "breakout", "expansion",
		"上升", "成長", "突破", "復甦", "強勢",
	}
	bearishScanThemeKeywords = []string{
		"decline", "fall", "bear", "breakdown", "contraction", "flight",
		"下跌", "衰退", "崩盤", "走弱", "外逃",
	}
)

// scanSeverityWeight maps a severity string to a 0..1 weight. Unknown
// severities return 0 (the scan is ignored). Critical/high/medium/low
// mirror the constants in internal/narrative/detector.go.
func scanSeverityWeight(severity string) float64 {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 1.0
	case "high":
		return 0.7
	case "medium":
		return 0.4
	case "low":
		return 0.1
	default:
		return 0
	}
}

// scanThemeDirection returns +1 for bullish themes, -1 for bearish, 0
// for neutral / unrecognised themes. The first matching keyword wins
// (bullish table is checked first).
func scanThemeDirection(theme string) float64 {
	tl := strings.ToLower(theme)
	for _, kw := range bullishScanThemeKeywords {
		if strings.Contains(tl, kw) {
			return 1.0
		}
	}
	for _, kw := range bearishScanThemeKeywords {
		if strings.Contains(tl, kw) {
			return -1.0
		}
	}
	return 0
}

// applyScanThemes reads recent detector scans and returns a directional
// tilt in [-scanThemeTiltDampener, +scanThemeTiltDampener] plus the
// contributing theme names (capped at scanThemeMaxDriversKept).
//
// Returns (0, nil) when:
//   - p.scanStore is nil (default — scan store never wired)
//   - the store returns an error (logged at WARN, never propagated —
//     predictor must stay resilient to scan pipeline hiccups)
//   - no scan passes the recency / confidence / severity gates
//
// Filters applied (in order, all AND):
//   - DetectedAt must be within [day-scanThemeLookback, day+1d]
//   - Confidence must be >= scanThemeMinConfidence
//   - Severity must be one of critical/high/medium/low
//   - Theme must match a bullish or bearish keyword
//
// This is the W4-consumption layer. It is orthogonal to the W3b
// narrative-match path: W3b validates that scans are consistent with
// the narrative model's ActiveThemes; W4 here reads scans as raw
// observations and projects them onto the bullish/bearish axis.
func (p *Predictor) applyScanThemes(ctx context.Context, day time.Time) (float64, []string) {
	if p.scanStore == nil {
		return 0, nil
	}
	scans, err := p.scanStore.LoadRecentScans(ctx, scanThemeLoadLimit)
	if err != nil {
		logging.Warn("eventdriven", "scan_theme_load_failed",
			logging.FStr("day", day.Format("2006-01-02")),
			logging.Err(err))
		return 0, nil
	}

	cutoff := day.Add(-scanThemeLookback)
	upperBound := day.Add(24 * time.Hour)

	var tilt float64
	var drivers []string

	for _, s := range scans {
		if s.DetectedAt.Before(cutoff) || s.DetectedAt.After(upperBound) {
			continue
		}
		if s.Confidence < scanThemeMinConfidence {
			continue
		}
		weight := scanSeverityWeight(s.Severity)
		if weight == 0 {
			continue
		}
		sign := scanThemeDirection(s.Theme)
		if sign == 0 {
			continue
		}
		tilt += sign * weight * s.Confidence
		if len(drivers) < scanThemeMaxDriversKept {
			drivers = append(drivers, s.Theme)
		}
	}

	// Bound tilt to [-1, 1] before dampening so a stack of
	// high-severity scans can't accumulate unboundedly.
	if tilt > 1.0 {
		tilt = 1.0
	} else if tilt < -1.0 {
		tilt = -1.0
	}
	tilt *= scanThemeTiltDampener

	return tilt, drivers
}
