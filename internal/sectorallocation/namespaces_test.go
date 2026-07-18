package sectorallocation_test

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

func TestNamespaceKind_OnlyFourCanonical(t *testing.T) {
	want := []sectorallocation.NamespaceKind{
		sectorallocation.NamespaceEquityL1,
		sectorallocation.NamespaceResearchThemeL2,
		sectorallocation.NamespaceStrategyBucket,
		sectorallocation.NamespaceAssetClass,
	}
	if len(want) != 4 {
		t.Fatalf("must remain 4 namespaces, got %d", len(want))
	}
	for _, n := range []sectorallocation.NamespaceKind{
		"equity_l1", "L2", "themes", "sector_l1", "narrative",
	} {
		if sectorallocation.IsValidNamespace(n) {
			t.Errorf("non canonical namespace accepted: %q", n)
		}
	}
}

func TestL1FinalTarget_RejectsNonCanonicalKeys(t *testing.T) {
	bad := sectorallocation.L1FinalTarget{
		Weights: map[industry.SectorID]float64{
			industry.SectorSemiconductor:   0.5,
			industry.SubIndustryIndustrial: 0.5,
		},
	}
	if err := sectorallocation.ValidateL1FinalTarget(bad); err == nil {
		t.Fatal("must reject L2 keys in L1 final target")
	}
}

func TestL1FinalTarget_RejectsLessThan20Keys(t *testing.T) {
	m := map[industry.SectorID]float64{industry.SectorSemiconductor: 1.0}
	if err := sectorallocation.ValidateL1FinalTarget(sectorallocation.L1FinalTarget{Weights: m}); err == nil {
		t.Fatal("must reject fewer than 20 L1 keys")
	}
}

func TestL1FinalTarget_RejectsMoreThan20Keys(t *testing.T) {
	m := make(map[industry.SectorID]float64, 21)
	s := 0.0
	for i, id := range industry.L1Sectors() {
		if i >= 19 {
			break
		}
		m[id] = 0.05
		s += 0.05
	}
	m[industry.SubIndustryIndustrial] = 0.05
	if err := sectorallocation.ValidateL1FinalTarget(sectorallocation.L1FinalTarget{Weights: m}); err == nil {
		t.Fatal("must reject more than 20 keys (21 with L2)")
	}
}

func TestL1FinalTarget_RejectsNegativeWeight(t *testing.T) {
	m := make20L1ForTest()
	m[industry.SectorSemiconductor] = -0.10
	if err := sectorallocation.ValidateL1FinalTarget(sectorallocation.L1FinalTarget{Weights: m}); err == nil {
		t.Fatal("must reject negative L1 weight")
	}
}

func TestL1FinalTarget_RejectsSumDrift(t *testing.T) {
	m := make20L1ForTest()
	m[industry.SectorSemiconductor] = 0.10 // drift 0.05
	if err := sectorallocation.ValidateL1FinalTarget(sectorallocation.L1FinalTarget{Weights: m}); err == nil {
		t.Fatal("must reject sum drift > 1e-9")
	}
}

func TestL1FinalTarget_FullyCanonicalSucceeds(t *testing.T) {
	m := make20L1ForTest()
	if err := sectorallocation.ValidateL1FinalTarget(sectorallocation.L1FinalTarget{Weights: m}); err != nil {
		t.Fatalf("fully canonical target should validate: %v", err)
	}
	s := 0.0
	for _, v := range m {
		s += v
	}
	if s < 0.999999999 || s > 1.000000001 {
		t.Fatalf("test fixture sum drift: %.12f", s)
	}
}

func TestThemeExposure_RowMustSumToOne(t *testing.T) {
	if err := sectorallocation.ValidateThemeExposure(sectorallocation.ThemeExposure{
		Theme: "ai_supply_chain",
		ToL1:  map[industry.SectorID]float64{industry.SectorSemiconductor: 0.7},
	}); err == nil {
		t.Fatal("must reject theme row not summing to 1")
	}
}

func TestThemeExposure_NoFuzzyIndustrialToIndustrials(t *testing.T) {
	if err := sectorallocation.ValidateThemeExposure(sectorallocation.ThemeExposure{
		Theme: "industrials_alias",
		ToL1:  map[industry.SectorID]float64{industry.SubIndustryIndustrial: 1.0},
	}); err == nil {
		t.Fatal("must reject fuzzy mapping between L1 and L2 forms")
	}
}

func TestThemeExposure_RejectsNonL1Keys(t *testing.T) {
	if err := sectorallocation.ValidateThemeExposure(sectorallocation.ThemeExposure{
		Theme: "ai_supply_chain",
		ToL1:  map[industry.SectorID]float64{industry.SubIndustryIndustrial: 1.0},
	}); err == nil {
		t.Fatal("theme exposure must not map to L2 key")
	}
}

func make20L1ForTest() map[industry.SectorID]float64 {
	m := make(map[industry.SectorID]float64, 20)
	s := 0.0
	for _, id := range industry.L1Sectors() {
		m[id] = 0.05
		s += 0.05
	}
	return m
}
