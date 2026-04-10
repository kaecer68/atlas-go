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

func TestResolveBrokerModeLiveUsesGuardedBroker(t *testing.T) {
	requested, effective, broker, audit := resolveBrokerMode(OrchestratorConfig{BrokerMode: "live", BrokerAdapter: "guarded"})
	if requested != "live" {
		t.Fatalf("requested mode mismatch: got=%q want=live", requested)
	}
	if effective != "live-guarded" {
		t.Fatalf("effective mode mismatch: got=%q want=live-guarded", effective)
	}
	if broker == nil || broker.Mode() != "live" {
		t.Fatalf("expected live guarded broker, got %+v", broker)
	}
	if audit == "" {
		t.Fatalf("expected non-empty audit message for live guarded mode")
	}
}

func TestResolveBrokerModeUnknownFallsBackToDryRun(t *testing.T) {
	requested, effective, broker, audit := resolveBrokerMode(OrchestratorConfig{BrokerMode: "experimental", BrokerAdapter: "guarded"})
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

func TestResolveBrokerModeLiveUsesMockAdapter(t *testing.T) {
	requested, effective, broker, audit := resolveBrokerMode(OrchestratorConfig{BrokerMode: "live", BrokerAdapter: "mock"})
	if requested != "live" {
		t.Fatalf("requested mode mismatch: got=%q want=live", requested)
	}
	if effective != "live-mock" {
		t.Fatalf("effective mode mismatch: got=%q want=live-mock", effective)
	}
	if broker == nil || broker.Mode() != "live" {
		t.Fatalf("expected guarded live broker with mock adapter, got %+v", broker)
	}
	if audit == "" {
		t.Fatalf("expected non-empty audit message for live mock mode")
	}
}

func TestResolveBrokerModeLiveHTTPMissingConfigFallsBackToGuarded(t *testing.T) {
	requested, effective, broker, audit := resolveBrokerMode(OrchestratorConfig{BrokerMode: "live", BrokerAdapter: "http"})
	if requested != "live" {
		t.Fatalf("requested mode mismatch: got=%q want=live", requested)
	}
	if effective != "live-guarded" {
		t.Fatalf("effective mode mismatch: got=%q want=live-guarded", effective)
	}
	if broker == nil || broker.Mode() != "live" {
		t.Fatalf("expected guarded live broker fallback, got %+v", broker)
	}
	if audit == "" {
		t.Fatalf("expected non-empty audit message for http fallback")
	}
}

func TestResolveBrokerModeLiveHTTPConfiguredUsesLiveHTTP(t *testing.T) {
	requested, effective, broker, audit := resolveBrokerMode(OrchestratorConfig{
		BrokerMode:           "live",
		BrokerAdapter:        "http",
		BrokerAPIBaseURL:     "https://broker.example",
		BrokerAPIKey:         "key1",
		BrokerHTTPTimeoutS:   5,
		BrokerHTTPAttempts:   2,
		BrokerNonceStore:     "file",
		BrokerNonceStorePath: t.TempDir() + "/nonce-store.json",
	})
	if requested != "live" {
		t.Fatalf("requested mode mismatch: got=%q want=live", requested)
	}
	if effective != "live-http" {
		t.Fatalf("effective mode mismatch: got=%q want=live-http", effective)
	}
	if broker == nil || broker.Mode() != "live" {
		t.Fatalf("expected live broker wrapper, got %+v", broker)
	}
	if audit == "" {
		t.Fatalf("expected non-empty audit message for live-http mode")
	}
}

func TestResolveBrokerModeLiveHTTPInvalidNonceStoreFallsBackToGuarded(t *testing.T) {
	requested, effective, broker, audit := resolveBrokerMode(OrchestratorConfig{
		BrokerMode:       "live",
		BrokerAdapter:    "http",
		BrokerAPIBaseURL: "https://broker.example",
		BrokerAPIKey:     "key1",
		BrokerNonceStore: "invalid",
	})
	if requested != "live" {
		t.Fatalf("requested mode mismatch: got=%q want=live", requested)
	}
	if effective != "live-guarded" {
		t.Fatalf("effective mode mismatch: got=%q want=live-guarded", effective)
	}
	if broker == nil || broker.Mode() != "live" {
		t.Fatalf("expected guarded live broker fallback, got %+v", broker)
	}
	if audit == "" {
		t.Fatalf("expected non-empty audit message for nonce store fallback")
	}
}

func TestNewOrchestratorAppliesLiveGuardedMode(t *testing.T) {
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
			BrokerAdapter:     "guarded",
			BrokerSigner:      "placeholder",
			BrokerKeyID:       "kid-test",
			BrokerNonceStore:  "memory",
			BrokerMaxRetries:  2,
		},
	)

	if o.requestedBrokerMode != "live" {
		t.Fatalf("requested broker mode: got=%q want=live", o.requestedBrokerMode)
	}
	if o.effectiveBrokerMode != "live-guarded" {
		t.Fatalf("effective broker mode: got=%q want=live-guarded", o.effectiveBrokerMode)
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
	if configMap["broker_mode_effective"] != "live-guarded" {
		t.Fatalf("status effective mode mismatch: %v", configMap["broker_mode_effective"])
	}
	if configMap["broker_max_retries"] != 2 {
		t.Fatalf("status retries mismatch: %v", configMap["broker_max_retries"])
	}
	if configMap["broker_adapter"] != "guarded" {
		t.Fatalf("status broker_adapter mismatch: %v", configMap["broker_adapter"])
	}
	if configMap["broker_signer"] != "placeholder" {
		t.Fatalf("status broker_signer mismatch: %v", configMap["broker_signer"])
	}
	if configMap["broker_key_id"] != "kid-test" {
		t.Fatalf("status broker_key_id mismatch: %v", configMap["broker_key_id"])
	}
	if configMap["broker_nonce_store"] != "memory" {
		t.Fatalf("status broker_nonce_store mismatch: %v", configMap["broker_nonce_store"])
	}
}

func TestNewOrchestratorAppliesLiveHTTPFileNonceStoreConfig(t *testing.T) {
	store := NewStateStore(t.TempDir())
	bus := NewChannelEventBus(32)
	t.Cleanup(func() {
		_ = bus.Close()
	})

	nonceStorePath := t.TempDir() + "/nonce-store.json"
	o := NewOrchestrator(
		store,
		bus,
		stubProvider{name: "stub-market", quotes: []domain.Quote{}},
		domain.AgentRegistry{},
		nil,
		OrchestratorConfig{
			MarketOpenTime:       "09:00",
			MarketCloseTime:      "13:30",
			IntradayInterval:     time.Hour,
			QuotePollInterval:    10 * time.Second,
			BrokerMode:           "live",
			BrokerAdapter:        "http",
			BrokerAPIBaseURL:     "https://broker.example",
			BrokerAPIKey:         "key-test",
			BrokerNonceStore:     "file",
			BrokerNonceStorePath: nonceStorePath,
			BrokerSigner:         "placeholder",
		},
	)

	if o.effectiveBrokerMode != "live-http" {
		t.Fatalf("effective broker mode: got=%q want=live-http", o.effectiveBrokerMode)
	}

	status := o.Status()
	configMap, ok := status["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected config status type: %T", status["config"])
	}
	if configMap["broker_nonce_store"] != "file" {
		t.Fatalf("status broker_nonce_store mismatch: %v", configMap["broker_nonce_store"])
	}
	if configMap["broker_nonce_store_path"] != nonceStorePath {
		t.Fatalf("status broker_nonce_store_path mismatch: got=%v want=%s", configMap["broker_nonce_store_path"], nonceStorePath)
	}
}
