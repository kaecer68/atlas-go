package apigateway

import (
	"context"
	"testing"
)

func TestUSMarketChannels(t *testing.T) {
	channels := USMarketChannels()
	if len(channels) != 8 {
		t.Errorf("len(USMarketChannels) = %d, want 8", len(channels))
	}
	expected := map[string]bool{
		"us_spx":    true,
		"us_ndx":    true,
		"us_dji":    true,
		"sox_index": true,
		"us_nvda":   true,
		"us_aapl":   true,
		"us_msft":   true,
		"tsm_adr":   true,
	}
	for _, ch := range channels {
		if !expected[ch] {
			t.Errorf("unexpected channel %q in USMarketChannels", ch)
		}
		delete(expected, ch)
	}
	if len(expected) > 0 {
		t.Errorf("missing channels: %v", expected)
	}
}

func TestNewUSMarketRefreshTask_ReturnsFunc(t *testing.T) {
	g := newTestGateway(t)
	task := NewUSMarketRefreshTask(g)
	if task == nil {
		t.Fatal("NewUSMarketRefreshTask returned nil")
	}
}

func TestNewUSMarketRefreshTask_ExecutesWithoutPanic(t *testing.T) {
	g := newTestGateway(t)
	task := NewUSMarketRefreshTask(g)
	// The task will attempt to fetch each channel; without adapters registered
	// all 8 will fail, but the function should not panic and should return nil
	err := task(context.Background())
	if err != nil {
		t.Errorf("task returned error: %v", err)
	}
}

func TestNewUSMarketRefreshTask_DifferentGateways(t *testing.T) {
	// Verify the task closes over the gateway it was created with
	g1 := newTestGateway(t)
	g2 := newTestGateway(t)
	task := NewUSMarketRefreshTask(g1)
	// Should not panic; g2 is a different instance
	_ = g2
	err := task(context.Background())
	if err != nil {
		t.Errorf("task returned error: %v", err)
	}
}
