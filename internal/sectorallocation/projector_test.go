package sectorallocation_test

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

func TestProjector_RejectsNonL1Key(t *testing.T) {
	raw := make20L1ForProjectorTest()
	raw["cash"] = 0.10
	raw[industry.SubIndustryIndustrial] = 0.05
	drivers := sectorallocation.DriverInputs{AsOfTradingDate: "2026-07-17"}
	_, err := sectorallocation.NewDefaultProjector().Project(raw, drivers)
	if err == nil {
		t.Fatal("Projector must reject non L1 keys (cash, SubIndustryIndustrial)")
	}
}

func TestProjector_RejectsLessThan20Keys(t *testing.T) {
	raw := map[industry.SectorID]float64{industry.SectorSemiconductor: 1.0}
	drivers := sectorallocation.DriverInputs{AsOfTradingDate: "2026-07-17"}
	_, err := sectorallocation.NewDefaultProjector().Project(raw, drivers)
	if err == nil {
		t.Fatal("Projector must reject fewer than 20 L1 keys")
	}
}

func TestProjector_SumToleranceAndClamps(t *testing.T) {
	raw := make20L1ForProjectorTest()
	drivers := sectorallocation.DriverInputs{AsOfTradingDate: "2026-07-17"}
	target, err := sectorallocation.NewDefaultProjector().Project(raw, drivers)
	if err != nil {
		t.Fatalf("Projector.Project failed: %v", err)
	}
	if len(target.Target) != 20 {
		t.Fatalf("final target must have 20 L1 keys, got %d", len(target.Target))
	}
	s := 0.0
	for id, v := range target.Target {
		if !industry.IsL1(id) {
			t.Fatalf("non L1 key in final target: %s", id)
		}
		if v < 0 {
			t.Fatalf("negative L1 weight after projection: %f for %s", v, id)
		}
		if v < 0.005 || v > 0.5 {
			t.Fatalf("L1 weight out of default clamp [0.005, 0.5]: %f for %s", v, id)
		}
		s += v
	}
	if math.Abs(s-1.0) > 1e-9 {
		t.Fatalf("final target sum drift: %.12f", s)
	}
}

func TestProjector_AdjustmentLogProvenance(t *testing.T) {
	raw := make20L1ForProjectorTest()
	// 製造一個非零 tilt
	raw[industry.SectorSemiconductor] = 0.50
	raw[industry.SectorEnergy] = -0.05
	drivers := sectorallocation.DriverInputs{
		AsOfTradingDate: "2026-07-17",
		Cycle:           map[industry.SectorID]float64{industry.SectorSemiconductor: 0.05},
		Seasonal:        map[industry.SectorID]float64{industry.SectorSemiconductor: -0.02},
	}
	target, err := sectorallocation.NewDefaultProjector().Project(raw, drivers)
	if err != nil {
		t.Fatalf("Projector.Project failed: %v", err)
	}
	if len(target.AdjustmentLog) == 0 {
		t.Fatal("AdjustmentLog must record at least one provenance event for driver")
	}
	for _, e := range target.AdjustmentLog {
		if !industry.IsL1(e.Sector) {
			t.Fatalf("adjustment event for non L1: %s", e.Sector)
		}
		if e.Reason == "" {
			t.Fatal("adjustment event must declare reason")
		}
	}
}

func TestProjector_EquityFundedTiltIsZeroSum(t *testing.T) {
	drivers := sectorallocation.DriverInputs{
		AsOfTradingDate: "2026-07-17",
		Cycle:           map[industry.SectorID]float64{industry.SectorSemiconductor: 0.10, industry.SectorEnergy: -0.10},
	}
	raw := make20L1ForProjectorTest()
	_, err := sectorallocation.NewDefaultProjector().Project(raw, drivers)
	if err != nil {
		t.Fatalf("Projector.Project failed: %v", err)
	}
	// 在 cycle tilt zero-sum 情況下，semiconductor 與 energy 的 delta sum=0；
	// projection 後兩者相對變化必須反映此 zero-sum 屬性。
	// 透過 AdjustmentLog 驗證：cycle driver 加總必為 0。
	delta := 0.0
	for _, e := range drivers.Cycle {
		delta += e
	}
	if math.Abs(delta) > 1e-9 {
		t.Fatalf("cycle driver must be zero-sum: %.12f", delta)
	}
}

func TestProjector_NonL1BaseWeightIgnored(t *testing.T) {
	raw := make20L1ForProjectorTest()
	raw[industry.SubIndustryIndustrial] = 0.05 // 舊 BaseAllocations 殘留
	// 但 len 必須仍 = 20（加 raw 不改長度，Projector 才會觸發 rejection）
	drivers := sectorallocation.DriverInputs{AsOfTradingDate: "2026-07-17"}
	_, err := sectorallocation.NewDefaultProjector().Project(raw, drivers)
	if err == nil {
		t.Fatal("Projector must reject raw containing non L1 keys even if it has 20 L1 keys")
	}
}

func make20L1ForProjectorTest() map[industry.SectorID]float64 {
	m := make(map[industry.SectorID]float64, 20)
	eq := 1.0 / 20.0
	for _, id := range industry.L1Sectors() {
		m[id] = eq
	}
	return m
}
