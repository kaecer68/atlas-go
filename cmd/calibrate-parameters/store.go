// store.go — 決定 calibrate-parameters 這次執行要用哪一個 ledger outcome store。
//
// 為什麼要有這個檔案（2026-10-16，Q2b2 ledger seam 站點 3）
// ----------------------------------------------------------
// darwinian 校準原本在 internal/calibration 內用 <cwd>/data/state 直建一個
// JSONL-only store。生產是 PG-primary：session 寫入走 PostgreSQL，那份 flat
// ledger（JSONL）只是 session_summary_reconcile 每 12 小時由 PG 回填的**鏡射**，
// 最多滯後 12 小時 ⇒ 每 7 天的 auto_calibrate 讀的是「過期的鏡射」而不是真相來源。
// 校準端因此接受一個注入的 store，本檔負責決定那個 store 用哪個後端。
//
// 失敗政策：**設定錯誤不得讓週任務失敗**。解析或開池的錯誤一律記一則 WARN 並降級成
// nil store —— 校準端於是讀同一份 flat JSONL 鏡射（與遷移前行為相同；該鏡射由
// session_summary_reconcile 持續維持）。降級絕不靜默（WARN 一定帶原因）。
//
// 後端解析只有一條路徑（ledger.ResolveStoreBackend）：未知值直接回錯誤。
package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	atlasdb "github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
)

const (
	// migrationsRelDir 是相對 workDir 的 migration 目錄（與
	// cmd/backfill-quotes-range、cmd/backfill-futures-bars 同慣例）。
	migrationsRelDir = "sql/migrations"
	// ledgerMirrorRelDir 是 darwinian 校準實際讀取的 flat ledger 鏡射目錄
	// （<workDir>/data/state）。注入的 store 必須指向**同一個**目錄，
	// 才能與 legacy mirror 對得上。
	ledgerMirrorRelDir = "data/state"
)

// storeDeps 是 store wiring 的可注入依賴：生產走真 atlasdb.Init ＋
// ledger.SetPostgresPool ＋ ledger.NewOutcomeStore；單元測試替換成 fake，
// 因此**測試永遠不會真的連上 PostgreSQL**（同 cmd/backfill-quotes-range
// 的 storeDeps 慣例）。
type storeDeps struct {
	initPool   func(ctx context.Context, dsn, migrationsPath string) (*pgxpool.Pool, error)
	injectPool func(pool *pgxpool.Pool)
	newStore   func(cfg config.Config) (ledger.OutcomeStore, error)
}

// defaultStoreDeps 是生產 wiring。
func defaultStoreDeps() storeDeps {
	return storeDeps{
		initPool:   atlasdb.Init,
		injectPool: ledger.SetPostgresPool,
		newStore:   ledger.NewOutcomeStore,
	}
}

// moduleNeedsLedgerStore 回報這個模組是否會讀 ledger。
//
// 只有 darwinian 讀（garch/var/factor 純由 replay returns 推算）⇒ 其餘模組
// 完全不碰 DSN、不開池（--module=garch|var|factor 不該付這筆連線成本）。
func moduleNeedsLedgerStore(module string) bool {
	return module == "darwinian" || module == "all"
}

// calibrationStoreConfig 由 appCfg 推導 store 設定。
//
// LedgerDir 一律釘在 <workDir>/data/state —— 也就是校準端 legacy mirror 的同一
// 目錄 —— 讓「注入的 store」與「鏡射」指到同一份檔案。其餘欄位（StoreBackend／
// SQLitePath／…）沿用 appCfg，即 ATLAS_STORE_BACKEND 等環境設定。
func calibrationStoreConfig(appCfg config.Config, workDir string) config.Config {
	storeCfg := appCfg
	storeCfg.LedgerDir = filepath.Join(workDir, ledgerMirrorRelDir)
	return storeCfg
}

// resolveCalibrationStore 建立這次執行要用的 store。它**不降級**：未知後端或缺
// DSN 都是明確錯誤，由呼叫端決定怎麼辦（calibrationStoreForRun 會降級成 nil）。
//
// 不需要 ledger 的模組回 (nil, no-op, nil)。
func resolveCalibrationStore(ctx context.Context, module, workDir string, appCfg config.Config, deps storeDeps) (ledger.OutcomeStore, func(), error) {
	noop := func() {}

	if !moduleNeedsLedgerStore(module) {
		return nil, noop, nil
	}

	backend, err := ledger.ResolveStoreBackend(appCfg.StoreBackend)
	if err != nil {
		return nil, noop, err
	}

	storeCfg := calibrationStoreConfig(appCfg, workDir)
	storeCfg.StoreBackend = backend

	if backend != "postgres" {
		// jsonl/sqlite：完全不碰 DSN。
		store, err := deps.newStore(storeCfg)
		if err != nil {
			return nil, noop, err
		}
		return store, noop, nil
	}

	dsn := appCfg.DatabaseURL
	if dsn == "" {
		// 明確可診斷：說清楚是哪個後端、要補哪個環境變數；不靜默退回 jsonl。
		return nil, noop, fmt.Errorf(
			"calibration outcome store: backend %q requires a PostgreSQL DSN: set DATABASE_URL (refusing to fall back to the JSONL mirror)",
			backend)
	}

	migrationsPath := appCfg.MigrationsPath
	if migrationsPath == "" {
		migrationsPath = migrationsRelDir
	}
	if !filepath.IsAbs(migrationsPath) {
		migrationsPath = filepath.Join(workDir, migrationsPath)
	}

	pool, err := deps.initPool(ctx, dsn, migrationsPath)
	if err != nil {
		return nil, noop, fmt.Errorf("connect postgres: %w", err)
	}

	deps.injectPool(pool)

	store, err := deps.newStore(storeCfg)
	if err != nil {
		pool.Close()
		return nil, noop, err
	}
	return store, pool.Close, nil
}

// calibrationStoreForRun 是 run 層的包裝：解析或開池失敗**不得**讓（週）校準任務
// 失敗，因此記一則含原因的 WARN 並降級成 nil store。校準端於是走同一條 legacy
// 路徑讀 flat JSONL 鏡射 —— 該鏡射由 session_summary_reconcile 從 PostgreSQL
// 回填維持（本函式不碰任何一行 ledger 程式碼）。
func calibrationStoreForRun(ctx context.Context, module, workDir string, appCfg config.Config, deps storeDeps) (ledger.OutcomeStore, func()) {
	store, closer, err := resolveCalibrationStore(ctx, module, workDir, appCfg, deps)
	if err != nil {
		logging.Warn("calibration", "outcome_store_unavailable_falling_back_to_jsonl_mirror",
			logging.Err(err),
			logging.FStr("module", module),
			logging.FStr("store_backend", appCfg.StoreBackend),
			logging.FStr("fallback_ledger_dir", filepath.Join(workDir, ledgerMirrorRelDir)))
		return nil, func() {}
	}
	return store, closer
}
