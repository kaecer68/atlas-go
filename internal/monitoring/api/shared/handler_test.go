package shared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdapt_Success(t *testing.T) {
	h := Adapt(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"status": "ok"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("body.status = %q, want \"ok\"", body["status"])
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want \"application/json\"", ct)
	}
}

func TestAdapt_Error(t *testing.T) {
	h := Adapt(func(r *http.Request) (int, any) {
		return http.StatusBadRequest, map[string]string{"error": "bad input"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "bad input" {
		t.Errorf("body.error = %q, want \"bad input\"", body["error"])
	}
}

func TestGet_AllowsGet(t *testing.T) {
	h := Get(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"method": "get"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestGet_RejectsPost(t *testing.T) {
	h := Get(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"method": "get"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "method not allowed" {
		t.Errorf("body.error = %q, want \"method not allowed\"", body["error"])
	}
}

func TestPost_AllowsPost(t *testing.T) {
	h := Post(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"method": "post"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestPost_RejectsGet(t *testing.T) {
	h := Post(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"method": "post"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "method not allowed" {
		t.Errorf("body.error = %q, want \"method not allowed\"", body["error"])
	}
}

func TestAdaptRaw_AlreadyWrote(t *testing.T) {
	h := AdaptRaw(func(w http.ResponseWriter, r *http.Request) (int, any) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html>hello</html>"))
		return 0, nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html" {
		t.Errorf("Content-Type = %q, want \"text/html\"", ct)
	}
	if body := w.Body.String(); body != "<html>hello</html>" {
		t.Errorf("body = %q, want \"<html>hello</html>\"", body)
	}
}

func TestAdaptRaw_FallbackJSON(t *testing.T) {
	h := AdaptRaw(func(w http.ResponseWriter, r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"key": "value"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["key"] != "value" {
		t.Errorf("body.key = %q, want \"value\"", body["key"])
	}
}

func TestGetRaw_AllowsGet(t *testing.T) {
	h := GetRaw(func(w http.ResponseWriter, r *http.Request) (int, any) {
		w.Write([]byte("raw"))
		return 0, nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != "raw" {
		t.Errorf("body = %q, want \"raw\"", body)
	}
}

func TestGetRaw_RejectsPost(t *testing.T) {
	h := GetRaw(func(w http.ResponseWriter, r *http.Request) (int, any) {
		w.Write([]byte("raw"))
		return 0, nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestAdapt_NoContent_EmptyBody(t *testing.T) {
	h := Adapt(func(r *http.Request) (int, any) {
		return http.StatusNoContent, nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	body := strings.TrimSpace(w.Body.String())
	if body != "" {
		t.Errorf("body = %q, want empty for 204 No Content", body)
	}
}

func TestAuthMiddleware_ProductionRequiresAPIKey(t *testing.T) {
	t.Setenv("ATLAS_ENV", "production")
	t.Setenv("ATLAS_API_KEY", "")
	h := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run when production key is missing")
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestAuthMiddleware_AcceptsBearerAndRejectsBadKey(t *testing.T) {
	t.Setenv("ATLAS_ENV", "")
	t.Setenv("ATLAS_API_KEY", "secret")
	h := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/test", nil))
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad key status = %d, want %d", bad.Code, http.StatusUnauthorized)
	}

	good := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(good, req)
	if good.Code != http.StatusTeapot {
		t.Fatalf("good key status = %d, want %d", good.Code, http.StatusTeapot)
	}
}

// TestAuthMiddleware_ProbingPathsBypassAuth verifies that /health and
// /metrics are reachable without any auth header even when
// ATLAS_API_KEY is set. This is the contract docker healthcheck and
// Prometheus scrapers rely on. Regression guard for the bypass that
// was previously only at cmd/atlas/main.go finalMux — moving the
// logic into AuthMiddleware itself means every caller (including
// Adapt() wrappers in monitoring handlers) gets the same exemption.
func TestAuthMiddleware_ProbingPathsBypassAuth(t *testing.T) {
	t.Setenv("ATLAS_ENV", "production")
	t.Setenv("ATLAS_API_KEY", "secret-key")

	hits := 0
	h := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/health", "/metrics"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("%s without auth: status = %d, want %d (body=%s)",
				path, w.Code, http.StatusOK, w.Body.String())
		}
	}
	if hits != 2 {
		t.Errorf("next handler invocations = %d, want 2 (one per probe path)", hits)
	}

	// Non-probing paths still require auth
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/control/approve-recommendation", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("protected path without auth: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// TestAuthMiddleware_LLMHealthBypassAuth verifies that /api/llm/health
// is reachable without X-API-Key. The endpoint is consumed by the
// atlas-mcp server (handleLLMGetHealth) and by the health endpoint of
// the LLM router; the MCP client may not always carry the API token,
// and LLM health is treated as a probing path like /health and /metrics.
func TestAuthMiddleware_LLMHealthBypassAuth(t *testing.T) {
	t.Setenv("ATLAS_ENV", "production")
	t.Setenv("ATLAS_API_KEY", "secret-key")

	hits := 0
	h := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/llm/health", nil))
	if w.Code != http.StatusOK {
		t.Errorf("/api/llm/health without auth: status = %d, want %d (body=%s)",
			w.Code, http.StatusOK, w.Body.String())
	}
	if hits != 1 {
		t.Errorf("next handler invocations = %d, want 1", hits)
	}
}

// TestAuthMiddleware_WebUIPathsBypassAuth verifies that /admin, /client,
// and their sub-paths (served via http.StripPrefix on /admin/, /client/)
// are reachable without any auth header. The browser loads the HTML and
// static JS bundles first; only subsequent /api/* calls carry X-API-Key.
// Without these exemptions the login page itself would 401 before any JS
// runs, breaking the web UI entirely.
func TestAuthMiddleware_WebUIPathsBypassAuth(t *testing.T) {
	t.Setenv("ATLAS_ENV", "production")
	t.Setenv("ATLAS_API_KEY", "secret-key")

	hits := 0
	h := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))

	// Exact + prefix paths the web UI uses for HTML/static assets.
	// Each must bypass auth — otherwise the login page itself 401s before JS runs.
	paths := []string{
		"/admin",
		"/admin/",
		"/admin/index.html",
		"/admin/some/nested/asset.js",
		"/client",
		"/client/",
		"/client/dashboard.html",
	}
	for _, path := range paths {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("%s without auth: status = %d, want %d (body=%s)",
				path, w.Code, http.StatusOK, w.Body.String())
		}
	}
	if hits != len(paths) {
		t.Errorf("next handler invocations = %d, want %d", hits, len(paths))
	}

	for _, path := range []string{"/api/control/approve-recommendation", "/api/experiment/judge", "/adminfoo", "/adminx"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("protected path %s without auth: status = %d, want %d",
				path, w.Code, http.StatusUnauthorized)
		}
	}
}

// TestAuthMiddleware_DashboardAPIsBypassAuth verifies that the dashboard
// read-only API prefixes (/api/dashboard, /api/taiwan, /api/narrative,
// /api/macro, /api/alerts, /api/synergy) bypass auth without X-API-Key.
//
// The investor /client/ pages do not carry the API token in browser
// fetches, and these endpoints expose only public-read summary data
// (regime, channel health, narrative bundles, macro snapshot, etc.).
// Without these exemptions the dashboard's <main> panel renders empty
// because every panel fetch returns 401. Parallels PR #931, which added
// /api/llm/health with the same reasoning.
//
// Regression guard: AGENTS.md §關鍵跨模組陷阱 requires these bypass
// lists to be mirrored in BOTH cmd/atlas/main.go isPublicPath AND
// internal/monitoring/api/shared/handler.go authFree{Exact,Prefix}Paths.
// Forgetting either side leaves the dashboard 401.
func TestAuthMiddleware_DashboardAPIsBypassAuth(t *testing.T) {
	t.Setenv("ATLAS_ENV", "production")
	t.Setenv("ATLAS_API_KEY", "secret-key")

	hits := 0
	h := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))

	prefixPaths := []string{
		"/api/dashboard/system-health",
		"/api/dashboard/macro-radar",
		"/api/dashboard/agent-observatory",
		"/api/taiwan/stress-index",
		"/api/narrative/bundle",
		"/api/macro/snapshot/latest",
		"/api/alerts/unacknowledged",
		"/api/synergy/darwinian/status",
		"/api/strategies/active",
		"/api/risk/metrics",
		"/api/regime/history",
		"/api/scheduler/status",
		"/api/tasks",
		"/api/traces/sim-latest",
		"/api/llm/cost",
		"/api/llm_annotator/cost",
		"/api/prism/training-results",
		"/api/recommendations",
		"/api/reports/latest",
		"/api/strategy-ranker/rank",
	}
	exactPaths := []string{
		"/api/alerts",
		"/api/field-contract",
		"/api/control/audit-log",
		"/api/control/active-overrides",
		"/api/experiment/history",
		"/api/experiment/diff",
	}

	for _, path := range prefixPaths {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("prefix %s without auth: status = %d, want %d (body=%s)",
				path, w.Code, http.StatusOK, w.Body.String())
		}
	}
	for _, path := range exactPaths {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("exact %s without auth: status = %d, want %d (body=%s)",
				path, w.Code, http.StatusOK, w.Body.String())
		}
	}

	wantHits := len(prefixPaths) + len(exactPaths)
	if hits != wantHits {
		t.Errorf("next handler invocations = %d, want %d", hits, wantHits)
	}

	// Write operations and other sensitive paths still require auth.
	for _, path := range []string{
		"/api/control/approve-recommendation",
		"/api/control/reject-recommendation",
		"/api/control/pause-agent",
		"/api/control/resume-agent",
		"/api/control/sector-ban",
		"/api/experiment/judge",
		"/api/experiment/promote",
		"/api/experiment/revert",
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("protected path %s without auth: status = %d, want %d",
				path, w.Code, http.StatusUnauthorized)
		}
	}
}

func TestRequireAdmin(t *testing.T) {
	t.Setenv("ATLAS_ADMIN_KEY", "admin-secret")
	h := RequireAdmin(func(r *http.Request) (int, any) {
		return http.StatusAccepted, map[string]string{"ok": "true"}
	})

	status, body := h(httptest.NewRequest(http.MethodPost, "/admin", nil))
	if status != http.StatusUnauthorized {
		t.Fatalf("missing admin status = %d, want 401 (body=%v)", status, body)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin", nil)
	req.Header.Set("Authorization", "Admin admin-secret")
	status, _ = h(req)
	if status != http.StatusAccepted {
		t.Fatalf("authorized status = %d, want 202", status)
	}
}

func TestAdminPost_RequiresPostAndAdmin(t *testing.T) {
	t.Setenv("ATLAS_ADMIN_KEY", "admin-secret")
	h := AdminPost(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"status": "ok"}
	})

	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", get.Code)
	}

	unauthorized := httptest.NewRecorder()
	h.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/admin", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorized.Code)
	}

	authorized := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin", nil)
	req.Header.Set("X-Admin-Key", "admin-secret")
	h.ServeHTTP(authorized, req)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want 200", authorized.Code)
	}
}

// TestIsAuthFreePath_ParametersNoSlash is a regression guard for the SK-22
// endpoint-2 audit: /api/parameters (no trailing slash) must be auth-free.
// The authFreePrefixPaths entry "/api/parameters/" does not match the exact
// path via HasPrefix, so the exact path must live in authFreeExactPaths.
func TestIsAuthFreePath_ParametersNoSlash(t *testing.T) {
	if !isAuthFreePath("/api/parameters") {
		t.Fatal("isAuthFreePath(/api/parameters) = false, want true (exact path)")
	}
	if !isAuthFreePath("/api/parameters/metadata") {
		t.Fatal("isAuthFreePath(/api/parameters/metadata) = false, want true (prefix)")
	}
	// Guard against over-broadening: exact-path check must not match a
	// different top-level endpoint.
	if isAuthFreePath("/api/parametersx") {
		t.Fatal("isAuthFreePath(/api/parametersx) = true, want false")
	}
}
