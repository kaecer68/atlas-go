package narrative

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

func writeTestReplayCSV(dir string) (string, error) {
	symbols := map[string]float64{
		"2330":  1.02,
		"2317":  1.02,
		"2382":  1.02,
		"3231":  1.02,
		"2303":  1.02,
		"2308":  1.02,
		"2454":  1.02,
		"3034":  1.02,
		"3037":  1.02,
		"2357":  1.02,
		"2345":  1.02,
		"6669":  1.02,
		"2881":  1.001,
		"2882":  1.001,
		"2884":  1.001,
		"2885":  1.001,
		"2886":  1.001,
		"2891":  1.001,
		"2892":  1.001,
		"0056":  1.005,
		"00878": 1.005,
		"0050":  1.005,
		"2603":  0.99,
		"2609":  0.99,
		"2615":  0.99,
		"3008":  1.015,
		"3711":  1.015,
		"1301":  1.001,
		"1303":  1.001,
		"1326":  1.001,
		"1216":  1.001,
	}
	basePrices := map[string]float64{
		"2330": 500, "2317": 100, "2382": 200, "3231": 80,
		"2303": 50, "2308": 150, "2454": 300, "3034": 120,
		"3037": 180, "2357": 90, "2345": 70, "6669": 400,
		"2881": 25, "2882": 30, "2884": 20, "2885": 22,
		"2886": 28, "2891": 18, "2892": 32,
		"0056": 35, "00878": 20, "0050": 140,
		"2603": 60, "2609": 55, "2615": 45,
		"3008": 250, "3711": 180,
		"1301": 70, "1303": 65, "1326": 55, "1216": 90,
	}

	path := filepath.Join(dir, "replay.csv")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	fmt.Fprintln(f, "Date,Code,Name,TradeVolume,Open,High,Low,Close")
	startDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for day := 0; day < 45; day++ {
		date := startDate.Add(time.Duration(day) * 24 * time.Hour)
		dateStr := date.Format("2006-01-02")
		for code, multiplier := range symbols {
			base := basePrices[code]
			price := base
			for i := 0; i < day; i++ {
				price *= multiplier
			}
			open := price * 0.995
			high := price * 1.01
			low := price * 0.99
			close := price
			fmt.Fprintf(f, "%s,%s,%s,1000,%.2f,%.2f,%.2f,%.2f\n",
				dateStr, code, code, open, high, low, close)
		}
	}
	return path, nil
}

func TestEvaluateModels(t *testing.T) {
	csvPath, err := writeTestReplayCSV(t.TempDir())
	if err != nil {
		t.Fatalf("failed to write test csv: %v", err)
	}

	ne := NewNarrativeEngine()
	if err := ne.EvaluateModels(csvPath); err != nil {
		t.Fatalf("EvaluateModels failed: %v", err)
	}

	var allHalf bool = true
	var minErr, maxErr float64 = 1.0, 0.0
	for i, m := range ne.models {
		if m.RecentError != 0.5 {
			allHalf = false
		}
		if m.RecentError < minErr {
			minErr = m.RecentError
		}
		if m.RecentError > maxErr {
			maxErr = m.RecentError
		}
		t.Logf("model %d (%s): error=%.4f weight=%.4f", i, m.ID, m.RecentError, m.Weight)
	}

	if allHalf {
		t.Fatalf("all models have RecentError == 0.5 (fallback); expected differentiated errors")
	}

	if maxErr-minErr < 0.1 {
		t.Fatalf("expected at least 2 models to differ by > 0.1 in RecentError, got min=%.4f max=%.4f", minErr, maxErr)
	}

	var totalWeight float64
	for _, m := range ne.models {
		totalWeight += m.Weight
	}
	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Fatalf("expected weights to sum to ~1.0, got %f", totalWeight)
	}

	var distinctWeights int
	seen := make(map[float64]struct{})
	for _, m := range ne.models {
		if _, ok := seen[m.Weight]; !ok {
			seen[m.Weight] = struct{}{}
			distinctWeights++
		}
	}
	if distinctWeights < 2 {
		t.Fatalf("expected at least 2 models with different weights, got %d distinct", distinctWeights)
	}
}

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
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d", len(events))
	}

	themeCount := make(map[string]int)
	for _, e := range events {
		themeCount[e.Theme]++
	}
	expected := []string{"US_rates_up", "AI_capex_surge", "geopolitical_risk_spike", "oil_price_shock", "JPY_carry_unwind", "taiwan_political_risk"}
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
	config.ResetParametersConfig()
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
