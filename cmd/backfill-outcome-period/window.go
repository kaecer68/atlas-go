package main

// 交易日窗口（#2124）。
//
// 這條 CLI 的 UPDATE 原本沒有日期界：任何 market_period IS NULL 且當天有
// period_history 的列都會被填 —— 操作者以為「只動某個窗口」，實際上可能一次改寫
// 整張表（歷史回填列尤其多）。本檔提供三件事：
//
//	validateWindow   : flag 格式與順序檢查（YYYY-MM-DD，含端點）
//	inWindow         : 純函式，供 jsonl 模式逐列過濾
//	guardLargeUpdate : 影響列數上限；超過且沒有 -force ⇒ 中止（必須在 UPDATE 之前）

import (
	"fmt"
	"time"
)

// validateWindow 檢查 -start／-end。空字串代表不設界（沿用舊行為）。
func validateWindow(start, end string) error {
	for _, s := range []string{start, end} {
		if s == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return fmt.Errorf("invalid date %q (want YYYY-MM-DD): %w", s, err)
		}
	}
	if start != "" && end != "" && start > end {
		return fmt.Errorf("-start %s is after -end %s", start, end)
	}
	return nil
}

// inWindow 回報交易日 date 是否落在 [start, end] 內。空字串端點不設界。
// YYYY-MM-DD 的字典序等於時間序，故直接字串比較。
func inWindow(start, end, date string) bool {
	if start != "" && date < start {
		return false
	}
	if end != "" && date > end {
		return false
	}
	return true
}

// windowLabel 是輸出用的窗口標示（空字串端點寫成開放區間）。
func windowLabel(start, end string) string {
	switch {
	case start == "" && end == "":
		return "(whole table)"
	case start == "":
		return ".." + end
	case end == "":
		return start + ".."
	default:
		return start + ".." + end
	}
}

// guardLargeUpdate 是「超門檻需明示」的護欄：影響列數超過 maxUnattendedRows
// 且未帶 -force 時回錯誤。
//
// 呼叫端**必須**在 UPDATE 之前呼叫（否則就不是防護，而是事後報告）。
func guardLargeUpdate(res backfillResult, force bool) error {
	if force || res.Matched <= maxUnattendedRows {
		return nil
	}
	return fmt.Errorf("refusing to update %d rows without -force (limit %d per run): narrow the window with -start/-end, or pass -force to accept it",
		res.Matched, maxUnattendedRows)
}
