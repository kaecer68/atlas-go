package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func TestGrowthMomentumOverrideChangesRecommendations(t *testing.T) {
	registry := SeedRegistry()
	quotes := []domain.Quote{
		{
			Symbol:     "2317.TW",
			Open:       160.5,
			High:       161.5,
			Low:        158.5,
			Last:       159,
			Volume:     42506789,
			IsTradable: true,
		},
	}

	quoteBySymbol := map[string]domain.Quote{}
	for _, quote := range quotes {
		quoteBySymbol[quote.Symbol] = quote
	}
	plugins := NewPluginRegistry()

	baseline := collectRecommendations(registry, quoteBySymbol, plugins, map[string]string{
		"growth_momentum": "qualify candidates using trend persistence and volume confirmation",
	}, domain.RegimeNeutral)
	candidate := collectRecommendations(registry, quoteBySymbol, plugins, map[string]string{
		"growth_momentum": "require trend confirmation\ndowngrade conviction\nreject setups\n",
	}, domain.RegimeNeutral)

	baselineCount := countSkillRecommendations(baseline, "growth_momentum")
	candidateCount := countSkillRecommendations(candidate, "growth_momentum")
	if baselineCount == 0 {
		t.Fatalf("expected baseline growth momentum recommendations")
	}
	if candidateCount >= baselineCount {
		t.Fatalf("expected stricter override to reduce or remove growth momentum recommendations")
	}
}

func TestAdditionalPluginsAppearInRegistryExecution(t *testing.T) {
	registry := SeedRegistry()
	quotes := []domain.Quote{
		{Symbol: "2881.TW", Open: 68, High: 69, Low: 67.8, Last: 68.8, Volume: 6800000, IsTradable: true},
		{Symbol: "2882.TW", Open: 49, High: 49.3, Low: 48.8, Last: 49.1, Volume: 7200000, IsTradable: true},
		{Symbol: "2891.TW", Open: 31, High: 31.2, Low: 30.9, Last: 31.15, Volume: 5400000, IsTradable: true},
		{Symbol: "2603.TW", Open: 188, High: 192, Low: 187, Last: 191, Volume: 21000000, IsTradable: true},
		{Symbol: "2609.TW", Open: 245, High: 248, Low: 244, Last: 247, Volume: 17500000, IsTradable: true},
		{Symbol: "2615.TW", Open: 112, High: 113.5, Low: 111.2, Last: 113, Volume: 6200000, IsTradable: true},
		{Symbol: "2330.TW", Open: 998, High: 1008, Low: 994, Last: 1005, Volume: 32000000, IsTradable: true},
	}

	_, recs := ExecuteRegistryResearch(registry, quotes, nil)

	skills := map[string]bool{}
	for _, rec := range recs {
		skills[rec.Skill] = true
	}

	for _, skill := range []string{"cio_portfolio"} {
		if !skills[skill] {
			t.Fatalf("expected aggregated recommendation for skill %s", skill)
		}
	}
}

func TestExecuteRegistryResearchDetailedPreservesRawAgentSignals(t *testing.T) {
	registry := SeedRegistry()
	quotes := []domain.Quote{
		{Symbol: "2881.TW", Open: 68, High: 69, Low: 67.8, Last: 68.8, Volume: 6800000, IsTradable: true},
		{Symbol: "2603.TW", Open: 188, High: 192, Low: 187, Last: 191, Volume: 21000000, IsTradable: true},
	}

	_, raw, final := ExecuteRegistryResearchDetailed(registry, quotes, nil)
	if countSkillRecommendations(raw, "financials_desk") == 0 {
		t.Fatalf("expected raw financials desk recommendations")
	}
	if countSkillRecommendations(raw, "shipping_desk") == 0 {
		t.Fatalf("expected raw shipping desk recommendations")
	}
	if countSkillRecommendations(final, "cio_portfolio") == 0 {
		t.Fatalf("expected final CIO recommendations after control layer")
	}
}

func TestTechnicalBreakoutOverrideRejectsLowVolumeSetups(t *testing.T) {
	registry := SeedRegistry()
	quotes := []domain.Quote{
		{Symbol: "2317.TW", Open: 160, High: 161, Low: 159.8, Last: 160.8, Volume: 4200000, IsTradable: true},
	}

	quoteBySymbol := map[string]domain.Quote{}
	for _, quote := range quotes {
		quoteBySymbol[quote.Symbol] = quote
	}
	plugins := NewPluginRegistry()

	baseline := collectRecommendations(registry, quoteBySymbol, plugins, map[string]string{
		"technical_breakout": "require volume\nrequire close strength",
	}, domain.RegimeNeutral)
	candidate := collectRecommendations(registry, quoteBySymbol, plugins, map[string]string{
		"technical_breakout": "require volume\nrequire close strength\nreject low volume",
	}, domain.RegimeNeutral)

	baselineCount := countSkillRecommendations(baseline, "technical_breakout")
	candidateCount := countSkillRecommendations(candidate, "technical_breakout")
	if baselineCount == 0 {
		t.Fatalf("expected baseline technical breakout recommendations")
	}
	if candidateCount >= baselineCount {
		t.Fatalf("expected strict technical breakout override to reduce recommendations")
	}
}

func TestBuildReplayOutcomesUsesRecommendationAgentAndSkill(t *testing.T) {
	ds := &replay.Dataset{
		ByDate: map[string]map[string]domain.DailyBar{
			"2026-03-26": {
				"2881.TW": {Date: time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC), Symbol: "2881.TW", Close: 68},
			},
			"2026-03-27": {
				"2881.TW": {Date: time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC), Symbol: "2881.TW", Close: 69.36},
			},
		},
		Dates: []time.Time{
			time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
		},
	}

	recs := []domain.Recommendation{
		{
			Agent:      "financials-desk-01",
			Skill:      "financials_desk",
			Symbol:     "2881.TW",
			Side:       domain.SideBuy,
			Conviction: 60,
			Reason:     "test",
		},
	}
	outcomes := buildReplayOutcomes(recs, recs, nil, time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC), ds)

	if len(outcomes) != 1 {
		t.Fatalf("expected one outcome, got %d", len(outcomes))
	}
	if outcomes[0].AgentID != "financials-desk-01" {
		t.Fatalf("expected outcome agent id to come from recommendation, got %s", outcomes[0].AgentID)
	}
	if outcomes[0].Skill != "financials_desk" {
		t.Fatalf("expected outcome skill to come from recommendation, got %s", outcomes[0].Skill)
	}
}

func TestRegistrySymbolsIncludesNewSectorUniverses(t *testing.T) {
	registry := SeedRegistry()
	symbols := RegistrySymbols(registry)

	for _, want := range []string{"2881.TW", "2609.TW", "0050.TW"} {
		if !containsSymbol(symbols, want) {
			t.Fatalf("expected registry symbols to include %s", want)
		}
	}
}

func countSkillRecommendations(recs []domain.Recommendation, skill string) int {
	count := 0
	for _, rec := range recs {
		if rec.Skill == skill {
			count++
		}
	}
	return count
}

func containsSymbol(symbols []string, want string) bool {
	for _, symbol := range symbols {
		if symbol == want {
			return true
		}
	}
	return false
}
