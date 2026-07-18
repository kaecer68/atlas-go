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
