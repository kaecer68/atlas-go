package janus

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/prism"
)

func TestCohortPerformanceTracker_RecordAndRetrieve(t *testing.T) {
	tracker := NewCohortPerformanceTracker(10)

	tracker.RecordSnapshot(CohortSnapshot{
		Regime:      prism.RegimeRiskOn,
		SharpeRatio: 1.0,
		HitRate:     0.6,
		RecordedAt:  time.Now(),
	})

	perf := tracker.GetCohortPerformance(prism.RegimeRiskOn)
	if perf == nil {
		t.Fatal("expected performance for RegimeRiskOn")
	}
	if perf.ShortWindow == nil || math.Abs(perf.ShortWindow.SharpeRatio-1.0) > 1e-9 {
		t.Fatalf("expected short sharpe 1.0, got %v", perf.ShortWindow)
	}
}

func TestCohortPerformanceTracker_RollingWindow(t *testing.T) {
	tracker := NewCohortPerformanceTracker(10)

	for i := range 10 {
		tracker.RecordSnapshot(CohortSnapshot{
			Regime:      prism.RegimeRiskOn,
			SharpeRatio: float64(i),
			RecordedAt:  time.Now(),
		})
	}

	perf := tracker.GetCohortPerformance(prism.RegimeRiskOn)
	// Last 5 values are 5,6,7,8,9 => average = 7.0
	if math.Abs(perf.ShortWindow.SharpeRatio-7.0) > 1e-9 {
		t.Fatalf("expected short sharpe 7.0, got %f", perf.ShortWindow.SharpeRatio)
	}
	// All 10 values average = 4.5
	if math.Abs(perf.MedWindow.SharpeRatio-4.5) > 1e-9 {
		t.Fatalf("expected med sharpe 4.5, got %f", perf.MedWindow.SharpeRatio)
	}
}

func TestCohortWeightCalculator_AllNegative(t *testing.T) {
	calc := NewCohortWeightCalculator(DefaultJANUSConfig())
	perf := map[prism.RegimeType]*CohortPerformance{
		prism.RegimeRiskOn: {
			Regime:      prism.RegimeRiskOn,
			ShortWindow: &WindowPerformance{SharpeRatio: -0.5},
			MedWindow:   &WindowPerformance{SharpeRatio: -0.3},
			LongWindow:  &WindowPerformance{SharpeRatio: -0.2},
		},
		prism.RegimeRiskOff: {
			Regime:      prism.RegimeRiskOff,
			ShortWindow: &WindowPerformance{SharpeRatio: -0.8},
			MedWindow:   &WindowPerformance{SharpeRatio: -0.6},
			LongWindow:  &WindowPerformance{SharpeRatio: -0.4},
		},
	}

	weights := calc.CalculateWeights(perf)
	expected := 0.5
	for _, cw := range weights {
		if math.Abs(cw.Weight-expected) > 1e-9 {
			t.Fatalf("expected equal weight %v, got %v", expected, cw.Weight)
		}
	}
}

func TestCohortWeightCalculator_MixedScores(t *testing.T) {
	calc := NewCohortWeightCalculator(DefaultJANUSConfig())
	perf := map[prism.RegimeType]*CohortPerformance{
		prism.RegimeRiskOn: {
			Regime:      prism.RegimeRiskOn,
			ShortWindow: &WindowPerformance{SharpeRatio: 1.2},
		},
		prism.RegimeRiskOff: {
			Regime:      prism.RegimeRiskOff,
			ShortWindow: &WindowPerformance{SharpeRatio: 0.8},
		},
		prism.RegimeHighVolatility: {
			Regime:      prism.RegimeHighVolatility,
			ShortWindow: &WindowPerformance{SharpeRatio: -0.2},
		},
	}

	weights := calc.CalculateWeights(perf)
	total := 0.0
	for _, cw := range weights {
		total += cw.Weight
	}
	if math.Abs(total-1.0) > 1e-9 {
		t.Fatalf("expected weights to sum to 1.0, got %f", total)
	}

	// Positive sharpe cohorts should get materially more than epsilon.
	if weights[prism.RegimeRiskOn].Weight <= weights[prism.RegimeHighVolatility].Weight {
		t.Fatalf("expected RiskOn weight > HighVol weight, got %v vs %v",
			weights[prism.RegimeRiskOn].Weight, weights[prism.RegimeHighVolatility].Weight)
	}
}

func TestRegimeDetector_NovélRegime(t *testing.T) {
	det := NewRegimeDetector(DefaultJANUSConfig())

	short := map[prism.RegimeType]CohortWeight{
		prism.RegimeRiskOn:         {Weight: 0.45},
		prism.RegimeRiskOff:        {Weight: 0.30},
		prism.RegimeHighVolatility: {Weight: 0.15},
		prism.RegimeLowVolatility:  {Weight: 0.05},
		prism.RegimeTransition:     {Weight: 0.05},
	}

	// Same cohorts, but long window shows a different leader.
	long := map[prism.RegimeType]CohortWeight{
		prism.RegimeRiskOn:         {Weight: 0.20},
		prism.RegimeRiskOff:        {Weight: 0.50},
		prism.RegimeHighVolatility: {Weight: 0.15},
		prism.RegimeLowVolatility:  {Weight: 0.10},
		prism.RegimeTransition:     {Weight: 0.05},
	}

	// Short winner = RiskOn (0.45), its long weight = 0.20 => delta 0.25 > threshold 0.15
	// and short winner != long winner => NOVEL_REGIME.
	cls := det.Detect(short, long)
	if cls != NovelRegime {
		t.Fatalf("expected NOVEL_REGIME, got %s", cls)
	}
}

func TestRegimeDetector_HistoricalRegime(t *testing.T) {
	det := NewRegimeDetector(DefaultJANUSConfig())

	short := map[prism.RegimeType]CohortWeight{
		prism.RegimeRiskOn:  {Weight: 0.25},
		prism.RegimeRiskOff: {Weight: 0.30},
	}

	long := map[prism.RegimeType]CohortWeight{
		prism.RegimeRiskOn:  {Weight: 0.15},
		prism.RegimeRiskOff: {Weight: 0.55},
	}

	// Long winner = RiskOff (0.55), its short weight = 0.30 => delta 0.25 > threshold
	// and long winner weight >= short winner weight => HISTORICAL_REGIME.
	cls := det.Detect(short, long)
	if cls != HistoricalRegime {
		t.Fatalf("expected HISTORICAL_REGIME, got %s", cls)
	}
}

func TestRegimeDetector_Mixed(t *testing.T) {
	det := NewRegimeDetector(DefaultJANUSConfig())

	short := map[prism.RegimeType]CohortWeight{
		prism.RegimeRiskOn:  {Weight: 0.35},
		prism.RegimeRiskOff: {Weight: 0.35},
	}

	long := map[prism.RegimeType]CohortWeight{
		prism.RegimeRiskOn:  {Weight: 0.35},
		prism.RegimeRiskOff: {Weight: 0.35},
	}

	cls := det.Detect(short, long)
	if cls != MixedRegime && cls != HistoricalRegime {
		t.Fatalf("expected MIXED or HISTORICAL_REGIME for stable tie, got %s", cls)
	}
}

func TestEngine_EndToEnd(t *testing.T) {
	engine := NewEngine()
	engine.EnsureAllRegimes()

	// Simulate 20 days of observations where RiskOn dominates recently.
	now := time.Now()
	for day := range 20 {
		for r := range int(prism.RegimeCount) {
			regime := prism.RegimeType(r)
			sharpe := 0.3
			if regime == prism.RegimeRiskOn {
				if day >= 15 {
					sharpe = 1.5 // recent surge
				} else {
					sharpe = 0.4 // modest historical performance
				}
			}
			engine.RecordSnapshot(CohortSnapshot{
				Regime:      regime,
				SharpeRatio: sharpe,
				HitRate:     0.55,
				RecordedAt:  now.Add(time.Duration(day) * 24 * time.Hour),
			})
		}
	}

	engine.Update()

	weights := engine.GetCohortWeights()
	if len(weights) == 0 {
		t.Fatal("expected non-empty weights after update")
	}

	cls := engine.GetRegimeClassification()
	if cls != NovelRegime && cls != HistoricalRegime && cls != MixedRegime {
		t.Fatalf("unexpected classification: %s", cls)
	}

	status := engine.GetStatus()
	if status.Classification == "" {
		t.Fatal("expected non-empty classification in status")
	}
}

func TestEngine_ApplyAdjustment(t *testing.T) {
	engine := NewEngine()
	engine.EnsureAllRegimes()

	// Seed data so that RiskOn gets a high weight and others low.
	for range 10 {
		engine.RecordSnapshot(CohortSnapshot{
			Regime:      prism.RegimeRiskOn,
			SharpeRatio: 2.0,
			RecordedAt:  time.Now(),
		})
	}
	for r := 1; r < int(prism.RegimeCount); r++ {
		for range 10 {
			engine.RecordSnapshot(CohortSnapshot{
				Regime:      prism.RegimeType(r),
				SharpeRatio: -0.1,
				RecordedAt:  time.Now(),
			})
		}
	}

	engine.Update()

	recs := []domain.Recommendation{
		{Agent: "agent1", Symbol: "2330", Conviction: 70, Side: domain.SideBuy},
		{Agent: "agent2", Symbol: "2317", Conviction: 60, Side: domain.SideBuy},
	}

	adjusted := engine.ApplyAdjustment(recsForJanus(recs), domain.RegimeRiskOn)
	if len(adjusted) != len(recs) {
		t.Fatalf("expected %d recommendations, got %d", len(recs), len(adjusted))
	}

	// RiskOn should have high weight (>0.2 neutral), so conviction should be boosted.
	if adjusted[0].Conviction <= recs[0].Conviction {
		t.Fatalf("expected conviction boost for RiskOn regime, got %d vs original %d",
			adjusted[0].Conviction, recs[0].Conviction)
	}

	// Test with a regime that has no data (fallback to unchanged not applicable here
	// because EnsureAllRegimes creates entries for all regimes).
	adjustedNeutral := engine.ApplyAdjustment(recsForJanus(recs), domain.RegimeNeutral)
	if len(adjustedNeutral) != len(recs) {
		t.Fatalf("expected %d recommendations for neutral, got %d", len(recs), len(adjustedNeutral))
	}
}

func TestEngine_ApplyAdjustment_NoWeights(t *testing.T) {
	engine := NewEngine()
	// No snapshots recorded, no weights computed.
	recs := []domain.Recommendation{
		{Agent: "agent1", Symbol: "2330", Conviction: 70},
	}
	adjusted := engine.ApplyAdjustment(recsForJanus(recs), domain.RegimeRiskOn)
	if adjusted[0].Conviction != 70 {
		t.Fatalf("expected unchanged conviction when no weights exist, got %d", adjusted[0].Conviction)
	}
}

func recsForJanus(recs []domain.Recommendation) []domain.Recommendation {
	out := make([]domain.Recommendation, len(recs))
	copy(out, recs)
	return out
}

func TestMapDomainRegimeToPRISM(t *testing.T) {
	cases := []struct {
		domain domain.Regime
		want   prism.RegimeType
	}{
		{domain.RegimeRiskOn, prism.RegimeRiskOn},
		{domain.RegimeRiskOff, prism.RegimeRiskOff},
		{domain.RegimeNeutral, prism.RegimeLowVolatility},
		{"UNKNOWN", prism.RegimeTransition},
	}
	for _, c := range cases {
		got := mapDomainRegimeToPRISM(c.domain)
		if got != c.want {
			t.Fatalf("mapDomainRegimeToPRISM(%q) = %v, want %v", c.domain, got, c.want)
		}
	}
}

func TestNewEngineWithConfig(t *testing.T) {
	cfg := JANUSConfig{
		MinWeight: 0.1, MaxWeight: 0.5, NovelThreshold: 0.2,
		HistoricalThreshold: 0.2, EpsilonWeight: 0.01,
	}
	engine := NewEngineWithConfig(cfg)
	engine.EnsureAllRegimes()
	engine.Update()
	stats := engine.HealthStatus()
	if !stats.Initialized {
		t.Fatal("expected initialized engine after Update")
	}
	weights := engine.GetCohortWeights()
	if len(weights) == 0 {
		t.Fatal("expected non-empty weights after update")
	}
}

func TestEngine_RecordTrainingResult(t *testing.T) {
	engine := NewEngine()
	engine.EnsureAllRegimes()
	engine.RecordTrainingResult(prism.RegimeRiskOn, prism.TrainingResult{
		SharpeRatio: 1.5, HitRate: 0.7, TotalReturn: 0.12, SignalsCount: 30,
	})
	engine.Update()
	weights := engine.GetCohortWeights()
	if _, ok := weights[prism.RegimeRiskOn]; !ok {
		t.Fatal("expected RiskOn cohort weight after recording result")
	}
}

func TestEngine_HealthStatus(t *testing.T) {
	t.Run("uninitialized", func(t *testing.T) {
		engine := NewEngine()
		hs := engine.HealthStatus()
		if hs.Initialized {
			t.Fatal("expected uninitialized before any update")
		}
	})

	t.Run("after_update", func(t *testing.T) {
		engine := NewEngine()
		engine.EnsureAllRegimes()
		engine.Update()
		hs := engine.HealthStatus()
		if !hs.Initialized {
			t.Fatal("expected initialized after update")
		}
		if hs.CohortCount == 0 {
			t.Fatal("expected non-zero cohort count")
		}
	})
}

func TestEngine_RecordHealthTo(t *testing.T) {
	engine := NewEngine()
	store := &stubHealthStore{}
	engine.RecordHealthTo(store)
	if len(store.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(store.records))
	}
	if store.records[0].status != "error" {
		t.Fatalf("expected error for uninitialized engine, got %s", store.records[0].status)
	}

	engine.EnsureAllRegimes()
	engine.Update()
	store.records = nil
	engine.RecordHealthTo(store)
	if len(store.records) != 1 {
		t.Fatalf("expected 1 record after update, got %d", len(store.records))
	}
	if store.records[0].status != "ok" {
		t.Fatalf("expected ok after update, got %s", store.records[0].status)
	}
}

func TestEngine_String(t *testing.T) {
	engine := NewEngine()
	engine.EnsureAllRegimes()
	engine.Update()
	s := engine.String()
	if s == "" {
		t.Fatal("expected non-empty string")
	}
}

type stubHealthStore struct {
	records []stubRecord
}

type stubRecord struct {
	channelID string
	status    string
	message   string
}

func (s *stubHealthStore) Record(channelID, status, message string) {
	s.records = append(s.records, stubRecord{channelID, status, message})
}
