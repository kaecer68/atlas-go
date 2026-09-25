package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/symbolindustry"
)

// ─── issue #1943: per-stock industry substrate wiring ───────────────────────

// withSymbolIndustryGate flips the config gate for one test and restores the
// previous parameters singleton on cleanup (same pattern as
// sectorallocation.withHitRateConfig).
func withSymbolIndustryGate(t *testing.T, enabled bool) {
	t.Helper()
	prevPath := config.GetParametersConfigPath()
	cfg := config.DefaultParametersConfig()
	cfg.Industry.SubstrateFromSymbolIndustryEnabled.Value = enabled
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal test parameters: %v", err)
	}
	path := filepath.Join(t.TempDir(), "parameters.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write test parameters: %v", err)
	}
	config.SetParametersConfigPath(path)
	config.ResetParametersConfig()
	t.Cleanup(func() {
		config.SetParametersConfigPath(prevPath)
		config.ResetParametersConfig()
	})
	if got := config.GetIndustrySubstrateFromSymbolIndustryEnabled(); got != enabled {
		t.Fatalf("gate did not load: got %v, want %v", got, enabled)
	}
}

// writeSymbolIndustrySnapshot writes a channel snapshot with one row per
// (symbol, code) pair into <workDir>/data/state/symbol_industry.json.
func writeSymbolIndustrySnapshot(t *testing.T, workDir string, updatedAt time.Time, rows map[string]string) {
	t.Helper()
	entries := make([]symbolindustry.Entry, 0, len(rows))
	mapped, unmapped := 0, 0
	l1 := map[string]struct{}{}
	for sym, l1ID := range rows {
		status := symbolindustry.StatusMapped
		if l1ID == "" {
			status = symbolindustry.StatusUnmapped
			unmapped++
		} else {
			mapped++
			l1[l1ID] = struct{}{}
		}
		entries = append(entries, symbolindustry.Entry{
			Symbol:         sym,
			CompanyName:    "公司" + sym,
			Market:         "TWSE",
			IndustryCode:   "24",
			IndustryNameZH: "半導體業",
			CanonicalL1:    l1ID,
			MappingStatus:  status,
			Source:         "TWSE:t187ap03_L",
			AsOf:           updatedAt.Format("2006-01-02"),
		})
	}
	snap := map[string]any{
		"channel":    "symbol_industry",
		"updated_at": updatedAt.UTC().Format(time.RFC3339),
		"sources":    []string{"TWSE:t187ap03_L"},
		"counts": map[string]int{
			"total": len(entries), "mapped": mapped, "unmapped": unmapped,
			"unknown": 0, "canonical_l1": len(l1),
		},
		"entries": entries,
	}
	dir := filepath.Join(workDir, "data", "state")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, symbolIndustryStateFileName), raw, 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
}

// TestSymbolIndustryRefreshTask_MirrorsSnapshotIntoStore is the end-to-end
// mirror path: the task reads the channel snapshot and lands it in the
// queryable per-stock industry field through the backend-aware store, re-running
// as a no-op once the row count matches.
func TestSymbolIndustryRefreshTask_MirrorsSnapshotIntoStore(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Load()
	cfg.WorkDir = workDir
	cfg.StoreBackend = "sqlite"

	writeSymbolIndustrySnapshot(t, workDir, time.Now(), map[string]string{
		"2330": "semiconductor",
		"2603": "shipping",
		"9999": "", // unmapped upstream code: reported, never imputed
	})

	task := symbolIndustryRefreshTask(nil, cfg, nil, nil)
	if err := task(context.Background()); err != nil {
		t.Fatalf("refresh task: %v", err)
	}

	store, err := symbolindustry.NewStore(context.Background(), cfg.StoreBackend, nil, workDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	entries, err := store.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("load all: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("stored rows = %d, want 3", len(entries))
	}

	// Second run: the snapshot is fresh and the row count matches, so the task
	// must not touch the store again (idempotent steady state).
	if err := task(context.Background()); err != nil {
		t.Fatalf("second refresh task run: %v", err)
	}
	if n, err := store.Count(context.Background()); err != nil || n != 3 {
		t.Fatalf("row count after re-run = %d (%v), want 3", n, err)
	}
}

// TestNewSymbolIndustrySubstrate_GateOffInstallsNothing pins the reversibility
// contract: with the gate off the constructor returns nil, which is what keeps
// every consumer byte-identical with the pre-#1943 revision.
func TestNewSymbolIndustrySubstrate_GateOffInstallsNothing(t *testing.T) {
	withSymbolIndustryGate(t, false)
	workDir := t.TempDir()
	cfg := config.Load()
	cfg.WorkDir = workDir
	cfg.StoreBackend = "sqlite"

	writeSymbolIndustrySnapshot(t, workDir, time.Now(), map[string]string{"2330": "semiconductor"})

	if sub := newSymbolIndustrySubstrate(context.Background(), cfg, nil); sub != nil {
		t.Fatalf("gate off must install nothing, got %T", sub)
	}
}

// TestNewSymbolIndustrySubstrate_GateOnLoadsPerStockField is the "母體變大"
// evidence: with the gate on, the substrate answers for symbols the
// representative-stock tables never contained, and it refuses to invent a
// sector for unmapped upstream codes.
func TestNewSymbolIndustrySubstrate_GateOnLoadsPerStockField(t *testing.T) {
	withSymbolIndustryGate(t, true)
	workDir := t.TempDir()
	cfg := config.Load()
	cfg.WorkDir = workDir
	cfg.StoreBackend = "sqlite"

	// A synthetic full-market slice: 3 rows per canonical L1 sector, one
	// deliberate unmapped code, and one symbol the representative-stock tables
	// never declared.
	rows := map[string]string{
		"2330": "semiconductor",
		"2603": "shipping",
		"6116": "optoelectronics",
		"9999": "", // unmapped code: must stay unresolved
	}
	l1s := []string{
		"cement", "food", "plastics", "textiles", "machinery", "steel", "auto",
		"construction", "tourism", "financials", "retail", "chemicals", "biotech",
		"energy", "semiconductor", "electronics", "optoelectronics", "telecom",
		"other_electronics", "shipping",
	}
	synthetic := 0
	for i, l1 := range l1s {
		for j := 1; j <= 3; j++ {
			sym := fmt.Sprintf("%02d%02d", 10+i%20, j)
			rows[sym] = l1
			synthetic++
		}
	}
	writeSymbolIndustrySnapshot(t, workDir, time.Now(), rows)
	if err := symbolIndustryRefreshTask(nil, cfg, nil, nil)(context.Background()); err != nil {
		t.Fatalf("mirror snapshot: %v", err)
	}

	sub := newSymbolIndustrySubstrate(context.Background(), cfg, nil)
	if sub == nil {
		t.Fatal("gate on must install the substrate")
	}
	// The population is exactly the rows carrying a canonical L1: every
	// synthetic row plus the hand-written mapped rows, minus the one unmapped
	// code that must stay unresolved. Counted from the fixture so a collision in
	// the synthetic code space cannot silently change the expectation.
	mappedRows := 0
	for _, l1 := range rows {
		if l1 != "" {
			mappedRows++
		}
	}
	if got := len(sub.Symbols()); got != mappedRows {
		t.Fatalf("population = %d, want %d symbols with a canonical L1", got, mappedRows)
	}
	if id, ok := sub.ResolveL1("6116"); !ok || id != industry.SectorOptoelectronics {
		t.Fatalf("ResolveL1(6116) = %q, %v; want optoelectronics", id, ok)
	}
	if _, ok := sub.ResolveL1("9999"); ok {
		t.Fatal("an unmapped upstream code must stay unresolved, never imputed")
	}
	if _, ok := sub.ResolveL1("1234"); ok {
		t.Fatal("an unknown symbol must stay unresolved")
	}

	// The built population must exceed the representative-stock universe the
	// pre-#1943 wiring was limited to.
	repoUniverse := industry.ComputeCanonicalCoverage(
		industry.DeclaredRepresentativeUniverse(industry.DefaultClassification(), industry.Level1),
		mustSymbolL1Mapper(t))
	if len(sub.Symbols()) <= repoUniverse.Universe {
		t.Fatalf("substrate population %d must exceed the declared representative universe %d",
			len(sub.Symbols()), repoUniverse.Universe)
	}
	t.Logf("population: representative-stock universe=%d -> per-stock field=%d",
		repoUniverse.Universe, len(sub.Symbols()))
}

func mustSymbolL1Mapper(t *testing.T) *industry.SymbolL1Mapper {
	t.Helper()
	m, err := industry.NewSymbolL1Mapper(industry.DefaultClassification())
	if err != nil {
		t.Fatalf("NewSymbolL1Mapper: %v", err)
	}
	return m
}
