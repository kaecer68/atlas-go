package capitalflow

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

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
// eventCalendar (added in CL-1 fix, spec CF-INV-16) is the
// Taiwan trading-day calendar used by Refresh to skip non-trading
// days. Production wiring (cmd/atlas/main.go) passes the shared
// *industry.EventCalendar instance created at main.go:427.
// Tests that do not call Refresh may pass nil; Refresh itself
// performs a defensive nil-check (logs warning, treats as
// trading day) so a missing calendar never panics in production
// due to wiring bugs — it just stops filtering weekend data.
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
// latest cached ResonanceResult. Mapping:
//
//	score = (coefficient - 1.0) * 2.0 * sign(direction)
//
// so bullish alignment (coefficient 1.5, dir bullish) → +1,
// bearish alignment (coefficient 1.5, dir bearish) → -1,
// mixed / neutral → 0.
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
// Note: refreshIfStale runs Score with an empty history today, so
// until Refresh has populated the store, QualityScore reflects
// "today's snapshot with zero prior samples" (Z=raw for non-zero
// values). See Task 4 report §Concerns.
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
// window — Extract now delegates to Score(history=nil), so the
// cached resonance reflects "today's snapshot with empty prior
// samples". This is a known limitation tracked in the Task 4
// report §Concerns.
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
	forces := s.extractor.Extract(snap)
	s.cachedResonance = ComputeResonance(forces)
	s.cachedAt = time.Now()
	return s.cachedResonance
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
// Non-trading-day skip (CF-INV-16): if the snapshot's date is
// not a Taiwan trading day per s.eventCalendar.IsTaiwanTradingDay,
// Refresh returns nil after a skip-and-log — no empty sample is
// written (CF-INV-06) and no error is raised (avoids noisy
// retries). A nil eventCalendar degrades to "treat as trading
// day" with a warning log so a missing-wiring bug surfaces in
// observability without breaking the hot path.
//
// Errors (wrapped with %w for errors.Is / errors.As):
//   - nil store: the wiring is incomplete for the write path;
//   - provider fetch failure: propagated so callers can retry;
//   - empty snapshot (every source channel was empty): returning a
//     wrapped error makes the missing-day condition visible
//     instead of silently dropping the day's reading
//     (spec §8.3 / CF-INV-06);
//   - store.UpsertDay failure: propagated so callers can decide
//     whether to retry the same trading date.
func (s *Service) Refresh(ctx context.Context) error {
	if s.store == nil {
		return fmt.Errorf("capitalflow: Refresh called with nil rolling store")
	}
	snap, err := s.provider.FetchSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("capitalflow: Refresh fetch snapshot: %w", err)
	}

	taipei := time.FixedZone("Asia/Taipei", 8*3600)
	recordTime := time.Unix(snap.RecordedAt, 0).In(taipei)
	currentDate := recordTime.Format("2006-01-02")

	if s.eventCalendar == nil {
		logging.Warn("capitalflow", "refresh_no_calendar",
			logging.FStr("date", currentDate))
	} else if !s.eventCalendar.IsTaiwanTradingDay(recordTime) {
		logging.Info("capitalflow", "skip_non_trading_day",
			logging.FStr("date", currentDate),
			logging.FInt("recorded_at", int(snap.RecordedAt)))
		return nil
	}

	forces := s.extractor.Score(snap, currentDate, nil)
	var samples []RollingSample
	for _, f := range forces {
		if !f.DataAvailable {
			continue
		}
		unit, sourceID := dimensionSource(f.Force)
		samples = append(samples, RollingSample{
			TradingDate: currentDate,
			Dimension:   f.Force,
			RawValue:    f.RawValue,
			Unit:        unit,
			SourceID:    sourceID,
		})
	}
	if len(samples) == 0 {
		return fmt.Errorf("capitalflow: Refresh on %s produced no samples (every source channel was empty; spec §8.3 / CF-INV-06 forbids zero-valued fallbacks)", currentDate)
	}
	if err := s.store.UpsertDay(ctx, currentDate, samples); err != nil {
		return fmt.Errorf("capitalflow: Refresh upsert %s: %w", currentDate, err)
	}
	return nil
}

// LatestDaily runs the FetchSnapshot → Score(history) →
// ComputeResonance → GenerateDailyReport pipeline as a Go call.
//
// derivedDate is the trading date used as the History upper bound;
// it is taken from snap.RecordedAt (UTC, kept for back-compat —
// Task 5 will introduce Taipei-timezone derivation per spec §6's
// `as_of_trading_date` field). The rolling-history lookup is
// per-dimension against s.store with a strictly-before
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
	derivedDate := time.Unix(snap.RecordedAt, 0).Format("2006-01-02")
	forces, err := s.extractAsOf(cctx, snap, derivedDate)
	if err != nil {
		return DailyReport{}, err
	}
	date := time.Unix(snap.RecordedAt, 0)
	resonance := ComputeResonance(forces)
	report := GenerateDailyReport(date, forces, resonance)

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
			samples, err := s.store.History(ctx, dim, derivedDate, defaultHistoryLimit)
			if err != nil {
				return nil, fmt.Errorf("capitalflow: history %s before %s: %w", dim, derivedDate, err)
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
	return GenerateSummaryReport(daily.Date, daily.Forces, daily.Resonance), nil
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
// a RollingSample for the given capital dimension, per the source
// registry in docs/specs/capital-flow-seven-dimension-spec.md §5
// and the rolling_store.go source-id constants. Keeping the table
// here (instead of on ForceExtractor) makes Refresh a single
// switch — the extractor stays focused on scoring, the persistence
// writer owns source provenance.
//
// Spec §7 calls these out per dimension: foreign/institutional/
// dealer share TWSE-T86 (T86 億股 proxy), government uses an
// operator-imported source, futures uses TAIFEX institutional OI
// (口數), retail uses TWSE margin/short balance (percent), TSM ADR
// uses the Yahoo-derived daily change.
func dimensionSource(dim ForceName) (unit, sourceID string) {
	switch dim {
	case ForceForeign, ForceInstitutional, ForceDealer:
		return "hundred_million_shares", SourceTWSET86
	case ForceGovernment:
		return "hundred_million_shares", SourceGovernmentOperator
	case ForceFutures:
		return "contracts", SourceTAIFEXInst
	case ForceRetail:
		return "hundred_million_shares", SourceTWSEODDLOT
	case ForceTSMADR:
		return "percent", SourceYahoo
	}
	return "", ""
}
