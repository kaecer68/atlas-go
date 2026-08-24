package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// TestFubonClient_DefaultURL_UsesFubonProxy (P1-S1) verifies that
// NewFubonClient() uses "http://fubon-proxy:18081" as the default proxy URL
// (Docker compose service name, resolved via Docker DNS).
// 本機開發時 register_adapters.go 會自動切換至 127.0.0.1。
func TestFubonClient_DefaultURL_UsesFubonProxy(t *testing.T) {
	client := NewFubonClient()

	want := "http://fubon-proxy:18081"
	if client.proxyURL != want {
		t.Errorf("NewFubonClient().proxyURL = %q, want %q\n\n"+
			"原因：fubon-proxy 已容器化，透過 Docker DNS service name fubon-proxy 連線，"+
			"不須再依賴 host.docker.internal。",
			client.proxyURL, want)
	}
}

// TestFubonClient_RejectsFUBONProxyURL (P1-S3, PR #572 regression guard)
// verifies that setting the FUBON_PROXY_URL environment variable does NOT
// override the default proxy URL. PR #572 explicitly removed
// os.Getenv("FUBON_PROXY_URL") from fubon_client.go; the client must always
// use fubonproxy.ProxyBaseURL() regardless of environment.
func TestFubonClient_RejectsFUBONProxyURL(t *testing.T) {
	t.Setenv("FUBON_PROXY_URL", "http://evil-override.example.com:9999")
	ResetSharedFubonClient()

	client := NewFubonClient()

	want := "http://fubon-proxy:18081"
	if client.proxyURL != want {
		t.Errorf("NewFubonClient().proxyURL = %q after setting FUBON_PROXY_URL, want %q\n\n"+
			"PR #572 已移除 FUBON_PROXY_URL 環境變數覆寫。FubonClient 必須永遠使用"+
			"fubonproxy.ProxyBaseURL() 回傳值。",
			client.proxyURL, want)
	}

	if v := os.Getenv("FUBON_PROXY_URL"); v != "" && v != "http://evil-override.example.com:9999" {
		t.Logf("FUBON_PROXY_URL env value: %q", v)
	}
}

// ─── P2-17: request-failure breaker ────────────────────────────────────────

// TestFubonClient_BreakerOpensOnRequestFailures verifies that actual request
// failures (not just health-probe misses) accumulate in the provider breaker
// and flip IsHealthy() to false once the threshold is crossed. Before P2-17,
// healthy stayed true while the proxy was down, so every quote request still
// hammered the dead proxy.
func TestFubonClient_BreakerOpensOnRequestFailures(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		http.Error(w, "proxy down", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewFubonClient()
	client.proxyURL = server.URL
	client.SetHTTPClient(server.Client())
	client.SetHealthClient(server.Client())

	// Feed consecutive request failures until the breaker opens (threshold 3).
	var lastErr error
	for i := 0; i < 5; i++ {
		_, lastErr = client.GetQuote(context.Background(), "2330")
	}
	if lastErr == nil {
		t.Fatal("expected error from GetQuote")
	}
	info := client.BreakerInfo()
	if info.State != ProviderCircuitOpen {
		t.Fatalf("breaker state = %s, want open (threshold %d, failures %d)", info.State, info.Threshold, info.FailureCount)
	}
	if client.IsHealthy() {
		t.Error("IsHealthy() = true after breaker opened, want false (request failures must flip the health flag)")
	}

	// Once open, subsequent calls fail fast WITHOUT touching the network.
	mu.Lock()
	before := calls
	mu.Unlock()
	_, err := client.GetQuote(context.Background(), "2330")
	mu.Lock()
	after := calls
	mu.Unlock()
	if after != before {
		t.Errorf("open breaker still hit the network: calls before=%d after=%d", before, after)
	}
	if !strings.Contains(err.Error(), "circuit breaker open") {
		t.Errorf("err = %v, want circuit breaker open", err)
	}
}

// TestFubonClient_BreakerRecoversViaProbe verifies 成功即復位 through the
// health probe: once the request-failure breaker opens, a successful probe
// (proxy back) resets it to closed WITHOUT waiting for the 5-minute recovery
// timeout, and the next quote request goes through again.
func TestFubonClient_BreakerRecoversViaProbe(t *testing.T) {
	var mu sync.Mutex
	failuresLeft := 3
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if failuresLeft > 0 {
			failuresLeft--
			http.Error(w, "proxy down", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"symbol":"2330","name":"台積電","last":1000,"open":990,"high":1005,"low":985,
			"volume":1000,"reference_price":990,"previous_close":990,"change":10,"change_percent":1.01,
			"bids":[],"asks":[],"is_open":true,"is_close":false,"timestamp":0,"source":"fubon"}`))
	}))
	defer server.Close()

	client := NewFubonClient()
	client.proxyURL = server.URL
	client.SetHTTPClient(server.Client())
	client.SetHealthClient(server.Client())

	// 3 failures open the breaker.
	for i := 0; i < 3; i++ {
		_, _ = client.GetQuote(context.Background(), "2330")
	}
	if got := client.BreakerInfo().State; got != ProviderCircuitOpen {
		t.Fatalf("breaker state after 3 failures = %s, want open", got)
	}

	// While open, GetQuote fails fast without touching the proxy.
	if _, err := client.GetQuote(context.Background(), "2330"); err == nil {
		t.Fatal("expected fast-fail error while breaker open")
	}

	// Probe success (proxy back) resets the breaker immediately (成功即復位).
	if err := client.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck after recovery: %v", err)
	}
	if got := client.BreakerInfo().State; got != ProviderCircuitClosed {
		t.Fatalf("breaker state after probe success = %s, want closed", got)
	}
	if !client.IsHealthy() {
		t.Error("IsHealthy() = false after probe success, want true")
	}

	// Next quote request goes through and succeeds.
	q, err := client.GetQuote(context.Background(), "2330")
	if err != nil {
		t.Fatalf("GetQuote after recovery: %v", err)
	}
	if q.Last != 1000 {
		t.Errorf("Last = %v, want 1000", q.Last)
	}
}

// TestFubonClient_HealthProbeFeedsBreaker verifies the health probe also
// feeds the request-failure breaker: a dead proxy opens the breaker via the
// probe path (no quote request in flight), and a recovered proxy closes it.
func TestFubonClient_HealthProbeFeedsBreaker(t *testing.T) {
	var mu sync.Mutex
	failuresLeft := 3
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if failuresLeft > 0 {
			failuresLeft--
			http.Error(w, "proxy down", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","is_open":true,"timestamp":0}`))
	}))
	defer server.Close()

	client := NewFubonClient()
	client.proxyURL = server.URL
	client.SetHTTPClient(server.Client())
	client.SetHealthClient(server.Client())

	// 3 probe failures → breaker open + healthy false.
	for i := 0; i < 3; i++ {
		_ = client.HealthCheck(context.Background())
	}
	if got := client.BreakerInfo().State; got != ProviderCircuitOpen {
		t.Fatalf("breaker state after 3 probe failures = %s, want open", got)
	}
	if client.IsHealthy() {
		t.Error("IsHealthy() = true after 3 probe failures, want false")
	}

	// Recovered probe → breaker closed again.
	if err := client.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck after recovery: %v", err)
	}
	if got := client.BreakerInfo().State; got != ProviderCircuitClosed {
		t.Fatalf("breaker state after recovered probe = %s, want closed", got)
	}
}
