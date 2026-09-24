package main

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/kaecer68/atlas-go/internal/sectorallocation"
	"github.com/kaecer68/atlas-go/internal/sectormap"
)

func sectorallocationETFSymbols() []string { return sectorallocation.ETFRepresentativeSymbols() }

// etfCoverageFloor mirrors the acceptance floor for ETF L1 coverage. It is
// deliberately duplicated here: this test guards the *published report*, and a
// change to the floor has to be made in both places on purpose.
const etfCoverageFloor = 12

func TestBuildETFReport_PublishesTheCoverageFloor(t *testing.T) {
	rep := buildETFReport()
	if rep.Count < etfCoverageFloor {
		t.Fatalf("audit reports ETF L1 coverage %d, floor is %d (%v)", rep.Count, etfCoverageFloor, rep.L1Covered)
	}
	if rep.Count != len(rep.L1Covered) {
		t.Errorf("count %d != len(l1_covered) %d", rep.Count, len(rep.L1Covered))
	}
	if rep.CanonicalL1Total != len(sectormap.CanonicalL1IDs()) {
		t.Errorf("canonical_l1_total = %d, taxonomy has %d", rep.CanonicalL1Total, len(sectormap.CanonicalL1IDs()))
	}
	if !slices.IsSorted(rep.L1Covered) {
		t.Errorf("l1_covered is not sorted: %v", rep.L1Covered)
	}
}

func TestBuildETFReport_RowsCoverEveryDeclaredETFWithNoUnmappedL1(t *testing.T) {
	rep := buildETFReport()
	symbols := make([]string, 0, len(rep.Rows))
	for _, r := range rep.Rows {
		symbols = append(symbols, r.Symbol)
		if r.UnmappedL1 != 0 {
			t.Errorf("%s publishes %d non-canonical L1 keys", r.Symbol, r.UnmappedL1)
		}
		if len(r.L1Covered) == 0 || len(r.L1Weights) != len(r.L1Covered) {
			t.Errorf("%s: %d covered vs %d weights", r.Symbol, len(r.L1Covered), len(r.L1Weights))
		}
		if r.AsOf == "" || r.SourceURL == "" {
			t.Errorf("%s: the published row must carry its evidence (as_of/source_url)", r.Symbol)
		}
		sum := 0.0
		for _, w := range r.L1Weights {
			sum += w
		}
		if sum < 0.999999 || sum > 1.000001 {
			t.Errorf("%s: weights sum to %v", r.Symbol, sum)
		}
	}
	if !slices.IsSorted(symbols) {
		t.Errorf("rows are not sorted by symbol: %v", symbols)
	}
	if !slices.Equal(symbols, sectorallocationETFSymbols()) {
		t.Errorf("rows %v != declared ETF symbols %v", symbols, sectorallocationETFSymbols())
	}
	if rep.Symbols != len(rep.Rows) {
		t.Errorf("symbols = %d, rows = %d", rep.Symbols, len(rep.Rows))
	}
}

func TestBuildETFReport_JSONShapeIsStable(t *testing.T) {
	raw, err := json.Marshal(struct {
		ETF *etfCoverageReport `json:"etf_l1_coverage,omitempty"`
	}{ETF: buildETFReport()})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		ETF struct {
			Symbols   int              `json:"symbols"`
			Count     int              `json:"count"`
			L1Covered []string         `json:"l1_covered"`
			Rows      []etfCoverageRow `json:"rows"`
		} `json:"etf_l1_coverage"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ETF.Count == 0 || decoded.ETF.Symbols == 0 || len(decoded.ETF.Rows) == 0 {
		t.Fatalf("etf_l1_coverage block lost its content in JSON: %s", raw[:min(len(raw), 200)])
	}
	if len(decoded.ETF.Rows) > 0 && len(decoded.ETF.Rows[0].L1Weights) == 0 {
		t.Error("per-ETF l1_weights did not survive the JSON round trip")
	}
}
