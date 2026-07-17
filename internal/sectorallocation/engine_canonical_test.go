package sectorallocation_test

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

func TestDefaultEngine_ComputeProjectedTarget_Exactly20L1(t *testing.T) {
	prior, err := sectorallocation.LoadStrategicPrior(nil)
	_ = err
	if prior == nil {
		// LoadStrategicPrior(nil) 已 reject；改用 LoadStrategicPrior 從 config 拿
		// test fixture: 直接用 BuildStrategicPriorFromConfig
	}
	cfg := sectorallocation.NewDefaultProjector()
	_ = cfg
	priorMap := makeStrategicPriorForTest(t)
	engine := sectorallocation.NewDefaultEngineWithProjector(
		/* cfg */ sectorallocation.NewEngineTestConfig(),
		priorMap,
		sectorallocation.NewDefaultProjector(),
		nil, nil, nil, nil, nil, nil, 0.3, 2.5,
	)
	drivers := sectorallocation.DriverInputs{AsOfTradingDate: "2026-07-17"}
	target, err := engine.ComputeProjectedTarget(context.Background(), drivers)
	if err != nil {
		t.Fatalf("ComputeProjectedTarget failed: %v", err)
	}
	if len(target.Target) != 20 {
		t.Fatalf("ComputeProjectedTarget must return 20 L1 keys, got %d", len(target.Target))
	}
	for id := range target.Target {
		if !industry.IsL1(id) {
			t.Fatalf("non L1 key in projected target: %s", id)
		}
	}
}

func TestDefaultEngine_ComputeProjectedTarget_RejectsNonL1Driver(t *testing.T) {
	priorMap := makeStrategicPriorForTest(t)
	engine := sectorallocation.NewDefaultEngineWithProjector(
		sectorallocation.NewEngineTestConfig(),
		priorMap,
		sectorallocation.NewDefaultProjector(),
		nil, nil, nil, nil, nil, nil, 0.3, 2.5,
	)
	drivers := sectorallocation.DriverInputs{
		AsOfTradingDate: "2026-07-17",
		Cycle:           map[industry.SectorID]float64{industry.SubIndustryIndustrial: 0.05},
	}
	_, err := engine.ComputeProjectedTarget(context.Background(), drivers)
	if err == nil {
		t.Fatal("ComputeProjectedTarget must reject driver maps to non L1 key")
	}
}

func TestDefaultEngine_ComputeProjectedTarget_PriorNilRejected(t *testing.T) {
	engine := sectorallocation.NewDefaultEngineWithProjector(
		sectorallocation.NewEngineTestConfig(),
		nil, // nil prior
		sectorallocation.NewDefaultProjector(),
		nil, nil, nil, nil, nil, nil, 0.3, 2.5,
	)
	drivers := sectorallocation.DriverInputs{AsOfTradingDate: "2026-07-17"}
	_, err := engine.ComputeProjectedTarget(context.Background(), drivers)
	if err == nil {
		t.Fatal("ComputeProjectedTarget must reject nil prior (Projector SA-INV-05)")
	}
}

func TestDefaultEngine_ComputeProjectedTarget_AdjustmentLogRecorded(t *testing.T) {
	priorMap := makeStrategicPriorForTest(t)
	engine := sectorallocation.NewDefaultEngineWithProjector(
		sectorallocation.NewEngineTestConfig(),
		priorMap,
		sectorallocation.NewDefaultProjector(),
		nil, nil, nil, nil, nil, nil, 0.3, 2.5,
	)
	drivers := sectorallocation.DriverInputs{
		AsOfTradingDate: "2026-07-17",
		Cycle:           map[industry.SectorID]float64{industry.SectorSemiconductor: 0.05},
	}
	target, err := engine.ComputeProjectedTarget(context.Background(), drivers)
	if err != nil {
		t.Fatalf("ComputeProjectedTarget failed: %v", err)
	}
	if len(target.AdjustmentLog) == 0 {
		t.Fatal("AdjustmentLog must record at least one provenance event when driver applies")
	}
}

func TestDefaultEngine_ComputeProjectedTarget_SumIsOne(t *testing.T) {
	priorMap := makeStrategicPriorForTest(t)
	engine := sectorallocation.NewDefaultEngineWithProjector(
		sectorallocation.NewEngineTestConfig(),
		priorMap,
		sectorallocation.NewDefaultProjector(),
		nil, nil, nil, nil, nil, nil, 0.3, 2.5,
	)
	drivers := sectorallocation.DriverInputs{AsOfTradingDate: "2026-07-17"}
	target, err := engine.ComputeProjectedTarget(context.Background(), drivers)
	if err != nil {
		t.Fatalf("ComputeProjectedTarget failed: %v", err)
	}
	s := 0.0
	for _, v := range target.Target {
		s += v
	}
	if s < (1.0-1e-9) || s > (1.0+1e-9) {
		t.Fatalf("projected target sum drift: %.12f", s)
	}
}

func makeStrategicPriorForTest(t *testing.T) *sectorallocation.StrategicSectorPrior {
	t.Helper()
	return sectorallocation.LoadStrategicPriorFromConfigForTest()
}
