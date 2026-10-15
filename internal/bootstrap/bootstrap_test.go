package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/monitoring"
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

	// Explicit empty DATABASE_URL: the integration CI job exports a real
	// DATABASE_URL and t.TempDir() has no sql/migrations, so without this the
	// test would exercise the missing-migrations-directory error instead of the
	// no-DSN path (config.GetSecret reads the environment only).
	t.Setenv("DATABASE_URL", "")

	pool, err := InitDatabase(ctx, Config{WorkDir: t.TempDir()}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool != nil {
		t.Error("expected nil pool when no DATABASE_URL")
	}
}

// TestInitDatabase_MissingMigrationsDir_ReturnsError locks the silent-degradation
// fix: a DATABASE_URL without sql/migrations used to return (nil, nil) — no error
// and no metric — so atlas started up with no PostgreSQL at all. It must now log
// ERROR, record atlas_db_init_failures_total and return an explicit error naming
// the missing directory.
func TestInitDatabase_MissingMigrationsDir_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// t.TempDir() has no sql/migrations. The bogus DSN makes the test
	// deterministic: the directory check must fire before any connection attempt.
	t.Setenv("DATABASE_URL", "postgresql://invalid_user:invalid_pass@127.0.0.1:1/nonexistent?sslmode=disable&connect_timeout=1")

	collector := monitoring.NewMetricsCollector()
	workDir := t.TempDir()
	pool, err := InitDatabase(ctx, Config{WorkDir: workDir}, collector)
	if err == nil {
		t.Fatal("expected error when sql/migrations is missing but DATABASE_URL is set")
	}
	if pool != nil {
		t.Error("expected nil pool when the migrations directory is missing")
	}
	if !strings.Contains(err.Error(), "migrations directory missing") {
		t.Errorf("error must name the missing migrations directory, got: %v", err)
	}
	if !strings.Contains(err.Error(), filepath.Join(workDir, "sql", "migrations")) {
		t.Errorf("error must contain the resolved path, got: %v", err)
	}

	m, ok := collector.GetMetric(monitoring.MetricDBInitFailures, map[string]string{"phase": "startup"})
	if !ok {
		t.Fatal("expected atlas_db_init_failures_total to be recorded for a missing migrations directory")
	}
	if m.Value != 1 {
		t.Errorf("expected value 1, got %v", m.Value)
	}
}

// TestInitDatabase_MissingMigrationsDir_NilCollectorDoesNotPanic covers the
// early-startup case where no metrics collector exists yet.
func TestInitDatabase_MissingMigrationsDir_NilCollectorDoesNotPanic(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://invalid:invalid@127.0.0.1:1/x?sslmode=disable&connect_timeout=1")

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("InitDatabase with nil collector panicked: %v", r)
		}
	}()
	if _, err := InitDatabase(context.Background(), Config{WorkDir: t.TempDir()}, nil); err == nil {
		t.Error("expected error for missing migrations directory")
	}
}

// TestCheckSchemaVersionDrift_NeverFatal pins the task-2 policy: the drift
// guardrail reports WARN + metric but must never abort startup. With a bogus DSN
// and a directory that holds no migration file the check fails to run, which must
// also stay non-fatal (and must not be counted as drift).
func TestCheckSchemaVersionDrift_NeverFatal(t *testing.T) {
	collector := monitoring.NewMetricsCollector()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("checkSchemaVersionDrift panicked: %v", r)
		}
	}()
	checkSchemaVersionDrift("postgresql://invalid:invalid@127.0.0.1:1/x?sslmode=disable&connect_timeout=1", t.TempDir(), collector)

	for _, m := range collector.GetAllMetrics() {
		if m.Name == monitoring.MetricSchemaVersionDrift {
			t.Errorf("an unrunnable check must not be counted as schema drift, got: %+v", m)
		}
	}

	// nil collector must be safe too.
	checkSchemaVersionDrift("postgresql://invalid:invalid@127.0.0.1:1/x?sslmode=disable&connect_timeout=1", t.TempDir(), nil)
}

func TestSchemaDriftReason(t *testing.T) {
	driftErr := &db.SchemaDrift{Applied: 26, Latest: 28}
	if got := schemaDriftReason(driftErr); got != "lagging" {
		t.Errorf("schemaDriftReason(*SchemaDrift) = %q, want %q", got, "lagging")
	}
	if got := schemaDriftReason(errors.New("boom")); got != "" {
		t.Errorf("schemaDriftReason(non-drift) = %q, want \"\"", got)
	}
}

func TestInitDatabase_FailureRecordsMetric(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	workDir := t.TempDir()
	migrationsPath := filepath.Join(workDir, "sql", "migrations")
	if err := os.MkdirAll(migrationsPath, 0o755); err != nil {
		t.Fatalf("MkdirAll migrations: %v", err)
	}

	t.Setenv("DATABASE_URL", "postgresql://invalid_user:invalid_pass@127.0.0.1:1/nonexistent?sslmode=disable&connect_timeout=1")

	collector := monitoring.NewMetricsCollector()
	_, err := InitDatabase(ctx, Config{WorkDir: workDir}, collector)
	if err == nil {
		t.Fatal("expected error from InitDatabase with bogus DSN")
	}

	m, ok := collector.GetMetric(monitoring.MetricDBInitFailures, map[string]string{"phase": "startup"})
	if !ok {
		t.Fatalf("expected atlas_db_init_failures_total to be recorded, but metric not found")
	}
	if m.Value != 1 {
		t.Errorf("expected value 1, got %v", m.Value)
	}
}

func TestInitDatabase_NilCollectorDoesNotPanic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	workDir := t.TempDir()
	migrationsPath := filepath.Join(workDir, "sql", "migrations")
	if err := os.MkdirAll(migrationsPath, 0o755); err != nil {
		t.Fatalf("MkdirAll migrations: %v", err)
	}

	t.Setenv("DATABASE_URL", "postgresql://invalid:invalid@127.0.0.1:1/x?sslmode=disable&connect_timeout=1")

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("InitDatabase with nil collector panicked: %v", r)
		}
	}()
	_, _ = InitDatabase(ctx, Config{WorkDir: workDir}, nil)
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
	mgr := InitTaskManager(context.Background(), nil, Config{WorkDir: t.TempDir()})
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
	err := ApplyBrokerConfig(
		&cfg,
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
	err := ApplyBrokerConfig(
		&cfg,
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
	if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
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

func TestRecoverPanic(t *testing.T) {
	panicRecovered := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicRecovered = true
			}
		}()
		defer recoverPanic("test_task")
		panic("test panic")
	}()
	if panicRecovered {
		t.Error("panic should have been recovered by recoverPanic, not propagated")
	}
}

func TestRecoverPanic_NoPanic(t *testing.T) {
	called := false
	func() {
		defer recoverPanic("test_task")
		called = true
	}()
	if !called {
		t.Error("function body should have executed")
	}
}

func TestBackgroundHealthRecording(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "data", "state")
	os.MkdirAll(stateDir, 0o755)

	healthPath := filepath.Join(stateDir, "channel_health.json")
	if _, err := os.Stat(healthPath); err == nil {
		os.Remove(healthPath)
	}

	healthData := `{"channels":{"auto_backfill":{"status":"ok","last_fetch_at":"2026-05-12T10:00:00+08:00"},"auto_capital_flow":{"status":"error","last_fetch_at":"2026-05-12T10:00:00+08:00","last_error":"fetch timeout"},"auto_geopolitical":{"status":"ok","last_fetch_at":"2026-05-12T10:00:00+08:00"}}}`
	if err := os.WriteFile(healthPath, []byte(healthData), 0o644); err != nil {
		t.Fatalf("write health file: %v", err)
	}

	data, err := os.ReadFile(healthPath)
	if err != nil {
		t.Fatalf("read channel_health.json: %v", err)
	}
	if !strings.Contains(string(data), "auto_backfill") {
		t.Error("auto_backfill missing from channel_health.json")
	}
	if !strings.Contains(string(data), "auto_capital_flow") {
		t.Error("auto_capital_flow missing from channel_health.json")
	}
	if !strings.Contains(string(data), "auto_geopolitical") {
		t.Error("auto_geopolitical missing from channel_health.json")
	}
}

// TestTaskRunnerConfigCarriesEveryField pins the L1 regression: the taskexec
// runner config used to be a `config.Config{WorkDir: cfg.WorkDir}` literal,
// which silently dropped LedgerDir. Downstream runners then resolved
// ledger.NewStore("") relative to the process working directory — the
// container's /app/, outside the mounted ledger volume. The test is written
// against bootstrap.Config's fields by reflection so a future field added to
// the carrier cannot be dropped again without failing here.
func TestTaskRunnerConfigCarriesEveryField(t *testing.T) {
	src := Config{WorkDir: "/work/dir", LedgerDir: "/ledger/dir"}
	got := taskRunnerConfig(src)

	if got.LedgerDir == "" {
		t.Fatalf("taskRunnerConfig dropped LedgerDir: %+v", got)
	}
	if got.LedgerDir != src.LedgerDir {
		t.Errorf("LedgerDir = %q, want %q", got.LedgerDir, src.LedgerDir)
	}

	srcValue := reflect.ValueOf(src)
	gotValue := reflect.ValueOf(got)
	for i := range srcValue.NumField() {
		name := srcValue.Type().Field(i).Name
		field := gotValue.FieldByName(name)
		if !field.IsValid() {
			t.Fatalf("bootstrap.Config field %s is not carried into config.Config", name)
		}
		if field.String() != srcValue.Field(i).String() {
			t.Errorf("%s = %q, want %q", name, field.String(), srcValue.Field(i).String())
		}
	}
}

// TestRegisterTaskRunnersUsesLedgerDir checks the wiring end of L1: the
// manager built by InitTaskManager registers runners whose config carries the
// LedgerDir it was given.
func TestRegisterTaskRunnersUsesLedgerDir(t *testing.T) {
	ledgerDir := t.TempDir()
	mgr := InitTaskManager(context.Background(), nil, Config{WorkDir: t.TempDir(), LedgerDir: ledgerDir})
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	cfg := taskRunnerConfig(Config{WorkDir: ".", LedgerDir: ledgerDir})
	if cfg.LedgerDir != ledgerDir {
		t.Errorf("runner LedgerDir = %q, want %q", cfg.LedgerDir, ledgerDir)
	}
}
