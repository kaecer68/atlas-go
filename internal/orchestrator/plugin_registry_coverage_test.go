package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/screener"
)

func TestPortfolioRotator_Rotate_NilOrEmpty(t *testing.T) {
	var nilRotator *PortfolioRotator
	if got := nilRotator.Rotate(nil, nil, domain.AgentSpec{}, "", domain.RegimeNeutral, nil); got != nil {
		t.Error("nil Rotate should return nil")
	}

	rotator := NewPortfolioRotator()
	if got := rotator.Rotate(nil, nil, domain.AgentSpec{}, "", domain.RegimeNeutral, nil); got != nil {
		t.Error("empty rotator should return nil")
	}
}

type mockPositionEvaluator struct {
	supports   func(domain.AgentSpec) bool
	evalResult domain.Recommendation
	evalOK     bool
}

func (m *mockPositionEvaluator) Supports(agent domain.AgentSpec) bool {
	return m.supports == nil || m.supports(agent)
}

func (m *mockPositionEvaluator) EvaluatePosition(pos domain.Position, quote domain.Quote, agent domain.AgentSpec, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	return m.evalResult, m.evalOK
}

type mockFactorQueryCoverage struct{}

func (m *mockFactorQueryCoverage) GetScore(string, portfolio.FactorType) (float64, bool) {
	return 0, false
}

func TestPortfolioRotator_Rotate(t *testing.T) {
	eval := &mockPositionEvaluator{
		evalResult: domain.Recommendation{Symbol: "2330.TW", Side: domain.SideSell, Conviction: 80},
		evalOK:     true,
	}
	rotator := NewPortfolioRotator(eval)

	positions := []domain.Position{{Symbol: "2330.TW", MarketValue: 100000}}
	quotes := map[string]domain.Quote{"2330.TW": {Symbol: "2330.TW", Last: 600, IsTradable: true}}
	recs := rotator.Rotate(positions, quotes, domain.AgentSpec{ID: "test", Layer: domain.LayerSector}, "", domain.RegimeNeutral, &mockFactorQueryCoverage{})
	if len(recs) != 1 || recs[0].Symbol != "2330.TW" {
		t.Errorf("expected SELL rec for 2330.TW, got %v", recs)
	}
}

func TestPortfolioRotator_Rotate_SkipsNonTradable(t *testing.T) {
	eval := &mockPositionEvaluator{evalOK: true}
	rotator := NewPortfolioRotator(eval)
	positions := []domain.Position{{Symbol: "2330.TW"}}
	quotes := map[string]domain.Quote{"2330.TW": {Symbol: "2330.TW", IsTradable: false}}
	recs := rotator.Rotate(positions, quotes, domain.AgentSpec{}, "", domain.RegimeNeutral, nil)
	if len(recs) != 0 {
		t.Error("expected no recs for non-tradable symbol")
	}
}

func TestPortfolioRotator_RotatePortfolio_NilOrEmpty(t *testing.T) {
	var nilRotator *PortfolioRotator
	if got := nilRotator.RotatePortfolio(nil, nil, nil, domain.AgentRegistry{}, nil, nil, domain.RegimeNeutral, nil); got != nil {
		t.Error("nil RotatePortfolio should return nil")
	}

	eval := &mockPositionEvaluator{evalOK: true}
	rotator := NewPortfolioRotator(eval)
	if got := rotator.RotatePortfolio(nil, nil, nil, domain.AgentRegistry{}, nil, nil, domain.RegimeNeutral, nil); got != nil {
		t.Error("empty positions/buyRecs should return nil")
	}
}

func TestPortfolioRotator_RotatePortfolio(t *testing.T) {
	eval := &mockPositionEvaluator{
		supports:   func(a domain.AgentSpec) bool { return a.Layer != domain.LayerControl && a.Layer != domain.LayerContext },
		evalResult: domain.Recommendation{Symbol: "2330.TW", Side: domain.SideSell, Conviction: 80},
		evalOK:     true,
	}
	rotator := NewPortfolioRotator(eval)
	registry := domain.AgentRegistry{Agents: []domain.AgentSpec{{ID: "sec", Layer: domain.LayerSector, Enabled: true}}}
	positions := []domain.Position{{Symbol: "2330.TW", MarketValue: 100000}}
	buyRecs := []domain.Recommendation{{Agent: "best", Skill: "growth", Symbol: "2454.TW", Conviction: 90, Layer: domain.LayerStyle}}
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Last: 600, IsTradable: true},
		"2454.TW": {Symbol: "2454.TW", Last: 500, IsTradable: true},
	}
	recs := rotator.RotatePortfolio(positions, buyRecs, quotes, registry, NewPluginRegistry(), nil, domain.RegimeNeutral, &mockFactorQueryCoverage{})
	if len(recs) != 1 || recs[0].Side != domain.SideSell {
		t.Errorf("expected rotation SELL rec, got %v", recs)
	}
}

func TestPluginRegistry_IsAgentHealthy(t *testing.T) {
	reg := NewPluginRegistry()
	if !reg.IsAgentHealthy("any") {
		t.Error("expected healthy when no health manager")
	}

	hm := portfolio.NewAgentHealthManager()
	reg.WithAgentHealthManager(hm)
	if !reg.IsAgentHealthy("any") {
		t.Error("expected healthy with fresh health manager")
	}
}

func TestPluginRegistry_CalculateFactorScores(t *testing.T) {
	reg := NewPluginRegistry()
	if got := reg.CalculateFactorScores("2330.TW", nil, nil, nil); got != nil {
		t.Error("expected nil scores when factor engine missing")
	}

	cfg := productionSystemConfig(t)
	fe, _, _ := buildFactorEngine(loadRuntimeParamsOrDefault(cfg.ParametersConfigPath), &marketdata.MacroDataSnapshot{}, cfg.ReplayDataPath)
	reg.WithFactorEngine(fe)
	quotes := map[string]domain.Quote{"2330.TW": {Symbol: "2330.TW", Last: 600, Open: 590, Volume: 1_000_000}}
	scores := reg.CalculateFactorScores("2330.TW", quotes, nil, nil)
	if scores == nil {
		t.Fatal("expected non-nil scores")
	}
}

func TestPluginRegistry_CalculateFactorScoresWithBreakdown(t *testing.T) {
	reg := NewPluginRegistry()
	bd, scores := reg.CalculateFactorScoresWithBreakdown("2330.TW", nil, nil, nil)
	if bd != nil || scores != nil {
		t.Error("expected nil when factor engine missing")
	}

	cfg := productionSystemConfig(t)
	fe, _, _ := buildFactorEngine(loadRuntimeParamsOrDefault(cfg.ParametersConfigPath), &marketdata.MacroDataSnapshot{}, cfg.ReplayDataPath)
	reg.WithFactorEngine(fe)
	quotes := map[string]domain.Quote{"2330.TW": {Symbol: "2330.TW", Last: 600, Open: 590, Volume: 1_000_000}}
	_, scores = reg.CalculateFactorScoresWithBreakdown("2330.TW", quotes, nil, nil)
	if scores == nil {
		t.Fatal("expected non-nil scores")
	}
}

func TestPluginRegistry_Screen(t *testing.T) {
	reg := NewPluginRegistry()
	agent := domain.AgentSpec{ScreeningCriteria: domain.ScreeningCriteria{VolumeIntraday: &domain.MinFilter{Min: int64Ptr(1_000_000)}}}
	quotes := map[string]domain.Quote{"2330.TW": {Symbol: "2330.TW", Volume: 2_000_000}}

	passed, err := reg.Screen(context.Background(), agent, "2330.TW", quotes)
	if err != nil || !passed {
		t.Errorf("expected pass without screener, got %v, err %v", passed, err)
	}

	fe := portfolio.NewFactorEngine()
	fp := portfolio.NewFundamentalProvider()
	reg.WithScreener(screener.NewEngine(fe, fp))
	passed, err = reg.Screen(context.Background(), agent, "2330.TW", quotes)
	if err != nil || !passed {
		t.Errorf("expected pass with screener, got %v, err %v", passed, err)
	}

	agentNoFilters := domain.AgentSpec{}
	passed, err = reg.Screen(context.Background(), agentNoFilters, "2330.TW", quotes)
	if err != nil || !passed {
		t.Errorf("expected pass without filters, got %v, err %v", passed, err)
	}
}

func TestPluginRegistry_WithCapitalFlowReportProvider(t *testing.T) {
	reg := NewPluginRegistry()
	// nil provider is a no-op.
	reg.WithCapitalFlowReportProvider(nil)

	provider := mockCapitalFlowReportProvider{report: marketPassReport()}
	reg.WithCapitalFlowReportProvider(provider)

	found := false
	for _, exec := range reg.agentExecutors {
		e, ok := exec.(StockpickerWinrateExecutor)
		if !ok {
			continue
		}
		found = true
		if e.CapitalFlow == nil {
			t.Fatal("WithCapitalFlowReportProvider did not inject CapitalFlow into StockpickerWinrateExecutor")
		}
		if _, err := e.CapitalFlow.LatestDaily(context.Background()); err != nil {
			t.Fatalf("injected provider returned error: %v", err)
		}
	}
	if !found {
		t.Fatal("StockpickerWinrateExecutor not found in builtin agent executors")
	}
}

func TestPluginRegistry_Rotator(t *testing.T) {
	reg := NewPluginRegistry()
	if reg.Rotator() != nil {
		t.Error("expected nil rotator initially")
	}
	eval := &mockPositionEvaluator{}
	reg.RegisterPositionEvaluators(eval)
	if reg.Rotator() == nil {
		t.Error("expected rotator after registration")
	}
}

func TestPluginRegistry_WithSetters(t *testing.T) {
	reg := NewPluginRegistry()
	reg.WithAgentHealthManager(portfolio.NewAgentHealthManager())
	reg.WithCycleModulator(&IndustryCycleModulator{})
	reg.WithNarrativeModulator(&NarrativeConvictionModulator{})
	reg.WithMLScorer(NewMLScorer(nil))
	reg.WithHeldPositions([]domain.Position{{Symbol: "2330.TW"}})
	reg.WithRecOverrides(map[string]string{"a:2330": "approved"})
}

func TestPluginRegistry_ResolvePrompt(t *testing.T) {
	reg := NewPluginRegistry()
	agent := domain.AgentSpec{ID: "a1", Skill: "skill1"}
	if got := reg.ResolvePrompt(agent, map[string]string{"a1": "override"}); got != "override" {
		t.Errorf("expected agent override, got %q", got)
	}
	if got := reg.ResolvePrompt(agent, map[string]string{"skill1": "skill-override"}); got != "skill-override" {
		t.Errorf("expected skill override, got %q", got)
	}
}

func TestFileSystemPromptResolver(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "test.md")
	_ = os.WriteFile(promptPath, []byte("HELLO WORLD"), 0o644)

	resolver := NewFileSystemPromptResolver(dir)
	got, err := resolver.Resolve(domain.AgentSpec{PromptFile: "test.md"})
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("expected lowercase prompt, got %q", got)
	}

	_, err = resolver.Resolve(domain.AgentSpec{PromptFile: "missing.md"})
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestPluginRegistry_WithPromptResolver(t *testing.T) {
	reg := NewPluginRegistry()
	resolver := NewFileSystemPromptResolver("")
	reg.WithPromptResolver(resolver)
	if reg.promptResolver != resolver {
		t.Error("WithPromptResolver did not assign resolver")
	}
}

func TestPluginRegistry_WireScreenerTraceWriter(t *testing.T) {
	reg := NewPluginRegistry()
	reg.WireScreenerTraceWriter(nil) // nil screener no-op

	fe := portfolio.NewFactorEngine()
	fp := portfolio.NewFundamentalProvider()
	reg.WithScreener(screener.NewEngine(fe, fp))
	tw := NewSimTraceWriter(t.TempDir(), "test", false)
	reg.WireScreenerTraceWriter(tw)
}
