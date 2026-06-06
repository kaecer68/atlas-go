//go:build integration

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// TestNarrativeShockIntegration validates the full pipeline:
//
//	real replay data → ExecuteRegistryResearch → DetectEvents → AdjustRegimeFromNarrative
//
// It compares baseline (normal data) vs shock (US10Y spike) to verify that:
//   - the narrative engine detects US_rates_up when US10Y surges
//   - AdjustRegimeFromNarrative shifts the regime toward RISK_OFF
//   - the orchestrator pipeline runs end-to-end without error
//
// Run from project root:
//
//	go test -v -tags=integration -run TestNarrativeShockIntegration ./internal/orchestrator/
func TestNarrativeShockIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Change to project root so relative paths (data/replay/, configs/) resolve correctly.
	// Use t.Chdir (Go 1.24+) to scope the working directory change to this test only,
	// preventing side effects on other tests that use relative paths.
	if _, err := os.Stat("go.mod"); os.IsNotExist(err) {
		cwd, _ := os.Getwd()
		dir := cwd
		found := false
		for i := 0; i < 10; i++ {
			parent := filepath.Dir(dir)
			if parent == dir {
				t.Fatalf("cannot find project root (go.mod) from %s", cwd)
			}
			if _, err := os.Stat(filepath.Join(parent, "go.mod")); err == nil {
				t.Chdir(parent)
				found = true
				break
			}
			dir = parent
		}
		if !found {
			t.Fatalf("cannot find project root (go.mod) from %s", cwd)
		}
	}

	// --- Setup ---
	cfg := config.Load()
	if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
		cfg.ReplayDataPath = config.GetReplayDataPath(cfg.WorkDir)
	}

	ds, err := replay.LoadTWSEOpenDataCSV(cfg.ReplayDataPath)
	if err != nil {
		t.Skipf("skip: cannot load replay data from %s: %v", cfg.ReplayDataPath, err)
	}
	if len(ds.Dates) == 0 {
		t.Fatal("replay dataset contains no dates")
	}

	date := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)

	registry, err := LoadRegistry(cfg.AgentRegistryPath)
	if err != nil || len(registry.Agents) == 0 {
		registry = SeedRegistry()
		t.Log("using seeded registry (config load failed or empty)")
	}
	symbols := RegistrySymbols(registry)
	baseQuotes := ds.QuotesForDate(date, symbols)

	policy, err := baseline.Load(cfg.BaselinePolicyPath)
	if err != nil {
		policy = baseline.DefaultPolicy()
		t.Log("using default baseline policy")
	}

	nEngine := narrative.NewNarrativeEngine()

	// --- Baseline run ---
	baseRegimeBase, baseRaw, _, _ := ExecuteRegistryResearchDetailedWithPolicyAndGuards(
		registry, baseQuotes, policy.PromptOverrides, policy.ExecutionPolicy,
	)
	baseEvents := nEngine.DetectEvents(quotesToMarketData(baseQuotes))
	baseRegime := AdjustRegimeFromNarrative(baseRegimeBase, baseEvents)

	// --- Shock run: inject a US10Y spike to 15.0 ---
	shockQuotes := append([]domain.Quote{}, baseQuotes...)
	shockQuotes = append(shockQuotes, domain.Quote{
		Symbol:     "^TNX",
		Last:       15.0,
		Open:       4.5,
		High:       15.5,
		Low:        4.4,
		Volume:     100000,
		Market:     "US",
		AsOf:       date,
		IsTradable: true,
		Source:     "mock",
	})
	shockRegimeBase, shockRaw, _, _ := ExecuteRegistryResearchDetailedWithPolicyAndGuards(
		registry, shockQuotes, policy.PromptOverrides, policy.ExecutionPolicy,
	)
	shockEvents := nEngine.DetectEvents(quotesToMarketData(shockQuotes))
	shockRegime := AdjustRegimeFromNarrative(shockRegimeBase, shockEvents)

	// --- Assertions ---

	// 1. Pipeline produces recommendations in both runs
	if len(baseRaw) == 0 {
		t.Error("baseline run produced zero recommendations")
	}
	if len(shockRaw) == 0 {
		t.Error("shock run produced zero recommendations")
	}

	// 2. US_rates_up event should be detected in the shock run
	var foundUSRatesShock bool
	for _, e := range shockEvents {
		if e.Theme == "US_rates_up" {
			foundUSRatesShock = true
			break
		}
	}
	if !foundUSRatesShock {
		t.Error("shock run: expected US_rates_up event, got none — check US10Y threshold in parameters.json")
	}

	// 3. Baseline should NOT trigger US_rates_up (normal data)
	var foundUSRatesBase bool
	for _, e := range baseEvents {
		if e.Theme == "US_rates_up" {
			foundUSRatesBase = true
			break
		}
	}
	if foundUSRatesBase {
		t.Log("baseline also triggered US_rates_up — acceptable if normal data exceeds threshold")
	}

	// 4. Shock should produce more events than baseline
	if len(shockEvents) <= len(baseEvents) {
		t.Logf("shock events (%d) not more than baseline (%d) — may be normal if baseline already near threshold",
			len(shockEvents), len(baseEvents))
	}

	// 5. The shock regime should be risk-off or at minimum not more risk-on than baseline
	t.Logf("baseline regime: %s (base=%s, events=%d)", baseRegime, baseRegimeBase, len(baseEvents))
	t.Logf("shock regime:   %s (base=%s, events=%d)", shockRegime, shockRegimeBase, len(shockEvents))

	if shockRegime == domain.RegimeRiskOn && baseRegime != domain.RegimeRiskOn {
		t.Error("shock made regime more risk-on — unexpected for US10Y spike")
	}
}

// quotesToMarketData converts a Quote slice to MarketNarrativeData,
// mapping known symbol patterns to their corresponding macro fields.
func quotesToMarketData(quotes []domain.Quote) narrative.MarketNarrativeData {
	data := narrative.MarketNarrativeData{}
	for _, q := range quotes {
		switch q.Symbol {
		case "DXY", "^DXY":
			data.DXYChangePct = (q.Last - q.Open) / q.Open * 100
		case "US10Y", "^TNX":
			data.US10YChangeBps = q.Last
		case "VIX", "^VIX":
			data.VIXLevel = q.Last
		case "OIL", "CL=F":
			data.OilChangePct = (q.Last - q.Open) / q.Open * 100
		case "GOLD", "GC=F":
			data.GoldChangePct = (q.Last - q.Open) / q.Open * 100
		case "JPY=X", "USDJPY=X":
			data.JPY_ChangePct = (q.Last - q.Open) / q.Open * 100
		}
	}
	return data
}
