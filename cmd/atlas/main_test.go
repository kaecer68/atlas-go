package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func TestRunAPIModeStartsServerAndRegistersRoutes(t *testing.T) {
	ledgerDir := t.TempDir()
	var gotAddr string
	var gotHandler http.Handler

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: ledgerDir}
		},
		newDashboardAPI: func(dir string) routeRegistrar {
			if dir != ledgerDir {
				t.Fatalf("ledger dir = %q, want %q", dir, ledgerDir)
			}
			return monitoring.NewDashboardAPI(dir)
		},
		listenAndServe: func(addr string, handler http.Handler) error {
			gotAddr = addr
			gotHandler = handler
			return nil
		},
	}

	err := run([]string{"-api", "-addr", ":19090"}, deps)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
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
		newDashboardAPI: func(dir string) routeRegistrar {
			return monitoring.NewDashboardAPI(dir)
		},
		listenAndServe: func(addr string, handler http.Handler) error {
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
		newDashboardAPI: func(dir string) routeRegistrar {
			return monitoring.NewDashboardAPI(dir)
		},
		listenAndServe: func(addr string, handler http.Handler) error {
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

	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: ledgerDir, BrokerMode: "dry-run", BrokerMaxRetries: 1}
		},
		newDashboardAPI: func(dir string) routeRegistrar {
			if dir != ledgerDir {
				t.Fatalf("ledger dir = %q, want %q", dir, ledgerDir)
			}
			return monitoring.NewDashboardAPI(dir)
		},
		listenAndServe: func(addr string, handler http.Handler) error {
			gotAddr = addr
			return nil
		},
	}

	err := run([]string{"-api", "-broker-mode", "live", "-allow-live-broker"}, deps)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
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
		newDashboardAPI: func(dir string) routeRegistrar {
			return monitoring.NewDashboardAPI(dir)
		},
		listenAndServe: func(addr string, handler http.Handler) error {
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
		newDashboardAPI: func(dir string) routeRegistrar {
			return monitoring.NewDashboardAPI(dir)
		},
		listenAndServe: func(addr string, handler http.Handler) error {
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
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "live", BrokerAdapter: "guarded", BrokerMaxRetries: 1}
		},
		newDashboardAPI: func(dir string) routeRegistrar {
			return monitoring.NewDashboardAPI(dir)
		},
		listenAndServe: func(addr string, handler http.Handler) error {
			return nil
		},
	}

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
		newDashboardAPI: func(dir string) routeRegistrar {
			return monitoring.NewDashboardAPI(dir)
		},
		listenAndServe: func(addr string, handler http.Handler) error {
			return nil
		},
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
	deps := appDeps{
		loadConfig: func() config.Config {
			return config.Config{LedgerDir: t.TempDir(), BrokerMode: "live", BrokerAdapter: "http", BrokerMaxRetries: 1, BrokerSigner: "placeholder"}
		},
		newDashboardAPI: func(dir string) routeRegistrar {
			return monitoring.NewDashboardAPI(dir)
		},
		listenAndServe: func(addr string, handler http.Handler) error {
			return nil
		},
	}

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
		newDashboardAPI: func(dir string) routeRegistrar {
			return monitoring.NewDashboardAPI(dir)
		},
		listenAndServe: func(addr string, handler http.Handler) error {
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
	cfg := config.Config{BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1, BrokerMaxClockSkewS: 300, BrokerNonceTTLS: 300, BrokerNonceStore: "redis"}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "nonce store") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBrokerRuntimeConfigRejectsFileNonceStoreWithoutPath(t *testing.T) {
	cfg := config.Config{BrokerMode: "dry-run", BrokerAdapter: "guarded", BrokerMaxRetries: 1, BrokerMaxClockSkewS: 300, BrokerNonceTTLS: 300, BrokerNonceStore: "file"}
	err := validateBrokerRuntimeConfig(&cfg, false, false, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "store path") {
		t.Fatalf("unexpected error: %v", err)
	}
}
