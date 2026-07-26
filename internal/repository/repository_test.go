package repository

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// mockAlertStore implements AlertStore for testing.
type mockAlertStore struct {
	alerts []domain.AlertRecord
}

func (m *mockAlertStore) Save(alert domain.AlertRecord) error {
	m.alerts = append(m.alerts, alert)
	return nil
}

func (m *mockAlertStore) LoadAll() ([]domain.AlertRecord, error) {
	return m.alerts, nil
}

func (m *mockAlertStore) LoadUnacknowledged() ([]domain.AlertRecord, error) {
	var result []domain.AlertRecord
	for _, a := range m.alerts {
		if !a.Acknowledged {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *mockAlertStore) Acknowledge(alertID string, user string) error {
	for i := range m.alerts {
		if m.alerts[i].ID == alertID {
			m.alerts[i].Acknowledged = true
			m.alerts[i].AcknowledgedBy = user
			now := time.Now()
			m.alerts[i].AcknowledgedAt = &now
			return nil
		}
	}
	return nil
}

func (m *mockAlertStore) FindByDedupKey(dedupKey string) (*domain.AlertRecord, error) {
	for i := range m.alerts {
		if m.alerts[i].DedupKey == dedupKey {
			return &m.alerts[i], nil
		}
	}
	return nil, nil
}

func (m *mockAlertStore) Update(id string, fn func(*domain.AlertRecord)) error {
	for i := range m.alerts {
		if m.alerts[i].ID == id {
			fn(&m.alerts[i])
			return nil
		}
	}
	return nil
}

// mockMetricsStore implements MetricsStore for testing.
type mockMetricsStore struct {
	snapshots []MetricsSnapshot
}

func (m *mockMetricsStore) SaveSnapshot(snapshot MetricsSnapshot) error {
	m.snapshots = append(m.snapshots, snapshot)
	return nil
}

func (m *mockMetricsStore) LoadToday() (*MetricsSnapshot, error) {
	if len(m.snapshots) == 0 {
		return nil, nil
	}
	return &m.snapshots[len(m.snapshots)-1], nil
}

func (m *mockMetricsStore) LoadRecent(n int) ([]MetricsSnapshot, error) {
	if n > len(m.snapshots) {
		return m.snapshots, nil
	}
	return m.snapshots[len(m.snapshots)-n:], nil
}

// mockOutcomeStore implements OutcomeStore for testing.
type mockOutcomeStore struct {
	outcomes []domain.RecommendationOutcome
}

func (m *mockOutcomeStore) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	m.outcomes = append(m.outcomes, outcomes...)
	return nil
}

func (m *mockOutcomeStore) RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	return m.RecordOutcomes(outcomes)
}

func (m *mockOutcomeStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return m.outcomes, nil
}

func (m *mockOutcomeStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	return m.outcomes, nil
}

func (m *mockOutcomeStore) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	var result []domain.RecommendationOutcome
	for _, o := range m.outcomes {
		if o.Window == sessionID {
			result = append(result, o)
		}
	}
	return result, nil
}

func (m *mockOutcomeStore) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	return nil
}

func (m *mockOutcomeStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return nil, nil
}

func (m *mockOutcomeStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return nil, m.outcomes, nil
}

func (m *mockOutcomeStore) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	return nil
}

func (m *mockOutcomeStore) LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error) {
	return nil, nil
}

func (m *mockOutcomeStore) RecordExperiment(record domain.ExperimentRecord) error {
	return nil
}

func (m *mockOutcomeStore) RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error {
	return nil
}

func (m *mockOutcomeStore) RecordHumanIntervention(intervention domain.HumanIntervention) error {
	return nil
}

func (m *mockOutcomeStore) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	return nil, nil
}

// mockScreeningRejectStore implements ScreeningRejectStore for testing.
type mockScreeningRejectStore struct{}

func (m *mockScreeningRejectStore) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	return nil
}

func (m *mockScreeningRejectStore) LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error) {
	return nil, nil
}

// mockSessionSummaryStore implements SessionSummaryStore for testing.
type mockSessionSummaryStore struct {
	summaries []domain.SessionSummary
}

func (m *mockSessionSummaryStore) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	m.summaries = append(m.summaries, summary)
	return nil
}

func (m *mockSessionSummaryStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return m.summaries, nil
}

func (m *mockSessionSummaryStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return nil, nil, nil
}

// mockHumanInterventionStore implements HumanInterventionStore for testing.
type mockHumanInterventionStore struct{}

func (m *mockHumanInterventionStore) RecordHumanIntervention(intervention domain.HumanIntervention) error {
	return nil
}

func (m *mockHumanInterventionStore) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	return nil, nil
}

func newTestJSONLRepo() *JSONLRepository {
	return &JSONLRepository{
		alertStore:             &mockAlertStore{},
		metricsStore:           &mockMetricsStore{},
		outcomeStore:           &mockOutcomeStore{},
		screeningRejectStore:   &mockScreeningRejectStore{},
		sessionSummaryStore:    &mockSessionSummaryStore{},
		humanInterventionStore: &mockHumanInterventionStore{},
	}
}

func TestJSONLRepository_AlertStore(t *testing.T) {
	jsonl := newTestJSONLRepo()

	alert := domain.AlertRecord{
		ID:        "test-alert-1",
		Timestamp: time.Now(),
		Rule:      "test-rule",
		Severity:  "warning",
		Message:   "test message",
	}

	if err := jsonl.alertStore.Save(alert); err != nil {
		t.Fatalf("save alert: %v", err)
	}

	alerts, err := jsonl.alertStore.LoadAll()
	if err != nil {
		t.Fatalf("load all alerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].ID != alert.ID {
		t.Errorf("expected alert ID %q, got %q", alert.ID, alerts[0].ID)
	}
}

func TestJSONLRepository_MetricsStore(t *testing.T) {
	jsonl := newTestJSONLRepo()

	snapshot := MetricsSnapshot{
		ScreeningTotal:  100,
		ScreeningPassed: 80,
		ScreeningRate:   0.8,
		Timestamp:       time.Now(),
	}

	if err := jsonl.metricsStore.SaveSnapshot(snapshot); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	loaded, err := jsonl.metricsStore.LoadToday()
	if err != nil {
		t.Fatalf("load today: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if loaded.ScreeningTotal != snapshot.ScreeningTotal {
		t.Errorf("expected screening total %d, got %d", snapshot.ScreeningTotal, loaded.ScreeningTotal)
	}

	recent, err := jsonl.metricsStore.LoadRecent(1)
	if err != nil {
		t.Fatalf("load recent: %v", err)
	}
	if len(recent) != 1 {
		t.Errorf("expected 1 snapshot, got %d", len(recent))
	}
}

func TestJSONLRepository_OutcomeStore(t *testing.T) {
	jsonl := newTestJSONLRepo()

	outcomes := []domain.RecommendationOutcome{
		{Window: "session-20260101", Symbol: "2330", AgentID: "test-agent", Conviction: 80},
		{Window: "session-20260101", Symbol: "2317", AgentID: "test-agent", Conviction: 70},
	}

	if err := jsonl.outcomeStore.RecordOutcomes(outcomes); err != nil {
		t.Fatalf("record outcomes: %v", err)
	}

	loaded, err := jsonl.outcomeStore.LoadSessionOutcomes("session-20260101")
	if err != nil {
		t.Fatalf("load session outcomes: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("expected 2 outcomes, got %d", len(loaded))
	}
}

func TestMetricPoint_Struct(t *testing.T) {
	mp := MetricPoint{
		Time:      time.Now(),
		Name:      "test-metric",
		Value:     42.0,
		AgentID:   "agent-01",
		SessionID: "session-20260101",
		Symbol:    "2330",
		Regime:    "bull",
		Metadata:  map[string]any{"key": "value"},
	}

	if mp.Name != "test-metric" {
		t.Errorf("expected name %q, got %q", "test-metric", mp.Name)
	}
	if mp.Value != 42.0 {
		t.Errorf("expected value 42.0, got %f", mp.Value)
	}
}

func TestMetricsSnapshot_Struct(t *testing.T) {
	snapshot := MetricsSnapshot{
		ScreeningTotal:     100,
		ScreeningPassed:    80,
		ScreeningRate:      0.8,
		AlertsTriggered:    5,
		AlertsAcknowledged: 3,
		AlertsByType:       map[string]int64{"warning": 3, "critical": 2},
		Timestamp:          time.Now(),
	}

	if snapshot.ScreeningRate != 0.8 {
		t.Errorf("expected screening rate 0.8, got %f", snapshot.ScreeningRate)
	}
	if len(snapshot.AlertsByType) != 2 {
		t.Errorf("expected 2 alert types, got %d", len(snapshot.AlertsByType))
	}
}

func TestSymbolCount_Struct(t *testing.T) {
	sc := SymbolCount{Symbol: "2330", Count: 5}
	if sc.Symbol != "2330" {
		t.Errorf("expected symbol 2330, got %s", sc.Symbol)
	}
	if sc.Count != 5 {
		t.Errorf("expected count 5, got %d", sc.Count)
	}
}

func TestCapitalFlowRecord_Struct(t *testing.T) {
	cfr := CapitalFlowRecord{
		Time:      time.Now(),
		Channel:   "foreign",
		NetBuy:    1000.5,
		TotalBuy:  5000.0,
		TotalSell: 4000.0,
	}

	if cfr.Channel != "foreign" {
		t.Errorf("expected channel foreign, got %s", cfr.Channel)
	}
	if cfr.NetBuy != 1000.5 {
		t.Errorf("expected net buy 1000.5, got %f", cfr.NetBuy)
	}
}

func TestExportStatsRecord_Struct(t *testing.T) {
	esr := ExportStatsRecord{
		Time:         time.Now(),
		Year:         2026,
		Month:        1,
		ExportTotal:  1000000.0,
		ImportTotal:  800000.0,
		TradeBalance: 200000.0,
	}

	if esr.Year != 2026 {
		t.Errorf("expected year 2026, got %d", esr.Year)
	}
	if esr.TradeBalance != 200000.0 {
		t.Errorf("expected trade balance 200000.0, got %f", esr.TradeBalance)
	}
}

func TestRepository_Struct(t *testing.T) {
	repo := Repository{}
	if repo.Metrics != nil {
		t.Error("expected nil metrics repo")
	}
	if repo.Alerts != nil {
		t.Error("expected nil alerts repo")
	}
	if repo.Outcomes != nil {
		t.Error("expected nil outcomes repo")
	}
}
