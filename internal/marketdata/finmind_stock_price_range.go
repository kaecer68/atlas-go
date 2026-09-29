// Package marketdata — FinMind TaiwanStockPrice **range** fetch.
//
// 為什麼需要這一支（#2126 後續）：本 package 原本只有 **per-day** 的
// GetStockPrice(symbol, date)（finmind_client.go），呼叫端要補一段歷史就得逐日
// 發請求。生產缺口：quotes 表在 2026-06-25 之前只有 114 檔（FinMind 全市場回填
// 自該日起）。要補 2026-03-02→06-24（約 80 個交易日）× 約 1,600 檔 = 約 128k
// 次呼叫（免費速率 6s/req ⇒ 約 64 小時），在配額（每日上限見
// FinMindDailyLimit）與時間上皆不可行。
//
// 正解：TaiwanStockPrice **支援日期區間查詢**——一次呼叫回整段，呼叫次數從
// 「檔數 × 交易日數」降為「檔數 × 1」（約 1,600 次 ⇒ 免費速率約 2.7 小時）。
// range 形式本來就只有 GetStockPrice 的逐日版本在用，其他 provider 早已走
// FetchDatasetRaw 的區間形式（twse_sbl_provider.go、tdcc_provider.go）。
//
// 兩層日期界（缺一不可，皆有測試釘住）：
//  1. 請求本身帶 start_date/end_date（一次呼叫覆蓋整段）。
//  2. 回傳列在客戶端**再過濾一次**：落在 [start, end] 之外的列一律丟棄。
//     第 2 層不是裝飾——本平台已實證 FinMind 有 dataset 會忽略窗口
//     （twse_sbl_provider.go：「full-market query returns only the START date's
//     rows regardless of the window」）。回補是寫入端，寧可少寫也不能把
//     窗口外的列寫進 quotes。
package marketdata

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// stockPriceRangeSource 是 range 抓取在 DailyBar.Source 上放的 provider 身分，
// 與 GetStockPrice 的 quote.Source="finmind" 一致。
//
// 發佈端的來源標籤由呼叫端決定：cmd/backfill-quotes-range 會覆寫成
// "finmind_quotes_range"（比 provider 身分更精確的 producer 標籤，讓事後稽核
// 能分辨某一列是逐日補的、還是區間補的）。本 package 只負責 provider 身分。
const stockPriceRangeSource = "finmind"

// GetStockPriceRange 以**一次** FinMind TaiwanStockPrice 請求取得 symbol 在
// [startDate, endDate]（皆為 YYYY-MM-DD，含頭含尾）之間的全部日線。
//
// 回傳值依日期遞增排序、同日去重（後出現者為準）。symbol 可帶交易所後綴
// （2330.TW）；送去上游的 data_id 由 fetchDataset 統一正規化為裸代碼。
// DailyBar.Symbol 保留呼叫端傳入的原字串（呼叫端決定 quotes 的鍵形式），
// DailyBar.Date 為 **UTC 午夜**——quotes 的三個 store 對日期的格式化方式不同
// （sqlite/jsonl 用 q.Date.Format，postgres 用 q.Date.UTC().Format），只有
// UTC 午夜能讓兩者寫出同一天（見 cmd/backfill-quotes-range 的說明與測試）。
//
// 沒有任何列時回傳 ErrNoDataForSymbol：對一段幾十天的窗口而言，空結果不是
// 「今天還沒收盤」而是「這個代碼在該區間沒有資料」（下市／代碼錯誤），
// 呼叫端據此與傳輸錯誤分流。
func (c *FinMindClient) GetStockPriceRange(ctx context.Context, symbol string, startDate string, endDate string) ([]domain.DailyBar, error) {
	rows, err := c.FetchDatasetRaw(ctx, "TaiwanStockPrice", symbol, startDate, endDate)
	if err != nil {
		return nil, err
	}
	bars, stats := parseStockPriceRange(symbol, startDate, endDate, rows)
	if len(bars) == 0 {
		return nil, fmt.Errorf("finmind: no price data for %s in %s..%s (rows=%d dropped_out_of_window=%d dropped_other_symbol=%d): %w",
			symbol, startDate, endDate, len(rows), stats.outOfWindow, stats.otherSymbol, ErrNoDataForSymbol)
	}
	return bars, nil
}

// stockPriceRangeStats 記錄解析階段丟棄了哪些列（可稽核，不靜默）。
type stockPriceRangeStats struct {
	// outOfWindow：日期落在 [start, end] 之外（上游忽略窗口時的守門）。
	outOfWindow int
	// otherSymbol：列上的 stock_id 與請求的 symbol 不符。
	otherSymbol int
	// unparsable：日期欄缺失或無法解析。
	unparsable int
	// duplicate：同一天重複列（後出現者覆蓋前者）。
	duplicate int
}

// parseStockPriceRange 把 TaiwanStockPrice 的原始列轉成 DailyBar。
//
// 純函式（不碰網路、不碰時鐘）以便直接以 fixture 測解析與過濾規則；
// GetStockPriceRange 只負責呼叫上游後轉交。
func parseStockPriceRange(symbol string, startDate string, endDate string, rows []map[string]any) ([]domain.DailyBar, stockPriceRangeStats) {
	stats := stockPriceRangeStats{}
	wantID := normalizeFinMindStockID(symbol)

	byDate := make(map[string]domain.DailyBar, len(rows))
	for _, row := range rows {
		dateStr := strField(row, "date")
		if dateStr == "" {
			stats.unparsable++
			continue
		}
		// 第 2 層日期界：上游若忽略窗口（見檔頭），窗口外的列在這裡被擋下。
		// YYYY-MM-DD 的字典序比較等於時間序，不需 parse 就能篩。
		if dateStr < startDate || dateStr > endDate {
			stats.outOfWindow++
			continue
		}
		// 逐檔查詢理論上只會回這一檔；仍明確比對一次，避免上游忽略 data_id
		// 時把別的股票的價格寫進這條序列（這種污染不會自己浮現）。
		if rowID := normalizeFinMindStockID(strField(row, "stock_id")); rowID != "" && rowID != wantID {
			stats.otherSymbol++
			continue
		}
		day, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			stats.unparsable++
			continue
		}

		if _, dup := byDate[dateStr]; dup {
			stats.duplicate++
		}
		byDate[dateStr] = domain.DailyBar{
			Date:   day,
			Symbol: symbol,
			Open:   floatField(row, "open"),
			High:   floatField(row, "max"),
			Low:    floatField(row, "min"),
			Close:  floatField(row, "close"),
			Volume: int64(floatField(row, "Trading_Volume")),
			Source: stockPriceRangeSource,
		}
	}

	bars := make([]domain.DailyBar, 0, len(byDate))
	for _, bar := range byDate {
		bars = append(bars, bar)
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Date.Before(bars[j].Date) })
	return bars, stats
}
