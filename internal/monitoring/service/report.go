package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/risk"
)

// ReportEntry represents a backtest report file listing.
type ReportEntry struct {
	Filename  string `json:"filename"`
	Path      string `json:"path"`
	UpdatedAt string `json:"updated_at"`
}

// ReportService encapsulates backtest report loading operations.
type ReportService struct {
	workDir         string
	ledgerDir       string
	reportGenerator *narrative.ReportGenerator
}

// NewReportService creates a new ReportService.
func NewReportService(workDir, ledgerDir string) *ReportService {
	return &ReportService{
		workDir:         workDir,
		ledgerDir:       ledgerDir,
		reportGenerator: narrative.NewReportGenerator(),
	}
}

// LoadLatestReport returns the content and filename of the most recent backtest report.
func (s *ReportService) LoadLatestReport() (content []byte, filename string, err error) {
	reportDir := filepath.Join(s.workDir, "reports")
	entries, err := os.ReadDir(reportDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("no reports directory found")
		}
		return nil, "", fmt.Errorf("read reports dir: %w", err)
	}

	var latestFile string
	var latestTime time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "backtest_") || !strings.HasSuffix(name, ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if latestFile == "" || info.ModTime().After(latestTime) {
			latestFile = name
			latestTime = info.ModTime()
		}
	}

	if latestFile == "" {
		return nil, "", fmt.Errorf("no backtest report found")
	}

	path := filepath.Join(reportDir, latestFile)
	content, err = os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read report: %w", err)
	}

	return content, latestFile, nil
}

// LoadReportList returns all backtest report entries sorted by updated_at descending.
func (s *ReportService) LoadReportList() ([]ReportEntry, error) {
	reportDir := filepath.Join(s.workDir, "reports")
	entries, err := os.ReadDir(reportDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ReportEntry{}, nil
		}
		return nil, fmt.Errorf("read reports dir: %w", err)
	}

	reports := make([]ReportEntry, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "backtest_") || !strings.HasSuffix(name, ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		reports = append(reports, ReportEntry{
			Filename:  name,
			Path:      "/api/report/latest?file=" + name,
			UpdatedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	slices.SortFunc(reports, func(a, b ReportEntry) int {
		aTime, _ := time.Parse(time.RFC3339, a.UpdatedAt)
		bTime, _ := time.Parse(time.RFC3339, b.UpdatedAt)
		switch {
		case aTime.After(bTime):
			return -1
		case aTime.Before(bTime):
			return 1
		default:
			return 0
		}
	})

	return reports, nil
}

// LoadDailySummary returns a DailySummaryReport for the given date.
func (s *ReportService) LoadDailySummary(date string) (*domain.DailySummaryReport, error) {
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	events := s.loadNarrativeEventsForDate(date)
	recs := s.loadRecommendationsForDate(date)
	riskSnap := s.loadRiskSnapshot()

	report := s.reportGenerator.GenerateDailySummary(date, events, recs, riskSnap)
	return report, nil
}

func (s *ReportService) loadNarrativeEventsForDate(date string) []narrative.NarrativeEvent {
	sessionsDir := filepath.Join(s.ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionDate := sessionDateFromID(entry.Name())
		if sessionDate.IsZero() {
			continue
		}
		if sessionDate.Format("2006-01-02") != date {
			continue
		}

		eventsPath := filepath.Join(sessionsDir, entry.Name(), "narrative_events.json")
		data, err := os.ReadFile(eventsPath)
		if err != nil {
			return nil
		}

		var events []narrative.NarrativeEvent
		if err := json.Unmarshal(data, &events); err != nil {
			return nil
		}
		return events
	}

	return nil
}

func (s *ReportService) loadRiskSnapshot() *domain.RiskSnapshot {
	sessionsDir := filepath.Join(s.ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}

	type sessionEntry struct {
		name  string
		value float64
	}
	sessions := make([]sessionEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, entry.Name(), "summary.json")
		bytes, err := os.ReadFile(summaryPath)
		if err != nil {
			continue
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(bytes, &summary); err != nil {
			continue
		}
		sessions = append(sessions, sessionEntry{name: entry.Name(), value: summary.PortfolioValue})
	}

	slices.SortFunc(sessions, func(a, b sessionEntry) int {
		return strings.Compare(a.name, b.name)
	})

	portfolioValues := make([]float64, len(sessions))
	for i, se := range sessions {
		portfolioValues[i] = se.value
	}

	dailyReturns := make([]float64, 0, max(0, len(portfolioValues)-1))
	for i := 1; i < len(portfolioValues); i++ {
		if portfolioValues[i-1] > 0 {
			dailyReturns = append(dailyReturns, (portfolioValues[i]-portfolioValues[i-1])/portfolioValues[i-1])
		}
	}

	if len(dailyReturns) >= 30 {
		computed := risk.ComputeRiskSnapshot(dailyReturns, portfolioValues)
		return &domain.RiskSnapshot{
			VaR95:          computed.VaR95,
			VaR99:          computed.VaR99,
			CVaR95:         computed.CVaR95,
			MaxDrawdownPct: computed.MaxDrawdownPct,
		}
	}

	return nil
}

func (s *ReportService) loadRecommendationsForDate(date string) []domain.Recommendation {
	sessionsDir := filepath.Join(s.ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionDate := sessionDateFromID(entry.Name())
		if sessionDate.IsZero() {
			continue
		}
		if sessionDate.Format("2006-01-02") != date {
			continue
		}

		outcomesPath := filepath.Join(sessionsDir, entry.Name(), "recommendation_outcomes.jsonl")
		data, err := os.ReadFile(outcomesPath)
		if err != nil {
			return nil
		}

		var recs []domain.Recommendation
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var outcome struct {
				AgentID             string                      `json:"AgentID"`
				Skill               string                      `json:"Skill"`
				Layer               string                      `json:"Layer"`
				Symbol              string                      `json:"Symbol"`
				Side                string                      `json:"Side"`
				Conviction          int                         `json:"Conviction"`
				TargetPrice         float64                     `json:"TargetPrice"`
				StopLossPrice       float64                     `json:"StopLossPrice"`
				Reason              string                      `json:"Reason"`
				FactorScores        domain.FactorScores         `json:"factor_scores,omitempty"`
				ConvictionBreakdown *domain.ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
			}
			if err := json.Unmarshal([]byte(line), &outcome); err != nil {
				continue
			}
			recs = append(recs, domain.Recommendation{
				Agent:               outcome.AgentID,
				Skill:               outcome.Skill,
				Layer:               domain.AgentLayer(outcome.Layer),
				Symbol:              outcome.Symbol,
				Side:                domain.Side(outcome.Side),
				Conviction:          outcome.Conviction,
				TargetPrice:         outcome.TargetPrice,
				StopLossPrice:       outcome.StopLossPrice,
				Reason:              outcome.Reason,
				FactorScores:        outcome.FactorScores,
				ConvictionBreakdown: outcome.ConvictionBreakdown,
			})
		}
		return recs
	}

	return nil
}
