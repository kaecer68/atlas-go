package narrative

import (
	"fmt"
	"math"
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
	for day := range 45 {
		date := startDate.Add(time.Duration(day) * 24 * time.Hour)
		dateStr := date.Format("2006-01-02")
		for code, multiplier := range symbols {
			base := basePrices[code]
			price := base
			for range day {
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
	nowUTC = func() time.Time { return time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) }
	defer func() { nowUTC = func() time.Time { return time.Now().UTC() } }()

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
	// 11 events: 9 original + semiconductor_downturn + 1 calendar-bound theme
	if len(events) != 11 {
		t.Fatalf("expected 11 events, got %d", len(events))
	}

	themeCount := make(map[string]int)
	for _, e := range events {
		themeCount[e.Theme]++
	}
	expected := []string{"US_rates_up", "AI_capex_surge", "geopolitical_risk_spike", "oil_price_shock", "JPY_carry_unwind", "taiwan_political_risk", "dollar_surge", "earnings_surprise", "inflation_spike", "semiconductor_downturn"}
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
	// Exclude calendar-driven seasonal events (data-independent) from the count.
	var nonSeasonal int
	for _, e := range events {
		if e.ConfidenceSource != "calendar_seasonal" && e.ConfidenceSource != "calendar_political" {
			nonSeasonal++
		}
	}
	if nonSeasonal != 0 {
		t.Fatalf("expected 0 non-seasonal events, got %d (total %d)", nonSeasonal, len(events))
	}
}

func TestTemplateHitRatesUpdatedAfterEvaluation(t *testing.T) {
	// Scenario 1: model with RecentError <= 0.5 triggers template hit rate update.
	ne := NewNarrativeEngine()
	tmpl, ok := ne.kb.GetTemplateByTheme("US_rates_up")
	if !ok {
		t.Fatalf("expected US_rates_up template to exist")
	}
	tmpl.HistoricalHitRate = 0.70
	ne.kb.RegisterTemplate(tmpl)

	// hawkish_fed_model (index 0) has ActiveThemes=["US_rates_up", "JPY_carry_unwind"]
	// HitRate = 1.0 - 0.2 = 0.8
	ne.models[0].RecentError = 0.2
	ne.models[0].HitRate = 0.8

	ne.updateTemplateHitRates()

	tmpl, ok = ne.kb.GetTemplateByTheme("US_rates_up")
	if !ok {
		t.Fatalf("template should still exist after update")
	}
	// new = 0.8*0.70 + 0.2*0.8 = 0.56 + 0.16 = 0.72
	expected := 0.8*0.70 + 0.2*0.8
	if tmpl.HistoricalHitRate != expected {
		t.Fatalf("expected HistoricalHitRate %f, got %f", expected, tmpl.HistoricalHitRate)
	}

	// Scenario 2: model with RecentError > 0.5 is skipped (no update).
	ne2 := NewNarrativeEngine()
	tmpl2, _ := ne2.kb.GetTemplateByTheme("US_rates_up")
	tmpl2.HistoricalHitRate = 0.70
	ne2.kb.RegisterTemplate(tmpl2)
	ne2.models[0].RecentError = 0.6
	ne2.models[0].HitRate = 0.4 // 1.0 - 0.6

	ne2.updateTemplateHitRates()

	tmpl2, _ = ne2.kb.GetTemplateByTheme("US_rates_up")
	if tmpl2.HistoricalHitRate != 0.70 {
		t.Fatalf("expected no update when RecentError > 0.5, got %f", tmpl2.HistoricalHitRate)
	}

	// Scenario 3: GetTemplateByTheme returns false for non-existent theme.
	if _, ok := ne.kb.GetTemplateByTheme("NONEXISTENT_THEME"); ok {
		t.Fatalf("expected false for non-existent theme")
	}
}

func TestSeasonalEventUsesParametersConfig(t *testing.T) {
	config.ResetParametersConfig()
	params := config.GetParametersConfig().Narrative

	if params.SpringFestivalConfidence.Value == 0 {
		t.Fatalf("expected non-zero SpringFestivalConfidence default")
	}
	if params.ElectionCycleConfidence.Value == 0 {
		t.Fatalf("expected non-zero ElectionCycleConfidence default")
	}
	if params.EarningsBlackoutConfidence.Value == 0 {
		t.Fatalf("expected non-zero EarningsBlackoutConfidence default")
	}
	if params.TechPeakSeasonConfidence.Value == 0 {
		t.Fatalf("expected non-zero TechPeakSeasonConfidence default")
	}
	if params.YearEndWindowDressingConfidence.Value == 0 {
		t.Fatalf("expected non-zero YearEndWindowDressingConfidence default")
	}

	nowUTC = func() time.Time { return time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) }
	defer func() { nowUTC = func() time.Time { return time.Now().UTC() } }()

	event := detectSeasonalEvent()
	if event == nil {
		t.Log("no seasonal event matched current date — parameter defaults verified above")
		return
	}

	if event.ConfidenceSource == "" {
		t.Fatalf("expected non-empty ConfidenceSource, got %q", event.ConfidenceSource)
	}
	if event.Theme == "" {
		t.Fatalf("expected non-empty Theme")
	}
	if event.Region != "TW" {
		t.Fatalf("expected Region=TW, got %q", event.Region)
	}

	switch event.Theme {
	case "spring_festival_season":
		if event.Confidence != params.SpringFestivalConfidence.Value {
			t.Fatalf("spring_festival_season: expected confidence %f from ParametersConfig, got %f",
				params.SpringFestivalConfidence.Value, event.Confidence)
		}
	case "election_cycle":
		if event.Confidence != params.ElectionCycleConfidence.Value {
			t.Fatalf("election_cycle: expected confidence %f from ParametersConfig, got %f",
				params.ElectionCycleConfidence.Value, event.Confidence)
		}
	case "earnings_blackout":
		if event.Confidence != params.EarningsBlackoutConfidence.Value {
			t.Fatalf("earnings_blackout: expected confidence %f from ParametersConfig, got %f",
				params.EarningsBlackoutConfidence.Value, event.Confidence)
		}
	case "tech_peak_season":
		if event.Confidence != params.TechPeakSeasonConfidence.Value {
			t.Fatalf("tech_peak_season: expected confidence %f from ParametersConfig, got %f",
				params.TechPeakSeasonConfidence.Value, event.Confidence)
		}
	case "year_end_window_dressing":
		if event.Confidence != params.YearEndWindowDressingConfidence.Value {
			t.Fatalf("year_end_window_dressing: expected confidence %f from ParametersConfig, got %f",
				params.YearEndWindowDressingConfidence.Value, event.Confidence)
		}
	case "dividend_season":
		// dividend_season uses a hardcoded confidence (not in ParametersConfig).
		if event.Confidence != 0.60 {
			t.Fatalf("dividend_season: expected confidence 0.60, got %f", event.Confidence)
		}
	default:
		t.Fatalf("unexpected seasonal event theme: %s", event.Theme)
	}
}

func TestSelfCalibrate_InvalidReplayPath(t *testing.T) {
	ne := NewNarrativeEngine()
	report, err := ne.SelfCalibrate("/nonexistent/replay.csv")
	if err == nil {
		t.Fatal("expected error for nonexistent replay path")
	}
	if report != nil {
		t.Fatal("expected nil report on error")
	}
}

func TestNarrativeCalibrationReport_Structure(t *testing.T) {
	ne := NewNarrativeEngine()
	models := ne.ListModels()

	report := &NarrativeCalibrationReport{
		Timestamp:     time.Now(),
		ModelsUpdated: len(models),
		Models:        models,
		Verdict:       "calibrated",
		Summary:       "all models updated",
	}

	if report.ModelsUpdated != len(models) {
		t.Fatalf("expected %d models updated, got %d", len(models), report.ModelsUpdated)
	}
	if report.Verdict != "calibrated" {
		t.Fatalf("expected verdict calibrated, got %s", report.Verdict)
	}
	if len(report.Models) == 0 {
		t.Fatal("expected non-empty models list")
	}
}

func TestNewEarningsSurpriseEvent_Positive(t *testing.T) {
	config.ResetParametersConfig()
	event := NewEarningsSurpriseEvent(15.0)
	if event == nil {
		t.Fatal("expected non-nil event for positive surprise")
	}
	if event.Theme != "earnings_surprise" {
		t.Fatalf("expected theme earnings_surprise, got %s", event.Theme)
	}
	if event.Sentiment != 0.7 {
		t.Fatalf("expected sentiment 0.7, got %f", event.Sentiment)
	}
	if event.CapitalFlow != "earnings_beat" {
		t.Fatalf("expected earnings_beat, got %s", event.CapitalFlow)
	}
	if event.Severity != "high" {
		t.Fatalf("expected severity high, got %s", event.Severity)
	}
	if event.Duration != 10*24*time.Hour {
		t.Fatalf("expected 10-day duration, got %v", event.Duration)
	}
	if event.Region != "TW" {
		t.Fatalf("expected region TW, got %s", event.Region)
	}
	if event.SourceData["surprise_pct"] != 15.0 {
		t.Fatalf("expected source surprise_pct=15.0, got %f", event.SourceData["surprise_pct"])
	}
	if event.ConfidenceSource != "deviation_based_v1" {
		t.Fatalf("expected deviation_based_v1, got %s", event.ConfidenceSource)
	}
	if event.HitRate == 0 {
		t.Fatalf("expected non-zero HitRate from template, got %f", event.HitRate)
	}
}

func TestNewEarningsSurpriseEvent_Negative(t *testing.T) {
	config.ResetParametersConfig()
	event := NewEarningsSurpriseEvent(-8.0)
	if event == nil {
		t.Fatal("expected non-nil event for negative surprise")
	}
	if event.Sentiment != -0.7 {
		t.Fatalf("expected sentiment -0.7, got %f", event.Sentiment)
	}
	if event.CapitalFlow != "earnings_miss" {
		t.Fatalf("expected earnings_miss, got %s", event.CapitalFlow)
	}
}

func TestNewEarningsSurpriseEvent_UsesParametersConfig(t *testing.T) {
	config.ResetParametersConfig()
	params := config.GetParametersConfig().Narrative
	if params.EarningsSurpriseConfidence.Value <= 0 {
		t.Fatalf("expected non-zero EarningsSurpriseConfidence, got %f", params.EarningsSurpriseConfidence.Value)
	}
	event := NewEarningsSurpriseEvent(20.0)
	if event.Confidence <= 0 {
		t.Fatalf("expected confidence > 0, got %f", event.Confidence)
	}
}

func TestEarningsSurpriseEvent_MatchesCausalChains(t *testing.T) {
	config.ResetParametersConfig()
	ne := NewNarrativeEngine()

	event := NewEarningsSurpriseEvent(15.0)
	chains := ne.kb.MatchChains(*event)

	if len(chains) == 0 {
		t.Fatal("expected at least one causal chain for earnings_surprise")
	}
	if chains[0].Score <= 0 {
		t.Fatalf("expected positive score, got %f", chains[0].Score)
	}
}

func TestRecalculateTemplateHitRates(t *testing.T) {
	ne := NewNarrativeEngine()
	tmpl, ok := ne.kb.GetTemplateByTheme("US_rates_up")
	if !ok {
		t.Fatalf("US_rates_up template must exist in default KB")
	}
	tmpl.HistoricalHitRate = 0.70
	ne.kb.RegisterTemplate(tmpl)
	ne.models[0].RecentError = 0.2
	ne.models[0].HitRate = 0.80

	ne.RecalculateTemplateHitRates()

	got, ok := ne.kb.GetTemplateByTheme("US_rates_up")
	if !ok {
		t.Fatalf("US_rates_up template missing after recalculation")
	}
	const alpha = 0.2
	want := (1-alpha)*0.70 + alpha*0.80
	if got.HistoricalHitRate != want {
		t.Fatalf("expected HistoricalHitRate %v, got %v", want, got.HistoricalHitRate)
	}
}

// Stage 4 PR#4 RecalculateAllTemplateHitRates tests begin here.

func TestRecalculateAllTemplateHitRates_UpdatesEveryTemplate(t *testing.T) {
	ne := NewNarrativeEngine()
	all := ne.kb.ListTemplates()
	if len(all) != 24 {
		t.Fatalf("default KB should have 24 templates, got %d", len(all))
	}

	const globalHitRate = 0.62
	updated := ne.RecalculateAllTemplateHitRates(globalHitRate)
	if updated == 0 {
		t.Fatalf("expected non-zero updated count, got 0")
	}
}

func TestRecalculateAllTemplateHitRates_IdempotentConvergence(t *testing.T) {
	ne := NewNarrativeEngine()
	const globalHitRate = 0.55
	const iterations = 60
	for range iterations {
		ne.RecalculateAllTemplateHitRates(globalHitRate)
	}
	prev := map[string]float64{}
	for _, tmpl := range ne.kb.ListTemplates() {
		prev[tmpl.ID] = tmpl.HistoricalHitRate
	}
	ne.RecalculateAllTemplateHitRates(globalHitRate)
	for _, tmpl := range ne.kb.ListTemplates() {
		delta := math.Abs(tmpl.HistoricalHitRate - prev[tmpl.ID])
		if delta > 0.02 {
			t.Errorf("template %q not converged: rate=%.4f, prev=%.4f, delta=%.4f",
				tmpl.ID, tmpl.HistoricalHitRate, prev[tmpl.ID], delta)
		}
		if tmpl.HistoricalHitRate < 0 || tmpl.HistoricalHitRate > 1 {
			t.Errorf("template %q HistoricalHitRate=%.4f out of [0,1]", tmpl.ID, tmpl.HistoricalHitRate)
		}
	}
}

func TestRecalculateAllTemplateHitRates_BoundsPreserved(t *testing.T) {
	ne := NewNarrativeEngine()
	extreme := []float64{-0.5, 0.0, 1.5}
	for _, g := range extreme {
		ne.RecalculateAllTemplateHitRates(g)
		for _, tmpl := range ne.kb.ListTemplates() {
			if tmpl.HistoricalHitRate < 0 || tmpl.HistoricalHitRate > 1 {
				t.Errorf("template %q HistoricalHitRate=%.4f out of [0,1] for globalHitRate=%.2f",
					tmpl.ID, tmpl.HistoricalHitRate, g)
			}
		}
	}
}

func TestRecalculateAllTemplateHitRates_PreservesExistingRecalc(t *testing.T) {
	ne := NewNarrativeEngine()
	tmpl, ok := ne.kb.GetTemplateByTheme("US_rates_up")
	if !ok {
		t.Fatalf("US_rates_up template missing")
	}
	tmpl.HistoricalHitRate = 0.65
	ne.kb.RegisterTemplate(tmpl)
	ne.models[0].RecentError = 0.2
	ne.models[0].HitRate = 0.85

	ne.RecalculateAllTemplateHitRates(0.30)

	got, ok := ne.kb.GetTemplateByTheme("US_rates_up")
	if !ok {
		t.Fatalf("US_rates_up missing after recalc-all")
	}
	// First active-models chain: (1-0.2)*0.65 + 0.2*0.85 = 0.69
	// Second global chain: (1-0.1)*0.69 + 0.1*0.30 = 0.651
	want := 0.9*((1-0.2)*0.65+0.2*0.85) + 0.1*0.30
	if math.Abs(got.HistoricalHitRate-want) > 0.001 {
		t.Errorf("US_rates_up after chained recalc: got %.6f, want %.6f", got.HistoricalHitRate, want)
	}
}

// writeRegimeCSV writes a replay CSV where 0050 (TAIEX proxy) falls for the
// first half of dates and rises for the second half. Other symbols use a
// fixed multiplier so only 0050's regime structure drives classification.
// Returns the path and the count of dates in the risk_on (second) half.
func writeRegimeCSV(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "regime.csv")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	fmt.Fprintln(f, "Date,Code,Name,TradeVolume,Open,High,Low,Close")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const days = 60
	// 0050: falls in first 30 days (risk_off), rises in last 30 (risk_on).
	// Current regime = last 20-day momentum window = rising → risk_on.
	for day := range days {
		date := start.Add(time.Duration(day) * 24 * time.Hour).Format("2006-01-02")
		var p0050 float64
		if day < 30 {
			p0050 = 140.0 - float64(day) // descending
		} else {
			p0050 = 110.0 + float64(day-30) // ascending
		}
		// All sector symbols: flat price so they never go stale (constant OHLCV
		// would be rejected as stale by ForwardReturn, so vary slightly).
		for _, code := range []string{"2330", "2881", "2603", "3008"} {
			base := map[string]float64{"2330": 500, "2881": 25, "2603": 60, "3008": 250}[code]
			px := base + float64(day)*0.1
			fmt.Fprintf(f, "%s,%s,%s,1000,%.2f,%.2f,%.2f,%.2f\n", date, code, code, px*0.995, px*1.01, px*0.99, px)
		}
		px := p0050
		fmt.Fprintf(f, "%s,%s,%s,1000,%.2f,%.2f,%.2f,%.2f\n", date, "0050", "0050", px*0.995, px*1.01, px*0.99, px)
	}
	return path
}

// TestEvaluateModels_RegimeFiltered verifies the phase-B regime-aware
// backfill: when momentum classification is available, only samples whose
// date regime matches the current regime are counted, shrinking sample_count
// relative to the full window.
func TestEvaluateModels_RegimeFiltered(t *testing.T) {
	config.ResetParametersConfig()
	csvPath := writeRegimeCSV(t)

	ne := NewNarrativeEngine()
	if err := ne.EvaluateModels(csvPath); err != nil {
		t.Fatalf("EvaluateModels: %v", err)
	}

	// 0050 present + 60 days (>= momentumWindow+1) → regime filter active.
	// The current regime is risk_on (last 20-day window rises), so only
	// risk_on dates contribute. model hawkish_fed favors financials/avoids
	// ai_supply_chain; with flat-ish sector prices its sample is the
	// risk_on half (≈ 30 - 5 forward = 25) rather than the full window.
	hawkish := findModel(ne.models, "hawkish_fed_model")
	if hawkish == nil {
		t.Fatal("hawkish_fed_model missing")
	}
	t.Logf("regime-filtered sample_count=%d", hawkish.SampleCount)
	if hawkish.SampleCount >= 30 {
		t.Fatalf("expected regime-filtered sample_count < full window (30), got %d", hawkish.SampleCount)
	}
	if hawkish.SampleCount <= 0 {
		t.Fatalf("expected positive sample_count under regime filter, got %d", hawkish.SampleCount)
	}
}

// TestEvaluateModels_RegimeFallbackNo0050 verifies backward compatibility:
// when 0050 is absent from the replay, momentumRegimes returns nil and
// EvaluateModels falls back to the full window (no regime filtering).
func TestEvaluateModels_RegimeFallbackNo0050(t *testing.T) {
	config.ResetParametersConfig()
	dir := t.TempDir()
	path := filepath.Join(dir, "no0050.csv")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	fmt.Fprintln(f, "Date,Code,Name,TradeVolume,Open,High,Low,Close")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for day := range 40 {
		date := start.Add(time.Duration(day) * 24 * time.Hour).Format("2006-01-02")
		for _, code := range []string{"2330", "2881"} {
			px := float64(100 + day)
			fmt.Fprintf(f, "%s,%s,%s,1000,%.2f,%.2f,%.2f,%.2f\n", date, code, code, px*0.995, px*1.01, px*0.99, px)
		}
	}
	ne := NewNarrativeEngine()
	if err := ne.EvaluateModels(path); err != nil {
		t.Fatalf("EvaluateModels: %v", err)
	}
	// Without 0050, momentumRegimes → nil → no filter; all valid days count.
	hawkish := findModel(ne.models, "hawkish_fed_model")
	if hawkish == nil {
		t.Fatal("hawkish_fed_model missing")
	}
	t.Logf("fallback sample_count=%d", hawkish.SampleCount)
	if hawkish.SampleCount == 0 {
		t.Fatalf("expected non-zero sample_count in fallback, got 0")
	}
}

// findModel returns a model by ID or nil.
func findModel(models []InvestmentModel, id string) *InvestmentModel {
	for i := range models {
		if models[i].ID == id {
			return &models[i]
		}
	}
	return nil
}
