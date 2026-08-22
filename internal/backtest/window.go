package backtest

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/charter"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/reporting"
)

type Runner struct {
	cfg         config.Config
	store       ledger.OutcomeStore
	janusEngine *janus.Engine
	charterOpts *charter.Options // per-arm charter switches (Phase C3); nil = cfg.CharterMode governs
	charterTr   *charter.RecommendationTrace
	lastState   *domain.SimulationState
	eventBus    *eventbus.ChannelEventBus
}

func NewRunner(cfg config.Config, store ledger.OutcomeStore) *Runner {
	return &Runner{cfg: cfg, store: store}
}

func (r *Runner) WithEventBus(eventBus *eventbus.ChannelEventBus) *Runner {
	r.eventBus = eventBus
	return r
}

func (r *Runner) LastState() *domain.SimulationState {
	return r.lastState
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
	for _, date := range ds.WindowDates(startDate, endDate, 1) {
		cfg := r.cfg
		cfg.ReplaySessionDate = date.Format("2006-01-02")
		system, err := orchestrator.NewSystem(cfg)
		if err != nil {
			return domain.BacktestWindowSummary{}, fmt.Errorf("create system: %w", err)
		}
		if r.eventBus != nil {
			system.SetEventBus(r.eventBus)
		}
		if r.janusEngine != nil {
			system.WithJANUS(r.janusEngine, nil)
		}
		if r.charterOpts != nil {
			system.WithCharterMode(*r.charterOpts)
		}
		system.WithPersistentState(&persistentState)
		result, err := system.RunDailySimulation(date)
		if err != nil {
			return domain.BacktestWindowSummary{}, err
		}
		if r.charterTr != nil {
			if rr := system.LastResearchResult(); rr != nil {
				r.charterTr.Append(charter.DailyRecommendationTrace{
					Date:         date.Format("2006-01-02"),
					Regime:       rr.Regime,
					Period:       rr.Period,
					RawCount:     len(rr.RawRecommendations),
					FinalCount:   len(rr.FinalRecommendations),
					RawByAgent:   charter.CountByAgent(rr.RawRecommendations),
					FinalByAgent: charter.CountByAgent(rr.FinalRecommendations),
				})
			}
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

	r.lastState = &persistentState

	scorecards, outcomes, err := r.store.LoadAllSessionScorecards()
	if err != nil {
		return domain.BacktestWindowSummary{}, err
	}

	windowOutcomes := 0
	for _, outcome := range outcomes {
		outcomeDate, err := time.Parse("2006-01-02", outcome.Window)
		if err != nil {
			continue
		}
		if !outcomeDate.Before(startDate) && !outcomeDate.After(endDate) {
			windowOutcomes++
		}
	}

	registry, err := orchestrator.LoadRegistry(r.cfg.AgentRegistryPath)
	if err != nil {
		registry = orchestrator.SeedRegistry()
	}
	candidate := domain.SelectWeakestAgent(registry, scorecards)

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

	bt, ok := r.store.(ledger.BacktestStore)
	if !ok {
		return domain.BacktestWindowSummary{}, fmt.Errorf("store does not implement BacktestStore")
	}
	if err := bt.RecordWindowSummary(summary); err != nil {
		return domain.BacktestWindowSummary{}, err
	}
	if brief := domain.BuildMutationBrief(summary.WindowID, candidate); brief != nil {
		if err := bt.RecordMutationBrief(summary.WindowID, *brief); err != nil {
			return domain.BacktestWindowSummary{}, err
		}
	}

	return summary, nil
}

func (r *Runner) GenerateReport(summary domain.BacktestWindowSummary) (string, error) {
	scorecards, _, err := r.store.LoadAllSessionScorecards()
	if err != nil {
		return "", fmt.Errorf("load scorecards: %w", err)
	}

	sessionSummaries, err := r.store.LoadSessionSummaries()
	if err != nil {
		return "", fmt.Errorf("load session summaries: %w", err)
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

	return reporting.RenderMarkdown(reportData), nil
}

// WithJANUS attaches a JANUS engine to the runner for A/B validation.
func (r *Runner) WithJANUS(j *janus.Engine) *Runner {
	r.janusEngine = j
	return r
}

// WithCharterMode sets per-arm charter switches (Phase C3). Each System built
// inside Run gets these options via System.WithCharterMode, so the harness can
// A/B each charter layer with cfg.CharterMode left off (the global flag is not
// consulted when options are present). nil / zero options leave cfg.CharterMode
// to govern (Phase A when off, full charter when on).
func (r *Runner) WithCharterMode(options charter.Options) *Runner {
	if !options.Enabled() {
		r.charterOpts = nil
		return r
	}
	opts := options
	r.charterOpts = &opts
	return r
}

// WithRecommendationTrace attaches a per-day recommendation pipeline trace
// collector for A/B attribution (raw vs final recommendation counts, detected
// period per day).
func (r *Runner) WithRecommendationTrace(t *charter.RecommendationTrace) *Runner {
	r.charterTr = t
	return r
}
