package monitoring

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/repository"
)

// MetricsStoreAdapter adapts MetricsStore to repository.MetricsStore interface.
type MetricsStoreAdapter struct {
	store *MetricsStore
}

// NewMetricsStoreAdapter creates a new adapter.
func NewMetricsStoreAdapter(store *MetricsStore) *MetricsStoreAdapter {
	return &MetricsStoreAdapter{store: store}
}

func (a *MetricsStoreAdapter) SaveSnapshot(snapshot repository.MetricsSnapshot) error {
	return a.store.SaveSnapshot(MetricsSnapshot{
		ScreeningTotal:     snapshot.ScreeningTotal,
		ScreeningPassed:    snapshot.ScreeningPassed,
		ScreeningRate:      snapshot.ScreeningRate,
		AlertsTriggered:    snapshot.AlertsTriggered,
		AlertsAcknowledged: snapshot.AlertsAcknowledged,
		AlertsByType:       snapshot.AlertsByType,
		Timestamp:          snapshot.Timestamp,
	})
}

func (a *MetricsStoreAdapter) LoadToday() (*repository.MetricsSnapshot, error) {
	snap, err := a.store.LoadToday()
	if err != nil || snap == nil {
		return nil, err
	}
	return &repository.MetricsSnapshot{
		ScreeningTotal:     snap.ScreeningTotal,
		ScreeningPassed:    snap.ScreeningPassed,
		ScreeningRate:      snap.ScreeningRate,
		AlertsTriggered:    snap.AlertsTriggered,
		AlertsAcknowledged: snap.AlertsAcknowledged,
		AlertsByType:       snap.AlertsByType,
		Timestamp:          snap.Timestamp,
	}, nil
}

func (a *MetricsStoreAdapter) LoadRecent(n int) ([]repository.MetricsSnapshot, error) {
	snaps, err := a.store.LoadRecent(n)
	if err != nil {
		return nil, err
	}
	result := make([]repository.MetricsSnapshot, len(snaps))
	for i, snap := range snaps {
		result[i] = repository.MetricsSnapshot{
			ScreeningTotal:     snap.ScreeningTotal,
			ScreeningPassed:    snap.ScreeningPassed,
			ScreeningRate:      snap.ScreeningRate,
			AlertsTriggered:    snap.AlertsTriggered,
			AlertsAcknowledged: snap.AlertsAcknowledged,
			AlertsByType:       snap.AlertsByType,
			Timestamp:          snap.Timestamp,
		}
	}
	return result, nil
}

// AlertStoreAdapter adapts AlertStore to repository.AlertStore interface.
type AlertStoreAdapter struct {
	store *AlertStore
}

// NewAlertStoreAdapter creates a new adapter.
func NewAlertStoreAdapter(store *AlertStore) *AlertStoreAdapter {
	return &AlertStoreAdapter{store: store}
}

func (a *AlertStoreAdapter) Save(alert domain.AlertRecord) error {
	return a.store.Save(alert)
}

func (a *AlertStoreAdapter) LoadAll() ([]domain.AlertRecord, error) {
	return a.store.LoadAll()
}

func (a *AlertStoreAdapter) LoadUnacknowledged() ([]domain.AlertRecord, error) {
	return a.store.LoadUnacknowledged()
}

func (a *AlertStoreAdapter) Acknowledge(alertID string, user string) error {
	return a.store.Acknowledge(alertID, user)
}

func (a *AlertStoreAdapter) FindByDedupKey(dedupKey string) (*domain.AlertRecord, error) {
	return a.store.FindByDedupKey(dedupKey)
}

func (a *AlertStoreAdapter) Update(id string, fn func(*domain.AlertRecord)) error {
	return a.store.Update(id, fn)
}

// OutcomeStoreAdapter adapts ledger.OutcomeStore to repository.OutcomeStore interface.
type OutcomeStoreAdapter struct {
	store ledger.OutcomeStore
}

// NewOutcomeStoreAdapter creates a new adapter.
func NewOutcomeStoreAdapter(store ledger.OutcomeStore) *OutcomeStoreAdapter {
	return &OutcomeStoreAdapter{store: store}
}

// scorecardSlimAdapterFallbackTotal counts observatory slim-projection
// fallbacks at the OutcomeStoreAdapter layer (#1780 Phase 1, B1): when the
// wrapped ledger store lacks the optional LoadScorecardOutcomes method, the
// adapter silently falling back to LoadOutcomesFromSessions would re-enable
// the full-metadata ~1.9GB load the slim projection eliminates, with no
// signal. A non-zero delta since deploy means the slim path is NOT active and
// the OOM mitigation is not engaged. Mirrors the DualWriteFallbackTotal
// observability pattern.
var scorecardSlimAdapterFallbackTotal atomic.Int64

// ScorecardSlimAdapterFallbackTotal returns the total OutcomeStoreAdapter
// slim-projection fallbacks. Exposed for monitoring/alerting consumption.
func ScorecardSlimAdapterFallbackTotal() int64 {
	return scorecardSlimAdapterFallbackTotal.Load()
}

func (a *OutcomeStoreAdapter) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	return a.store.RecordOutcomes(outcomes)
}

func (a *OutcomeStoreAdapter) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	return a.store.LoadSessionOutcomes(sessionID)
}

func (a *OutcomeStoreAdapter) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return a.store.LoadOutcomes()
}

func (a *OutcomeStoreAdapter) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	return a.store.LoadOutcomesFromSessions()
}

// LoadScorecardOutcomes serves the observatory scorecard slim projection (8
// scalar fields from JSONB, #1780 Phase 1) when the wrapped ledger store
// implements the optional ledger.ScorecardOutcomeStore interface. Stores that
// lack the method (jsonl/sqlite backends, existing mocks) fall back to the
// full LoadOutcomesFromSessions read — the pre-#1780 behavior — with a warn
// log and counter so a silently inactive slim path is observable (B1).
func (a *OutcomeStoreAdapter) LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error) {
	if sl, ok := a.store.(ledger.ScorecardOutcomeStore); ok {
		return sl.LoadScorecardOutcomes()
	}
	scorecardSlimAdapterFallbackTotal.Add(1)
	logging.Warn("monitoring", "scorecard_slim_fallback",
		"layer", "outcome_store_adapter",
		"store_type", fmt.Sprintf("%T", a.store),
		"reason", "store does not implement ledger.ScorecardOutcomeStore; full metadata read")
	return a.store.LoadOutcomesFromSessions()
}

func (a *OutcomeStoreAdapter) RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	return a.store.RecordSessionOutcomes(session, outcomes)
}

func (a *OutcomeStoreAdapter) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	return a.store.RecordSessionScreeningRejects(sessionID, rejects)
}

func (a *OutcomeStoreAdapter) LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error) {
	return a.store.LoadSessionScreeningRejects(sessionID)
}

func (a *OutcomeStoreAdapter) RecordSessionTrades(sessionID string, trades []domain.TradeRecord) error {
	return a.store.RecordSessionTrades(sessionID, trades)
}

func (a *OutcomeStoreAdapter) LoadSessionTrades(sessionID string) ([]domain.TradeRecord, error) {
	return a.store.LoadSessionTrades(sessionID)
}

func (a *OutcomeStoreAdapter) LoadAllSessionTrades() ([]domain.TradeRecord, error) {
	return a.store.LoadAllSessionTrades()
}

func (a *OutcomeStoreAdapter) RecordExperiment(record domain.ExperimentRecord) error {
	return a.store.RecordExperiment(record)
}

func (a *OutcomeStoreAdapter) RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error {
	return a.store.RecordSessionExperiment(session, record)
}

func (a *OutcomeStoreAdapter) RecordHumanIntervention(intervention domain.HumanIntervention) error {
	return a.store.RecordHumanIntervention(intervention)
}

func (a *OutcomeStoreAdapter) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	return a.store.LoadHumanInterventions()
}

type DualWriteOutcomeStoreAdapter struct {
	repo *repository.DualWriteRepository
	ctx  context.Context
}

func NewDualWriteOutcomeStoreAdapter(repo *repository.DualWriteRepository) *DualWriteOutcomeStoreAdapter {
	return &DualWriteOutcomeStoreAdapter{repo: repo, ctx: context.Background()}
}

func (a *DualWriteOutcomeStoreAdapter) SetContext(ctx context.Context) {
	if ctx != nil {
		a.ctx = ctx
	}
}

func (a *DualWriteOutcomeStoreAdapter) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	return a.repo.RecordOutcomes(a.ctx, outcomes)
}

func (a *DualWriteOutcomeStoreAdapter) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	return a.repo.QueryOutcomesBySession(a.ctx, sessionID)
}

func (a *DualWriteOutcomeStoreAdapter) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return a.repo.QueryAllOutcomes(a.ctx)
}

func (a *DualWriteOutcomeStoreAdapter) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	return a.repo.QueryAllOutcomes(a.ctx)
}

func (a *DualWriteOutcomeStoreAdapter) LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error) {
	return a.repo.QueryScorecardOutcomes(a.ctx)
}

func (a *DualWriteOutcomeStoreAdapter) RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	return a.repo.RecordSessionOutcomes(a.ctx, session, outcomes)
}

func (a *DualWriteOutcomeStoreAdapter) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	return a.repo.RecordSessionSummary(a.ctx, session, summary)
}

func (a *DualWriteOutcomeStoreAdapter) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return a.repo.LoadAllSessionSummaries(a.ctx)
}

func (a *DualWriteOutcomeStoreAdapter) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return a.repo.QueryAllSessionScorecards(a.ctx)
}

func (a *DualWriteOutcomeStoreAdapter) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	return a.repo.RecordSessionScreeningRejects(a.ctx, sessionID, rejects)
}

func (a *DualWriteOutcomeStoreAdapter) LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error) {
	return a.repo.LoadSessionScreeningRejects(a.ctx, sessionID)
}

func (a *DualWriteOutcomeStoreAdapter) RecordSessionTrades(sessionID string, trades []domain.TradeRecord) error {
	return fmt.Errorf("record session trades: not supported by dual-write adapter")
}

func (a *DualWriteOutcomeStoreAdapter) LoadSessionTrades(sessionID string) ([]domain.TradeRecord, error) {
	return nil, fmt.Errorf("load session trades: not supported by dual-write adapter")
}

func (a *DualWriteOutcomeStoreAdapter) LoadAllSessionTrades() ([]domain.TradeRecord, error) {
	return nil, fmt.Errorf("load all session trades: not supported by dual-write adapter")
}

func (a *DualWriteOutcomeStoreAdapter) RecordExperiment(record domain.ExperimentRecord) error {
	return a.repo.RecordExperiment(a.ctx, record)
}

func (a *DualWriteOutcomeStoreAdapter) RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error {
	return a.repo.RecordSessionExperiment(a.ctx, session, record)
}

func (a *DualWriteOutcomeStoreAdapter) RecordHumanIntervention(intervention domain.HumanIntervention) error {
	return a.repo.RecordHumanIntervention(a.ctx, intervention)
}

func (a *DualWriteOutcomeStoreAdapter) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	return a.repo.LoadHumanInterventions(a.ctx)
}

type SessionStoreAdapter struct {
	store ledger.SessionStore
}

func NewSessionStoreAdapter(store ledger.SessionStore) *SessionStoreAdapter {
	return &SessionStoreAdapter{store: store}
}

func (a *SessionStoreAdapter) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	return a.store.RecordSessionSummary(session, summary)
}

func (a *SessionStoreAdapter) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return a.store.LoadSessionSummaries()
}

func (a *SessionStoreAdapter) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return a.store.LoadAllSessionScorecards()
}

type DualWriteSessionStoreAdapter struct {
	repo *repository.DualWriteRepository
	ctx  context.Context
}

func NewDualWriteSessionStoreAdapter(repo *repository.DualWriteRepository) *DualWriteSessionStoreAdapter {
	return &DualWriteSessionStoreAdapter{repo: repo, ctx: context.Background()}
}

func (a *DualWriteSessionStoreAdapter) SetContext(ctx context.Context) {
	if ctx != nil {
		a.ctx = ctx
	}
}

func (a *DualWriteSessionStoreAdapter) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	return a.repo.RecordSessionSummary(a.ctx, session, summary)
}

func (a *DualWriteSessionStoreAdapter) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return a.repo.LoadAllSessionSummaries(a.ctx)
}

func (a *DualWriteSessionStoreAdapter) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return a.repo.QueryAllSessionScorecards(a.ctx)
}
