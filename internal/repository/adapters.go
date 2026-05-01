package repository

import (
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// MonitoringMetricsStoreAdapter adapts monitoring.MetricsStore to repository.MetricsStore interface.
type MonitoringMetricsStoreAdapter struct {
	store *monitoring.MetricsStore
}

// NewMonitoringMetricsStoreAdapter creates a new adapter.
func NewMonitoringMetricsStoreAdapter(store *monitoring.MetricsStore) *MonitoringMetricsStoreAdapter {
	return &MonitoringMetricsStoreAdapter{store: store}
}

func (a *MonitoringMetricsStoreAdapter) SaveSnapshot(snapshot MetricsSnapshot) error {
	return a.store.SaveSnapshot(monitoring.MetricsSnapshot{
		ScreeningTotal:     snapshot.ScreeningTotal,
		ScreeningPassed:    snapshot.ScreeningPassed,
		ScreeningRate:      snapshot.ScreeningRate,
		AlertsTriggered:    snapshot.AlertsTriggered,
		AlertsAcknowledged: snapshot.AlertsAcknowledged,
		AlertsByType:       snapshot.AlertsByType,
		Timestamp:          snapshot.Timestamp,
	})
}

func (a *MonitoringMetricsStoreAdapter) LoadToday() (*MetricsSnapshot, error) {
	snap, err := a.store.LoadToday()
	if err != nil || snap == nil {
		return nil, err
	}
	return &MetricsSnapshot{
		ScreeningTotal:     snap.ScreeningTotal,
		ScreeningPassed:    snap.ScreeningPassed,
		ScreeningRate:      snap.ScreeningRate,
		AlertsTriggered:    snap.AlertsTriggered,
		AlertsAcknowledged: snap.AlertsAcknowledged,
		AlertsByType:       snap.AlertsByType,
		Timestamp:          snap.Timestamp,
	}, nil
}

func (a *MonitoringMetricsStoreAdapter) LoadRecent(n int) ([]MetricsSnapshot, error) {
	snaps, err := a.store.LoadRecent(n)
	if err != nil {
		return nil, err
	}
	result := make([]MetricsSnapshot, len(snaps))
	for i, snap := range snaps {
		result[i] = MetricsSnapshot{
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

type MonitoringAlertStoreAdapter struct {
	store *monitoring.AlertStore
}

func NewMonitoringAlertStoreAdapter(store *monitoring.AlertStore) *MonitoringAlertStoreAdapter {
	return &MonitoringAlertStoreAdapter{store: store}
}

func (a *MonitoringAlertStoreAdapter) Save(alert domain.AlertRecord) error {
	return a.store.Save(alert)
}

func (a *MonitoringAlertStoreAdapter) LoadAll() ([]domain.AlertRecord, error) {
	return a.store.LoadAll()
}

func (a *MonitoringAlertStoreAdapter) LoadUnacknowledged() ([]domain.AlertRecord, error) {
	return a.store.LoadUnacknowledged()
}

func (a *MonitoringAlertStoreAdapter) Acknowledge(alertID string, user string) error {
	return a.store.Acknowledge(alertID, user)
}

// LedgerOutcomeStoreAdapter adapts ledger.Store to repository.OutcomeStore interface.
type LedgerOutcomeStoreAdapter struct {
	store *ledger.Store
}

// NewLedgerOutcomeStoreAdapter creates a new adapter.
func NewLedgerOutcomeStoreAdapter(store *ledger.Store) *LedgerOutcomeStoreAdapter {
	return &LedgerOutcomeStoreAdapter{store: store}
}

func (a *LedgerOutcomeStoreAdapter) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	return a.store.RecordOutcomes(outcomes)
}

func (a *LedgerOutcomeStoreAdapter) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	return a.store.LoadSessionOutcomes(sessionID)
}

func (a *LedgerOutcomeStoreAdapter) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return a.store.LoadOutcomes()
}
