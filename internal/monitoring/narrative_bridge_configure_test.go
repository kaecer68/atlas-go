package monitoring

import (
	"math"
	"sync"
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
	if diff := math.Abs(b.ComputeConfidence(3) - 3.0/7.0); diff > 1e-9 {
		t.Errorf("hitCount=3, threshold=7, expected 3/7 (%.9f), got %v, diff=%.9f", 3.0/7.0, b.ComputeConfidence(3), diff)
	}
}

// TestNarrativeEventBridge_Configure_Concurrent verifies that concurrent calls
// to Configure and ComputeConfidence do not race, exercising the confidenceMu lock.
func TestNarrativeEventBridge_Configure_Concurrent(t *testing.T) {
	b := NewNarrativeEventBridgeWithFetcher("", nil)
	cfg := config.SmartUniverseConfig{
		ConfidenceThreshold: config.ParameterMetadata[int]{Value: 7},
	}

	var wg sync.WaitGroup
	const goroutines = 20
	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			b.Configure(cfg)
			_ = b.ComputeConfidence(id % 8)
		}(i)
	}
	wg.Wait()

	if got := b.ComputeConfidence(7); got != 1.0 {
		t.Errorf("after concurrent Configure+ComputeConfidence: hitCount=7, threshold=7, expected 1.0, got %v", got)
	}
}
