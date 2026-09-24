package main

// fix/20260905-task-tz — auto_twse_sbl / auto_tdcc_dispersion 時區閘門修復。
//
// 背景（2026-09-05 實證）：兩個任務的 weekday/Hour gate 用 container 本地
// 時間（production 容器 TZ=UTC），未做 Asia/Taipei 轉換：
//
//   - auto_twse_sbl: `Hour() < 15`（意圖：台北 15:00 收盤後）實際在
//     UTC 15:00 = 台北 23:00 才放行 → SBL 借券賣出餘額每天晚抓 8 小時，
//     且台北 23:23 的執行撞上 FinMind 日額度耗盡（配額日邊界 00:00 UTC =
//     台北 08:00 重置前最後數小時，見 sblQuotaResetHourTaipei 的實證）→
//     通道 warn。修復後台北 15:00+ 即抓。
//   - auto_tdcc_dispersion: `Hour() < 10`（意圖：台北 10:00 後）實際在
//     台北 18:00 才放行。修復後台北 10:00+ 即抓（Tue/Fri）。
//
// 本檔提供（照 govflow_cadence.go / cf_hypothesis_validation_task.go 先例）：
//   - sblFetchGate / tdccFetchGate 純函數：gate + 每日一次邏輯集中，
//     時間參數注入，方便 table-driven 單元測試。
//   - 回傳 skip reason 字串，給 logging.Info 帶上，避免週末/時段
//     silent skip 無 log（對照 cf_hypothesis_validation_skipped 先例）。

import "time"

// sblQuotaResetHourTaipei 是共用 FinMind 日配額「實際」重置的台北時刻。
//
// 實證（2026-09-24 生產）：DailyQuotaTracker 以
// time.Now().Truncate(24*time.Hour)（internal/marketdata/daily_quota.go）在
// **process 本地時區**判定換日，而 production container 的 TZ 未設（= UTC）
// —— 狀態檔可證：`data/state/finmind_daily_quota.json` 的
// `last_reset = "2026-09-24T00:00:00Z"`。因此配額日是
// 00:00Z–24:00Z = 台北 08:00–隔日 08:00。程式碼多處註解寫「00:00 TW 自動
// 重置」是錯的（本檔一併更正）。這個常數讓補抓窗口不會在重置真正發生前
// 就開跑（那只是白打一次必然失敗的請求）。
const sblQuotaResetHourTaipei = 8

// sblQuotaCatchUpAllowed 判斷 `now` 是否落在「FinMind 配額重置後的補抓窗口」。
//
// fix/20260924-finmind-quota：前一日（quotaDeferredDay）的 SBL 日報在前一日
// 傍晚就已發布，而配額在隔日台北 08:00 重置，因此重置後的第一個 tick 就能
// 補到該日資料（provider 會回探最多 sblProbeDays-1 天）。沒有這個窗口時，
// 配額耗盡的那一天會被標記為「今日已完成」，只能等下一次台北 15:00 的常態
// 時段才可能補回（多延 ~7 小時，且期間通道持續 warn）。
func sblQuotaCatchUpAllowed(now time.Time, quotaDeferredDay string) bool {
	if quotaDeferredDay == "" {
		return false
	}
	d, err := time.Parse("2006-01-02", quotaDeferredDay)
	if err != nil {
		return false
	}
	t := now.In(taipeiLocation())
	if taipeiDateString(t) != d.AddDate(0, 0, 1).Format("2006-01-02") {
		return false
	}
	return t.Hour() >= sblQuotaResetHourTaipei
}

// sblFetchGate 判斷 auto_twse_sbl 在 tick `now` 是否該跑一次 fetch。
// 規則：週末 skip；配額重置後的補抓窗口放行（見 sblQuotaCatchUpAllowed）；
// 台北 15:00 前 skip（收盤後資料）；今日台北已抓過 skip。
// 注意：常態窗口（台北 15:00+）與配額日邊界（台北 08:00）不同 —— 參見
// sblQuotaResetHourTaipei 的實證說明。
// lastFetchDay 是「成功抓取」的台北日期，quotaDeferredDay 是「因 FinMind
// 配額耗盡而延後」的台北日期（皆為 in-memory 守衛，"2006-01-02"）。
// 回傳 (是否該跑, skip reason)；reason 僅在不該跑時有意義。
func sblFetchGate(now time.Time, lastFetchDay, quotaDeferredDay string) (bool, string) {
	t := now.In(taipeiLocation())
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false, "weekend"
	}
	if sblQuotaCatchUpAllowed(t, quotaDeferredDay) {
		return true, ""
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
