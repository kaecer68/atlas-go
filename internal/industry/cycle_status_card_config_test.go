package industry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

// TestDefaultCardConfig_ConsumesConfigFile pins the wiring issue #1944 Batch 2
// added and Batch A had no test for: industry.composite_card is the config
// authority for the cycle card's tunables, so a config edit must change the card.
// Before the wiring, defaultCardConfig() returned hardcoded copies and this test
// fails on every assertion (LayerWeights, ClampMin/ClampMax).
//
// The shipped composite_card block is deliberately value-identical to the
// hardcoded defaults (the wiring had to be behavior-neutral when it landed), so
// reading the shipped file can never separate "consumed the config" from
// "ignored the config". This test therefore builds its own SSOT: the shipped
// document with a different composite_card, which also keeps the rest of the
// document valid so the config validator accepts it.
func TestDefaultCardConfig_ConsumesConfigFile(t *testing.T) {
	shipped, err := os.ReadFile(shippedParametersPath(t))
	if err != nil {
		t.Fatalf("read shipped parameters: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(shipped, &doc); err != nil {
		t.Fatalf("parse shipped parameters: %v", err)
	}
	industrySection, ok := doc["industry"].(map[string]any)
	if !ok {
		t.Fatal("shipped document has no industry section")
	}
	// Override only the fields under test. sentiment_thresholds is deliberately
	// left out: an absent field must keep the hardcoded default (merge
	// convention), which is the second half of the contract.
	industrySection["composite_card"] = map[string]any{
		"value": map[string]any{
			"layer_weights": map[string]any{"silicon": 0.30, "business_cycle": 0.40},
			"clamp_min":     0.70,
			"clamp_max":     1.30,
		},
	}
	mutated, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal mutated parameters: %v", err)
	}
	ssotPath := filepath.Join(t.TempDir(), "parameters.json")
	if err := os.WriteFile(ssotPath, mutated, 0o644); err != nil {
		t.Fatalf("write parameters: %v", err)
	}

	previous := config.GetParametersConfigPath()
	config.SetParametersConfigPath(ssotPath)
	config.ResetParametersConfig()
	t.Cleanup(func() {
		config.SetParametersConfigPath(previous)
		config.ResetParametersConfig()
	})

	// Precondition: the config under test must really be the mutated document.
	// GetParametersConfig falls back to the compiled-in defaults when the file
	// does not validate, which would make this test pass vacuously.
	if got := config.GetParametersConfig().Industry.CompositeCard.Value.ClampMin; got != 0.70 {
		t.Fatalf("precondition: loaded composite_card.clamp_min = %v, want 0.70 (config not loaded, test would be vacuous)", got)
	}

	got := defaultCardConfig()
	if got.ClampMin != 0.70 || got.ClampMax != 1.30 {
		t.Errorf("clamp window = [%v, %v], want the configured [0.7, 1.3]", got.ClampMin, got.ClampMax)
	}
	if len(got.LayerWeights) != 2 || got.LayerWeights["silicon"] != 0.30 || got.LayerWeights["business_cycle"] != 0.40 {
		t.Errorf("layer weights = %v, want the configured {silicon:0.3, business_cycle:0.4}", got.LayerWeights)
	}
	if got.SentimentThresholds["中性"].Min != 0.95 || got.SentimentThresholds["中性"].Max != 1.05 {
		t.Errorf("sentiment thresholds = %+v, want the hardcoded default kept for a field absent from config", got.SentimentThresholds)
	}
}

// shippedParametersPath locates configs/parameters.json by walking up to the
// module root (go.mod), the same way the config package's own tests do.
func shippedParametersPath(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs cwd: %v", err)
	}
	for range 10 {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return filepath.Join(dir, "configs", "parameters.json")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("module root (go.mod) not found")
	return ""
}
