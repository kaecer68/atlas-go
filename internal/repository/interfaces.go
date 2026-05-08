package repository

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// MetricsRepository defines the interface for metrics persistence.
type MetricsRepository interface {
	Record(ctx context.Context, metricName string, value float64, labels map[string]string) error
	QueryRange(ctx context.Context, metricName string, start, end time.Time) ([]MetricPoint, error)
	QueryLatest(ctx context.Context, metricName string, labels map[string]string) (*MetricPoint, error)
	Aggregate(ctx context.Context, metricName string, start, end time.Time, agg string) (float64, error)
}

// MetricsSnapshotReader defines methods for reading metrics snapshots.
type MetricsSnapshotReader interface {
	LoadToday(ctx context.Context) (*MetricsSnapshot, error)
	LoadRecent(ctx context.Context, n int) ([]MetricsSnapshot, error)
}

// MetricsSnapshotWriter defines methods for writing metrics snapshots.
type MetricsSnapshotWriter interface {
	SaveSnapshot(ctx context.Context, snapshot *MetricsSnapshot) error
}

// MetricsSnapshot represents a point-in-time snapshot of all metrics.
type MetricsSnapshot struct {
	ScreeningTotal     int64            `json:"screening_total"`
	ScreeningPassed    int64            `json:"screening_passed"`
	ScreeningRate      float64          `json:"screening_rate"`
	AlertsTriggered    int64            `json:"alerts_triggered"`
	AlertsAcknowledged int64            `json:"alerts_acknowledged"`
	AlertsByType       map[string]int64 `json:"alerts_by_type"`
	Timestamp          time.Time        `json:"timestamp"`
}

// MetricPoint represents a single time-series data point.
type MetricPoint struct {
	Time      time.Time      `json:"time"`
	Name      string         `json:"name"`
	Value     float64        `json:"value"`
	AgentID   string         `json:"agent_id,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Symbol    string         `json:"symbol,omitempty"`
	Regime    string         `json:"regime,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// AlertRepository defines the interface for alert persistence.
type AlertRepository interface {
	Save(ctx context.Context, alert domain.AlertRecord) error
	LoadAll(ctx context.Context, limit int) ([]domain.AlertRecord, error)
	LoadUnacknowledged(ctx context.Context) ([]domain.AlertRecord, error)
	Acknowledge(ctx context.Context, alertID string, user string) error
	LoadBySeverity(ctx context.Context, severity string, limit int) ([]domain.AlertRecord, error)
	LoadByTimeRange(ctx context.Context, start, end time.Time) ([]domain.AlertRecord, error)
}

// OutcomeRepository defines the interface for recommendation outcomes persistence.
type OutcomeRepository interface {
	RecordOutcomes(ctx context.Context, outcomes []domain.RecommendationOutcome) error
	QueryOutcomesBySession(ctx context.Context, sessionID string) ([]domain.RecommendationOutcome, error)
	QueryOutcomesBySymbol(ctx context.Context, symbol string, start, end time.Time) ([]domain.RecommendationOutcome, error)
	QueryOutcomesByAgent(ctx context.Context, agentID string, start, end time.Time) ([]domain.RecommendationOutcome, error)
	QueryPassRate(ctx context.Context, agentID string, window time.Duration) (float64, error)
	QueryTopSymbols(ctx context.Context, limit int, start, end time.Time) ([]SymbolCount, error)
}

// SymbolCount represents a symbol and its occurrence count.
type SymbolCount struct {
	Symbol string `json:"symbol"`
	Count  int    `json:"count"`
}

// CapitalFlowRepository defines the interface for capital flow data persistence.
type CapitalFlowRepository interface {
	Record(ctx context.Context, channel string, netBuy, totalBuy, totalSell float64) error
	QueryLatest(ctx context.Context, channel string) (*CapitalFlowRecord, error)
	QueryRange(ctx context.Context, channel string, start, end time.Time) ([]CapitalFlowRecord, error)
}

// CapitalFlowRecord represents a single capital flow data point.
type CapitalFlowRecord struct {
	Time      time.Time `json:"time"`
	Channel   string    `json:"channel"`
	NetBuy    float64   `json:"net_buy"`
	TotalBuy  float64   `json:"total_buy"`
	TotalSell float64   `json:"total_sell"`
}

// ExportStatsRepository defines the interface for export statistics persistence.
type ExportStatsRepository interface {
	Save(ctx context.Context, year, month int, exportTotal, importTotal, tradeBalance float64) error
	QueryLatest(ctx context.Context) (*ExportStatsRecord, error)
	QueryByYearMonth(ctx context.Context, year, month int) (*ExportStatsRecord, error)
}

// ExportStatsRecord represents export statistics for a specific period.
type ExportStatsRecord struct {
	Time         time.Time `json:"time"`
	Year         int       `json:"year"`
	Month        int       `json:"month"`
	ExportTotal  float64   `json:"export_total"`
	ImportTotal  float64   `json:"import_total"`
	TradeBalance float64   `json:"trade_balance"`
}

type ScreeningRejectRepository interface {
	RecordScreeningRejects(ctx context.Context, sessionID string, rejects []domain.ScreeningReject) error
	QueryScreeningRejectsBySession(ctx context.Context, sessionID string) ([]domain.ScreeningReject, error)
}

type SessionSummaryRepository interface {
	SaveSessionSummary(ctx context.Context, summary domain.SessionSummary) error
	LoadSessionSummary(ctx context.Context, sessionID string) (*domain.SessionSummary, error)
	LoadAllSessionSummaries(ctx context.Context) ([]domain.SessionSummary, error)
}

type HumanInterventionRepository interface {
	RecordHumanIntervention(ctx context.Context, intervention domain.HumanIntervention) error
	LoadHumanInterventions(ctx context.Context) ([]domain.HumanIntervention, error)
}

// Repository combines all repository interfaces for convenience.
type Repository struct {
	Metrics            MetricsRepository
	Alerts             AlertRepository
	Outcomes           OutcomeRepository
	CapitalFlow        CapitalFlowRepository
	ExportStats        ExportStatsRepository
	ScreeningRejects   ScreeningRejectRepository
	SessionSummaries   SessionSummaryRepository
	HumanInterventions HumanInterventionRepository
}
