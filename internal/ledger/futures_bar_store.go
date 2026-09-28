package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// 期貨日 bar 儲存（階段 0；見 docs/specs/futures-bars-firstparty-spec.md §5）。
//
// Backend-aware 是**硬性要求**：後端一律由 ATLAS_STORE_BACKEND / config.StoreBackend
// 經 ResolveStoreBackend 決定。
//
// 反面教材（#2107，2026-09-27）：某 CLI 以「預設 sqlite、Postgres 需明示 flag」的形狀
// 寫入，於是生產（postgres）跑起來時資料靜默寫到本機 sqlite artifact，查詢層看到的是
// 空表。因此本檔：
//   - 不硬編任何 DB 路徑；
//   - 組態為 postgres 但連線池未注入 ⇒ **回錯誤**，絕不降級；
//   - 期貨 bar 寫入專用表 futures_bars，**不**污染 quotes 的 symbol 命名空間。
type FuturesBarStore interface {
	// RecordFuturesBars upsert 一批 bar，回傳寫入（含覆寫）列數。
	// 以 (contract, contract_month, trade_date, session) 為冪等鍵。
	RecordFuturesBars(ctx context.Context, bars []domain.FuturesBar) (int, error)

	// LoadFuturesBars 取回 [start, end] 的 bar。
	// session 為空字串時不過濾時段；contractMonth 為空字串時不過濾到期月。
	LoadFuturesBars(ctx context.Context, contract, contractMonth string, session domain.FuturesSession, start, end time.Time) ([]domain.FuturesBar, error)

	// LoadLatestFuturesBars 回傳每個到期月最新一根 bar（時段不過濾，由呼叫端挑選）。
	LoadLatestFuturesBars(ctx context.Context, contract string) ([]domain.FuturesBar, error)

	// RecordFuturesRollovers upsert 連續契約的 splice 事件。
	//
	// 為什麼要落庫（規格 §6.3）：調整後的連續序列本身**不落庫**（可重算），
	// 但 splice 錨點（換倉日、由哪個月換到哪個月、價差是多少）必須落庫，
	// 否則第三人無法用「原始 bar ＋ 這張表」重算出同一條序列。
	// 以 (contract, roll_date, from_month, to_month) 為冪等鍵。
	RecordFuturesRollovers(ctx context.Context, rollovers []domain.FuturesRollover) (int, error)

	// LoadFuturesRollovers 取回某契約的 splice 事件（依 roll_date 升冪）。
	LoadFuturesRollovers(ctx context.Context, contract string) ([]domain.FuturesRollover, error)
}

// NewFuturesBarStore 依 cfg.StoreBackend 建立後端對應的期貨 bar store。
//
// 分派與 NewFullStore 一致（ResolveStoreBackend 為單一權威）；未知後端或
// postgres 未注入連線池一律回錯誤。
func NewFuturesBarStore(cfg config.Config) (FuturesBarStore, error) {
	backend, err := ResolveStoreBackend(cfg.StoreBackend)
	if err != nil {
		return nil, fmt.Errorf("futures bar store: %w", err)
	}
	return newFuturesBarStoreForBackend(backend, cfg)
}

// NewFuturesBarStoreForBackend 以**已正規化**的後端名建立 store。
// 供 CLI 在使用者已顯式指定後端時直接使用（仍會再驗證一次合法性）。
func NewFuturesBarStoreForBackend(backend string, cfg config.Config) (FuturesBarStore, error) {
	resolved, err := ResolveStoreBackend(backend)
	if err != nil {
		return nil, fmt.Errorf("futures bar store: %w", err)
	}
	return newFuturesBarStoreForBackend(resolved, cfg)
}

func newFuturesBarStoreForBackend(backend string, cfg config.Config) (FuturesBarStore, error) {
	switch backend {
	case "postgres":
		if postgresPool == nil {
			// 不降級：生產若設 postgres 卻沒有 pool，寧可大聲失敗。
			return nil, fmt.Errorf("futures bar store: postgres backend requires SetPostgresPool before use (refusing to silently fall back)")
		}
		return NewPostgresFuturesBarStore(postgresPool), nil
	case "sqlite":
		if cfg.SQLitePath == "" {
			return nil, fmt.Errorf("futures bar store: sqlite backend requires a non-empty SQLitePath")
		}
		return NewSQLiteFuturesBarStoreFromPath(cfg.SQLitePath)
	case "jsonl":
		return NewJSONLFuturesBarStore(cfg.LedgerDir), nil
	default:
		return nil, fmt.Errorf("futures bar store: unexpected backend %q", backend)
	}
}
