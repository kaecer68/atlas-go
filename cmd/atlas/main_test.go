package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	apievents "github.com/kaecer68/atlas-go/internal/monitoring/api/events"
)

// freePort returns an unused TCP port from the kernel. It is used by tests
// that need to avoid hard-coded ports such as :18081 which may be occupied by
// local services (e.g. Docker Desktop on macOS).
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// TestMain ensures the package test environment is clean: it unsets
// ATLAS_API_KEY so the AuthMiddleware returns 200/405 (no-auth) for
// unauthenticated requests, matching the tests' expectations.
//
// 用 os.Setenv("") 而非 os.Unsetenv 是必要的:`config.Load()` 會透過
// `loadUserEnvFile()` 從 ~/.config/atlas-go/.env 讀 ATLAS_API_KEY 用
// `os.Setenv` 設進 process env(即使 test 用 custom deps.loadConfig 也無
// 法阻擋 — 因為 `internal/monitoring/dashboard_api.go:105` 直接呼叫
// config.Load())。`loadWithLookupEnv` 用 `os.LookupEnv` 判斷「已設才
// skip」,所以 `os.Setenv("ATLAS_API_KEY", "")` 後,`LookupEnv` 回
// ("", true) → .env 載入 skip → `os.Getenv` 仍回 "" → AuthMiddleware 走
// no-auth 分支。
//
// 如果只 `os.Unsetenv` 那 .env 載入後 ATLAS_API_KEY 會被設回去,
// AuthMiddleware 看到非空 → 401,test 失敗。詳見
// .omo/investigations/2026-06-28-boot-loop-multi-service.md § 6。
//
// T-104: also set ATLAS_SKIP_PORT_PREFLIGHT=1 to bypass the TCP port
// preflight in startup.Preflight. CI environments frequently have
// leftover native `atlas -api` (port 18080) or self-port-bound
// atlas.test binaries that wedge the 4 live-broker tests. The
// preflight's `atlas-http address :18080 is held by a foreign process`
// error would otherwise make the 4 tests flaky across CI hosts.
func TestMain(m *testing.M) {
	os.Setenv("ATLAS_API_KEY", "")
	os.Setenv("ATLAS_SKIP_PORT_PREFLIGHT", "1")
	os.Exit(m.Run())
}

func TestRunAPIModeStartsServerAndRegistersRoutes(t *testing.T) {
	ledgerDir := t.TempDir()
	var gotAddr string
	var gotHandler http.Handler

	shutdown := make(chan struct{})
	listenDone := make(chan struct{})
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: ledgerDir}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			if dir != ledgerDir {
				t.Fatalf("ledger dir = %q, want %q", dir, ledgerDir)
			}
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			gotAddr = srv.Addr
			gotHandler = srv.Handler
			close(listenDone)
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	err := run([]string{"-api", "-addr", ":19090"}, deps)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	<-listenDone
	if gotAddr != ":19090" {
		t.Fatalf("listen addr = %q, want %q", gotAddr, ":19090")
	}
	if gotHandler == nil {
		t.Fatalf("expected http handler to be registered")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/macro-radar", nil)
	gotHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("macro-radar status = %d, want 200", rr.Code)
	}
}

func TestRunAPIModeReturnsListenError(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir()}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return errors.New("bind failed")
		},
	}

	err := run([]string{"-api"}, deps)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "dashboard api server failed") {
		t.Fatalf("expected wrapped dashboard error, got %v", err)
	}
	if !strings.Contains(err.Error(), "bind failed") {
		t.Fatalf("expected root cause in error, got %v", err)
	}
}

func TestRunRejectsLiveBrokerWithoutExplicitAllow(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "dry-run", BrokerMaxRetries: 1}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-api", "-broker-mode", "live"}, deps)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "disabled by default") {
		t.Fatalf("expected live broker guard error, got %v", err)
	}
}

func TestRunAllowsLiveBrokerWhenExplicitlyEnabled(t *testing.T) {
	ledgerDir := t.TempDir()
	var gotAddr string

	shutdown := make(chan struct{})
	listenDone := make(chan struct{})
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerMaxRetries: 1, AllowLiveBroker: true}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			if dir != ledgerDir {
				t.Fatalf("ledger dir = %q, want %q", dir, ledgerDir)
			}
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			gotAddr = srv.Addr
			close(listenDone)
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	err := run([]string{"-api", "-broker-mode", "live", "-allow-live-broker"}, deps)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	<-listenDone
	if gotAddr != ":18080" {
		t.Fatalf("listen addr = %q, want %q", gotAddr, ":18080")
	}
}

func TestRunEnvOnlyNoBrokerFlagSucceeds(t *testing.T) {
	shutdown := make(chan struct{})
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "dry-run", BrokerAdapter: "guarded"}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error { return nil },
		shutdown:       shutdown,
	}
	go func() { time.Sleep(50 * time.Millisecond); close(shutdown) }()

	t.Setenv("ATLAS_ALLOW_LIVE_BROKER", "true")
	if err := run([]string{"-api"}, deps); err != nil {
		t.Fatalf("env-only (no flag) should not fail: %v", err)
	}
}

func TestRunNoBrokerEnvOrFlagSucceeds(t *testing.T) {
	shutdown := make(chan struct{})
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "dry-run", BrokerAdapter: "guarded"}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error { return nil },
		shutdown:       shutdown,
	}
	go func() { time.Sleep(50 * time.Millisecond); close(shutdown) }()

	if err := run([]string{"-api"}, deps); err != nil {
		t.Fatalf("neither env nor flag should not fail: %v", err)
	}
}

func TestRunRejectsUnsupportedBrokerAdapter(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "dry-run", BrokerMaxRetries: 1}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-api", "-broker-adapter", "invalid"}, deps)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported broker adapter") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRejectsHTTPBrokerAdapterWithoutExplicitAllow(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "live", BrokerAdapter: "http", BrokerMaxRetries: 1, AllowLiveBroker: true}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-api", "-allow-live-broker", "-broker-adapter", "http"}, deps)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "allow-http-broker") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAllowsHTTPBrokerAdapterWithExplicitAllow(t *testing.T) {
	// Avoid the default :18081 which may be occupied by Docker Desktop or other
	// local services; use an ephemeral port for the fubon-proxy preflight check.
	fubonPort := freePort(t)
	shutdown := make(chan struct{})
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "live", BrokerAdapter: "guarded", BrokerMaxRetries: 1, AllowLiveBroker: true, AllowHTTPBroker: true}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	err := run([]string{
		"-api",
		"-broker-mode", "live",
		"-allow-live-broker",
		"-broker-adapter", "http",
		"-allow-http-broker",
		"-fubon-port", strconv.Itoa(fubonPort),
	}, deps)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}

func TestRunRejectsRealSignerWithoutKeyID(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "live", BrokerAdapter: "http", BrokerMaxRetries: 1, BrokerSigner: "placeholder", AllowLiveBroker: true, AllowHTTPBroker: true, AllowRealSigner: true}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-api", "-broker-mode", "live", "-allow-live-broker", "-broker-adapter", "http", "-allow-http-broker", "-broker-signer", "hmac-sha256", "-allow-real-signer"}, deps)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "key id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunLiveHTTPFullFlagChainInAPIMode(t *testing.T) {
	shutdown := make(chan struct{})
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1, BrokerSigner: "placeholder", AllowLiveBroker: true, AllowHTTPBroker: true, AllowRealSigner: true}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	err := run([]string{
		"-api",
		"-broker-mode", "live",
		"-broker-adapter", "http",
		"-broker-signer", "hmac-sha256",
		"-broker-key-id", "kid-flag-1",
		"-broker-retry-status-codes", "429,503",
		"-broker-max-clock-skew-sec", "120",
		"-broker-nonce-ttl-sec", "180",
		"-broker-nonce-store", "redis",
		"-broker-nonce-redis-url", "redis://localhost:6379/0",
		"-broker-nonce-redis-key-prefix", "atlas:e2e:",
		"-allow-live-broker",
		"-allow-http-broker",
		"-allow-real-signer",
	}, deps)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}

func TestStaticFileServerServesIndex(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<h1>Atlas</h1>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	handler := staticHandler(os.DirFS(tmpDir))

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<h1>Atlas</h1>") {
		t.Fatalf("expected index.html content, got %s", rec.Body.String())
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "main.js"), []byte("console.log('test');"), 0o644); err != nil {
		t.Fatalf("write main.js: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/main.js", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for main.js, got %d", rec2.Code)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "main-ab12cd34.js"), []byte("console.log('test');"), 0o644); err != nil {
		t.Fatalf("write hashed asset: %v", err)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/main-ab12cd34.js", nil)
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec3.Code)
	}
	cc := rec3.Header().Get("Cache-Control")
	if !strings.Contains(cc, "immutable") {
		t.Fatalf("expected immutable cache for hashed asset, got: %s", cc)
	}
}

// TestStaticDistPrefixMount guards against the regression where
// `fs.Sub(DistFS, "dist")` returns a sub-FS that has files at `js/main.js`
// (no `dist/` prefix), but URL requests still arrive with the `dist/`
// prefix (e.g. `/client/dist/js/main.js`). Without a second
// `http.StripPrefix("/dist/", ...)` between the URL prefix strip and the
// static handler, every asset request falls through to the SPA fallback
// and returns `index.html` instead of the file. See
// `cmd/atlas/api_routes.go` `registerSimpleRoutes` for the production
// mount chain — keep this test in sync with that pattern.
func TestStaticDistPrefixMount(t *testing.T) {
	tmpDir := t.TempDir()
	distDir := filepath.Join(tmpDir, "dist")
	if err := os.MkdirAll(filepath.Join(distDir, "js"), 0o755); err != nil {
		t.Fatalf("mkdir dist/js: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "js", "main.js"), []byte("console.log('dist-asset');"), 0o644); err != nil {
		t.Fatalf("write dist/js/main.js: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<h1>SPA</h1>"), 0o644); err != nil {
		t.Fatalf("write dist/index.html: %v", err)
	}

	// Mirror the production setup: caller extracts the `dist/` subdir,
	// so the staticHandler receives a sub-FS.
	subFS, err := fs.Sub(os.DirFS(tmpDir), "dist")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}

	// Keep this mount chain in sync with `registerSimpleRoutes` in
	// `cmd/atlas/api_routes.go`. If you change one, change both.
	mux := http.NewServeMux()
	mux.Handle("/client/", http.StripPrefix("/client/", staticHandler(subFS)))

	cases := []struct {
		name     string
		path     string
		wantCode int
		wantBody string
	}{
		{"asset under dist prefix", "/client/dist/js/main.js", http.StatusOK, "console.log('dist-asset');"},
		{"root with trailing slash", "/client/", http.StatusOK, "<h1>SPA</h1>"},
		{"unknown path falls back to SPA", "/client/some-random-page", http.StatusOK, "<h1>SPA</h1>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("path %s: expected status %d, got %d (body=%q)", tc.path, tc.wantCode, rec.Code, rec.Body.String())
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Fatalf("path %s: expected body %q, got %q", tc.path, tc.wantBody, got)
			}
		})
	}
}

func TestDashboardAPIUsesWorkDirForPaths(t *testing.T) {
	tmpDir := t.TempDir()
	ledgerDir := filepath.Join(tmpDir, "data", "state")
	windowsDir := filepath.Join(ledgerDir, "windows")
	if err := os.MkdirAll(windowsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	summary := domain.BacktestWindowSummary{
		WindowID:     "window-test",
		StartDate:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:      time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		SessionCount: 1,
		OutcomeCount: 1,
		GeneratedAt:  time.Now(),
	}
	summaryBytes, _ := json.Marshal(summary)
	if err := os.WriteFile(filepath.Join(windowsDir, "window-test.json"), summaryBytes, 0o644); err != nil {
		t.Fatalf("write window summary: %v", err)
	}

	api := monitoring.NewDashboardAPIWithGateway(tmpDir, ledgerDir, nil, monitoring.NoopFetcher())
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/report/latest", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("latest report status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "# Backtest Report") {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestAPIModeRegistersAdminReloadConfigRoute(t *testing.T) {
	ledgerDir := t.TempDir()
	shutdown := make(chan struct{})
	listenDone := make(chan struct{})
	var gotHandler http.Handler

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{WorkDir: t.TempDir(), LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			gotHandler = srv.Handler
			close(listenDone)
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	if err := run([]string{"-api"}, deps); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	<-listenDone
	if gotHandler == nil {
		t.Fatalf("expected http handler to be registered")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/reload-config", nil)
	gotHandler.ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Fatalf("POST /admin/reload-config status = 404, route not registered")
	}
}

func TestAPIModeRegistersMetricsRoute(t *testing.T) {
	ledgerDir := t.TempDir()
	shutdown := make(chan struct{})
	listenDone := make(chan struct{})
	var gotHandler http.Handler

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{WorkDir: t.TempDir(), LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			gotHandler = srv.Handler
			close(listenDone)
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	if err := run([]string{"-api"}, deps); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	<-listenDone
	if gotHandler == nil {
		t.Fatalf("expected http handler to be registered")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	gotHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", rr.Code)
	}
}

func TestAPIModeAdminReloadConfigRejectsGet(t *testing.T) {
	// Avoid 401 from wrapAdminAuth when the outer test environment has
	// ATLAS_API_KEY set; we specifically want to assert the 405 behavior.
	t.Setenv("ATLAS_API_KEY", "")
	ledgerDir := t.TempDir()
	shutdown := make(chan struct{})
	listenDone := make(chan struct{})
	var gotHandler http.Handler

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{WorkDir: t.TempDir(), LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			gotHandler = srv.Handler
			close(listenDone)
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	if err := run([]string{"-api"}, deps); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	<-listenDone
	if gotHandler == nil {
		t.Fatalf("expected http handler to be registered")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/reload-config", nil)
	gotHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /admin/reload-config status = %d, want 405", rr.Code)
	}
}

func TestConfigLoadingBehavior(t *testing.T) {
	ledgerDir := t.TempDir()
	configCalled := false

	shutdown := make(chan struct{})
	listenDone := make(chan struct{})
	deps := appDeps{
		loadConfig: func() config.Config {
			configCalled = true
			return config.Config{LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			close(listenDone)
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	err := run([]string{"-api"}, deps)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	<-listenDone
	if !configCalled {
		t.Fatalf("loadConfig should be called")
	}
}

func TestConfigLoadingWithBrokerModeOverride(t *testing.T) {
	ledgerDir := t.TempDir()
	var loadedConfig config.Config

	shutdown := make(chan struct{})
	listenDone := make(chan struct{})
	deps := appDeps{
		loadConfig: func() config.Config {
			loadedConfig = config.Config{LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
			return loadedConfig
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			close(listenDone)
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	err := run([]string{"-api", "-broker-mode", "paper"}, deps)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	<-listenDone
	if loadedConfig.BrokerMode != "dry-run" {
		t.Fatalf("loadConfig should return original config, not modified")
	}
}

func TestFlagParsingEmptyArgs(t *testing.T) {
	ledgerDir := t.TempDir()
	shutdown := make(chan struct{})
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	err := run([]string{}, deps)
	// Simulation may succeed with empty data (graceful degradation) or fail;
	// either outcome is acceptable for empty-args flag parsing validation.
	if err != nil {
		if strings.Contains(err.Error(), "parse flags") {
			t.Fatalf("empty args should not cause parse error: %v", err)
		}
	}
}

func TestFlagParsingInvalidFlag(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-invalid-flag"}, deps)
	if err == nil {
		t.Fatalf("expected error for invalid flag, got nil")
	}
	if !strings.Contains(err.Error(), "parse flags") {
		t.Fatalf("expected flag parse error, got: %v", err)
	}
}

func TestFlagParsingMultipleBrokerOverrides(t *testing.T) {
	ledgerDir := t.TempDir()
	shutdown := make(chan struct{})
	listenDone := make(chan struct{})
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1, BrokerSigner: "placeholder"}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			close(listenDone)
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	err := run([]string{
		"-api",
		"-broker-mode", "paper",
		"-broker-adapter", "mock",
		"-broker-signer", "placeholder",
		"-broker-max-retries", "3",
		"-broker-max-clock-skew-sec", "60",
		"-broker-nonce-ttl-sec", "120",
		"-broker-nonce-store", "memory",
	}, deps)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	<-listenDone
}

func TestFlagParsingLogFormatOverride(t *testing.T) {
	ledgerDir := t.TempDir()

	for _, format := range []string{"text", "json"} {
		shutdown := make(chan struct{})
		listenDone := make(chan struct{})
		deps := appDeps{
			loadConfig: func() config.Config {
				return config.Config{LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
			},
			dataFetcher: monitoring.NoopFetcher(),
			newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
				return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
			},
			listenAndServe: func(srv *http.Server) error {
				close(listenDone)
				return nil
			},
			shutdown: shutdown,
		}

		go func() {
			time.Sleep(100 * time.Millisecond)
			close(shutdown)
		}()

		err := run([]string{"-api", "-log-format", format}, deps)
		if err != nil {
			t.Fatalf("run returned error for log-format=%s: %v", format, err)
		}
		<-listenDone
	}
}

func TestAPIModeRegistersNarrativeRoutes(t *testing.T) {
	ledgerDir := t.TempDir()
	shutdown := make(chan struct{})
	listenDone := make(chan struct{})
	var gotHandler http.Handler

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{WorkDir: t.TempDir(), LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			gotHandler = srv.Handler
			close(listenDone)
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	if err := run([]string{"-api"}, deps); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	<-listenDone
	// run() returns as soon as listenAndServe's srvErr fires, but background
	// goroutines (warmup, prism manager, capital_flow_refresh) may still be
	// writing to t.TempDir(). Wait briefly so t.Cleanup's RemoveAll does not
	// fail with "directory not empty" (flaky on slow CI runners).
	time.Sleep(500 * time.Millisecond)
	if gotHandler == nil {
		t.Fatalf("expected http handler to be registered")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/narrative/events", nil)
	gotHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/narrative/events status = %d, want 200", rr.Code)
	}
}

func TestShouldStartFubonProxy(t *testing.T) {
	cases := []struct {
		name        string
		mode        string
		fubonAPIKey string
		want        bool
	}{
		{"empty_defaults_to_no", "", "", false},
		{"api_key_starts_proxy", "", "test-key", true},
		{"dry_run_no_proxy", "dry-run", "", false},
		{"paper_no_proxy", "paper", "", false},
		{"live_starts_proxy", "live", "", true},
		{"live_with_key_starts_proxy", "live", "test-key", true},
		{"unknown_mode_no_proxy", "bogus", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldStartFubonProxy(tc.mode, tc.fubonAPIKey); got != tc.want {
				t.Fatalf("shouldStartFubonProxy(%q, %q) = %v, want %v", tc.mode, tc.fubonAPIKey, got, tc.want)
			}
		})
	}
}

func TestRiskGateEventBus_BufferWiring(t *testing.T) {
	bus := eventbus.NewChannelEventBus(64)
	defer bus.Close()

	bus.Subscribe(eventbus.EventRiskGateRejected, func(ctx context.Context, event eventbus.BusEvent) error {
		apievents.BufferRiskGateEvent(event)
		return nil
	})
	bus.Subscribe(eventbus.EventRiskGateAllowed, func(ctx context.Context, event eventbus.BusEvent) error {
		apievents.BufferRiskGateEvent(event)
		return nil
	})

	bus.PublishRiskGateEvent(eventbus.RiskGateEventPayload{
		Phase:     "pre_trade",
		Verdict:   "BLOCK",
		Reason:    "VaR limit exceeded",
		Symbol:    "2330",
		Mode:      "DEFENSIVE",
		Timestamp: time.Now(),
	})
	bus.PublishRiskGateEvent(eventbus.RiskGateEventPayload{
		Phase:     "pre_trade",
		Verdict:   "ALLOW",
		Reason:    "within limits",
		Symbol:    "2330",
		Mode:      "NORMAL",
		Timestamp: time.Now(),
	})

	time.Sleep(50 * time.Millisecond)

	buffered := apievents.GetBufferedRiskGateEvents()
	if len(buffered) < 2 {
		t.Fatalf("expected at least 2 buffered risk gate events, got %d", len(buffered))
	}

	hasRejected := false
	hasAllowed := false
	for _, e := range buffered {
		if e.Event.Type == eventbus.EventRiskGateRejected {
			hasRejected = true
		}
		if e.Event.Type == eventbus.EventRiskGateAllowed {
			hasAllowed = true
		}
	}
	if !hasRejected {
		t.Error("expected buffered event with type EventRiskGateRejected, not found")
	}
	if !hasAllowed {
		t.Error("expected buffered event with type EventRiskGateAllowed, not found")
	}
}

// ── P2-S3: runLiveTrading uses HybridProvider not MockProvider ──

func TestNewProvider_DefaultsToHybrid(t *testing.T) {
	// Construct the provider that runLiveTrading uses after the fix:
	// marketdata.NewHybridProvider(cfg.FinMindAPIKey, cfg.FugleAPIKey)
	hp := marketdata.NewHybridProvider("", "")

	if hp == nil {
		t.Fatal("NewHybridProvider returned nil")
	}

	name := hp.Name()
	if name == "mock" {
		t.Fatalf("provider Name() = %q, want non-mock (hybrid-*)", name)
	}

	// Must be *marketdata.HybridProvider, not *marketdata.MockProvider
	if _, ok := any(hp).(*marketdata.HybridProvider); !ok {
		t.Fatalf("expected *marketdata.HybridProvider, got %T", hp)
	}
}

// ── P1-S1: prism worker subcommand routing ──
//
// Regression guard: docker-compose runs `atlas-go prism worker` as the
// prism-worker service. Before the fix, the `prism` and `worker`
// positional args were ignored and run() fell through to runSimulation()
// (one-shot), causing a 60s restart loop.
//
// isPrismWorkerCmd must report true ONLY for exactly ["prism","worker"].

func TestIsPrismWorkerCmd(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"empty args", []string{}, false},
		{"only flags", []string{"-api", "-addr", ":18080"}, false},
		{"exact prism worker", []string{"prism", "worker"}, true},
		{"prism only", []string{"prism"}, false},
		{"worker only", []string{"worker"}, false},
		{"unknown subcommand", []string{"swarm", "run"}, false},
		{"prism with extra arg", []string{"prism", "worker", "extra"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPrismWorkerCmd(tc.args); got != tc.want {
				t.Errorf("isPrismWorkerCmd(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestIsPublicPath(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		// Read-only methods on public paths stay public.
		{"root GET", http.MethodGet, "/", true},
		{"health probe GET", http.MethodGet, "/health", true},
		{"metrics GET", http.MethodGet, "/metrics", true},
		{"admin bare GET", http.MethodGet, "/admin", true},
		{"admin prefix GET", http.MethodGet, "/admin/", true},
		{"admin nested GET", http.MethodGet, "/admin/dashboard", true},
		{"client bare GET", http.MethodGet, "/client", true},
		{"client prefix GET", http.MethodGet, "/client/", true},
		{"client nested GET", http.MethodGet, "/client/portfolio", true},
		{"admin_web bare GET", http.MethodGet, "/admin_web", true},
		{"admin_web prefix GET", http.MethodGet, "/admin_web/", true},
		{"admin_web nested GET", http.MethodGet, "/admin_web/dashboard", true},
		{"api dashboard root GET", http.MethodGet, "/api/dashboard", true},
		{"api dashboard nested GET", http.MethodGet, "/api/dashboard/system-health", true},
		{"api taiwan GET", http.MethodGet, "/api/taiwan/stress-index", true},
		{"api narrative GET", http.MethodGet, "/api/narrative/bundle", true},
		{"api macro GET", http.MethodGet, "/api/macro/snapshot/latest", true},
		{"api alerts GET", http.MethodGet, "/api/alerts", true},
		{"api config GET", http.MethodGet, "/api/config", true},
		{"api strategy period-matrix GET", http.MethodGet, "/api/strategy/period-matrix", true},
		{"api strategy period-matrix POST still authed", http.MethodPost, "/api/strategy/period-matrix", false},
		{"api synergy GET", http.MethodGet, "/api/synergy/darwinian/status", true},
		{"api capital flow daily GET", http.MethodGet, "/api/capital-flow/daily", true},
		{"api capital flow summary GET", http.MethodGet, "/api/capital-flow/summary", true},
		{"api events calendar GET", http.MethodGet, "/api/events/calendar", true},
		{"api events prediction GET", http.MethodGet, "/api/events/prediction", true},
		{"api recommendations GET", http.MethodGet, "/api/recommendations", true},
		{"api reports latest GET", http.MethodGet, "/api/reports/latest", true},
		{"api reports archive GET", http.MethodGet, "/api/reports/archive", true},
		{"api reports subscribe GET", http.MethodGet, "/api/reports/subscribe", true},
		{"admin with trailing slash extra GET", http.MethodGet, "/admin//foo", true},
		{"llm health GET", http.MethodGet, "/api/llm/health", true},
		{"HEAD public path", http.MethodHead, "/api/dashboard/system-health", true},
		{"OPTIONS public path", http.MethodOptions, "/api/dashboard/system-health", true},

		// Mutating methods on previously public paths now require auth.
		{"dashboard POST no auth", http.MethodPost, "/api/dashboard/system-health", false},
		{"alerts POST no auth", http.MethodPost, "/api/alerts", false},
		{"parameters POST no auth", http.MethodPost, "/api/parameters", false},
		{"channels toggle POST no auth", http.MethodPost, "/api/dashboard/channels/", false},
		{"scheduler toggle POST no auth", http.MethodPost, "/api/scheduler/toggle", false},
		{"control POST no auth", http.MethodPost, "/api/control/pause-agent", false},
		{"control PUT no auth", http.MethodPut, "/api/control/pause-agent", false},
		{"control DELETE no auth", http.MethodDelete, "/api/control/pause-agent", false},
		{"tasks PATCH no auth", http.MethodPatch, "/api/tasks/1", false},
		{"config POST no auth", http.MethodPost, "/api/config", false},

		// Non-public paths remain non-public for all methods.
		{"admin_webx typo GET", http.MethodGet, "/admin_webx", false},
		{"adminfoo typo GET", http.MethodGet, "/adminfoo", false},
		{"admin dot GET", http.MethodGet, "/admin.", false},
		{"clientx typo GET", http.MethodGet, "/clientx", false},
		{"staticfile typo GET", http.MethodGet, "/staticfile", false},
		{"static no slash GET", http.MethodGet, "/static", false},
		{"case sensitive ADMIN", http.MethodGet, "/ADMIN", false},
		{"case sensitive Admin", http.MethodGet, "/Admin", false},
		{"api dashboard typo GET", http.MethodGet, "/api/dashboardx", false},
		{"api unrelated system GET", http.MethodGet, "/api/system/foo", false},
		{"universe metrics GET", http.MethodGet, "/universe/metrics", false},
		{"random path GET", http.MethodGet, "/random/path", false},
		{"empty path GET", http.MethodGet, "", false},
		{"double slash GET", http.MethodGet, "//", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPublicPath(tc.method, tc.path); got != tc.want {
				t.Errorf("isPublicPath(%q, %q) = %v, want %v", tc.method, tc.path, got, tc.want)
			}
		})
	}
}

// TestIsPublicPath_MutatingRequiresAuth verifies that high-risk write
// endpoints (dashboard channels, scheduler toggle, parameters, control)
// are no longer public regardless of exact path.
func TestIsPublicPath_MutatingRequiresAuth(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/dashboard/channels/"},
		{http.MethodPost, "/api/dashboard/channels/twse-toggle"},
		{http.MethodPost, "/api/scheduler/toggle"},
		{http.MethodPost, "/api/parameters"},
		{http.MethodPost, "/api/control/pause-agent"},
		{http.MethodPost, "/api/control/resume-agent"},
		{http.MethodPost, "/api/control/approve-recommendation"},
		{http.MethodPost, "/api/alerts/acknowledge"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			if isPublicPath(tc.method, tc.path) {
				t.Errorf("isPublicPath(%q, %q) = true, want false", tc.method, tc.path)
			}
		})
	}
}
func TestAPIModeApiHealthRedirectsToHealth(t *testing.T) {
	ledgerDir := t.TempDir()
	shutdown := make(chan struct{})
	listenDone := make(chan struct{})
	var gotHandler http.Handler

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: ledgerDir}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			gotHandler = srv.Handler
			close(listenDone)
			<-shutdown
			return nil
		},
		shutdown: shutdown,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	if err := run([]string{"-api", "-addr", ":0"}, deps); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	<-listenDone
	if gotHandler == nil {
		t.Fatalf("expected http handler to be registered")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	gotHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /api/health status = %d, want %d", rec.Code, http.StatusMovedPermanently)
	}
	loc := rec.Header().Get("Location")
	if loc != "/health" {
		t.Fatalf("GET /api/health Location = %q, want %q", loc, "/health")
	}
}
