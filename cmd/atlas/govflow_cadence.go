package main

// fix/20260801-govflow-cadence — 排程餓死修復。
//
// 兩個政府資金背景任務的舊實作把排程錨點釘在開機瞬間的時分秒上（見
// internal/apigateway/background.go:313 time.NewTicker 啟動後立即 executeTask
// + 24h interval），搭配「weekday + 台北 Hour >= 15」閘門，會在以下情境
// 永久被擋：
//
//   - 開機時間 < 15:00（ex: 14:21）→ 24h 後仍 14:21，永遠不過 gate
//   - 週末由 Weekday() 提前擋，但下一個 24h tick 仍落在開機時分秒
//
// 本檔提供：
//   - shouldRunGovFlow 純函數：把 gate + 每日一次邏輯集中，方便單元測試
//   - gateSkipReason 列舉：給 logging.Info 帶明確 reason
//   - taipeiDateString 小工具：把 time.Time 收斂成 Asia/Taipei YYYY-MM-DD
//
// 兩個任務（auto_government_flow + government_flow_aggregate）各自用一份
// in-memory 成功日守衛，互不共用。

import "time"

// gateSkipReason 是 shouldRunGovFlow 的反向語意：當 shouldRun 為 false 時
// 提供「為什麼不跑」的具體 reason，給 logging.Info 帶上，避免 silent skip。
// 每個值對應 production log 中的 event name 後綴。
type gateSkipReason string

const (
	gateReasonWeekend          gateSkipReason = "gate_weekend"
	gateReasonBefore15         gateSkipReason = "gate_before_15"
	gateReasonAlreadyDoneToday gateSkipReason = "already_done_today"
)

// taipeiLocation 回傳 Asia/Taipei *time.Location。tzdata 缺失時 fallback UTC
// （與 operations_tasks.go:currentTaipeiTradingDate 同樣的容忍策略）。
func taipeiLocation() *time.Location {
	if loc, err := time.LoadLocation("Asia/Taipei"); err == nil {
		return loc
	}
	return time.UTC
}

// taipeiDateString 把 time.Time 收斂成 Asia/Taipei 的 YYYY-MM-DD 字串。
// 用於「每日一次」守衛的成功日儲存與比對。
func taipeiDateString(now time.Time) string {
	t := now.In(taipeiLocation())
	return t.Format("2006-01-02")
}

// shouldRunGovFlow 是兩個政府資金任務共用的 gate 純函數。給定「現在」與
// 上一個成功 fetch 的台北日期字串（首次或 process restart 後為空），
// 回傳「現在是否該跑一次 upstream fetch」。
//
// 規則（依 fix/20260801-govflow-cadence 規格）：
//  1. 週末（Sat/Sun）→ false（gate_weekend）
//  2. weekday 但 Hour < 15 → false（gate_before_15）
//  3. lastSuccessDate == 今日台北日期 → false（already_done_today）
//  4. 以上皆否 → true
//
// 純函數：無 I/O、無 global state，方便 table-driven 測試。task body 依
// 回傳結果決定是否呼叫 gateway.Fetch，並在 err==nil 後把今日日期寫入
// 各自的 in-memory lastSuccessDate 守衛。
func shouldRunGovFlow(now time.Time, lastSuccessDate string) bool {
	t := now.In(taipeiLocation())
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false
	}
	if t.Hour() < 15 {
		return false
	}
	if lastSuccessDate != "" && lastSuccessDate == taipeiDateString(t) {
		return false
	}
	return true
}

// classifyGateSkip 把 shouldRunGovFlow == false 的情況翻成具體 reason。
// 給 logging.Info 帶上 event name，確保 silent skip 不再發生。
// 規則順序對齊 shouldRunGovFlow：先週末、再時段、最後已做過。
func classifyGateSkip(now time.Time, lastSuccessDate string) gateSkipReason {
	t := now.In(taipeiLocation())
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return gateReasonWeekend
	}
	if t.Hour() < 15 {
		return gateReasonBefore15
	}
	if lastSuccessDate != "" && lastSuccessDate == taipeiDateString(t) {
		return gateReasonAlreadyDoneToday
	}
	// 理論上 shouldRun==true 時不該呼叫本函式；若被誤呼叫，回傳空字串
	// 表示「不該 skip」給 caller 自行 fallback。
	return ""
}
