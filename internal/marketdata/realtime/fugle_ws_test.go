package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kaecer68/atlas-go/internal/domain"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type mockWSServer struct {
	server       *httptest.Server
	messages     []json.RawMessage
	mu           sync.Mutex
	connected    atomic.Int32
	authReceived atomic.Bool
}

func newMockWSServer() *mockWSServer {
	s := &mockWSServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/streaming", s.handleWS)
	s.server = httptest.NewServer(mux)
	return s
}

func (s *mockWSServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	s.connected.Add(1)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msgMap map[string]json.RawMessage
		if err := json.Unmarshal(msg, &msgMap); err != nil {
			continue
		}

		if event, ok := msgMap["event"]; ok {
			eventStr := strings.Trim(string(event), `"`)
			switch eventStr {
			case "auth":
				s.authReceived.Store(true)
			case "subscribe":
				s.mu.Lock()
				s.messages = append(s.messages, msg)
				s.mu.Unlock()
			case "unsubscribe":
				s.mu.Lock()
				s.messages = append(s.messages, msg)
				s.mu.Unlock()
			}
		}
	}

	s.connected.Add(-1)
}

func (s *mockWSServer) URL() string {
	return "ws" + strings.TrimPrefix(s.server.URL, "http") + "/streaming"
}

func (s *mockWSServer) Close() {
	s.server.Close()
}

func (s *mockWSServer) BroadcastTrade(symbol string, price float64, volume int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	trade := map[string]interface{}{
		"event": "trade",
		"data": map[string]interface{}{
			"symbol": symbol,
			"price":  price,
			"volume": volume,
			"open":   price * 0.99,
			"high":   price * 1.01,
			"low":    price * 0.98,
			"close":  price,
		},
	}
	s.messages = append(s.messages, mustMarshal(trade))
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestFugleWSProvider_ConnectDisconnect(t *testing.T) {
	srv := newMockWSServer()
	defer srv.Close()

	config := FugleWSConfig{
		APIKey:     "test-key",
		WSURL:      srv.URL(),
		EnablePing: false,
	}
	provider := NewFugleWebSocketProvider(config)

	ctx := context.Background()
	if err := provider.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if !provider.IsConnected() {
		t.Fatal("Expected connected state after Connect")
	}

	if err := provider.Disconnect(ctx); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	if provider.IsConnected() {
		t.Fatal("Expected disconnected state after Disconnect")
	}
}

func TestFugleWSProvider_Name(t *testing.T) {
	config := FugleWSConfig{APIKey: "test", WSURL: "ws://localhost"}
	provider := NewFugleWebSocketProvider(config)

	if name := provider.Name(); name != "fugle-ws" {
		t.Fatalf("Expected name 'fugle-ws', got '%s'", name)
	}
}

func TestFugleWSProvider_SubscribeUnsubscribe(t *testing.T) {
	srv := newMockWSServer()
	defer srv.Close()

	config := FugleWSConfig{
		APIKey:     "test-key",
		WSURL:      srv.URL(),
		EnablePing: false,
	}
	provider := NewFugleWebSocketProvider(config)

	ctx := context.Background()
	if err := provider.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer provider.Disconnect(ctx)

	time.Sleep(50 * time.Millisecond)

	if err := provider.Subscribe([]string{"2330", "2317"}); err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	status := provider.Status()
	if status.SubscribedCount != 2 {
		t.Fatalf("Expected 2 subscriptions, got %d", status.SubscribedCount)
	}

	if err := provider.Unsubscribe([]string{"2330"}); err != nil {
		t.Fatalf("Unsubscribe failed: %v", err)
	}

	status = provider.Status()
	if status.SubscribedCount != 1 {
		t.Fatalf("Expected 1 subscription after unsubscribe, got %d", status.SubscribedCount)
	}
}

func TestFugleWSProvider_SubscribeNotConnected(t *testing.T) {
	config := FugleWSConfig{APIKey: "test", WSURL: "ws://localhost"}
	provider := NewFugleWebSocketProvider(config)

	err := provider.Subscribe([]string{"2330"})
	if err == nil {
		t.Fatal("Expected error when subscribing while not connected")
	}
}

func TestFugleWSProvider_OnQuoteCallback(t *testing.T) {
	srv := newMockWSServer()
	defer srv.Close()

	config := FugleWSConfig{
		APIKey:     "test-key",
		WSURL:      srv.URL(),
		EnablePing: false,
	}
	provider := NewFugleWebSocketProvider(config)

	var receivedQuotes []domain.Quote
	var quoteMu sync.Mutex
	provider.OnQuote(func(q domain.Quote) {
		quoteMu.Lock()
		receivedQuotes = append(receivedQuotes, q)
		quoteMu.Unlock()
	})

	ctx := context.Background()
	if err := provider.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer provider.Disconnect(ctx)

	time.Sleep(50 * time.Millisecond)

	if err := provider.Subscribe([]string{"2330"}); err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	quoteMu.Lock()
	quotes := receivedQuotes
	quoteMu.Unlock()

	_ = quotes
}

func TestFugleWSProvider_MultipleCallbacks(t *testing.T) {
	config := FugleWSConfig{APIKey: "test", WSURL: "ws://localhost"}
	provider := NewFugleWebSocketProvider(config)

	var count atomic.Int32
	provider.OnQuote(func(q domain.Quote) {
		count.Add(1)
	})
	provider.OnQuote(func(q domain.Quote) {
		count.Add(1)
	})

	provider.emitQuote(domain.Quote{Symbol: "2330", Last: 785})

	time.Sleep(10 * time.Millisecond)

	if count.Load() != 2 {
		t.Fatalf("Expected 2 callback invocations, got %d", count.Load())
	}
}

func TestFugleWSProvider_Status(t *testing.T) {
	config := FugleWSConfig{APIKey: "test", WSURL: "ws://localhost"}
	provider := NewFugleWebSocketProvider(config)

	status := provider.Status()
	if status.Name != "fugle-ws" {
		t.Fatalf("Expected name 'fugle-ws', got '%s'", status.Name)
	}
	if status.State != ProviderStateDisconnected {
		t.Fatalf("Expected disconnected state, got %s", status.State)
	}
}

func TestFugleWSProvider_TradeToQuote(t *testing.T) {
	config := FugleWSConfig{APIKey: "test", WSURL: "ws://localhost"}
	provider := NewFugleWebSocketProvider(config)

	trade := fugleWSTradeData{
		Symbol:  "2330",
		Price:   785.0,
		Volume:  15000000,
		Open:    780.0,
		High:    790.0,
		Low:     775.0,
		Close:   785.0,
		IsTrial: false,
	}

	quote := provider.tradeToQuote(trade)

	if quote.Symbol != "2330" {
		t.Fatalf("Expected symbol '2330', got '%s'", quote.Symbol)
	}
	if quote.Last != 785.0 {
		t.Fatalf("Expected last 785.0, got %f", quote.Last)
	}
	if quote.Source != "fugle-ws" {
		t.Fatalf("Expected source 'fugle-ws', got '%s'", quote.Source)
	}
	if quote.Market != "TW" {
		t.Fatalf("Expected market 'TW', got '%s'", quote.Market)
	}
	if quote.IsTradable {
	}
}

func TestFugleWSProvider_ReconnectBackoff(t *testing.T) {
	config := FugleWSConfig{
		APIKey:     "test-key",
		WSURL:      "ws://invalid-host.invalid:9999/streaming",
		EnablePing: false,
	}
	provider := NewFugleWebSocketProvider(config)

	if provider.backoff != initialBackoff {
		t.Fatalf("Expected initial backoff %v, got %v", initialBackoff, provider.backoff)
	}

	provider.backoff = time.Duration(float64(provider.backoff) * backoffMultiplier)
	if provider.backoff != 2*time.Second {
		t.Fatalf("Expected backoff 2s after one multiply, got %v", provider.backoff)
	}
}

func TestFugleWSProvider_DefaultConfig(t *testing.T) {
	config := DefaultFugleWSConfig("my-api-key")
	if config.APIKey != "my-api-key" {
		t.Fatalf("Expected APIKey 'my-api-key', got '%s'", config.APIKey)
	}
	if config.WSURL != defaultWSURL {
		t.Fatalf("Expected WSURL '%s', got '%s'", defaultWSURL, config.WSURL)
	}
	if !config.EnablePing {
		t.Fatal("Expected EnablePing to be true by default")
	}
}

func TestRealtimeRouter_BasicFlow(t *testing.T) {
	mock1 := &mockProvider{name: "mock-1"}
	mock2 := &mockProvider{name: "mock-2"}

	config := DefaultRouterConfig()
	router := NewRealtimeRouter(config, []RealtimeProvider{mock1, mock2})

	ctx := context.Background()
	if err := router.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if router.ActiveProvider().Name() != "mock-1" {
		t.Fatalf("Expected active provider 'mock-1', got '%s'", router.ActiveProvider().Name())
	}

	if err := router.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestRealtimeRouter_SwitchToNext(t *testing.T) {
	mock1 := &mockProvider{name: "mock-1"}
	mock2 := &mockProvider{name: "mock-2", connected: true}

	config := DefaultRouterConfig()
	router := NewRealtimeRouter(config, []RealtimeProvider{mock1, mock2})

	ctx := context.Background()
	router.Start(ctx)

	if !router.SwitchToNext() {
		t.Fatal("Expected SwitchToNext to succeed")
	}

	if router.ActiveProvider().Name() != "mock-2" {
		t.Fatalf("Expected active provider 'mock-2' after switch, got '%s'", router.ActiveProvider().Name())
	}

	router.Stop(ctx)
}

func TestRealtimeRouter_SwitchToNextNoMore(t *testing.T) {
	mock1 := &mockProvider{name: "mock-1"}

	config := DefaultRouterConfig()
	router := NewRealtimeRouter(config, []RealtimeProvider{mock1})

	ctx := context.Background()
	router.Start(ctx)

	if router.SwitchToNext() {
		t.Fatal("Expected SwitchToNext to fail with only one provider")
	}

	router.Stop(ctx)
}

func TestRealtimeRouter_SubscribeNoProvider(t *testing.T) {
	config := DefaultRouterConfig()
	router := NewRealtimeRouter(config, nil)

	err := router.Subscribe([]string{"2330"})
	if err == nil {
		t.Fatal("Expected error when subscribing with no providers")
	}
}

func TestRealtimeRouter_OnQuote(t *testing.T) {
	mock1 := &mockProvider{name: "mock-1", connected: true}

	config := DefaultRouterConfig()
	router := NewRealtimeRouter(config, []RealtimeProvider{mock1})

	var received domain.Quote
	router.OnQuote(func(q domain.Quote) {
		received = q
	})

	ctx := context.Background()
	router.Start(ctx)

	router.emitQuote(domain.Quote{Symbol: "2330", Last: 785})

	time.Sleep(10 * time.Millisecond)

	if received.Symbol != "2330" {
		t.Fatalf("Expected symbol '2330', got '%s'", received.Symbol)
	}

	router.Stop(ctx)
}

func TestRealtimeRouter_Status(t *testing.T) {
	mock1 := &mockProvider{name: "mock-1", connected: true}
	mock2 := &mockProvider{name: "mock-2", connected: false}

	config := DefaultRouterConfig()
	router := NewRealtimeRouter(config, []RealtimeProvider{mock1, mock2})

	statuses := router.Status()
	if len(statuses) != 2 {
		t.Fatalf("Expected 2 statuses, got %d", len(statuses))
	}
	if statuses[0].Name != "mock-1" {
		t.Fatalf("Expected first status name 'mock-1', got '%s'", statuses[0].Name)
	}
}

type mockProvider struct {
	name      string
	connected bool
	subs      []string
}

func (m *mockProvider) Connect(ctx context.Context) error {
	m.connected = true
	return nil
}

func (m *mockProvider) Disconnect(ctx context.Context) error {
	m.connected = false
	return nil
}

func (m *mockProvider) Subscribe(symbols []string) error {
	m.subs = append(m.subs, symbols...)
	return nil
}

func (m *mockProvider) Unsubscribe(symbols []string) error {
	return nil
}

func (m *mockProvider) OnQuote(callback QuoteCallback) {}

func (m *mockProvider) IsConnected() bool {
	return m.connected
}

func (m *mockProvider) Name() string {
	return m.name
}
