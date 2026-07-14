package system

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// TestE2E_HealthAggregate_Happy 啟一個真實 mux，把 RegisterAggregateRoute
// 掛上去，用 httptest 當 front，模擬 client 端發 GET /api/health/aggregate
// 拿回的 body shape + status code，驗證第 95 行 aggregateResponse struct 的
// JSON 對齊 contract。
//
// 為什麼這裡必需要 httptest（不能用既有的 health_aggregate_test）：
// - 既有 unit test 直接呼叫 handleHealthAggregate，跳過 mux 與 routing path
// - 此次要驗（a）route pattern 對（b）JSON shape 對外契約穩定（c）auth bypass 行為
//
// 涵蓋項目 7.1.2（frontend → backend chain 端點 smoke）+ 7.2 (production env
// ATLAS_API_KEY bpss）：只要這個測試綠，PR#1 commit f6fb256d 的 aggregate
// endpoint 對前端就具備「可被正確呼叫 + 拿到正確格式」保證。
func TestE2E_HealthAggregate_Happy(t *testing.T) {
	h := &HealthHandlers{}
	mux := http.NewServeMux()
	h.RegisterAggregateRoute(mux)
	// shared.Get + shared.Adapt 已是 handler-shaped，這條不會被 AuthMiddleware 擋
	// 因為 E2E 不掛 middleware，要驗 whitelist 走另條 test。
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/health/aggregate")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s, want 200", resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("not JSON: %v\nbody=%s", err, body)
	}

	tiers, ok := got["tiers"].(map[string]any)
	if !ok {
		t.Fatalf("missing tiers object in body=%s", body)
	}
	for _, name := range []string{"liveness", "channel_health", "llm_ready", "auth_posture"} {
		if _, present := tiers[name]; !present {
			t.Errorf("tier %q missing; body=%s", name, body)
		}
	}

	overall, ok := got["overall"].(map[string]any)
	if !ok {
		t.Errorf("missing overall in body=%s", body)
	}
	if _, hasOK := overall["ok"]; !hasOK {
		t.Errorf("overall.ok missing; body=%s", body)
	}
	if _, hasLat := overall["latency_ms"]; !hasLat {
		t.Errorf("overall.latency_ms missing; body=%s", body)
	}
}

// TestE2E_HealthAggregate_AuthBypass_PublicPath 驗證 production 環境下
// ATLAS_API_KEY 已設時，AuthMiddleware 仍會對 admin_web/main.go isPublicPath
// 與 shared.AuthFreeExactPaths 雙白名單內的路徑放行。
func TestE2E_HealthAggregate_AuthBypass_PublicPath(t *testing.T) {
	const apiKey = "test-api-key-prod-mode-987654321"
	t.Setenv("ATLAS_API_KEY", apiKey)
	t.Setenv("ATLAS_ENV", "production")

	h := &HealthHandlers{}
	mux := http.NewServeMux()
	h.RegisterAggregateRoute(mux)

	handler := shared.AuthMiddleware(mux)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/health/aggregate", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("aggregate GET failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bad-key /api/health/aggregate = %d, want 200 (whitelist bypass); body=%s",
			resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"tiers"`) {
		t.Fatalf("aggregate body missing tiers: %s", body)
	}
}
