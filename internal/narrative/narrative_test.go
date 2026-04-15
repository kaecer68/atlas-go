package narrative

import (
	"testing"
	"time"
)

func TestKnowledgeBaseDefaults(t *testing.T) {
	kb := NewKnowledgeBase()
	templates := kb.ListTemplates()
	if len(templates) == 0 {
		t.Fatalf("expected default templates, got none")
	}

	tmpl, ok := kb.GetTemplate("美國升息 / 鷹派聯準會")
	if !ok {
		t.Fatalf("expected 美國升息 / 鷹派聯準會 template")
	}
	if tmpl.HistoricalHitRate <= 0 {
		t.Fatalf("expected positive hit rate")
	}
}

func TestKnowledgeBaseRegisterAndMatch(t *testing.T) {
	kb := NewKnowledgeBase()
	custom := CausalTemplate{
		ID:             "custom_test",
		Name:           "Custom Test",
		TriggerTheme:   "TEST_THEME",
		RequiredRegion: "Asia",
		Steps: []CausalStep{
			{Description: "step1", Affected: []string{"sectorA"}, Impact: 0.5},
		},
		HistoricalHitRate: 0.9,
	}
	kb.RegisterTemplate(custom)

	event := NarrativeEvent{
		ID:         "evt-1",
		Theme:      "TEST_THEME",
		Region:     "Asia",
		Confidence: 0.8,
	}
	chains := kb.MatchChains(event)
	if len(chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(chains))
	}
	expectedScore := 0.8 * 0.9
	if chains[0].Score < expectedScore-1e-9 || chains[0].Score > expectedScore+1e-9 {
		t.Fatalf("expected score %f, got %f", expectedScore, chains[0].Score)
	}

	// Mismatch region should produce no chains.
	event2 := NarrativeEvent{
		ID:         "evt-2",
		Theme:      "TEST_THEME",
		Region:     "US",
		Confidence: 0.8,
	}
	chains2 := kb.MatchChains(event2)
	if len(chains2) != 0 {
		t.Fatalf("expected 0 chains for region mismatch, got %d", len(chains2))
	}
}

func TestNarrativeEngineDetectEvents(t *testing.T) {
	nowUnix = func() int64 { return 123456789 }
	defer func() { nowUnix = func() int64 { return time.Now().UnixNano() } }()

	ne := NewNarrativeEngine()
	data := MarketNarrativeData{
		US10YChangeBps:   15,
		DXYChangePct:     2.0,
		AICapexSentiment: 0.8,
		GeopoliticalGPR:  160,
		OilChangePct:     6.0,
		JPY_ChangePct:    3.0,
		VIXLevel:         30,
	}
	events := ne.DetectEvents(data)
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	themeCount := make(map[string]int)
	for _, e := range events {
		themeCount[e.Theme]++
	}
	expected := []string{"US_rates_up", "AI_capex_surge", "geopolitical_risk_spike", "oil_price_shock", "JPY_carry_unwind"}
	for _, theme := range expected {
		if themeCount[theme] != 1 {
			t.Fatalf("expected 1 event for theme %s, got %d", theme, themeCount[theme])
		}
	}
}

func TestNarrativeEngineMatchChains(t *testing.T) {
	ne := NewNarrativeEngine()
	events := []NarrativeEvent{
		{ID: "evt-1", Theme: "US_rates_up", Region: "US", Confidence: 0.8},
		{ID: "evt-2", Theme: "AI_capex_surge", Region: "US", Confidence: 0.7},
	}
	chains := ne.MatchChains(events)
	if len(chains) != 2 {
		t.Fatalf("expected 2 chains, got %d", len(chains))
	}
}

func TestNarrativeEngineActiveModels(t *testing.T) {
	ne := NewNarrativeEngine()
	themes := []string{"US_rates_up"}
	models := ne.ActiveModels(themes)
	if len(models) != 1 {
		t.Fatalf("expected 1 active model, got %d", len(models))
	}
	if models[0].ID != "hawkish_fed_model" {
		t.Fatalf("unexpected model id: %s", models[0].ID)
	}
}

func TestNarrativeEngineUpdateModelWeights(t *testing.T) {
	ne := NewNarrativeEngine()
	ne.models[0].RecentError = 0.1
	ne.models[1].RecentError = 0.2
	ne.models[2].RecentError = 0.3

	ne.UpdateModelWeights()

	var total float64
	for _, m := range ne.models {
		total += m.Weight
	}
	if total < 0.99 || total > 1.01 {
		t.Fatalf("expected weights to sum to ~1.0, got %f", total)
	}

	if ne.models[0].Weight <= ne.models[1].Weight || ne.models[1].Weight <= ne.models[2].Weight {
		t.Fatalf("expected inverse-error ordering, got %v", []float64{ne.models[0].Weight, ne.models[1].Weight, ne.models[2].Weight})
	}
}

func TestDetectEventsNoTrigger(t *testing.T) {
	ne := NewNarrativeEngine()
	data := MarketNarrativeData{
		US10YChangeBps:   5,   // below 10bps threshold
		AICapexSentiment: 0.2, // below 0.5 threshold
		GeopoliticalGPR:  100, // below 150 threshold
		OilChangePct:     2.0, // below 5% threshold
		JPY_ChangePct:    1.0, // below 2% threshold
		VIXLevel:         15,  // below 25 threshold
	}
	events := ne.DetectEvents(data)
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}
