package backtest

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/evolution"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/reporting"
)

type Runner struct {
	cfg         config.Config
	janusEngine *janus.Engine
}

func NewRunner(cfg config.Config) *Runner {
	return &Runner{cfg: cfg}
}

func (r *Runner) Run(startDate, endDate time.Time) (domain.BacktestWindowSummary, error) {
	ds, err := replay.LoadTWSEOpenDataCSV(r.cfg.ReplayDataPath)
	if err != nil {
		return domain.BacktestWindowSummary{}, err
	}

	policy, err := baseline.Load(r.cfg.BaselinePolicyPath)
	if err != nil {
		policy = baseline.DefaultPolicy()
	}

	sessionCount := 0
	persistentState := domain.NewSimulationState(policy.Constraints.StartingCash)
	for _, date := range ds.Dates {
		if date.Before(startDate) || date.After(endDate) {
			continue
		}

		nextDate, ok := ds.NextDate(date, 1)
		if !ok || nextDate.After(endDate) {
			continue
		}

		cfg := r.cfg
		cfg.ReplaySessionDate = date.Format("2006-01-02")
		system := orchestrator.NewSystem(cfg)
		if r.janusEngine != nil {
			system.WithJANUS(r.janusEngine)
		}
		system.WithPersistentState(&persistentState)
		result, err := system.RunDailySimulation(date)
		if err != nil {
			return domain.BacktestWindowSummary{}, err
		}
		candidate, err := system.NextExperimentCandidate()
		if err != nil {
			return domain.BacktestWindowSummary{}, err
		}
		if err := system.RecordSessionSummary(result, candidate); err != nil {
			return domain.BacktestWindowSummary{}, err
		}
		sessionCount++
	}

	store := ledger.NewStore(r.cfg.LedgerDir)
	scorecards, outcomes, err := store.LoadAllSessionScorecards()
	if err != nil {
		return domain.BacktestWindowSummary{}, err
	}

	windowOutcomes := 0
	for _, outcome := range outcomes {
		if !outcome.RecordedAt.Before(startDate) && !outcome.RecordedAt.After(endDate) {
			windowOutcomes++
		}
	}

	registry, err := orchestrator.LoadRegistry(r.cfg.AgentRegistryPath)
	if err != nil {
		registry = orchestrator.SeedRegistry()
	}
	candidate := evolution.SelectWeakestAgent(registry, scorecards)

	summary := domain.BacktestWindowSummary{
		WindowID:     "window-" + startDate.Format("20060102") + "-" + endDate.Format("20060102"),
		StartDate:    startDate,
		EndDate:      endDate,
		SessionCount: sessionCount,
		OutcomeCount: windowOutcomes,
		GeneratedAt:  time.Now(),
	}
	if candidate != nil {
		summary.WorstAgentID = candidate.Agent.ID
		summary.WorstAgentSkill = candidate.Agent.Skill
		summary.WorstAgentLayer = candidate.Agent.Layer
		summary.WorstAgentWindowCount = candidate.Scorecard.WindowCount
		summary.WorstAgentSharpeLike = candidate.Scorecard.SharpeLike
	}

	bt, ok := store.(ledger.BacktestStore)
	if !ok {
		return domain.BacktestWindowSummary{}, fmt.Errorf("store does not implement BacktestStore")
	}
	if err := bt.RecordWindowSummary(summary); err != nil {
		return domain.BacktestWindowSummary{}, err
	}
	if brief := evolution.BuildMutationBrief(summary.WindowID, candidate); brief != nil {
		if err := bt.RecordMutationBrief(summary.WindowID, *brief); err != nil {
			return domain.BacktestWindowSummary{}, err
		}
	}

	return summary, nil
}

// GenerateReport creates a markdown report from the backtest summary and persists it to reports/.
func (r *Runner) GenerateReport(summary domain.BacktestWindowSummary) error {
	store := ledger.NewStore(r.cfg.LedgerDir)
	scorecards, _, err := store.LoadAllSessionScorecards()
	if err != nil {
		return fmt.Errorf("load scorecards: %w", err)
	}

	sessionSummaries, err := store.LoadSessionSummaries()
	if err != nil {
		return fmt.Errorf("load session summaries: %w", err)
	}

	equityCurve := make([]float64, 0, len(sessionSummaries))
	regimeCounts := make(map[string]int)
	for _, s := range sessionSummaries {
		pv := s.PortfolioValue
		if pv == 0 {
			pv = s.EndingCash
		}
		equityCurve = append(equityCurve, pv)
		regimeCounts[string(s.Regime)]++
	}

	reportData := reporting.BacktestReportData{
		WindowID:        summary.WindowID,
		StartDate:       summary.StartDate,
		EndDate:         summary.EndDate,
		SessionCount:    summary.SessionCount,
		OutcomeCount:    summary.OutcomeCount,
		EquityCurve:     equityCurve,
		AgentRows:       reporting.BuildAgentRows(scorecards, nil),
		MutationStats:   reporting.MutationStats{},
		WorstAgentID:    summary.WorstAgentID,
		WorstAgentSkill: summary.WorstAgentSkill,
		WorstSharpeLike: summary.WorstAgentSharpeLike,
		RegimeCounts:    regimeCounts,
	}

	report := reporting.RenderMarkdown(reportData)
	reportDir := "reports"
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return fmt.Errorf("create report dir: %w", err)
	}
	reportPath := filepath.Join(reportDir, fmt.Sprintf("backtest_%s.md", summary.WindowID))
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// WithJANUS attaches a JANUS engine to the runner for A/B validation.
func (r *Runner) WithJANUS(j *janus.Engine) *Runner {
	r.janusEngine = j
	return r
}
