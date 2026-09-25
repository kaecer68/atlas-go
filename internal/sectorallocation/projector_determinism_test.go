package sectorallocation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// issue #1961: the sectorallocation production path was not deterministic.
//
// Root cause: Project.Project accumulated floats while iterating Go maps
// (a map of the 8 driver maps, and the normalization sum over `target`).
// Go randomizes map iteration order per range statement and IEEE-754 addition
// is not associative, so `(a+b)+c != a+(b+c)` made the last bits of the
// normalized weights drift between runs; all 20 canonical L1 values moved
// together by up to 4 ULP.
//
// The fix pins both orders (SA-DET-01): the 8 drivers are applied in one
// declared order and every accumulation walks the sector keys in sorted order.
// These tests are the regression gate: N in-process runs must render the exact
// same JSON, and the historical production-prior scenario must match a
// committed golden byte-for-byte.

const determinismRuns = 20

// determinismRunsLimit keeps the gate cheap; 20 runs already fail with
// probability > 1 - 1e-18 on the pre-fix code (measured drift rate: 44/50
// runs differed from the canonical render).

// productionPriorForTest returns the strategic prior the composition root
// loads in production (internal/config defaults, non-uniform: 0.33/0.16/0.13/
// 0.08 + 16 x 0.01875). Non-uniform weights are what made the pre-fix
// normalization order-sensitive, so the regression must use them.
func productionPriorForTest(t *testing.T) *StrategicSectorPrior {
	t.Helper()
	prior := LoadStrategicPriorFromConfigForTest()
	if len(prior.Weights) != 20 {
		t.Fatalf("production strategic prior keys = %d, want 20", len(prior.Weights))
	}
	return prior
}

// multiDriverInputs returns driver deltas that overlap on several sectors, so a
// single sector receives deltas from more than one driver (the second, less
// obvious order dependency: the driver application order itself).
func multiDriverInputs() DriverInputs {
	l1 := industry.L1Sectors()
	cycle := make(map[industry.SectorID]float64, len(l1))
	seasonal := make(map[industry.SectorID]float64, len(l1))
	linkage := make(map[industry.SectorID]float64, len(l1))
	for i, id := range l1 {
		cycle[id] = float64(i+1) * 0.00037
		if i%3 == 0 {
			seasonal[id] = float64(i) * 0.00011
		}
		if i%5 == 0 {
			linkage[id] = -float64(i) * 0.00023
		}
	}
	return DriverInputs{
		AsOfTradingDate: "2026-07-17",
		Cycle:           cycle,
		Seasonal:        seasonal,
		Linkage:         linkage,
	}
}

// reproductionRuns renders the production projection entry point N times and
// returns the distinct renders in first-seen order with their counts.
func reproductionRuns(t *testing.T, prior *StrategicSectorPrior, drivers func() DriverInputs) (distinct []string, counts []int) {
	t.Helper()
	index := map[string]int{}
	for i := 0; i < determinismRuns; i++ {
		engine := NewDefaultEngineWithProjector(
			NewEngineTestConfig(), prior, NewDefaultProjector(),
			nil, nil, nil, nil, nil, nil, 0.3, 2.5,
		)
		target, err := engine.ComputeProjectedTarget(context.Background(), drivers())
		if err != nil {
			t.Fatalf("run %d: ComputeProjectedTarget: %v", i, err)
		}
		data, err := json.Marshal(target)
		if err != nil {
			t.Fatalf("run %d: marshal: %v", i, err)
		}
		key := sha256Hex(data)
		if pos, ok := index[key]; ok {
			counts[pos]++
			continue
		}
		index[key] = len(distinct)
		distinct = append(distinct, key)
		counts = append(counts, 1)
	}
	return distinct, counts
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestProductionPath_Deterministic_ZeroDriverDeltas is the regression for the
// exact scenario reported in issue #1961: the production prior, no driver
// deltas => the only arithmetic that matters is the normalization sum.
func TestProductionPath_Deterministic_ZeroDriverDeltas(t *testing.T) {
	prior := productionPriorForTest(t)
	distinct, counts := reproductionRuns(t, prior, func() DriverInputs {
		return DriverInputs{AsOfTradingDate: "2026-07-17"}
	})
	if len(distinct) != 1 {
		t.Fatalf("production path rendered %d distinct outputs in %d runs (counts %v); want 1", len(distinct), determinismRuns, counts)
	}

	golden := readGolden(t, productionPriorGoldenPath)
	if distinct[0] != sha256Hex(golden) {
		t.Fatalf("production path output sha256 = %s, golden sha256 = %s (byte-identity broken)", distinct[0], sha256Hex(golden))
	}
}

// TestProductionPath_Deterministic_WithDriverDeltas covers the second order
// dependency: several drivers writing the same sector.
func TestProductionPath_Deterministic_WithDriverDeltas(t *testing.T) {
	prior := productionPriorForTest(t)
	distinct, counts := reproductionRuns(t, prior, multiDriverInputs)
	if len(distinct) != 1 {
		t.Fatalf("driver path rendered %d distinct outputs in %d runs (counts %v); want 1", len(distinct), determinismRuns, counts)
	}
}

// TestProjectorProject_ZeroDriverDeltas_ReturnsPriorExactly pins the canonical
// render: when the drivers contribute nothing and the sorted prior sum is
// exactly 1.0, normalization must be the identity. Pre-fix this held in only
// 12% of runs (the random sum order produced up to 4 ULP of downward drift);
// post-fix it holds in 100%.
func TestProjectorProject_ZeroDriverDeltas_ReturnsPriorExactly(t *testing.T) {
	prior := productionPriorForTest(t)
	raw := map[industry.SectorID]float64{}
	for id, w := range prior.Weights {
		raw[id] = w
	}
	for i := 0; i < determinismRuns; i++ {
		target, err := NewDefaultProjector().Project(raw, DriverInputs{AsOfTradingDate: "2026-07-17"})
		if err != nil {
			t.Fatalf("run %d: Project: %v", i, err)
		}
		for id, want := range prior.Weights {
			if got := target.Target[id]; got != want {
				t.Fatalf("run %d: target[%s] = %v, want prior value %v (normalized by a non-identity sum)", i, id, got, want)
			}
		}
	}
}

// TestProjectorProject_DriverOrderIsDeclarationOrder pins the driver
// application order (and therefore the AdjustmentLog order) so a future
// refactor cannot reintroduce map-order dependence.
func TestProjectorProject_DriverOrderIsDeclarationOrder(t *testing.T) {
	raw := uniformRawForTest(t)
	drivers := DriverInputs{
		AsOfTradingDate: "2026-07-17",
		Cycle:           map[industry.SectorID]float64{"auto": 0.01},
		Seasonal:        map[industry.SectorID]float64{"auto": 0.02},
		Linkage:         map[industry.SectorID]float64{"auto": 0.03},
		Narrative:       map[industry.SectorID]float64{"auto": 0.04},
		Macro:           map[industry.SectorID]float64{"auto": 0.05},
		CapitalFlow:     map[industry.SectorID]float64{"auto": 0.06},
		Theme:           map[industry.SectorID]float64{"auto": 0.07},
		StrategicPrior:  map[industry.SectorID]float64{"auto": 0.08},
	}
	target, err := NewDefaultProjector().Project(raw, drivers)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	want := []string{"cycle", "seasonal", "linkage", "narrative", "macro", "capital_flow", "theme", "prior"}
	if len(target.AdjustmentLog) != len(want) {
		t.Fatalf("AdjustmentLog len = %d, want %d", len(target.AdjustmentLog), len(want))
	}
	for i, reason := range want {
		if got := target.AdjustmentLog[i].Reason; got != reason {
			t.Errorf("AdjustmentLog[%d].Reason = %q, want %q (all keys)", i, got, reason)
		}
		if got := target.AdjustmentLog[i].Sector; got != industry.SectorID("auto") {
			t.Errorf("AdjustmentLog[%d].Sector = %q, want auto", i, got)
		}
	}
}

func uniformRawForTest(t *testing.T) map[industry.SectorID]float64 {
	t.Helper()
	l1 := industry.L1Sectors()
	if len(l1) != 20 {
		t.Fatalf("canonical L1 universe = %d, want 20", len(l1))
	}
	raw := make(map[industry.SectorID]float64, len(l1))
	for _, id := range l1 {
		raw[id] = 1.0 / float64(len(l1))
	}
	return raw
}

func readGolden(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	for len(data) > 0 && (data[len(data)-1] == '\n' || data[len(data)-1] == '\r') {
		data = data[:len(data)-1]
	}
	return data
}

const productionPriorGoldenPath = "testdata/projected_target_production_prior.golden.json"
