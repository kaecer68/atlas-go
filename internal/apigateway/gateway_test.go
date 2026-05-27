package apigateway

import (
	"context"
	"testing"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/janus"
)

func newTestGateway(t *testing.T) *Gateway {
	t.Helper()
	g, err := NewGateway(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewGateway failed: %v", err)
	}
	return g
}

func TestGateway_ChannelIDs(t *testing.T) {
	g := newTestGateway(t)
	ids := g.ChannelIDs()
	// ChannelIDs returns the static list of all known channel IDs
	if len(ids) == 0 {
		t.Errorf("ChannelIDs() should return non-empty list, got %v", ids)
	}
}

func TestGateway_HasChannel(t *testing.T) {
	g := newTestGateway(t)
	if g.HasChannel("nonexistent") {
		t.Error("HasChannel(nonexistent) should be false for new Gateway")
	}
}

func TestGateway_channelIDs(t *testing.T) {
	result := channelIDs()
	found := false
	for _, id := range result {
		if id == "fubon" {
			found = true
			break
		}
	}
	if !found {
		t.Error("channelIDs() should include fubon")
	}
}

func TestGateway_Fetch_NoChannel(t *testing.T) {
	g := newTestGateway(t)
	_, err := g.Fetch(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Fetch for nonexistent channel should return error")
	}
}

func TestGateway_HealthCheck_NoGateway(t *testing.T) {
	g := newTestGateway(t)
	_, err := g.HealthCheck(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("HealthCheck for nonexistent channel should return error")
	}
}

func TestGateway_RateLimitStatus(t *testing.T) {
	g := newTestGateway(t)
	status := g.RateLimitStatus()
	if status == nil {
		t.Fatal("RateLimitStatus returned nil")
	}
}

func TestGateway_BreakerStatus(t *testing.T) {
	g := newTestGateway(t)
	status := g.BreakerStatus()
	if status == nil {
		t.Fatal("BreakerStatus returned nil")
	}
}

func TestGateway_Health(t *testing.T) {
	g := newTestGateway(t)
	store := g.Health()
	if store == nil {
		t.Fatal("Health() returned nil")
	}
}

func TestGateway_Summary_NoChannels(t *testing.T) {
	g := newTestGateway(t)
	summary := g.Summary()
	if summary == nil {
		t.Fatal("Summary() returned nil")
	}
}

func TestRegisterChannelAdapters_NilGateway(t *testing.T) {
	err := RegisterChannelAdapters(nil, "/tmp", config.Config{}, nil)
	if err == nil {
		t.Fatal("RegisterChannelAdapters with nil gateway should return error")
	}
}

func TestRegisterChannelAdapters_EmptyConfig(t *testing.T) {
	g := newTestGateway(t)
	cfg := config.Config{}
	err := RegisterChannelAdapters(g, t.TempDir(), cfg, nil)
	if err != nil {
		t.Fatalf("RegisterChannelAdapters failed: %v", err)
	}
	ids := g.ChannelIDs()
	if len(ids) == 0 {
		t.Error("RegisterChannelAdapters should register some non-key-gated channels")
	}
}

func TestRegisterChannelAdapters_WithJanusEngine(t *testing.T) {
	g := newTestGateway(t)
	cfg := config.Config{}
	engine := &janus.Engine{}
	err := RegisterChannelAdapters(g, t.TempDir(), cfg, engine)
	if err != nil {
		t.Fatalf("RegisterChannelAdapters failed: %v", err)
	}
	if !g.HasChannel("janus_regime") {
		t.Error("janus_regime channel should be registered when engine is provided")
	}
}

func TestRateLimitManager_Get_Nonexistent(t *testing.T) {
	m := NewRateLimitManager()
	limiter, err := m.Get("nonexistent")
	if err == nil {
		t.Error("Get for nonexistent should return error")
	}
	if limiter != nil {
		t.Error("Get for nonexistent should return nil limiter")
	}
}

func TestRateLimitManager_Remaining_Nonexistent(t *testing.T) {
	m := NewRateLimitManager()
	remaining, err := m.Remaining("nonexistent")
	if err == nil {
		t.Error("Remaining for nonexistent should return error")
	}
	if remaining != 0 {
		t.Errorf("Remaining = %f, want 0", remaining)
	}
}

func TestRateLimitManager_Register(t *testing.T) {
	m := NewRateLimitManager()
	limiter := rate.NewLimiter(rate.Inf, 0)
	m.Register("test_channel", limiter)

	got, err := m.Get("test_channel")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got != limiter {
		t.Error("Get did not return registered limiter")
	}
}

func TestRateLimitManager_Status(t *testing.T) {
	m := NewRateLimitManager()
	status := m.Status()
	if status == nil {
		t.Fatal("Status returned nil")
	}
}
