package live

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type stubProvider struct {
	name   string
	quotes []domain.Quote
}

func (p stubProvider) Name() string { return p.name }

func (p stubProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	return p.quotes, nil
}

func TestResolveBrokerModeLiveFallsBackToDryRun(t *testing.T) {
	requested, effective, broker, audit := resolveBrokerMode("live")
	if requested != "live" {
		t.Fatalf("requested mode mismatch: got=%q want=live", requested)
	}
	if effective != "dry-run" {
		t.Fatalf("effective mode mismatch: got=%q want=dry-run", effective)
	}
	if broker == nil || broker.Mode() != "dry-run" {
		t.Fatalf("expected dry-run broker, got %+v", broker)
	}
	if audit == "" {
		t.Fatalf("expected non-empty audit message for live fallback")
	}
}

func TestResolveBrokerModeUnknownFallsBackToDryRun(t *testing.T) {
	requested, effective, broker, audit := resolveBrokerMode("experimental")
	if requested != "experimental" {
		t.Fatalf("requested mode mismatch: got=%q want=experimental", requested)
	}
	if effective != "dry-run" {
		t.Fatalf("effective mode mismatch: got=%q want=dry-run", effective)
	}
	if broker == nil || broker.Mode() != "dry-run" {
		t.Fatalf("expected dry-run broker, got %+v", broker)
	}
	if audit == "" {
		t.Fatalf("expected non-empty audit message for unknown mode fallback")
	}
}

func TestNewOrchestratorAppliesLiveFailSafeMode(t *testing.T) {
	store := NewStateStore(t.TempDir())
	bus := NewChannelEventBus(32)
	t.Cleanup(func() {
		_ = bus.Close()
	})

	o := NewOrchestrator(
		store,
		bus,
		stubProvider{name: "stub-market", quotes: []domain.Quote{}},
		domain.AgentRegistry{},
		nil,
		OrchestratorConfig{
			MarketOpenTime:    "09:00",
			MarketCloseTime:   "13:30",
			IntradayInterval:  time.Hour,
			QuotePollInterval: 10 * time.Second,
			BrokerMode:        "live",
			BrokerMaxRetries:  2,
		},
	)

	if o.requestedBrokerMode != "live" {
		t.Fatalf("requested broker mode: got=%q want=live", o.requestedBrokerMode)
	}
	if o.effectiveBrokerMode != "dry-run" {
		t.Fatalf("effective broker mode: got=%q want=dry-run", o.effectiveBrokerMode)
	}
	if o.executionAuditMsg == "" {
		t.Fatalf("expected non-empty audit message when live mode is requested")
	}

	status := o.Status()
	configMap, ok := status["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected config status type: %T", status["config"])
	}
	if configMap["broker_mode_requested"] != "live" {
		t.Fatalf("status requested mode mismatch: %v", configMap["broker_mode_requested"])
	}
	if configMap["broker_mode_effective"] != "dry-run" {
		t.Fatalf("status effective mode mismatch: %v", configMap["broker_mode_effective"])
	}
	if configMap["broker_max_retries"] != 2 {
		t.Fatalf("status retries mismatch: %v", configMap["broker_max_retries"])
	}
}
