package service

import (
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/reporting"
)

// PerformanceService provides performance report generation operations.
type PerformanceService struct {
	store     ledger.OutcomeStore
	ledgerDir string
}

// NewPerformanceService creates a new PerformanceService backed by the given
// outcome store. The store is created by the caller via
// ledger.NewReportOutcomeStore(cfg): postgres backend reads PG first with a
// JSONL fallback + degraded marker (SSoT decision
// docs/decisions/2026-08-23-performance-report-ssot.md); other backends keep
// NewOutcomeStore semantics (perf-report-zero audit BL-01).
func NewPerformanceService(store ledger.OutcomeStore, ledgerDir string) *PerformanceService {
	return &PerformanceService{store: store, ledgerDir: ledgerDir}
}

// GetPerformanceReport generates a performance report for the given period.
// Supported periods: "30d", "90d", "1y", "all".
func (s *PerformanceService) GetPerformanceReport(period string) (*reporting.PerformanceReport, error) {
	return reporting.GenerateReport(s.store, s.ledgerDir, period)
}

// GetAgentContributions returns the top agent contributions for the given period.
func (s *PerformanceService) GetAgentContributions(period string) ([]reporting.AgentContribution, error) {
	report, err := reporting.GenerateReport(s.store, s.ledgerDir, period)
	if err != nil {
		return nil, err
	}
	return report.TopAgents, nil
}

// GetRegimeBreakdown returns the regime breakdown for the given period.
func (s *PerformanceService) GetRegimeBreakdown(period string) (*reporting.RegimeBreakdown, error) {
	report, err := reporting.GenerateReport(s.store, s.ledgerDir, period)
	if err != nil {
		return nil, err
	}
	return &report.RegimeBreakdown, nil
}
