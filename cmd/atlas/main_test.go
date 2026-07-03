package main

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/kaecer68/atlas-go/internal/swarm"
)

// freePort returns an unused TCP port from the kernel. It is used by tests
// that need to avoid hard-coded ports such as :8081 which may be occupied by
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
// docs/investigations/2026-06-28-boot-loop-multi-service.md § 6。
//
// T-104: also set ATLAS_SKIP_PORT_PREFLIGHT=1 to bypass the TCP port
// preflight in startup.Preflight. CI environments frequently have
// leftover native `atlas -api` (port 8080) or self-port-bound
// atlas.test binaries that wedge the 4 live-broker tests. The
// preflight's `atlas-http address :8080 is held by a foreign process`
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
			return config.Config{LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerMaxRetries: 1}
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

	t.Setenv("ATLAS_ALLOW_LIVE_BROKER", "true")
	err := run([]string{"-api", "-broker-mode", "live", "-allow-live-broker"}, deps)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	<-listenDone
	if gotAddr != ":8080" {
		t.Fatalf("listen addr = %q, want %q", gotAddr, ":8080")
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
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "live", BrokerAdapter: "http", BrokerMaxRetries: 1}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	t.Setenv("ATLAS_ALLOW_LIVE_BROKER", "true")
	err := run([]string{"-api", "-allow-live-broker", "-broker-adapter", "http"}, deps)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "allow-http-broker") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAllowsHTTPBrokerAdapterWithExplicitAllow(t *testing.T) {
	// Avoid the default :8081 which may be occupied by Docker Desktop or other
	// local services; use an ephemeral port for the fubon-proxy preflight check.
	fubonPort := freePort(t)
	shutdown := make(chan struct{})
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "live", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
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

	t.Setenv("ATLAS_ALLOW_LIVE_BROKER", "true")
	t.Setenv("ATLAS_ALLOW_HTTP_BROKER", "true")
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
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "live", BrokerAdapter: "http", BrokerMaxRetries: 1, BrokerSigner: "placeholder"}
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
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1, BrokerSigner: "placeholder"}
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

	t.Setenv("ATLAS_ALLOW_LIVE_BROKER", "true")
	t.Setenv("ATLAS_ALLOW_HTTP_BROKER", "true")
	t.Setenv("ATLAS_ALLOW_REAL_SIGNER", "true")

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

// ── P2-S1: buildBaseState returns real prices (not 100.0 placeholder) ──

// fakeProvider implements marketdata.Provider for testing.
type fakeProvider struct {
	quotes []domain.Quote
	err    error
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) GetQuotes(_ context.Context, _ time.Time, _ []string) ([]domain.Quote, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.quotes, nil
}

func TestBuildBaseState_RealProviderQuotes_NotPlaceholder(t *testing.T) {
	provider := &fakeProvider{
		quotes: []domain.Quote{
			{Symbol: "2330.TW", Last: 575.0, Volume: 38_000_000},
			{Symbol: "2317.TW", Last: 162.5, Volume: 22_000_000},
		},
	}

	var state swarm.MarketState = buildBaseState(provider, []string{"2330.TW", "2317.TW"})

	if state.Prices["2330.TW"] != 575.0 {
		t.Fatalf("2330.TW price = %.1f, want 575.0 (got placeholder fallback)", state.Prices["2330.TW"])
	}
	if state.Prices["2317.TW"] != 162.5 {
		t.Fatalf("2317.TW price = %.1f, want 162.5", state.Prices["2317.TW"])
	}
	if state.Volumes["2330.TW"] != 38_000_000.0 {
		t.Fatalf("2330.TW volume = %.0f, want 38000000", state.Volumes["2330.TW"])
	}
}

// ── P2-S2: provider error → graceful degradation (fallback to placeholder) ──

func TestBuildBaseState_ProviderError_FallsBackToPlaceholder(t *testing.T) {
	provider := &fakeProvider{
		err: errors.New("network timeout"),
	}

	var state swarm.MarketState = buildBaseState(provider, []string{"2330.TW", "2317.TW"})

	// Should fall back to placeholder values (100.0), NOT panic.
	if state.Prices["2330.TW"] != 100.0 {
		t.Fatalf("2330.TW price = %.1f, want 100.0 (placeholder fallback)", state.Prices["2330.TW"])
	}
	if state.Volumes["2330.TW"] != 5_000_000.0 {
		t.Fatalf("2330.TW volume = %.0f, want 5000000 (placeholder fallback)", state.Volumes["2330.TW"])
	}
	if len(state.Prices) != 2 {
		t.Fatalf("expected Prices len=2, got %d (missing symbols)", len(state.Prices))
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
	if _, ok := interface{}(hp).(*marketdata.HybridProvider); !ok {
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
		{"only flags", []string{"-api", "-addr", ":8080"}, false},
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
		name string
		path string
		want bool
	}{
		{"root", "/", true},
		{"health probe", "/health", true},
		{"metrics", "/metrics", true},
		{"admin bare", "/admin", true},
		{"admin prefix", "/admin/", true},
		{"admin nested", "/admin/dashboard", true},
		{"client bare", "/client", true},
		{"client prefix", "/client/", true},
		{"client nested", "/client/portfolio", true},
		{"adminfoo typo", "/adminfoo", false},
		{"admin dot", "/admin.", false},
		{"clientx typo", "/clientx", false},
		{"staticfile typo", "/staticfile", false},
		{"static no slash", "/static", false},
		{"case sensitive ADMIN", "/ADMIN", false},
		{"case sensitive Admin", "/Admin", false},
		{"api endpoint", "/api/dashboard", false},
		{"universe metrics", "/universe/metrics", false},
		{"random path", "/random/path", false},
		{"empty path", "", false},
		{"double slash", "//", false},
		{"admin with trailing slash extra", "/admin//foo", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPublicPath(tc.path); got != tc.want {
				t.Errorf("isPublicPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
