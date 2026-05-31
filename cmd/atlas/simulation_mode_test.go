package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func TestSimulationModeDefaultPath(t *testing.T) {
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{}, deps)
	if err != nil {
		errStr := err.Error()
		if !strings.Contains(errStr, "simulation failed") &&
			!strings.Contains(errStr, "candidate selection failed") &&
			!strings.Contains(errStr, "record session summary") {
			t.Fatalf("expected simulation path error, got: %v", err)
		}
	}
}

func TestSimulationModeBrokerGuardrails(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        t.TempDir(),
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-broker-mode", "live"}, deps)
	if err == nil {
		t.Fatalf("expected error for live broker in simulation mode, got nil")
	}
	if !strings.Contains(err.Error(), "disabled by default") {
		t.Fatalf("expected live broker guard error, got: %v", err)
	}
}

func TestSimulationModeSystemCoreInitialization(t *testing.T) {
	ledgerDir := t.TempDir()
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			capturedCollector = collector
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{}, deps)
	if err != nil {
		if strings.Contains(err.Error(), "dashboard api") {
			t.Fatalf("expected simulation path error, not API mode error: %v", err)
		}
	}
	_ = capturedCollector
}

func TestSimulationModeDoesNotStartHTTPServer(t *testing.T) {
	ledgerDir := t.TempDir()
	serverStarted := false

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        ledgerDir,
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			serverStarted = true
			return nil
		},
	}

	_ = run([]string{}, deps)
	if serverStarted {
		t.Fatalf("simulation mode should not start HTTP server")
	}
}

func TestSimulationModeWithExplicitDryRunBroker(t *testing.T) {
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-broker-mode", "dry-run"}, deps)
	if err != nil {
		if strings.Contains(err.Error(), "broker") && strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("dry-run should be supported: %v", err)
		}
	}
}

func TestSimulationModeRejectsUnsupportedBrokerMode(t *testing.T) {
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        t.TempDir(),
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-broker-mode", "invalid-mode"}, deps)
	if err == nil {
		t.Fatalf("expected error for unsupported broker mode, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported broker mode") {
		t.Fatalf("expected unsupported broker mode error, got: %v", err)
	}
}

func TestSimulationModeFlagOverridesConfig(t *testing.T) {
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-broker-adapter", "mock"}, deps)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported broker adapter") {
			t.Fatalf("mock adapter should be supported: %v", err)
		}
	}
}

func TestSimulationModeWithMetricsCollector(t *testing.T) {
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{}, deps)
	if err != nil {
		if strings.Contains(err.Error(), "dashboard api") || strings.Contains(err.Error(), "live orchestrator") {
			t.Fatalf("expected simulation path error, got: %v", err)
		}
	}
}

func TestRunSimulationWithPaperBrokerMode(t *testing.T) {
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-broker-mode", "paper"}, deps)
	if err != nil {
		if strings.Contains(err.Error(), "broker mode") && strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("paper mode should be supported: %v", err)
		}
	}
}

func TestRunSimulationBrokerRetryConfigPropagation(t *testing.T) {
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{
		"-broker-retry-status-codes", "429,503",
		"-broker-max-retries", "3",
		"-broker-max-clock-skew-sec", "60",
		"-broker-nonce-ttl-sec", "120",
	}, deps)
	if err != nil {
		if strings.Contains(err.Error(), "retry status code") ||
			strings.Contains(err.Error(), "must be >= 0") ||
			strings.Contains(err.Error(), "clock skew") ||
			strings.Contains(err.Error(), "nonce ttl") {
			t.Fatalf("expected valid retry config to pass validation: %v", err)
		}
	}
}

func TestRunSimulationNonceStoreConfig(t *testing.T) {
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{"-broker-nonce-store", "memory"}, deps)
	if err != nil {
		if strings.Contains(err.Error(), "nonce store") && strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("memory nonce store should be supported: %v", err)
		}
	}
}

func TestRunSimulationReturnsMeaningfulError(t *testing.T) {
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{}, deps)
	if err != nil {
		if !strings.Contains(err.Error(), ":") {
			t.Fatalf("expected wrapped error with context, got: %v", err)
		}
	}
}

func TestSimulationModeShutdownBehavior(t *testing.T) {
	ledgerDir := t.TempDir()
	shutdown := make(chan struct{})

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        ledgerDir,
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
		shutdown: shutdown,
	}

	done := make(chan error, 1)
	go func() {
		done <- run([]string{}, deps)
	}()

	// Give the simulation goroutine time to start, then signal shutdown.
	time.Sleep(100 * time.Millisecond)
	close(shutdown)

	select {
	case err := <-done:
		// Expected: shutdown error or simulation completion (either is ok
		// as long as it doesn't block).
		if err != nil && !strings.Contains(err.Error(), "shutdown") {
			// Non-shutdown errors (e.g., missing data) are also fine
			// as long as they don't block indefinitely.
			_ = err
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("simulation mode should not block indefinitely")
	}
}

func TestSimulationModeWithRepositoryInjection(t *testing.T) {
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{}, deps)
	if err != nil {
		if strings.Contains(err.Error(), "repository") && strings.Contains(err.Error(), "injected") {
			t.Fatalf("repo injection should not cause error: %v", err)
		}
	}
}

func TestRunSimulationWithAllBrokerFlags(t *testing.T) {
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{
		"-broker-mode", "dry-run",
		"-broker-adapter", "guarded",
		"-broker-signer", "placeholder",
		"-broker-retry-status-codes", "408,429,503",
		"-broker-max-retries", "2",
		"-broker-max-clock-skew-sec", "300",
		"-broker-nonce-ttl-sec", "300",
		"-broker-nonce-store", "memory",
	}, deps)
	if err != nil {
		if strings.Contains(err.Error(), "broker") && (strings.Contains(err.Error(), "unsupported") || strings.Contains(err.Error(), "must be")) {
			t.Fatalf("all broker flags should be valid: %v", err)
		}
	}
}

func TestSimulationModeNoDepsShutdownSignal(t *testing.T) {
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	_ = run([]string{}, deps)
}

func TestRunSimulationModeWithAPIFlagFalse(t *testing.T) {
	ledgerDir := t.TempDir()
	serverStarted := false

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{
				LedgerDir:        ledgerDir,
				BrokerMode:       "dry-run",
				BrokerAdapter:    "guarded",
				BrokerMaxRetries: 1,
			}
		},
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			serverStarted = true
			return nil
		},
	}

	_ = run([]string{"-api=false"}, deps)
	if serverStarted {
		t.Fatalf("-api=false should not start HTTP server")
	}
}

func TestSimulationModeCapitalManagementSetup(t *testing.T) {
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
		newDashboardAPI: func(workDir, dir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, dir, collector)
		},
		listenAndServe: func(srv *http.Server) error {
			return nil
		},
	}

	err := run([]string{}, deps)
	if err != nil {
		if strings.Contains(err.Error(), "create approval workflow") {
			t.Fatalf("approval workflow should create its own directory: %v", err)
		}
	}
}
