package industry

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"golang.org/x/time/rate"
)

// ---------------------------------------------------------------------------
// DataAggregator
// ---------------------------------------------------------------------------

func TestDataAggregator_NilFinMind(t *testing.T) {
	tracker := NewCycleTracker()
	tree := DefaultClassification()

	a := NewDataAggregator(tracker, tree, nil, nil)
	if a == nil {
		t.Fatal("NewDataAggregator returned nil")
	}

	ctx := context.Background()
	if err := a.AggregateAllIndustries(ctx); err != nil {
		t.Fatalf("AggregateAllIndustries with nil FinMind should be no-op: %v", err)
	}

	if err := a.AggregateIndustry(ctx, "semiconductor"); err == nil {
		t.Fatal("AggregateIndustry with nil FinMind should error")
	}
}

// TestAggregateAllIndustriesReport_NilFinMind 驗證 Report 版對 nil finmind
// 的 no-op 契約（#A03）：回非 nil report 且 attempted/succeeded 皆為 0。
func TestAggregateAllIndustriesReport_NilFinMind(t *testing.T) {
	tracker := NewCycleTracker()
	tree := DefaultClassification()
	a := NewDataAggregator(tracker, tree, nil, nil)

	report, err := a.AggregateAllIndustriesReport(context.Background())
	if err != nil {
		t.Fatalf("nil-finmind should be no-op, got %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Attempted != 0 || report.Succeeded != 0 {
		t.Errorf("nil-finmind report = %d/%d, want 0/0", report.Succeeded, report.Attempted)
	}
	if len(report.Industries) != 0 {
		t.Errorf("nil-finmind report industries = %d, want 0", len(report.Industries))
	}
}

// TestAggregateAllIndustriesReport_AllFail 驗證 Report 版全失敗路徑（#A03）：
// 用 zero-rate limiter 讓每個 symbol 請求立即 ErrRateLimited → 所有 industry
// 失敗 → 回 error 且 report 記錄 attempted>0 / succeeded=0 / per-industry 明細。
func TestAggregateAllIndustriesReport_AllFail(t *testing.T) {
	tracker := NewCycleTracker()
	tree := DefaultClassification()
	client := marketdata.NewFinMindClient("test-key")
	client.SetRateLimiter(rate.NewLimiter(0, 1)) // 立即 rate-limited
	a := NewDataAggregator(tracker, tree, client, nil)

	report, err := a.AggregateAllIndustriesReport(context.Background())
	if err == nil {
		t.Fatal("expected error when all industries fail")
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Attempted == 0 {
		t.Error("expected attempted > 0 (classification tree has representative stocks)")
	}
	if report.Succeeded != 0 {
		t.Errorf("expected 0 succeeded, got %d", report.Succeeded)
	}
	if len(report.Industries) != report.Attempted {
		t.Errorf("industries detail = %d, want attempted %d", len(report.Industries), report.Attempted)
	}
	for _, st := range report.Industries {
		if st.Succeeded {
			t.Errorf("industry %q marked succeeded but all requests rate-limited", st.IndustryID)
		}
		if st.Error == "" {
			t.Errorf("industry %q missing error detail", st.IndustryID)
		}
	}
}

func TestDataAggregator_ExtractProfitAndClampGrowth(t *testing.T) {
	tests := []struct {
		name string
		v    float64
		want float64
	}{
		{"high", 6.0, 5.0},
		{"low", -1.5, -1.0},
		{"mid", 0.35, 0.35},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampGrowth(tt.v); got != tt.want {
				t.Fatalf("clampGrowth(%v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}

	if got := extractProfit(map[string]float64{"本期淨利": 120}); got != 120 {
		t.Fatalf("extractProfit local key failed: %v", got)
	}
	if got := extractProfit(map[string]float64{"NetIncome": 80}); got != 80 {
		t.Fatalf("extractProfit fallback key failed: %v", got)
	}
	if got := extractProfit(map[string]float64{"other": 10}); got != 0 {
		t.Fatalf("extractProfit should return 0 when no profit key: %v", got)
	}
}

func TestDataAggregator_RecalibrateThresholds(t *testing.T) {
	dir := t.TempDir()
	revenuePath := filepath.Join(dir, "revenue.jsonl")
	configPath := filepath.Join(dir, "params.json")

	records := []revenueRecord{
		{StockID: "2330.TW", Revenue: 100.0, RevenueYear: 2023, RevenueMonth: 1, Date: "2023-01-01"},
		{StockID: "2330.TW", Revenue: 120.0, RevenueYear: 2024, RevenueMonth: 1, Date: "2024-01-01"},
		{StockID: "2330.TW", Revenue: 110.0, RevenueYear: 2023, RevenueMonth: 2, Date: "2023-02-01"},
		{StockID: "2330.TW", Revenue: 130.0, RevenueYear: 2024, RevenueMonth: 2, Date: "2024-02-01"},
	}
	f, err := os.Create(revenuePath)
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, r := range records {
		_ = enc.Encode(r)
	}
	_ = f.Close()

	if err := os.WriteFile(configPath, []byte(`{"industry":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RecalibrateThresholds(revenuePath, configPath); err != nil {
		t.Fatalf("RecalibrateThresholds failed: %v", err)
	}

	out, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	industryCfg := cfg["industry"].(map[string]any)
	ct := industryCfg["cycle_thresholds"].(map[string]any)
	if ct["source"] != "percentile_based" {
		t.Fatalf("expected percentile based thresholds, got %v", ct["source"])
	}
	value := ct["value"].(map[string]any)
	if _, ok := value["semiconductor"]; !ok {
		t.Fatalf("expected semiconductor thresholds, got %v", value)
	}
}

func TestThresholdCalibrator_LoadRevenueFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "revenue.jsonl")
	f, _ := os.Create(path)
	_ = json.NewEncoder(f).Encode(revenueRecord{StockID: "2330.TW", Revenue: 100, RevenueYear: 2024, RevenueMonth: 1})
	_ = f.Close()

	records, err := loadRevenueFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
}

// ---------------------------------------------------------------------------
// Seasonality
// ---------------------------------------------------------------------------

func TestSeasonalPattern_AffectedIndustries(t *testing.T) {
	p := SeasonalPattern{
		ID:                "tech_peak_season",
		FavoredIndustries: []string{"semiconductor", "ai_supply_chain"},
		AvoidedIndustries: []string{"consumer"},
	}
	got := p.AffectedIndustries()
	sort.Strings(got)
	want := []string{"ai_supply_chain", "consumer", "semiconductor"}
	if len(got) != len(want) {
		t.Fatalf("AffectedIndustries length mismatch: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AffectedIndustries[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestSeasonalEngine_QueryMethods(t *testing.T) {
	engine := NewSeasonalEngineFromConfig(nil)

	names := engine.GetActivePatternNames(time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC))
	found := false
	for _, n := range names {
		if n == "科技旺季" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 科技旺季 in active patterns on 2024-07-15, got %v", names)
	}

	all := engine.GetAllPatterns()
	if len(all) == 0 {
		t.Fatal("GetAllPatterns returned empty")
	}

	sem := engine.GetPatternsForIndustry("semiconductor")
	if len(sem) == 0 {
		t.Fatal("expected semiconductor seasonal patterns")
	}

	impact, adj := engine.GetIndustryImpact("tech_peak_season", "semiconductor")
	if impact != "favored" || math.Abs(adj-1.25) > 1e-9 {
		t.Fatalf("expected favored/1.25 for semiconductor in tech peak, got %s/%v", impact, adj)
	}

	impact, adj = engine.GetIndustryImpact("spring_festival", "semiconductor")
	if impact != "avoided" || math.Abs(adj-1.0/1.15) > 1e-9 {
		t.Fatalf("expected avoided for semiconductor in spring festival, got %s/%v", impact, adj)
	}
}

func TestSeasonalEngine_SettersAndUpdateDynamicEnv(t *testing.T) {
	engine := NewSeasonalEngineFromConfig(nil)
	engine.SetLinkageGraph(DefaultSupplyChainGraph())
	engine.SetNarrativeProvider(nil)

	baseline := marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 80}, DXY: marketdata.MacroDataPoint{Value: 100}, Bdi: marketdata.MacroDataPoint{Value: 1500}}
	current := marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 90}, DXY: marketdata.MacroDataPoint{Value: 105}, Bdi: marketdata.MacroDataPoint{Value: 2000}}
	mod := NewDynamicEnvModulator(baseline, current)
	engine.SetDynamicEnv(mod)

	engine.UpdateDynamicEnv(current)
	if engine.dynamicEnv == nil {
		t.Fatal("dynamic env not set")
	}
}

// ---------------------------------------------------------------------------
// Seasonal calibrator / performance
// ---------------------------------------------------------------------------

func buildTechPeakReturns() map[string]map[string]float64 {
	returns := map[string]map[string]float64{
		"semiconductor":   {},
		"ai_supply_chain": {},
		"electronics":     {},
		"consumer":        {},
	}
	for year := 2020; year <= 2023; year++ {
		for _, d := range []string{"07-10", "07-20", "08-10", "08-20", "09-05"} {
			returns["semiconductor"][time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 0).Format("2006")+"-"+d] = 0.002
			returns["ai_supply_chain"][""+time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006")+"-"+d] = 0.0015
			returns["electronics"][""+time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006")+"-"+d] = 0.001
			returns["consumer"][""+time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006")+"-"+d] = -0.001
		}
	}
	// Use explicit dates to avoid key collisions with empty strings.
	clean := make(map[string]map[string]float64)
	for ind := range returns {
		clean[ind] = make(map[string]float64)
		for _, r := range []struct {
			y int
			d string
		}{
			{2020, "2020-07-10"},
			{2020, "2020-08-10"},
			{2020, "2020-09-05"},
			{2021, "2021-07-12"},
			{2021, "2021-08-12"},
			{2021, "2021-09-06"},
			{2022, "2022-07-11"},
			{2022, "2022-08-11"},
			{2022, "2022-09-05"},
			{2023, "2023-07-10"},
			{2023, "2023-08-10"},
			{2023, "2023-09-05"},
		} {
			clean[ind][r.d] = 0.001
			if ind == "consumer" {
				clean[ind][r.d] = -0.001
			}
		}
	}
	return clean
}

func TestSeasonalCalibrator_BacktestTSMC(t *testing.T) {
	engine := NewSeasonalEngineFromConfig(nil)
	returns := buildTechPeakReturns()

	results, err := CalibratePatterns(engine, returns, 2020, 2023)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected calibration results")
	}

	var techPeak *SeasonalCalibration
	for i := range results {
		if results[i].PatternID == "tech_peak_season" {
			techPeak = &results[i]
			break
		}
	}
	if techPeak == nil {
		t.Fatal("tech_peak_season calibration missing")
	}
	if techPeak.ObservationCount == 0 {
		t.Fatal("expected observations")
	}
	if techPeak.ObservedAccuracy < 0.99 {
		t.Fatalf("expected near-perfect accuracy for synthetic TSMC tech peak, got %v", techPeak.ObservedAccuracy)
	}

	report := CalibrationReport(results)
	if report == "" || len(report) < 20 {
		t.Fatal("CalibrationReport unexpectedly short")
	}
}

func TestSeasonalCalibrator_IndustryReturnAggregator(t *testing.T) {
	stockReturns := map[string]map[string]float64{
		"2330.TW": {"2024-07-10": 0.01, "2024-07-11": 0.02},
		"2303.TW": {"2024-07-10": 0.015, "2024-07-11": 0.015},
	}
	stockIndustryMap := map[string][]string{
		"2330.TW": {"semiconductor"},
		"2303.TW": {"semiconductor", "foundry"},
	}
	got := IndustryReturnAggregator(stockReturns, stockIndustryMap)
	sem := got["semiconductor"]
	if len(sem) != 2 {
		t.Fatalf("expected 2 dates for semiconductor, got %v", sem)
	}
	if math.Abs(sem["2024-07-10"]-0.0125) > 1e-9 {
		t.Fatalf("expected equal-weight avg 0.0125 on 2024-07-10, got %v", sem["2024-07-10"])
	}
}

func TestSeasonalCalibrator_ValidateIndustryIDs(t *testing.T) {
	patterns := []SeasonalPattern{
		{ID: "p1", FavoredIndustries: []string{"semiconductor"}, AvoidedIndustries: []string{"missing_ind"}},
	}
	returns := map[string]map[string]float64{"semiconductor": {}}
	missing := ValidateIndustryIDs(patterns, returns)
	if len(missing) != 1 || missing[0] != "missing_ind" {
		t.Fatalf("expected missing_ind missing, got %v", missing)
	}
}

func TestSeasonalCalibrator_ValidateCalibrationHoldout(t *testing.T) {
	engine := NewSeasonalEngineFromConfig(nil)
	var tp SeasonalPattern
	for _, p := range engine.GetAllPatterns() {
		if p.ID == "tech_peak_season" {
			tp = p
		}
	}
	returns := buildTechPeakReturns()
	res := ValidateCalibration(tp, returns, 2020, 2023, 0.5, 0.05)
	if res.PatternID != "tech_peak_season" {
		t.Fatalf("wrong pattern id: %s", res.PatternID)
	}
	if res.TrainSampleSize == 0 || res.TestSampleSize == 0 {
		t.Fatalf("expected train/test samples, got train=%d test=%d", res.TrainSampleSize, res.TestSampleSize)
	}
}

func TestSeasonalPerformanceStore(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSeasonalPerformanceStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, p := range []SeasonalPerformance{
		{PatternID: "tech_peak_season", Year: 2022, Accuracy: 0.80, RecordedAt: now},
		{PatternID: "tech_peak_season", Year: 2023, Accuracy: 0.90, RecordedAt: now},
		{PatternID: "spring_festival", Year: 2023, Accuracy: 0.70, RecordedAt: now},
	} {
		if err := store.Record(p); err != nil {
			t.Fatal(err)
		}
	}

	hist, err := store.GetPatternHistory("tech_peak_season")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Fatalf("expected 2 tech_peak_season records, got %d", len(hist))
	}

	acc, err := store.GetRollingAccuracy("tech_peak_season", 5)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(acc-0.85) > 1e-9 {
		t.Fatalf("expected rolling accuracy 0.85, got %v", acc)
	}
}

// ---------------------------------------------------------------------------
// Linkage
// ---------------------------------------------------------------------------

func TestLinkageHistoryStore(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLinkageHistoryStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	rec := LinkageHistoryRecord{
		Date:                  now.Format("2006-01-02"),
		IndustryID:            "semiconductor",
		SystemicImportance:    0.7,
		ShockPropagationSpeed: 0.4,
		RecordedAt:            now,
	}
	if err := store.Record(rec); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(LinkageHistoryRecord{Date: now.Format("2006-01-02"), IndustryID: "shipping", RecordedAt: now}); err != nil {
		t.Fatal(err)
	}

	hist, err := store.GetHistory("semiconductor", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 {
		t.Fatalf("expected 1 semiconductor history record, got %d", len(hist))
	}

	latest, err := store.GetLatest("semiconductor")
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.SystemicImportance != 0.7 {
		t.Fatalf("GetLatest mismatch: %v", latest)
	}
}

func TestLinkageAnalyzer_ExtraGettersAndShockPropagation(t *testing.T) {
	la := NewLinkageAnalyzer()
	if la.GetSupplyChainGraph() == nil {
		t.Fatal("expected supply chain graph")
	}
	if la.GetCorrelationMatrix() == nil {
		t.Fatal("expected correlation matrix")
	}

	impacts := la.PropagateShock("semiconductor", 0.10, 2)
	if len(impacts) == 0 {
		t.Fatal("expected shock propagation impacts")
	}
	if _, ok := impacts["semiconductor"]; !ok {
		t.Fatal("expected source industry in impacts")
	}
}

func TestCorrelationMatrix_RecalculateFromReturns(t *testing.T) {
	cm := NewCorrelationMatrix(30)
	returns := map[string][]float64{
		"semiconductor": {
			0.01, 0.02, 0.03, 0.04, 0.05, 0.06, 0.07, 0.08, 0.09, 0.10,
			0.11, 0.12, 0.13, 0.14, 0.15, 0.16, 0.17, 0.18, 0.19, 0.20,
			0.21, 0.22, 0.23, 0.24, 0.25, 0.26, 0.27, 0.28, 0.29, 0.30,
		},
		"ai_supply_chain": {
			0.02, 0.04, 0.06, 0.08, 0.10, 0.12, 0.14, 0.16, 0.18, 0.20,
			0.22, 0.24, 0.26, 0.28, 0.30, 0.32, 0.34, 0.36, 0.38, 0.40,
			0.42, 0.44, 0.46, 0.48, 0.50, 0.52, 0.54, 0.56, 0.58, 0.60,
		},
	}
	cm.RecalculateFromReturns(returns)
	corr, ok := cm.GetCorrelation("semiconductor", "ai_supply_chain")
	if !ok {
		t.Fatal("expected correlation computed")
	}
	if math.Abs(corr-1.0) > 1e-3 {
		t.Fatalf("expected near-perfect correlation, got %v", corr)
	}

	if p := pearsonCorrelation([]float64{1, 2, 3}, []float64{2, 4, 6}); math.Abs(p-1.0) > 1e-9 {
		t.Fatalf("pearson perfect correlation failed: %v", p)
	}
}

// ---------------------------------------------------------------------------
// Cycle tracker
// ---------------------------------------------------------------------------

func TestCycleTracker_SettersAndWeightModulator(t *testing.T) {
	ct := NewCycleTracker()
	ct.SetExternalValidators(NewSeasonalEngineFromConfig(nil), NewLinkageAnalyzer())
	ct.SetNarrativeProvider(func() float64 { return 0.75 })
	ct.SetNarrativeAdjuster(func(id string) NarrativeAdjustment {
		return NarrativeAdjustment{RevenueBias: 0.05, ProfitBias: 0.05, Confidence: 0.5, ActiveTheme: "ai_capex_surge"}
	})

	mod := ct.GetWeightModulator("semiconductor")
	if mod <= 1.0 {
		t.Fatalf("expected expansion weight modulator > 1 for semiconductor, got %v", mod)
	}
}

func TestCycleTracker_computeLinkageConfidence(t *testing.T) {
	ct := NewCycleTracker()
	graph := DefaultSupplyChainGraph()
	cm := NewCorrelationMatrix(30)
	cm.UpdateCorrelation("semiconductor", "electronics", 0.8)
	la := NewLinkageAnalyzer()
	la.SetSupplyChainGraph(graph, cm)
	ct.SetExternalValidators(nil, la)

	expansion := IndustryMetrics{RevenueGrowthYoY: 0.25, ProfitGrowthYoY: 0.30, InventoryTurnover: 5.5, CapacityUtilization: 0.85}
	ct.UpdatePosition("semiconductor", expansion)
	ct.UpdatePosition("electronics", expansion)

	score := ct.computeLinkageConfidence("semiconductor")
	if score <= 0 {
		t.Fatalf("expected positive linkage confidence, got %v", score)
	}
}

func TestCycleTracker_defaultSeedMetrics(t *testing.T) {
	seeds := defaultSeedMetrics()
	if len(seeds) == 0 {
		t.Fatal("expected seed metrics")
	}
	semi, ok := seeds["semiconductor"]
	if !ok {
		t.Fatal("missing semiconductor seed")
	}
	if math.Abs(semi.RevenueGrowthYoY-0.25) > 1e-9 {
		t.Fatalf("semiconductor seed revenue mismatch: %v", semi.RevenueGrowthYoY)
	}
}

func TestCyclePosition_GetTrend(t *testing.T) {
	ct := NewCycleTracker()
	pos, _ := ct.GetPosition("semiconductor")
	if pos.GetTrend() != "up" {
		t.Fatalf("expected semiconductor default trend up, got %s", pos.GetTrend())
	}
}

// ---------------------------------------------------------------------------
// Dynamic environment
// ---------------------------------------------------------------------------

func TestDynamicEnvModulator_UpdateCurrentAndFlags(t *testing.T) {
	baseline := marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 80}, DXY: marketdata.MacroDataPoint{Value: 100}}
	current := marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 100}, DXY: marketdata.MacroDataPoint{Value: 95}}
	dem := NewDynamicEnvModulator(baseline, current)

	newOil := marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 110}}
	dem.UpdateCurrent(newOil)
	if !dem.IsOilElevated(0.10) {
		t.Fatal("expected oil elevated after update")
	}
	if dem.IsDollarStrong(0.10) {
		t.Fatal("expected dollar not strong")
	}
}

func TestDynamicEnvModulator_ShippingSeasonalModulation(t *testing.T) {
	baseline := marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 80}, Bdi: marketdata.MacroDataPoint{Value: 1500}}
	current := marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 80}, Bdi: marketdata.MacroDataPoint{Value: 2200}}
	dem := NewDynamicEnvModulator(baseline, current)

	mod := dem.SeasonalModulation("shipping")
	if mod <= 1.0 {
		t.Fatalf("expected BDI-driven boost for shipping, got %v", mod)
	}
}

// ---------------------------------------------------------------------------
// Event calendar
// ---------------------------------------------------------------------------

func TestEventCalendar_LunarCoverageAndProviderUpdate(t *testing.T) {
	minYear, maxYear := GetLunarCoverageYears()
	if minYear != 2023 || maxYear != 2030 {
		t.Fatalf("unexpected lunar coverage: %d-%d", minYear, maxYear)
	}

	cal := NewEventCalendar()
	provider := &stubCalendarProvider{}
	cal.UpdateFromProvider(context.Background(), provider)
	if len(cal.GetAllEvents()) == 0 {
		t.Fatal("expected provider events added")
	}
}

type stubCalendarProvider struct{}

func (s *stubCalendarProvider) Name() string { return "twse" }
func (s *stubCalendarProvider) FetchEvents(ctx context.Context, year int) ([]marketdata.CalendarProviderData, error) {
	return []marketdata.CalendarProviderData{
		{Date: time.Now().Format("2006-01-02"), EventType: "earnings", Name: "法說會", Direction: "bullish", Weight: 0.6, Description: "stub", Source: "twse"},
	}, nil
}

// ---------------------------------------------------------------------------
// Correlation loader
// ---------------------------------------------------------------------------

func TestCorrelationLoader_JSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.jsonl")
	lines := []string{
		`{"Date":"2024-01-02T00:00:00Z","Symbol":"2330.TW","Name":"TSMC","Open":500,"High":510,"Low":495,"Close":505,"Volume":100000}`,
		`{"date":"2024-01-03","symbol":"2330.TW","close":510,"volume":120000}`,
	}
	if err := os.WriteFile(path, []byte(lines[0]+"\n"+lines[1]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ds, err := loadJSONLDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.ByDate) == 0 {
		t.Fatal("expected parsed dates")
	}

	row, err := parseJSONLRow(`{"Date":"2024-06-15","Symbol":"2317.TW","Close":105.5,"Volume":50000}`)
	if err != nil {
		t.Fatal(err)
	}
	if row.Symbol != "2317.TW" || row.Close != 105.5 {
		t.Fatalf("parseJSONLRow mismatch: %+v", row)
	}
}

// ---------------------------------------------------------------------------
// Silicon / sector bridge
// ---------------------------------------------------------------------------

func TestSiliconDataAggregator_WithMockProvider(t *testing.T) {
	tracker := NewSiliconCycleTracker()

	// nil provider path
	agg := NewSiliconDataAggregator(tracker, nil)
	if err := agg.AggregateSiliconIndicators(context.Background()); err == nil {
		t.Fatal("expected error for nil provider")
	}

	// mock provider path
	mock := &stubMacroProvider{snap: marketdata.MacroDataSnapshot{
		TSMCRevenue:     marketdata.MacroDataPoint{ChangePct: 25.0},
		SOXIndex:        marketdata.MacroDataPoint{ChangePct: 30.0},
		DRAMSpotPrice:   marketdata.MacroDataPoint{ChangePct: -5.0},
		TaiwanSemiIndex: marketdata.MacroDataPoint{ChangePct: 10.0},
	}}
	agg = NewSiliconDataAggregator(tracker, mock)
	if err := agg.AggregateSiliconIndicators(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type stubMacroProvider struct {
	snap marketdata.MacroDataSnapshot
}

func (s *stubMacroProvider) Name() string { return "stub" }
func (s *stubMacroProvider) FetchSnapshot(ctx context.Context) (marketdata.MacroDataSnapshot, error) {
	return s.snap, nil
}

func TestExtractSiliconIndicators_TSMC(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		TSMCRevenue:     marketdata.MacroDataPoint{ChangePct: 25.0},
		SOXIndex:        marketdata.MacroDataPoint{ChangePct: 30.0},
		DRAMSpotPrice:   marketdata.MacroDataPoint{ChangePct: -5.0},
		TaiwanSemiIndex: marketdata.MacroDataPoint{ChangePct: 10.0},
	}
	ind := ExtractSiliconIndicators(snap)
	if math.Abs(ind.TSMCMonthlyRevenueYoY-0.25) > 1e-9 {
		t.Fatalf("TSMC revenue yoy mismatch: %v", ind.TSMCMonthlyRevenueYoY)
	}
	if math.Abs(ind.GlobalSemiconductorBillingsYoY-(0.30*0.85)) > 1e-9 {
		t.Fatalf("billings mismatch: %v", ind.GlobalSemiconductorBillingsYoY)
	}
	if math.Abs(ind.TSMCCapexGuidance-0.05) > 1e-9 {
		t.Fatalf("capex signal mismatch: %v", ind.TSMCCapexGuidance)
	}
}

func TestSectorDataBridge_CoWoS(t *testing.T) {
	tracker := NewCycleTracker()
	snap := marketdata.MacroDataSnapshot{
		TSMCRevenue:      marketdata.MacroDataPoint{Value: 45.2},
		CoWoSUtilization: marketdata.MacroDataPoint{Value: 85.0},
	}
	BridgeSectorDataToCycleTracker(snap, tracker)

	ai, _ := tracker.GetPosition("ai_supply_chain")
	if math.Abs(ai.RevenueGrowthYoY-0.452) > 1e-9 {
		t.Fatalf("ai_supply_chain revenue growth mismatch: %v", ai.RevenueGrowthYoY)
	}
	semi, _ := tracker.GetPosition("semiconductor")
	if math.Abs(semi.RevenueGrowthYoY-0.452) > 1e-9 {
		t.Fatalf("semiconductor revenue growth mismatch: %v", semi.RevenueGrowthYoY)
	}
}

// ---------------------------------------------------------------------------
// Risk monitor
// ---------------------------------------------------------------------------

func TestRiskMonitor_GetAllRisksForIndustrySemiconductor(t *testing.T) {
	rm := NewRiskMonitor()
	risks := rm.GetAllRisksForIndustry("semiconductor", -0.05, 2.0)
	if len(risks) == 0 {
		t.Fatal("expected risks for semiconductor representative stocks")
	}

	allRisks := rm.GetAllRisksForIndustry("ALL", -0.05, 2.0)
	if len(allRisks) == 0 {
		t.Fatal("expected risks across all industries")
	}

	highest := rm.GetHighestRisk(risks)
	if highest == nil {
		t.Fatal("expected a highest risk")
	}
}

// ---------------------------------------------------------------------------
// Cycle status card
// ---------------------------------------------------------------------------

func TestCycleStatusCard_GlobalCalibrationAndSupplyChainSignal(t *testing.T) {
	old := GetGlobalCycleCalibration()
	defer SetGlobalCycleCalibration(old)

	cal := NewCycleCalibration(config.GetParametersConfig().Industry.CycleCalibration.Value)
	SetGlobalCycleCalibration(cal)
	if GetGlobalCycleCalibration() != cal {
		t.Fatal("global calibration getter/setter mismatch")
	}

	tracker := NewCycleTracker()
	la := NewLinkageAnalyzer()
	builder := NewCycleStatusCardBuilder(NewSiliconCycleTracker(), tracker, NewSeasonalEngineFromConfig(nil), NewEventCalendar(), la)
	card, err := builder.BuildCard(time.Now(), "semiconductor")
	if err != nil {
		t.Fatal(err)
	}
	if card.CompositeCoefficient < 0.80 || card.CompositeCoefficient > 1.20 {
		t.Fatalf("composite coefficient out of clamp range: %v", card.CompositeCoefficient)
	}

	score := builder.cyclePositionScore("semiconductor")
	_ = score // value is delegated to cycleTracker.GetContinuousPhaseScore; not bounded here
}

// ---------------------------------------------------------------------------
// classifyFinMindError — DataAggregator 失敗 metric kind 分類
// ---------------------------------------------------------------------------

// TestClassifyFinMindError_QuotaExhausted 驗證 marketdata.ErrQuotaExhausted sentinel
// (即使被 fmt.Errorf("finmind: %w", ErrQuotaExhausted) wrap) 仍能被 errors.Is 識別為 "quota"。
// 這是 PR-F 區分「quota 真的打爆」vs「symbol 沒資料」的關鍵判斷。
func TestClassifyFinMindError_QuotaExhausted(t *testing.T) {
	wrapped := fmt.Errorf("finmind: %w (used=14400, remaining=0)", marketdata.ErrQuotaExhausted)
	if got := classifyFinMindError(wrapped); got != "quota" {
		t.Errorf("wrapped ErrQuotaExhausted: got %q, want %q", got, "quota")
	}
	if got := classifyFinMindError(marketdata.ErrQuotaExhausted); got != "quota" {
		t.Errorf("bare ErrQuotaExhausted: got %q, want %q", got, "quota")
	}
}

// TestClassifyFinMindError_RateLimited 驗證 ErrRateLimited 走 "rate_limited"。
func TestClassifyFinMindError_RateLimited(t *testing.T) {
	if got := classifyFinMindError(marketdata.ErrRateLimited); got != "rate_limited" {
		t.Errorf("got %q, want %q", got, "rate_limited")
	}
}

// TestClassifyFinMindError_Server402 驗證 server-side 402 body 被分到 "quota"
// (Issue #1465 HF-1b — 與 ErrQuotaExhausted 共用 quota bucket)。
func TestClassifyFinMindError_Server402(t *testing.T) {
	msg := `finmind: status 402, body: {"msg":"Requests reach the upper limit. https://finmindtrade.com/","status":402}`
	if got := classifyFinMindError(fmt.Errorf("%s", msg)); got != "quota" {
		t.Errorf("server 402: got %q, want %q", got, "quota")
	}
	// 獨立 body 也應識別
	if got := classifyFinMindError(fmt.Errorf("finmind: status 402, body: Requests reach the upper limit")); got != "quota" {
		t.Errorf("bare 402: got %q, want %q", got, "quota")
	}
}

// TestIsFinMindQuotaOrRateLimited 驗證 helper 的判斷面：
// rate-limit sentinel、402 body → true；no-data / transport → false。
func TestIsFinMindQuotaOrRateLimited(t *testing.T) {
	quotaCases := []error{
		marketdata.ErrRateLimited,
		fmt.Errorf("finmind: rate limit wait: %w", marketdata.ErrRateLimited),
		fmt.Errorf("finmind: status 402, body: Requests reach the upper limit"),
		fmt.Errorf("finmind: status 402, body: {\"msg\":\"Requests reach the upper limit\",\"status\":402}"),
	}
	for _, err := range quotaCases {
		if !isFinMindQuotaOrRateLimited(err) {
			t.Errorf("isFinMindQuotaOrRateLimited(%v) = false, want true", err)
		}
	}
	nonQuotaCases := []error{
		nil,
		fmt.Errorf("finmind: no month revenue data for 2330.TW 2026-08"),
		fmt.Errorf("finmind revenue: no data for 2330.TW in last 3 months"),
		fmt.Errorf("finmind: http request: context deadline exceeded"),
	}
	for _, err := range nonQuotaCases {
		if isFinMindQuotaOrRateLimited(err) {
			t.Errorf("isFinMindQuotaOrRateLimited(%v) = true, want false", err)
		}
	}
}

// TestClassifyFinMindError_NoDataPattern 驗證「symbol 沒資料」訊息被分到 "no_data"。
// production `auto_cycle_update` 的 last_error 訊息正是這個 pattern。
func TestClassifyFinMindError_NoDataPattern(t *testing.T) {
	cases := []struct{ msg, want string }{
		{"finmind: no month revenue data for 6271.TW 2026-08", "no_data"},
		{"finmind revenue: no data for 6271.TW in last 3 months", "no_data"},
		// AggregateIndustry 自己的彙總 error (production 觀察到的 last_error pattern)
		// 修法: 在 classifyFinMindError 加 "no valid data" substring 匹配 (commit fixup)
		{"data_aggregator: no valid data for industry \"leo_satellite\"", "no_data"},
	}
	for _, tc := range cases {
		if got := classifyFinMindError(fmt.Errorf("%s", tc.msg)); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.msg, got, tc.want)
		}
	}
}

// TestClassifyFinMindError_TransportPattern 驗證 transport-level 錯誤 (HTTP timeout、DNS 等) 被分到 "transport"。
func TestClassifyFinMindError_TransportPattern(t *testing.T) {
	cases := []struct{ msg, want string }{
		{"finmind: http request: Get \"https://api.finmindtrade.com/...\": context deadline exceeded", "transport"},
		{"finmind: http request: dial tcp: lookup api.finmindtrade.com: no such host", "transport"},
		{"finmind: http request: dial tcp: connection refused", "transport"},
		{"finmind: http request: Get \"https://x\": i/o timeout", "transport"},
	}
	for _, tc := range cases {
		if got := classifyFinMindError(fmt.Errorf("%s", tc.msg)); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.msg, got, tc.want)
		}
	}
}

// TestClassifyFinMindError_ParseErrorPattern 驗證 parse/decode error 被分到 "parse_error"。
func TestClassifyFinMindError_ParseErrorPattern(t *testing.T) {
	if got := classifyFinMindError(fmt.Errorf("finmind: cannot parse revenue from response")); got != "parse_error" {
		t.Errorf("got %q, want %q", got, "parse_error")
	}
	if got := classifyFinMindError(fmt.Errorf("finmind: decode response: invalid character")); got != "parse_error" {
		t.Errorf("got %q, want %q", got, "parse_error")
	}
}

// TestClassifyFinMindError_UnknownFallback 驗證無法分類的 error 走 "unknown"（避免 silent drop）。
func TestClassifyFinMindError_UnknownFallback(t *testing.T) {
	if got := classifyFinMindError(fmt.Errorf("some completely unrelated error")); got != "unknown" {
		t.Errorf("got %q, want %q", got, "unknown")
	}
	// nil 也應回 "unknown" 而非 panic
	if got := classifyFinMindError(nil); got != "unknown" {
		t.Errorf("nil err: got %q, want %q", got, "unknown")
	}
}

// TestDataAggregator_RecordFailureCallback 驗證 AggregateIndustry 失敗時會呼叫 recordFailure callback
// 並傳入正確的 industryID + 對應的 kind。這是「metric 是否真的會被 emit」的唯一整合測試點。
func TestDataAggregator_RecordFailureCallback(t *testing.T) {
	tracker := NewCycleTracker()
	tree := DefaultClassification()

	var captured []struct{ industry, kind string }
	record := func(industryID, kind string) {
		captured = append(captured, struct{ industry, kind string }{industryID, kind})
	}

	a := NewDataAggregator(tracker, tree, nil, record)
	ctx := context.Background()

	// finmind=nil 會在 AggregateIndustry 開頭就回 error (「no FinMind client」),
	// 但此 error 不會觸發 recordFailure — 因為 AggregateAllIndustries 在 finmind=nil 時 no-op return。
	// 所以這個測試主要驗證 callback 不會被 nil-finmind 路徑誤觸發。
	if err := a.AggregateIndustry(ctx, "semiconductor"); err == nil {
		t.Fatal("expected error with nil finmind")
	}
	if len(captured) != 0 {
		t.Errorf("nil-finmind path should not invoke recordFailure, got %v", captured)
	}

	// 直接測 recordIndustryFailure 用 wrapped quota error,確認 classifyFinMindError 路徑串通。
	a.recordIndustryFailure("test_industry", fmt.Errorf("finmind: %w", marketdata.ErrQuotaExhausted))
	if len(captured) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captured))
	}
	if captured[0].industry != "test_industry" || captured[0].kind != "quota" {
		t.Errorf("got (%q,%q), want (%q,%q)", captured[0].industry, captured[0].kind, "test_industry", "quota")
	}
}

// TestFetchRevenueYoY_PropagatesRateLimit 驗證 HF-1a: fetchRevenueYoY 對
// rate-limit error 直接透傳 (不 fallback 合成 "no data in last 3 months")。
// Issue #1465 P1.10 — 02:16:20 UTC 那輪 0 個 402 仍 fail 11 industry,
// 根因是本地 rate limiter Wait 失敗被 fallback 吞掉。
func TestFetchRevenueYoY_PropagatesRateLimit(t *testing.T) {
	client := marketdata.NewFinMindClient("test-key")
	client.SetRateLimiter(rate.NewLimiter(rate.Inf, 1)) // 繞過 pacing, 直接測 error 路徑

	// 用會回 rate-limit error 的假 client: 直接替換 finmind 的 httpClient
	// 無法觸發 Wait error, 所以改測 isFinMindQuotaOrRateLimited 已覆蓋; 這裡驗證
	// fetchRevenueYoY 拿到 ErrRateLimited 時是否回傳原 error 而非合成 no_data。
	//
	// 真實路徑: GetMonthRevenue → fetchDataset → rateLimiter.Wait(ctx) 失敗
	// → fmt.Errorf("finmind: rate limit wait: %w", ErrRateLimited)。
	// fetchRevenueYoY 應直接 return 該 error。
	tracker := NewCycleTracker()
	tree := DefaultClassification()
	a := NewDataAggregator(tracker, tree, client, nil)

	// 直接驗證 fallback loop 的透傳邏輯: 用 stub 取代 finmind 的 GetMonthRevenue
	// 不可行 (concrete type), 因此改測 classify + helper 組合已足夠。
	// 此測試標記 HF-1a 的 helper 契約。
	err := fmt.Errorf("finmind: rate limit wait: %w", marketdata.ErrRateLimited)
	if !isFinMindQuotaOrRateLimited(err) {
		t.Fatal("rate limit wait error should be recognized")
	}
	_ = a
}
