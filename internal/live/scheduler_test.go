package live

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	recommendation "github.com/kaecer68/atlas-go/internal/domain/recommendation"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// mockProvider implements marketdata.Provider for Scheduler tests.
type mockProvider struct {
	name   string
	quotes []domain.Quote
	err    error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.quotes, nil
}

// mockSchedulerSystem implements the system interface for Scheduler.
type mockSchedulerSystem struct {
	registry domain.AgentRegistry
	plugins  any
}

func (m *mockSchedulerSystem) Registry() domain.AgentRegistry { return m.registry }

func (m *mockSchedulerSystem) GetPlugins() any { return m.plugins }

func TestScheduler_NewScheduler(t *testing.T) {
	ctx := context.Background()
	provider := &mockProvider{name: "test"}
	st := livestore.NewStateStore(t.TempDir())
	cb := NewCircuitBreaker("", "")
	cfg := OrchestratorConfig{
		MarketOpenTime:    "09:00",
		MarketCloseTime:   "13:30",
		IntradayInterval:  time.Minute,
		QuotePollInterval: 5 * time.Second,
	}

	s := NewScheduler(ctx, provider, st, cb, cfg, "paper")
	if s == nil {
		t.Fatal("expected non-nil Scheduler")
	}
	if s.marketData != provider {
		t.Error("marketData not set")
	}
	if s.effectiveBrokerMode != "paper" {
		t.Errorf("expected effectiveBrokerMode paper, got %s", s.effectiveBrokerMode)
	}
	if s.config.MarketOpenTime != "09:00" {
		t.Errorf("expected MarketOpenTime 09:00, got %s", s.config.MarketOpenTime)
	}
	if s.ctx == nil {
		t.Error("expected non-nil context")
	}
}

func TestScheduler_SetSystem(t *testing.T) {
	s := &Scheduler{}
	sys := &mockSchedulerSystem{}
	s.SetSystem(sys)
	if s.system != sys {
		t.Error("system not set")
	}
}

func TestScheduler_SetWatchlist(t *testing.T) {
	s := &Scheduler{}
	symbols := []string{"2330.TW", "0050.TW"}
	s.SetWatchlist(symbols)
	if len(s.watchlist) != 2 {
		t.Fatalf("expected 2 watchlist symbols, got %d", len(s.watchlist))
	}
	if s.watchlist[0] != "2330.TW" {
		t.Errorf("expected 2330.TW, got %s", s.watchlist[0])
	}
}

func TestScheduler_SetMetrics(t *testing.T) {
	s := &Scheduler{}
	m := &noopMetrics{}
	s.SetMetrics(m)
	if s.metrics != m {
		t.Error("metrics not set")
	}
}

func TestScheduler_SetEventBus(t *testing.T) {
	s := &Scheduler{}
	eb := NewChannelEventBus(8)
	defer eb.Close()
	s.SetEventBus(eb)
	if s.eventBus != eb {
		t.Error("eventBus not set")
	}
}

func TestScheduler_SetCycleCallbacks(t *testing.T) {
	s := &Scheduler{}
	openCalled := false
	closeCalled := false
	s.SetCycleCallbacks(
		func() { openCalled = true },
		func() {}, // intraday
		func() { closeCalled = true },
		func() {}, // fetch quotes
	)
	if s.onMarketOpen == nil {
		t.Error("onMarketOpen not set")
	}
	if s.onMarketClose == nil {
		t.Error("onMarketClose not set")
	}
	s.onMarketOpen()
	if !openCalled {
		t.Error("onMarketOpen callback not invoked")
	}
	s.onMarketClose()
	if !closeCalled {
		t.Error("onMarketClose callback not invoked")
	}
}

func TestScheduler_StartAndStop(t *testing.T) {
	ctx := context.Background()
	provider := &mockProvider{name: "test"}
	st := livestore.NewStateStore(t.TempDir())
	cb := NewCircuitBreaker("", "")
	cfg := OrchestratorConfig{
		MarketOpenTime:    "09:00",
		MarketCloseTime:   "13:30",
		IntradayInterval:  time.Minute,
		QuotePollInterval: 5 * time.Second,
	}

	s := NewScheduler(ctx, provider, st, cb, cfg, "paper")

	err := s.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Start should have set up quote ticker
	if s.quoteTicker == nil {
		t.Error("expected quoteTicker to be set after Start")
	}

	err = s.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestScheduler_Status(t *testing.T) {
	ctx := context.Background()
	provider := &mockProvider{name: "test"}
	st := livestore.NewStateStore(t.TempDir())
	cb := NewCircuitBreaker("", "")
	cfg := OrchestratorConfig{
		MarketOpenTime:    "09:00",
		MarketCloseTime:   "13:30",
		IntradayInterval:  time.Minute,
		QuotePollInterval: 5 * time.Second,
	}

	s := NewScheduler(ctx, provider, st, cb, cfg, "paper")

	status := s.Status()
	if status == nil {
		t.Fatal("expected non-nil status map")
	}
	if status["quote_ticker"] != false {
		t.Error("expected quote_ticker=false before Start")
	}
	if status["intraday_ticker"] != false {
		t.Error("expected intraday_ticker=false before Start")
	}
	if status["effective_broker"] != "paper" {
		t.Errorf("expected effective_broker paper, got %v", status["effective_broker"])
	}
	if wl := status["watchlist_size"]; wl.(int) != 0 {
		t.Errorf("expected watchlist_size 0, got %v", wl)
	}

	s.SetWatchlist([]string{"2330.TW", "0050.TW", "2317.TW"})
	status = s.Status()
	if wl := status["watchlist_size"]; wl.(int) != 3 {
		t.Errorf("expected watchlist_size 3, got %v", wl)
	}
}

func TestScheduler_Status_AfterStart(t *testing.T) {
	ctx := context.Background()
	provider := &mockProvider{name: "test"}
	st := livestore.NewStateStore(t.TempDir())
	cb := NewCircuitBreaker("", "")
	cfg := OrchestratorConfig{
		MarketOpenTime:    "09:00",
		MarketCloseTime:   "13:30",
		IntradayInterval:  time.Minute,
		QuotePollInterval: 5 * time.Second,
	}

	s := NewScheduler(ctx, provider, st, cb, cfg, "live")

	err := s.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Stop()

	status := s.Status()
	if status["quote_ticker"] != true {
		t.Error("expected quote_ticker=true after Start")
	}
}

func TestScheduler_Stop_BeforeStart(t *testing.T) {
	ctx := context.Background()
	provider := &mockProvider{name: "test"}
	st := livestore.NewStateStore(t.TempDir())
	cb := NewCircuitBreaker("", "")
	cfg := OrchestratorConfig{
		MarketOpenTime:    "09:00",
		MarketCloseTime:   "13:30",
		IntradayInterval:  time.Minute,
		QuotePollInterval: 5 * time.Second,
	}

	s := NewScheduler(ctx, provider, st, cb, cfg, "paper")

	// Stop before Start should not panic
	err := s.Stop()
	if err != nil {
		t.Fatalf("Stop before Start failed: %v", err)
	}
}

func TestScheduler_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &mockProvider{name: "test"}
	st := livestore.NewStateStore(t.TempDir())
	cb := NewCircuitBreaker("", "")
	cfg := OrchestratorConfig{
		MarketOpenTime:    "09:00",
		MarketCloseTime:   "13:30",
		IntradayInterval:  time.Minute,
		QuotePollInterval: 5 * time.Second,
	}

	s := NewScheduler(ctx, provider, st, cb, cfg, "paper")

	// Cancel before Start
	cancel()

	err := s.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = s.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestScheduler_NilDependencies(t *testing.T) {
	ctx := context.Background()
	cfg := OrchestratorConfig{
		MarketOpenTime:    "09:00",
		MarketCloseTime:   "13:30",
		IntradayInterval:  time.Minute,
		QuotePollInterval: 5 * time.Second,
	}

	// Test with nil provider, nil stateStore, nil circuitBreaker
	s := NewScheduler(ctx, nil, nil, nil, cfg, "dry-run")
	if s == nil {
		t.Fatal("expected non-nil Scheduler even with nil dependencies")
	}

	// Status should work with nil deps
	status := s.Status()
	if status == nil {
		t.Fatal("expected non-nil status with nil deps")
	}
}

func TestScheduler_WithSystemIntegration(t *testing.T) {
	ctx := context.Background()
	provider := &mockProvider{name: "test"}
	st := livestore.NewStateStore(t.TempDir())
	cb := NewCircuitBreaker("", "")
	cfg := OrchestratorConfig{
		MarketOpenTime:    "09:00",
		MarketCloseTime:   "13:30",
		IntradayInterval:  time.Minute,
		QuotePollInterval: 5 * time.Second,
	}

	s := NewScheduler(ctx, provider, st, cb, cfg, "paper")

	sys := &mockSchedulerSystem{
		registry: domain.AgentRegistry{
			Agents: []recommendation.AgentSpec{
				{ID: "test-1", Layer: shared.LayerContext, Skill: "context", Enabled: true},
			},
		},
		plugins: "mock-plugins",
	}
	s.SetSystem(sys)

	if s.system != sys {
		t.Error("system not set correctly")
	}

	// Verify getters work via the interface
	s.mu.RLock()
	reg := s.system.Registry()
	s.mu.RUnlock()
	if reg.Version != 0 || len(reg.Agents) != 1 {
		t.Errorf("unexpected registry: %+v", reg)
	}

	s.mu.RLock()
	plugins := s.system.GetPlugins()
	s.mu.RUnlock()
	if plugins != "mock-plugins" {
		t.Errorf("unexpected plugins: %v", plugins)
	}
}

func TestDefaultOrchestratorConfig(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	if cfg.MarketOpenTime != "09:00" {
		t.Errorf("expected MarketOpenTime 09:00, got %s", cfg.MarketOpenTime)
	}
	if cfg.MarketCloseTime != "13:30" {
		t.Errorf("expected MarketCloseTime 13:30, got %s", cfg.MarketCloseTime)
	}
	if cfg.BrokerMode != "dry-run" {
		t.Errorf("expected BrokerMode dry-run, got %s", cfg.BrokerMode)
	}
}

// Ensure mockProvider satisfies marketdata.Provider interface
var _ marketdata.Provider = (*mockProvider)(nil)

func TestScheduler_OffHours_ForceOff_NoTicker(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatalf("failed to load Asia/Taipei location: %v", err)
	}
	fakeNow := time.Date(2026, 6, 20, 23, 0, 0, 0, loc) // 23:00 Taipei = off-hours

	ctx := context.Background()
	provider := &mockProvider{name: "test"}
	st := livestore.NewStateStore(t.TempDir())
	cb := NewCircuitBreaker("", "")
	cfg := OrchestratorConfig{
		MarketOpenTime:      "09:00",
		MarketCloseTime:     "13:30",
		IntradayInterval:    time.Minute,
		QuotePollInterval:   5 * time.Second,
		ForceIntradayCycles: false,
	}

	s := NewScheduler(ctx, provider, st, cb, cfg, "paper")
	s.nowFunc = func() time.Time { return fakeNow }
	s.checkInterval = 10 * time.Millisecond

	s.SetCycleCallbacks(
		func() {}, // onMarketOpen — should NOT be called
		func() {}, // onIntradayCycle
		func() {}, // onMarketClose
		func() {}, // onFetchQuotes
	)

	err = s.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Stop()

	time.Sleep(100 * time.Millisecond)

	status := s.Status()
	ticker, ok := status["intraday_ticker"]
	if !ok {
		t.Fatal("status missing intraday_ticker key")
	}
	if ticker != false {
		t.Errorf("expected intraday_ticker=false for off-hours with ForceIntradayCycles=false, got %v", ticker)
	}
}

func TestScheduler_OffHours_ForceOn_TickerStarts(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatalf("failed to load Asia/Taipei location: %v", err)
	}
	fakeNow := time.Date(2026, 6, 20, 23, 0, 0, 0, loc) // 23:00 Taipei = off-hours

	ctx := context.Background()
	provider := &mockProvider{name: "test"}
	st := livestore.NewStateStore(t.TempDir())
	cb := NewCircuitBreaker("", "")
	cfg := OrchestratorConfig{
		MarketOpenTime:      "09:00",
		MarketCloseTime:     "13:30",
		IntradayInterval:    time.Minute,
		QuotePollInterval:   5 * time.Second,
		ForceIntradayCycles: true,
	}

	s := NewScheduler(ctx, provider, st, cb, cfg, "paper")
	s.nowFunc = func() time.Time { return fakeNow }
	s.checkInterval = 10 * time.Millisecond

	openCalled := false
	s.SetCycleCallbacks(
		func() { openCalled = true }, // onMarketOpen — SHOULD be called
		func() {},                    // onIntradayCycle
		func() {},                    // onMarketClose
		func() {},                    // onFetchQuotes
	)

	err = s.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Stop()

	time.Sleep(100 * time.Millisecond)

	status := s.Status()
	ticker, ok := status["intraday_ticker"]
	if !ok {
		t.Fatal("status missing intraday_ticker key")
	}
	if ticker != true {
		t.Errorf("expected intraday_ticker=true for off-hours with ForceIntradayCycles=true, got %v", ticker)
	}
	if !openCalled {
		t.Error("expected onMarketOpen callback to be invoked when force-starting intraday cycles")
	}
}

func TestScheduler_OnHours_ForceOn_DoesNotAutoStop(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatalf("failed to load Asia/Taipei location: %v", err)
	}
	fakeNow := time.Date(2026, 6, 20, 10, 0, 0, 0, loc) // 10:00 Taipei = on-hours
	var mu sync.Mutex

	ctx := context.Background()
	provider := &mockProvider{name: "test"}
	st := livestore.NewStateStore(t.TempDir())
	cb := NewCircuitBreaker("", "")
	cfg := OrchestratorConfig{
		MarketOpenTime:      "09:00",
		MarketCloseTime:     "13:30",
		IntradayInterval:    time.Minute,
		QuotePollInterval:   5 * time.Second,
		ForceIntradayCycles: true,
	}

	s := NewScheduler(ctx, provider, st, cb, cfg, "paper")
	s.nowFunc = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return fakeNow
	}
	s.checkInterval = 10 * time.Millisecond

	closeCalled := false
	s.SetCycleCallbacks(
		func() {}, // onMarketOpen
		func() {}, // onIntradayCycle
		func() { closeCalled = true }, // onMarketClose — should NOT be called
		func() {}, // onFetchQuotes
	)

	err = s.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Stop()

	// Wait for first iteration: ticker should start (10:00 is on-hours)
	time.Sleep(100 * time.Millisecond)

	status := s.Status()
	ticker, ok := status["intraday_ticker"]
	if !ok {
		t.Fatal("status missing intraday_ticker key")
	}
	if ticker != true {
		t.Fatalf("expected intraday_ticker=true at 10:00 (on-hours), got %v", ticker)
	}

	// Advance time past 13:30 close
	mu.Lock()
	fakeNow = time.Date(2026, 6, 20, 14, 0, 0, 0, loc) // 14:00 Taipei = past close
	mu.Unlock()

	// Wait for next scheduler loop iteration
	time.Sleep(200 * time.Millisecond)

	status = s.Status()
	ticker, ok = status["intraday_ticker"]
	if !ok {
		t.Fatal("status missing intraday_ticker key after time advance")
	}
	if ticker != true {
		t.Errorf("expected intraday_ticker=true past close with ForceIntradayCycles=true (should NOT auto-stop), got %v", ticker)
	}
	if closeCalled {
		t.Error("expected onMarketClose NOT to be called when ForceIntradayCycles=true prevents auto-stop")
	}
}
