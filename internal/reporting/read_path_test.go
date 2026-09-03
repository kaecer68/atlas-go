package reporting

import (
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// fakeSourceStore implements ledger.OutcomeStore minimally plus the
// reportSourceInfo interface, so GenerateReport surfaces Source/Degraded.
type fakeSourceStore struct {
	sourceBackend string
	degraded      bool
	summaries     []domain.SessionSummary
}

func (f *fakeSourceStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return f.summaries, nil
}
func (f *fakeSourceStore) LoadSessionOutcomes(string) ([]domain.RecommendationOutcome, error) {
	return nil, nil
}
func (f *fakeSourceStore) SourceBackend() string { return f.sourceBackend }
func (f *fakeSourceStore) Degraded() bool        { return f.degraded }

// The rest of ledger.OutcomeStore is unused by GenerateReport — panicking
// keeps the fake honest.
func (f *fakeSourceStore) RecordOutcomes([]domain.RecommendationOutcome) error { panic("unused") }
func (f *fakeSourceStore) RecordSessionOutcomes(domain.ReplaySession, []domain.RecommendationOutcome) error {
	panic("unused")
}
func (f *fakeSourceStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) { panic("unused") }
func (f *fakeSourceStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	panic("unused")
}
func (f *fakeSourceStore) RecordSessionScreeningRejects(string, []domain.ScreeningReject) error {
	panic("unused")
}
func (f *fakeSourceStore) LoadSessionScreeningRejects(string) ([]domain.ScreeningReject, error) {
	panic("unused")
}
func (f *fakeSourceStore) RecordSessionTrades(string, []domain.TradeRecord) error { panic("unused") }
func (f *fakeSourceStore) LoadSessionTrades(string) ([]domain.TradeRecord, error) { panic("unused") }
func (f *fakeSourceStore) LoadAllSessionTrades() ([]domain.TradeRecord, error) {
	// GenerateReport now counts executed trades (SSOT P1-4) — the fake has no
	// trades, which is a legitimate "no executions in window" answer.
	return nil, nil
}
func (f *fakeSourceStore) RecordExperiment(domain.ExperimentRecord) error { panic("unused") }
func (f *fakeSourceStore) RecordSessionExperiment(domain.ReplaySession, domain.ExperimentRecord) error {
	panic("unused")
}
func (f *fakeSourceStore) RecordSessionSummary(domain.ReplaySession, domain.SessionSummary) error {
	panic("unused")
}
func (f *fakeSourceStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	panic("unused")
}
func (f *fakeSourceStore) RecordHumanIntervention(domain.HumanIntervention) error { panic("unused") }
func (f *fakeSourceStore) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	panic("unused")
}

func testSummary(id string, pv float64) domain.SessionSummary {
	return domain.SessionSummary{
		SessionID:      id,
		Regime:         domain.RegimeRiskOn,
		EndingCash:     100_000,
		PortfolioValue: pv,
		OutcomeCount:   1,
	}
}

// TestGenerateReport_SourceMarkers verifies the SSoT read-path contract:
// a store reporting jsonl/degraded surfaces Source=jsonl + Degraded=true on
// the report (and in markdown); a healthy postgres store stays clean.
func TestGenerateReport_SourceMarkers(t *testing.T) {
	summaries := []domain.SessionSummary{
		testSummary("session-20260710-daily", 1_000_000),
		testSummary("session-20260714-daily", 1_100_000),
	}

	t.Run("degraded jsonl fallback", func(t *testing.T) {
		store := &fakeSourceStore{sourceBackend: "jsonl", degraded: true, summaries: summaries}
		report, err := GenerateReport(store, "", "all")
		if err != nil {
			t.Fatalf("GenerateReport: %v", err)
		}
		if report.Source != "jsonl" {
			t.Errorf("Source = %q, want jsonl", report.Source)
		}
		if !report.Degraded {
			t.Error("Degraded = false, want true")
		}
		md := GenerateMarkdownReport(report)
		if !strings.Contains(md, "Degraded source") {
			t.Errorf("markdown missing degraded annotation:\n%s", md)
		}
		if !strings.Contains(md, "JSONL fallback") {
			t.Errorf("markdown missing fallback explanation:\n%s", md)
		}
	})

	t.Run("healthy postgres", func(t *testing.T) {
		store := &fakeSourceStore{sourceBackend: "postgres", degraded: false, summaries: summaries}
		report, err := GenerateReport(store, "", "all")
		if err != nil {
			t.Fatalf("GenerateReport: %v", err)
		}
		if report.Source != "postgres" {
			t.Errorf("Source = %q, want postgres", report.Source)
		}
		if report.Degraded {
			t.Error("Degraded = true, want false")
		}
		md := GenerateMarkdownReport(report)
		if strings.Contains(md, "Degraded source") {
			t.Errorf("markdown should not warn degraded for healthy postgres:\n%s", md)
		}
	})

	t.Run("empty report carries source", func(t *testing.T) {
		store := &fakeSourceStore{sourceBackend: "jsonl", degraded: true, summaries: nil}
		report, err := GenerateReport(store, "", "all")
		if err != nil {
			t.Fatalf("GenerateReport: %v", err)
		}
		if report.Source != "jsonl" || !report.Degraded {
			t.Errorf("empty report should still carry source markers, got Source=%q Degraded=%v", report.Source, report.Degraded)
		}
	})

	t.Run("plain store has no markers", func(t *testing.T) {
		report, err := GenerateReport(&fakeSourceStore{summaries: summaries}, "", "all")
		if err != nil {
			t.Fatalf("GenerateReport: %v", err)
		}
		if report.Source != "" || report.Degraded {
			t.Errorf("plain store should leave markers empty, got Source=%q Degraded=%v", report.Source, report.Degraded)
		}
	})
}
