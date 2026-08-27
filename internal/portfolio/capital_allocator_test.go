package portfolio

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestNewCapitalAllocator(t *testing.T) {
	a := NewCapitalAllocator()
	if a == nil {
		t.Fatal("expected non-nil allocator")
	}
}

func TestAllocate_EmptyRecommendations(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()

	result := a.Allocate(cfg, nil, 1000000, 0.1)

	if result.TotalCapital != 1000000 {
		t.Errorf("expected total capital 1000000, got %.2f", result.TotalCapital)
	}
	if result.ReserveCash != 100000 {
		t.Errorf("expected reserve cash 100000, got %.2f", result.ReserveCash)
	}
	expectedDeployable := (1000000 - 100000) * 1.0
	if result.TotalDeployable != expectedDeployable {
		t.Errorf("expected deployable %.2f, got %.2f", expectedDeployable, result.TotalDeployable)
	}
	if len(result.PositionSizes) != 0 {
		t.Errorf("expected empty position sizes, got %d entries", len(result.PositionSizes))
	}
}

func TestAllocate_PhaseLimit(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.CurrentPhase = domain.PhasePaper

	result := a.Allocate(cfg, nil, 1000000, 0.1)

	expectedDeployable := (1000000 - 100000) * 0.10
	if result.TotalDeployable != expectedDeployable {
		t.Errorf("expected deployable %.2f, got %.2f", expectedDeployable, result.TotalDeployable)
	}
}

func TestAllocate_EqualDistribution(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.CurrentPhase = domain.PhaseSimulation

	recs := []domain.Recommendation{
		{Symbol: "2330", Conviction: 0},
		{Symbol: "2454", Conviction: 0},
	}

	result := a.Allocate(cfg, recs, 1000000, 0.1)

	equalShare := 900000.0 / 2
	if result.PositionSizes["2330"] != equalShare {
		t.Errorf("expected 2330 size %.2f, got %.2f", equalShare, result.PositionSizes["2330"])
	}
	if result.PositionSizes["2454"] != equalShare {
		t.Errorf("expected 2454 size %.2f, got %.2f", equalShare, result.PositionSizes["2454"])
	}
}

func TestAllocate_ConvictionWeighted(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.CurrentPhase = domain.PhaseSimulation

	recs := []domain.Recommendation{
		{Symbol: "2330", Conviction: 3},
		{Symbol: "2454", Conviction: 1},
	}

	result := a.Allocate(cfg, recs, 1000000, 0.0)

	totalConviction := 4.0
	expected2330 := 1000000.0 * (3.0 / totalConviction)
	expected2454 := 1000000.0 * (1.0 / totalConviction)

	if diff := absFloat(result.PositionSizes["2330"] - expected2330); diff > 0.01 {
		t.Errorf("expected 2330 size %.2f, got %.2f", expected2330, result.PositionSizes["2330"])
	}
	if diff := absFloat(result.PositionSizes["2454"] - expected2454); diff > 0.01 {
		t.Errorf("expected 2454 size %.2f, got %.2f", expected2454, result.PositionSizes["2454"])
	}
}

func TestAllocate_PhaseFullLimit(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.CurrentPhase = domain.PhaseFull

	recs := []domain.Recommendation{
		{Symbol: "2330", Conviction: 1},
	}

	result := a.Allocate(cfg, recs, 1000000, 0.1)

	expectedDeployable := (1000000 - 100000) * 1.0
	if result.TotalDeployable != expectedDeployable {
		t.Errorf("expected deployable %.2f, got %.2f", expectedDeployable, result.TotalDeployable)
	}
}

func TestAllocate_ZeroDeployable(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.CurrentPhase = domain.CapitalPhase("unknown")
	cfg.CapitalLimits = map[string]float64{}

	recs := []domain.Recommendation{
		{Symbol: "2330", Conviction: 1},
	}

	result := a.Allocate(cfg, recs, 1000000, 1.0)

	if result.TotalDeployable != 0 {
		t.Errorf("expected deployable 0, got %.2f", result.TotalDeployable)
	}
}

func TestReallocateWithTax_NoTaxSnapshots(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()

	recs := []domain.Recommendation{
		{Symbol: "2330", Conviction: 1},
	}

	base := a.Allocate(cfg, recs, 1000000, 0.1)
	withTax := a.ReallocateWithTax(cfg, recs, 1000000, 0.1, nil)

	if base.TotalDeployable != withTax.TotalDeployable {
		t.Errorf("expected same deployable without tax, base=%.2f withTax=%.2f", base.TotalDeployable, withTax.TotalDeployable)
	}
}

func TestReallocateWithTax_Reduction(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()

	recs := []domain.Recommendation{
		{Symbol: "2330", Conviction: 1},
	}

	taxSnapshots := []domain.TaxSnapshot{
		{Symbol: "2330", TotalTax: 5000},
	}

	result := a.ReallocateWithTax(cfg, recs, 1000000, 0.1, taxSnapshots)

	if result.PositionSizes["2330"] >= 900000 {
		t.Errorf("expected position size reduced below 900000, got %.2f", result.PositionSizes["2330"])
	}
	if result.TotalDeployable >= 900000 {
		t.Errorf("expected total deployable reduced below 900000, got %.2f", result.TotalDeployable)
	}
}

func TestReallocateWithTax_TaxExceedsPosition(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()

	recs := []domain.Recommendation{
		{Symbol: "2330", Conviction: 1},
	}

	taxSnapshots := []domain.TaxSnapshot{
		{Symbol: "2330", TotalTax: 999999999},
	}

	result := a.ReallocateWithTax(cfg, recs, 1000000, 0.1, taxSnapshots)

	if result.PositionSizes["2330"] != 0 {
		t.Errorf("expected position size 0 when tax exceeds, got %.2f", result.PositionSizes["2330"])
	}
}

func TestReallocateWithTax_PartialTaxCoverage(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()

	recs := []domain.Recommendation{
		{Symbol: "2330", Conviction: 1},
		{Symbol: "2454", Conviction: 1},
	}

	taxSnapshots := []domain.TaxSnapshot{
		{Symbol: "2330", TotalTax: 1000},
	}

	result := a.ReallocateWithTax(cfg, recs, 1000000, 0.0, taxSnapshots)

	if result.PositionSizes["2454"] != 500000 {
		t.Errorf("expected 2454 unchanged at 500000, got %.2f", result.PositionSizes["2454"])
	}
	if result.PositionSizes["2330"] != 499000 {
		t.Errorf("expected 2330 reduced to 499000, got %.2f", result.PositionSizes["2330"])
	}
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestAllocate_DuplicateSymbolDedupConvictionWeighted(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.CurrentPhase = domain.PhaseSimulation

	// 同 symbol 兩筆（外資 conviction=3、投信 conviction=4）+ 一筆獨立 symbol。
	// 去重語意：conviction 取 max 並保留該筆 entry；分母用去重後 symbol 集。
	recs := []domain.Recommendation{
		{Symbol: "2330", Conviction: 3, TargetPrice: 900},
		{Symbol: "2330", Conviction: 4, TargetPrice: 950},
		{Symbol: "2454", Conviction: 1},
	}

	result := a.Allocate(cfg, recs, 1000000, 0.0)

	deployable := 1000000.0
	// 去重後 totalConviction = 4 + 1 = 5
	expected2330 := deployable * 4.0 / 5.0
	expected2454 := deployable * 1.0 / 5.0

	if diff := absFloat(result.PositionSizes["2330"] - expected2330); diff > 0.01 {
		t.Errorf("expected 2330 size %.2f (max conviction wins), got %.2f", expected2330, result.PositionSizes["2330"])
	}
	if diff := absFloat(result.PositionSizes["2454"] - expected2454); diff > 0.01 {
		t.Errorf("expected 2454 size %.2f, got %.2f", expected2454, result.PositionSizes["2454"])
	}

	var sum float64
	for _, v := range result.PositionSizes {
		sum += v
	}
	if diff := absFloat(sum - deployable); diff > 0.01 {
		t.Errorf("expected position sizes sum to deployable %.2f, got %.2f", deployable, sum)
	}
	if len(result.PositionSizes) != 2 {
		t.Errorf("expected 2 unique symbols after dedup, got %d", len(result.PositionSizes))
	}
}

func TestAllocate_DuplicateSymbolDedupEqualShare(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.CurrentPhase = domain.PhaseSimulation

	// 零 conviction 路徑也要去重：同 symbol 兩筆只算一個 symbol。
	recs := []domain.Recommendation{
		{Symbol: "2330", Conviction: 0},
		{Symbol: "2330", Conviction: 0},
		{Symbol: "2454", Conviction: 0},
	}

	result := a.Allocate(cfg, recs, 1000000, 0.0)

	deployable := 1000000.0
	equalShare := deployable / 2.0 // 去重後 2 個 unique symbol

	if diff := absFloat(result.PositionSizes["2330"] - equalShare); diff > 0.01 {
		t.Errorf("expected 2330 equal share %.2f after dedup, got %.2f", equalShare, result.PositionSizes["2330"])
	}
	if diff := absFloat(result.PositionSizes["2454"] - equalShare); diff > 0.01 {
		t.Errorf("expected 2454 equal share %.2f after dedup, got %.2f", equalShare, result.PositionSizes["2454"])
	}

	var sum float64
	for _, v := range result.PositionSizes {
		sum += v
	}
	if diff := absFloat(sum - deployable); diff > 0.01 {
		t.Errorf("expected position sizes sum to deployable %.2f, got %.2f", deployable, sum)
	}
	if len(result.PositionSizes) != 2 {
		t.Errorf("expected 2 unique symbols after dedup, got %d", len(result.PositionSizes))
	}
}

func TestAllocate_DuplicateSymbolTieKeepsFirst(t *testing.T) {
	a := NewCapitalAllocator()
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.CurrentPhase = domain.PhaseSimulation

	// 同 conviction 平手：保留第一筆（deterministic）。
	recs := []domain.Recommendation{
		{Symbol: "2330", Conviction: 3, TargetPrice: 900},
		{Symbol: "2330", Conviction: 3, TargetPrice: 950},
		{Symbol: "2454", Conviction: 1},
	}

	result := a.Allocate(cfg, recs, 1000000, 0.0)

	deployable := 1000000.0
	// 去重後 totalConviction = 3 + 1 = 4
	expected2330 := deployable * 3.0 / 4.0
	expected2454 := deployable * 1.0 / 4.0

	if diff := absFloat(result.PositionSizes["2330"] - expected2330); diff > 0.01 {
		t.Errorf("expected 2330 size %.2f, got %.2f", expected2330, result.PositionSizes["2330"])
	}
	if diff := absFloat(result.PositionSizes["2454"] - expected2454); diff > 0.01 {
		t.Errorf("expected 2454 size %.2f, got %.2f", expected2454, result.PositionSizes["2454"])
	}

	var sum float64
	for _, v := range result.PositionSizes {
		sum += v
	}
	if diff := absFloat(sum - deployable); diff > 0.01 {
		t.Errorf("expected position sizes sum to deployable %.2f, got %.2f", deployable, sum)
	}
}
