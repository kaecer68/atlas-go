package repository

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
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
	if r.pg == nil || r.pg.pool == nil {
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

func (r *DualWriteRepository) QueryAllSessionScorecards(ctx context.Context) ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return r.jsonl.sessionSummaryStore.LoadAllSessionScorecards()
}

func (r *DualWriteRepository) RecordSessionOutcomes(ctx context.Context, session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	_ = r.jsonl.outcomeStore.RecordSessionOutcomes(session, outcomes)
	return nil
}

func (r *DualWriteRepository) RecordSessionSummary(ctx context.Context, session domain.ReplaySession, summary domain.SessionSummary) error {
	_ = r.jsonl.sessionSummaryStore.RecordSessionSummary(session, summary)
	if r.pgUsable() {
		return r.pg.SaveSessionSummary(ctx, summary)
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

func (r *DualWriteRepository) SaveSessionSummary(ctx context.Context, summary domain.SessionSummary) error {
	_ = r.jsonl.sessionSummaryStore.RecordSessionSummary(domain.ReplaySession{ID: summary.SessionID}, summary)
	if r.pgUsable() {
		return r.pg.SaveSessionSummary(ctx, summary)
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

func (r *DualWriteRepository) LoadAllSessionSummaries(ctx context.Context) ([]domain.SessionSummary, error) {
	if r.pgUsable() {
		summaries, err := r.pg.LoadAllSessionSummaries(ctx)
		if err == nil && len(summaries) > 0 {
			log.Printf("[DualWrite] LoadAllSessionSummaries: PG served %d summaries", len(summaries))
			return summaries, nil
		}
		fallbackCounter.Add(1)
		log.Printf("[DualWrite] LoadAllSessionSummaries: PG returned %d (err=%v); falling back to JSONL (total=%d)", len(summaries), err, fallbackCounter.Load())
	} else {
		fallbackCounter.Add(1)
		log.Printf("[DualWrite] LoadAllSessionSummaries: PG not usable; falling back to JSONL (total=%d)", fallbackCounter.Load())
	}
	jsonlSummaries, err := r.jsonl.sessionSummaryStore.LoadSessionSummaries()
	if err != nil {
		log.Printf("[DualWrite] LoadAllSessionSummaries: JSONL fallback failed: %v", err)
		return nil, err
	}
	log.Printf("[DualWrite] LoadAllSessionSummaries: JSONL served %d summaries", len(jsonlSummaries))
	return jsonlSummaries, nil
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
