package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/futures"
)

// 期貨訊號的**影子儲存**（階段 1）。
//
// 為什麼是**獨立命名空間**而不是共用 prediction_backtest：
// 本 repo 的預測校準器（internal/calibration/predictor_calibrator.go）以
// `store.LoadPredictionBacktestRange(ctx, "", "", 90)` 讀取資料，而該方法的 SQL
// （internal/ledger/historical_store.go:797-802）**只以日期與 is_synthetic 過濾，
// 完全不過濾 model_version**，接著把所有列混算成單一命中率。
// ⇒ 任何外來 ModelVersion 的列都會直接改變 `predictor_*` 的校準分數。
// 因此在該前提改變前（見 docs/specs/futures-crossmarket-signal-spec.md §7），
// 影子列一律寫入本檔自己的表，零干擾。
//
// Backend-aware 是硬性要求（#2107）：後端由 ATLAS_STORE_BACKEND / config.StoreBackend
// 經 ResolveStoreBackend 決定；postgres 未注入連線池 ⇒ 回錯誤，**不降級**。
type FuturesShadowStore interface {
	// RecordFuturesShadow upsert 影子列，以 (contract, trade_date, model_version) 為冪等鍵。
	RecordFuturesShadow(ctx context.Context, rows []FuturesShadowRow) (int, error)
	// LoadFuturesShadow 取回 [start, end] 的影子列；modelVersion 為空字串表示不過濾。
	LoadFuturesShadow(ctx context.Context, contract, modelVersion string, start, end time.Time) ([]FuturesShadowRow, error)
}

// FuturesShadowRow 是落庫的影子列：模型版本 ＋ 樣本內容。
//
// 樣本本體沿用 internal/futures.ShadowSample（與記憶體路徑同一形狀），
// 版本標記留在外層，避免把「跑這批資料的模型版本」混進樣本語意。
type FuturesShadowRow struct {
	ModelVersion string               `json:"model_version"`
	Sample       futures.ShadowSample `json:"sample"`
}

// NewFuturesShadowStore 依 cfg.StoreBackend 建立影子儲存。
func NewFuturesShadowStore(cfg config.Config) (FuturesShadowStore, error) {
	backend, err := ResolveStoreBackend(cfg.StoreBackend)
	if err != nil {
		return nil, fmt.Errorf("futures shadow store: %w", err)
	}
	return NewFuturesShadowStoreForBackend(backend, cfg)
}

// NewFuturesShadowStoreForBackend 以已正規化的後端名建立影子儲存。
func NewFuturesShadowStoreForBackend(backend string, cfg config.Config) (FuturesShadowStore, error) {
	resolved, err := ResolveStoreBackend(backend)
	if err != nil {
		return nil, fmt.Errorf("futures shadow store: %w", err)
	}
	switch resolved {
	case "postgres":
		if postgresPool == nil {
			return nil, fmt.Errorf("futures shadow store: postgres backend requires SetPostgresPool before use (refusing to silently fall back)")
		}
		return NewPostgresFuturesShadowStore(postgresPool), nil
	case "sqlite":
		if cfg.SQLitePath == "" {
			return nil, fmt.Errorf("futures shadow store: sqlite backend requires a non-empty SQLitePath")
		}
		return NewSQLiteFuturesShadowStoreFromPath(cfg.SQLitePath)
	case "jsonl":
		return NewJSONLFuturesShadowStore(cfg.LedgerDir), nil
	default:
		return nil, fmt.Errorf("futures shadow store: unexpected backend %q", resolved)
	}
}
