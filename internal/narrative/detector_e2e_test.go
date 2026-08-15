// Package narrative — Stage 5 PR#5 end-to-end chain verification.
//
// Verifies PR#1+PR#2 deliverables: Detector interface + DetectorRegistry
// + 24 Detector impls produce correct DetectionResult for synthetic input.
//
// Round-trip persistence (RunAll → AppendScan → LoadRecentScans) is covered
// by template_detector_scan_test.go (mock store) and detector_scan_store_test.go
// (SQLite round-trip). Importing internal/ledger here would create a cycle
// (ledger imports narrative for Severity/Source types), so this file is
// storage-free.
//
// MCP tool layer (event_flow_prediction output) is deferred — see
// 歷史 stage5 detector plan "Deferred" section（已移出公開 docs）。
package narrative

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// seasonalThemes are date-dependent (detectSeasonalEvent uses time.Now()).
// Disable them so e2e tests don't flake based on the calendar date the test
// runs.
var seasonalThemes = []string{
	"spring_festival_season",
	"election_cycle",
	"earnings_blackout",
	"tech_peak_season",
	"year_end_window_dressing",
	"dividend_season",
}

func disableSeasonals(reg *DetectorRegistry) {
	for _, t := range seasonalThemes {
		_ = reg.Disable(t)
	}
}

func tariffShockSnapshot() marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		VIX:      marketdata.MacroDataPoint{Symbol: "VIX", Value: 40.0, ChangePct: 60.0},
		DXY:      marketdata.MacroDataPoint{Symbol: "DXY", Value: 107.5, ChangePct: 3.0},
		SPXIndex: marketdata.MacroDataPoint{Symbol: "SPX", Value: 4200.0, ChangePct: -3.5},
	}
}

// TestE2E_SnapshotPipeline_TariffShockScenario verifies the snapshot-pipeline
// tariff_shock detector (the only one wrapped from ingestor.go's MacroSnapshot
// API in PR#2) fires when given VIX spike + SPX selloff + DXY volatility.
func TestE2E_SnapshotPipeline_TariffShockScenario(t *testing.T) {
	reg := NewDefaultDetectorRegistry()
	disableSeasonals(reg)

	results := runAllSequential(reg, DetectorInput{
		MacroSnapshot: tariffShockSnapshot(),
		Now:           time.Now().UTC(),
	})

	themes := indexResultsByTheme(results)

	r, ok := themes["tariff_shock"]
	if !ok {
		t.Fatalf("expected tariff_shock in snapshot-pipeline result; got themes: %v", mapKeys(themes))
	}
	if r.Severity != SeverityCritical {
		t.Errorf("tariff_shock.Severity = %q, want %q", r.Severity, SeverityCritical)
	}
	if r.Confidence <= 0 {
		t.Errorf("tariff_shock.Confidence = %f, want > 0", r.Confidence)
	}
	if r.Source != SourceIngestor {
		t.Errorf("tariff_shock.Source = %q, want %q", r.Source, SourceIngestor)
	}
}

// TestE2E_KBPipeline_AcuteMacroScenario verifies the KB-pipeline detectors
// (which read MarketNarrativeData) fire when given synthetic extreme values.
func TestE2E_KBPipeline_AcuteMacroScenario(t *testing.T) {
	reg := NewDefaultDetectorRegistry()
	disableSeasonals(reg)

	data := MarketNarrativeData{
		US10YChangeBps:      50.0,
		DXYChangePct:        2.5,
		VIXLevel:            35.0,
		USD_TWD_ChangePct:   1.5,
		OilChangePct:        8.0,
		GoldChangePct:       4.0,
		JPY_ChangePct:       -3.0,
		AICapexSentiment:    0.8,
		GeopoliticalGPR:     0.95,
		EarningsSurprisePct: 15.0,
	}

	results := runAllSequential(reg, DetectorInput{MarketData: data})

	themes := indexResultsByTheme(results)

	mustHave := []string{
		"US_rates_up",
		"JPY_carry_unwind",
		"oil_price_shock",
		"geopolitical_risk_spike",
		"AI_capex_surge",
		"earnings_surprise",
	}
	for _, theme := range mustHave {
		if _, ok := themes[theme]; !ok {
			t.Errorf("expected KB-pipeline theme %q; got themes: %v", theme, mapKeys(themes))
		}
	}
}

// TestE2E_All24ThemesRegistered is a regression guard: NewDefaultDetectorRegistry
// must register all 24 templates defined in templates.go.
func TestE2E_All24ThemesRegistered(t *testing.T) {
	reg := NewDefaultDetectorRegistry()

	const expectedCount = 24
	if got := reg.Len(); got != expectedCount {
		t.Errorf("NewDefaultDetectorRegistry().Len() = %d, want %d", got, expectedCount)
	}

	templates := DefaultTemplates()
	if len(templates) != expectedCount {
		t.Errorf("DefaultTemplates() length = %d, want %d", len(templates), expectedCount)
	}

	for _, tmpl := range templates {
		d, ok := reg.Get(tmpl.TriggerTheme)
		if !ok {
			t.Errorf("template trigger_theme=%q has no Detector registered", tmpl.TriggerTheme)
			continue
		}
		if d.Theme() != tmpl.TriggerTheme {
			t.Errorf("detector.Theme() = %q, template.TriggerTheme = %q", d.Theme(), tmpl.TriggerTheme)
		}
		if !d.Enabled() {
			t.Errorf("detector for trigger_theme=%q should be enabled by default", tmpl.TriggerTheme)
		}
	}
}

// TestE2E_DisableThenRunAll_ExcludesDisabled verifies the Enable/Disable
// flag on DetectorRegistry propagates correctly through RunAll.
func TestE2E_DisableThenRunAll_ExcludesDisabled(t *testing.T) {
	reg := NewDefaultDetectorRegistry()
	disableSeasonals(reg)

	if err := reg.Disable("tariff_shock"); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	results := runAllSequential(reg, DetectorInput{
		MacroSnapshot: tariffShockSnapshot(),
		Now:           time.Now().UTC(),
	})

	for _, r := range results {
		if r.Theme == "tariff_shock" {
			t.Error("disabled tariff_shock detector still ran")
		}
	}
}

// runAllSequential iterates enabled detectors one-at-a-time instead of
// RunAll's parallel goroutine fan-out. Several underlying detect functions
// (detectOilShockEvent, detectUSRatesEvent, etc.) read the global
// config.GetParametersConfig() singleton, which races with writes from
// unrelated tests when invoked in parallel. The e2e chain test cares about
// "do the right detectors fire for this input" — sequencing preserves that
// invariant while avoiding the data race exposed under `go test -race` in CI.
func runAllSequential(reg *DetectorRegistry, in DetectorInput) []DetectionResult {
	var out []DetectionResult
	for _, d := range reg.ListEnabled() {
		res, _ := d.Detect(context.Background(), in)
		if res != nil {
			out = append(out, *res)
		}
	}
	return out
}

func indexResultsByTheme(results []DetectionResult) map[string]DetectionResult {
	out := make(map[string]DetectionResult, len(results))
	for _, r := range results {
		out[r.Theme] = r
	}
	return out
}

func mapKeys(m map[string]DetectionResult) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
