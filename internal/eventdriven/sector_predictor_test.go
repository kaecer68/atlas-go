package eventdriven

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// ── Helpers ────────────────────────────────────────────────────────────

func testMacroSnapshot() *marketdata.MacroDataSnapshot {
	return &marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{Symbol: "DXY", Value: 104.5, ChangePct: -0.3},
		US10Y:              marketdata.MacroDataPoint{Symbol: "US10Y", Value: 4.25, ChangePct: 0.06},
		TSMADR:             marketdata.MacroDataPoint{Symbol: "TSM", Value: 180.0, ChangePct: 2.5},
		NVDA:               marketdata.MacroDataPoint{Symbol: "NVDA", Value: 120.0, ChangePct: 1.8},
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "FIN", Value: 5.2, ChangePct: 1.2},
		Bdi:                marketdata.MacroDataPoint{Symbol: "BDI", Value: 2100.0, ChangePct: 1.5},
		SOXIndex:           marketdata.MacroDataPoint{Symbol: "SOX", Value: 5200.0, ChangePct: 1.2},
		TAIEX:              marketdata.MacroDataPoint{Symbol: "TAIEX", Value: 23500.0, ChangePct: 0.8},
		DataStatus:         "ok",
	}
}

func testFlowPredictions() []FlowPrediction {
	base := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	var out []FlowPrediction
	for i := 0; i < 5; i++ {
		day := base.AddDate(0, 0, i)
		out = append(out, FlowPrediction{
			Date:       day,
			Direction:  "inflow",
			Confidence: 0.65,
			Distribution: PredictionDistribution{
				Inflow:  0.55,
				Neutral: 0.30,
				Outflow: 0.15,
			},
		})
	}
	return out
}

func testActiveEvents() []EventCalendarItem {
	return []EventCalendarItem{
		{
			Name:               "MSCI Rebalance",
			EventType:          string(industry.EventMSCIRebalance),
			Direction:          "bullish",
			Confidence:         0.7,
			AffectedIndustries: []string{"semiconductor", "electronics"},
		},
		{
			Name:               "ETF Outflow",
			EventType:          string(industry.EventExDividend),
			Direction:          "bearish",
			Confidence:         0.5,
			AffectedIndustries: []string{"financials"},
		},
	}
}

// ── Tests ──────────────────────────────────────────────────────────────

func TestSectorPredictor_NilProviders_ReturnsPredictions(t *testing.T) {
	sp := NewSectorPredictor(nil, nil)
	preds := testFlowPredictions()
	events := testActiveEvents()
	result := sp.Predict(preds, events)

	if len(result) != 5 {
		t.Fatalf("expected 5 days, got %d", len(result))
	}
	for i, sdp := range result {
		if len(sdp.Sectors) != 20 {
			t.Errorf("day %d: expected 20 sectors, got %d", i, len(sdp.Sectors))
		}
		if sdp.Date == "" {
			t.Errorf("day %d: expected non-empty date", i)
		}
	}
}

func TestSectorPredictor_DistributionSumsToOne(t *testing.T) {
	sp := NewSectorPredictor(testMacroSnapshot(), nil)
	preds := testFlowPredictions()
	events := testActiveEvents()
	result := sp.Predict(preds, events)

	for _, sdp := range result {
		for _, sec := range sdp.Sectors {
			sum := sec.Distribution.Inflow + sec.Distribution.Neutral + sec.Distribution.Outflow
			if math.Abs(sum-1.0) > 1e-6 {
				t.Errorf("sector %s day %s: distribution sums to %.6f, want 1.0 (±1e-6)",
					sec.SectorID, sdp.Date, sum)
			}
		}
	}
}

func TestSectorPredictor_ConfidenceFloor(t *testing.T) {
	sp := NewSectorPredictor(nil, nil)
	preds := testFlowPredictions()
	events := testActiveEvents()
	result := sp.Predict(preds, events)

	for _, sdp := range result {
		for _, sec := range sdp.Sectors {
			if sec.Confidence < 0.40 {
				t.Errorf("sector %s day %s: confidence %.4f < 0.40 floor",
					sec.SectorID, sdp.Date, sec.Confidence)
			}
			if sec.Confidence > 1.0 {
				t.Errorf("sector %s day %s: confidence %.4f > 1.0",
					sec.SectorID, sdp.Date, sec.Confidence)
			}
		}
	}
}

func TestSectorPredictor_ValidDirections(t *testing.T) {
	sp := NewSectorPredictor(testMacroSnapshot(), nil)
	preds := testFlowPredictions()
	events := testActiveEvents()
	result := sp.Predict(preds, events)

	valid := map[string]bool{"inflow": true, "outflow": true, "neutral": true}
	for _, sdp := range result {
		for _, sec := range sdp.Sectors {
			if !valid[sec.Direction] {
				t.Errorf("sector %s day %s: invalid direction %q",
					sec.SectorID, sdp.Date, sec.Direction)
			}
		}
	}
}

func TestSectorPredictor_CanonicalSectorIDs(t *testing.T) {
	sp := NewSectorPredictor(nil, nil)
	preds := testFlowPredictions()
	result := sp.Predict(preds, []EventCalendarItem{})

	l1Set := make(map[string]bool)
	for _, id := range industry.L1Sectors() {
		l1Set[string(id)] = true
	}

	for _, sdp := range result {
		for _, sec := range sdp.Sectors {
			if !l1Set[sec.SectorID] {
				t.Errorf("non-canonical sector_id %q on day %s", sec.SectorID, sdp.Date)
			}
		}
	}
}

func TestSectorPredictor_FixedSectorOrder(t *testing.T) {
	sp := NewSectorPredictor(nil, nil)
	preds := testFlowPredictions()
	result := sp.Predict(preds, []EventCalendarItem{})

	expected := industry.L1Sectors()
	for i, sdp := range result {
		for j, sec := range sdp.Sectors {
			if sec.SectorID != string(expected[j]) {
				t.Errorf("day %d sector[%d]: expected %s, got %s (order mismatch)",
					i, j, expected[j], sec.SectorID)
			}
		}
	}
}

func TestSectorPredictor_All20L1SectorsPerDay(t *testing.T) {
	sp := NewSectorPredictor(nil, nil)
	preds := testFlowPredictions()
	result := sp.Predict(preds, []EventCalendarItem{})

	for i, sdp := range result {
		if len(sdp.Sectors) != 20 {
			t.Errorf("day %d: expected 20 L1 sectors, got %d", i, len(sdp.Sectors))
		}
	}
}

func TestSectorPredictor_EachSectorHasDisplayName(t *testing.T) {
	sp := NewSectorPredictor(nil, nil)
	preds := testFlowPredictions()
	result := sp.Predict(preds, []EventCalendarItem{})

	for _, sdp := range result {
		for _, sec := range sdp.Sectors {
			if sec.SectorName == "" {
				t.Errorf("sector %s: empty sector_name on day %s", sec.SectorID, sdp.Date)
			}
		}
	}
}

func TestSectorPredictor_DriversNotEmpty(t *testing.T) {
	sp := NewSectorPredictor(testMacroSnapshot(), nil)
	preds := testFlowPredictions()
	events := testActiveEvents()
	result := sp.Predict(preds, events)

	// With overall baseline + events + macro, at least one driver should exist.
	hasDrivers := false
	for _, sdp := range result {
		for _, sec := range sdp.Sectors {
			if len(sec.Drivers) > 0 {
				hasDrivers = true
			}
		}
	}
	if !hasDrivers {
		t.Error("expected at least one sector to have drivers with macro+events input")
	}
}

func TestSectorPredictor_DriversMaxTwo(t *testing.T) {
	sp := NewSectorPredictor(testMacroSnapshot(), nil)
	preds := testFlowPredictions()
	events := testActiveEvents()
	result := sp.Predict(preds, events)

	for _, sdp := range result {
		for _, sec := range sdp.Sectors {
			if len(sec.Drivers) > 2 {
				t.Errorf("sector %s day %s: %d drivers > 2 max",
					sec.SectorID, sdp.Date, len(sec.Drivers))
			}
		}
	}
}

func TestSectorPredictor_EmptyEvents_StillReturns20Sectors(t *testing.T) {
	sp := NewSectorPredictor(nil, nil)
	preds := testFlowPredictions()
	result := sp.Predict(preds, []EventCalendarItem{})

	for i, sdp := range result {
		if len(sdp.Sectors) != 20 {
			t.Errorf("day %d with empty events: expected 20 sectors, got %d", i, len(sdp.Sectors))
		}
	}
}

func TestSectorPredictor_ZeroMacroData_GracefulDegrade(t *testing.T) {
	zeroMacro := &marketdata.MacroDataSnapshot{}
	sp := NewSectorPredictor(zeroMacro, nil)
	preds := testFlowPredictions()
	result := sp.Predict(preds, testActiveEvents())

	for _, sdp := range result {
		for _, sec := range sdp.Sectors {
			if sec.Confidence < 0.40 {
				t.Errorf("sector %s day %s: confidence %.4f below floor with zero macro",
					sec.SectorID, sdp.Date, sec.Confidence)
			}
		}
	}
}

func TestSectorPredictor_DifferentOverallDirections(t *testing.T) {
	sp := NewSectorPredictor(nil, nil)

	base := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	dirs := []struct {
		dir  string
		dist PredictionDistribution
	}{
		{"inflow", PredictionDistribution{Inflow: 0.60, Neutral: 0.25, Outflow: 0.15}},
		{"outflow", PredictionDistribution{Outflow: 0.60, Neutral: 0.25, Inflow: 0.15}},
		{"neutral", PredictionDistribution{Neutral: 0.60, Inflow: 0.20, Outflow: 0.20}},
	}

	for _, tc := range dirs {
		preds := []FlowPrediction{
			{
				Date:         base,
				Direction:    tc.dir,
				Confidence:   0.65,
				Distribution: tc.dist,
			},
		}
		result := sp.Predict(preds, []EventCalendarItem{})
		if len(result) == 0 {
			t.Fatalf("no results for direction %s", tc.dir)
		}
	}
}

func TestSectorPredictor_JSDConsistencyCheck(t *testing.T) {
	// Create two distributions with high divergence to trigger JSD > 0.25.
	// sector prediction strongly inflow, overall prediction neutral.
	sp := NewSectorPredictor(nil, nil)

	base := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	preds := []FlowPrediction{
		{
			Date:         base,
			Direction:    "neutral",
			Confidence:   0.50,
			Distribution: PredictionDistribution{Inflow: 0.30, Neutral: 0.50, Outflow: 0.20},
		},
	}

	// Events push heavily bullish → sectors will lean inflow.
	events := []EventCalendarItem{
		{
			Name:               "Strong Bullish Event",
			Direction:          "bullish",
			Confidence:         0.9,
			AffectedIndustries: []string{"semiconductor", "electronics", "financials"},
		},
	}
	result := sp.Predict(preds, events)

	// Just verify output is well-formed. JSD is computed internally;
	// we verify confidence is not NaN/inf and drivers are present.
	for _, sdp := range result {
		for _, sec := range sdp.Sectors {
			if math.IsNaN(sec.Confidence) || math.IsInf(sec.Confidence, 0) {
				t.Errorf("sector %s: confidence is NaN/Inf", sec.SectorID)
			}
			if sec.Confidence < 0.40 || sec.Confidence > 1.0 {
				t.Errorf("sector %s: confidence %.4f out of [0.40, 1.0]", sec.SectorID, sec.Confidence)
			}
		}
	}
}

func TestSectorPredictor_NoPanicOnNilProviders(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic with nil providers: %v", r)
		}
	}()
	sp := NewSectorPredictor(nil, nil)
	preds := testFlowPredictions()
	_ = sp.Predict(preds, testActiveEvents())
}

func TestSectorPredictor_SoftmaxConsistency(t *testing.T) {
	// Verify softmax produces valid distributions for various inputs.
	tests := []struct {
		a, b, c float64
	}{
		{1.0, 0.5, 0.3},
		{0, 0, 0},
		{5.0, 2.0, 0.1},
		{-1.0, -2.0, -3.0},
		{0.5, 0.5, 0.5},
	}

	for _, tc := range tests {
		dist := softmax3(tc.a, tc.b, tc.c)
		sum := dist.Inflow + dist.Neutral + dist.Outflow
		if math.Abs(sum-1.0) > 1e-6 {
			t.Errorf("softmax3(%.2f, %.2f, %.2f): sum = %.6f, want 1.0", tc.a, tc.b, tc.c, sum)
		}
		if dist.Inflow < 0 || dist.Neutral < 0 || dist.Outflow < 0 {
			t.Errorf("softmax3(%.2f, %.2f, %.2f): negative probabilities", tc.a, tc.b, tc.c)
		}
	}
}

func TestSectorPredictor_DirectionConfidence(t *testing.T) {
	tests := []struct {
		dist    PredictionDistribution
		wantDir string
	}{
		{PredictionDistribution{Inflow: 0.55, Neutral: 0.30, Outflow: 0.15}, "inflow"},
		{PredictionDistribution{Inflow: 0.15, Neutral: 0.30, Outflow: 0.55}, "outflow"},
		{PredictionDistribution{Inflow: 0.30, Neutral: 0.40, Outflow: 0.30}, "neutral"},
		{PredictionDistribution{Inflow: 0.33, Neutral: 0.34, Outflow: 0.33}, "neutral"},
	}

	for _, tc := range tests {
		dir, conf := directionConfidence(tc.dist)
		if dir != tc.wantDir {
			t.Errorf("directionConfidence(%+v): dir = %s, want %s", tc.dist, dir, tc.wantDir)
		}
		if conf < 0.40 || conf > 1.0 {
			t.Errorf("directionConfidence(%+v): conf %.4f out of [0.40, 1.0]", tc.dist, conf)
		}
	}
}

func TestSectorPredictor_SetProviders(t *testing.T) {
	sp := NewSectorPredictor(nil, nil)
	if sp.macro != nil {
		t.Error("expected nil macro after NewSectorPredictor(nil, nil)")
	}
	if sp.cycle != nil {
		t.Error("expected nil cycle after NewSectorPredictor(nil, nil)")
	}

	m := testMacroSnapshot()
	sp.SetMacroSnapshot(m)
	if sp.macro != m {
		t.Error("SetMacroSnapshot did not set macro")
	}

	sp.SetCycleProvider(nil)
	if sp.cycle != nil {
		t.Error("SetCycleProvider(nil) should set cycle to nil")
	}
}

func TestSectorPredictor_JSD_Identical(t *testing.T) {
	// Two identical distributions → JSD should be 0.
	a := PredictionDistribution{Inflow: 0.55, Neutral: 0.30, Outflow: 0.15}
	b := PredictionDistribution{Inflow: 0.55, Neutral: 0.30, Outflow: 0.15}
	d := jsd(a, b)
	if d > 1e-6 {
		t.Errorf("JSD of identical distributions = %.6f, want 0", d)
	}
}

func TestSectorPredictor_JSD_Divergent(t *testing.T) {
	a := PredictionDistribution{Inflow: 0.90, Neutral: 0.05, Outflow: 0.05}
	b := PredictionDistribution{Inflow: 0.05, Neutral: 0.05, Outflow: 0.90}
	d := jsd(a, b)
	if d < 0.4 {
		t.Errorf("JSD of divergent distributions = %.6f, expected > 0.4", d)
	}
}

func TestSectorPredictor_SectorWeightsSumToOne(t *testing.T) {
	// SA02: 驗證 inject 的 StrategicSectorPrior 在 sector_predictor 內的 baseline sum=1。
	sp := NewSectorPredictor(nil, nil)
	prior, err := sectorallocation.LoadStrategicPrior(config.GetParametersConfig())
	if err != nil {
		t.Fatalf("LoadStrategicPrior failed: %v", err)
	}
	sp.SetStrategicPrior(prior)
	var sum float64
	for _, sid := range industry.L1Sectors() {
		sum += sp.PriorWeight(sid)
	}
	if math.Abs(sum-1.0) > 1e-6 {
		t.Errorf("prior weights sum to %.6f, want 1.0", sum)
	}
}

func TestPredictor_SectorPredictionsField_AlwaysPresent(t *testing.T) {
	p := NewPredictor(industry.NewEventCalendar())
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	report := p.Predict(now)

	// Without SectorPredictor wired, field must be present as empty slice.
	if report.SectorPredictions == nil {
		t.Error("SectorPredictions must not be nil (always present)")
	}
	if len(report.SectorPredictions) != 0 {
		t.Errorf("expected empty SectorPredictions without sector predictor, got %d", len(report.SectorPredictions))
	}
}

func TestPredictor_SectorPredictions_WithPredictor(t *testing.T) {
	p := NewPredictor(industry.NewEventCalendar())
	sp := NewSectorPredictor(testMacroSnapshot(), nil)
	p.SetSectorPredictor(sp)

	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	report := p.Predict(now)

	if report.SectorPredictions == nil {
		t.Fatal("SectorPredictions must not be nil when SectorPredictor is wired")
	}
	if len(report.SectorPredictions) != 5 {
		t.Errorf("expected 5 days of sector predictions, got %d", len(report.SectorPredictions))
	}
	for i, sdp := range report.SectorPredictions {
		if len(sdp.Sectors) != 20 {
			t.Errorf("day %d: expected 20 sectors, got %d", i, len(sdp.Sectors))
		}
	}
}

func TestSectorPredictor_NormalizedEntropy(t *testing.T) {
	uniform := PredictionDistribution{Inflow: 0.33, Neutral: 0.34, Outflow: 0.33}
	deterministic := PredictionDistribution{Inflow: 1.0, Neutral: 0.0, Outflow: 0.0}

	uniEnt := normalizedEntropy(uniform)
	detEnt := normalizedEntropy(deterministic)

	if uniEnt < 0.9 {
		t.Errorf("uniform entropy %.4f, expected near 1.0", uniEnt)
	}
	if detEnt > 1e-6 {
		t.Errorf("deterministic entropy %.6f, expected near 0", detEnt)
	}
}

func TestSectorPredictor_IndustryL1SectorsCount(t *testing.T) {
	l1s := industry.L1Sectors()
	if len(l1s) != 20 {
		t.Errorf("industry.L1Sectors(): expected 20, got %d", len(l1s))
	}
}
