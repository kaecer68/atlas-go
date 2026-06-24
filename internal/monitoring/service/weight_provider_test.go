package service

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/portfolio"
)

func TestFactorWeightEngineWeightProvider_GetWeights(t *testing.T) {
	engine := portfolio.NewFactorWeightEngine()
	provider := NewFactorWeightEngineWeightProvider(engine)

	weights := provider.GetWeights("")
	if weights == nil {
		t.Fatal("expected non-nil weights")
	}

	if _, ok := weights["momentum"]; !ok {
		t.Errorf("expected key 'momentum' in weights")
	}
	if _, ok := weights["value"]; !ok {
		t.Errorf("expected key 'value' in weights")
	}

	// All values should be non-negative and sum to 1.0.
	var total float64
	for _, w := range weights {
		if w < 0 {
			t.Errorf("weight %f is negative", w)
		}
		total += w
	}
	if total <= 0.99 || total >= 1.01 {
		t.Errorf("weights sum = %f, want ~1.0", total)
	}
}

func TestFactorWeightEngineWeightProvider_GetWeights_KeysAreFactorStrings(t *testing.T) {
	engine := portfolio.NewFactorWeightEngine()
	provider := NewFactorWeightEngineWeightProvider(engine)

	weights := provider.GetWeights("")
	if len(weights) == 0 {
		t.Fatal("expected non-empty weights")
	}

	for k, w := range weights {
		if k == "" {
			t.Errorf("unexpected empty weight key")
		}
		if w < 0 {
			t.Errorf("weight for %q is negative: %f", k, w)
		}
	}
}

func TestFactorWeightEngineWeightProvider_NilEngine(t *testing.T) {
	provider := NewFactorWeightEngineWeightProvider(nil)
	if got := provider.GetWeights(""); got != nil {
		t.Errorf("nil engine should return nil weights, got %v", got)
	}
}

func TestFactorWeightEngineWeightProvider_RegimeSwitching(t *testing.T) {
	engine := portfolio.NewFactorWeightEngine()
	provider := NewFactorWeightEngineWeightProvider(engine)

	engine.OnRegimeChange("", "RISK_ON", 0.8)
	bull := provider.GetWeights("")

	engine.OnRegimeChange("RISK_ON", "RISK_OFF", 0.8)
	bear := provider.GetWeights("")

	if bull["momentum"] == bear["momentum"] {
		t.Errorf("expected different momentum weights across regimes, got bull=%f bear=%f", bull["momentum"], bear["momentum"])
	}
}
