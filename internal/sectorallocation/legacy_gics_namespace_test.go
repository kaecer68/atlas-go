package sectorallocation_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
	"github.com/kaecer68/atlas-go/internal/sectormap"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, f, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(f), "..", "..")
}

// loadBaseWeights reads the live GICS-style base_weights from the split
// parameters file, so this test fails when the config vocabulary drifts.
func loadBaseWeights(t *testing.T) map[string]float64 {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "configs", "parameters", "sector_allocation.json"))
	if err != nil {
		t.Fatalf("read sector_allocation.json: %v", err)
	}
	var doc struct {
		BaseWeights map[string]float64 `json:"base_weights"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse sector_allocation.json: %v", err)
	}
	if len(doc.BaseWeights) == 0 {
		t.Fatal("base_weights is empty")
	}
	return doc.BaseWeights
}

func TestProjectLegacyGICSWeights_ReportsUnmappedKeys(t *testing.T) {
	base := loadBaseWeights(t)

	proj, err := sectorallocation.ProjectLegacyGICSWeights(base)
	if !errors.Is(err, sectorallocation.ErrLegacyGICSUnmapped) {
		t.Fatalf("err = %v, want ErrLegacyGICSUnmapped", err)
	}

	wantUnmapped := []string{"_cash_reserve", "industrials", "materials", "real_estate", "utilities"}
	if !slices.Equal(proj.UnmappedKeys, wantUnmapped) {
		t.Errorf("unmapped keys = %v, want %v", proj.UnmappedKeys, wantUnmapped)
	}
	wantBlocked := base["_cash_reserve"] + base["industrials"] + base["materials"] + base["real_estate"] + base["utilities"]
	if diff := proj.BlockedWeight - wantBlocked; diff > 1e-12 || diff < -1e-12 {
		t.Errorf("blocked weight = %v, want %v", proj.BlockedWeight, wantBlocked)
	}

	// Candidates are documentation for the operator decision; industries must
	// not gain an automatic target.
	if len(proj.Candidates["industrials"]) == 0 {
		t.Error("industrials should list candidate canonical L1 sectors")
	}
	if _, present := proj.Candidates["_cash_reserve"]; present {
		t.Error("_cash_reserve is an asset class and must not have equity candidates")
	}
	for _, key := range proj.UnmappedKeys {
		for _, id := range proj.Candidates[key] {
			if !id.IsL1() {
				t.Errorf("candidate %s for %s is not a canonical L1 sector", id, key)
			}
		}
	}

	// The translatable part must be exact.
	want := map[industry.SectorID]float64{
		industry.SectorSemiconductor: base["semiconductor"],
		industry.SectorElectronics:   base["electronics"],
		industry.SectorFinancials:    base["financials"],
		industry.SectorEnergy:        base["energy"],
		industry.SectorTelecom:       base["telecom"],
		industry.SectorBiotech:       base["healthcare"], // GICS Health Care → 生技醫療
		industry.SectorRetail:        base["consumer"],   // GICS Consumer → canonical L2 consumer → retail
	}
	if len(proj.Weights) != len(want) {
		t.Errorf("projected L1 weights = %v, want keys %v", proj.Weights, want)
	}
	for id, w := range want {
		if diff := proj.Weights[id] - w; diff > 1e-12 || diff < -1e-12 {
			t.Errorf("Weights[%s] = %v, want %v", id, proj.Weights[id], w)
		}
	}
	for id := range proj.Weights {
		if !id.IsL1() {
			t.Errorf("projected vector contains non-L1 key %s", id)
		}
	}
}

func TestProjectLegacyGICSWeights_TranslatableSubsetIsUsable(t *testing.T) {
	base := loadBaseWeights(t)
	// Drop the keys the operator has not decided yet.
	for _, key := range []string{"_cash_reserve", "industrials", "materials", "real_estate", "utilities"} {
		delete(base, key)
	}

	proj, err := sectorallocation.ProjectLegacyGICSWeights(base)
	if err != nil {
		t.Fatalf("translatable subset must project without error: %v", err)
	}
	if len(proj.UnmappedKeys) != 0 {
		t.Errorf("unmapped keys = %v, want none", proj.UnmappedKeys)
	}
	sum := 0.0
	for _, w := range proj.Weights {
		sum += w
	}
	if diff := sum - 0.75; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("projected sum = %v, want 0.75 (no renormalization)", sum)
	}
}

func TestLegacyGICSKeysAreDeclaredInNamespaceRegistry(t *testing.T) {
	base := loadBaseWeights(t)
	declared := sectormap.Keys(sectormap.NamespaceGICSBaseWeights)
	got := slices.Sorted(slices.Values(func() []string {
		out := make([]string, 0, len(base))
		for k := range base {
			out = append(out, k)
		}
		return out
	}()))
	if !slices.Equal(got, declared) {
		t.Errorf("base_weights keys drifted from internal/sectormap:\n config:   %v\n declared: %v", got, declared)
	}
}

func TestLegacyGICSKeySpaceCannotProduceL1FinalTarget(t *testing.T) {
	base := loadBaseWeights(t)

	// The legacy engine iterates base_weights, so its output key space is the
	// GICS one. That vector can never satisfy SA-INV-01, which is why the
	// canonical path is ComputeProjectedTarget (StrategicPrior), not
	// ComputeWeights.
	legacy := sectorallocation.L1FinalTarget{Weights: map[industry.SectorID]float64{}}
	share := 1.0 / float64(len(base))
	for k := range base {
		legacy.Weights[industry.SectorID(k)] = share
	}
	if err := sectorallocation.ValidateL1FinalTarget(legacy); err == nil {
		t.Fatal("legacy base_weights key space unexpectedly validates as an L1 final target")
	}
}

func TestFilterL1KeysReport_ReportsDroppedKeys(t *testing.T) {
	in := map[string]float64{
		"semiconductor": 0.3,
		"healthcare":    0.05,
		"_cash_reserve": 0.02,
	}
	kept, dropped := sectorallocation.FilterL1KeysReport(in)
	if len(kept) != 1 || kept["semiconductor"] != 0.3 {
		t.Errorf("kept = %v, want only semiconductor", kept)
	}
	if !slices.Equal(dropped, []string{"_cash_reserve", "healthcare"}) {
		t.Errorf("dropped = %v, want [_cash_reserve healthcare]", dropped)
	}
}

func TestProjectedL1Keys_OnlyCanonicalL1(t *testing.T) {
	keys := sectorallocation.ProjectedL1Keys(map[string]float64{
		"semiconductor": 0.5,
		"consumer":      0.2,
		"materials":     0.3,
	})
	if !slices.Contains(keys, industry.SectorSemiconductor) || !slices.Contains(keys, industry.SectorRetail) {
		t.Errorf("ProjectedL1Keys = %v, want semiconductor and retail", keys)
	}
	for _, id := range keys {
		if !id.IsL1() {
			t.Errorf("ProjectedL1Keys returned non-L1 %s", id)
		}
	}
}
