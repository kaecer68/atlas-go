package backtest

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/evolution"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/replay"
)

type Runner struct {
	cfg config.Config
}

func NewRunner(cfg config.Config) *Runner {
	return &Runner{cfg: cfg}
}

func (r *Runner) Run(startDate, endDate time.Time) (domain.BacktestWindowSummary, error) {
	ds, err := replay.LoadTWSEOpenDataCSV(r.cfg.ReplayDataPath)
	if err != nil {
		return domain.BacktestWindowSummary{}, err
	}

	sessionCount := 0
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
		OutcomeCount: len(outcomes),
		GeneratedAt:  time.Now(),
	}
	if candidate != nil {
		summary.WorstAgentID = candidate.Agent.ID
		summary.WorstAgentSkill = candidate.Agent.Skill
		summary.WorstAgentLayer = candidate.Agent.Layer
		summary.WorstAgentWindowCount = candidate.Scorecard.WindowCount
		summary.WorstAgentSharpeLike = candidate.Scorecard.SharpeLike
	}

	if err := store.RecordWindowSummary(summary); err != nil {
		return domain.BacktestWindowSummary{}, err
	}
	if brief := evolution.BuildMutationBrief(summary.WindowID, candidate); brief != nil {
		if err := store.RecordMutationBrief(summary.WindowID, *brief); err != nil {
			return domain.BacktestWindowSummary{}, err
		}
	}

	return summary, nil
}
