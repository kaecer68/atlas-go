package repository

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// DualWriteRepository implements dual-write to both JSONL and PostgreSQL.
// It writes to both backends for data safety during migration.
// Reads prefer PostgreSQL, fallback to JSONL if PG is unavailable.
type DualWriteRepository struct {
	pg    *PostgresRepository
	jsonl *JSONLRepository

	// PG health state (BL-06/Step2): pgUsable() now performs a live SELECT 1
	// probe with a short TTL cache, so a dead PostgreSQL (e.g. the 2026-07-29
	// timescaledb version mismatch that silently broke every query for 15 days)
	// is surfaced via logs and state transitions instead of being silently
	// swallowed by `_ =` call sites.
	pgHealthMu     sync.Mutex
	pgHealthy      bool
	pgCheckedAt    time.Time
	pgHealthWarned bool // true once we've logged the unhealthy transition
}

const pgHealthCheckTTL = 15 * time.Second

// fallbackCounter tracks how many times LoadAllSessionSummaries falls back
// to JSONL because PostgreSQL is unavailable or returned an error.
var fallbackCounter atomic.Int64

// DualWriteFallbackTotal returns the current count of JSONL fallbacks in
// LoadAllSessionSummaries. Exposed for monitoring/alerting consumption.
func DualWriteFallbackTotal() int64 {
	return fallbackCounter.Load()
}

// scorecardSlimRepoFallbackTotal counts observatory slim-projection fallbacks
// at the DualWriteRepository layer (#1780 Phase 1, B1): a JSONL outcome store
// that lacks the optional LoadScorecardOutcomes method makes
// QueryScorecardOutcomes fall back to the full QueryAllOutcomes metadata read
// — the exact ~1.9GB OOM pattern #1780 eliminates. A non-zero delta since
// deploy means the slim path is NOT active. Mirrors the DualWriteFallbackTotal
// observability pattern.
var scorecardSlimRepoFallbackTotal atomic.Int64

// ScorecardSlimRepoFallbackTotal returns the total DualWriteRepository
// slim-projection fallbacks. Exposed for monitoring/alerting consumption.
func ScorecardSlimRepoFallbackTotal() int64 {
	return scorecardSlimRepoFallbackTotal.Load()
}

// B6 (session summary dual-write consistency): session-summary write and read
// error counters. These replace the silent `_ =` JSONL error swallowing so a
// one-sided write/read failure is observable via monitoring instead of
// producing a silently split PG/JSONL history.
var (
	// sessionSummaryJSONLErrorTotal counts session-summary writes whose
	// JSONL side failed (summary.json / sqlite side). Incremented together
	// with sessionSummaryReconcilePendingTotal.
	sessionSummaryJSONLErrorTotal atomic.Int64

	// sessionSummaryPGErrorTotal counts session-summary writes whose
	// PostgreSQL side failed while the PG backend was usable.
	sessionSummaryPGErrorTotal atomic.Int64

	// sessionSummaryReconcilePendingTotal is the B6 "reconcile pending"
	// marker counter: incremented on ANY one-sided session-summary write
	// failure. A non-zero delta since the last reconcile run means the two
	// backends may have drifted and cmd/reconcile-sessions should be run.
	sessionSummaryReconcilePendingTotal atomic.Int64

	// sessionSummaryMergeDivergenceTotal counts sessions whose summary
	// existed on BOTH sides of LoadAllSessionSummaries with different
	// content/timestamps (newer-wins merge, PG tiebreak). These are the
	// read-path evidence of dual-write drift.
	sessionSummaryMergeDivergenceTotal atomic.Int64

	// sessionSummaryJSONLOnlyTotal / sessionSummaryPGOnlyTotal count
	// sessions served from only one side during the merged read path
	// (one-sided gaps that reconcile would backfill).
	sessionSummaryJSONLOnlyTotal atomic.Int64
	sessionSummaryPGOnlyTotal    atomic.Int64
)

// SessionSummaryJSONLErrorTotal returns the total JSONL-side session-summary
// write failures (B6). Exposed for monitoring/alerting consumption.
func SessionSummaryJSONLErrorTotal() int64 { return sessionSummaryJSONLErrorTotal.Load() }

// SessionSummaryPGErrorTotal returns the total PostgreSQL-side session-summary
// write failures observed while PG was usable (B6).
func SessionSummaryPGErrorTotal() int64 { return sessionSummaryPGErrorTotal.Load() }

// SessionSummaryReconcilePendingTotal returns the B6 reconcile-pending marker
// count: total one-sided session-summary write failures observed at runtime.
func SessionSummaryReconcilePendingTotal() int64 {
	return sessionSummaryReconcilePendingTotal.Load()
}

// SessionSummaryMergeDivergenceTotal returns the number of sessions resolved
// as divergent during the merged LoadAllSessionSummaries read (B6).
func SessionSummaryMergeDivergenceTotal() int64 {
	return sessionSummaryMergeDivergenceTotal.Load()
}

// SessionSummaryJSONLOnlyTotal returns the number of sessions served only from
// JSONL during the merged read path (PG missing them — a backfill candidate).
func SessionSummaryJSONLOnlyTotal() int64 { return sessionSummaryJSONLOnlyTotal.Load() }

// SessionSummaryPGOnlyTotal returns the number of sessions served only from PG
// during the merged read path (JSONL missing them — a backfill candidate).
func SessionSummaryPGOnlyTotal() int64 { return sessionSummaryPGOnlyTotal.Load() }

type JSONLRepository struct {
	alertStore             AlertStore
	metricsStore           MetricsStore
	outcomeStore           OutcomeStore
	screeningRejectStore   ScreeningRejectStore
	sessionSummaryStore    SessionSummaryStore
	humanInterventionStore HumanInterventionStore
}

// AlertStore defines the interface for the existing alert store
type AlertStore interface {
	Save(alert domain.AlertRecord) error
	LoadAll() ([]domain.AlertRecord, error)
	LoadUnacknowledged() ([]domain.AlertRecord, error)
	Acknowledge(alertID string, user string) error
	FindByDedupKey(dedupKey string) (*domain.AlertRecord, error)
	Update(id string, fn func(*domain.AlertRecord)) error
}

// MetricsStore defines the interface for the existing metrics store
type MetricsStore interface {
	SaveSnapshot(snapshot MetricsSnapshot) error
	LoadToday() (*MetricsSnapshot, error)
	LoadRecent(n int) ([]MetricsSnapshot, error)
}

// OutcomeStore defines the interface for the existing outcome store.
// Mirrors ledger.OutcomeStore for interface compatibility.
type OutcomeStore interface {
	RecordOutcomes(outcomes []domain.RecommendationOutcome) error
	RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error
	LoadOutcomes() ([]domain.RecommendationOutcome, error)
	LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error)
	LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error)

	RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error
	LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error)

	RecordExperiment(record domain.ExperimentRecord) error
	RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error

	RecordHumanIntervention(intervention domain.HumanIntervention) error
	LoadHumanInterventions() ([]domain.HumanIntervention, error)
}

// scorecardOutcomeStore mirrors ledger.ScorecardOutcomeStore for the
// repository-side OutcomeStore contract. Optional narrow interface: stores
// that implement LoadScorecardOutcomes get the slim projection, everything
// else falls back to the full read (B1).
type scorecardOutcomeStore interface {
	LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error)
}

// SessionStore mirrors ledger.SessionStore for interface compatibility.
type SessionStore interface {
	RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error
	LoadSessionSummaries() ([]domain.SessionSummary, error)
	LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error)
}

type ScreeningRejectStore interface {
	RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error
	LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error)
}

type SessionSummaryStore interface {
	RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error
	LoadSessionSummaries() ([]domain.SessionSummary, error)
	LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error)
}

type HumanInterventionStore interface {
	RecordHumanIntervention(intervention domain.HumanIntervention) error
	LoadHumanInterventions() ([]domain.HumanIntervention, error)
}

// NewDualWriteRepository creates a new dual-write repository.
func NewDualWriteRepository(pool *pgxpool.Pool, alertStore AlertStore, metricsStore MetricsStore, outcomeStore OutcomeStore, screeningRejectStore ScreeningRejectStore, sessionSummaryStore SessionSummaryStore, humanInterventionStore HumanInterventionStore) *DualWriteRepository {
	return &DualWriteRepository{
		pg: NewPostgresRepository(pool),
		jsonl: &JSONLRepository{
			alertStore:             alertStore,
			metricsStore:           metricsStore,
			outcomeStore:           outcomeStore,
			screeningRejectStore:   screeningRejectStore,
			sessionSummaryStore:    sessionSummaryStore,
			humanInterventionStore: humanInterventionStore,
		},
	}
}

// pgUsable reports whether the PostgreSQL backend is fully reachable.
// Centralizes the nil-struct + nil-pool guard so direct construction of
// &PostgresRepository{pool: nil} (e.g. in tests or after pool teardown)
// cannot trigger a nil-pointer panic at the Query/Exec call site.
//
// BL-06/Step2: beyond the nil guard, this performs a live SELECT 1 probe
// (TTL-cached) so a PostgreSQL whose pool is non-nil but actually dead (e.g.
// the 2026-07-29 timescaledb version mismatch that broke every query for 15
// days) is surfaced via a logged unhealthy transition instead of every call
// site silently swallowing the failure. On the first detected failure and on
// recovery we log; steady-state unhealthy calls return fast from cache.
func (r *DualWriteRepository) pgUsable() bool {
	// r == nil guards a typed-nil receiver (e.g. InitRepository returns nil
	// when the PG pool is unavailable and the value is later passed as an
	// interface): treat it as JSON-only mode instead of panicking on the r.pg
	// dereference below.
	if r == nil || r.pg == nil || r.pg.pool == nil {
		return false
	}
	ctx := context.Background()

	r.pgHealthMu.Lock()
	defer r.pgHealthMu.Unlock()

	// Serve from cache within TTL to avoid a probe on every dual-write call.
	if !r.pgCheckedAt.IsZero() && time.Since(r.pgCheckedAt) < pgHealthCheckTTL {
		return r.pgHealthy
	}

	// Probe: SELECT 1 via the pool interface (no Ping method on pgPool).
	healthy := true
	var probe int
	if err := r.pg.pool.QueryRow(ctx, "SELECT 1").Scan(&probe); err != nil {
		healthy = false
		if !r.pgHealthWarned {
			r.pgHealthWarned = true
			log.Printf("[repository] PG health FAIL: %v (falling back to SQLite/JSONL)", err)
		}
	} else if r.pgHealthWarned {
		r.pgHealthWarned = false
		log.Printf("[repository] PG health RECOVERED")
	}
	r.pgHealthy = healthy
	r.pgCheckedAt = time.Now()

	return healthy
}

// ============================================
// Metrics Operations
// ============================================

func (r *DualWriteRepository) Record(ctx context.Context, metricName string, value float64, labels map[string]string) error {
	// Always write to JSONL
	_ = r.jsonl.metricsStore.SaveSnapshot(MetricsSnapshot{
		Timestamp: time.Now(),
	})

	// Also write to PostgreSQL (best effort)
	if r.pgUsable() {
		if err := r.pg.Record(ctx, metricName, value, labels); err != nil {
			// Log but don't fail - JSONL is the source of truth during transition
			return nil
		}
	}
	return nil
}

func (r *DualWriteRepository) QueryRange(ctx context.Context, metricName string, start, end time.Time) ([]MetricPoint, error) {
	// Prefer PostgreSQL
	if r.pgUsable() {
		points, err := r.pg.QueryRange(ctx, metricName, start, end)
		if err == nil && len(points) > 0 {
			return points, nil
		}
	}
	return nil, nil
}

func (r *DualWriteRepository) QueryLatest(ctx context.Context, metricName string, labels map[string]string) (*MetricPoint, error) {
	if r.pgUsable() {
		return r.pg.QueryLatest(ctx, metricName, labels)
	}
	return nil, nil
}

func (r *DualWriteRepository) Aggregate(ctx context.Context, metricName string, start, end time.Time, agg string) (float64, error) {
	if r.pgUsable() {
		return r.pg.Aggregate(ctx, metricName, start, end, agg)
	}
	return 0, nil
}

func (r *DualWriteRepository) SaveSnapshot(ctx context.Context, snapshot *MetricsSnapshot) error {
	_ = r.jsonl.metricsStore.SaveSnapshot(*snapshot)
	if r.pgUsable() {
		return r.pg.SaveSnapshot(ctx, snapshot)
	}
	return nil
}

func (r *DualWriteRepository) LoadToday(ctx context.Context) (*MetricsSnapshot, error) {
	// Try PostgreSQL first
	if r.pgUsable() {
		snap, err := r.pg.LoadToday(ctx)
		if err == nil && snap != nil {
			return snap, nil
		}
	}
	// Fallback to JSONL
	return r.jsonl.metricsStore.LoadToday()
}

func (r *DualWriteRepository) LoadRecent(ctx context.Context, n int) ([]MetricsSnapshot, error) {
	if r.pgUsable() {
		snaps, err := r.pg.LoadRecent(ctx, n)
		if err == nil && len(snaps) > 0 {
			return snaps, nil
		}
	}
	return r.jsonl.metricsStore.LoadRecent(n)
}

// ============================================
// Alert Operations
// ============================================

func (r *DualWriteRepository) SaveAlert(ctx context.Context, alert domain.AlertRecord) error {
	_ = r.jsonl.alertStore.Save(alert)
	if r.pgUsable() {
		return r.pg.SaveAlert(ctx, alert)
	}
	return nil
}

func (r *DualWriteRepository) LoadAllAlerts(ctx context.Context, limit int) ([]domain.AlertRecord, error) {
	if r.pgUsable() {
		records, err := r.pg.LoadAllAlerts(ctx, limit)
		if err == nil && len(records) > 0 {
			return records, nil
		}
	}
	return r.jsonl.alertStore.LoadAll()
}

func (r *DualWriteRepository) LoadUnacknowledgedAlerts(ctx context.Context) ([]domain.AlertRecord, error) {
	if r.pgUsable() {
		records, err := r.pg.LoadUnacknowledgedAlerts(ctx)
		if err == nil && len(records) > 0 {
			return records, nil
		}
	}
	return r.jsonl.alertStore.LoadUnacknowledged()
}

func (r *DualWriteRepository) AcknowledgeAlert(ctx context.Context, alertID string, user string) error {
	_ = r.jsonl.alertStore.Acknowledge(alertID, user)
	if r.pgUsable() {
		return r.pg.AcknowledgeAlert(ctx, alertID, user)
	}
	return nil
}

func (r *DualWriteRepository) FindAlertByDedupKey(ctx context.Context, dedupKey string) (*domain.AlertRecord, error) {
	if r.pgUsable() {
		rec, err := r.pg.FindAlertByDedupKey(ctx, dedupKey)
		if err == nil && rec != nil {
			return rec, nil
		}
	}
	return r.jsonl.alertStore.FindByDedupKey(dedupKey)
}

func (r *DualWriteRepository) UpdateAlert(ctx context.Context, id string, fn func(*domain.AlertRecord)) error {
	_ = r.jsonl.alertStore.Update(id, fn)
	if r.pgUsable() {
		return r.pg.UpdateAlertByID(ctx, id, fn)
	}
	return nil
}

func (r *DualWriteRepository) LoadAlertsBySeverity(ctx context.Context, severity string, limit int) ([]domain.AlertRecord, error) {
	if r.pgUsable() {
		return r.pg.LoadAlertsBySeverity(ctx, severity, limit)
	}
	return nil, nil
}

func (r *DualWriteRepository) LoadAlertsByTimeRange(ctx context.Context, start, end time.Time) ([]domain.AlertRecord, error) {
	if r.pgUsable() {
		return r.pg.LoadAlertsByTimeRange(ctx, start, end)
	}
	return nil, nil
}

func (r *DualWriteRepository) QuerySessions(ctx context.Context) ([]SessionInfo, error) {
	if r.pgUsable() {
		return r.pg.QuerySessions(ctx)
	}
	return nil, nil
}

// ============================================
// Outcome Operations
// ============================================

func (r *DualWriteRepository) RecordOutcomes(ctx context.Context, outcomes []domain.RecommendationOutcome) error {
	_ = r.jsonl.outcomeStore.RecordOutcomes(outcomes)
	if r.pgUsable() {
		return r.pg.RecordOutcomes(ctx, outcomes)
	}
	return nil
}

func (r *DualWriteRepository) QueryOutcomesBySession(ctx context.Context, sessionID string) ([]domain.RecommendationOutcome, error) {
	if r.pgUsable() {
		outcomes, err := r.pg.QueryOutcomesBySession(ctx, sessionID)
		if err == nil && len(outcomes) > 0 {
			return outcomes, nil
		}
	}
	return r.jsonl.outcomeStore.LoadSessionOutcomes(sessionID)
}

func (r *DualWriteRepository) QueryOutcomesBySymbol(ctx context.Context, symbol string, start, end time.Time) ([]domain.RecommendationOutcome, error) {
	if r.pgUsable() {
		return r.pg.QueryOutcomesBySymbol(ctx, symbol, start, end)
	}
	return nil, nil
}

func (r *DualWriteRepository) QueryOutcomesByAgent(ctx context.Context, agentID string, start, end time.Time) ([]domain.RecommendationOutcome, error) {
	if r.pgUsable() {
		return r.pg.QueryOutcomesByAgent(ctx, agentID, start, end)
	}
	return nil, nil
}

func (r *DualWriteRepository) QueryPassRate(ctx context.Context, agentID string, window time.Duration) (float64, error) {
	if r.pgUsable() {
		return r.pg.QueryPassRate(ctx, agentID, window)
	}
	return 0, nil
}

func (r *DualWriteRepository) QueryTopSymbols(ctx context.Context, limit int, start, end time.Time) ([]SymbolCount, error) {
	if r.pgUsable() {
		return r.pg.QueryTopSymbols(ctx, limit, start, end)
	}
	return nil, nil
}

func (r *DualWriteRepository) QueryAllOutcomes(ctx context.Context) ([]domain.RecommendationOutcome, error) {
	return r.jsonl.outcomeStore.LoadOutcomesFromSessions()
}

// QueryScorecardOutcomes reads the observatory scorecard slim projection (8
// scalar fields from JSONB, #1780 Phase 1) when the JSONL outcome store
// implements the optional scorecardOutcomeStore interface; otherwise it falls
// back to the full QueryAllOutcomes metadata read with a warn log and counter
// (B1). The nil outcome-store guard keeps the JSON-only cmd paths (judge /
// macro-ingest construct DualWriteRepository with nil stores) failing loudly
// instead of panicking on a nil dereference inside QueryAllOutcomes.
func (r *DualWriteRepository) QueryScorecardOutcomes(ctx context.Context) ([]domain.RecommendationOutcome, error) {
	if r.jsonl.outcomeStore == nil {
		return nil, fmt.Errorf("query scorecard outcomes: outcome store unavailable")
	}
	if sl, ok := r.jsonl.outcomeStore.(scorecardOutcomeStore); ok {
		return sl.LoadScorecardOutcomes()
	}
	scorecardSlimRepoFallbackTotal.Add(1)
	logging.Warn("repository", "scorecard_slim_fallback",
		"layer", "dual_write_repository",
		"store_type", fmt.Sprintf("%T", r.jsonl.outcomeStore),
		"reason", "outcome store does not implement LoadScorecardOutcomes; full metadata read")
	return r.QueryAllOutcomes(ctx)
}

func (r *DualWriteRepository) QueryAllSessionScorecards(ctx context.Context) ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return r.jsonl.sessionSummaryStore.LoadAllSessionScorecards()
}

func (r *DualWriteRepository) RecordSessionOutcomes(ctx context.Context, session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	_ = r.jsonl.outcomeStore.RecordSessionOutcomes(session, outcomes)
	return nil
}

// RecordSessionSummary dual-writes a session summary to both backends (B6).
//
// Prior to B6 the JSONL-side error was swallowed (`_ =`), so a failing
// summary.json write silently produced a PG-only history. Both sides are now
// checked: a JSONL failure is logged (WARN) and counted, and the PG
// conditional write is preserved. The error contract:
//
//   - JSONL fails but PG write succeeds → nil (data is safe in PG).
//   - PG fails (while usable) → error returned (unchanged, so the caller's
//     recordSummaryWithRetry can retry).
//   - JSONL fails while PG is NOT usable → the error IS returned: nothing was
//     persisted anywhere, and returning nil would be silent total loss.
//
// Either failure bumps the reconcile-pending marker counter so monitoring can
// alert that cmd/reconcile-sessions should be run.
func (r *DualWriteRepository) RecordSessionSummary(ctx context.Context, session domain.ReplaySession, summary domain.SessionSummary) error {
	// SSoT write guard (2026-08-23): a corrupted summary must fail loudly at
	// write time instead of being persisted to one or both backends and
	// surfacing later as a performance-report data problem. Strict validation
	// (PortfolioValue > 0, legal regime, non-negative cash/counts) applies to
	// the real-time sim write path; count-only legacy rows go through
	// SaveSessionSummary (ValidateLegacy).
	if err := summary.Validate(); err != nil {
		return fmt.Errorf("record session summary: rejected corrupted summary: %w", err)
	}
	jsonlErr := r.jsonl.sessionSummaryStore.RecordSessionSummary(session, summary)
	if jsonlErr != nil {
		sessionSummaryJSONLErrorTotal.Add(1)
		sessionSummaryReconcilePendingTotal.Add(1)
		logging.Warn("repository", "session_summary_jsonl_write_failed",
			"session_id", summary.SessionID,
			"side", "jsonl",
			"error", jsonlErr)
	}
	if r.pgUsable() {
		if err := r.pg.SaveSessionSummary(ctx, summary); err != nil {
			sessionSummaryPGErrorTotal.Add(1)
			sessionSummaryReconcilePendingTotal.Add(1)
			logging.Warn("repository", "session_summary_pg_write_failed",
				"session_id", summary.SessionID,
				"side", "postgres",
				"error", err)
			return err
		}
		return nil
	}
	if jsonlErr != nil {
		// PG unavailable AND JSONL failed: the summary was persisted nowhere.
		return fmt.Errorf("record session summary: jsonl write failed and PG unavailable: %w", jsonlErr)
	}
	return nil
}

func (r *DualWriteRepository) RecordExperiment(ctx context.Context, record domain.ExperimentRecord) error {
	_ = r.jsonl.outcomeStore.RecordExperiment(record)
	return nil
}

func (r *DualWriteRepository) RecordSessionExperiment(ctx context.Context, session domain.ReplaySession, record domain.ExperimentRecord) error {
	_ = r.jsonl.outcomeStore.RecordSessionExperiment(session, record)
	return nil
}

func (r *DualWriteRepository) RecordSessionScreeningRejects(ctx context.Context, sessionID string, rejects []domain.ScreeningReject) error {
	return r.RecordScreeningRejects(ctx, sessionID, rejects)
}

func (r *DualWriteRepository) LoadSessionScreeningRejects(ctx context.Context, sessionID string) ([]domain.ScreeningReject, error) {
	return r.QueryScreeningRejectsBySession(ctx, sessionID)
}

// ============================================
// Capital Flow Operations
// ============================================

func (r *DualWriteRepository) RecordCapitalFlow(ctx context.Context, channel string, netBuy, totalBuy, totalSell float64) error {
	if r.pgUsable() {
		return r.pg.RecordCapitalFlow(ctx, channel, netBuy, totalBuy, totalSell)
	}
	return nil
}

func (r *DualWriteRepository) QueryLatestCapitalFlow(ctx context.Context, channel string) (*CapitalFlowRecord, error) {
	if r.pgUsable() {
		return r.pg.QueryLatestCapitalFlow(ctx, channel)
	}
	return nil, nil
}

func (r *DualWriteRepository) QueryCapitalFlowRange(ctx context.Context, channel string, start, end time.Time) ([]CapitalFlowRecord, error) {
	if r.pgUsable() {
		return r.pg.QueryCapitalFlowRange(ctx, channel, start, end)
	}
	return nil, nil
}

// ============================================
// Export Stats Operations
// ============================================

func (r *DualWriteRepository) SaveExportStats(ctx context.Context, year, month int, exportTotal, importTotal, tradeBalance float64) error {
	if r.pgUsable() {
		return r.pg.SaveExportStats(ctx, year, month, exportTotal, importTotal, tradeBalance)
	}
	return nil
}

func (r *DualWriteRepository) QueryLatestExportStats(ctx context.Context) (*ExportStatsRecord, error) {
	if r.pgUsable() {
		return r.pg.QueryLatestExportStats(ctx)
	}
	return nil, nil
}

func (r *DualWriteRepository) QueryExportStatsByYearMonth(ctx context.Context, year, month int) (*ExportStatsRecord, error) {
	if r.pgUsable() {
		return r.pg.QueryExportStatsByYearMonth(ctx, year, month)
	}
	return nil, nil
}

func (r *DualWriteRepository) RecordScreeningRejects(ctx context.Context, sessionID string, rejects []domain.ScreeningReject) error {
	_ = r.jsonl.screeningRejectStore.RecordSessionScreeningRejects(sessionID, rejects)
	if r.pgUsable() {
		return r.pg.RecordScreeningRejects(ctx, sessionID, rejects)
	}
	return nil
}

func (r *DualWriteRepository) QueryScreeningRejectsBySession(ctx context.Context, sessionID string) ([]domain.ScreeningReject, error) {
	if r.pgUsable() {
		rejects, err := r.pg.QueryScreeningRejectsBySession(ctx, sessionID)
		if err == nil && len(rejects) > 0 {
			return rejects, nil
		}
	}
	return r.jsonl.screeningRejectStore.LoadSessionScreeningRejects(sessionID)
}

// SaveSessionSummary is the direct (non-session) dual-write entry point used
// by backfill/migration paths. Same B6 error handling as RecordSessionSummary:
// the JSONL write error is logged and counted instead of silently dropped; the
// error is only returned when the summary was persisted nowhere (JSONL failed
// while PG is unavailable).
func (r *DualWriteRepository) SaveSessionSummary(ctx context.Context, summary domain.SessionSummary) error {
	// Backfill/migration write guard (SSoT 2026-08-23): lenient validation.
	// Legacy count-only rows (PortfolioValue=0, EndingCash=0, empty regime —
	// cmd/backfill-summaries, pre-backfill SQLite rows) are legal production
	// data that reconcile-sessions copies into PG; strict validation would
	// block the backfill. Genuinely corrupted rows (zero portfolio with cash
	// or orders) are still rejected.
	if err := summary.ValidateLegacy(); err != nil {
		return fmt.Errorf("save session summary: rejected corrupted summary: %w", err)
	}
	jsonlErr := r.jsonl.sessionSummaryStore.RecordSessionSummary(domain.ReplaySession{ID: summary.SessionID}, summary)
	if jsonlErr != nil {
		sessionSummaryJSONLErrorTotal.Add(1)
		sessionSummaryReconcilePendingTotal.Add(1)
		logging.Warn("repository", "session_summary_jsonl_write_failed",
			"session_id", summary.SessionID,
			"side", "jsonl",
			"error", jsonlErr)
	}
	if r.pgUsable() {
		if err := r.pg.SaveSessionSummary(ctx, summary); err != nil {
			sessionSummaryPGErrorTotal.Add(1)
			sessionSummaryReconcilePendingTotal.Add(1)
			logging.Warn("repository", "session_summary_pg_write_failed",
				"session_id", summary.SessionID,
				"side", "postgres",
				"error", err)
			return err
		}
		return nil
	}
	if jsonlErr != nil {
		return fmt.Errorf("save session summary: jsonl write failed and PG unavailable: %w", jsonlErr)
	}
	return nil
}

func (r *DualWriteRepository) LoadSessionSummary(ctx context.Context, sessionID string) (*domain.SessionSummary, error) {
	if r.pgUsable() {
		summary, err := r.pg.LoadSessionSummary(ctx, sessionID)
		if err == nil && summary != nil {
			return summary, nil
		}
	}
	summaries, err := r.jsonl.sessionSummaryStore.LoadSessionSummaries()
	if err != nil {
		return nil, err
	}
	for i := range summaries {
		if summaries[i].SessionID == sessionID {
			return &summaries[i], nil
		}
	}
	return nil, nil
}

// LoadAllSessionSummaries returns the union of PG and JSONL session
// summaries (B6 merge semantics).
//
// The pre-B6 behavior was PG-first with JSONL fallback only when PG was
// unavailable/empty — so a split like the audited one (PG had 07-02→07-23,
// JSONL had 07-24→08-21) silently returned just the PG half. The merged read
// now: (1) keeps the fallback path when PG is unusable/errored/empty (the
// fallbackCounter contract is unchanged), and (2) when PG is usable and
// non-empty, merges both sides by SessionID — union of session IDs, newer
// RecordedAt wins, PG wins on a timestamp tie (authoritative backend).
// Trade-off documented: a session present on both sides with equal timestamps
// but different content resolves to the PG copy; divergent sessions are
// surfaced via SessionSummaryMergeDivergenceTotal + WARN log, and
// cmd/reconcile-sessions is the durable fix. LoadSessionSummary (singular)
// intentionally stays PG-first/JSONL-fallback (chart-anchor hot path), so
// after a drift the plural report and a singular anchor may disagree until a
// reconcile is run.
func (r *DualWriteRepository) LoadAllSessionSummaries(ctx context.Context) ([]domain.SessionSummary, error) {
	jsonlSummaries, jsonlErr := r.jsonl.sessionSummaryStore.LoadSessionSummaries()

	if !r.pgUsable() {
		fallbackCounter.Add(1)
		log.Printf("[DualWrite] LoadAllSessionSummaries: PG not usable; serving JSONL (total=%d)", fallbackCounter.Load())
		if jsonlErr != nil {
			log.Printf("[DualWrite] LoadAllSessionSummaries: JSONL load failed: %v", jsonlErr)
			return nil, jsonlErr
		}
		return jsonlSummaries, nil
	}

	pgSummaries, err := r.pg.LoadAllSessionSummaries(ctx)
	if err != nil || len(pgSummaries) == 0 {
		fallbackCounter.Add(1)
		log.Printf("[DualWrite] LoadAllSessionSummaries: PG returned %d (err=%v); falling back to JSONL (total=%d)", len(pgSummaries), err, fallbackCounter.Load())
		if jsonlErr != nil {
			log.Printf("[DualWrite] LoadAllSessionSummaries: JSONL fallback failed: %v", jsonlErr)
			return nil, jsonlErr
		}
		return jsonlSummaries, nil
	}

	// PG usable and non-empty: B6 merge with the JSONL side.
	if jsonlErr != nil {
		// JSONL read failed — log and continue with whatever was loaded
		// (typically nil), so a broken JSONL store cannot take down the
		// report; the divergence counters still surface the problem.
		logging.Warn("repository", "session_summary_jsonl_load_failed",
			"error", jsonlErr)
	}
	merged, onlyJSONL, onlyPG, diverged := mergeSessionSummaries(pgSummaries, jsonlSummaries)
	sessionSummaryJSONLOnlyTotal.Add(int64(onlyJSONL))
	sessionSummaryPGOnlyTotal.Add(int64(onlyPG))
	sessionSummaryMergeDivergenceTotal.Add(int64(diverged))
	log.Printf("[DualWrite] LoadAllSessionSummaries: merged PG(%d)+JSONL(%d) -> %d summaries (onlyJSONL=%d onlyPG=%d diverged=%d)",
		len(pgSummaries), len(jsonlSummaries), len(merged), onlyJSONL, onlyPG, diverged)
	if onlyJSONL > 0 || onlyPG > 0 || diverged > 0 {
		logging.Warn("repository", "session_summary_dual_write_diverged",
			"only_jsonl", onlyJSONL,
			"only_pg", onlyPG,
			"diverged", diverged,
			"hint", "run go run ./cmd/reconcile-sessions to backfill one-sided gaps")
	}
	return merged, nil
}

// mergeSessionSummaries merges two session-summary sets by SessionID (B6).
// Union of session IDs; when a session exists on both sides the newer
// RecordedAt wins and a PG entry wins on a timestamp tie. Returns the merged
// list (newest first, matching the PG ORDER BY time DESC contract), plus the
// counts of sessions present only in JSONL / only in PG / present on both but
// divergent.
func mergeSessionSummaries(pg, jsonl []domain.SessionSummary) (merged []domain.SessionSummary, onlyJSONL, onlyPG, diverged int) {
	merged = make([]domain.SessionSummary, 0, len(pg)+len(jsonl))
	pgByID := make(map[string]domain.SessionSummary, len(pg))
	jsonlByID := make(map[string]domain.SessionSummary, len(jsonl))
	for _, s := range pg {
		pgByID[s.SessionID] = s
	}
	for _, s := range jsonl {
		jsonlByID[s.SessionID] = s
	}

	// PG entries first (authoritative tiebreak + newest-first base order).
	merged = append(merged, pg...)
	// JSONL-only sessions appended after the PG block.
	for _, s := range jsonl {
		if _, ok := pgByID[s.SessionID]; ok {
			continue
		}
		merged = append(merged, s)
		onlyJSONL++
	}
	for _, s := range pg {
		if _, ok := jsonlByID[s.SessionID]; !ok {
			onlyPG++
		}
	}
	// Resolve conflicts: newer wins; tie keeps the PG entry already appended.
	for i := range merged {
		pgS, inPG := pgByID[merged[i].SessionID]
		jsonlS, inJSONL := jsonlByID[merged[i].SessionID]
		if !inPG || !inJSONL {
			continue
		}
		if jsonlS.RecordedAt.After(pgS.RecordedAt) {
			merged[i] = jsonlS // JSONL is newer — replace the PG copy.
			diverged++
		} else if sessionSummariesConflict(pgS, jsonlS) {
			diverged++ // tie or PG newer but content drifted — PG copy kept.
		}
	}

	// Newest first (PG contract). Stable so equal timestamps keep the
	// PG-before-JSONL insertion order.
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].RecordedAt.After(merged[j].RecordedAt)
	})
	return merged, onlyJSONL, onlyPG, diverged
}

// sessionSummariesConflict reports whether two summaries for the same session
// carry different content. RecordedAt is compared with time.Equal (handles
// location/monotonic differences); the remaining fields with DeepEqual after
// zeroing RecordedAt so the timestamp is not double-counted.
func sessionSummariesConflict(a, b domain.SessionSummary) bool {
	if !a.RecordedAt.Equal(b.RecordedAt) {
		return true
	}
	a.RecordedAt, b.RecordedAt = time.Time{}, time.Time{}
	return !reflect.DeepEqual(a, b)
}

func (r *DualWriteRepository) RecordHumanIntervention(ctx context.Context, intervention domain.HumanIntervention) error {
	_ = r.jsonl.humanInterventionStore.RecordHumanIntervention(intervention)
	if r.pgUsable() {
		return r.pg.RecordHumanIntervention(ctx, intervention)
	}
	return nil
}

func (r *DualWriteRepository) LoadHumanInterventions(ctx context.Context) ([]domain.HumanIntervention, error) {
	if r.pgUsable() {
		interventions, err := r.pg.LoadHumanInterventions(ctx)
		if err == nil && len(interventions) > 0 {
			return interventions, nil
		}
	}
	return r.jsonl.humanInterventionStore.LoadHumanInterventions()
}
