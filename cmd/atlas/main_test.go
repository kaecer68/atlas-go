package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			if dir != ledgerDir {
				t.Fatalf("ledger dir = %q, want %q", dir, ledgerDir)
			}
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			if dir != ledgerDir {
				t.Fatalf("ledger dir = %q, want %q", dir, ledgerDir)
			}
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
	if gotAddr != ":8080" {
		t.Fatalf("listen addr = %q, want %q", gotAddr, ":8080")
	}
}

func TestRunRejectsUnsupportedBrokerAdapter(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "dry-run", BrokerMaxRetries: 1}
		},
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
	shutdown := make(chan struct{})
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "live", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
		},
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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

	err := run([]string{"-api", "-broker-mode", "live", "-allow-live-broker", "-broker-adapter", "http", "-allow-http-broker"}, deps)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}

func TestRunRejectsRealSignerWithoutKeyID(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "live", BrokerAdapter: "http", BrokerMaxRetries: 1, BrokerSigner: "placeholder"}
		},
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
	staticDir := filepath.Join(tmpDir, "web", "static")
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<h1>Atlas</h1>"), 0644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	shutdown := make(chan struct{})
	listenDone := make(chan struct{})
	var gotHandler http.Handler
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{WorkDir: tmpDir, LedgerDir: t.TempDir(), BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
		},
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	gotHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "<h1>Atlas</h1>") {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestDashboardAPIUsesWorkDirForPaths(t *testing.T) {
	tmpDir := t.TempDir()
	reportsDir := filepath.Join(tmpDir, "reports")
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(reportsDir, "backtest_test.md"), []byte("# Backtest Report"), 0644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	api := monitoring.NewDashboardAPI(tmpDir, filepath.Join(tmpDir, "data", "state"), nil)
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
		},
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{}, deps)
	if err == nil {
		t.Fatalf("expected simulation to fail, got nil")
	}
	if strings.Contains(err.Error(), "parse flags") {
		t.Fatalf("empty args should not cause parse error: %v", err)
	}
}

func TestFlagParsingInvalidFlag(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
		},
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
			newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
				return monitoring.NewDashboardAPI(workDir, dir, collector)
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
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
