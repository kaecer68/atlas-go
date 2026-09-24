package capitalflow

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// QualityCacheTTL bounds cache reuse for QualityScore/Label. Kept
// short because the event-driven predictor calls these on every
// request and longer TTLs risk predictions reflecting pre-news
// resonance.
const QualityCacheTTL = 60 * time.Second

// defaultHistoryLimit bounds how many historical samples
// LatestDaily pulls per dimension when building the scoring
// history. Raised from 60 to 252 (one trading year) per spec §10
// H-CF-05 walk-forward calibration gate (docs/manifests/2026-07-20-cl5-capital-flow-handlehistory.md
// A01). The store enforces its own capacity (production main.go
// also passes 252); this number is only the upper bound we ask
// for and stays in sync via CF-INV-15.
const defaultHistoryLimit = 252

// LatestDailyCacheTTL controls how long LatestDaily results are cached.
// Set to 30s to stay aligned with the frontend auto-refresh interval.
const LatestDailyCacheTTL = 30 * time.Second

// taipeiZone is the fixed Asia/Taipei offset (UTC+8, no DST) used to
// derive the trading-date key on both the write path (Refresh) and the
// read path (LatestDaily / refreshIfStale). Keeping one zone constant
// here prevents the two paths from disagreeing around the UTC midnight
// boundary (CF-INV-15 / audit M4).
var taipeiZone = time.FixedZone("Asia/Taipei", 8*3600)

// deriveTradingDate converts a snapshot RecordedAt Unix timestamp to
// the Asia/Taipei YYYY-MM-DD trading date. M4 (spec §6 / CF-INV-15):
// the read path must derive its history upper bound and as-of date
// from the same Taipei clock the write path (Refresh) uses — a UTC
// derivation made Taiwan mornings before 08:00 read and write under
// different date keys, dropping one day of history and mislabeling
// the as-of date.
func deriveTradingDate(recordedAt int64) string {
	return time.Unix(recordedAt, 0).In(taipeiZone).Format("2006-01-02")
}

// Service exposes capital-flow aggregation as a callable interface
// so downstream consumers (e.g. internal/recommender,
// internal/eventdriven) can reuse the same pipeline the HTTP
// handler runs, without going through *http.Request.
//
// The pipeline (FetchSnapshot → Score(history) → ComputeResonance
// → GenerateDailyReport) is purely data-driven and HTTP-agnostic.
// Refresh is the only writer to the rolling sample store; the
// read path (LatestDaily, Summary, QualityScore, refreshIfStale)
// never calls UpsertDay (BK-15 / spec §8.1 / CF-INV-04).
//
// eventCalendar (added in CL-1 fix, spec CF-INV-16) is the shared
// *industry.EventCalendar instance created at cmd/atlas/main.go.
// It is event/sentiment data, NOT the trading-day table: Refresh's
// CF-INV-16 gate uses marketdata.IsTaiwanTradingDay (the authoritative
// static+lunar holiday table) and only consults eventCalendar to warn
// about the long_holiday-window divergence that froze the radar
// (issue #1947). Passing nil is allowed and simply skips that warning.
type Service struct {
	provider      marketdata.MacroDataProvider
	extractor     *ForceExtractor
	timeout       time.Duration
	store         RollingSampleStore
	eventCalendar *industry.EventCalendar

	mu              sync.RWMutex
	cachedResonance ResonanceResult
	cachedAt        time.Time

	reportMu       sync.RWMutex
	cachedReport   *DailyReport
	reportCachedAt time.Time

	// skipMu guards consecutiveSkips, the observable counter for CF-INV-16's
	// skip-and-log path (issue #1947: the rolling store sat frozen for a week
	// while task_liveness reported consecutive_failures=0, because a skip is
	// not a failure and nothing counted it).
	skipMu           sync.Mutex
	consecutiveSkips int

	// periodProvider resolves the seven-period market classification for a
	// trading date (PR-3a). Optional: nil keeps the legacy behavior (no
	// period → period-weighted score equals the equal-weight composite).
	// Wired in cmd/atlas from period_history (HistoricalStore).
	periodProvider func(tradingDate string) (*domain.MarketPeriod, bool)
}

// WithPeriodProvider wires a period resolver keyed by trading date
// (YYYY-MM-DD, Asia/Taipei). Used by LatestDaily/Summary to feed the
// current period into the quality-score report (PR-3a). The resolver must
// be safe for concurrent use; returning (nil, false) is always allowed and
// means "period unknown for that date" (legacy semantics).
func (s *Service) WithPeriodProvider(p func(tradingDate string) (*domain.MarketPeriod, bool)) *Service {
	s.periodProvider = p
	return s
}

// periodFor resolves the market period for a trading date via the wired
// provider. Never fails: unknown periods are represented by nil.
func (s *Service) periodFor(tradingDate string) *domain.MarketPeriod {
	if s.periodProvider == nil {
		return nil
	}
	period, ok := s.periodProvider(tradingDate)
	if !ok || period == nil {
		return nil
	}
	return period
}

// NewService constructs a Service backed by the given macrodata
// provider and an in-memory rolling sample store (capacity
// defaultHistoryLimit). Pass timeout=0 to use the default 15s
// context timeout. Callers that need persistence should use
// NewServiceWithStore directly. Pass nil for cal when the
// caller never invokes Refresh (e.g. handler-only test paths).
func NewService(p marketdata.MacroDataProvider, timeout time.Duration, cal *industry.EventCalendar) *Service {
	return NewServiceWithStore(p, timeout, NewMemoryRollingSampleStore(defaultHistoryLimit), cal)
}

// NewServiceWithStore wires a custom rolling sample store and
// trading-day calendar into the Service. LatestDaily reads
// through store.History; Refresh writes through store.UpsertDay
// (exactly once per call). Passing a nil store is allowed for
// tests that exercise only the provider → Score pipeline, but
// Refresh and the history-based Z-score path will return errors
// in that configuration. Passing a nil cal disables the
// non-trading-day skip-and-log guard (see Service struct doc).
func NewServiceWithStore(p marketdata.MacroDataProvider, timeout time.Duration, store RollingSampleStore, cal *industry.EventCalendar) *Service {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Service{
		provider:      p,
		extractor:     NewForceExtractor(),
		timeout:       timeout,
		store:         store,
		eventCalendar: cal,
	}
}

// Store returns the rolling sample store the Service was wired with.
// Exported so cmd/atlas's wire_recommender test can assert that the
// production path used NewServiceWithStore(p, 0, store) rather than
// the in-memory fallback. Production readers should treat the value
// as opaque — the only public read path is History(...), never
// direct access to the underlying file/memory map.
func (s *Service) Store() RollingSampleStore { return s.store }

// QualityScore returns a signed score in [-1, 1] derived from the
// latest cached ResonanceResult. Mapping (resonanceToScore):
//
//	score = sign(direction) * max(0.5, coefficient - 0.5)
//
// so bullish alignment (coefficient 1.5, dir bullish) → +1,
// bearish alignment (coefficient 1.5, dir bearish) → -1,
// typical alignment (coefficient 1.0, dir bullish) → +0.5,
// coefficient 0.5 with a non-neutral direction → ±0.5 (floor),
// mixed / neutral → 0.
//
// M5 (audit): the previous doc claimed
// (coefficient - 1.0) * 2.0 * sign(direction), which disagrees with
// the implementation (it would map coefficient 1.0 bullish → 0 and
// could emit ±1.0 only at coefficient 1.5). The implementation is the
// behavior eventdriven's scaleQualityScoreToBaseline depends on and
// is kept; this comment now documents it exactly.
//
// Returns 0 if no successful resonance has been observed yet.
// Auto-refreshes when the cache is older than QualityCacheTTL.
//
// E07 note: this is the legacy resonance-derived compatibility score,
// distinct from DailyReport.QualityScore's F+Inst-Retail Z composite.
// While the assessment's CalibrationStatus is "calibrating" or
// "degraded", this value MUST NOT be fed into automation — callers
// must gate on Service.LatestAssessment().EligibleForAutomation().
// See spec §9.5 / CF-INV-13.
//
// Note: refreshIfStale threads the rolling-store history through
// Score (same path as LatestDaily), so QualityScore reflects
// "today's snapshot against prior samples" once Refresh has
// populated the store; before that, history is empty and every
// Z-score is pinned to 0 (neutral).
func (s *Service) QualityScore() float64 {
	return resonanceToScore(s.refreshIfStale())
}

// QualityLabel returns the direction label for the latest cached
// resonance ("bullish" / "bearish" / "mixed" / "neutral").
// Auto-refreshes when stale. Returns "neutral" when no successful
// resonance has been observed yet.
func (s *Service) QualityLabel() string {
	r := s.refreshIfStale()
	if r.Direction == "" {
		return "neutral"
	}
	return r.Direction
}

// resonanceToScore maps a ResonanceResult to the legacy signed quality
// score consumed by QualityScore and by eventdriven's
// scaleQualityScoreToBaseline:
//
//	bullish → +max(0.5, coefficient-0.5)
//	bearish → -max(0.5, coefficient-0.5)
//	mixed / neutral / "" → 0
//
// The ±0.5 floor keeps every non-neutral direction at a non-zero
// magnitude even at the minimum coefficient (0.5), so the eventdriven
// baseline never treats a real direction as "no signal". M5: the
// implementation is kept (changing it would shift the eventdriven
// baseline semantics); this doc now matches it exactly.
func resonanceToScore(r ResonanceResult) float64 {
	switch r.Direction {
	case "bullish":
		return math.Max(0.5, r.Coefficient-0.5)
	case "bearish":
		return -math.Max(0.5, r.Coefficient-0.5)
	default:
		return 0
	}
}

// refreshIfStale returns the cached ResonanceResult, refreshing it
// when older than QualityCacheTTL or when the cache has never
// been populated. Concurrent callers serialize on the write lock.
// A failed refresh leaves the previous cached value intact so
// stale-but-better-than-nothing wins over zeros during provider
// outages.
//
// BK-15: refreshIfStale no longer pushes into an in-memory rolling
// window — it delegates to extractAsOf, which reads prior samples
// from the rolling store (strictly before the as-of date, spec §8.4)
// and runs Score against them. With an unpopulated store the history
// is empty and every Z-score is pinned to 0 (neutral).
func (s *Service) refreshIfStale() ResonanceResult {
	s.mu.RLock()
	if !s.cachedAt.IsZero() && time.Since(s.cachedAt) < QualityCacheTTL {
		r := s.cachedResonance
		s.mu.RUnlock()
		return r
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.cachedAt.IsZero() && time.Since(s.cachedAt) < QualityCacheTTL {
		return s.cachedResonance
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	snap, err := s.provider.FetchSnapshot(ctx)
	if err != nil {
		return s.cachedResonance
	}
	// FIX-4: thread the rolling-store history through scoring so Z-scores are
	// computed against real prior samples instead of an empty window (which
	// pinned every force to Z=0 / raw and made 品質 read 0.00). Same
	// strictly-before-as-of path as LatestDaily/extractAsOf (spec §8.4).
	// M4: the as-of date is derived in Asia/Taipei (deriveTradingDate) so it
	// matches the TradingDate key Refresh persists.
	derivedDate := deriveTradingDate(snap.RecordedAt)
	forces, err := s.extractAsOf(ctx, snap, derivedDate)
	if err != nil {
		return s.cachedResonance
	}
	s.cachedResonance = ComputeResonance(forces)
	s.cachedAt = time.Now()
	return s.cachedResonance
}

// nonTradingSkipStreakWarn is the number of consecutive non-trading-day skips
// after which the skip itself is escalated to WARN, once (issue #1947). A
// normal weekend reaches it once per weekend, so the escalation is not
// periodic noise, while an unexpected run of skips shows up in logs.
const nonTradingSkipStreakWarn = 3

// recordNonTradingSkip logs a CF-INV-16 skip and maintains the consecutive
// skip counter.
//
// Observability (issue #1947): the production radar was frozen from
// 2026-09-22 while every layer reported healthy — `skip_non_trading_day` was
// log INFO, the task ran, and consecutive_failures stayed 0. Two things now
// make a wrong skip visible:
//
//  1. skipAlertLevel escalates to WARN when the snapshot carries capital-flow
//     inputs (a skip with usable readings is self-contradictory: TWSE
//     publishes nothing on a holiday, so at least one of the two judgements
//     is wrong) and when the skip streak first crosses
//     nonTradingSkipStreakWarn.
//  2. every skip carries `consecutive_skips`, so a frozen store can be read
//     straight off the log instead of from the store's mtime.
func (s *Service) recordNonTradingSkip(date string, snap marketdata.MacroDataSnapshot) {
	s.skipMu.Lock()
	s.consecutiveSkips++
	streak := s.consecutiveSkips
	s.skipMu.Unlock()

	present := presentCapitalFlowInputs(snap)
	fields := []any{
		logging.FStr("date", date),
		logging.FInt("consecutive_skips", streak),
		logging.FInt("recorded_at", int(snap.RecordedAt)),
	}
	if present != "" {
		fields = append(fields, logging.FStr("inputs_present", present))
	}
	switch skipAlertLevel(present, streak) {
	case "warn":
		logging.Warn("capitalflow", "skip_non_trading_day_suspicious", fields...)
	default:
		logging.Info("capitalflow", "skip_non_trading_day", fields...)
	}
}

// resetNonTradingSkip clears the consecutive-skip counter once a trading day
// has been reached and the refresh proceeds.
func (s *Service) resetNonTradingSkip() {
	s.skipMu.Lock()
	s.consecutiveSkips = 0
	s.skipMu.Unlock()
}

// skipCount returns the current consecutive non-trading-day skip count.
// Unexported accessor kept for tests and for any future health endpoint that
// wants to expose "the rolling store has not advanced for N skips".
func (s *Service) skipCount() int {
	s.skipMu.Lock()
	defer s.skipMu.Unlock()
	return s.consecutiveSkips
}

// skipAlertLevel returns the log level ("warn" or "info") for a
// non-trading-day skip. present is the comma-joined list of capital-flow
// inputs the snapshot carried ("" = none); streak is the 1-based number of
// consecutive skips including this one.
//
// Pure so the escalation policy is unit-testable without a log sink.
func skipAlertLevel(present string, streak int) string {
	if present != "" {
		return "warn"
	}
	if streak == nonTradingSkipStreakWarn {
		return "warn"
	}
	return "info"
}

// presentCapitalFlowInputs lists the capital-flow inputs the snapshot actually
// carries, or "" when it carries none.
//
// A skip is only legitimate when upstream published nothing; any capital-flow
// input being present on a non-trading day means the calendar judgement is
// wrong (issue #1947: `government_flow`/`taiex`/`market_volume` were fresh
// while capitalflow skipped 2026-09-23 and 2026-09-24). The seven dimensions
// contribute eight channels because retail has two (margin balance + short
// balance).
func presentCapitalFlowInputs(snap marketdata.MacroDataSnapshot) string {
	candidates := []struct {
		name string
		ok   bool
	}{
		{"foreign", snap.ForeignInvestorNet.Symbol != ""},
		{"institutional", snap.DomesticFundNet.Symbol != ""},
		{"dealer", snap.DealerNet.Symbol != ""},
		{"futures_oi", snap.ForeignFuturesOINet.Symbol != ""},
		{"government", snap.GovernmentNet.Symbol != ""},
		{"retail_margin", snap.RetailMarginBalance.Symbol != ""},
		{"retail_short", snap.RetailShortBalance.Symbol != ""},
		{"tsm_adr", snap.TSMADR.Symbol != ""},
	}
	present := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c.ok {
			present = append(present, c.name)
		}
	}
	return strings.Join(present, ",")
}

// warnIfLongHolidayWindowCoversTradingDay reports the issue #1947 divergence:
// the authoritative holiday table says "trading day" while the wired
// industry.EventCalendar reports the date inside a long_holiday window.
//
// It only logs. A long_holiday event is a [holiday-3d, holiday+2d] market
// sentiment window, not a market closure, so it must never suppress the
// refresh; but the divergence is exactly what froze the radar, so it is
// worth a WARN while it exists (one line per refresh tick, matching the
// cadence of the condition).
func (s *Service) warnIfLongHolidayWindowCoversTradingDay(date time.Time, tradingDate string) {
	if s.eventCalendar == nil {
		return
	}
	if s.eventCalendar.IsTaiwanTradingDay(date) {
		return
	}
	logging.Warn("capitalflow", "long_holiday_window_covers_trading_day",
		logging.FStr("date", tradingDate),
		logging.FStr("hint", "industry.EventCalendar reports a long-holiday window here; marketdata.IsTaiwanTradingDay says trading day (authoritative) and Refresh proceeds (issue #1947)"))
}

// Refresh fetches a fresh snapshot and persists the available
// dimensions as RollingSamples for the snapshot's own trading
// date, exactly once. It is the only writer to s.store (BK-15 /
// spec §8.5): LatestDaily, Summary, QualityScore, and
// refreshIfStale never call UpsertDay.
//
// Data-driven keying (CF-INV-15): the trading-date key is
// derived from snap.RecordedAt (converted to Asia/Taipei
// YYYY-MM-DD), not from the caller's wall clock. This decouples
// the write key from cron execution time, which previously caused
// a cutoff+last-write-wins overwrite trap (see docs/manifests/
// 2026-07-20-capital-flow-history-audit.md §證據鏈摘要).
//
// Dated-channel keying (CF-INV-18, issue #1940 R2): CF-INV-15 holds
// for the five same-day channels. The two channels whose reading
// carries its own date (government_flow's YYYYMMDD.json, TAIFEX
// futures OI) are keyed by that reading date instead — the government
// file for day D is only published on D+1, so stamping it D+1 both
// shifts the value by one session and re-stamps the same file once per
// calendar day. dimensionSampleDate (reading_dates.go) is the single
// decision point; see spec §18.8.
//
// Non-trading-day skip (CF-INV-16): if the snapshot's date is
// not a Taiwan trading day per marketdata.IsTaiwanTradingDay (the
// authoritative taiwanholidays table — see issue #1947 for why
// industry.EventCalendar must not be used here), Refresh returns nil
// after a skip-and-log — no empty sample is written (CF-INV-06) and
// no error is raised (avoids noisy retries).
//
// Observability of the skip path (issue #1947): every skip logs
// consecutive_skips, and skipAlertLevel escalates to WARN when the
// snapshot actually carries capital-flow inputs (a contradictory skip)
// or when the streak first crosses nonTradingSkipStreakWarn. The gate
// needs no wiring, so there is no "nil calendar → treat as trading day"
// degradation any more; a nil eventCalendar only disables the
// long_holiday-window warning.
//
// Errors (wrapped with %w for errors.Is / errors.As):
//   - nil store: the wiring is incomplete for the write path;
//   - provider fetch failure: propagated so callers can retry;
//   - empty snapshot (every source channel was empty): returning a
//     wrapped error makes the missing-day condition visible
//     instead of silently dropping the day's reading
//     (spec §8.3 / CF-INV-06);
//   - store.UpsertDay failure: propagated so callers can decide
//     whether to retry the same trading date. When dated channels
//     produce a second key, each key is upserted independently,
//     ascending; a failure on one key leaves the others written and
//     the next 5-minute refresh re-derives all of them.
func (s *Service) Refresh(ctx context.Context) error {
	if s.store == nil {
		return fmt.Errorf("capitalflow: Refresh called with nil rolling store")
	}
	snap, err := s.provider.FetchSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("capitalflow: Refresh fetch snapshot: %w", err)
	}

	recordTime := time.Unix(snap.RecordedAt, 0).In(taipeiZone)
	currentDate := deriveTradingDate(snap.RecordedAt)

	// CF-INV-16 trading-day gate. The authority is marketdata.IsTaiwanTradingDay
	// (→ internal/taiwanholidays.IsTradingDay), the table 19 other call sites
	// use. It must NOT be industry.EventCalendar.IsTaiwanTradingDay: that
	// method returns false for ANY date inside a long_holiday *window*, and
	// buildHolidayEvent defines every public holiday as a window of
	// [holiday-3d, holiday+2d]. For 2026 中秋 (09-25) the window is
	// 2026-09-22..09-27, so 09-23 and 09-24 — both real trading days — were
	// judged non-trading and the radar stopped advancing from 2026-09-22
	// (issue #1947; the store last advanced at 09-22 07:59, i.e. just before
	// the window opened in UTC terms).
	if !marketdata.IsTaiwanTradingDay(recordTime) {
		s.recordNonTradingSkip(currentDate, snap)
		return nil
	}
	s.resetNonTradingSkip()
	// The event calendar is event data, not a holiday table: report the
	// divergence, never act on it (issue #1947).
	s.warnIfLongHolidayWindowCoversTradingDay(recordTime, currentDate)

	forces := s.extractor.Score(snap, currentDate, nil)
	// Issue #1940 R2: a dimension whose reading carries its own date must be
	// persisted under THAT date, not the refresh run's trading day — the
	// government file for day D is published on D+1, and stamping it D+1 is
	// exactly the +1 shift the issue reports. Samples are therefore grouped
	// by key and each key is upserted once, keeping CF-INV-05 ("at most one
	// sample per (dimension, trading_date)") intact.
	byDate := make(map[string][]RollingSample, 1)
	for _, f := range forces {
		if !f.DataAvailable {
			continue
		}
		unit, sourceID := dimensionSource(f.Force)
		key := dimensionSampleDate(snap, f.Force, currentDate)
		byDate[key] = append(byDate[key], RollingSample{
			TradingDate: key,
			Dimension:   f.Force,
			RawValue:    f.RawValue,
			Unit:        unit,
			SourceID:    sourceID,
		})
	}
	if len(byDate) == 0 {
		return fmt.Errorf("capitalflow: Refresh on %s produced no samples (every source channel was empty; spec §8.3 / CF-INV-06 forbids zero-valued fallbacks)", currentDate)
	}
	for _, key := range slices.Sorted(maps.Keys(byDate)) {
		if err := s.store.UpsertDay(ctx, key, byDate[key]); err != nil {
			return fmt.Errorf("capitalflow: Refresh upsert %s: %w", key, err)
		}
	}
	return nil
}

// LatestDaily runs the FetchSnapshot → Score(history) →
// ComputeResonance → GenerateDailyReport pipeline as a Go call.
//
// derivedDate is the trading date used as the History upper bound;
// it is derived from snap.RecordedAt in Asia/Taipei via
// deriveTradingDate, matching the TradingDate key Refresh persists
// (CF-INV-15 / M4) so the read and write paths never disagree on the
// date key around the UTC midnight boundary. The rolling-history
// lookup is per-dimension against s.store with a strictly-before
// `derivedDate` upper bound so today's reading never bleeds into
// its own reference window (spec §8.4).
//
// This method never calls UpsertDay: it is a pure read, satisfying
// spec §8.1 / CF-INV-04. The only writer is Refresh.
func (s *Service) LatestDaily(ctx context.Context) (DailyReport, error) {
	s.reportMu.RLock()
	if s.cachedReport != nil && time.Since(s.reportCachedAt) < LatestDailyCacheTTL {
		report := *s.cachedReport
		s.reportMu.RUnlock()
		return report, nil
	}
	s.reportMu.RUnlock()

	cctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	snap, err := s.provider.FetchSnapshot(cctx)
	if err != nil {
		return DailyReport{}, err
	}
	derivedDate := deriveTradingDate(snap.RecordedAt)
	forces, err := s.extractAsOf(cctx, snap, derivedDate)
	if err != nil {
		return DailyReport{}, err
	}
	date := time.Unix(snap.RecordedAt, 0)
	resonance := ComputeResonance(forces)
	// PR-3a: feed the current period + the config-gated quality formula.
	// The config default (capitalflow.period_weighted_quality=false)
	// keeps quality_score bit-identical to the legacy composite.
	report := GenerateDailyReport(date, forces, resonance,
		s.periodFor(derivedDate), config.GetCapitalflowPeriodWeightedQuality())

	s.reportMu.Lock()
	s.cachedReport = &report
	s.reportCachedAt = time.Now()
	s.reportMu.Unlock()
	return report, nil
}

// extractAsOf builds the rolling-history map for every capital
// dimension and runs Score against it. Each dimension's history
// is fetched from s.store with a strictly-before `derivedDate`
// upper bound so today's reading never bleeds into its own
// reference window (spec §8.4).
//
// When s.store is nil (a defensive path — NewService always wires
// a MemoryRollingSampleStore), every dimension gets an empty
// history and Score returns Z=raw for non-zero values. This
// matches the pre-BK-15 "fresh process" behavior for processes
// that have not called Refresh at all.
func (s *Service) extractAsOf(ctx context.Context, snap marketdata.MacroDataSnapshot, derivedDate string) ([]ForceScore, error) {
	history := make(map[ForceName][]RollingSample, 7)
	if s.store != nil {
		for _, dim := range []ForceName{
			ForceForeign, ForceFutures, ForceTSMADR,
			ForceInstitutional, ForceDealer, ForceGovernment, ForceRetail,
		} {
			// Issue #1940 R2: for a dated channel the window must end strictly
			// before the READING's own date, not before the report's trading
			// day, or the reading ends up inside its own reference window
			// (spec §8.4) once the write path keys it correctly.
			bound := dimensionSampleDate(snap, dim, derivedDate)
			samples, err := s.store.History(ctx, dim, bound, defaultHistoryLimit)
			if err != nil {
				return nil, fmt.Errorf("capitalflow: history %s before %s: %w", dim, bound, err)
			}
			history[dim] = samples
		}
	}
	return s.extractor.Score(snap, derivedDate, history), nil
}

// Summary returns the latest summary report by reusing
// LatestDaily's FetchSnapshot → Score → ComputeResonance pipeline.
// It exists to give non-HTTP consumers (background jobs, internal
// adapters such as internal/recommender) a SummaryReport without
// routing through Handler.HandleSummary (which requires
// *http.Request).
//
// Caller cost: a single provider fetch + Score + ComputeResonance,
// shared with LatestDaily if both are called on the same snapshot.
// SummaryReport is derived deterministically from the same
// (date, forces, resonance) tuple that feeds DailyReport.
func (s *Service) Summary(ctx context.Context) (SummaryReport, error) {
	daily, err := s.LatestDaily(ctx)
	if err != nil {
		return SummaryReport{}, fmt.Errorf("capitalflow: build summary from latest daily: %w", err)
	}
	return GenerateSummaryReport(daily.Date, daily.Forces, daily.Resonance,
		s.periodFor(deriveTradingDateFromReport(daily.Date)),
		config.GetCapitalflowPeriodWeightedQuality()), nil
}

// deriveTradingDateFromReport converts a report date back to the
// Asia/Taipei YYYY-MM-DD trading-date key used by the period provider
// (PR-3a). LatestDaily already stores the report under the derived date,
// so this keeps Summary consistent with it.
func deriveTradingDateFromReport(date time.Time) string {
	return date.In(taipeiZone).Format("2006-01-02")
}

// LatestAssessment is the E07 automation face (spec §9.5 /
// CF-INV-08 / CF-INV-13). It returns the E07 4-layer assessment
// for the latest trading day by reusing the LatestDaily pipeline
// (no extra provider fetch, no extra score pass).
//
// On a fresh service the assessment is always
// CalibrationStatus="calibrating" because no rolling history has
// been written yet (Refresh has not run); automation consumers
// MUST gate on EligibleForAutomation() and stay neutral while
// the gate is closed. Once Refresh has been called the assessment
// still reports "calibrating" until H-CF-02 is validated — that
// flip lives in the per-source calibration pipeline that Task 8
// will wire.
func (s *Service) LatestAssessment(ctx context.Context) (CapitalFlowAssessment, error) {
	daily, err := s.LatestDaily(ctx)
	if err != nil {
		return CapitalFlowAssessment{}, fmt.Errorf("capitalflow: build latest assessment: %w", err)
	}
	return daily.Assessment, nil
}

// dimensionSource returns the (unit, source_id) tuple to attach to
// a RollingSample for the given capital dimension.
//
// M3 (audit): this used to be a second, hand-maintained provenance
// table that contradicted ComputeForceProvenance on government
// (hundred_million_shares vs twd), retail (hundred_million_shares
// vs pct_composite) and TSM ADR (percent/SourceYahoo vs
// pct/SourceSECTSMC). It now delegates to ComputeForceProvenance
// (forces.go) so the write path (Refresh) persists exactly the
// same unit/source metadata the extractor and the API surface
// expose — spec §6 / CF-INV-01 / CF-INV-11 single source of truth.
// T86 三法人 keep hundred_million_shares (spec §5.1: T86 億股 proxy).
func dimensionSource(dim ForceName) (unit, sourceID string) {
	prov := ComputeForceProvenance(dim)
	return prov.Unit, prov.SourceID
}
