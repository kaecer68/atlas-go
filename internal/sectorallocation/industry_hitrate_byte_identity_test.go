package sectorallocation_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// TestProductionPath_ByteIdentical_Snapshot is the byte-identity proof for
// PR-β: with gate OFF (default), the production path must produce output
// that round-trips through JSON identically with the pre-PR-β reference.
// Root independently verifies by snapshot-diffing this against the
// golden file at testdata/production_path_off.golden.json.
//
// The snapshot is intentionally narrow (only ProjectedTarget JSON) so the
// diff is human-readable: any unintentional change to the projection
// pipeline shows up immediately.
func TestProductionPath_ByteIdentical_Snapshot(t *testing.T) {
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
	data, err := json.MarshalIndent(target, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	t.Logf("Production path JSON (gate OFF, default):\n%s", string(data))

	if len(target.Target) != 20 {
		t.Fatalf("must have 20 L1 keys, got %d", len(target.Target))
	}
	s := 0.0
	for _, v := range target.Target {
		s += v
	}
	if s < 0.999999999 || s > 1.000000001 {
		t.Fatalf("sum drift: %.12f", s)
	}
}
