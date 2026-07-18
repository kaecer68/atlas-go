package sectorallocation_test

import (
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

func TestStrategicPrior_DefaultSeedIsHeuristicCalibrating(t *testing.T) {
	cfg := config.GetParametersConfig()
	p, err := sectorallocation.LoadStrategicPrior(cfg)
	if err != nil {
		t.Fatalf("LoadStrategicPrior failed: %v", err)
	}
	if p.Source != "heuristic" {
		t.Fatalf("prior source must be heuristic by default, got %q", p.Source)
	}
	if p.CalibrationStatus != "calibrating" {
		t.Fatalf("prior calibration status must be calibrating, got %q", p.CalibrationStatus)
	}
	if p.PromotionGate() {
		t.Fatal("heuristic calibrating prior must not promote")
	}
}

func TestStrategicPrior_RequiresExactly20L1(t *testing.T) {
	p := &sectorallocation.StrategicSectorPrior{
		Weights: map[industry.SectorID]float64{industry.SectorSemiconductor: 1.0},
		Source:  "heuristic", ModelVersion: "v1.0.0", CalibrationStatus: "calibrating",
	}
	if err := sectorallocation.ValidatePrior(p); err == nil {
		t.Fatal("must reject fewer than 20 L1 keys")
	}
}

func TestStrategicPrior_RejectsNonL1Key(t *testing.T) {
	m := make20L1SumOneForPriorTest()
	m[industry.SubIndustryIndustrial] = 0.10 // override one L1 to be replaced
	delete(m, industry.SectorSemiconductor)
	m[industry.SectorSemiconductor] = 0.0
	m[industry.SubIndustryIndustrial] = 0.05
	// rebalance to 1.0
	for id := range m {
		if id == industry.SubIndustryIndustrial {
			continue
		}
	}
	if err := sectorallocation.ValidatePrior(&sectorallocation.StrategicSectorPrior{
		Weights: m, Source: "heuristic", ModelVersion: "v1.0.0", CalibrationStatus: "calibrating",
	}); err == nil {
		t.Fatal("prior must not contain L2 keys")
	}
}

func TestStrategicPrior_RejectsNonSemverVersion(t *testing.T) {
	m := make20L1SumOneForPriorTest()
	p := &sectorallocation.StrategicSectorPrior{
		Weights: m, Source: "heuristic", ModelVersion: "c07-heuristic", CalibrationStatus: "calibrating",
	}
	if err := sectorallocation.ValidatePrior(p); err == nil {
		t.Fatal("prior ModelVersion must be semver")
	}
}

func TestStrategicPrior_RejectsNegativeWeight(t *testing.T) {
	m := make20L1SumOneForPriorTest()
	m[industry.SectorSemiconductor] = -0.05
	if err := sectorallocation.ValidatePrior(&sectorallocation.StrategicSectorPrior{
		Weights: m, Source: "heuristic", ModelVersion: "v1.0.0", CalibrationStatus: "calibrating",
	}); err == nil {
		t.Fatal("prior must not contain negative weight")
	}
}

func TestStrategicPrior_RejectsSumDrift(t *testing.T) {
	m := make20L1SumOneForPriorTest()
	m[industry.SectorSemiconductor] = 0.10
	if err := sectorallocation.ValidatePrior(&sectorallocation.StrategicSectorPrior{
		Weights: m, Source: "heuristic", ModelVersion: "v1.0.0", CalibrationStatus: "calibrating",
	}); err == nil {
		t.Fatal("prior sum must be 1.0±1e-9")
	}
}

func TestStrategicPrior_PromotionRequiresEmpiricalSource(t *testing.T) {
	m := make20L1SumOneForPriorTest()
	p := &sectorallocation.StrategicSectorPrior{
		Weights: m, Source: "heuristic", ModelVersion: "v1.0.0", CalibrationStatus: "calibrated",
	}
	if p.PromotionGate() {
		t.Fatal("PromotionGate must require source=calibrated (empirical upgrade not in plan scope)")
	}
}

func TestStrategicPrior_PromotionRequiresCalibratedStatus(t *testing.T) {
	m := make20L1SumOneForPriorTest()
	p := &sectorallocation.StrategicSectorPrior{
		Weights: m, Source: "calibrated", ModelVersion: "v1.0.0", CalibrationStatus: "calibrating",
	}
	if p.PromotionGate() {
		t.Fatal("PromotionGate must require calibration_status=calibrated")
	}
}

func TestStrategicPrior_DefaultSeedSumsToOne(t *testing.T) {
	cfg := config.GetParametersConfig()
	p, err := sectorallocation.LoadStrategicPrior(cfg)
	if err != nil {
		t.Fatalf("LoadStrategicPrior failed: %v", err)
	}
	if len(p.Weights) != 20 {
		t.Fatalf("default prior must have 20 L1 keys, got %d", len(p.Weights))
	}
	s := 0.0
	for _, v := range p.Weights {
		if v < 0 {
			t.Fatalf("negative prior weight: %f", v)
		}
		s += v
	}
	if s < 0.999999999 || s > 1.000000001 {
		t.Fatalf("prior sum drift: %.12f", s)
	}
}

func TestStrategicPrior_ModelVersionIsSemver(t *testing.T) {
	cfg := config.GetParametersConfig()
	p, err := sectorallocation.LoadStrategicPrior(cfg)
	if err != nil {
		t.Fatalf("LoadStrategicPrior failed: %v", err)
	}
	if !strings.HasPrefix(p.ModelVersion, "v") {
		t.Fatalf("ModelVersion must start with v (semver): %q", p.ModelVersion)
	}
	if !strings.Contains(p.ModelVersion, ".") {
		t.Fatalf("ModelVersion must contain . (semver): %q", p.ModelVersion)
	}
}

func make20L1SumOneForPriorTest() map[industry.SectorID]float64 {
	m := make(map[industry.SectorID]float64, 20)
	eq := 1.0 / 20.0
	for _, id := range industry.L1Sectors() {
		m[id] = eq
	}
	return m
}
