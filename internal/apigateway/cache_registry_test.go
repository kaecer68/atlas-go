package apigateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// mockProvider implements DataProvider for testing.
type mockProvider struct {
	name string
}

func (m *mockProvider) Fetch(ctx context.Context) (*FetchResult, error) {
	return nil, nil
}

func (m *mockProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return HealthStatus{}, nil
}

func (m *mockProvider) RateLimit() *rate.Limiter {
	return nil
}

func (m *mockProvider) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: m.name}
}

// =========================================================================
// CacheLayer Tests
// =========================================================================

func TestNewCacheLayer_EmptyInitially(t *testing.T) {
	c := NewCacheLayer()
	if c == nil {
		t.Fatal("NewCacheLayer() returned nil")
	}
	if len(c.entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(c.entries))
	}
	if c.ttl != 5*time.Minute {
		t.Errorf("expected ttl of 5m, got %v", c.ttl)
	}
}

func TestCacheLayer_Get_NonExistentChannel(t *testing.T) {
	c := NewCacheLayer()
	got := c.Get("nonexistent")
	if got != nil {
		t.Errorf("expected nil for nonexistent channel, got %v", got)
	}
}

func TestCacheLayer_Get_ExpiredEntry(t *testing.T) {
	c := NewCacheLayer()
	result := &FetchResult{Data: []byte("test")}
	c.Set("ch1", result)

	// Manually set cachedAt to 10 minutes ago (TTL is 5 minutes)
	c.mu.Lock()
	c.entries["ch1"].cachedAt = time.Now().Add(-10 * time.Minute)
	c.mu.Unlock()

	got := c.Get("ch1")
	if got != nil {
		t.Errorf("expected nil for expired entry, got %v", got)
	}
}

func TestCacheLayer_Get_ValidNonExpiredEntry(t *testing.T) {
	c := NewCacheLayer()
	expectedData := []byte("valid-data")
	result := &FetchResult{Data: expectedData, Cached: true}
	c.Set("ch1", result)

	got := c.Get("ch1")
	if got == nil {
		t.Fatal("expected non-nil result for valid entry")
	}
	if string(got.Data) != string(expectedData) {
		t.Errorf("expected data %q, got %q", expectedData, got.Data)
	}
	if !got.Cached {
		t.Error("expected Cached to be true")
	}
}

func TestCacheLayer_Set_ThenGet(t *testing.T) {
	c := NewCacheLayer()
	result := &FetchResult{
		Data:  []byte("hello"),
		Stale: true,
	}
	c.Set("ch1", result)

	got := c.Get("ch1")
	if got == nil {
		t.Fatal("expected non-nil result after Set")
	}
	if string(got.Data) != "hello" {
		t.Errorf("expected 'hello', got %q", got.Data)
	}
	if !got.Stale {
		t.Error("expected Stale to be true")
	}
}

func TestCacheLayer_Set_OverwritesExisting(t *testing.T) {
	c := NewCacheLayer()

	first := &FetchResult{Data: []byte("first")}
	c.Set("ch1", first)

	second := &FetchResult{Data: []byte("second")}
	c.Set("ch1", second)

	got := c.Get("ch1")
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if string(got.Data) != "second" {
		t.Errorf("expected 'second' after overwrite, got %q", got.Data)
	}
}

func TestCacheLayer_Invalidate_RemovesChannel(t *testing.T) {
	c := NewCacheLayer()
	c.Set("ch1", &FetchResult{Data: []byte("x")})
	c.Set("ch2", &FetchResult{Data: []byte("y")})

	c.Invalidate("ch1")

	if got := c.Get("ch1"); got != nil {
		t.Errorf("expected nil after Invalidate, got %v", got)
	}
	// ch2 should still be present
	if got := c.Get("ch2"); got == nil {
		t.Error("ch2 should still be present after invalidating ch1")
	}
}

func TestCacheLayer_InvalidateAll_ClearsAll(t *testing.T) {
	c := NewCacheLayer()
	c.Set("ch1", &FetchResult{Data: []byte("a")})
	c.Set("ch2", &FetchResult{Data: []byte("b")})
	c.Set("ch3", &FetchResult{Data: []byte("c")})

	c.InvalidateAll()

	for _, id := range []string{"ch1", "ch2", "ch3"} {
		if got := c.Get(id); got != nil {
			t.Errorf("expected nil after InvalidateAll for %s, got %v", id, got)
		}
	}
	if len(c.entries) != 0 {
		t.Errorf("expected 0 entries after InvalidateAll, got %d", len(c.entries))
	}
}

func TestCacheLayer_ConcurrentAccess(t *testing.T) {
	c := NewCacheLayer()
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for range 100 {
			c.Set("ch", &FetchResult{Data: []byte("w")})
		}
		done <- true
	}()

	// Reader goroutines
	for range 10 {
		go func() {
			for range 100 {
				_ = c.Get("ch")
			}
			done <- true
		}()
	}

	// Wait for writer
	<-done
	// Wait for readers
	for range 10 {
		<-done
	}

	// After concurrent access, the cache should still work
	c.Set("ch", &FetchResult{Data: []byte("final")})
	got := c.Get("ch")
	if got == nil {
		t.Fatal("cache should be functional after concurrent access")
	}
	if string(got.Data) != "final" {
		t.Errorf("expected 'final', got %q", got.Data)
	}
}

// =========================================================================
// ChannelRegistry Tests
// =========================================================================

func TestNewChannelRegistry_CreatesWithGivenManagers(t *testing.T) {
	limiters := NewRateLimitManager()
	breakers := NewCircuitBreakerManager(nil)

	r := NewChannelRegistry(limiters, breakers)

	if r == nil {
		t.Fatal("NewChannelRegistry() returned nil")
	}
	if r.limiters != limiters {
		t.Error("limiters not set correctly")
	}
	if r.breakers != breakers {
		t.Error("breakers not set correctly")
	}
	if len(r.providers) != 0 {
		t.Errorf("expected 0 providers, got %d", len(r.providers))
	}
}

func TestChannelRegistry_Register_ThenGet(t *testing.T) {
	r := NewChannelRegistry(NewRateLimitManager(), NewCircuitBreakerManager(nil))
	p := &mockProvider{name: "test-channel"}

	r.Register("test-channel", p)

	got, err := r.Get("test-channel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected provider, got nil")
	}
	meta := got.Metadata()
	if meta.ChannelID != "test-channel" {
		t.Errorf("expected ChannelID 'test-channel', got %q", meta.ChannelID)
	}
}

func TestChannelRegistry_Register_OverwritesExisting(t *testing.T) {
	r := NewChannelRegistry(NewRateLimitManager(), NewCircuitBreakerManager(nil))

	p1 := &mockProvider{name: "ch1"}
	p2 := &mockProvider{name: "ch1-v2"}

	r.Register("ch1", p1)
	r.Register("ch1", p2)

	got, err := r.Get("ch1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Metadata().ChannelID != "ch1-v2" {
		t.Errorf("expected 'ch1-v2' after overwrite, got %q", got.Metadata().ChannelID)
	}
}

func TestChannelRegistry_Get_UnregisteredChannel(t *testing.T) {
	r := NewChannelRegistry(NewRateLimitManager(), NewCircuitBreakerManager(nil))

	got, err := r.Get("nonexistent")
	if err == nil {
		t.Error("expected error for unregistered channel")
	}
	if got != nil {
		t.Errorf("expected nil provider, got %v", got)
	}
}

func TestChannelRegistry_List_EmptyInitially(t *testing.T) {
	r := NewChannelRegistry(NewRateLimitManager(), NewCircuitBreakerManager(nil))

	ids := r.List()
	if len(ids) != 0 {
		t.Errorf("expected empty list, got %v", ids)
	}
}

func TestChannelRegistry_List_ReturnsAllIDs(t *testing.T) {
	r := NewChannelRegistry(NewRateLimitManager(), NewCircuitBreakerManager(nil))

	r.Register("ch-a", &mockProvider{name: "ch-a"})
	r.Register("ch-b", &mockProvider{name: "ch-b"})
	r.Register("ch-c", &mockProvider{name: "ch-c"})

	ids := r.List()
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d: %v", len(ids), ids)
	}

	found := make(map[string]bool)
	for _, id := range ids {
		found[id] = true
	}
	for _, want := range []string{"ch-a", "ch-b", "ch-c"} {
		if !found[want] {
			t.Errorf("expected %q in list, not found", want)
		}
	}
}

func TestChannelRegistry_ConcurrentAccess(t *testing.T) {
	r := NewChannelRegistry(NewRateLimitManager(), NewCircuitBreakerManager(nil))
	done := make(chan bool)

	// Register providers concurrently
	for i := range 10 {
		go func(idx int) {
			id := "ch-" + string(rune('a'+idx))
			r.Register(id, &mockProvider{name: id})
			done <- true
		}(i)
	}

	// Read concurrently
	for range 10 {
		go func() {
			for range 50 {
				_, _ = r.Get("ch-a")
				_ = r.List()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for range 20 {
		<-done
	}

	// After concurrent access, registry should still be functional
	ids := r.List()
	if len(ids) == 0 {
		t.Fatal("expected some registered providers after concurrent access")
	}
}

// =========================================================================
// FetchResult Fallback & LastError Tests
// =========================================================================

func TestFetchResult_FallbackAndLastError_ZeroDefaults(t *testing.T) {
	r := &FetchResult{
		Data: []byte("test"),
		Meta: FetchMetadata{ChannelID: "test"},
	}
	if r.Fallback {
		t.Error("expected Fallback to default to false")
	}
	if r.LastError != "" {
		t.Errorf("expected LastError to default to empty string, got %q", r.LastError)
	}
	if r.Meta.Fallback {
		t.Error("expected FetchMetadata.Fallback to default to false")
	}
	if r.Meta.LastError != "" {
		t.Errorf("expected FetchMetadata.LastError to default to empty string, got %q", r.Meta.LastError)
	}
}

func TestCacheLayer_Fallback_Roundtrip(t *testing.T) {
	layer := NewCacheLayer()
	layer.Set("ch1", &FetchResult{
		Data:      []byte("stale"),
		Stale:     true,
		Fallback:  true,
		LastError: "circuit breaker open",
		Meta:      FetchMetadata{ChannelID: "ch1", Stale: true, Fallback: true, LastError: "circuit breaker open"},
	})
	got := layer.Get("ch1")
	if !got.Fallback {
		t.Error("expected Fallback to be true after roundtrip")
	}
	if !got.Stale {
		t.Error("expected Stale to be true after roundtrip")
	}
	if got.LastError != "circuit breaker open" {
		t.Errorf("expected LastError to survive, got %q", got.LastError)
	}
	if !got.Meta.Fallback {
		t.Error("expected FetchMetadata.Fallback to be true after roundtrip")
	}
}

func TestFetchResult_JSONMarshal_OmitsEmptyFallbackAndLastError(t *testing.T) {
	r := &FetchResult{
		Data: []byte("test"),
		Meta: FetchMetadata{ChannelID: "test"},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	jsonStr := string(data)
	if strings.Contains(jsonStr, `"fallback"`) {
		t.Errorf("expected fallback to be omitted when false, got: %s", jsonStr)
	}
	if strings.Contains(jsonStr, `"last_error"`) {
		t.Errorf("expected last_error to be omitted when empty, got: %s", jsonStr)
	}

	r2 := &FetchResult{
		Data:      []byte("test"),
		Fallback:  true,
		LastError: "fetch failed",
		Meta:      FetchMetadata{ChannelID: "test", Fallback: true, LastError: "fetch failed"},
	}
	data2, err := json.Marshal(r2)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	jsonStr2 := string(data2)
	if !strings.Contains(jsonStr2, `"fallback":true`) {
		t.Errorf("expected fallback:true in JSON, got: %s", jsonStr2)
	}
	if !strings.Contains(jsonStr2, `"last_error":"fetch failed"`) {
		t.Errorf("expected last_error in JSON, got: %s", jsonStr2)
	}
}

func TestChannelIDs(t *testing.T) {
	ids := channelIDs()
	if len(ids) != 39 {
		t.Fatalf("expected 39 channel IDs, got %d", len(ids))
	}

	expected := []string{
		"us_yahoo", "twse_replay", "twse_capital_flow", "fugle", "fubon",
		"finmind", "frankfurter_fx", "geopolitical", "twse_margin",
		"export_statistics", "tsmc_revenue", "geopolitical_taiwan",
		"janus_regime", "tej", "exchange_rate", "sox_index",
		"dram_spot_price", "twse_sector_index", "sector_data", "day_trading",
		"twse_etf", "taifex_daily", "taifex_institutional", "tdcc_equity_dispersion",
		"twse_oddlot", "twse_sbl", "government_flow", "government_broker", "bdi",
		"us_spx", "us_ndx", "us_dji", "taiex_index", "tw_vol",
		"us_nvda", "us_aapl", "us_msft", "tsm_adr", "twse_insider",
	}
	seen := make(map[string]bool)
	for _, id := range ids {
		seen[id] = true
	}
	for _, want := range expected {
		if !seen[want] {
			t.Errorf("expected channel %q in list", want)
		}
	}
}
