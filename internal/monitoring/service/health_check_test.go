package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// mockGatewayHealthChecker simulates an HTTP-like dependency health summary.
type mockGatewayHealthChecker struct {
	summary map[string]string
}

func (m *mockGatewayHealthChecker) Summary() map[string]string {
	return m.summary
}

func pollHistory(mon *monitoring.Monitor, want int, timeout time.Duration) []monitoring.Alert {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		h := mon.GetHistory(want)
		if len(h) >= want {
			return h
		}
		time.Sleep(5 * time.Millisecond)
	}
	return mon.GetHistory(want)
}

func historyCategories(alerts []monitoring.Alert) []string {
	seen := make(map[string]bool)
	var cats []string
	for _, a := range alerts {
		if !seen[a.Category] {
			seen[a.Category] = true
			cats = append(cats, a.Category)
		}
	}
	return cats
}

func findAlert(alerts []monitoring.Alert, categoryContains, messageContains string) *monitoring.Alert {
	for i := range alerts {
		a := alerts[i]
		if strings.Contains(a.Category, categoryContains) && strings.Contains(a.Message, messageContains) {
			return &a
		}
	}
	return nil
}

func hasCategory(alerts []monitoring.Alert, category string) bool {
	for _, a := range alerts {
		if a.Category == category {
			return true
		}
	}
	return false
}

func TestHealthChecker_RunOnce_NoDependencies(t *testing.T) {
	mon := monitoring.NewMonitor()
	hc := monitoring.NewHealthChecker(mon, nil)

	if err := hc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	alerts := pollHistory(mon, 1, 100*time.Millisecond)
	if len(alerts) != 0 {
		t.Errorf("expected no alerts with nil deps, got %d: %+v", len(alerts), alerts)
	}
}

func TestHealthChecker_RunOnce_GatewayHealthy(t *testing.T) {
	mon := monitoring.NewMonitor()
	hc := monitoring.NewHealthChecker(mon, nil)
	hc.SetGateway(&mockGatewayHealthChecker{
		summary: map[string]string{
			"twse":  "ok",
			"fugle": "ok",
		},
	})

	if err := hc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// Gateway heartbeat no longer creates alerts — channel health is tracked
	// by ChannelHealthStore. Only state_store alerts (if any) should exist.
	alerts := pollHistory(mon, 1, 100*time.Millisecond)
	if hasCategory(alerts, "gateway") {
		t.Errorf("gateway heartbeat should NOT create alerts anymore, got: %v", historyCategories(alerts))
	}
}

func TestHealthChecker_RunOnce_GatewayUnhealthy(t *testing.T) {
	mon := monitoring.NewMonitor()
	hc := monitoring.NewHealthChecker(mon, nil)
	hc.SetGateway(&mockGatewayHealthChecker{
		summary: map[string]string{
			"twse":  "error",
			"fugle": "warn",
		},
	})

	if err := hc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// Gateway heartbeat no longer creates alerts — channel health is tracked
	// by ChannelHealthStore.
	alerts := pollHistory(mon, 1, 100*time.Millisecond)
	if hasCategory(alerts, "gateway") {
		t.Errorf("gateway heartbeat should NOT create alerts anymore, got: %v", historyCategories(alerts))
	}
}

func TestHealthChecker_RunOnce_StateStoreHealthy(t *testing.T) {
	mon := monitoring.NewMonitor()
	st := store.NewStateStore(t.TempDir())
	st.UpdatePortfolio(store.PortfolioState{Cash: 1_000_000, LastUpdated: time.Now()})
	hc := monitoring.NewHealthChecker(mon, st)

	if err := hc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	alerts := pollHistory(mon, 1, 100*time.Millisecond)
	if !hasCategory(alerts, "state_store") {
		t.Errorf("expected state_store alert, got categories: %v", historyCategories(alerts))
	}
	alert := findAlert(alerts, "state_store", "State store healthy")
	if alert == nil {
		t.Fatalf("expected healthy state_store alert, got: %v", alerts)
	}
	if alert.Level != monitoring.AlertLevelInfo {
		t.Errorf("expected info level for healthy state store, got %v", alert.Level)
	}
}

func TestHealthChecker_RunOnce_StateStoreUnhealthy(t *testing.T) {
	mon := monitoring.NewMonitor()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "state"), 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state", "portfolio_state.json"), []byte(`{"cash":0,"last_updated":"0001-01-01T00:00:00Z"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write portfolio state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state", "regime_state.json"), []byte(`{"current_regime":"neutral","confidence":0.5,"last_changed_at":"0001-01-01T00:00:00Z","determined_by":"test"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write regime state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state", "positions_current.json"), []byte("[]\n"), 0o644); err != nil {
		t.Fatalf("write positions state: %v", err)
	}
	st := store.NewStateStore(dir)
	if err := st.Load(); err != nil {
		t.Fatalf("load state: %v", err)
	}
	hc := monitoring.NewHealthChecker(mon, st)

	if err := hc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	alerts := pollHistory(mon, 1, 100*time.Millisecond)
	alert := findAlert(alerts, "state_store", "Failed to retrieve valid portfolio")
	if alert == nil {
		t.Fatalf("expected unhealthy state_store alert, got: %v", alerts)
	}
	if alert.Level != monitoring.AlertLevelError {
		t.Errorf("expected error level for unhealthy state store, got %v", alert.Level)
	}
}

func TestHealthChecker_RunOnce_ContextTimeout(t *testing.T) {
	mon := monitoring.NewMonitor()
	hc := monitoring.NewHealthChecker(mon, nil)
	hc.SetGateway(&mockGatewayHealthChecker{summary: map[string]string{"http": "ok"}})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := hc.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// Gateway heartbeat no longer creates alerts.
	alerts := pollHistory(mon, 1, 100*time.Millisecond)
	if hasCategory(alerts, "gateway") {
		t.Error("gateway heartbeat should NOT create alerts anymore")
	}
}

func TestHealthChecker_SetGateway_NilSafe(t *testing.T) {
	mon := monitoring.NewMonitor()
	hc := monitoring.NewHealthChecker(mon, nil)
	hc.SetGateway(nil)
	hc.SetGateway(nil)
	if err := hc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	alerts := pollHistory(mon, 1, 100*time.Millisecond)
	if len(alerts) != 0 {
		t.Errorf("expected no alerts, got %d", len(alerts))
	}
}

func TestHealthChecker_RunOnce_AllDependencies(t *testing.T) {
	mon := monitoring.NewMonitor()
	st := store.NewStateStore(t.TempDir())
	st.UpdatePortfolio(store.PortfolioState{Cash: 500_000, LastUpdated: time.Now()})
	hc := monitoring.NewHealthChecker(mon, st)
	hc.SetGateway(&mockGatewayHealthChecker{
		summary: map[string]string{
			"db":    "ok",   // simulated DB dependency
			"redis": "ok",   // simulated Redis dependency
			"http":  "warn", // simulated HTTP dependency
		},
	})

	if err := hc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// Gateway heartbeat no longer creates alerts — only state_store.
	alerts := pollHistory(mon, 1, 100*time.Millisecond)
	if hasCategory(alerts, "gateway") {
		t.Error("gateway heartbeat should NOT create alerts anymore")
	}
	if !hasCategory(alerts, "state_store") {
		t.Error("expected state_store alert")
	}
}
