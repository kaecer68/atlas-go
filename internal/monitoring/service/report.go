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
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/reporting"
	"github.com/kaecer68/atlas-go/internal/risk"
)

// ReportEntry represents a backtest report file listing.
type ReportEntry struct {
	Filename  string `json:"filename"`
	Path      string `json:"path"`
	UpdatedAt string `json:"updated_at"`
}

// PeriodInfo carries the seven-period classification result for daily summary reports.
type PeriodInfo struct {
	MarketPeriod        string
	Confidence          float64
	CashLevel           float64
	ConditionsHit       int
	ConditionsTotal     int
	TriggeredIndicators []IndicatorHit
}

// IndicatorHit mirrors the portfolio.TriggeredIndicator at the report-service DTO layer.
type IndicatorHit struct {
	Name           string  `json:"name"`
	Value          float64 `json:"value"`
	Threshold      float64 `json:"threshold"`
	Relation       string  `json:"relation"`
	Hit            bool    `json:"hit"`
	InputAvailable bool    `json:"input_available"`
}

// PeriodProvider returns the current period classification when available.
type PeriodProvider func() *PeriodInfo

type ReportService struct {
	workDir         string
	ledgerDir       string
	reportGenerator *narrative.ReportGenerator
	store           ledger.OutcomeStore
	periodProvider  PeriodProvider
}

// SetPeriodProvider sets an optional callback for period-enriched daily summaries.
func (s *ReportService) SetPeriodProvider(p PeriodProvider) {
	s.periodProvider = p
}

// NewReportService creates a new ReportService.
func NewReportService(workDir, ledgerDir string, store ledger.OutcomeStore) *ReportService {
	return &ReportService{
		workDir:         workDir,
		ledgerDir:       ledgerDir,
		reportGenerator: narrative.NewReportGenerator(),
		store:           store,
	}
}

// LoadLatestReport returns the content and filename of the most recent backtest report.
func (s *ReportService) LoadLatestReport() (content []byte, filename string, err error) {
	summary, err := s.loadLatestWindowSummary()
	if err != nil {
		return nil, "", err
	}

	report, err := s.renderWindowReport(summary)
	if err != nil {
		return nil, "", fmt.Errorf("render report: %w", err)
	}

	filename = fmt.Sprintf("backtest_%s.md", summary.WindowID)
	return []byte(report), filename, nil
}

// LoadReportList returns all backtest report entries sorted by updated_at descending.
func (s *ReportService) LoadReportList() ([]ReportEntry, error) {
	summaries, err := s.loadAllWindowSummaries()
	if err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return []ReportEntry{}, nil
	}

	reports := make([]ReportEntry, 0, len(summaries))
	for _, summary := range summaries {
		filename := fmt.Sprintf("backtest_%s.md", summary.WindowID)
		reports = append(reports, ReportEntry{
			Filename:  filename,
			Path:      "/api/report/latest?window_id=" + summary.WindowID,
			UpdatedAt: summary.GeneratedAt.UTC().Format(time.RFC3339),
		})
	}

	slices.SortFunc(reports, func(a, b ReportEntry) int {
		aTime, err := time.Parse(time.RFC3339, a.UpdatedAt)
		if err != nil {
			logging.Warn("report", "parse_updated_at_failed", "err", err.Error())
			return 1
		}
		bTime, err := time.Parse(time.RFC3339, b.UpdatedAt)
		if err != nil {
			logging.Warn("report", "parse_updated_at_failed", "err", err.Error())
			return -1
		}
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

	if s.periodProvider != nil {
		if info := s.periodProvider(); info != nil {
			report.MarketPeriod = info.MarketPeriod
			report.PeriodConfidence = info.Confidence
			report.PeriodCashLevel = info.CashLevel
			report.PeriodCondHit = info.ConditionsHit
			report.PeriodCondTotal = info.ConditionsTotal
		}
	}

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
		sessionDate := domain.SessionDateFromID(entry.Name())
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
			logging.Warn("report_service", "parse_narrative_events_failed", logging.Err(err))
			return nil
		}
		return events
	}

	return nil
}

func (s *ReportService) loadLatestWindowSummary() (domain.BacktestWindowSummary, error) {
	summaries, err := s.loadAllWindowSummaries()
	if err != nil {
		return domain.BacktestWindowSummary{}, err
	}
	if len(summaries) == 0 {
		return domain.BacktestWindowSummary{}, fmt.Errorf("no backtest window found")
	}

	latest := summaries[0]
	for _, summary := range summaries[1:] {
		if summary.GeneratedAt.After(latest.GeneratedAt) {
			latest = summary
		}
	}
	return latest, nil
}

func (s *ReportService) loadAllWindowSummaries() ([]domain.BacktestWindowSummary, error) {
	windowsDir := filepath.Join(s.ledgerDir, "windows")
	entries, err := os.ReadDir(windowsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read windows dir: %w", err)
	}

	var summaries []domain.BacktestWindowSummary
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, "-mutation-brief.json") {
			continue
		}

		path := filepath.Join(windowsDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var summary domain.BacktestWindowSummary
		if err := json.Unmarshal(data, &summary); err != nil {
			logging.Warn("report_service", "corrupted_window_summary_skipped", logging.Err(err))
			continue
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

func (s *ReportService) renderWindowReport(summary domain.BacktestWindowSummary) (string, error) {
	store := s.store
	scorecards, _, err := store.LoadAllSessionScorecards()
	if err != nil {
		return "", fmt.Errorf("load scorecards: %w", err)
	}

	sessionSummaries, err := store.LoadSessionSummaries()
	if err != nil {
		return "", fmt.Errorf("load session summaries: %w", err)
	}

	equityCurve := make([]float64, 0, len(sessionSummaries))
	regimeCounts := make(map[string]int)
	for _, sess := range sessionSummaries {
		pv := sess.PortfolioValue
		if pv == 0 {
			pv = sess.EndingCash
		}
		equityCurve = append(equityCurve, pv)
		regimeCounts[string(sess.Regime)]++
	}

	reportData := reporting.BacktestReportData{
		WindowID:              summary.WindowID,
		StartDate:             summary.StartDate,
		EndDate:               summary.EndDate,
		SessionCount:          summary.SessionCount,
		OutcomeCount:          summary.OutcomeCount,
		EquityCurve:           equityCurve,
		AgentRows:             reporting.BuildAgentRows(scorecards, nil),
		MutationStats:         reporting.MutationStats{},
		WorstAgentID:          summary.WorstAgentID,
		WorstAgentSkill:       summary.WorstAgentSkill,
		WorstAgentLayer:       string(summary.WorstAgentLayer),
		WorstAgentWindowCount: summary.WorstAgentWindowCount,
		WorstSharpeLike:       summary.WorstAgentSharpeLike,
		RegimeCounts:          regimeCounts,
	}

	return reporting.RenderMarkdown(reportData), nil
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
			logging.Warn("report_service", "corrupted_session_summary_skipped", logging.Err(err))
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
		sessionDate := domain.SessionDateFromID(entry.Name())
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
		lines := strings.SplitSeq(strings.TrimSpace(string(data)), "\n")
		for line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var outcome struct {
				AgentID             string                      `json:"agent_id"`
				Skill               string                      `json:"skill"`
				Layer               string                      `json:"layer"`
				Symbol              string                      `json:"symbol"`
				Side                string                      `json:"side"`
				Conviction          int                         `json:"conviction"`
				TargetPrice         float64                     `json:"target_price"`
				StopLossPrice       float64                     `json:"stop_loss_price"`
				Reason              string                      `json:"reason"`
				FactorScores        domain.FactorScores         `json:"factor_scores"`
				ConvictionBreakdown *domain.ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
			}
			if err := json.Unmarshal([]byte(line), &outcome); err != nil {
				logging.Warn("report_service", "corrupted_recommendation_skipped", logging.Err(err))
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
