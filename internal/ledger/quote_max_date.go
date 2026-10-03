package ledger

import (
	"context"
	"time"
)

// QuoteMaxDater is an optional interface for QuoteStore implementations that
// can answer "what is the newest quote date in the store". It is intentionally
// separate from QuoteStore (same pattern as QuoteSymbolLister) so existing
// implementations and callers are not broken by the addition.
//
// 為什麼存在（F54 phase 1，2026-10-03）：quotes 沒有排程結構性保障
// （2026-10-02 實損：universe build 只有 2/117 有 quotes），需要
// `select max(date)` 的新鮮度觀測。只有關聯式後端（postgres/sqlite）能
// 高效回答；JSONL 後端不實作（呼叫端以型別斷言優雅跳過）。
type QuoteMaxDater interface {
	// MaxQuoteDate 回傳 quotes 表中最新的交易日（date 欄位最大值）。
	// 表為空時回傳 zero time 與 nil error（「空表」是**可觀測的狀態**，
	// 由 monitoring 側判成不新鮮，而不是查詢失敗）。
	MaxQuoteDate(ctx context.Context) (time.Time, error)
}
