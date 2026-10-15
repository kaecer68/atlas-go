package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

func TestRunBacktestWindow(t *testing.T) {
	origReplay := os.Getenv("ATLAS_REPLAY_DATA_PATH")
	origLedger := os.Getenv("ATLAS_LEDGER_DIR")
	origSQLite := os.Getenv("ATLAS_SQLITE_PATH")
	origBackend := os.Getenv("ATLAS_STORE_BACKEND")
	defer func() {
		os.Setenv("ATLAS_REPLAY_DATA_PATH", origReplay)
		os.Setenv("ATLAS_LEDGER_DIR", origLedger)
		os.Setenv("ATLAS_SQLITE_PATH", origSQLite)
		os.Setenv("ATLAS_STORE_BACKEND", origBackend)
	}()
	// Hermetic: JSONL ledger in temp dir regardless of machine-global backend.
	os.Setenv("ATLAS_STORE_BACKEND", "jsonl")

	// Use sample replay data
	os.Setenv("ATLAS_REPLAY_DATA_PATH", filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv"))
	dir := t.TempDir()
	os.Setenv("ATLAS_LEDGER_DIR", dir)
	// Keep the test hermetic when the host environment defaults to sqlite.
	os.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))

	if err := run([]string{"-start", "2026-03-26", "-end", "2026-03-27"}); err != nil {
		t.Fatalf("run backtest-window: %v", err)
	}
}

func TestRunRejectsInvalidDate(t *testing.T) {
	if err := run([]string{"-start", "invalid", "-end", "2026-03-27"}); err == nil {
		t.Fatalf("expected error for invalid start date")
	}
}

// TestParseParamOverride_Valid verifies the helper splits a single override
// "name=value" into its parts and parses the value as float64.
func TestParseParamOverride_Valid(t *testing.T) {
	cases := []struct {
		input  string
		name   string
		floatV float64
	}{
		{"sizing_kelly_fraction=0.25", "sizing_kelly_fraction", 0.25},
		{"narrative_min_trend_strength=0.4701", "narrative_min_trend_strength", 0.4701},
		{"optimizer_min_trade_size=100", "optimizer_min_trade_size", 100.0},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			name, value, err := parseParamOverride(tc.input)
			if err != nil {
				t.Fatalf("parseParamOverride(%q) returned error: %v", tc.input, err)
			}
			if name != tc.name {
				t.Errorf("name: got %q, want %q", name, tc.name)
			}
			if value != tc.floatV {
				t.Errorf("value: got %v, want %v", value, tc.floatV)
			}
		})
	}
}

// TestParseParamOverride_Invalid verifies malformed entries are rejected with
// a descriptive error.
func TestParseParamOverride_Invalid(t *testing.T) {
	cases := []string{
		"sizing_kelly_fraction",     // missing '='
		"=0.25",                     // empty name
		"sizing_kelly_fraction=",    // empty value
		"sizing_kelly_fraction=abc", // non-numeric value
		"no_such_param=0.5",         // unknown registered name
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			_, _, err := parseParamOverride(input)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", input)
			}
		})
	}
}

// TestApplyParamOverrides_Accumulates verifies multiple overrides land on
// the same in-memory ParametersConfig.
func TestApplyParamOverrides_Accumulates(t *testing.T) {
	ie := config.NewInferenceEngine(config.DefaultParametersConfig())

	overrides := []string{
		"sizing_kelly_fraction=0.25",
		"narrative_min_trend_strength=0.4701",
	}
	if err := applyParamOverrides(ie, overrides); err != nil {
		t.Fatalf("applyParamOverrides: %v", err)
	}

	gotKelly, ok := ie.GetParameter("sizing_kelly_fraction")
	if !ok {
		t.Fatalf("sizing_kelly_fraction not registered")
	}
	if gotKelly != 0.25 {
		t.Errorf("kelly: got %v, want 0.25", gotKelly)
	}

	gotTrend, ok := ie.GetParameter("narrative_min_trend_strength")
	if !ok {
		t.Fatalf("narrative_min_trend_strength not registered")
	}
	if gotTrend != 0.4701 {
		t.Errorf("trend: got %v, want 0.4701", gotTrend)
	}
}

// TestApplyParamOverrides_RejectsUnknownParam ensures unknown names fail fast.
func TestApplyParamOverrides_RejectsUnknownParam(t *testing.T) {
	ie := config.NewInferenceEngine(config.DefaultParametersConfig())
	err := applyParamOverrides(ie, []string{"definitely_not_a_real_param=0.5"})
	if err == nil {
		t.Fatalf("expected error for unknown param, got nil")
	}
	if !strings.Contains(err.Error(), "definitely_not_a_real_param") {
		t.Errorf("error %v should name the offending param", err)
	}
}

// TestMaterializeParamConfig_OverriddenValuesWritten verifies the helper that
// serializes a ParametersConfig with overrides to a temp JSON file. The
// resulting file must round-trip and contain the overridden numeric values.
func TestMaterializeParamConfig_OverriddenValuesWritten(t *testing.T) {
	ie := config.NewInferenceEngine(config.DefaultParametersConfig())
	if err := applyParamOverrides(ie, []string{
		"sizing_kelly_fraction=0.25",
		"narrative_min_trend_strength=0.4701",
	}); err != nil {
		t.Fatalf("applyParamOverrides: %v", err)
	}

	path, err := materializeParamConfig(ie)
	if err != nil {
		t.Fatalf("materializeParamConfig: %v", err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(path)) }()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp params: %v", err)
	}
	loaded, err := config.LoadParametersConfig(path)
	if err != nil {
		t.Fatalf("LoadParametersConfig: %v", err)
	}

	// Validate the values in the file match what we set, not the defaults.
	if loaded.Sizing.KellyFraction.Value != 0.25 {
		t.Errorf("kelly in file: got %v, want 0.25", loaded.Sizing.KellyFraction.Value)
	}
	if loaded.Narrative.MinTrendStrength.Value != 0.4701 {
		t.Errorf("trend in file: got %v, want 0.4701", loaded.Narrative.MinTrendStrength.Value)
	}

	// JSON must parse without error and be an object, not a bare array.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("temp params not valid JSON object: %v", err)
	}
	if _, ok := raw["sizing"]; !ok {
		t.Errorf("expected 'sizing' top-level key in serialized params, got keys: %v", reflect.ValueOf(raw).MapKeys())
	}
}

// TestRunWithParamOverride verifies the -param-override flag is plumbed all
// the way through main.run() and produces a backtest that does not error.
// We point cfg.ParametersConfigPath at a temp copy via the override path.
func TestRunWithParamOverride(t *testing.T) {
	origReplay := os.Getenv("ATLAS_REPLAY_DATA_PATH")
	origLedger := os.Getenv("ATLAS_LEDGER_DIR")
	origSQLite := os.Getenv("ATLAS_SQLITE_PATH")
	origBackend := os.Getenv("ATLAS_STORE_BACKEND")
	defer func() {
		os.Setenv("ATLAS_REPLAY_DATA_PATH", origReplay)
		os.Setenv("ATLAS_LEDGER_DIR", origLedger)
		os.Setenv("ATLAS_SQLITE_PATH", origSQLite)
		os.Setenv("ATLAS_STORE_BACKEND", origBackend)
	}()

	// Hermetic like TestRunBacktestWindow: run() now resolves the store backend
	// through openRunnerStore, so a machine-global ATLAS_STORE_BACKEND=postgres
	// (e.g. the production host, where DATABASE_URL is exported too) would make
	// this test open a real pool. Pin the backend to JSONL in a temp dir.
	os.Setenv("ATLAS_STORE_BACKEND", "jsonl")
	os.Setenv("ATLAS_REPLAY_DATA_PATH", filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv"))
	dir := t.TempDir()
	os.Setenv("ATLAS_LEDGER_DIR", dir)
	// Keep the test hermetic when the host environment defaults to sqlite.
	os.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))

	err := run([]string{
		"-start", "2026-03-26",
		"-end", "2026-03-27",
		"-param-override", "sizing_kelly_fraction=0.25",
		"-param-override", "narrative_min_trend_strength=0.4701",
	})
	if err != nil {
		t.Fatalf("run with -param-override: %v", err)
	}
}

// TestRunWithInvalidParamOverrideErrors verifies the run() surface surfaces
// override errors (not silent ignores).
func TestRunWithInvalidParamOverrideErrors(t *testing.T) {
	err := run([]string{
		"-start", "2026-03-26",
		"-end", "2026-03-27",
		"-param-override", "garbage_format",
	})
	if err == nil {
		t.Fatalf("expected error for malformed -param-override, got nil")
	}
	if !strings.Contains(err.Error(), "param-override") {
		t.Errorf("error %v should mention param-override", err)
	}
}

// ─── store seam（Q2 batch 2；站點 cmd/backtest-window/main.go:65） ───────────
//
// 這組測試守著同一件事：本 CLI 算出來的 window 必須寫進**與 cmd/atlas 的
// window_backtest 週任務同一個** backend-aware store（PR #2238 之後生產走 PG）。
// 每條測試對應一種「安靜地寫錯後端」的失效模式，改壞任一步都會紅：
//   * jsonl/sqlite 路徑去讀 DSN（不該開池）⇒ failingDeps 紅
//   * postgres 缺 DSN 退回 JSONL ⇒ 負向測試紅
//   * 建 store 早於注入池 ⇒ TestOpenRunnerStore_PostgresBuildsStoreOnlyAfterInjectingPool 紅
//   * 兩個後端算出不同 window ⇒ parity 測試紅

// fakeRunnerDSN 是單元測試用的假 DSN：位址指向 127.0.0.1:1（不可連），
// 配合 testdb.UnconnectedPool 完全不建立連線。真實 DSN 一律不出現在測試裡。
const fakeRunnerDSN = "postgres://unit:unit@127.0.0.1:1/atlas_unit"

// failingDeps 是「不該被呼叫」的 wiring：任何呼叫都讓測試紅燈，用來證明
// jsonl/sqlite 路徑**完全不碰 DSN**（連池都不開）。訊息只印 dsn 長度，不印內容。
func failingDeps(t *testing.T) storeDeps {
	t.Helper()
	return storeDeps{
		initPool: func(_ context.Context, dsn, migrationsPath string) (*pgxpool.Pool, error) {
			t.Errorf("initPool must not be called on a non-postgres backend (dsn_len=%d migrations=%q)", len(dsn), migrationsPath)
			return nil, errors.New("initPool must not be called")
		},
		injectPool: func(*pgxpool.Pool) {
			t.Error("injectPool must not be called on a non-postgres backend")
		},
	}
}

// poolRecorder 記錄「順序 = [init, inject]」與注入的池，斷言池真的被開、真的被注入。
type poolRecorder struct {
	fakePool       *pgxpool.Pool
	order          []string
	dsn            string
	migrationsPath string
	injected       []*pgxpool.Pool
}

// recordingDeps 是「記錄並真的注入」的 wiring（形狀同 cmd/backfill-quotes-range）。
func recordingDeps(t *testing.T) (storeDeps, *poolRecorder) {
	t.Helper()
	rec := &poolRecorder{fakePool: testdb.UnconnectedPool(t, fakeRunnerDSN)}
	deps := storeDeps{
		initPool: func(_ context.Context, dsn, migrationsPath string) (*pgxpool.Pool, error) {
			rec.order = append(rec.order, "init")
			rec.dsn = dsn
			rec.migrationsPath = migrationsPath
			return rec.fakePool, nil
		},
		injectPool: func(pool *pgxpool.Pool) {
			rec.order = append(rec.order, "inject")
			rec.injected = append(rec.injected, pool)
			ledger.SetPostgresPool(pool)
		},
	}
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })
	return deps, rec
}

// runWindowWithSeam 走 CLI 的同一個 seam（openRunnerStore → backtest.NewRunner）
// 跑一個真 window，回傳 summary 與 seam 給的 store。
func runWindowWithSeam(t *testing.T, cfg config.Config, start, end time.Time) (domain.BacktestWindowSummary, ledger.FullStore, error) {
	t.Helper()
	store, closeStore, err := openRunnerStore(context.Background(), cfg, failingDeps(t))
	if err != nil {
		return domain.BacktestWindowSummary{}, nil, err
	}
	defer closeStore()
	summary, err := backtest.NewRunner(cfg, store).Run(start, end)
	return summary, store, err
}

// TestOpenRunnerStore_JSONLDoesNotTouchDSN：ATLAS_STORE_BACKEND=jsonl 時 seam 回
// JSONL 後端、滿足 ledger.BacktestStore，且**完全不碰 DSN**（真的設了 DATABASE_URL
// 也不准被讀 —— failingDeps 會在 initPool 被呼叫時直接紅）。
func TestOpenRunnerStore_JSONLDoesNotTouchDSN(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "jsonl")
	t.Setenv("ATLAS_LEDGER_DIR", dir)
	t.Setenv("DATABASE_URL", fakeRunnerDSN)

	cfg := config.Load()
	store, closeStore, err := openRunnerStore(context.Background(), cfg, failingDeps(t))
	if err != nil {
		t.Fatalf("openRunnerStore(jsonl): %v", err)
	}
	defer closeStore()

	if _, ok := store.(*ledger.Store); !ok {
		t.Fatalf("store type = %T, want *ledger.Store (the JSONL backend)", store)
	}
	if _, ok := store.(ledger.BacktestStore); !ok {
		t.Fatalf("store %T must satisfy ledger.BacktestStore", store)
	}
}

// TestOpenRunnerStore_SQLiteWritesThroughTheRealBackend：sqlite 後端要真的寫進
// sqlite（用 backend-aware 的 NewWindowStore 讀得回來），且**不得**同時在 JSONL
// 目錄留下窗口檔 —— 否則同一個 window 又有兩個互不相見的副本。
func TestOpenRunnerStore_SQLiteWritesThroughTheRealBackend(t *testing.T) {
	ledgerDir := t.TempDir()
	sqlitePath := filepath.Join(t.TempDir(), "atlas.db")
	t.Setenv("ATLAS_STORE_BACKEND", "sqlite")
	t.Setenv("ATLAS_LEDGER_DIR", ledgerDir)
	t.Setenv("ATLAS_SQLITE_PATH", sqlitePath)
	t.Setenv("DATABASE_URL", fakeRunnerDSN)

	cfg := config.Load()
	store, closeStore, err := openRunnerStore(context.Background(), cfg, failingDeps(t))
	if err != nil {
		t.Fatalf("openRunnerStore(sqlite): %v", err)
	}
	defer closeStore()

	if _, ok := store.(*ledger.SQLiteStore); !ok {
		t.Fatalf("store type = %T, want *ledger.SQLiteStore (the SQLite backend)", store)
	}

	summary := domain.BacktestWindowSummary{
		WindowID:     "window-20260326-20260327",
		StartDate:    time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
		EndDate:      time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
		SessionCount: 2,
		OutcomeCount: 7,
	}
	if err := store.RecordWindowSummary(summary); err != nil {
		t.Fatalf("RecordWindowSummary through the seam store: %v", err)
	}

	windowStore, err := ledger.NewWindowStore(cfg)
	if err != nil {
		t.Fatalf("NewWindowStore: %v", err)
	}
	got, ok, err := windowStore.LoadWindowSummary(summary.WindowID)
	if err != nil {
		t.Fatalf("LoadWindowSummary: %v", err)
	}
	if !ok {
		t.Fatalf("window %s not found in the sqlite backend: the write did not land in the resolved backend", summary.WindowID)
	}
	if got.SessionCount != summary.SessionCount || got.OutcomeCount != summary.OutcomeCount ||
		!got.StartDate.Equal(summary.StartDate) || !got.EndDate.Equal(summary.EndDate) {
		t.Errorf("read-back summary = %+v, want the recorded %+v", got, summary)
	}

	jsonlPath := filepath.Join(ledgerDir, "windows", summary.WindowID+".json")
	if _, statErr := os.Stat(jsonlPath); !os.IsNotExist(statErr) {
		t.Fatalf("the window was also written to JSONL (%s, stat err %v); the resolved backend must be the only write path", jsonlPath, statErr)
	}
}

// TestOpenRunnerStore_PostgresWithoutDSNIsAnError：postgres 後端缺 DSN 必須是
// **明確錯誤**，不得靜默退回 JSONL（#2107 的事故形狀）。
func TestOpenRunnerStore_PostgresWithoutDSNIsAnError(t *testing.T) {
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "")

	cfg := config.Load()
	if cfg.DatabaseURL != "" {
		t.Fatalf("test setup: DATABASE_URL must be empty, got len=%d", len(cfg.DatabaseURL))
	}

	store, closeStore, err := openRunnerStore(context.Background(), cfg, failingDeps(t))
	if err == nil {
		if closeStore != nil {
			closeStore()
		}
		t.Fatalf("want an error for backend postgres without a DSN, got store %T (a fallback to JSONL is exactly the defect this seam removes)", store)
	}
	if store != nil || closeStore != nil {
		t.Fatalf("on error the seam must return (nil, nil): store=%v closer=%v", store, closeStore != nil)
	}

	msg := err.Error()
	for _, want := range []string{"postgres", "DATABASE_URL", "refusing"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q must mention %q so the operator knows what to set and that no fallback happened", msg, want)
		}
	}
}

// TestOpenRunnerStore_UnknownBackendIsAnError：未知後端值必須紅（不得落到預設）。
func TestOpenRunnerStore_UnknownBackendIsAnError(t *testing.T) {
	t.Setenv("ATLAS_STORE_BACKEND", "mysql")

	_, _, err := openRunnerStore(context.Background(), config.Load(), failingDeps(t))
	if err == nil {
		t.Fatalf("want an error for an unknown store backend, got nil")
	}
	if !strings.Contains(err.Error(), "mysql") {
		t.Errorf("error %q must name the offending backend value", err)
	}
}

// TestOpenRunnerStore_PostgresWithFakeSeam...：postgres 後端必須「先開池（initPool）
// → 再注入（injectPool）→ 才建 store」，並回非 nil 的 PG store；整條路徑不真的連線。
func TestOpenRunnerStore_PostgresWithFakeSeamOpensInjectsThenBuilds(t *testing.T) {
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", fakeRunnerDSN)
	t.Setenv("ATLAS_MIGRATIONS_PATH", "sql/migrations")

	// 全域池先歸零：若程式把「建 store」排在「注入池」之前，這裡就會因為
	// “requires SetPostgresPool” 而紅 —— 下面那條 err != nil 的檢查是順序的證據。
	ledger.SetPostgresPool(nil)
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })

	cfg := config.Load()
	deps, rec := recordingDeps(t)

	store, closeStore, err := openRunnerStore(context.Background(), cfg, deps)
	if err != nil {
		t.Fatalf("openRunnerStore(postgres, fake seam): %v", err)
	}
	defer closeStore()

	if !slices.Equal(rec.order, []string{"init", "inject"}) {
		t.Fatalf("seam order = %v, want [init inject]: the pool must be opened, then injected, then used", rec.order)
	}
	if rec.dsn != cfg.DatabaseURL {
		t.Errorf("initPool dsn length = %d, want %d (the DSN must come from cfg.DatabaseURL)", len(rec.dsn), len(cfg.DatabaseURL))
	}
	if rec.migrationsPath != cfg.MigrationsPath {
		t.Errorf("initPool migrations = %q, want cfg.MigrationsPath %q", rec.migrationsPath, cfg.MigrationsPath)
	}
	if len(rec.injected) != 1 || rec.injected[0] != rec.fakePool {
		t.Fatalf("injected pools = %v, want exactly the pool returned by initPool", rec.injected)
	}
	if _, ok := store.(*ledger.PostgresLedgerStore); !ok {
		t.Fatalf("store type = %T, want *ledger.PostgresLedgerStore (a JSONL fallback would be the defect)", store)
	}
}

// TestOpenRunnerStore_MigrationsPathFallsBackToWorkDir：cfg.MigrationsPath 為空時
// 用 filepath.Join(workDir, "sql/migrations")（與 cmd/backfill-quotes-range 相同）。
func TestOpenRunnerStore_MigrationsPathFallsBackToWorkDir(t *testing.T) {
	cfg := config.Load()
	cfg.StoreBackend = "postgres"
	cfg.DatabaseURL = fakeRunnerDSN
	cfg.MigrationsPath = ""
	cfg.WorkDir = filepath.Join("work", "atlas")

	deps, rec := recordingDeps(t)
	_, closeStore, err := openRunnerStore(context.Background(), cfg, deps)
	if err != nil {
		t.Fatalf("openRunnerStore: %v", err)
	}
	defer closeStore()

	if want := filepath.Join("work", "atlas", "sql", "migrations"); rec.migrationsPath != want {
		t.Errorf("initPool migrations = %q, want %q", rec.migrationsPath, want)
	}
}

// TestOpenRunnerStore_PostgresBuildsStoreOnlyAfterInjectingPool：injectPool 故意
// 「記錄但不注入」。store 是由 factory 在**注入之後**建的，所以當池沒真的進到
// factory 時，這條路徑一定以 “requires SetPostgresPool” 收尾、不會回一個看起來
// 正常的 store（也就是說：不會有「池沒進去但窗口照寫」的靜默路徑）。
func TestOpenRunnerStore_PostgresBuildsStoreOnlyAfterInjectingPool(t *testing.T) {
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", fakeRunnerDSN)
	ledger.SetPostgresPool(nil)
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })

	var injected []*pgxpool.Pool
	deps := storeDeps{
		initPool: func(context.Context, string, string) (*pgxpool.Pool, error) {
			return testdb.UnconnectedPool(t, fakeRunnerDSN), nil
		},
		injectPool: func(pool *pgxpool.Pool) { injected = append(injected, pool) },
	}

	store, _, err := openRunnerStore(context.Background(), config.Load(), deps)
	if len(injected) != 1 {
		t.Fatalf("injectPool calls = %d, want 1 (the pool must be handed to the factory)", len(injected))
	}
	if err == nil {
		t.Fatalf("want an error when the pool was never injected into the factory, got store %T", store)
	}
	if !strings.Contains(err.Error(), "SetPostgresPool") {
		t.Errorf("error %q must say the factory has no pool (i.e. the store is built after the injection)", err)
	}
}

// TestOpenRunnerStore_ParityJSONLAndSQLiteBackends：同一個 fixture（真 window）
// 走 jsonl 與 sqlite 兩個後端，要求
//
//	(1) 兩者的 BacktestWindowSummary 關鍵欄位逐欄相同；
//	(2) 各自落在自己的後端（jsonl 有 <LedgerDir>/windows/<id>.json；sqlite 用
//	    backend-aware 的 NewWindowStore 讀得到同一個 window）。
func TestOpenRunnerStore_ParityJSONLAndSQLiteBackends(t *testing.T) {
	// 用整個 sample 的日期跨度（2026-03-20→03-30 = 6 個 session），不要只用兩天：
	// SessionCount 才是這條測試裡真正會被後端差異拉開的欄位（outcome 在這個樣本
	// 一律是 0 —— 樣本沒有 forward return）。實測一次 window run ≈ 0.4s。
	start, err := time.Parse("2006-01-02", "2026-03-20")
	if err != nil {
		t.Fatalf("parse start: %v", err)
	}
	end, err := time.Parse("2006-01-02", "2026-03-30")
	if err != nil {
		t.Fatalf("parse end: %v", err)
	}
	t.Setenv("ATLAS_REPLAY_DATA_PATH", filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv"))
	t.Setenv("DATABASE_URL", "")

	// (1) jsonl 後端
	jsonlDir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "jsonl")
	t.Setenv("ATLAS_LEDGER_DIR", jsonlDir)
	jsonlCfg := config.Load()
	jsonlSummary, _, err := runWindowWithSeam(t, jsonlCfg, start, end)
	if err != nil {
		t.Fatalf("run window (jsonl): %v", err)
	}
	jsonlPath := filepath.Join(jsonlDir, "windows", jsonlSummary.WindowID+".json")
	if _, statErr := os.Stat(jsonlPath); statErr != nil {
		t.Fatalf("the jsonl backend must write %s: %v", jsonlPath, statErr)
	}

	// (2) sqlite 後端（同一個 fixture）
	sqliteLedgerDir := t.TempDir()
	sqlitePath := filepath.Join(t.TempDir(), "atlas.db")
	t.Setenv("ATLAS_STORE_BACKEND", "sqlite")
	t.Setenv("ATLAS_LEDGER_DIR", sqliteLedgerDir)
	t.Setenv("ATLAS_SQLITE_PATH", sqlitePath)
	sqliteCfg := config.Load()
	sqliteSummary, _, err := runWindowWithSeam(t, sqliteCfg, start, end)
	if err != nil {
		t.Fatalf("run window (sqlite): %v", err)
	}

	// (1) 逐欄相同（GeneratedAt 是 wall clock，不列入）
	if jsonlSummary.WindowID != sqliteSummary.WindowID ||
		!jsonlSummary.StartDate.Equal(sqliteSummary.StartDate) ||
		!jsonlSummary.EndDate.Equal(sqliteSummary.EndDate) ||
		jsonlSummary.SessionCount != sqliteSummary.SessionCount ||
		jsonlSummary.OutcomeCount != sqliteSummary.OutcomeCount {
		t.Errorf("key fields differ between backends:\n jsonl  = %+v\n sqlite = %+v", jsonlSummary, sqliteSummary)
	}
	if jsonlSummary.WorstAgentID != sqliteSummary.WorstAgentID ||
		jsonlSummary.WorstAgentSkill != sqliteSummary.WorstAgentSkill ||
		jsonlSummary.WorstAgentLayer != sqliteSummary.WorstAgentLayer ||
		jsonlSummary.WorstAgentWindowCount != sqliteSummary.WorstAgentWindowCount ||
		jsonlSummary.WorstAgentSharpeLike != sqliteSummary.WorstAgentSharpeLike {
		t.Errorf("worst-agent fields differ between backends:\n jsonl  = %+v\n sqlite = %+v", jsonlSummary, sqliteSummary)
	}
	t.Logf("parity: window=%s sessions=%d outcomes=%d worst_agent=%q sharpe_like=%.6f",
		jsonlSummary.WindowID, jsonlSummary.SessionCount, jsonlSummary.OutcomeCount,
		jsonlSummary.WorstAgentID, jsonlSummary.WorstAgentSharpeLike)

	// (2a) sqlite 這一側：用 backend-aware 的 window store 讀得回同一個 window
	windowStore, err := ledger.NewWindowStore(sqliteCfg)
	if err != nil {
		t.Fatalf("NewWindowStore(sqlite): %v", err)
	}
	readBack, ok, err := windowStore.LoadWindowSummary(sqliteSummary.WindowID)
	if err != nil {
		t.Fatalf("LoadWindowSummary(sqlite): %v", err)
	}
	if !ok {
		t.Fatalf("window %s not found in the sqlite backend after a sqlite run", sqliteSummary.WindowID)
	}
	if readBack.SessionCount != sqliteSummary.SessionCount || readBack.OutcomeCount != sqliteSummary.OutcomeCount {
		t.Errorf("sqlite read-back = %+v, want %+v", readBack, sqliteSummary)
	}

	// (2b) sqlite 這一側不得在 JSONL 目錄留下窗口檔
	sqliteJSONLPath := filepath.Join(sqliteLedgerDir, "windows", sqliteSummary.WindowID+".json")
	if _, statErr := os.Stat(sqliteJSONLPath); !os.IsNotExist(statErr) {
		t.Fatalf("the sqlite run also wrote %s (%v): the window must live in exactly one backend", sqliteJSONLPath, statErr)
	}
}

// TestRunLandsInTheResolvedBackend 是 run() 這一端的**端到端**守門：後端設成 sqlite
// 後跑一次真的 CLI，window 必須出現在 sqlite、且**不得**出現在 JSONL 目錄。
// 若有人把 run() 改回裸 JSONL store（本任務修掉的那一行），這一條會兩邊同時紅
// —— 這是「修好的缺陷不會無聲回歸」的證據。
func TestRunLandsInTheResolvedBackend(t *testing.T) {
	jsonlDir := t.TempDir()
	sqlitePath := filepath.Join(t.TempDir(), "atlas.db")
	t.Setenv("ATLAS_REPLAY_DATA_PATH", filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv"))
	t.Setenv("ATLAS_STORE_BACKEND", "sqlite")
	t.Setenv("ATLAS_LEDGER_DIR", jsonlDir)
	t.Setenv("ATLAS_SQLITE_PATH", sqlitePath)
	t.Setenv("DATABASE_URL", "")

	if err := run([]string{"-start", "2026-03-20", "-end", "2026-03-30"}); err != nil {
		t.Fatalf("run backtest-window (sqlite backend): %v", err)
	}

	const wantWindow = "window-20260320-20260330"
	windowStore, err := ledger.NewWindowStore(config.Load())
	if err != nil {
		t.Fatalf("NewWindowStore: %v", err)
	}
	summary, ok, err := windowStore.LoadWindowSummary(wantWindow)
	if err != nil {
		t.Fatalf("LoadWindowSummary: %v", err)
	}
	if !ok {
		t.Fatalf("window %s is not in the sqlite backend after a sqlite run: the CLI wrote it somewhere production never reads", wantWindow)
	}
	if summary.SessionCount != 6 {
		t.Errorf("session count = %d, want 6 (the sample fixture covers 2026-03-20→03-27 = 6 sessions)", summary.SessionCount)
	}

	jsonlPath := filepath.Join(jsonlDir, "windows", wantWindow+".json")
	if _, statErr := os.Stat(jsonlPath); !os.IsNotExist(statErr) {
		t.Fatalf("%s exists (stat err %v): the CLI also wrote the window to JSONL while the configured backend is sqlite", jsonlPath, statErr)
	}
}

// TestRunnerStoreBackend_LogDoesNotLeakDSN 守住遮蔽紀律：解析出的後端可以印，
// DSN 不可以。seam 走完 postgres 路徑後，log 輸出不得含 DSN 的任何片段。
func TestRunnerStoreBackend_LogDoesNotLeakDSN(t *testing.T) {
	const secretDSN = "postgres://leak_user:sup3r-s3cret@127.0.0.1:1/atlas_unit"

	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", secretDSN)

	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	cfg := config.Load()
	deps, _ := recordingDeps(t)
	_, closeStore, err := openRunnerStore(context.Background(), cfg, deps)
	if err != nil {
		t.Fatalf("openRunnerStore: %v", err)
	}
	defer closeStore()

	out := buf.String()
	if !strings.Contains(out, "backend=postgres") {
		t.Errorf("log %q must record the resolved backend", out)
	}
	for _, secret := range []string{"sup3r-s3cret", "leak_user", secretDSN} {
		if strings.Contains(out, secret) {
			t.Errorf("log output leaked the DSN (contains %q): %q", secret, out)
		}
	}
}
