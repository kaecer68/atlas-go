package main

// fix/20260905-task-tz — sblFetchGate / tdccFetchGate 純函數規格對照表。
//
// 驗收核心（parent task 指定）：模擬台北 14:59 / 15:01 —
//   - 台北 (五) 14:59 → sbl 不 fetch（before_15）
//   - 台北 (五) 15:01 → sbl fetch（週五 15:00 收盤後）
// 時間參數直接注入（純函數），不需 mock clock。
//
// 同時覆蓋舊 bug 的回歸防護：UTC 時間軸的 15:00（= 台北 23:00）
// 不得再被誤判為「已過台北 15:00」。

import (
	"testing"
	"time"
)

// mkTaipei 建構某台北 wall-clock 時間（2026-09-04 是週五）。
func mkTaipei(month, day, hour, min int) time.Time {
	return time.Date(2026, time.Month(month), day, hour, min, 0, 0, fixtureTaipei)
}

func TestSBLFetchGate_TableDriven(t *testing.T) {
	cases := []struct {
		name           string
		now            time.Time
		lastFetchDay   string
		deferredDay    string
		wantRun        bool
		wantSkipReason string
	}{
		{
			name:           "fri_14:59_not_yet_close_no_fetch",
			now:            mkTaipei(9, 4, 14, 59),
			lastFetchDay:   "",
			wantRun:        false,
			wantSkipReason: "before_15",
		},
		{
			name:           "fri_15:01_after_close_fetch",
			now:            mkTaipei(9, 4, 15, 1),
			lastFetchDay:   "",
			wantRun:        true,
			wantSkipReason: "",
		},
		{
			name:           "fri_15:00_boundary_fetch",
			now:            mkTaipei(9, 4, 15, 0),
			lastFetchDay:   "",
			wantRun:        true,
			wantSkipReason: "",
		},
		{
			// 語意 sanity：UTC 週五 15:00 = 台北週五 23:00，已過台北
			// 15:00 → 修復後照樣放行（fetch 晚到但合法）。
			name:           "utc_15:00_is_taipei_23:00_still_runs",
			now:            time.Date(2026, time.September, 4, 15, 0, 0, 0, time.UTC),
			lastFetchDay:   "",
			wantRun:        true,
			wantSkipReason: "",
		},
		{
			// 回歐防護（bug 本體）：台北週五 14:59 = UTC 06:59 — 舊 code
			// 用 UTC Hour()<15 在整個「台北 15:00-22:59」窗口（UTC 07:00-
			// 14:59）全部 skip；修復後台北 15:00 整即放行（見 utc_07:00 case）。
			name:           "utc_06:59_is_taipei_14:59_no_fetch",
			now:            time.Date(2026, time.September, 4, 6, 59, 0, 0, time.UTC),
			lastFetchDay:   "",
			wantRun:        false,
			wantSkipReason: "before_15",
		},
		{
			// 台北 15:00 前的 UTC 07:00（= 台北 15:00）→ fetch。
			name:           "utc_07:00_is_taipei_15:00_fetch",
			now:            time.Date(2026, time.September, 4, 7, 0, 0, 0, time.UTC),
			lastFetchDay:   "",
			wantRun:        true,
			wantSkipReason: "",
		},
		{
			name:           "sat_afternoon_weekend_skip",
			now:            mkTaipei(9, 5, 16, 0),
			lastFetchDay:   "",
			wantRun:        false,
			wantSkipReason: "weekend",
		},
		{
			name:           "sun_morning_weekend_skip",
			now:            mkTaipei(9, 6, 9, 0),
			lastFetchDay:   "",
			wantRun:        false,
			wantSkipReason: "weekend",
		},
		{
			name:           "fri_15:30_already_fetched_today",
			now:            mkTaipei(9, 4, 15, 30),
			lastFetchDay:   "2026-09-04",
			wantRun:        false,
			wantSkipReason: "already_fetched_today",
		},
		{
			name:           "mon_15:30_last_fetch_was_fri_runs",
			now:            mkTaipei(9, 7, 15, 30),
			lastFetchDay:   "2026-09-04",
			wantRun:        true,
			wantSkipReason: "",
		},
		{
			name:           "mon_14:59_quota_retry_next_window",
			now:            mkTaipei(9, 7, 14, 59),
			lastFetchDay:   "2026-09-04",
			wantRun:        false,
			wantSkipReason: "before_15",
		},
		// ── fix/20260924-finmind-quota：配額重置後的補抓窗口 ──────────────
		{
			// 週一(9/7)配額耗盡 → 週二台北 08:00（= 00:00Z，配額日邊界）
			// 重置後的第一個 tick 即補抓 9/7 的日報（該日報已於週一傍晚
			// 發布）。這是修復前不存在的行為：舊碼在配額耗盡時把 9/7 標成
			// 「今日已完成」，只能等到 9/8 15:00 的常態時段才可能補回。
			name:           "quota_deferred_yesterday_catch_up_at_reset_boundary",
			now:            mkTaipei(9, 8, 8, 0),
			lastFetchDay:   "2026-09-04",
			deferredDay:    "2026-09-07",
			wantRun:        true,
			wantSkipReason: "",
		},
		{
			// 台北 07:59 尚未到配額日邊界（00:00Z）→ 不得開跑（白打）。
			name:           "quota_deferred_yesterday_before_reset_boundary",
			now:            mkTaipei(9, 8, 7, 59),
			lastFetchDay:   "2026-09-04",
			deferredDay:    "2026-09-07",
			wantRun:        false,
			wantSkipReason: "before_15",
		},
		{
			name:           "quota_deferred_yesterday_catch_up_before_15",
			now:            mkTaipei(9, 8, 9, 30),
			lastFetchDay:   "2026-09-04",
			deferredDay:    "2026-09-07",
			wantRun:        true,
			wantSkipReason: "",
		},
		{
			// 同一天內不得補抓（配額尚未重置，重試只會白打）→ before_15。
			name:           "quota_deferred_today_still_before_15",
			now:            mkTaipei(9, 7, 9, 0),
			lastFetchDay:   "2026-09-04",
			deferredDay:    "2026-09-07",
			wantRun:        false,
			wantSkipReason: "before_15",
		},
		{
			// 補抓窗口只有延後日的隔天：9/9 09:00 已非窗口且未過 15:00。
			name:           "quota_deferred_stale_window_expired",
			now:            mkTaipei(9, 9, 9, 0),
			lastFetchDay:   "2026-09-08",
			deferredDay:    "2026-09-07",
			wantRun:        false,
			wantSkipReason: "before_15",
		},
		{
			// 週末優先於補抓窗口（週五延後 → 週六不抓）。
			name:           "quota_deferred_friday_saturday_weekend_skip",
			now:            mkTaipei(9, 5, 0, 30),
			lastFetchDay:   "2026-09-03",
			deferredDay:    "2026-09-04",
			wantRun:        false,
			wantSkipReason: "weekend",
		},
		{
			// 壞掉的延後日期不得讓 gate 放行（防呆：忽略並走一般規則）。
			name:           "quota_deferred_malformed_ignored",
			now:            mkTaipei(9, 8, 9, 30),
			lastFetchDay:   "2026-09-04",
			deferredDay:    "not-a-date",
			wantRun:        false,
			wantSkipReason: "before_15",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := sblFetchGate(tc.now, tc.lastFetchDay, tc.deferredDay)
			if got != tc.wantRun {
				t.Errorf("sblFetchGate(%v, %q, %q) run = %v, want %v", tc.now, tc.lastFetchDay, tc.deferredDay, got, tc.wantRun)
			}
			if reason != tc.wantSkipReason {
				t.Errorf("sblFetchGate(%v, %q, %q) reason = %q, want %q", tc.now, tc.lastFetchDay, tc.deferredDay, reason, tc.wantSkipReason)
			}
		})
	}
}

// TestSBLQuotaCatchUpAllowed 直接鎖定補抓窗口的邊界（純函數）。
func TestSBLQuotaCatchUpAllowed(t *testing.T) {
	cases := []struct {
		name      string
		now       time.Time
		deferred  string
		wantAllow bool
	}{
		{name: "empty_deferred_never_allows", now: mkTaipei(9, 8, 0, 5), deferred: "", wantAllow: false},
		{name: "next_day_allows", now: mkTaipei(9, 8, 12, 0), deferred: "2026-09-07", wantAllow: true},
		{name: "reset_boundary_allows", now: mkTaipei(9, 8, 8, 0), deferred: "2026-09-07", wantAllow: true},
		{name: "one_minute_before_reset_denies", now: mkTaipei(9, 8, 7, 59), deferred: "2026-09-07", wantAllow: false},
		{name: "same_day_denies", now: mkTaipei(9, 7, 12, 0), deferred: "2026-09-07", wantAllow: false},
		{name: "two_days_later_denies", now: mkTaipei(9, 9, 12, 0), deferred: "2026-09-07", wantAllow: false},
		{name: "malformed_denies", now: mkTaipei(9, 8, 12, 0), deferred: "20260907", wantAllow: false},
		// UTC 時間軸也必須用台北日期/時刻判斷：台北 9/8 08:05 = UTC 9/8 00:05。
		{name: "utc_input_uses_taipei_clock", now: time.Date(2026, time.September, 8, 0, 5, 0, 0, time.UTC), deferred: "2026-09-07", wantAllow: true},
		// 台北 9/8 07:05 = UTC 9/7 23:05 → 尚未重置。
		{name: "utc_input_before_taipei_reset_denies", now: time.Date(2026, time.September, 7, 23, 5, 0, 0, time.UTC), deferred: "2026-09-07", wantAllow: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sblQuotaCatchUpAllowed(tc.now, tc.deferred); got != tc.wantAllow {
				t.Errorf("sblQuotaCatchUpAllowed(%v, %q) = %v, want %v", tc.now, tc.deferred, got, tc.wantAllow)
			}
		})
	}
}

func TestTDCCFetchGate_TableDriven(t *testing.T) {
	cases := []struct {
		name           string
		now            time.Time
		lastFetchDay   string
		wantRun        bool
		wantSkipReason string
	}{
		{
			// 2026-09-08 是週二。10:00 前 skip（舊 bug: 台北 10:00 =
			// UTC 02:00，實際要到台北 18:00 才放行）。
			name:           "tue_09:59_before_10_no_fetch",
			now:            mkTaipei(9, 8, 9, 59),
			lastFetchDay:   "",
			wantRun:        false,
			wantSkipReason: "before_10",
		},
		{
			name:           "tue_10:01_fetch",
			now:            mkTaipei(9, 8, 10, 1),
			lastFetchDay:   "",
			wantRun:        true,
			wantSkipReason: "",
		},
		{
			name:           "tue_10:00_boundary_fetch",
			now:            mkTaipei(9, 8, 10, 0),
			lastFetchDay:   "",
			wantRun:        true,
			wantSkipReason: "",
		},
		{
			name:           "fri_10:00_retry_day_fetch",
			now:            mkTaipei(9, 11, 10, 0),
			lastFetchDay:   "",
			wantRun:        true,
			wantSkipReason: "",
		},
		{
			// 回歐防護：UTC 02:00 = 台北 10:00 → fetch（舊 bug 不會）。
			name:           "utc_02:00_is_taipei_10:00_fetch",
			now:            time.Date(2026, time.September, 8, 2, 0, 0, 0, time.UTC),
			lastFetchDay:   "",
			wantRun:        true,
			wantSkipReason: "",
		},
		{
			name:           "wed_11:00_not_tue_fri_skip",
			now:            mkTaipei(9, 9, 11, 0),
			lastFetchDay:   "",
			wantRun:        false,
			wantSkipReason: "not_tue_fri",
		},
		{
			name:           "sat_11:00_weekend_skip",
			now:            mkTaipei(9, 12, 11, 0),
			lastFetchDay:   "",
			wantRun:        false,
			wantSkipReason: "not_tue_fri",
		},
		{
			name:           "tue_14:00_already_fetched_today",
			now:            mkTaipei(9, 8, 14, 0),
			lastFetchDay:   "2026-09-08",
			wantRun:        false,
			wantSkipReason: "already_fetched_today",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := tdccFetchGate(tc.now, tc.lastFetchDay)
			if got != tc.wantRun {
				t.Errorf("tdccFetchGate(%v, %q) run = %v, want %v", tc.now, tc.lastFetchDay, got, tc.wantRun)
			}
			if reason != tc.wantSkipReason {
				t.Errorf("tdccFetchGate(%v, %q) reason = %q, want %q", tc.now, tc.lastFetchDay, reason, tc.wantSkipReason)
			}
		})
	}
}
