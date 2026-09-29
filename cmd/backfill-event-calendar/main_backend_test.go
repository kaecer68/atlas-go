package main

// #2107 的後端決策釘子：宣告優先、顯式 flag 覆寫、**不降級**。
//
// 這一組測試刻意不帶 build tag：它們不需要 PostgreSQL（initPostgresPool 被替換成
// 不連線的池），所以 `go test ./...` 就會紅，而不是等到 integration job。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// defaultSQLiteArtifact 是 -db 預設值相對 -workdir 的落地位置：宣告非 sqlite 時
// 這個檔案不得被建出來（那正是 #2107 的垃圾 artifact）。
func defaultSQLiteArtifact(workDir string) string {
	return filepath.Join(workDir, "data", "state", "atlas.db")
}

// poolRecorder 記錄 initPostgresPool seam 被怎麼呼叫。
type poolRecorder struct {
	pool           *pgxpool.Pool
	initCalls      int
	dsn            string
	migrationsPath string
}

// stubInitPostgresPool 以「不連線的池」替換 initPostgresPool seam（測試結束還原），
// 讓單元測試能驗證「開池 → 注入 → 建 store」的真實順序。
func stubInitPostgresPool(t *testing.T) *poolRecorder {
	t.Helper()
	rec := &poolRecorder{pool: testdb.UnconnectedPool(t, "postgres://unit:unit@127.0.0.1:1/atlas_unit")}
	orig := initPostgresPool
	initPostgresPool = func(_ context.Context, dsn, migrationsPath string) (*pgxpool.Pool, error) {
		rec.initCalls++
		rec.dsn = dsn
		rec.migrationsPath = migrationsPath
		return rec.pool, nil
	}
	t.Cleanup(func() {
		initPostgresPool = orig
		ledger.SetPostgresPool(nil) // 不讓全域注入外洩到同 package 的其他測試
	})
	return rec
}

// failInitPostgresPool 是「不該被呼叫」的 seam：任何呼叫都讓測試紅燈。
func failInitPostgresPool(t *testing.T) {
	t.Helper()
	orig := initPostgresPool
	initPostgresPool = func(_ context.Context, dsn, migrationsPath string) (*pgxpool.Pool, error) {
		t.Errorf("initPostgresPool must not be called (dsn=%q migrations=%q)", dsn, migrationsPath)
		return nil, errors.New("initPostgresPool must not be called")
	}
	t.Cleanup(func() { initPostgresPool = orig })
}

func TestCheckBackendFlags(t *testing.T) {
	if err := checkBackendFlags(false, false); err != nil {
		t.Fatalf("no explicit flag must follow ATLAS_STORE_BACKEND: %v", err)
	}
	if err := checkBackendFlags(true, false); err != nil {
		t.Fatalf("-db alone is valid: %v", err)
	}
	if err := checkBackendFlags(false, true); err != nil {
		t.Fatalf("-pg alone is valid: %v", err)
	}
	if err := checkBackendFlags(true, true); err == nil {
		t.Fatal("-pg and -db together is ambiguous and must fail loudly")
	}
}

// TestOpenSink_DeclaredPostgresWithoutDSNFailsLoudly 是 #2107 形狀的釘子：
// ATLAS_STORE_BACKEND=postgres 且沒有 DSN ⇒ 直接失敗，**不得**因為「沒帶 -pg」
// 就寫到 sqlite。斷言訊息（而不只是 err != nil）是因為原事故的錯誤訊息
// （"requires pool"）無法診斷。
func TestOpenSink_DeclaredPostgresWithoutDSNFailsLoudly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "")
	appCfg := config.Load()

	store, closeStore, err := openSink(context.Background(), runConfig{workDir: dir}, appCfg)
	if err == nil {
		t.Fatal("want error: declared postgres without a DSN must not silently open sqlite")
	}
	if store != nil || closeStore != nil {
		t.Fatalf("want nil store/closer on error, got store=%T closerNil=%v", store, closeStore == nil)
	}
	msg := err.Error()
	for _, want := range []string{"postgres", "-pg-dsn", "DATABASE_URL"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q must mention %q so the operator knows what to fix", msg, want)
		}
	}
	if _, statErr := os.Stat(defaultSQLiteArtifact(dir)); statErr == nil {
		t.Fatalf("declared postgres must not create the sqlite artifact %s", defaultSQLiteArtifact(dir))
	}
}

// TestOpenSink_ExplicitDBOverridesDeclaredPostgres -db 顯式覆寫宣告；同時是上面
// 那條釘子的**反假陽性**對照：同一個環境（宣告 postgres、無 DSN）下 sqlite 路徑
// 必須真的可用，證明前一條不是因為別的錯誤而紅。
func TestOpenSink_ExplicitDBOverridesDeclaredPostgres(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "")
	appCfg := config.Load()

	dbPath := filepath.Join(dir, "explicit.db")
	store, closeStore, err := openSink(context.Background(),
		runConfig{workDir: dir, dbPath: dbPath, dbExplicit: true}, appCfg)
	if err != nil {
		t.Fatalf("explicit -db must win over the declared backend: %v", err)
	}
	defer func() { _ = closeStore() }()
	if _, ok := store.(*ledger.SQLiteHistoricalStore); !ok {
		t.Fatalf("got %T, want *ledger.SQLiteHistoricalStore", store)
	}
	if _, statErr := os.Stat(dbPath); statErr != nil {
		t.Fatalf("explicit sqlite store must be created at %s: %v", dbPath, statErr)
	}
}

// TestOpenSink_DeclaredJSONLFailsLoudly 宣告的後端在本指令沒有實作 ⇒ 明確錯誤，
// 不得靜默改寫到 sqlite（period_history 是關聯表，HistoricalStore 沒有 jsonl 實作）。
func TestOpenSink_DeclaredJSONLFailsLoudly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "jsonl")
	t.Setenv("DATABASE_URL", "")
	appCfg := config.Load()
	failInitPostgresPool(t)

	_, _, err := openSink(context.Background(), runConfig{workDir: dir}, appCfg)
	if err == nil {
		t.Fatal("declared jsonl has no history implementation; want a loud error, not a silent sqlite write")
	}
	if !strings.Contains(err.Error(), "jsonl") {
		t.Errorf("error %q must name the unsupported backend", err.Error())
	}
	if _, statErr := os.Stat(defaultSQLiteArtifact(dir)); statErr == nil {
		t.Fatalf("declared jsonl must not create the sqlite artifact %s", defaultSQLiteArtifact(dir))
	}
}

// TestOpenSink_UnknownDeclaredBackendFails 未知的宣告值直接失敗（不退回 jsonl）。
func TestOpenSink_UnknownDeclaredBackendFails(t *testing.T) {
	t.Setenv("ATLAS_STORE_BACKEND", "mysql")
	t.Setenv("DATABASE_URL", "")
	appCfg := config.Load()

	if _, _, err := openSink(context.Background(), runConfig{workDir: t.TempDir()}, appCfg); err == nil {
		t.Fatal("unknown backend must fail loudly")
	}
}

// TestOpenSink_PostgresOpensAndInjectsPool 是最關鍵的守門：後端 postgres 且拿得到
// DSN ⇒ **真的**經過 initPostgresPool seam 開池、注入 ledger factory，然後才建
// store。突變（拿掉開池／注入／改回硬編 sqlite）必紅。
func TestOpenSink_PostgresOpensAndInjectsPool(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "")
	appCfg := config.Load()
	rec := stubInitPostgresPool(t)

	store, closeStore, err := openSink(context.Background(),
		runConfig{workDir: dir, pgDSN: "postgres://u:p@localhost:5432/atlas"}, appCfg)
	if err != nil {
		t.Fatalf("openSink: %v", err)
	}
	if rec.initCalls != 1 {
		t.Fatalf("initPostgresPool called %d times, want 1 (the pool must actually be opened)", rec.initCalls)
	}
	if rec.dsn != "postgres://u:p@localhost:5432/atlas" {
		t.Fatalf("dsn = %q, want the -pg-dsn value", rec.dsn)
	}
	if got, want := rec.migrationsPath, filepath.Join(dir, "sql", "migrations"); got != want {
		t.Fatalf("migrations path = %q, want %q (<workdir>/sql/migrations)", got, want)
	}
	if _, ok := store.(*ledger.PostgresHistoricalStore); !ok {
		t.Fatalf("got %T, want *ledger.PostgresHistoricalStore (injection must reach the factory)", store)
	}
	if closeStore == nil {
		t.Fatal("want a closer that closes the pool")
	}
	closeStore()
	if _, statErr := os.Stat(defaultSQLiteArtifact(dir)); statErr == nil {
		t.Fatalf("postgres backend must not create the sqlite artifact %s", defaultSQLiteArtifact(dir))
	}
}

// TestOpenSink_PostgresUsesEnvDSN 未帶 -pg-dsn 時讀 $DATABASE_URL
// （生產以環境變數跑，不帶 flag）。
func TestOpenSink_PostgresUsesEnvDSN(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "postgres://env@localhost:5432/atlas")
	appCfg := config.Load()
	rec := stubInitPostgresPool(t)

	_, closeStore, err := openSink(context.Background(), runConfig{workDir: dir}, appCfg)
	if err != nil {
		t.Fatalf("openSink: %v", err)
	}
	defer closeStore()
	if rec.dsn != "postgres://env@localhost:5432/atlas" {
		t.Fatalf("dsn = %q, want $DATABASE_URL", rec.dsn)
	}
}

// TestOpenSink_SQLiteBackendNeverOpensPool sqlite 後端行為不變：不讀 DSN、不開池
// （failInitPostgresPool 的任何呼叫都會讓本測試紅）。
func TestOpenSink_SQLiteBackendNeverOpensPool(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "sqlite")
	t.Setenv("DATABASE_URL", "postgres://must-not-be-used@localhost:5432/atlas")
	appCfg := config.Load()
	failInitPostgresPool(t)

	store, closeStore, err := openSink(context.Background(), runConfig{workDir: dir}, appCfg)
	if err != nil {
		t.Fatalf("sqlite backend must not require a DSN: %v", err)
	}
	defer func() { _ = closeStore() }()
	if _, ok := store.(*ledger.SQLiteHistoricalStore); !ok {
		t.Fatalf("got %T, want *ledger.SQLiteHistoricalStore", store)
	}
	if _, statErr := os.Stat(defaultSQLiteArtifact(dir)); statErr != nil {
		t.Fatalf("sqlite backend must honour the default artifact path %s: %v", defaultSQLiteArtifact(dir), statErr)
	}
}
