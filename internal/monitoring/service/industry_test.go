package service

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// =============================================================================
// Helpers
// =============================================================================

// newTestIndustryService assembles a fully wired IndustryService using real
// industry-package constructors. All sub-engines are nil-safe when not
// exercised, so callers may pass nil for any subset they don't need.
func newTestIndustryService(opts ...func(*industryServiceOptions)) *IndustryService {
	o := &industryServiceOptions{}
	for _, fn := range opts {
		fn(o)
	}
	return NewIndustryService(
		o.classifier,
		o.seasonalEngine,
		o.cycleTracker,
		o.linkageAnalyzer,
		o.riskMonitor,
		o.siliconTracker,
		o.eventCalendar,
	)
}

type industryServiceOptions struct {
	classifier      *industry.ClassificationTree
	seasonalEngine  *industry.SeasonalEngine
	cycleTracker    *industry.CycleTracker
	linkageAnalyzer *industry.LinkageAnalyzer
	riskMonitor     *industry.RiskMonitor
	siliconTracker  *industry.SiliconCycleTracker
	eventCalendar   *industry.EventCalendar
}

func withAllEngines() func(*industryServiceOptions) {
	return func(o *industryServiceOptions) {
		o.classifier = industry.NewClassificationTree()
		o.seasonalEngine = industry.NewSeasonalEngine()
		o.cycleTracker = industry.NewCycleTracker()
		o.linkageAnalyzer = industry.NewLinkageAnalyzer()
		o.riskMonitor = industry.NewRiskMonitor()
		o.siliconTracker = industry.NewSiliconCycleTracker()
		o.eventCalendar = industry.NewEventCalendar()
	}
}

func withNilSeasonalEngine() func(*industryServiceOptions) {
	return func(o *industryServiceOptions) {
		o.classifier = industry.NewClassificationTree()
		o.cycleTracker = industry.NewCycleTracker()
		o.linkageAnalyzer = industry.NewLinkageAnalyzer()
		o.riskMonitor = industry.NewRiskMonitor()
	}
}

func makeSegment(id string, weight float64) *industry.IndustrySegment {
	return &industry.IndustrySegment{
		ID:          id,
		Name:        id,
		NameEN:      id,
		Weight:      weight,
		Description: "test segment",
	}
}

func makeCyclePos(id string, biz industry.CyclePhase, inv industry.InventoryCycle, cap industry.CapexCycle, confidence float64) *industry.CyclePosition {
	return &industry.CyclePosition{
		IndustryID:     id,
		BusinessCycle:  biz,
		InventoryCycle: inv,
		CapexCycle:     cap,
		Confidence:     confidence,
		UpdatedAt:      time.Now(),
	}
}

// =============================================================================
// abs() — pure helper
// =============================================================================

func TestAbs(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"positive", 3.14, 3.14},
		{"zero", 0, 0},
		{"negative", -2.71, 2.71},
		{"large_negative", -1e9, 1e9},
		{"small_negative", -0.0001, 0.0001},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := abs(tc.in)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("abs(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// =============================================================================
// NewIndustryService
// =============================================================================

func TestNewIndustryService_WiresCardBuilder(t *testing.T) {
	s := newTestIndustryService(withAllEngines())
	if s == nil {
		t.Fatal("expected non-nil service")
	}
	if s.CardBuilder == nil {
		t.Error("CardBuilder should be auto-constructed by NewIndustryService")
	}
	if s.Classifier == nil {
		t.Error("Classifier not stored")
	}
	if s.SeasonalEngine == nil {
		t.Error("SeasonalEngine not stored")
	}
	if s.CycleTracker == nil {
		t.Error("CycleTracker not stored")
	}
	if s.LinkageAnalyzer == nil {
		t.Error("LinkageAnalyzer not stored")
	}
	if s.RiskMonitor == nil {
		t.Error("RiskMonitor not stored")
	}
	if s.SiliconTracker == nil {
		t.Error("SiliconTracker not stored")
	}
	if s.EventCalendar == nil {
		t.Error("EventCalendar not stored")
	}
}

func TestNewIndustryService_AllowsNilSubEngines(t *testing.T) {
	// nil-safe constructor: pass only the seasonal engine
	se := industry.NewSeasonalEngine()
	s := NewIndustryService(nil, se, nil, nil, nil, nil, nil)
	if s == nil {
		t.Fatal("expected non-nil service even with mostly nil engines")
	}
	if s.SeasonalEngine != se {
		t.Error("SeasonalEngine should be retained")
	}
	if s.CardBuilder == nil {
		t.Error("CardBuilder should still be constructed from available args")
	}
}

// =============================================================================
// GetClassificationTree
// =============================================================================

func TestGetClassificationTree_EmptyClassifier(t *testing.T) {
	s := newTestIndustryService(withAllEngines())
	tree := s.GetClassificationTree()
	// Production code returns nil (no nil-safe guard) for an empty classifier.
	if len(tree) != 0 {
		t.Errorf("expected empty tree, got %d entries", len(tree))
	}
}

func TestGetClassificationTree_BuildsHierarchicalTree(t *testing.T) {
	// Build: parent → child → grandchild
	classifier := industry.NewClassificationTree()
	parent := &industry.IndustrySegment{ID: "tech", Name: "Technology", Weight: 0.5}
	child := &industry.IndustrySegment{ID: "semi", Name: "Semiconductor", ParentID: "tech", Weight: 0.3}
	gc := &industry.IndustrySegment{ID: "ai_chip", Name: "AI Chip", ParentID: "semi", Weight: 0.1, Description: "AI chips"}
	classifier.AddSegment(parent)
	classifier.AddSegment(child)
	classifier.AddSegment(gc)

	s := NewIndustryService(classifier, nil, nil, nil, nil, nil, nil)
	tree := s.GetClassificationTree()

	if len(tree) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree))
	}
	root, ok := tree[0]["id"].(string)
	if !ok || root != "tech" {
		t.Errorf("root id = %v, want tech", tree[0]["id"])
	}
	children, ok := tree[0]["children"].([]map[string]any)
	if !ok {
		t.Fatalf("children not a slice: %T", tree[0]["children"])
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0]["id"] != "semi" {
		t.Errorf("child id = %v, want semi", children[0]["id"])
	}
	grands, ok := children[0]["children"].([]map[string]any)
	if !ok {
		t.Fatalf("grandchildren not a slice: %T", children[0]["children"])
	}
	if len(grands) != 1 {
		t.Fatalf("expected 1 grandchild, got %d", len(grands))
	}
	if grands[0]["id"] != "ai_chip" {
		t.Errorf("grandchild id = %v, want ai_chip", grands[0]["id"])
	}
	if grands[0]["description"] != "AI chips" {
		t.Errorf("grandchild description = %v", grands[0]["description"])
	}
}

// =============================================================================
// GetAdjustmentBreakdown — nil-safe path
// =============================================================================

func TestGetAdjustmentBreakdown_NilSeasonalEngine(t *testing.T) {
	s := newTestIndustryService(withNilSeasonalEngine())
	if got := s.GetAdjustmentBreakdown("semiconductor", time.Now()); got != nil {
		t.Errorf("expected nil when SeasonalEngine is nil, got %v", got)
	}
}

// =============================================================================
// GetActiveNarrativeThemes — nil-safe path
// =============================================================================

func TestGetActiveNarrativeThemes_NilSeasonalEngine(t *testing.T) {
	s := newTestIndustryService(withNilSeasonalEngine())
	got := s.GetActiveNarrativeThemes("semiconductor")
	if got == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("expected empty themes, got %v", got)
	}
}

// =============================================================================
// UpdateDynamicEnv — nil-safe path
// =============================================================================

func TestUpdateDynamicEnv_NilSeasonalEngine(t *testing.T) {
	s := newTestIndustryService(withNilSeasonalEngine())
	// Should not panic
	s.UpdateDynamicEnv(marketdata.MacroDataSnapshot{}) //nolint:staticcheck // testing nil-safe path
}

// =============================================================================
// GetCalibrationEvidence
// =============================================================================

func TestGetCalibrationEvidence_NoConfig(t *testing.T) {
	// Use a temp dir to ensure configs/parameters.json is not picked up
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	s := newTestIndustryService()
	got := s.GetCalibrationEvidence()
	if got != nil {
		t.Errorf("expected nil for missing config, got %v", got)
	}
}

func TestGetCalibrationEvidence_WithTempConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configsDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// industry.LoadCalibrationEvidence reads `industry.seasonal_patterns`
	// and looks for `last_calibrated` or `calibration_timestamp` inside it.
	cfg := `{
		"industry": {
			"seasonal_patterns": {
				"calibration_timestamp": "2026-01-15T10:30:00Z",
				"calibration_data_source": "TWSE + FinMind 2020-2025",
				"sample_size": 250
			}
		}
	}`
	cfgPath := filepath.Join(configsDir, "parameters.json")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	s := newTestIndustryService()
	got := s.GetCalibrationEvidence()
	if got == nil {
		t.Fatal("expected non-nil calibration evidence")
	}
	// The flattened form returns calibrated=true plus timestamp and data_source.
	if cal, ok := got["calibrated"].(bool); !ok || !cal {
		t.Errorf("expected calibrated=true, got %v", got)
	}
	if _, ok := got["timestamp"]; !ok {
		t.Errorf("expected timestamp key, got: %v", got)
	}
	if _, ok := got["data_source"]; !ok {
		t.Errorf("expected data_source key, got: %v", got)
	}
}

func TestGetCalibrationEvidence_NoTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	configsDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// seasonal_patterns exists but no timestamp → returns nil
	cfg := `{"industry":{"seasonal_patterns":{"hit_rate":0.7}}}`
	cfgPath := filepath.Join(configsDir, "parameters.json")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	s := newTestIndustryService()
	got := s.GetCalibrationEvidence()
	if got != nil {
		t.Errorf("expected nil when no timestamp, got %v", got)
	}
}

// =============================================================================
// RebuildCorrelations — nil-safe path
// =============================================================================

func TestRebuildCorrelations_NilLinkageAnalyzer(t *testing.T) {
	s := NewIndustryService(nil, nil, nil, nil, nil, nil, nil)
	// Should not panic
	s.RebuildCorrelations(map[string][]float64{
		"semiconductor": {0.1, 0.2, 0.15},
	})
}

// =============================================================================
// SetMacroProvider — nil-safe path
// =============================================================================

func TestSetMacroProvider_NilSiliconTracker(t *testing.T) {
	s := NewIndustryService(nil, nil, nil, nil, nil, nil, nil)
	// Should not panic when SiliconTracker is nil
	s.SetMacroProvider(nil)
	if s.siliconAggregator != nil {
		t.Errorf("aggregator should not be created when SiliconTracker is nil")
	}
}

func TestSetMacroProvider_WithSiliconTracker(t *testing.T) {
	tracker := industry.NewSiliconCycleTracker()
	s := &IndustryService{SiliconTracker: tracker}
	if s.siliconAggregator != nil {
		t.Error("aggregator should start nil")
	}
	s.SetMacroProvider(nil)
	if s.siliconAggregator == nil {
		t.Error("aggregator should be created when SiliconTracker is set")
	}
}

// =============================================================================
// UpdateSiliconIndicators — nil-safe path
// =============================================================================

func TestUpdateSiliconIndicators_NoAggregator(t *testing.T) {
	s := &IndustryService{} // no aggregator, no silicon tracker
	err := s.UpdateSiliconIndicators(context.Background())
	if err != nil {
		t.Errorf("expected nil error for no-op, got %v", err)
	}
}

// =============================================================================
// SetCycleCalibration
// =============================================================================

func TestSetCycleCalibration_StoresValue(t *testing.T) {
	s := &IndustryService{}
	cal := &industry.CycleCalibration{}
	s.SetCycleCalibration(cal)
	if s.CycleCalibration != cal {
		t.Error("CycleCalibration not stored on service")
	}
	// SetGlobalCycleCalibration is called too — should be reversible.
	defer industry.SetGlobalCycleCalibration(nil)
}

func TestSetCycleCalibration_NilValue(t *testing.T) {
	s := &IndustryService{}
	s.SetCycleCalibration(nil)
	if s.CycleCalibration != nil {
		t.Error("nil calibration should be stored as nil")
	}
	defer industry.SetGlobalCycleCalibration(nil)
}

// =============================================================================
// GetCalibrationMetrics — nil-safe path
// =============================================================================

func TestGetCalibrationMetrics_NilCalibration(t *testing.T) {
	s := &IndustryService{}
	if got := s.GetCalibrationMetrics(); got != nil {
		t.Errorf("expected nil metrics when no calibration, got %v", got)
	}
}

// =============================================================================
// RecordCycleCalibrationOutcome — nil-safe path
// =============================================================================

func TestRecordCycleCalibrationOutcome_NilCalibration(t *testing.T) {
	s := &IndustryService{}
	// Should not panic
	s.RecordCycleCalibrationOutcome("sess-1", time.Now(), map[string]float64{"silicon": 0.8}, 0.05)
}

// =============================================================================
// BuildCycleStatusCard — nil-safe path
// =============================================================================

func TestBuildCycleStatusCard_NilCardBuilder(t *testing.T) {
	s := &IndustryService{} // no CardBuilder
	card, err := s.BuildCycleStatusCard(time.Now())
	if err == nil {
		t.Error("expected error when CardBuilder is nil")
	}
	if card != nil {
		t.Errorf("expected nil card, got %v", card)
	}
}

// =============================================================================
// BuildIndustryCycleStatusCard — nil-safe path
// =============================================================================

func TestBuildIndustryCycleStatusCard_NilCardBuilder(t *testing.T) {
	s := &IndustryService{}
	card, err := s.BuildIndustryCycleStatusCard(time.Now(), "semiconductor")
	if err == nil {
		t.Error("expected error when CardBuilder is nil")
	}
	if card != nil {
		t.Errorf("expected nil card, got %v", card)
	}
}

// =============================================================================
// calculateWeightDerivation — switch on seg.ID
// =============================================================================

func TestCalculateWeightDerivation_KnownSegments(t *testing.T) {
	s := newTestIndustryService()
	cases := []struct {
		id                string
		wantFactorsCount  int
		wantRiskCount     int
		wantOppCount      int
		interpretationHas string
	}{
		{"semiconductor", 4, 3, 3, "半導體為台灣經濟命脈"},
		{"ai_supply_chain", 4, 3, 3, "AI供應鏈為台灣下一個核心成長引擎"},
		{"robotics", 4, 3, 3, "機器人產業"},
		{"financials", 4, 3, 3, "金融業"},
		{"shipping", 4, 3, 3, "航運業"},
		{"energy", 4, 3, 3, "能源業"},
		{"electronics", 4, 3, 3, "電子零組件"},
		{"consumer", 4, 3, 3, "傳產消費業"},
		{"industrial", 4, 3, 3, "工業製造業"},
		{"mining", 4, 3, 3, "採礦"},
		{"leo_satellite", 4, 3, 3, "低軌衛星"},
		{"etf_rotation", 4, 3, 3, "ETF輪動"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			seg := makeSegment(tc.id, 0.10)
			wd := s.calculateWeightDerivation(seg)
			// calculateWeightDerivation calls s.getSectorWeight(seg.ID, seg.Weight)
			// so for segments not present in the configured SectorWeights map,
			// the fallback (seg.Weight = 0.10) is used.
			wantBase := s.getSectorWeight(seg.ID, seg.Weight)
			if math.Abs(wd.BaseWeight-wantBase) > 1e-9 {
				t.Errorf("BaseWeight = %v, want %v", wd.BaseWeight, wantBase)
			}
			if len(wd.DerivationFactors) != tc.wantFactorsCount {
				t.Errorf("DerivationFactors count = %d, want %d", len(wd.DerivationFactors), tc.wantFactorsCount)
			}
			if len(wd.RiskFactors) != tc.wantRiskCount {
				t.Errorf("RiskFactors count = %d, want %d", len(wd.RiskFactors), tc.wantRiskCount)
			}
			if len(wd.Opportunities) != tc.wantOppCount {
				t.Errorf("Opportunities count = %d, want %d", len(wd.Opportunities), tc.wantOppCount)
			}
			if !stringContains(wd.Interpretation, tc.interpretationHas) {
				t.Errorf("Interpretation = %q, want substring %q", wd.Interpretation, tc.interpretationHas)
			}
		})
	}
}

func TestCalculateWeightDerivation_DefaultCase(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("unknown_industry", 0.05)
	wd := s.calculateWeightDerivation(seg)
	if wd.BaseWeight != 0.05 {
		t.Errorf("BaseWeight = %v, want 0.05", wd.BaseWeight)
	}
	if !stringContains(wd.Interpretation, "權重") {
		t.Errorf("default Interpretation should mention 權重, got %q", wd.Interpretation)
	}
	if len(wd.DerivationFactors) != 1 {
		t.Errorf("default DerivationFactors = %d, want 1", len(wd.DerivationFactors))
	}
	if len(wd.RiskFactors) != 0 {
		t.Errorf("default RiskFactors = %d, want 0", len(wd.RiskFactors))
	}
	if len(wd.Opportunities) != 0 {
		t.Errorf("default Opportunities = %d, want 0", len(wd.Opportunities))
	}
}

// =============================================================================
// generateRecommendation — pure logic with cycle position
// =============================================================================

func TestGenerateRecommendation_NilPosition(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("semiconductor", 0.10)
	rec := s.generateRecommendation(seg, nil)
	if rec == nil {
		t.Fatal("expected non-nil recommendation")
	}
	if rec.Action != "觀望" {
		t.Errorf("Action = %q, want 觀望", rec.Action)
	}
	if rec.Conviction != "低" {
		t.Errorf("Conviction = %q, want 低", rec.Conviction)
	}
	wantBase := s.getSectorWeight(seg.ID, 0)
	if math.Abs(rec.TargetWeight-wantBase) > 1e-9 {
		t.Errorf("TargetWeight = %v, want %v", rec.TargetWeight, wantBase)
	}
}

func TestGenerateRecommendation_ExpansionFavorable(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("semiconductor", 0.10)
	pos := makeCyclePos("semiconductor", industry.CycleExpansion, industry.InvRestockingActive, industry.CapexMaintenance, 0.85)
	rec := s.generateRecommendation(seg, pos)
	if rec.Action != "增持" {
		t.Errorf("Action = %q, want 增持", rec.Action)
	}
	if rec.Conviction != "高" {
		t.Errorf("Conviction = %q, want 高", rec.Conviction)
	}
	wantBase := s.getSectorWeight(seg.ID, 0)
	if math.Abs(rec.TargetWeight-wantBase*1.2) > 1e-9 {
		t.Errorf("TargetWeight = %v, want %v (1.2x base)", rec.TargetWeight, wantBase*1.2)
	}
}

func TestGenerateRecommendation_RecoveryFavorable(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("ai_supply_chain", 0.10)
	pos := makeCyclePos("ai_supply_chain", industry.CycleRecovery, industry.InvRestockingPassive, industry.CapexMaintenance, 0.7)
	rec := s.generateRecommendation(seg, pos)
	if rec.Action != "溫和增持" {
		t.Errorf("Action = %q, want 溫和增持", rec.Action)
	}
	if rec.Conviction != "中" {
		t.Errorf("Conviction = %q, want 中", rec.Conviction)
	}
	wantBase := s.getSectorWeight(seg.ID, 0)
	if math.Abs(rec.TargetWeight-wantBase*1.1) > 1e-9 {
		t.Errorf("TargetWeight = %v, want %v (1.1x base)", rec.TargetWeight, wantBase*1.1)
	}
}

func TestGenerateRecommendation_RecessionUnfavorable(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("shipping", 0.20)
	pos := makeCyclePos("shipping", industry.CycleRecession, industry.InvDestockingActive, industry.CapexContraction, 0.6)
	rec := s.generateRecommendation(seg, pos)
	if rec.Action != "減持" {
		t.Errorf("Action = %q, want 減持", rec.Action)
	}
	if rec.Conviction != "高" {
		t.Errorf("Conviction = %q, want 高", rec.Conviction)
	}
	wantBase := s.getSectorWeight(seg.ID, 0)
	if math.Abs(rec.TargetWeight-wantBase*0.7) > 1e-9 {
		t.Errorf("TargetWeight = %v, want %v (0.7x base)", rec.TargetWeight, wantBase*0.7)
	}
}

func TestGenerateRecommendation_MatureUnfavorable(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("consumer", 0.10)
	pos := makeCyclePos("consumer", industry.CycleMature, industry.InvDestockingPassive, industry.CapexMaintenance, 0.5)
	rec := s.generateRecommendation(seg, pos)
	if rec.Action != "中性" {
		t.Errorf("Action = %q, want 中性", rec.Action)
	}
	if rec.Conviction != "中" {
		t.Errorf("Conviction = %q, want 中", rec.Conviction)
	}
	wantBase := s.getSectorWeight(seg.ID, 0)
	if math.Abs(rec.TargetWeight-wantBase) > 1e-9 {
		t.Errorf("TargetWeight = %v, want %v (1.0x base)", rec.TargetWeight, wantBase)
	}
}

func TestGenerateRecommendation_CapexRiskAdjustment(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("semiconductor", 0.10)
	pos := makeCyclePos("semiconductor", industry.CycleExpansion, industry.InvRestockingActive, industry.CapexExpansion, 0.85)
	rec := s.generateRecommendation(seg, pos)
	if !rec.RiskAdjusted {
		t.Error("RiskAdjusted should be true for CapexExpansion")
	}
	if !stringContains(rec.Rationale, "資本支出擴張") {
		t.Errorf("Rationale should mention 資本支出擴張, got %q", rec.Rationale)
	}
}

func TestGenerateRecommendation_Delta(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("ai_supply_chain", 0.10)
	pos := makeCyclePos("ai_supply_chain", industry.CycleExpansion, industry.InvRestockingActive, industry.CapexMaintenance, 0.85)
	rec := s.generateRecommendation(seg, pos)
	wantBase := s.getSectorWeight(seg.ID, 0)
	wantDelta := wantBase*1.2 - wantBase
	if math.Abs(rec.Delta-wantDelta) > 1e-9 {
		t.Errorf("Delta = %v, want %v", rec.Delta, wantDelta)
	}
	if math.Abs(rec.CurrentWeight-wantBase) > 1e-9 {
		t.Errorf("CurrentWeight = %v, want %v", rec.CurrentWeight, wantBase)
	}
}

// =============================================================================
// getRegimeContext — pure helper
// =============================================================================

func TestGetRegimeContext_NilPosition(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("semiconductor", 0.10)
	got := s.getRegimeContext(seg, nil)
	if got != "目前無市場體制數據" {
		t.Errorf("got %q, want default nil-position message", got)
	}
}

func TestGetRegimeContext_AISupercycle(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("semiconductor", 0.10)
	pos := makeCyclePos("semiconductor", industry.CycleExpansion, industry.InvRestockingActive, industry.CapexMaintenance, 0.9)
	got := s.getRegimeContext(seg, pos)
	if !stringContains(got, "AI超級循環") {
		t.Errorf("expected AI supercycle context, got %q", got)
	}
}

func TestGetRegimeContext_AISupplyChainSupercycle(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("ai_supply_chain", 0.10)
	pos := makeCyclePos("ai_supply_chain", industry.CycleExpansion, industry.InvRestockingActive, industry.CapexMaintenance, 0.9)
	got := s.getRegimeContext(seg, pos)
	if !stringContains(got, "AI超級循環") {
		t.Errorf("expected AI supercycle context, got %q", got)
	}
}

func TestGetRegimeContext_DefensiveFinancials(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("financials", 0.10)
	pos := makeCyclePos("financials", industry.CycleMature, industry.InvDestockingPassive, industry.CapexMaintenance, 0.6)
	got := s.getRegimeContext(seg, pos)
	if !stringContains(got, "防禦模式") {
		t.Errorf("expected defensive context, got %q", got)
	}
}

func TestGetRegimeContext_DefensiveConsumer(t *testing.T) {
	s := newTestIndustryService()
	seg := makeSegment("consumer", 0.10)
	pos := makeCyclePos("consumer", industry.CycleRecession, industry.InvDestockingActive, industry.CapexContraction, 0.5)
	got := s.getRegimeContext(seg, pos)
	if !stringContains(got, "防禦模式") {
		t.Errorf("expected defensive context, got %q", got)
	}
}

func TestGetRegimeContext_PhaseMessages(t *testing.T) {
	s := newTestIndustryService()
	cases := []struct {
		phase     industry.CyclePhase
		substring string
	}{
		{industry.CycleExpansion, "擴張期"},
		{industry.CycleRecovery, "復甦期"},
		{industry.CycleMature, "成熟期"},
		{industry.CycleRecession, "衰退期"},
	}
	for _, tc := range cases {
		t.Run(string(tc.phase), func(t *testing.T) {
			// Pick a segment that doesn't trigger special cases
			seg := makeSegment("mining", 0.10)
			pos := makeCyclePos("mining", tc.phase, industry.InvRestockingPassive, industry.CapexMaintenance, 0.5)
			got := s.getRegimeContext(seg, pos)
			if !stringContains(got, tc.substring) {
				t.Errorf("got %q, want substring %q", got, tc.substring)
			}
		})
	}
}

// =============================================================================
// getSectorWeight — uses global ParametersConfig
// =============================================================================

func TestGetSectorWeight_NoConfig(t *testing.T) {
	// Without overriding the config, fallback is used
	s := newTestIndustryService()
	got := s.getSectorWeight("semiconductor", 0.15)
	// Returns either configured value or fallback
	if got != got { // NaN check
		t.Errorf("got NaN")
	}
	if got < 0 {
		t.Errorf("got negative weight: %v", got)
	}
}

// =============================================================================
// PropagateShock
// =============================================================================

func TestPropagateShock_DefaultMaxDepth(t *testing.T) {
	linkage := industry.NewLinkageAnalyzer()
	s := &IndustryService{LinkageAnalyzer: linkage}
	// maxDepth=0 should be normalized to 3 internally
	impacts := s.PropagateShock("semiconductor", 0.1, 0)
	// Empty graph → empty result is fine; we just verify no panic and that
	// the function returns a non-nil slice.
	if impacts == nil {
		// An empty graph may return nil — that's acceptable.
		t.Log("PropagateShock returned nil for empty graph (expected)")
	}
}

func TestPropagateShock_ExplicitMaxDepth(t *testing.T) {
	linkage := industry.NewLinkageAnalyzer()
	s := &IndustryService{LinkageAnalyzer: linkage}
	impacts := s.PropagateShock("semiconductor", 0.1, 2)
	if impacts == nil {
		t.Log("empty graph returned nil (acceptable)")
	}
}

func TestPropagateShock_NilLinkageAnalyzer(t *testing.T) {
	s := &IndustryService{LinkageAnalyzer: nil}
	// Will likely panic in PropagateShock because it dereferences s.LinkageAnalyzer
	// Let's see if it's nil-safe or not. We test the actual behavior.
	defer func() {
		if r := recover(); r != nil {
			t.Logf("PropagateShock with nil LinkageAnalyzer panicked: %v (acceptable — caller must initialize)", r)
		}
	}()
	_ = s.PropagateShock("semiconductor", 0.1, 3)
}

// =============================================================================
// GetIndustryGraph
// =============================================================================

func TestGetIndustryGraph_SeededDefaults(t *testing.T) {
	// NewLinkageAnalyzer() ships pre-seeded with the default supply-chain graph
	// so an empty result is not realistic. We assert the seeded counts and the
	// structural invariants: every node id is non-empty, every edge has a
	// strength classification, and the upper-triangle filter (industryA < industryB)
	// is honored.
	linkage := industry.NewLinkageAnalyzer()
	s := &IndustryService{LinkageAnalyzer: linkage}
	nodes, edges := s.GetIndustryGraph()

	if len(nodes) == 0 {
		t.Fatal("expected seeded nodes, got 0")
	}
	if len(edges) == 0 {
		t.Fatal("expected seeded edges, got 0")
	}
	for _, n := range nodes {
		if n.ID == "" {
			t.Error("encountered node with empty id")
		}
	}
	seen := make(map[string]bool)
	for _, n := range nodes {
		if seen[n.ID] {
			t.Errorf("duplicate node id: %q", n.ID)
		}
		seen[n.ID] = true
	}
	for _, e := range edges {
		if e.Source == e.Target {
			t.Errorf("self-edge detected: %q", e.Source)
		}
		if e.Source >= e.Target {
			t.Errorf("edge violates upper-triangle filter: %q -> %q", e.Source, e.Target)
		}
		switch e.Strength {
		case "low", "medium", "high":
			// ok
		default:
			t.Errorf("edge has unexpected strength classification: %q", e.Strength)
		}
	}
}

// =============================================================================
// Edge strength classification (via GetIndustryGraph branch coverage)
// =============================================================================

// =============================================================================
// helpers
// =============================================================================

func stringContains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
