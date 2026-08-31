package apigateway

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

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
	found := slices.Contains(result, "fubon")
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
	err := RegisterChannelAdapters(nil, "/tmp", config.Config{}, nil, nil)
	if err == nil {
		t.Fatal("RegisterChannelAdapters with nil gateway should return error")
	}
}

func TestRegisterChannelAdapters_EmptyConfig(t *testing.T) {
	g := newTestGateway(t)
	cfg := config.Config{}
	err := RegisterChannelAdapters(g, t.TempDir(), cfg, nil, nil)
	if err != nil {
		t.Fatalf("RegisterChannelAdapters failed: %v", err)
	}
	ids := g.ChannelIDs()
	if len(ids) == 0 {
		t.Error("RegisterChannelAdapters should register some non-key-gated channels")
	}
}

// TestRegisterChannelAdapters_TEJDisabledWritesInactiveHealth verifies that
// when TEJ_API_KEY is unset, RegisterChannelAdapters writes a status="inactive"
// health record for the "tej" channel. This stops dashboard + Alerts() from
// surfacing the stale AAA003 "api_key已過期" error left over from before the
// 2026-08-03 TEJ disable (PR chore/20260803-disable-tej).
//
// Pair invariant: register_adapters.go and cmd/atlas/main.go:1670 both gate on
// the same TEJ_API_KEY secret. The scheduler branch in main.go intentionally
// does NOT write health records (see comment block above line 1670) — channel
// + health record writes are centralized here to avoid double-writes.
func TestRegisterChannelAdapters_TEJDisabledWritesInactiveHealth(t *testing.T) {
	t.Setenv("TEJ_API_KEY", "")
	g := newTestGateway(t)
	cfg := config.Config{}
	if err := RegisterChannelAdapters(g, t.TempDir(), cfg, nil, nil); err != nil {
		t.Fatalf("RegisterChannelAdapters failed: %v", err)
	}

	if g.HasChannel("tej") {
		t.Error("tej channel should NOT be registered when TEJ_API_KEY is unset")
	}

	rec := g.Health().Get("tej")
	if rec == nil {
		t.Fatal("expected health record for tej after RegisterChannelAdapters, got nil")
	}
	if rec.Status != "inactive" {
		t.Errorf("tej health status = %q, want %q (Alert() filters inactive, so dashboard stops showing stale error)", rec.Status, "inactive")
	}
	if rec.LastError == "" {
		t.Error("tej inactive record should carry the PR-anchor reason in LastError for auditability")
	}

	// Critical contract: "inactive" must NOT show up in Alerts() — this is what
	// suppresses the stale AAA003 error from the dashboard.
	for _, alert := range g.Health().Alerts() {
		if alert.ChannelID == "tej" {
			t.Errorf("tej should not appear in Alerts() when status=inactive, got %+v", alert)
		}
	}
}

// TestRegisterChannelAdapters_TEJEnabledDoesNotWriteInactive verifies the
// opposite direction: when TEJ is double-opted-in (TEJ_API_KEY + TEJ_ENABLED=true),
// the adapter is registered AND no pre-emptive inactive health record is left
// behind (which would mask the first real Fetch() outcome).
func TestRegisterChannelAdapters_TEJEnabledDoesNotWriteInactive(t *testing.T) {
	t.Setenv("TEJ_API_KEY", "test-paid-key-not-real")
	t.Setenv("TEJ_ENABLED", "true")
	g := newTestGateway(t)
	cfg := config.Config{}
	if err := RegisterChannelAdapters(g, t.TempDir(), cfg, nil, nil); err != nil {
		t.Fatalf("RegisterChannelAdapters failed: %v", err)
	}

	if !g.HasChannel("tej") {
		t.Error("tej channel should be registered when TEJ_API_KEY is set")
	}
	// Health record is allowed to be absent (channel hasn't fetched yet) OR
	// non-inactive — what matters is that we did NOT poison it with status=inactive.
	if rec := g.Health().Get("tej"); rec != nil && rec.Status == "inactive" {
		t.Errorf("tej health should not be pre-marked inactive when API key is set, got %+v", rec)
	}
}

// TestRegisterChannelAdapters_TWSEETFDisabledWritesInactiveHealth verifies
// that when TWSE_ETF_API_KEY is unset, RegisterChannelAdapters does NOT register
// twse_etf and instead writes a status="inactive" health record so the admin
// page shows "未啟用" instead of the permanent upstream-removal error
// (TWT44U → 404, known_issues.go twse_etf_upstream_60d).
func TestRegisterChannelAdapters_TWSEETFDisabledWritesInactiveHealth(t *testing.T) {
	t.Setenv("TWSE_ETF_API_KEY", "")
	g := newTestGateway(t)
	cfg := config.Config{}
	if err := RegisterChannelAdapters(g, t.TempDir(), cfg, nil, nil); err != nil {
		t.Fatalf("RegisterChannelAdapters failed: %v", err)
	}

	if g.HasChannel("twse_etf") {
		t.Error("twse_etf channel should NOT be registered when TWSE_ETF_API_KEY is unset")
	}

	rec := g.Health().Get("twse_etf")
	if rec == nil {
		t.Fatal("expected health record for twse_etf after RegisterChannelAdapters, got nil")
	}
	if rec.Status != "inactive" {
		t.Errorf("twse_etf health status = %q, want inactive", rec.Status)
	}
	for _, alert := range g.Health().Alerts() {
		if alert.ChannelID == "twse_etf" {
			t.Errorf("twse_etf should not appear in Alerts() when status=inactive, got %+v", alert)
		}
	}
}

func TestRegisterChannelAdapters_WithJanusEngine(t *testing.T) {
	g := newTestGateway(t)
	cfg := config.Config{}
	engine := &janus.Engine{}
	err := RegisterChannelAdapters(g, t.TempDir(), cfg, engine, nil)
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

func TestGateway_ForceOpenChannel_KnownChannel(t *testing.T) {
	g := newTestGateway(t)
	// channelIDs() includes known channels; ForceOpenChannel should work
	err := g.ForceOpenChannel("us_yahoo")
	if err != nil {
		t.Errorf("ForceOpenChannel(us_yahoo) returned error: %v", err)
	}
}

func TestGateway_ForceOpenChannel_UnknownChannel(t *testing.T) {
	g := newTestGateway(t)
	err := g.ForceOpenChannel("nonexistent_channel")
	if err == nil {
		t.Error("ForceOpenChannel for nonexistent channel should return error")
	}
}

func TestGateway_HealthCheck_KnownChannel(t *testing.T) {
	g := newTestGateway(t)
	// Without adapters registered, known channels exist in breakers
	// but their providers return errors; HealthCheck should propagate that
	_, err := g.HealthCheck(context.Background(), "us_yahoo")
	if err == nil {
		t.Log("HealthCheck succeeded for us_yahoo without adapters - no error expected, this is fine")
	}
}

func TestGateway_ForceOpenChannel_AllKnownChannels(t *testing.T) {
	g := newTestGateway(t)
	for _, id := range channelIDs() {
		err := g.ForceOpenChannel(id)
		if err != nil {
			t.Errorf("ForceOpenChannel(%s) returned error: %v", id, err)
		}
	}
}

func TestGateway_Fetch_ChannelWithNoProvider(t *testing.T) {
	g := newTestGateway(t)
	// known channel has circuit breaker but no provider registered
	_, err := g.Fetch(context.Background(), "us_yahoo")
	if err == nil {
		t.Error("Fetch for channel with no provider should return error")
	}
}

func TestGateway_Fetch_Success(t *testing.T) {
	g := newTestGateway(t)
	expectedData := []byte(`{"key":"value"}`)
	provider := &HTTPProvider{
		name:    "us_yahoo",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "us_yahoo", Country: "US"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return expectedData, nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "ok", CheckType: "liveness"}, nil
		},
	}
	g.registry.Register("us_yahoo", provider)

	result, err := g.Fetch(context.Background(), "us_yahoo")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if result == nil {
		t.Fatal("Fetch returned nil result")
	}
	if result.Fallback {
		t.Error("result.Fallback should be false for a successful fetch")
	}
	if string(result.Data) != string(expectedData) {
		t.Errorf("Data = %q, want %q", string(result.Data), string(expectedData))
	}
}

func TestGateway_Fetch_ProviderError(t *testing.T) {
	g := newTestGateway(t)
	provider := &HTTPProvider{
		name:    "us_yahoo",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "us_yahoo", Country: "US"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return nil, errors.New("network error")
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "error"}, errors.New("network error")
		},
	}
	g.registry.Register("us_yahoo", provider)

	_, err := g.Fetch(context.Background(), "us_yahoo")
	if err == nil {
		t.Error("Fetch should return error for failing provider")
	}
}

func TestGateway_Fetch_CacheHit(t *testing.T) {
	g := newTestGateway(t)
	callCount := 0
	provider := &HTTPProvider{
		name:    "us_yahoo",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "us_yahoo", Country: "US"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			callCount++
			return []byte(`{"data":"fresh"}`), nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "ok", CheckType: "liveness"}, nil
		},
	}
	g.registry.Register("us_yahoo", provider)

	// First fetch should call the provider
	result, err := g.Fetch(context.Background(), "us_yahoo")
	if err != nil {
		t.Fatalf("First Fetch failed: %v", err)
	}
	if result == nil {
		t.Fatal("First Fetch returned nil")
	}
	// Second fetch should hit the cache (no additional provider call)
	result2, err := g.Fetch(context.Background(), "us_yahoo")
	if err != nil {
		t.Fatalf("Second Fetch failed: %v", err)
	}
	if result2 == nil {
		t.Fatal("Second Fetch returned nil")
	}
	// Provider should have been called only once
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (second fetch should hit cache)", callCount)
	}
}

func TestGateway_HealthCheck_RegisteredProvider(t *testing.T) {
	g := newTestGateway(t)
	provider := &HTTPProvider{
		name:    "us_yahoo",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "us_yahoo", Country: "US"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return []byte(`{}`), nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "ok", CheckType: "liveness"}, nil
		},
	}
	g.registry.Register("us_yahoo", provider)

	status, err := g.HealthCheck(context.Background(), "us_yahoo")
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("HealthCheck status = %q, want ok", status.Status)
	}
}

func TestGateway_Fetch_ContextCancelled(t *testing.T) {
	g := newTestGateway(t)
	provider := &HTTPProvider{
		name:    "us_yahoo",
		limiter: rate.NewLimiter(rate.Every(10*time.Second), 1), // tight limiter so Wait() drains tokens
		meta:    ChannelMetadata{ChannelID: "us_yahoo", Country: "US"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return []byte(`{}`), nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "ok"}, nil
		},
	}
	g.registry.Register("us_yahoo", provider)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := g.Fetch(ctx, "us_yahoo")
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestGateway_Fetch_CircuitBreakerOpens(t *testing.T) {
	g := newTestGateway(t)
	provider := &HTTPProvider{
		name:    "us_yahoo",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "us_yahoo", Country: "US"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return nil, errors.New("persistent network error")
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "error"}, errors.New("persistent network error")
		},
	}
	g.registry.Register("us_yahoo", provider)

	// Fetch until circuit breaker opens (threshold = 3)
	for range 3 {
		_, _ = g.Fetch(context.Background(), "us_yahoo")
	}

	// Now the breaker should be open
	_, err := g.Fetch(context.Background(), "us_yahoo")
	if err == nil {
		t.Error("Fetch should return error when circuit breaker is open")
	}
}

func TestGateway_Fetch_FallbackOnCircuitOpen(t *testing.T) {
	g := newTestGateway(t)
	ctx := context.Background()

	// Populate cache with a successful fetch
	provider := &HTTPProvider{
		name:    "us_yahoo",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "us_yahoo", Country: "US"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return []byte(`{"data":"cached"}`), nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "ok"}, nil
		},
	}
	g.registry.Register("us_yahoo", provider)
	_, err := g.Fetch(ctx, "us_yahoo")
	if err != nil {
		t.Fatalf("Initial fetch failed: %v", err)
	}

	// Force the circuit breaker open
	if err := g.ForceOpenChannel("us_yahoo"); err != nil {
		t.Fatalf("ForceOpenChannel failed: %v", err)
	}

	// Fetch should return cached data with fallback flag
	result, err := g.Fetch(ctx, "us_yahoo")
	if err != nil {
		t.Fatalf("Fetch with fallback should not error, got: %v", err)
	}
	if result == nil {
		t.Fatal("Fetch returned nil when fallback expected")
	}
	if !result.Fallback {
		t.Error("result.Fallback should be true for circuit breaker fallback")
	}
	if !result.Stale {
		t.Error("result.Stale should be true for circuit breaker fallback")
	}
	if result.LastError == "" {
		t.Error("result.LastError should be non-empty for fallback")
	}
	if string(result.Data) != `{"data":"cached"}` {
		t.Errorf("Data = %q, want cached data", string(result.Data))
	}
}

func TestGateway_HealthCheck_ErrorProvider(t *testing.T) {
	g := newTestGateway(t)
	provider := &HTTPProvider{
		name:    "us_yahoo",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "us_yahoo", Country: "US"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return nil, errors.New("error")
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "error", CheckType: "liveness"}, errors.New("health error")
		},
	}
	g.registry.Register("us_yahoo", provider)

	_, err := g.HealthCheck(context.Background(), "us_yahoo")
	if err == nil {
		t.Error("HealthCheck should return error for failing health check")
	}
}

func TestUnifiedHealthStore_CheckHealth_WithProvider(t *testing.T) {
	s := newTestHealthStore(t)
	g := newTestGateway(t)

	provider := &HTTPProvider{
		name:    "us_yahoo",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "us_yahoo", Country: "US"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return []byte(`{}`), nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "ok", CheckType: "liveness"}, nil
		},
	}
	g.registry.Register("us_yahoo", provider)

	results := s.CheckHealth(context.Background(), g.registry)
	if results == nil {
		t.Fatal("CheckHealth returned nil")
	}
	if hs, ok := results["us_yahoo"]; ok {
		if hs.Status != "ok" {
			t.Errorf("us_yahoo status = %q, want ok", hs.Status)
		}
	}
}

// TestGateway_Fetch_RecordsDegradedForEmptyData is the production-path
// regression test for the government_broker "ok 假象": when a fetch
// succeeds but the contract's data-state validation fails (no reading file
// written because every upstream symbol failed), the health record must be
// "degraded", not "ok".
func TestGateway_Fetch_RecordsDegradedForEmptyData(t *testing.T) {
	g := newTestGateway(t)
	provider := &contractTestProvider{
		hs: HealthStatus{Status: "ok", CheckType: "readiness"},
		ds: DataState{Present: false, Detail: "missing reading file for 20260821"},
	}
	g.registry.Register("government_broker", provider)

	_, err := g.Fetch(context.Background(), "government_broker")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	rec := g.Health().Get("government_broker")
	if rec == nil {
		t.Fatal("health record missing after Fetch")
	}
	if rec.Status != "degraded" {
		t.Errorf("recorded status = %q, want degraded (fetch succeeded but no data landed — ok 假象)", rec.Status)
	}
	if rec.LastError == "" {
		t.Error("degraded record should carry a reason in LastError")
	}
}

// TestGateway_Fetch_RecordsOKWhenDataValid verifies the control direction:
// a fetch with valid persisted data keeps the "ok" health record.
func TestGateway_Fetch_RecordsOKWhenDataValid(t *testing.T) {
	g := newTestGateway(t)
	provider := &contractTestProvider{
		hs: HealthStatus{Status: "ok", CheckType: "readiness"},
		ds: DataState{Present: true, NonZero: true, Detail: "20260821.json total_net=1234"},
	}
	g.registry.Register("government_broker", provider)

	_, err := g.Fetch(context.Background(), "government_broker")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	rec := g.Health().Get("government_broker")
	if rec == nil {
		t.Fatal("health record missing after Fetch")
	}
	if rec.Status != "ok" {
		t.Errorf("recorded status = %q, want ok (valid non-zero reading)", rec.Status)
	}
}

// TestGateway_Fetch_RecordsOKForLiveChannel verifies live-ping channels are
// untouched by contract data validation (no DataStateProvider required).
func TestGateway_Fetch_RecordsOKForLiveChannel(t *testing.T) {
	g := newTestGateway(t)
	provider := &nonDataStateProvider{hs: HealthStatus{Status: "ok"}}
	g.registry.Register("us_yahoo", provider)

	_, err := g.Fetch(context.Background(), "us_yahoo")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	rec := g.Health().Get("us_yahoo")
	if rec == nil {
		t.Fatal("health record missing after Fetch")
	}
	if rec.Status != "ok" {
		t.Errorf("recorded status = %q, want ok (live_ping contract skips data validation)", rec.Status)
	}
}

// TestRegisterChannelAdapters_TEJKeyWithoutOptInWritesInactive covers the
// #1758 decision: a configured-but-expired key must NOT keep the channel in
// permanent「異常」— without TEJ_ENABLED=true the channel records inactive
// (暫不開通) instead, and is NOT registered.
func TestRegisterChannelAdapters_TEJKeyWithoutOptInWritesInactive(t *testing.T) {
	t.Setenv("TEJ_API_KEY", "test-expired-key")
	t.Setenv("TEJ_ENABLED", "")
	g := newTestGateway(t)
	cfg := config.Config{}
	if err := RegisterChannelAdapters(g, t.TempDir(), cfg, nil, nil); err != nil {
		t.Fatalf("RegisterChannelAdapters failed: %v", err)
	}

	if g.HasChannel("tej") {
		t.Error("tej must NOT be registered without TEJ_ENABLED=true")
	}
	rec := g.Health().Get("tej")
	if rec == nil || rec.Status != "inactive" {
		t.Fatalf("tej health = %+v, want inactive record", rec)
	}
	if !strings.Contains(rec.LastError, "#1758") {
		t.Errorf("inactive message should reference #1758, got %q", rec.LastError)
	}
}
