package sectorallocation_test

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

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

func make20L1ForTest() map[industry.SectorID]float64 {
	m := make(map[industry.SectorID]float64, 20)
	s := 0.0
	for _, id := range industry.L1Sectors() {
		m[id] = 0.05
		s += 0.05
	}
	return m
}
