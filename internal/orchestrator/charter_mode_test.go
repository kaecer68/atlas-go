package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/macroflow"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/methodology"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/sim"
)

// ─── fixtures ───────────────────────────────────────────────────────────

// charterTestRegistry is a minimal recommendation-producing registry whose
// skills span three charter strategy categories:
//
//	growth_momentum → growth, value_yield → value, etf_rotation_desk → all_weather
func charterTestRegistry() domain.AgentRegistry {
	return domain.AgentRegistry{
		Version: 1,
		Agents: []domain.AgentSpec{
			{ID: "gm-01", Name: "Growth Momentum", Layer: domain.LayerStyle, Skill: "growth_momentum", Enabled: true, Universe: []string{"2317.TW"}},
			{ID: "vy-01", Name: "Value Yield", Layer: domain.LayerStyle, Skill: "value_yield", Enabled: true, Universe: []string{"2881.TW"}},
			{ID: "etf-01", Name: "ETF Rotation", Layer: domain.LayerSector, Skill: "etf_rotation_desk", Enabled: true, Universe: []string{"0050.TW"}},
		},
	}
}

func charterTestQuotes() []domain.Quote {
	return []domain.Quote{
		{Symbol: "2317.TW", Open: 160.5, High: 160.5, Low: 158.5, Last: 160, Volume: 42_506_789, IsTradable: true},
		{Symbol: "2881.TW", Open: 68, High: 69, Low: 67.8, Last: 68.8, Volume: 6_800_000, IsTradable: true},
		{Symbol: "0050.TW", Open: 198, High: 199, Low: 197.5, Last: 198.5, Volume: 12_000_000, IsTradable: true},
	}
}

func charterSkills(result ResearchResult) map[string]int {
	counts := make(map[string]int)
	for _, rec := range result.RawRecommendations {
		counts[rec.Skill]++
	}
	return counts
}

// ─── System wiring (item 2) ─────────────────────────────────────────────

func TestNewSystem_CharterModeWiring(t *testing.T) {
	// CharterMode=true → period detector + macroflow engine + advisor wired.
	sys, err := NewSystem(config.Config{
		ReplayDataPath: config.GetReplayDataPath(".."),
		CharterMode:    true,
	})
	if err != nil {
		t.Fatalf("NewSystem(CharterMode=true): %v", err)
	}
	if sys.charter == nil {
		t.Fatal("charter should be wired when CharterMode=true")
	}
	if sys.charter.periodDetector == nil {
		t.Error("periodDetector should be initialized in CharterMode")
	}
	if sys.charter.macroflow == nil {
		t.Error("macroflow engine should be initialized in CharterMode")
	}
	if sys.charter.advisor == nil {
		t.Error("advisor should be initialized in CharterMode")
	}
	if sys.macroSnapshot == nil {
		t.Error("macroSnapshot should be initialized")
	}

	// CharterMode=false (default) → charter stays nil (Phase A).
	sysOff, err := NewSystem(config.Config{ReplayDataPath: config.GetReplayDataPath("..")})
	if err != nil {
		t.Fatalf("NewSystem(CharterMode=false): %v", err)
	}
	if sysOff.charter != nil {
		t.Error("charter should stay nil when CharterMode=false (Phase A)")
	}
}

// ─── ExecuteWithContext on/off (items 3+4) ──────────────────────────────

// TestExecuteWithContext_CharterModeOn verifies that with a charter-wired
// context: ctx.Period is detected (black_swan from VIX≥35), the macro flow
// adjustment is non-nil, and raw recommendations are gated by the period's
// allowed strategies (only all_weather survives black_swan).
func TestExecuteWithContext_CharterModeOn(t *testing.T) {
	snapshot := &marketdata.MacroDataSnapshot{
		// PR-3b graded rule: single extreme (VIX ≥ 35×1.5 = 52.5) → black_swan.
		VIX:        marketdata.MacroDataPoint{Value: 54},
		RecordedAt: time.Now().Unix(), // not stale
	}
	advisor := methodology.NewAdvisor(nil)
	execCtx := ExecutionContext{
		Registry:          charterTestRegistry(),
		Quotes:            charterTestQuotes(),
		Plugins:           NewPluginRegistry(),
		PeriodDetector:    portfolio.NewPeriodDetectorWithDefaults(),
		MacroFlow:         DefaultMacroFlowStrategy{engine: macroflow.NewEngine(0)},
		MacroDataSnapshot: snapshot,
		PeriodStrategyFilter: func(period domain.MarketPeriod, recs []domain.Recommendation, registry domain.AgentRegistry) []domain.Recommendation {
			return filterRecommendationsByPeriod(period, recs, registry, advisor)
		},
	}

	result := ExecuteWithContext(execCtx)

	if result.Period == nil {
		t.Fatal("CharterMode on: Period should be non-nil")
	}
	if *result.Period != domain.PeriodBlackSwan {
		t.Fatalf("CharterMode on: Period = %q, want black_swan", *result.Period)
	}
	if result.MacroFlowAdjustment == nil {
		t.Fatal("CharterMode on: MacroFlowAdjustment should be non-nil")
	}
	if result.MacroFlowAdjustment.RiskLevel != macroflow.RiskRed {
		t.Errorf("MacroFlowAdjustment.RiskLevel = %q, want red (black_swan)", result.MacroFlowAdjustment.RiskLevel)
	}

	skills := charterSkills(result)
	if skills["growth_momentum"] != 0 {
		t.Errorf("black_swan should drop growth_momentum (growth) recs, got %d", skills["growth_momentum"])
	}
	if skills["value_yield"] != 0 {
		t.Errorf("black_swan should drop value_yield (value) recs, got %d", skills["value_yield"])
	}
	if skills["etf_rotation_desk"] == 0 {
		t.Error("black_swan should keep etf_rotation_desk (all_weather) recs")
	}
	for skill := range skills {
		cat := methodology.SkillToStrategyCategory(skill)
		if !advisor.IsStrategyAllowed(*result.Period, cat) {
			t.Errorf("rec skill %q (category %q) survived black_swan gating but is not allowed", skill, cat)
		}
	}
}

// TestExecuteWithContext_CharterModeOff verifies Phase A parity: without the
// charter fields, Period stays nil, MacroFlowAdjustment stays nil, and no
// period filtering happens.
func TestExecuteWithContext_CharterModeOff(t *testing.T) {
	execCtx := ExecutionContext{
		Registry: charterTestRegistry(),
		Quotes:   charterTestQuotes(),
		Plugins:  NewPluginRegistry(),
	}

	result := ExecuteWithContext(execCtx)

	if result.Period != nil {
		t.Errorf("CharterMode off: Period should be nil, got %q", *result.Period)
	}
	if result.MacroFlowAdjustment != nil {
		t.Error("CharterMode off: MacroFlowAdjustment should be nil")
	}

	skills := charterSkills(result)
	for _, skill := range []string{"growth_momentum", "value_yield", "etf_rotation_desk"} {
		if skills[skill] == 0 {
			t.Errorf("CharterMode off: expected unfiltered recs from %q, got none", skill)
		}
	}
}

// ─── filterRecommendationsByPeriod unit (item 4) ────────────────────────

func TestFilterRecommendationsByPeriod(t *testing.T) {
	advisor := methodology.NewAdvisor(nil)
	registry := charterTestRegistry()
	recs := []domain.Recommendation{
		{Agent: "gm-01", Skill: "growth_momentum", Symbol: "2317.TW"},    // growth
		{Agent: "vy-01", Skill: "value_yield", Symbol: "2881.TW"},        // value
		{Agent: "etf-01", Skill: "etf_rotation_desk", Symbol: "0050.TW"}, // all_weather
		{Agent: "unk-01", Skill: "brand_new_skill", Symbol: "9999.TW"},   // unmapped → all_weather
	}

	t.Run("black_swan keeps only all_weather", func(t *testing.T) {
		got := filterRecommendationsByPeriod(domain.PeriodBlackSwan, recs, registry, advisor)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (etf_rotation_desk + unmapped all_weather)", len(got))
		}
		for _, rec := range got {
			if rec.Skill != "etf_rotation_desk" && rec.Skill != "brand_new_skill" {
				t.Errorf("black_swan should keep only all_weather-category skills, got %q", rec.Skill)
			}
		}
	})

	t.Run("bull keeps growth drops value/all_weather", func(t *testing.T) {
		got := filterRecommendationsByPeriod(domain.PeriodBull, recs, registry, advisor)
		if len(got) != 1 || got[0].Skill != "growth_momentum" {
			t.Fatalf("bull should keep only growth_momentum, got %+v", got)
		}
	})

	t.Run("unknown period passes through", func(t *testing.T) {
		got := filterRecommendationsByPeriod("unknown_period", recs, registry, advisor)
		if len(got) != len(recs) {
			t.Fatalf("unknown period should pass through unfiltered: got %d, want %d", len(got), len(recs))
		}
	})

	t.Run("nil recs pass through", func(t *testing.T) {
		if got := filterRecommendationsByPeriod(domain.PeriodBlackSwan, nil, registry, advisor); got != nil {
			t.Errorf("nil recs should stay nil, got %v", got)
		}
	})
}

// ─── reserve cash varies with period (item 5) ───────────────────────────

func TestSystem_ApplyCharterReserveCash_VariesWithPeriod(t *testing.T) {
	constraints := domain.SimulationConstraints{
		StartingCash:                1_000_000,
		MaxPositionWeight:           0.20,
		MaxOpenPositions:            1,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 0,
		RequireCROPass:              true,
		TransactionCostBPS:          1,
		SlippageBPS:                 1,
		ReserveCashFraction:         0.1,
	}
	sys := &System{
		SystemCore: &SystemCore{
			sim: SimulationCore{engine: sim.NewEngine(constraints)},
		},
		charter: &charterConfig{advisor: methodology.NewAdvisor(nil)},
	}
	quotes := []domain.Quote{{Symbol: "2330.TW", Last: 800, Volume: 1_000_000, IsTradable: true}}
	recs := []domain.Recommendation{{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"}}

	run := func() domain.SimulationResult {
		return sys.Sim().engine.Run(domain.RegimeRiskOn, quotes, recs)
	}

	// black_swan → 90% reserve (advisor.CashReserve/100).
	sys.applyCharterReserveCash(periodPtr(domain.PeriodBlackSwan))
	bs := run()
	// consolidation → 35% reserve.
	sys.applyCharterReserveCash(periodPtr(domain.PeriodConsolidation))
	cons := run()
	// nil period → override cleared → base 0.1 (Phase A).
	sys.applyCharterReserveCash(nil)
	base := run()

	if !(bs.EndingCash > cons.EndingCash && cons.EndingCash > base.EndingCash) {
		t.Errorf("reserve should grow with period defensiveness: black_swan cash=%.2f > consolidation cash=%.2f > base cash=%.2f",
			bs.EndingCash, cons.EndingCash, base.EndingCash)
	}
	if len(base.Orders) == 0 {
		t.Fatal("base reserve should still deploy capital")
	}
	if len(bs.Orders) > 0 && bs.Orders[0].Quantity >= base.Orders[0].Quantity {
		t.Errorf("black_swan order quantity (%d) should be smaller than base (%d)",
			bs.Orders[0].Quantity, base.Orders[0].Quantity)
	}
}

//go:fix inline
func periodPtr(p domain.MarketPeriod) *domain.MarketPeriod { return new(p) }
