package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestNewConfig(t *testing.T) {
	cfg := config.Config{WorkDir: "/tmp/test", LedgerDir: "/tmp/ledger"}
	bc := NewConfig(cfg)
	if bc.WorkDir != "/tmp/test" {
		t.Errorf("expected WorkDir /tmp/test, got %s", bc.WorkDir)
	}
	if bc.LedgerDir != "/tmp/ledger" {
		t.Errorf("expected LedgerDir /tmp/ledger, got %s", bc.LedgerDir)
	}
}

func TestInitMetrics(t *testing.T) {
	collector := InitMetrics()
	if collector == nil {
		t.Fatal("expected non-nil collector")
	}
}

func TestInitDatabaseNoDSN(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := InitDatabase(ctx, Config{WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool != nil {
		t.Error("expected nil pool when no DATABASE_URL")
	}
}

func TestInitStores(t *testing.T) {
	stores, err := InitStores(Config{WorkDir: t.TempDir(), LedgerDir: t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stores.OutcomeStore == nil {
		t.Error("expected non-nil outcome store")
	}
}

func TestInitRepositoryNilPool(t *testing.T) {
	stores, _ := InitStores(Config{WorkDir: t.TempDir(), LedgerDir: t.TempDir()})
	repo := InitRepository(nil, stores)
	if repo != nil {
		t.Error("expected nil repo when pool is nil")
	}
}

func TestInitTaskManagerNoPool(t *testing.T) {
	mgr := InitTaskManager(nil, Config{WorkDir: t.TempDir()})
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestInitRuntime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rt, err := InitRuntime(ctx, Config{WorkDir: t.TempDir(), LedgerDir: t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt == nil {
		t.Fatal("expected non-nil runtime")
	}
	if rt.MetricsCollector == nil {
		t.Error("expected metrics collector to be initialized")
	}
	if rt.TaskManager == nil {
		t.Error("expected task manager to be initialized")
	}
	rt.Close()
}

func TestApplyBrokerConfig(t *testing.T) {
	cfg := config.Config{BrokerMode: "dry-run", BrokerMaxRetries: 3}
	err := ApplyBrokerConfig(&cfg,
		BrokerOverrides{
			Mode:            "live",
			Adapter:         "http",
			Signer:          "hmac-sha256",
			AllowLiveBroker: true,
			AllowHTTPBroker: true,
			AllowRealSigner: true,
			KeyID:           "test-key",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BrokerMode != "live" {
		t.Errorf("expected mode live, got %s", cfg.BrokerMode)
	}
	if cfg.BrokerAdapter != "http" {
		t.Errorf("expected adapter http, got %s", cfg.BrokerAdapter)
	}
}

func TestApplyBrokerConfigRejectsInvalidMode(t *testing.T) {
	cfg := config.Config{BrokerMode: "dry-run"}
	err := ApplyBrokerConfig(&cfg, BrokerOverrides{Mode: "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestApplyBrokerConfigRejectsLiveWithoutFlag(t *testing.T) {
	cfg := config.Config{BrokerMode: "dry-run"}
	err := ApplyBrokerConfig(&cfg,
		BrokerOverrides{
			Mode:            "live",
			AllowLiveBroker: false,
		},
	)
	if err == nil {
		t.Fatal("expected error for live mode without flag")
	}
}

func TestParseStatusCodeCSV(t *testing.T) {
	fallback := []int{408, 429}
	got := parseStatusCodeCSV("400, 500, abc, 503", fallback)
	if len(got) != 3 || got[0] != 400 || got[1] != 500 || got[2] != 503 {
		t.Errorf("parseStatusCodeCSV = %v, want [400 500 503]", got)
	}

	gotFallback := parseStatusCodeCSV("invalid", fallback)
	if len(gotFallback) != 2 || gotFallback[0] != 408 || gotFallback[1] != 429 {
		t.Errorf("parseStatusCodeCSV fallback = %v, want [408 429]", gotFallback)
	}
}

func TestGetLatestReplayDate(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.csv")
	content := "date,open,close\n2024-01-01,100,110\n2024-01-03,105,115\n2024-01-02,102,112\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("write test csv: %v", err)
	}

	latest, err := getLatestReplayDate(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	if !latest.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, latest)
	}
}
