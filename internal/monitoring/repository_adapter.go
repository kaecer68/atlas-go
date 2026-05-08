package monitoring

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
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

// OutcomeStoreAdapter adapts ledger.OutcomeStore to repository.OutcomeStore interface.
type OutcomeStoreAdapter struct {
	store ledger.OutcomeStore
}

// NewOutcomeStoreAdapter creates a new adapter.
func NewOutcomeStoreAdapter(store ledger.OutcomeStore) *OutcomeStoreAdapter {
	return &OutcomeStoreAdapter{store: store}
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

func (a *OutcomeStoreAdapter) RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	return a.store.RecordSessionOutcomes(session, outcomes)
}

func (a *OutcomeStoreAdapter) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	return a.store.RecordSessionSummary(session, summary)
}

func (a *OutcomeStoreAdapter) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return a.store.LoadSessionSummaries()
}

func (a *OutcomeStoreAdapter) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return a.store.LoadAllSessionScorecards()
}

func (a *OutcomeStoreAdapter) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	return a.store.RecordSessionScreeningRejects(sessionID, rejects)
}

func (a *OutcomeStoreAdapter) LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error) {
	return a.store.LoadSessionScreeningRejects(sessionID)
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
}

func NewDualWriteOutcomeStoreAdapter(repo *repository.DualWriteRepository) *DualWriteOutcomeStoreAdapter {
	return &DualWriteOutcomeStoreAdapter{repo: repo}
}

func (a *DualWriteOutcomeStoreAdapter) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	return a.repo.RecordOutcomes(context.Background(), outcomes)
}

func (a *DualWriteOutcomeStoreAdapter) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	return a.repo.QueryOutcomesBySession(context.Background(), sessionID)
}

func (a *DualWriteOutcomeStoreAdapter) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return a.repo.QueryAllOutcomes(context.Background())
}

func (a *DualWriteOutcomeStoreAdapter) RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	return a.repo.RecordSessionOutcomes(context.Background(), session, outcomes)
}

func (a *DualWriteOutcomeStoreAdapter) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	return a.repo.RecordSessionSummary(context.Background(), session, summary)
}

func (a *DualWriteOutcomeStoreAdapter) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return a.repo.LoadAllSessionSummaries(context.Background())
}

func (a *DualWriteOutcomeStoreAdapter) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return a.repo.QueryAllSessionScorecards(context.Background())
}

func (a *DualWriteOutcomeStoreAdapter) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	return a.repo.RecordSessionScreeningRejects(context.Background(), sessionID, rejects)
}

func (a *DualWriteOutcomeStoreAdapter) LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error) {
	return a.repo.LoadSessionScreeningRejects(context.Background(), sessionID)
}

func (a *DualWriteOutcomeStoreAdapter) RecordExperiment(record domain.ExperimentRecord) error {
	return a.repo.RecordExperiment(context.Background(), record)
}

func (a *DualWriteOutcomeStoreAdapter) RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error {
	return a.repo.RecordSessionExperiment(context.Background(), session, record)
}

func (a *DualWriteOutcomeStoreAdapter) RecordHumanIntervention(intervention domain.HumanIntervention) error {
	return a.repo.RecordHumanIntervention(context.Background(), intervention)
}

func (a *DualWriteOutcomeStoreAdapter) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	return a.repo.LoadHumanInterventions(context.Background())
}
