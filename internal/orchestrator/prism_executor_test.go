package orchestrator

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func TestPRISMTrainingExecutorRunUsesLedgerMaxDrawdown(t *testing.T) {
	ds, err := replay.LoadTWSEOpenDataCSV("../../samples/replay/twse_stock_day_all_sample.csv")
	if err != nil {
		t.Fatalf("load replay dataset: %v", err)
	}

	registry := SeedRegistry()
	policy := baseline.DefaultPolicy()
	executor := NewPRISMTrainingExecutor(ds, registry, policy)
	start, _ := time.Parse("2006-01-02", "2026-03-20")
	end, _ := time.Parse("2006-01-02", "2026-03-30")

	checked := 0
	for _, agent := range registry.Agents {
		scorecard, ok := prismScorecardForAgent(ds, registry, policy, agent.ID, start, end)
		if !ok {
			continue
		}
		checked++

		result, err := executor.Run(prism.TrainingTask{
			AgentID:     agent.ID,
			WindowStart: start,
			WindowEnd:   end,
			Regime:      prism.RegimeRiskOn,
		})
		if err != nil {
			t.Fatalf("Run(%s): %v", agent.ID, err)
		}

		if math.Abs(result.MaxDrawdown-scorecard.MaxDrawdown) > 1e-12 {
			t.Fatalf("agent %s MaxDrawdown = %f, want ledger authority %f", agent.ID, result.MaxDrawdown, scorecard.MaxDrawdown)
		}
	}

	if checked == 0 {
		t.Fatal("expected at least one agent with PRISM outcomes in sample replay window")
	}
}

func prismScorecardForAgent(ds *replay.Dataset, registry domain.AgentRegistry, policy baseline.Policy, agentID string, start, end time.Time) (domain.Scorecard, bool) {
	symbols := RegistrySymbols(registry)
	outcomes := make([]domain.RecommendationOutcome, 0)

	for _, date := range ds.WindowDates(start, end, 1) {
		if _, ok := ds.NextDate(date, 1); !ok {
			continue
		}

		quotes := ds.QuotesForDate(date, symbols)
		regime, rawRecs, _, _ := ExecuteRegistryResearchDetailedWithPolicyAndGuards(
			registry, quotes, policy.PromptOverrides, policy.ExecutionPolicy,
		)
		if mapDomainRegimeToPRISMTrainingRegime(regime) != prism.RegimeRiskOn {
			continue
		}

		for _, rec := range rawRecs {
			if rec.Agent != agentID {
				continue
			}
			fr, ok := ds.ForwardReturn(rec.Symbol, date, 1)
			if !ok {
				continue
			}
			outcomes = append(outcomes, domain.RecommendationOutcome{
				AgentID:        rec.Agent,
				Skill:          rec.Skill,
				Layer:          rec.Layer,
				Symbol:         rec.Symbol,
				Window:         date.Format("2006-01-02"),
				ForwardReturn:  fr,
				BenchmarkDelta: fr - 0.003,
				Hit:            fr > 0,
				Reason:         rec.Reason,
				RecordedAt:     date,
			})
		}
	}

	if len(outcomes) == 0 {
		return domain.Scorecard{}, false
	}

	for _, scorecard := range ledger.BuildScorecards(outcomes) {
		if scorecard.AgentID == agentID {
			return scorecard, true
		}
	}

	return domain.Scorecard{}, false
}

func TestPRISMTrainingExecutorRunFiltersSamplesByTaskRegime(t *testing.T) {
	riskOnDate := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	riskOffDate := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	settlementDate := time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC)

	ds := &replay.Dataset{
		ByDate: map[string]map[string]domain.DailyBar{
			riskOnDate.Format("2006-01-02"): {
				"2881.TW": {Date: riskOnDate, Symbol: "2881.TW", Open: 80, High: 92, Low: 79, Close: 90, Volume: 5_000_000},
				"2882.TW": {Date: riskOnDate, Symbol: "2882.TW", Open: 70, High: 82, Low: 69, Close: 80, Volume: 5_000_000},
			},
			riskOffDate.Format("2006-01-02"): {
				"2881.TW": {Date: riskOffDate, Symbol: "2881.TW", Open: 100, High: 101, Low: 94, Close: 95, Volume: 5_000_000},
				"2882.TW": {Date: riskOffDate, Symbol: "2882.TW", Open: 90, High: 91, Low: 84, Close: 85, Volume: 5_000_000},
			},
			settlementDate.Format("2006-01-02"): {
				"2881.TW": {Date: settlementDate, Symbol: "2881.TW", Open: 96, High: 97, Low: 84, Close: 85, Volume: 5_000_000},
				"2882.TW": {Date: settlementDate, Symbol: "2882.TW", Open: 86, High: 87, Low: 74, Close: 75, Volume: 5_000_000},
			},
		},
		Dates: []time.Time{riskOnDate, riskOffDate, settlementDate},
	}

	registry := domain.AgentRegistry{
		Version: 1,
		Agents: []domain.AgentSpec{{
			ID:         "financials-desk-01",
			Name:       "Financials Desk",
			Layer:      domain.LayerSector,
			Skill:      "financials_desk",
			Enabled:    true,
			Universe:   []string{"2881.TW", "2882.TW"},
			PromptFile: "",
		}},
	}

	executor := NewPRISMTrainingExecutor(ds, registry, baseline.DefaultPolicy())
	result, err := executor.Run(prism.TrainingTask{
		AgentID:     "financials-desk-01",
		WindowStart: riskOnDate,
		WindowEnd:   riskOffDate,
		Regime:      prism.RegimeRiskOn,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.SignalsCount != 2 {
		t.Fatalf("expected only the two risk-on outcomes to count, got %d signals", result.SignalsCount)
	}
	if result.WinCount != 2 {
		t.Fatalf("expected 2 wins from the matching risk-on date, got %d", result.WinCount)
	}
	if result.LossCount != 0 {
		t.Fatalf("expected 0 losses after regime filtering, got %d", result.LossCount)
	}

	wantReturn := (95.0-90.0)/90.0 + (85.0-80.0)/80.0
	if math.Abs(result.TotalReturn-wantReturn) > 1e-9 {
		t.Fatalf("expected total return %.10f, got %.10f", wantReturn, result.TotalReturn)
	}
}
