package main

// #2107 的後端決策釘子（本檔刻意不帶 build tag：不需要 PostgreSQL 就能紅）。

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// defaultSQLiteArtifact 是 -db 未給時 sqlite 模式的落地位置：宣告非 sqlite 時
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

// stubInitPostgresPool 以「不連線的池」替換 initPostgresPool seam（測試結束還原）。
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
	t.Cleanup(func() { initPostgresPool = orig })
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

func TestCheckModeFlags(t *testing.T) {
	if err := checkModeFlags(false, false, ""); err != nil {
		t.Fatalf("no explicit flag must follow ATLAS_STORE_BACKEND: %v", err)
	}
	for _, tc := range []struct {
		dbSet bool
		usePG bool
		jsonl string
	}{
		{false, true, ""}, {false, false, "data/state"}, {true, false, ""},
	} {
		if err := checkModeFlags(tc.dbSet, tc.usePG, tc.jsonl); err != nil {
			t.Fatalf("single explicit mode must be valid (%+v): %v", tc, err)
		}
	}
	if err := checkModeFlags(true, true, "data/state"); err == nil {
		t.Fatal("-db with -pg/-jsonl is ambiguous and must fail loudly")
	}
	if err := checkModeFlags(true, false, "data/state"); err == nil {
		t.Fatal("-db with -jsonl is ambiguous and must fail loudly")
	}
}

// TestResolveMode_FollowsDeclaredBackend 是後端決策表：沒有顯式 flag 時**跟隨宣告**。
func TestResolveMode_FollowsDeclaredBackend(t *testing.T) {
	cases := []struct {
		declared string
		want     storeMode
		wantErr  bool
	}{
		{declared: "postgres", want: modePostgres},
		{declared: "sqlite", want: modeSQLite},
		// jsonl：period_history 是關聯表，沒有 jsonl 實作 ⇒ 明確錯誤（不偷偷寫 sqlite）。
		{declared: "jsonl", wantErr: true},
		{declared: "mysql", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.declared, func(t *testing.T) {
			t.Setenv("ATLAS_STORE_BACKEND", tc.declared)
			got, err := resolveMode(runConfig{workDir: t.TempDir()}, config.Load())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("declared %q must fail loudly, got mode %q", tc.declared, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveMode: %v", err)
			}
			if got != tc.want {
				t.Fatalf("mode = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestResolveMode_ExplicitFlagsWin 顯式 flag 覆寫宣告（-pg ＞ -jsonl ＞ -db）。
// 同時涵蓋「宣告 postgres 但開發者要寫 sqlite」的逃生門。
func TestResolveMode_ExplicitFlagsWin(t *testing.T) {
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	appCfg := config.Load()

	got, err := resolveMode(runConfig{workDir: ".", usePG: true}, appCfg)
	if err != nil || got != modePostgres {
		t.Fatalf("-pg: mode = %q err = %v, want postgres", got, err)
	}
	got, err = resolveMode(runConfig{workDir: ".", jsonl: "data/state"}, appCfg)
	if err != nil || got != modeJSONL {
		t.Fatalf("-jsonl: mode = %q err = %v, want jsonl", got, err)
	}
	got, err = resolveMode(runConfig{workDir: ".", dbPath: "x.db", dbExplicit: true}, appCfg)
	if err != nil || got != modeSQLite {
		t.Fatalf("-db: mode = %q err = %v, want sqlite", got, err)
	}
}

// TestRun_DeclaredPostgresWithoutDSNFailsLoudly 是 #2107 形狀的釘子：
// 宣告 postgres 且沒有 DSN ⇒ 明確錯誤（訊息要能診斷），**不得**落到 sqlite。
func TestRun_DeclaredPostgresWithoutDSNFailsLoudly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "")

	err := run([]string{"-workdir", dir}, io.Discard)
	if err == nil {
		t.Fatal("want error: declared postgres without a DSN must not silently open sqlite")
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

// TestRun_ExplicitDBOverridesDeclaredPostgres -db 顯式覆寫宣告；同時是上面那條
// 釘子的**反假陽性**對照：同一環境下 sqlite 路徑必須真的能用。
func TestRun_ExplicitDBOverridesDeclaredPostgres(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "")

	dbPath := filepath.Join(dir, "explicit.db")
	if err := run([]string{"-workdir", dir, "-db", dbPath}, io.Discard); err != nil {
		t.Fatalf("explicit -db must win over the declared backend: %v", err)
	}
	if _, statErr := os.Stat(dbPath); statErr != nil {
		t.Fatalf("explicit sqlite store must be created at %s: %v", dbPath, statErr)
	}
}

// TestRun_DeclaredJSONLFailsLoudly 宣告 jsonl ⇒ 明確錯誤（改寫 jsonl 檔要用
// -jsonl 顯式要求），且不得建出 sqlite artifact。
func TestRun_DeclaredJSONLFailsLoudly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "jsonl")
	t.Setenv("DATABASE_URL", "")

	err := run([]string{"-workdir", dir}, io.Discard)
	if err == nil {
		t.Fatal("declared jsonl must fail loudly instead of silently writing sqlite")
	}
	if !strings.Contains(err.Error(), "jsonl") {
		t.Errorf("error %q must name the backend and point at -jsonl", err.Error())
	}
	if _, statErr := os.Stat(defaultSQLiteArtifact(dir)); statErr == nil {
		t.Fatalf("declared jsonl must not create the sqlite artifact %s", defaultSQLiteArtifact(dir))
	}
}

// TestRunJSONL_DeclaredPostgresReadsPostgres 生產形狀：宣告 postgres 時 -jsonl
// 模式的 period_history 來源必須是 PG（開池），不是 job-local sqlite。
// 突變（改回 openSQLite）會讓 initCalls 變 0 且建出 sqlite artifact ⇒ 紅。
func TestRunJSONL_DeclaredPostgresReadsPostgres(t *testing.T) {
	dir := t.TempDir()
	jsonlDir := filepath.Join(dir, "data", "state")
	if err := os.MkdirAll(jsonlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "postgres://env@localhost:5432/atlas")
	rec := stubInitPostgresPool(t)

	// 不連線的假池會讓查詢失敗，測試只關心 wiring（有沒有走到 PG）。
	err := run([]string{"-workdir", dir, "-jsonl", "data/state"}, io.Discard)
	if err == nil {
		t.Log("(註) 假池意外讓流程成功；wiring 斷言仍有效")
	}
	if rec.initCalls != 1 {
		t.Fatalf("initPostgresPool called %d times, want 1 (jsonl mode must read period_history from the declared postgres backend)", rec.initCalls)
	}
	if rec.dsn != "postgres://env@localhost:5432/atlas" {
		t.Fatalf("dsn = %q, want $DATABASE_URL", rec.dsn)
	}
	if _, statErr := os.Stat(defaultSQLiteArtifact(dir)); statErr == nil {
		t.Fatalf("-jsonl in a postgres deployment must not create the sqlite artifact %s", defaultSQLiteArtifact(dir))
	}
}

// TestRunJSONL_ExplicitJSONLNeverOpensPool 開發形狀：宣告非 postgres 時 -jsonl
// 只讀 sqlite（既有行為），不得開 pool。
func TestRunJSONL_ExplicitJSONLNeverOpensPool(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "sqlite")
	t.Setenv("DATABASE_URL", "postgres://must-not-be-used@localhost:5432/atlas")
	failInitPostgresPool(t)

	if err := run([]string{"-workdir", dir, "-jsonl", "data/state"}, io.Discard); err != nil {
		t.Fatalf("-jsonl over sqlite must work: %v", err)
	}
}

// TestPeriodHistoryStore_SQLiteNeverOpensPool sqlite 後端不讀 DSN、不開池。
func TestPeriodHistoryStore_SQLiteNeverOpensPool(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "sqlite")
	t.Setenv("DATABASE_URL", "postgres://must-not-be-used@localhost:5432/atlas")
	failInitPostgresPool(t)

	hist, closeFn, err := periodHistoryStore(context.Background(), runConfig{workDir: dir}, config.Load())
	if err != nil {
		t.Fatalf("periodHistoryStore: %v", err)
	}
	defer func() { _ = closeFn() }()
	if _, ok := hist.(*ledger.SQLiteHistoricalStore); !ok {
		t.Fatalf("got %T, want *ledger.SQLiteHistoricalStore", hist)
	}
}
