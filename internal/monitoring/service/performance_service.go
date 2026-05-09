package service

import (
	"github.com/kaecer68/atlas-go/internal/reporting"
)

// PerformanceService provides performance report generation operations.
type PerformanceService struct {
	ledgerDir string
}

// NewPerformanceService creates a new PerformanceService.
func NewPerformanceService(ledgerDir string) *PerformanceService {
	return &PerformanceService{ledgerDir: ledgerDir}
}

// GetPerformanceReport generates a performance report for the given period.
// Supported periods: "30d", "90d", "1y", "all".
func (s *PerformanceService) GetPerformanceReport(period string) (*reporting.PerformanceReport, error) {
	return reporting.GenerateReport(s.ledgerDir, period)
}

// GetAgentContributions returns the top agent contributions for the given period.
func (s *PerformanceService) GetAgentContributions(period string) ([]reporting.AgentContribution, error) {
	report, err := reporting.GenerateReport(s.ledgerDir, period)
	if err != nil {
		return nil, err
	}
	return report.TopAgents, nil
}

// GetRegimeBreakdown returns the regime breakdown for the given period.
func (s *PerformanceService) GetRegimeBreakdown(period string) (*reporting.RegimeBreakdown, error) {
	report, err := reporting.GenerateReport(s.ledgerDir, period)
	if err != nil {
		return nil, err
	}
	return &report.RegimeBreakdown, nil
}
