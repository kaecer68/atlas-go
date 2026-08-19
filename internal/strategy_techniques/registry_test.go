package strategy_techniques

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFromFile_ValidSeedFile(t *testing.T) {
	path := filepath.Join("testdata", "seed.json")
	reg, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile(%q) error = %v, want nil", path, err)
	}
	if got := reg.Count(); got != 3 {
		t.Errorf("Count() = %d, want 3", got)
	}
}

func TestLoadFromFile_FileNotFound(t *testing.T) {
	_, err := LoadFromFile("testdata/nonexistent.json")
	if err == nil {
		t.Fatal("LoadFromFile on missing file: err = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Errorf("error %q does not mention 'read'", err.Error())
	}
}

func TestLoadFromBytes_MalformedJSON(t *testing.T) {
	_, err := LoadFromBytes([]byte("not valid json {{{"))
	if err == nil {
		t.Fatal("LoadFromBytes on malformed JSON: err = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("error %q does not mention 'unmarshal'", err.Error())
	}
}

func TestLoadFromBytes_InvalidFrame(t *testing.T) {
	// frame with empty ID should fail Validate
	bad := []byte(`[{"id":"","name":"x","layer":"L1","summary":"x","direction":"up","risk":"medium","source":"backtest","status":"active","attribution_mode":"rule_based","conditions":[]}]`)
	_, err := LoadFromBytes(bad)
	if err == nil {
		t.Fatal("LoadFromBytes with empty ID: err = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error %q does not mention 'invalid'", err.Error())
	}
}

func TestLoadFromBytes_InvalidEnum(t *testing.T) {
	// frame with invalid Layer enum should fail Validate
	bad := []byte(`[{"id":"x","name":"x","layer":"L99","summary":"x","direction":"up","risk":"medium","source":"backtest","status":"active","attribution_mode":"rule_based","conditions":[{"field":"f","operator":"gt","value":1,"string_value":"","timeframe":"1D","source":"s"}]}]`)
	_, err := LoadFromBytes(bad)
	if err == nil {
		t.Fatal("LoadFromBytes with invalid Layer: err = nil, want non-nil")
	}
}

func TestRegistry_Count(t *testing.T) {
	reg, err := LoadFromFile(filepath.Join("testdata", "seed.json"))
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if got := reg.Count(); got != 3 {
		t.Errorf("Count() = %d, want 3", got)
	}
}

func TestRegistry_All(t *testing.T) {
	reg, err := LoadFromFile(filepath.Join("testdata", "seed.json"))
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	all := reg.All()
	if len(all) != 3 {
		t.Errorf("len(All()) = %d, want 3", len(all))
	}
}

func TestRegistry_FindByID_Known(t *testing.T) {
	reg, err := LoadFromFile(filepath.Join("testdata", "seed.json"))
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	f, err := reg.FindByID("nvda-test")
	if err != nil {
		t.Fatalf("FindByID(nvda-test): err = %v, want nil", err)
	}
	if f.ID != "nvda-test" {
		t.Errorf("frame.ID = %q, want nvda-test", f.ID)
	}
	if f.Layer != LayerL3IndustryCatalysts {
		t.Errorf("frame.Layer = %q, want L3", f.Layer)
	}
}

func TestRegistry_FindByID_Unknown(t *testing.T) {
	reg, err := LoadFromFile(filepath.Join("testdata", "seed.json"))
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	_, err = reg.FindByID("nonexistent")
	if err == nil {
		t.Fatal("FindByID(nonexistent): err = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q does not mention 'not found'", err.Error())
	}
}

func TestRegistry_FindByLayer(t *testing.T) {
	reg, err := LoadFromFile(filepath.Join("testdata", "seed.json"))
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	l2 := reg.FindByLayer(LayerL2ForeignBehavior)
	if len(l2) != 1 {
		t.Errorf("FindByLayer(L2) count = %d, want 1", len(l2))
	}
	if l2[0].ID != "sox-test" {
		t.Errorf("L2[0].ID = %q, want sox-test", l2[0].ID)
	}
	l5 := reg.FindByLayer(LayerL5Geopolitics)
	if len(l5) != 1 {
		t.Errorf("FindByLayer(L5) count = %d, want 1", len(l5))
	}
	if l5[0].ID != "taiwan-strait-test" {
		t.Errorf("L5[0].ID = %q, want taiwan-strait-test", l5[0].ID)
	}
}

func TestRegistry_Layers_AllFivePresent(t *testing.T) {
	// Extend testdata to cover all 5 layers by building a custom registry
	custom := []byte(`[
		{"id":"a","name":"a","layer":"L1","summary":"x","direction":"up","risk":"medium","source":"backtest","status":"active","attribution_mode":"rule_based","conditions":[{"field":"f","operator":"gt","value":1,"string_value":"","timeframe":"1D","source":"s"}]},
		{"id":"b","name":"b","layer":"L2","summary":"x","direction":"up","risk":"medium","source":"backtest","status":"active","attribution_mode":"rule_based","conditions":[{"field":"f","operator":"gt","value":1,"string_value":"","timeframe":"1D","source":"s"}]},
		{"id":"c","name":"c","layer":"L3","summary":"x","direction":"up","risk":"medium","source":"backtest","status":"active","attribution_mode":"rule_based","conditions":[{"field":"f","operator":"gt","value":1,"string_value":"","timeframe":"1D","source":"s"}]},
		{"id":"d","name":"d","layer":"L4","summary":"x","direction":"up","risk":"medium","source":"backtest","status":"active","attribution_mode":"rule_based","conditions":[{"field":"f","operator":"gt","value":1,"string_value":"","timeframe":"1D","source":"s"}]},
		{"id":"e","name":"e","layer":"L5","summary":"x","direction":"up","risk":"medium","source":"backtest","status":"active","attribution_mode":"rule_based","conditions":[{"field":"f","operator":"gt","value":1,"string_value":"","timeframe":"1D","source":"s"}]}
	]`)
	reg, err := LoadFromBytes(custom)
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	layers := reg.Layers()
	if len(layers) != 5 {
		t.Errorf("Layers() count = %d, want 5", len(layers))
	}
	want := []Layer{
		LayerL1GlobalLiquidity, LayerL2ForeignBehavior,
		LayerL3IndustryCatalysts, LayerL4FXAndChips,
		LayerL5Geopolitics,
	}
	for i, l := range want {
		if layers[i] != l {
			t.Errorf("Layers()[%d] = %q, want %q", i, layers[i], l)
		}
	}
}

func TestRegistry_NilSafety(t *testing.T) {
	var r *Registry
	if got := r.Count(); got != 0 {
		t.Errorf("nil.Count() = %d, want 0", got)
	}
	if all := r.All(); all != nil {
		t.Errorf("nil.All() = %v, want nil", all)
	}
	if l2 := r.FindByLayer(LayerL2ForeignBehavior); l2 != nil {
		t.Errorf("nil.FindByLayer = %v, want nil", l2)
	}
	if _, err := r.FindByID("x"); err == nil {
		t.Error("nil.FindByID: err = nil, want non-nil")
	}
}

// TestProductionSeeds_Loads is a smoke test that the production JSON
// (data/seeds/strategy_techniques.json) loads cleanly with 9 seeds.
// Skipped automatically if the file is absent (e.g. running from a
// different working directory).
func TestProductionSeeds_Loads(t *testing.T) {
	candidates := []string{
		filepath.Join("..", "..", "data", "seeds", "strategy_techniques.json"),
		filepath.Join("data", "seeds", "strategy_techniques.json"),
	}
	var path string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			path = c
			break
		}
	}
	if path == "" {
		t.Skip("production seeds file not found; skipping")
	}
	reg, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile(%q): %v", path, err)
	}
	if got := reg.Count(); got != 12 {
		t.Errorf("production seed count = %d, want 12", got)
	}
}

// TestProductionSeeds_CBFxThreshold asserts the cb-fx-intervention-warning
// strategy's USD_TWD level threshold is 32.3 (decision 2026-08-19: 32.5→32.3,
// data/seeds/strategy_techniques.json) and that the second daily-rise
// condition (0.5) is preserved untouched.
func TestProductionSeeds_CBFxThreshold(t *testing.T) {
	candidates := []string{
		filepath.Join("..", "..", "data", "seeds", "strategy_techniques.json"),
		filepath.Join("data", "seeds", "strategy_techniques.json"),
	}
	var path string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			path = c
			break
		}
	}
	if path == "" {
		t.Skip("production seeds file not found; skipping")
	}
	reg, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile(%q): %v", path, err)
	}
	f, err := reg.FindByID("cb-fx-intervention-warning")
	if err != nil {
		t.Fatalf("FindByID(cb-fx-intervention-warning): %v", err)
	}
	var usdCond *Condition
	for i := range f.Conditions {
		if f.Conditions[i].Field == "USD_TWD" {
			usdCond = &f.Conditions[i]
			break
		}
	}
	if usdCond == nil {
		t.Fatal("cb-fx strategy has no USD_TWD condition")
	}
	for _, c := range f.Conditions {
		if c.Field != "USD_TWD" {
			continue
		}
		switch {
		case c.Value == 32.3:
			// level threshold — decision updated 32.5 → 32.3
		case c.Value == 0.5:
			// daily-rise threshold — intentionally untouched
		default:
			t.Errorf("unexpected USD_TWD condition value %v", c.Value)
		}
	}
}
