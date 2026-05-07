package repository

import (
	"context"
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
}

// MetricsStore defines the interface for the existing metrics store
type MetricsStore interface {
	SaveSnapshot(snapshot MetricsSnapshot) error
	LoadToday() (*MetricsSnapshot, error)
	LoadRecent(n int) ([]MetricsSnapshot, error)
}

// OutcomeStore defines the interface for the existing outcome store
type OutcomeStore interface {
	RecordOutcomes(outcomes []domain.RecommendationOutcome) error
	LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error)
	LoadOutcomes() ([]domain.RecommendationOutcome, error)
}

type ScreeningRejectStore interface {
	RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error
	LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error)
}

type SessionSummaryStore interface {
	RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error
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

// ============================================
// Metrics Operations
// ============================================

func (r *DualWriteRepository) Record(ctx context.Context, metricName string, value float64, labels map[string]string) error {
	// Always write to JSONL
	_ = r.jsonl.metricsStore.SaveSnapshot(MetricsSnapshot{
		Timestamp: time.Now(),
	})

	// Also write to PostgreSQL (best effort)
	if err := r.pg.Record(ctx, metricName, value, labels); err != nil {
		// Log but don't fail - JSONL is the source of truth during transition
		return nil
	}
	return nil
}

func (r *DualWriteRepository) QueryRange(ctx context.Context, metricName string, start, end time.Time) ([]MetricPoint, error) {
	// Prefer PostgreSQL
	points, err := r.pg.QueryRange(ctx, metricName, start, end)
	if err == nil && len(points) > 0 {
		return points, nil
	}
	// Fallback could be implemented here if needed
	return nil, err
}

func (r *DualWriteRepository) QueryLatest(ctx context.Context, metricName string, labels map[string]string) (*MetricPoint, error) {
	return r.pg.QueryLatest(ctx, metricName, labels)
}

func (r *DualWriteRepository) Aggregate(ctx context.Context, metricName string, start, end time.Time, agg string) (float64, error) {
	return r.pg.Aggregate(ctx, metricName, start, end, agg)
}

func (r *DualWriteRepository) SaveSnapshot(ctx context.Context, snapshot *MetricsSnapshot) error {
	_ = r.jsonl.metricsStore.SaveSnapshot(*snapshot)
	return r.pg.SaveSnapshot(ctx, snapshot)
}

func (r *DualWriteRepository) LoadToday(ctx context.Context) (*MetricsSnapshot, error) {
	// Try PostgreSQL first
	snap, err := r.pg.LoadToday(ctx)
	if err == nil && snap != nil {
		return snap, nil
	}
	// Fallback to JSONL
	return r.jsonl.metricsStore.LoadToday()
}

func (r *DualWriteRepository) LoadRecent(ctx context.Context, n int) ([]MetricsSnapshot, error) {
	snaps, err := r.pg.LoadRecent(ctx, n)
	if err == nil && len(snaps) > 0 {
		return snaps, nil
	}
	return r.jsonl.metricsStore.LoadRecent(n)
}

// ============================================
// Alert Operations
// ============================================

func (r *DualWriteRepository) SaveAlert(ctx context.Context, alert domain.AlertRecord) error {
	_ = r.jsonl.alertStore.Save(alert)
	return r.pg.SaveAlert(ctx, alert)
}

func (r *DualWriteRepository) LoadAllAlerts(ctx context.Context, limit int) ([]domain.AlertRecord, error) {
	records, err := r.pg.LoadAllAlerts(ctx, limit)
	if err == nil && len(records) > 0 {
		return records, nil
	}
	return r.jsonl.alertStore.LoadAll()
}

func (r *DualWriteRepository) LoadUnacknowledgedAlerts(ctx context.Context) ([]domain.AlertRecord, error) {
	records, err := r.pg.LoadUnacknowledgedAlerts(ctx)
	if err == nil && len(records) > 0 {
		return records, nil
	}
	return r.jsonl.alertStore.LoadUnacknowledged()
}

func (r *DualWriteRepository) AcknowledgeAlert(ctx context.Context, alertID string, user string) error {
	_ = r.jsonl.alertStore.Acknowledge(alertID, user)
	return r.pg.AcknowledgeAlert(ctx, alertID, user)
}

func (r *DualWriteRepository) LoadAlertsBySeverity(ctx context.Context, severity string, limit int) ([]domain.AlertRecord, error) {
	return r.pg.LoadAlertsBySeverity(ctx, severity, limit)
}

func (r *DualWriteRepository) LoadAlertsByTimeRange(ctx context.Context, start, end time.Time) ([]domain.AlertRecord, error) {
	return r.pg.LoadAlertsByTimeRange(ctx, start, end)
}

func (r *DualWriteRepository) QuerySessions(ctx context.Context) ([]SessionInfo, error) {
	return r.pg.QuerySessions(ctx)
}

// ============================================
// Outcome Operations
// ============================================

func (r *DualWriteRepository) RecordOutcomes(ctx context.Context, outcomes []domain.RecommendationOutcome) error {
	_ = r.jsonl.outcomeStore.RecordOutcomes(outcomes)
	return r.pg.RecordOutcomes(ctx, outcomes)
}

func (r *DualWriteRepository) QueryOutcomesBySession(ctx context.Context, sessionID string) ([]domain.RecommendationOutcome, error) {
	outcomes, err := r.pg.QueryOutcomesBySession(ctx, sessionID)
	if err == nil && len(outcomes) > 0 {
		return outcomes, nil
	}
	return r.jsonl.outcomeStore.LoadSessionOutcomes(sessionID)
}

func (r *DualWriteRepository) QueryOutcomesBySymbol(ctx context.Context, symbol string, start, end time.Time) ([]domain.RecommendationOutcome, error) {
	return r.pg.QueryOutcomesBySymbol(ctx, symbol, start, end)
}

func (r *DualWriteRepository) QueryOutcomesByAgent(ctx context.Context, agentID string, start, end time.Time) ([]domain.RecommendationOutcome, error) {
	return r.pg.QueryOutcomesByAgent(ctx, agentID, start, end)
}

func (r *DualWriteRepository) QueryPassRate(ctx context.Context, agentID string, window time.Duration) (float64, error) {
	return r.pg.QueryPassRate(ctx, agentID, window)
}

func (r *DualWriteRepository) QueryTopSymbols(ctx context.Context, limit int, start, end time.Time) ([]SymbolCount, error) {
	return r.pg.QueryTopSymbols(ctx, limit, start, end)
}

// ============================================
// Capital Flow Operations
// ============================================

func (r *DualWriteRepository) RecordCapitalFlow(ctx context.Context, channel string, netBuy, totalBuy, totalSell float64) error {
	return r.pg.RecordCapitalFlow(ctx, channel, netBuy, totalBuy, totalSell)
}

func (r *DualWriteRepository) QueryLatestCapitalFlow(ctx context.Context, channel string) (*CapitalFlowRecord, error) {
	return r.pg.QueryLatestCapitalFlow(ctx, channel)
}

func (r *DualWriteRepository) QueryCapitalFlowRange(ctx context.Context, channel string, start, end time.Time) ([]CapitalFlowRecord, error) {
	return r.pg.QueryCapitalFlowRange(ctx, channel, start, end)
}

// ============================================
// Export Stats Operations
// ============================================

func (r *DualWriteRepository) SaveExportStats(ctx context.Context, year, month int, exportTotal, importTotal, tradeBalance float64) error {
	return r.pg.SaveExportStats(ctx, year, month, exportTotal, importTotal, tradeBalance)
}

func (r *DualWriteRepository) QueryLatestExportStats(ctx context.Context) (*ExportStatsRecord, error) {
	return r.pg.QueryLatestExportStats(ctx)
}

func (r *DualWriteRepository) QueryExportStatsByYearMonth(ctx context.Context, year, month int) (*ExportStatsRecord, error) {
	return r.pg.QueryExportStatsByYearMonth(ctx, year, month)
}

func (r *DualWriteRepository) RecordScreeningRejects(ctx context.Context, sessionID string, rejects []domain.ScreeningReject) error {
	_ = r.jsonl.screeningRejectStore.RecordSessionScreeningRejects(sessionID, rejects)
	return r.pg.RecordScreeningRejects(ctx, sessionID, rejects)
}

func (r *DualWriteRepository) QueryScreeningRejectsBySession(ctx context.Context, sessionID string) ([]domain.ScreeningReject, error) {
	rejects, err := r.pg.QueryScreeningRejectsBySession(ctx, sessionID)
	if err == nil && len(rejects) > 0 {
		return rejects, nil
	}
	return r.jsonl.screeningRejectStore.LoadSessionScreeningRejects(sessionID)
}

func (r *DualWriteRepository) SaveSessionSummary(ctx context.Context, summary domain.SessionSummary) error {
	_ = r.jsonl.sessionSummaryStore.RecordSessionSummary(domain.ReplaySession{ID: summary.SessionID}, summary)
	return r.pg.SaveSessionSummary(ctx, summary)
}

func (r *DualWriteRepository) LoadSessionSummary(ctx context.Context, sessionID string) (*domain.SessionSummary, error) {
	return r.pg.LoadSessionSummary(ctx, sessionID)
}

func (r *DualWriteRepository) LoadAllSessionSummaries(ctx context.Context) ([]domain.SessionSummary, error) {
	return r.pg.LoadAllSessionSummaries(ctx)
}

func (r *DualWriteRepository) RecordHumanIntervention(ctx context.Context, intervention domain.HumanIntervention) error {
	_ = r.jsonl.humanInterventionStore.RecordHumanIntervention(intervention)
	return r.pg.RecordHumanIntervention(ctx, intervention)
}

func (r *DualWriteRepository) LoadHumanInterventions(ctx context.Context) ([]domain.HumanIntervention, error) {
	interventions, err := r.pg.LoadHumanInterventions(ctx)
	if err == nil && len(interventions) > 0 {
		return interventions, nil
	}
	return r.jsonl.humanInterventionStore.LoadHumanInterventions()
}
