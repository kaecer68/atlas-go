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
	err := validateBrokerRuntimeConfig(&cfg, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "must be >= 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}
