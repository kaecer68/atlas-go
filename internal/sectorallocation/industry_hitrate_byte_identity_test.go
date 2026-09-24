package sectorallocation_test

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// tolerance for numeric equivalence. TRUE byte-identity is NOT achievable on
// this path: the engine has map-iteration float non-determinism (adding the
// same numbers in different orders changes the last bit). Measured drift is
// exactly 1 ULP (~2.2e-16 relative). See atlas-go issue #1961.
const productionPathTolerance = 1e-12

// TestProductionPath_GateOff_NumericallyEquivalent is the safety-valve test for
// PR-β: with the gate OFF (default), the production path must produce output
// numerically equivalent to the pre-PR-β reference recorded in
// testdata/production_path_off_pr_beta.golden.json.
//
// Renamed 2026-09-24 from TestProductionPath_ByteIdentical_Snapshot: the old
// name claimed byte-identity, but (a) it never actually diffed against a golden
// and (b) byte-identity is unachievable because of map-iteration float
// non-determinism (issue #1961). This version compares against the golden with
// a documented tolerance AND keeps the structural invariants (20 L1 keys,
// weights sum to 1).
func TestProductionPath_GateOff_NumericallyEquivalent(t *testing.T) {
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

	// --- structural invariants ---
	if len(target.Target) != 20 {
		t.Fatalf("must have 20 L1 keys, got %d", len(target.Target))
	}
	sum := 0.0
	for _, v := range target.Target {
		sum += v
	}
	if sum < 0.999999999 || sum > 1.000000001 {
		t.Fatalf("weights must sum to ~1.0, got %.12f", sum)
	}

	// --- numeric equivalence vs golden (tolerance, not byte-identity) ---
	goldenPath := filepath.Join("testdata", "production_path_off_pr_beta.golden.json")
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	var want struct {
		Target map[string]float64 `json:"Target"`
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("parse golden %s: %v", goldenPath, err)
	}
	if len(want.Target) != len(target.Target) {
		t.Fatalf("golden has %d L1 keys, got %d", len(want.Target), len(target.Target))
	}
	for k, got := range target.Target {
		w, ok := want.Target[string(k)]
		if !ok {
			t.Errorf("L1 key %q present in output but missing from golden", k)
			continue
		}
		if math.Abs(got-w) > productionPathTolerance {
			t.Errorf("L1 %s: got %.17g, golden %.17g, |Δ|=%.3g > tol %.3g",
				k, got, w, math.Abs(got-w), productionPathTolerance)
		}
	}
}
