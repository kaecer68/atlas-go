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
		shutdown:                    shutdown,
		runAutoCapitalFlowOnStartup: func(string) {},
		runAutoBackfillOnStartup:    func(string) {},
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
		runAutoCapitalFlowOnStartup: func(string) {},
		runAutoBackfillOnStartup:    func(string) {},
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
		runAutoCapitalFlowOnStartup: func(string) {},
		runAutoBackfillOnStartup:    func(string) {},
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
		shutdown:                    shutdown,
		runAutoCapitalFlowOnStartup: func(string) {},
		runAutoBackfillOnStartup:    func(string) {},
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

func TestValidateBrokerRuntimeConfigRejectsNegativeRetries(t *testing.T) {
	cfg := config.Config{BrokerMode: "dry-run", BrokerMaxRetries: -1}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "must be >= 0") {
		t.Fatalf("unexpected error: %v", err)
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
		runAutoCapitalFlowOnStartup: func(string) {},
		runAutoBackfillOnStartup:    func(string) {},
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
		runAutoCapitalFlowOnStartup: func(string) {},
		runAutoBackfillOnStartup:    func(string) {},
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
		shutdown:                    shutdown,
		runAutoCapitalFlowOnStartup: func(string) {},
		runAutoBackfillOnStartup:    func(string) {},
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

func TestRunRejectsRealSignerWithoutExplicitAllow(t *testing.T) {
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
		runAutoCapitalFlowOnStartup: func(string) {},
		runAutoBackfillOnStartup:    func(string) {},
	}

	err := run([]string{"-api", "-broker-mode", "live", "-allow-live-broker", "-broker-adapter", "http", "-allow-http-broker", "-broker-signer", "hmac-sha256"}, deps)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "allow-real-signer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAllowsRealSignerWithExplicitAllow(t *testing.T) {
	shutdown := make(chan struct{})
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
		shutdown:                    shutdown,
		runAutoCapitalFlowOnStartup: func(string) {},
		runAutoBackfillOnStartup:    func(string) {},
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(shutdown)
	}()

	err := run([]string{"-api", "-broker-mode", "live", "-allow-live-broker", "-broker-adapter", "http", "-allow-http-broker", "-broker-signer", "hmac-sha256", "-broker-key-id", "kid-1", "-allow-real-signer"}, deps)
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
		runAutoCapitalFlowOnStartup: func(string) {},
		runAutoBackfillOnStartup:    func(string) {},
	}

	err := run([]string{"-api", "-broker-mode", "live", "-allow-live-broker", "-broker-adapter", "http", "-allow-http-broker", "-broker-signer", "hmac-sha256", "-allow-real-signer"}, deps)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "key id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseStatusCodeCSV(t *testing.T) {
	fallback := []int{408, 429}
	got := parseStatusCodeCSV("400, 500, abc, 503", fallback)
	if len(got) != 3 || got[0] != 400 || got[1] != 500 || got[2] != 503 {
		t.Fatalf("parseStatusCodeCSV = %v, want [400 500 503]", got)
	}

	gotFallback := parseStatusCodeCSV("invalid", fallback)
	if len(gotFallback) != 2 || gotFallback[0] != 408 || gotFallback[1] != 429 {
		t.Fatalf("parseStatusCodeCSV fallback = %v, want [408 429]", gotFallback)
	}
}

func TestValidateBrokerRuntimeConfigRejectsInvalidRetryStatusCode(t *testing.T) {
	cfg := config.Config{BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerHTTPRetryStatusCodes: []int{200}}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "retry status code") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBrokerRuntimeConfigRejectsNegativeClockSkew(t *testing.T) {
	cfg := config.Config{BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1, BrokerMaxClockSkewS: -1, BrokerNonceTTLS: 300}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "clock skew") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBrokerRuntimeConfigRejectsNegativeNonceTTL(t *testing.T) {
	cfg := config.Config{BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1, BrokerMaxClockSkewS: 300, BrokerNonceTTLS: -1}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "nonce ttl") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBrokerRuntimeConfigRejectsUnsupportedNonceStore(t *testing.T) {
	cfg := config.Config{BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1, BrokerMaxClockSkewS: 300, BrokerNonceTTLS: 300, BrokerNonceStore: "invalid-store"}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "nonce store") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBrokerRuntimeConfigDefaultsFileNonceStorePathFromLedgerDir(t *testing.T) {
	ledgerDir := t.TempDir()
	cfg := config.Config{LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1, BrokerMaxClockSkewS: 300, BrokerNonceTTLS: 300, BrokerNonceStore: "file"}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(cfg.BrokerNonceStorePath, "broker-nonce-replay.json") {
		t.Fatalf("unexpected nonce store path: %q", cfg.BrokerNonceStorePath)
	}
}

func TestValidateBrokerRuntimeConfigDefaultsFileNonceStorePathWithEmptyLedgerDir(t *testing.T) {
	cfg := config.Config{BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1, BrokerMaxClockSkewS: 300, BrokerNonceTTLS: 300, BrokerNonceStore: "file"}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BrokerNonceStorePath != "data/state/broker-nonce-replay.json" {
		t.Fatalf("unexpected nonce store path: %q", cfg.BrokerNonceStorePath)
	}
}

func TestValidateBrokerRuntimeConfigNormalizesRelativeFileNonceStorePath(t *testing.T) {
	ledgerDir := t.TempDir()
	cfg := config.Config{
		LedgerDir:            ledgerDir,
		BrokerMode:           "dry-run",
		BrokerAdapter:        "guarded",
		BrokerMaxRetries:     1,
		BrokerMaxClockSkewS:  300,
		BrokerNonceTTLS:      300,
		BrokerNonceStore:     "file",
		BrokerNonceStorePath: "nonces/custom.json",
	}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(ledgerDir, "nonces/custom.json")
	if cfg.BrokerNonceStorePath != want {
		t.Fatalf("unexpected nonce store path: got %q want %q", cfg.BrokerNonceStorePath, want)
	}
}

func TestValidateBrokerRuntimeConfigKeepsAbsoluteFileNonceStorePath(t *testing.T) {
	absPath := filepath.Join(t.TempDir(), "nonce-store.json")
	cfg := config.Config{
		LedgerDir:            t.TempDir(),
		BrokerMode:           "dry-run",
		BrokerAdapter:        "guarded",
		BrokerMaxRetries:     1,
		BrokerMaxClockSkewS:  300,
		BrokerNonceTTLS:      300,
		BrokerNonceStore:     "file",
		BrokerNonceStorePath: absPath,
	}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BrokerNonceStorePath != absPath {
		t.Fatalf("unexpected nonce store path: got %q want %q", cfg.BrokerNonceStorePath, absPath)
	}
}

func TestValidateBrokerRuntimeConfigRejectsRedisNonceStoreWithoutURL(t *testing.T) {
	cfg := config.Config{
		BrokerMode:          "dry-run",
		BrokerAdapter:       "guarded",
		BrokerMaxRetries:    1,
		BrokerMaxClockSkewS: 300,
		BrokerNonceTTLS:     300,
		BrokerNonceStore:    "redis",
	}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "redis url") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBrokerRuntimeConfigDefaultsRedisKeyPrefix(t *testing.T) {
	cfg := config.Config{
		BrokerMode:                "dry-run",
		BrokerAdapter:             "guarded",
		BrokerMaxRetries:          1,
		BrokerMaxClockSkewS:       300,
		BrokerNonceTTLS:           300,
		BrokerNonceStore:          "redis",
		BrokerNonceRedisURL:       "redis://localhost:6379/0",
		BrokerNonceRedisKeyPrefix: "",
	}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BrokerNonceRedisKeyPrefix != "atlas:nonce:" {
		t.Fatalf("unexpected redis key prefix: %q", cfg.BrokerNonceRedisKeyPrefix)
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
		shutdown:                    shutdown,
		runAutoCapitalFlowOnStartup: func(string) {},
		runAutoBackfillOnStartup:    func(string) {},
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
		shutdown:                    shutdown,
		runAutoCapitalFlowOnStartup: func(string) {},
		runAutoBackfillOnStartup:    func(string) {},
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
