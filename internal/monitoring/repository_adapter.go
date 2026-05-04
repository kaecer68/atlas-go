package monitoring

import (
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

// OutcomeStoreAdapter adapts ledger.Store to repository.OutcomeStore interface.
type OutcomeStoreAdapter struct {
	store *ledger.Store
}

// NewOutcomeStoreAdapter creates a new adapter.
func NewOutcomeStoreAdapter(store *ledger.Store) *OutcomeStoreAdapter {
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
