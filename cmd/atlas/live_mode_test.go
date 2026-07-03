package main

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func TestLiveModeBrokerGuardrails(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        t.TempDir(),
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-live", "-broker-mode", "live"}, deps)
	if err == nil {
		t.Fatalf("expected error for live broker in live mode without flag, got nil")
	}
	if !strings.Contains(err.Error(), "disabled by default") {
		t.Fatalf("expected live broker guard error, got: %v", err)
	}
}

func TestLiveModeDashboardAPIWiring(t *testing.T) {
	ledgerDir := t.TempDir()
	var mu sync.Mutex
	dashboardAPICalled := false
	var capturedLedgerDir string
	var capturedCollector *monitoring.MetricsCollector

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        ledgerDir,
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			mu.Lock()
			dashboardAPICalled = true
			capturedLedgerDir = dir
			capturedCollector = collector
			mu.Unlock()
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- run([]string{"-live"}, deps)
	}()

	select {
	case <-time.After(500 * time.Millisecond):
		mu.Lock()
		wasCalled := dashboardAPICalled
		dir := capturedLedgerDir
		coll := capturedCollector
		mu.Unlock()
		if !wasCalled {
			t.Fatalf("live mode should create dashboard API")
		}
		if dir != ledgerDir {
			t.Fatalf("ledger dir = %q, want %q", dir, ledgerDir)
		}
		if coll == nil {
			t.Fatalf("metrics collector should be passed to dashboard API")
		}
	case err := <-done:
		mu.Lock()
		wasCalled := dashboardAPICalled
		mu.Unlock()
		if err == nil {
			t.Fatalf("live mode returned nil unexpectedly (should block)")
		}
		if !wasCalled && !strings.Contains(err.Error(), "live orchestrator") {
			t.Fatalf("live mode should create dashboard API before error: %v", err)
		}
	}
}

func TestLiveModeRejectsUnsupportedBrokerAdapter(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        t.TempDir(),
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-live", "-broker-adapter", "invalid"}, deps)
	if err == nil {
		t.Fatalf("expected error for unsupported adapter in live mode, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported broker adapter") {
		t.Fatalf("expected unsupported adapter error, got: %v", err)
	}
}

func TestLiveModeValidatesBrokerBeforeStarting(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        t.TempDir(),
				BrokerMode:       "live",
				BrokerAdapter:    "http",
				BrokerMaxRetries: 1,
				BrokerSigner:     "placeholder",
				AllowLiveBroker:  true,
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-live", "-allow-live-broker"}, deps)
	if err == nil {
		t.Fatalf("expected error for http adapter without allow flag, got nil")
	}
	if !strings.Contains(err.Error(), "allow-http-broker") {
		t.Fatalf("expected http broker guard error, got: %v", err)
	}
}

func TestLiveModeAcceptsDryRunBroker(t *testing.T) {
	ledgerDir := t.TempDir()
	var dashboardAPICalled atomic.Bool

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        ledgerDir,
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			dashboardAPICalled.Store(true)
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- run([]string{"-live"}, deps)
	}()

	select {
	case <-time.After(500 * time.Millisecond):
		if !dashboardAPICalled.Load() {
			t.Fatalf("live mode with dry-run broker should create dashboard API")
		}
	case err := <-done:
		if err != nil && !strings.Contains(err.Error(), "live orchestrator") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestLiveModePropagatesBrokerConfig(t *testing.T) {
	ledgerDir := t.TempDir()
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        ledgerDir,
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
				BrokerSigner:     "placeholder",
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- run([]string{
			"-live",
			"-broker-mode", "paper",
			"-broker-adapter", "mock",
			"-broker-max-retries", "5",
			"-broker-max-clock-skew-sec", "120",
			"-broker-nonce-ttl-sec", "180",
		}, deps)
	}()

	select {
	case <-time.After(500 * time.Millisecond):
	case err := <-done:
		if err != nil && strings.Contains(err.Error(), "broker") && (strings.Contains(err.Error(), "unsupported") || strings.Contains(err.Error(), "must be")) {
			t.Fatalf("paper/mock broker config should be valid: %v", err)
		}
	}
}

func TestLiveModeCallsListenAndServeViaDeps(t *testing.T) {
	ledgerDir := t.TempDir()
	var listenAndServeCalled atomic.Bool

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        ledgerDir,
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			listenAndServeCalled.Store(true)
			return nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- run([]string{"-live"}, deps)
	}()

	select {
	case <-time.After(500 * time.Millisecond):
		if !listenAndServeCalled.Load() {
			t.Fatal("live mode should call deps.listenAndServe (runLiveTrading now routes through the dep)")
		}
	case <-done:
	}
}

func TestLiveModeWithSwaggerEnabled(t *testing.T) {
	ledgerDir := t.TempDir()
	var dashboardAPICalled atomic.Bool

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        ledgerDir,
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			dashboardAPICalled.Store(true)
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- run([]string{"-live", "-swagger"}, deps)
	}()

	select {
	case <-time.After(500 * time.Millisecond):
		if !dashboardAPICalled.Load() {
			t.Fatalf("live mode with swagger should still create dashboard API")
		}
	case err := <-done:
		if err != nil && !strings.Contains(err.Error(), "live orchestrator") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestLiveModeRejectsNegativeMaxRetries(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        t.TempDir(),
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: -1,
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-live"}, deps)
	if err == nil {
		t.Fatalf("expected error for negative max retries, got nil")
	}
	if !strings.Contains(err.Error(), "must be >= 0") {
		t.Fatalf("expected max retries validation error, got: %v", err)
	}
}

func TestLiveModeWithAllValidBrokerFlags(t *testing.T) {
	ledgerDir := t.TempDir()
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        ledgerDir,
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
				BrokerSigner:     "placeholder",
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- run([]string{
			"-live",
			"-broker-mode", "paper",
			"-broker-adapter", "mock",
			"-broker-signer", "placeholder",
			"-broker-retry-status-codes", "408,429,503",
			"-broker-max-retries", "2",
			"-broker-max-clock-skew-sec", "300",
			"-broker-nonce-ttl-sec", "300",
			"-broker-nonce-store", "memory",
		}, deps)
	}()

	select {
	case <-time.After(500 * time.Millisecond):
	case err := <-done:
		if err != nil && strings.Contains(err.Error(), "broker") && (strings.Contains(err.Error(), "unsupported") || strings.Contains(err.Error(), "must be")) {
			t.Fatalf("all broker flags should be valid: %v", err)
		}
	}
}

func TestLiveModeWithFileNonceStore(t *testing.T) {
	ledgerDir := t.TempDir()
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        ledgerDir,
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- run([]string{
			"-live",
			"-broker-nonce-store", "file",
			"-broker-nonce-store-path", "test-nonces.json",
		}, deps)
	}()

	select {
	case <-time.After(500 * time.Millisecond):
	case err := <-done:
		if err != nil && strings.Contains(err.Error(), "nonce store") && strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("file nonce store should be supported: %v", err)
		}
	}
}

func TestLiveModeRejectsInvalidRetryStatusCodes(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        t.TempDir(),
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-live", "-broker-retry-status-codes", "200,301"}, deps)
	if err == nil {
		t.Fatalf("expected error for invalid retry status codes, got nil")
	}
	if !strings.Contains(err.Error(), "retry status code") {
		t.Fatalf("expected retry status code validation error, got: %v", err)
	}
}

func TestLiveModeRejectsRedisNonceStoreWithoutURL(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        t.TempDir(),
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
				BrokerNonceStore: "redis",
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-live"}, deps)
	if err == nil {
		t.Fatalf("expected error for redis nonce store without URL, got nil")
	}
	if !strings.Contains(err.Error(), "redis url") {
		t.Fatalf("expected redis URL validation error, got: %v", err)
	}
}

func TestLiveModeWithRedisNonceStoreAndURL(t *testing.T) {
	ledgerDir := t.TempDir()
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        ledgerDir,
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		dataFetcher: monitoring.NoopFetcher(),
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPIWithGateway(workDir, dir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- run([]string{
			"-live",
			"-broker-nonce-store", "redis",
			"-broker-nonce-redis-url", "redis://localhost:6379/0",
		}, deps)
	}()

	select {
	case <-time.After(500 * time.Millisecond):
	case err := <-done:
		if err != nil && strings.Contains(err.Error(), "nonce store") && strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("redis nonce store with URL should be valid: %v", err)
		}
	}
}
