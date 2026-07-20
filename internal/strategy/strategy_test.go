package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestRiskAppetiteConstants(t *testing.T) {
	if RiskAppetiteConservative != 1 {
		t.Errorf("RiskAppetiteConservative = %d, want 1", RiskAppetiteConservative)
	}
	if RiskAppetiteBalanced != 2 {
		t.Errorf("RiskAppetiteBalanced = %d, want 2", RiskAppetiteBalanced)
	}
	if RiskAppetiteAggressive != 3 {
		t.Errorf("RiskAppetiteAggressive = %d, want 3", RiskAppetiteAggressive)
	}
}

func TestStrategyCreation(t *testing.T) {
	s := &Strategy{
		ID:           "test",
		Name:         "Test Strategy",
		Description:  "Test description",
		Enabled:      true,
		Agents:       []string{"agent1", "agent2"},
		Filters:      []string{"filter1"},
		Priority:     10,
		RiskAppetite: RiskAppetiteBalanced,
		RegimePrefs:  []domain.Regime{domain.RegimeRiskOn, domain.RegimeNeutral},
	}
	if s.ID != "test" {
		t.Errorf("Strategy ID = %s, want test", s.ID)
	}
	if len(s.Agents) != 2 {
		t.Errorf("len(Agents) = %d, want 2", len(s.Agents))
	}
	if len(s.RegimePrefs) != 2 {
		t.Errorf("len(RegimePrefs) = %d, want 2", len(s.RegimePrefs))
	}
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.strategies == nil {
		t.Error("strategies map not initialized")
	}
	if len(r.List()) != 0 {
		t.Errorf("len(List()) = %d, want 0", len(r.List()))
	}
}

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry()
	s := &Strategy{ID: "test", Name: "Test"}

	err := r.Register(s)
	if err != nil {
		t.Errorf("Register failed: %v", err)
	}

	got, ok := r.Get("test")
	if !ok {
		t.Error("Get returned false, want true")
	}
	if got.ID != "test" {
		t.Errorf("got.ID = %s, want test", got.ID)
	}
}

func TestRegistryRegisterEmptyID(t *testing.T) {
	r := NewRegistry()
	err := r.Register(&Strategy{ID: "", Name: "Test"})
	if err == nil {
		t.Error("Register with empty ID should return error")
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(&Strategy{ID: "s1", Name: "S1"})
	r.Register(&Strategy{ID: "s2", Name: "S2"})

	list := r.List()
	if len(list) != 2 {
		t.Errorf("len(List()) = %d, want 2", len(list))
	}
}

func TestRegistryListByRegime(t *testing.T) {
	r := NewRegistry()
	r.Register(&Strategy{
		ID:          "risk_on",
		RegimePrefs: []domain.Regime{domain.RegimeRiskOn},
		Enabled:     true,
	})
	r.Register(&Strategy{
		ID:          "risk_off",
		RegimePrefs: []domain.Regime{domain.RegimeRiskOff},
		Enabled:     true,
	})
	r.Register(&Strategy{
		ID:          "disabled",
		RegimePrefs: []domain.Regime{domain.RegimeRiskOn},
		Enabled:     false,
	})

	list := r.ListByRegime(domain.RegimeRiskOn)
	if len(list) != 1 {
		t.Errorf("len(ListByRegime(risk_on)) = %d, want 1", len(list))
	}
}

func TestNewRegistryWithDefaults(t *testing.T) {
	r := NewRegistryWithDefaults()
	list := r.List()

	if len(list) != 5 {
		t.Errorf("len(List()) = %d, want 5", len(list))
	}

	expected := map[string]bool{
		"all_weather": true,
		"growth":      true,
		"value":       true,
		"defensive":   true,
		"momentum":    true,
	}
	for _, s := range list {
		if !expected[s.ID] {
			t.Errorf("unexpected strategy: %s", s.ID)
		}
	}

	aw, ok := r.Get("all_weather")
	if !ok {
		t.Fatal("all_weather not found")
	}
	if len(aw.Agents) != 1 || aw.Agents[0] != "*" {
		t.Errorf("all_weather agents = %v, want [*]", aw.Agents)
	}
}

func TestNewComparisonEngine(t *testing.T) {
	e := NewComparisonEngine(20, nil)
	if e == nil {
		t.Fatal("NewComparisonEngine returned nil")
	}
	if e.window != 20 {
		t.Errorf("window = %d, want 20", e.window)
	}
	if e.trades == nil {
		t.Error("trades map not initialized")
	}
}

func TestComparisonEngineRecord(t *testing.T) {
	e := NewComparisonEngine(30, nil)

	trades := []*Trade{
		{StrategyID: "test", Date: time.Now(), Return: 0.05},
		{StrategyID: "test", Date: time.Now(), Return: 0.03},
	}
	e.Record(trades, 0.02)

	if len(e.trades["test"]) != 2 {
		t.Errorf("len(trades[test]) = %d, want 2", len(e.trades["test"]))
	}
}

func TestComparisonEngineRecordEmptyTrades(t *testing.T) {
	e := NewComparisonEngine(30, nil)
	e.Record([]*Trade{}, 0.02)

	if len(e.trades) != 0 {
		t.Errorf("len(trades) = %d, want 0", len(e.trades))
	}
}

func TestComparisonEnginePruneOldTrades(t *testing.T) {
	e := NewComparisonEngine(7, nil)
	now := time.Now()

	e.trades["recent"] = []*Trade{
		{StrategyID: "recent", Date: now.AddDate(0, 0, -1), Return: 0.01},
	}
	e.trades["old"] = []*Trade{
		{StrategyID: "old", Date: now.AddDate(0, 0, -30), Return: 0.01},
	}

	e.pruneOldTrades(now)

	if len(e.trades["recent"]) != 1 {
		t.Errorf("len(trades[recent]) = %d, want 1", len(e.trades["recent"]))
	}
	if _, ok := e.trades["old"]; ok {
		t.Error("old trades should be pruned")
	}
}

func TestComparisonEngineBestStrategy(t *testing.T) {
	e := NewComparisonEngine(20, nil)

	best, err := e.BestStrategy("return")
	if err != nil {
		t.Errorf("BestStrategy failed: %v", err)
	}
	if best != "" {
		t.Errorf("best = %s, want empty string when no history", best)
	}
}

func TestComparisonEngineBestStrategyInvalid(t *testing.T) {
	e := NewComparisonEngine(20, nil)

	e.history = append(e.history, &ComparisonResult{})

	_, err := e.BestStrategy("invalid")
	if err == nil {
		t.Error("BestStrategy with invalid criteria should return error")
	}
}

func TestComparisonEngineGetScore(t *testing.T) {
	e := NewComparisonEngine(20, nil)

	e.history = append(e.history, &ComparisonResult{
		Date: "2026-04-28",
		Comparisons: []*StrategyComparison{
			{StrategyID: "test", SharpeRatio: 0.8, DailyReturn: 0.02, WinRate: 0.6},
		},
	})

	score, err := e.GetScore("test", 1)
	if err != nil {
		t.Errorf("GetScore failed: %v", err)
	}
	if score <= 0 {
		t.Errorf("score = %f, want > 0", score)
	}
}

func TestComparisonEngineGetScoreNoHistory(t *testing.T) {
	e := NewComparisonEngine(20, nil)

	score, err := e.GetScore("test", 10)
	if err != nil {
		t.Errorf("GetScore failed: %v", err)
	}
	if score != 0.5 {
		t.Errorf("score = %f, want 0.5 when no history", score)
	}
}

func TestNewSelector(t *testing.T) {
	r := NewRegistry()
	e := NewComparisonEngine(20, nil)
	s := NewSelector(r, e)

	if s == nil {
		t.Fatal("NewSelector returned nil")
	}
	if s.registry != r {
		t.Error("registry not set correctly")
	}
	if s.comparison != e {
		t.Error("comparison not set correctly")
	}
	if s.current != nil {
		t.Error("current should be nil initially")
	}
	// Default MinSwitchInterval comes from config (7 days)
	expectedInterval := time.Duration(config.GetParametersConfig().Strategy.MinSwitchIntervalDays.Value) * 24 * time.Hour
	if s.config.MinSwitchInterval != expectedInterval {
		t.Errorf("MinSwitchInterval = %v, want %v", s.config.MinSwitchInterval, expectedInterval)
	}
}

func TestSelectorSelect(t *testing.T) {
	r := NewRegistryWithDefaults()
	e := NewComparisonEngine(20, nil)
	s := NewSelector(r, e)

	ctx := context.Background()
	strat, err := s.Select(ctx, 20.0, domain.RegimeRiskOn)
	if err != nil {
		t.Errorf("Select failed: %v", err)
	}
	if strat == nil {
		t.Error("Select returned nil strategy")
	}
}

func TestSelectorSelectFallback(t *testing.T) {
	r := NewRegistry()
	r.Register(&Strategy{
		ID:          "specific",
		RegimePrefs: []domain.Regime{domain.RegimeRiskOff},
		Enabled:     true,
	})
	e := NewComparisonEngine(20, nil)
	s := NewSelector(r, e)

	ctx := context.Background()
	strat, err := s.Select(ctx, 20.0, domain.RegimeRiskOn)
	if err != nil {
		t.Errorf("Select failed: %v", err)
	}
	if strat == nil {
		t.Error("Select returned nil strategy")
		return
	}
	if strat.ID != "all_weather" && strat.ID != "fallback" {
		t.Errorf("Expected all_weather or fallback, got %s", strat.ID)
	}
}

func TestSelectorGetCurrentStrategy(t *testing.T) {
	r := NewRegistryWithDefaults()
	e := NewComparisonEngine(20, nil)
	s := NewSelector(r, e)

	current := s.GetCurrentStrategy()
	if current != nil {
		t.Error("GetCurrentStrategy should return nil when no strategy selected")
	}

	ctx := context.Background()
	s.Select(ctx, 20.0, domain.RegimeRiskOn)

	current = s.GetCurrentStrategy()
	if current == nil {
		t.Error("GetCurrentStrategy should return strategy after Select")
	}
}

func TestSelectorShouldSwitch(t *testing.T) {
	r := NewRegistryWithDefaults()
	e := NewComparisonEngine(20, nil)
	s := NewSelector(r, e)

	old := &Strategy{ID: "old", Priority: 10}
	new := &Strategy{ID: "new", Priority: 20}

	if s.shouldSwitch(old, new, 0.05) {
		t.Error("shouldSwitch should return false when delta < threshold")
	}

	if !s.shouldSwitch(old, new, 0.15) {
		t.Error("shouldSwitch should return true when delta >= threshold")
	}

	s.lastSwitch = time.Now()
	if s.shouldSwitch(old, new, 0.15) {
		t.Error("shouldSwitch should return false within MinSwitchInterval")
	}

	if s.shouldSwitch(old, old, 0.15) {
		t.Error("shouldSwitch should return false for same ID")
	}
}

func TestSelectorStickiness(t *testing.T) {
	r := NewRegistryWithDefaults()
	e := NewComparisonEngine(20, nil)
	s := NewSelector(r, e)

	ctx := context.Background()

	s.Select(ctx, 20.0, domain.RegimeRiskOn)
	first := s.GetCurrentStrategy()

	s.Select(ctx, 20.0, domain.RegimeRiskOn)
	second := s.GetCurrentStrategy()

	if first.ID != second.ID {
		t.Errorf("Selector should stick to current strategy, got %s then %s", first.ID, second.ID)
	}
}

func TestComparisonEngineNewWithStore(t *testing.T) {
	store := NewFileComparisonStore("", 0)
	e := NewComparisonEngine(20, store)
	if e == nil {
		t.Fatal("NewComparisonEngine with store returned nil")
	}
	if e.shadowStore != store {
		t.Error("shadowStore not set")
	}
}

func TestComparisonEngineRecordShadowDay(t *testing.T) {
	e := NewComparisonEngine(100, nil)

	day := ComparisonDay{
		TradingDate: "2026-07-20",
		Benchmark: BenchmarkObservation{
			TradingDate: "2026-07-20",
			SourceID:    "TAIEX",
			ReasonCode:  "test",
			Return:      0.01,
			Available:   true,
		},
		Observations: []StrategyDailyObservation{
			{StrategyID: "growth", DailyReturn: 0.02, Outperformance: 0.01},
			{StrategyID: "value", DailyReturn: -0.01, Outperformance: -0.02},
		},
	}

	if err := e.RecordShadowDay(day); err != nil {
		t.Fatalf("RecordShadowDay failed: %v", err)
	}

	if len(e.shadowDays) != 1 {
		t.Fatalf("shadowDays = %d, want 1", len(e.shadowDays))
	}
	if e.shadowDays[0].TradingDate != "2026-07-20" {
		t.Errorf("TradingDate = %s, want 2026-07-20", e.shadowDays[0].TradingDate)
	}
}

func TestComparisonEngineRecordShadowDayUnavailableBenchmark(t *testing.T) {
	e := NewComparisonEngine(100, nil)

	day := ComparisonDay{
		TradingDate: "2026-07-20",
		Benchmark: BenchmarkObservation{
			Available: false,
		},
	}

	if err := e.RecordShadowDay(day); err != nil {
		t.Fatalf("RecordShadowDay should not error on unavailable benchmark: %v", err)
	}
	if len(e.shadowDays) != 0 {
		t.Error("shadowDays should be empty when benchmark unavailable")
	}
}

func TestComparisonEngineRecordShadowDayReplace(t *testing.T) {
	e := NewComparisonEngine(100, nil)

	e.RecordShadowDay(ComparisonDay{
		TradingDate: "2026-07-20",
		Benchmark:   BenchmarkObservation{Available: true},
		Observations: []StrategyDailyObservation{
			{StrategyID: "old", DailyReturn: 0.01},
		},
	})

	e.RecordShadowDay(ComparisonDay{
		TradingDate: "2026-07-20",
		Benchmark:   BenchmarkObservation{Available: true},
		Observations: []StrategyDailyObservation{
			{StrategyID: "new", DailyReturn: 0.02},
		},
	})

	if len(e.shadowDays) != 1 {
		t.Fatalf("shadowDays = %d, want 1 after replace", len(e.shadowDays))
	}
	if len(e.shadowDays[0].Observations) != 1 || e.shadowDays[0].Observations[0].StrategyID != "new" {
		t.Error("RecordShadowDay did not replace existing entry")
	}
}

func TestRankingSnapshot_WarmingUp(t *testing.T) {
	e := NewComparisonEngine(100, nil)

	snap := e.RankingSnapshot(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC))
	if snap.WarmingUp.Status != "warming_up" {
		t.Errorf("Status = %s, want warming_up", snap.WarmingUp.Status)
	}
	if snap.WarmingUp.ReasonCode != "no_history" {
		t.Errorf("ReasonCode = %s, want no_history", snap.WarmingUp.ReasonCode)
	}
	if len(snap.Ranked) != 0 {
		t.Errorf("len(Ranked) = %d, want 0", len(snap.Ranked))
	}
}

func TestRankingSnapshot_WithData(t *testing.T) {
	e := NewComparisonEngine(100, nil)

	dates := []string{
		"2026-07-01", "2026-07-02", "2026-07-03", "2026-07-04", "2026-07-05",
		"2026-07-06", "2026-07-07", "2026-07-08", "2026-07-09", "2026-07-10",
		"2026-07-11", "2026-07-12", "2026-07-13", "2026-07-14", "2026-07-15",
		"2026-07-16", "2026-07-17", "2026-07-18", "2026-07-19", "2026-07-20",
	}
	for _, d := range dates {
		e.shadowDays = append(e.shadowDays, ComparisonDay{
			TradingDate: d,
			Benchmark:   BenchmarkObservation{Available: true},
			Observations: []StrategyDailyObservation{
				{StrategyID: "growth", EvaluationMode: EvaluationModeShadow, DailyReturn: 0.02, Outperformance: 0.01},
				{StrategyID: "value", EvaluationMode: EvaluationModeShadow, DailyReturn: 0.01, Outperformance: 0.005},
			},
			DeployedMix: map[string]float64{"growth": 0.6, "value": 0.4},
		})
	}

	snap := e.RankingSnapshot(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC))

	if snap.WarmingUp.Status != "eligible" {
		t.Errorf("Status = %s, want eligible", snap.WarmingUp.Status)
	}
	if len(snap.Ranked) != 2 {
		t.Fatalf("len(Ranked) = %d, want 2", len(snap.Ranked))
	}
	if snap.Ranked[0].Score < snap.Ranked[1].Score {
		t.Error("Ranked should be sorted by Score descending")
	}
	if len(snap.DeployedMix) != 2 {
		t.Errorf("len(DeployedMix) = %d, want 2", len(snap.DeployedMix))
	}
}

func TestRankedIDs_WarmingUp(t *testing.T) {
	e := NewComparisonEngine(100, nil)

	ids, err := e.RankedIDs()
	if err != nil {
		t.Fatalf("RankedIDs: %v", err)
	}
	if ids != nil {
		t.Errorf("ids = %v, want nil when warming up", ids)
	}
}

func TestRankedIDs_Eligible(t *testing.T) {
	e := NewComparisonEngine(20, nil)

	for _, d := range []string{
		"2026-07-01", "2026-07-02", "2026-07-03", "2026-07-04", "2026-07-05",
		"2026-07-06", "2026-07-07", "2026-07-08", "2026-07-09", "2026-07-10",
		"2026-07-11", "2026-07-12", "2026-07-13", "2026-07-14", "2026-07-15",
		"2026-07-16", "2026-07-17", "2026-07-18", "2026-07-19", "2026-07-20",
	} {
		e.shadowDays = append(e.shadowDays, ComparisonDay{
			TradingDate: d,
			Benchmark:   BenchmarkObservation{Available: true},
			Observations: []StrategyDailyObservation{
				{StrategyID: "growth", EvaluationMode: EvaluationModeShadow, DailyReturn: 0.02, Outperformance: 0.01},
				{StrategyID: "value", EvaluationMode: EvaluationModeShadow, DailyReturn: 0.01, Outperformance: 0.005},
			},
		})
	}

	ids, err := e.RankedIDs()
	if err != nil {
		t.Fatalf("RankedIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len(ids) = %d, want 2", len(ids))
	}
}

func TestComparisonEngineGetResult(t *testing.T) {
	e := NewComparisonEngine(20, nil)
	now := time.Now()

	e.history = append(e.history, &ComparisonResult{
		Date: now.Format("2006-01-02"),
	})

	result, ok := e.GetResult(now)
	if !ok {
		t.Error("GetResult returned false, want true")
	}
	if result == nil {
		t.Error("GetResult returned nil")
	}

	_, ok = e.GetResult(time.Now().AddDate(0, 0, -100))
	if ok {
		t.Error("GetResult should return false for non-existent date")
	}
}
