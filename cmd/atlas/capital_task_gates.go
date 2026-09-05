package main

// fix/20260905-task-tz — auto_twse_sbl / auto_tdcc_dispersion 時區閘門修復。
//
// 背景（2026-09-05 實證）：兩個任務的 weekday/Hour gate 用 container 本地
// 時間（production 容器 TZ=UTC），未做 Asia/Taipei 轉換：
//
//   - auto_twse_sbl: `Hour() < 15`（意圖：台北 15:00 收盤後）實際在
//     UTC 15:00 = 台北 23:00 才放行 → SBL 借券賣出餘額每天晚抓 8 小時，
//     且台北 23:23 的執行撞上 FinMind 日額度耗盡（00:00 TW 重置前最後
//     一刻）→ 通道 warn。修復後台北 15:00+ 即抓，離額度重置僅 15 小時。
//   - auto_tdcc_dispersion: `Hour() < 10`（意圖：台北 10:00 後）實際在
//     台北 18:00 才放行。修復後台北 10:00+ 即抓（Tue/Fri）。
//
// 本檔提供（照 govflow_cadence.go / cf_hypothesis_validation_task.go 先例）：
//   - sblFetchGate / tdccFetchGate 純函數：gate + 每日一次邏輯集中，
//     時間參數注入，方便 table-driven 單元測試。
//   - 回傳 skip reason 字串，給 logging.Info 帶上，避免週末/時段
//     silent skip 無 log（對照 cf_hypothesis_validation_skipped 先例）。

import "time"

// sblFetchGate 判斷 auto_twse_sbl 在 tick `now` 是否該跑一次 fetch。
// 規則：週末 skip；台北 15:00 前 skip（收盤後資料）；今日台北已抓過 skip。
// lastFetchDay 是 in-memory 每日一次守衛（"2006-01-02" 台北日期）。
// 回傳 (是否該跑, skip reason)；reason 僅在不該跑時有意義。
func sblFetchGate(now time.Time, lastFetchDay string) (bool, string) {
	t := now.In(taipeiLocation())
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false, "weekend"
	}
	if t.Hour() < 15 {
		return false, "before_15"
	}
	if lastFetchDay == taipeiDateString(t) {
		return false, "already_fetched_today"
	}
	return true, ""
}

// tdccFetchGate 判斷 auto_tdcc_dispersion 在 tick `now` 是否該跑一次
// fetch。規則：集保股權分散表為週頻（資料日期週五、次週初發布），
// 只在週二（primary）與週五（retry）的台北 10:00 後各抓一次。
// lastFetchDay 是 in-memory 每日一次守衛（"2006-01-02" 台北日期）。
func tdccFetchGate(now time.Time, lastFetchDay string) (bool, string) {
	t := now.In(taipeiLocation())
	if wd := t.Weekday(); wd != time.Tuesday && wd != time.Friday {
		return false, "not_tue_fri"
	}
	if t.Hour() < 10 {
		return false, "before_10"
	}
	if lastFetchDay == taipeiDateString(t) {
		return false, "already_fetched_today"
	}
	return true, ""
}
