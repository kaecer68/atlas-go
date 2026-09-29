package industry

// silicon_index_ma.go — I22-overheat: make SiliconIndicators.TaiwanSemiconductorIndexMA
// carry the statistic its name and IndexMAPercentThreshold claim, i.e. "how far
// the Taiwan semiconductor index sits ABOVE its moving average".
//
// The gap (registered as I22-overheat, docs/reference/inert-registry.md): the only
// production writer of MacroDataSnapshot.TaiwanSemiIndex stores the index's
// single-day return (apigateway TWSESectorIndexChannelAdapter → latest.ReturnPct),
// so the 1→2 trigger compared a one-day move against a "deviation above MA"
// threshold and PhaseOverheat was unreachable by construction.
//
// Why the moving average is computed here and not in the producer:
//   - the TWSE MI_INDEX openapi is latest-only; the provider hard-blocks
//     historical dates older than five days with ErrLatestOnly, so no producer
//     can obtain a series in one call;
//   - the FinMind sector-index provider walks one request per day, so a
//     60-session window per 10-minute refresh would burn its quota;
//   - a gateway adapter reading another component's state files would be a
//     layering violation.
//
// So the series comes from the daily macro snapshot archive the platform already
// maintains (data/state/macro/YYYY-MM-DD.json, written by
// internal/globalmarket.FetchDailyMacro; its _metadata.json already names
// internal/industry as a consumer). Each dated file carries
// taiwan_semi_index.value — the index LEVEL — which is exactly what a moving
// average needs.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// SiliconIndexMAWindowSessions is the moving-average window, in TRADING
// sessions. 60 sessions is the 季線 (quarterly line) of Taiwanese charting
// convention, which is what the field name TaiwanSemiconductorIndexMA (偏離季線)
// and IndexMAPercentThreshold refer to.
//
// The window counts trading sessions, not files or calendar days: the macro
// archive writes one file per UTC day, so weekends and holidays appear as
// carry-forward copies of the previous close (32 of the 113 files present in
// production on 2026-09-30). Averaging those would turn a 60-session quarterly
// line into a ~42-session line spread over 60 calendar days.
const SiliconIndexMAWindowSessions = 60

// SiliconIndexMADeviation is the resolved "how far above the moving average"
// reading, with the absence of a reading stated explicitly.
//
// Available=false is never rendered as 0.0: a zero deviation and "no series to
// average" are different facts, and conflating them would let a missing archive
// look like a flat market (the discipline established for the universe
// exclusion accounting, issue #2019).
type SiliconIndexMADeviation struct {
	// Value is last/MA - 1 (fraction, +0.05 = 5% above the moving average).
	// Meaningful only when Available.
	Value float64
	// Available reports whether a full-window deviation was computed.
	Available bool
	// Sessions is the number of trading sessions that entered the window.
	Sessions int
	// Source identifies the directory the series came from (provenance).
	Source string
	// Reason explains an unavailable reading (machine-readable token).
	Reason string
}

// Reasons a reading can be unavailable.
const (
	// SiliconIndexMAReasonNoSeries: no macro archive directory was found.
	SiliconIndexMAReasonNoSeries = "no_series_directory"
	// SiliconIndexMAReasonInsufficient: the archive is too shallow for the window.
	SiliconIndexMAReasonInsufficient = "insufficient_sessions"
	// SiliconIndexMAReasonNoAsOf: the caller supplied no observation time, so an
	// as-of window cannot be built (keeps the reader deterministic rather than
	// silently falling back to wall-clock time).
	SiliconIndexMAReasonNoAsOf = "no_observation_time"
)

// ComputeSiliconIndexMADeviation averages the last `window` levels and reports
// how far the latest level sits above that average. Pure: same input, same
// output, no clock and no I/O.
func ComputeSiliconIndexMADeviation(levels []float64, window int) SiliconIndexMADeviation {
	if window <= 0 {
		return SiliconIndexMADeviation{Reason: SiliconIndexMAReasonInsufficient}
	}
	if len(levels) < window {
		return SiliconIndexMADeviation{
			Sessions: len(levels),
			Reason:   SiliconIndexMAReasonInsufficient,
		}
	}
	tail := levels[len(levels)-window:]
	sum := 0.0
	for _, v := range tail {
		sum += v
	}
	mean := sum / float64(window)
	if mean == 0 {
		// A zero mean makes the ratio undefined; report unavailable rather than
		// emitting +Inf or a fabricated percentage.
		return SiliconIndexMADeviation{Sessions: window, Reason: SiliconIndexMAReasonInsufficient}
	}
	return SiliconIndexMADeviation{
		Value:     levels[len(levels)-1]/mean - 1,
		Available: true,
		Sessions:  window,
	}
}

// siliconIndexMASeriesDirOverride, when set, replaces directory discovery. Tests
// point it at a fixture directory; it is otherwise empty in production.
var siliconIndexMASeriesDirOverride string

var (
	siliconIndexMACacheMu sync.Mutex
	siliconIndexMACache   = map[string]SiliconIndexMADeviation{}
	siliconIndexMAWarned  = map[string]struct{}{}
)

// resetSiliconIndexMACache clears the memoised readings and the warn-once
// bookkeeping. Tests call it to keep cases independent.
func resetSiliconIndexMACache() {
	siliconIndexMACacheMu.Lock()
	defer siliconIndexMACacheMu.Unlock()
	siliconIndexMACache = map[string]SiliconIndexMADeviation{}
	siliconIndexMAWarned = map[string]struct{}{}
}

// LoadSiliconIndexMADeviation resolves the index-level series and returns the
// deviation as of `asOf`. The reading is memoised per (directory, day): the
// dashboard extracts indicators on every request, so an uncached reader would
// rescan the whole archive each time.
func LoadSiliconIndexMADeviation(asOf time.Time) SiliconIndexMADeviation {
	if asOf.IsZero() {
		return SiliconIndexMADeviation{Reason: SiliconIndexMAReasonNoAsOf}
	}
	dir := siliconIndexMASeriesDir()
	if dir == "" {
		return SiliconIndexMADeviation{Reason: SiliconIndexMAReasonNoSeries}
	}
	key := dir + "|" + asOf.In(taipeiLocation()).Format("2006-01-02")

	siliconIndexMACacheMu.Lock()
	if cached, ok := siliconIndexMACache[key]; ok {
		siliconIndexMACacheMu.Unlock()
		return cached
	}
	siliconIndexMACacheMu.Unlock()

	dev := loadSiliconIndexMADeviationUncached(dir, asOf)

	siliconIndexMACacheMu.Lock()
	siliconIndexMACache[key] = dev
	siliconIndexMACacheMu.Unlock()
	return dev
}

// loadSiliconIndexMADeviationUncached does the file work for one directory.
func loadSiliconIndexMADeviationUncached(dir string, asOf time.Time) SiliconIndexMADeviation {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return SiliconIndexMADeviation{Source: dir, Reason: SiliconIndexMAReasonNoSeries}
	}
	taipei := taipeiLocation()
	cutoff := asOf.In(taipei)
	cutoffDay := time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, taipei)

	type session struct {
		day   time.Time
		level float64
	}
	var sessions []session
	for _, path := range matches {
		base := filepath.Base(path)
		if base == "latest.json" || base == "previous.json" || base == "_metadata.json" {
			continue
		}
		day, err := time.ParseInLocation("2006-01-02", trimJSONExt(base), taipei)
		if err != nil {
			continue
		}
		if day.After(cutoffDay) {
			continue // future-dated archive file: never average it
		}
		if !taiwanholidays.IsTradingDay(day) {
			continue // weekend/holiday carry-forward copy, not a session
		}
		level, ok := readMacroIndexLevel(path)
		if !ok {
			continue // this day has no TAISEMI level recorded
		}
		sessions = append(sessions, session{day: day, level: level})
	}
	if len(sessions) < SiliconIndexMAWindowSessions {
		return SiliconIndexMADeviation{
			Sessions: len(sessions),
			Source:   dir,
			Reason:   SiliconIndexMAReasonInsufficient,
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].day.Before(sessions[j].day) })
	levels := make([]float64, 0, len(sessions))
	for _, s := range sessions {
		levels = append(levels, s.level)
	}
	dev := ComputeSiliconIndexMADeviation(levels, SiliconIndexMAWindowSessions)
	dev.Source = dir
	return dev
}

// readMacroIndexLevel extracts taiwan_semi_index.value from one dated macro
// snapshot file. Returns false when the file has no such reading, so a day
// without a TAISEMI level is skipped instead of counted as zero.
func readMacroIndexLevel(path string) (float64, bool) {
	// #nosec G304 -- path comes from a glob inside the configured state
	// directory, never from user input.
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var snapshot struct {
		TaiwanSemiIndex marketdata.MacroDataPoint `json:"taiwan_semi_index"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return 0, false
	}
	if snapshot.TaiwanSemiIndex.Symbol == "" || snapshot.TaiwanSemiIndex.Value == 0 {
		return 0, false
	}
	return snapshot.TaiwanSemiIndex.Value, true
}

// siliconIndexMASeriesDir returns the macro archive directory, or "" when no
// candidate exists. Order: explicit override (tests/ops), the container's data
// root (ATLAS_DATA_DIR=/app/data), the configured work directory, then the
// repository-relative default.
func siliconIndexMASeriesDir() string {
	if siliconIndexMASeriesDirOverride != "" {
		return siliconIndexMASeriesDirOverride
	}
	var candidates []string
	if dataDir := os.Getenv("ATLAS_DATA_DIR"); dataDir != "" {
		candidates = append(candidates, filepath.Join(dataDir, "state", "macro"))
	}
	workDir := config.Load().WorkDir
	if workDir == "" {
		workDir = "."
	}
	candidates = append(candidates, filepath.Join(workDir, "data", "state", "macro"))
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	// Report the work-directory candidate as the source even when it is absent,
	// so an unavailable reading still names where the series was expected.
	return candidates[len(candidates)-1]
}

// siliconIndexMAForIndicator applies the kill switch. Pure, so both branches are
// reachable from one build: tests exercise enabled=false without recompiling.
//
// enabled is SiliconTWIndexIsMADeviation. When it is false — or when no series
// was available — the legacy single-day return is returned unchanged, which is
// bit-for-bit the pre-fix behavior.
func siliconIndexMAForIndicator(legacy float64, dev SiliconIndexMADeviation, enabled bool) float64 {
	if !enabled || !dev.Available {
		return legacy
	}
	return dev.Value
}

// SiliconIndexMA reads the value for SiliconIndicators.TaiwanSemiconductorIndexMA,
// including the fallback decision. It is the single call site of the kill switch
// in production code.
func SiliconIndexMA(point marketdata.MacroDataPoint) float64 {
	legacy := point.ChangePct / 100.0
	if !SiliconTWIndexIsMADeviation {
		return legacy
	}
	dev := LoadSiliconIndexMADeviation(indexPointAsOf(point))
	if !dev.Available {
		warnSiliconIndexMAUnavailable(dev)
	}
	return siliconIndexMAForIndicator(legacy, dev, true)
}

// indexPointAsOf returns the observation time of the index reading. Using the
// snapshot's own timestamp (instead of the wall clock) keeps the window
// deterministic for a given snapshot, which is what makes the behavior testable.
func indexPointAsOf(point marketdata.MacroDataPoint) time.Time {
	if point.Timestamp == 0 {
		return time.Time{}
	}
	return time.Unix(point.Timestamp, 0)
}

// warnSiliconIndexMAUnavailable reports an unavailable reading once per
// (reason, directory) so an ice-cold archive cannot emit one line per request.
func warnSiliconIndexMAUnavailable(dev SiliconIndexMADeviation) {
	key := dev.Reason + "|" + dev.Source
	siliconIndexMACacheMu.Lock()
	if _, warned := siliconIndexMAWarned[key]; warned {
		siliconIndexMACacheMu.Unlock()
		return
	}
	siliconIndexMAWarned[key] = struct{}{}
	siliconIndexMACacheMu.Unlock()

	logging.Warn("industry", "silicon_index_ma_unavailable",
		logging.FStr("reason", dev.Reason),
		logging.FStr("source", dev.Source),
		logging.FInt("sessions", dev.Sessions),
		logging.FStr("fallback", "single-day return (pre-I22 behavior)"),
	)
}

// taipeiLocation returns the exchange timezone, falling back to a fixed offset
// when the tzdata is unavailable (Taiwan has no daylight saving, so the fixed
// offset is exact).
func taipeiLocation() *time.Location {
	if loc, err := time.LoadLocation("Asia/Taipei"); err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*60*60)
}

func trimJSONExt(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}
