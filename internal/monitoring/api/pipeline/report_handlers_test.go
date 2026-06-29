package pipeline

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// fakeOutcomeStore satisfies ledger.OutcomeStore for report handler tests.
// LoadLatestReport fails before touching the store when no window summaries exist,
// so all methods can return empty/error safely.
type fakeOutcomeStore struct{}

func (f *fakeOutcomeStore) RecordOutcomes([]domain.RecommendationOutcome) error                         { return nil }
func (f *fakeOutcomeStore) RecordSessionOutcomes(domain.ReplaySession, []domain.RecommendationOutcome) error { return nil }
func (f *fakeOutcomeStore) LoadOutcomes() ([]domain.RecommendationOutcome, error)                        { return nil, nil }
func (f *fakeOutcomeStore) LoadSessionOutcomes(string) ([]domain.RecommendationOutcome, error)           { return nil, nil }
func (f *fakeOutcomeStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error)            { return nil, nil }
func (f *fakeOutcomeStore) RecordSessionScreeningRejects(string, []domain.ScreeningReject) error        { return nil }
func (f *fakeOutcomeStore) LoadSessionScreeningRejects(string) ([]domain.ScreeningReject, error)         { return nil, nil }
func (f *fakeOutcomeStore) RecordSessionTrades(string, []domain.TradeRecord) error                      { return nil }
func (f *fakeOutcomeStore) LoadSessionTrades(string) ([]domain.TradeRecord, error)                       { return nil, nil }
func (f *fakeOutcomeStore) LoadAllSessionTrades() ([]domain.TradeRecord, error)                          { return nil, nil }
func (f *fakeOutcomeStore) RecordExperiment(domain.ExperimentRecord) error                              { return nil }
func (f *fakeOutcomeStore) RecordSessionExperiment(domain.ReplaySession, domain.ExperimentRecord) error { return nil }
func (f *fakeOutcomeStore) RecordSessionSummary(domain.ReplaySession, domain.SessionSummary) error       { return nil }
func (f *fakeOutcomeStore) LoadSessionSummaries() ([]domain.SessionSummary, error)                      { return nil, nil }
func (f *fakeOutcomeStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return nil, nil, nil
}
func (f *fakeOutcomeStore) RecordHumanIntervention(domain.HumanIntervention) error   { return nil }
func (f *fakeOutcomeStore) LoadHumanInterventions() ([]domain.HumanIntervention, error) { return nil, nil }

var _ ledger.OutcomeStore = (*fakeOutcomeStore)(nil)

func TestHandleLatestReport_EmptyWindowsReturnsNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	svc := service.NewReportService(tmpDir, tmpDir, &fakeOutcomeStore{})
	h := NewReportHandlers(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/report/latest", nil)

	status, _ := h.HandleLatestReport(rec, req)
	if status != http.StatusNotFound {
		t.Errorf("HandleLatestReport status = %d, want %d", status, http.StatusNotFound)
	}
}
