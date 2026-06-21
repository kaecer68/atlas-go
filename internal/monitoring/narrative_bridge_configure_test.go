package monitoring

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestNarrativeEventBridge_Configure(t *testing.T) {
	cfg := config.SmartUniverseConfig{
		ConfidenceThreshold: config.ParameterMetadata[int]{Value: 7},
	}
	b := NewNarrativeEventBridgeWithFetcher("", nil)
	b.Configure(cfg)
	if got := b.ComputeConfidence(7); got != 1.0 {
		t.Errorf("hitCount=7, threshold=7, expected saturate to 1.0, got %v", got)
	}
	if got := b.ComputeConfidence(3); got < 0.4 || got > 0.5 {
		t.Errorf("hitCount=3, threshold=7, expected ~0.43, got %v", got)
	}
}
