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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := sblFetchGate(tc.now, tc.lastFetchDay)
			if got != tc.wantRun {
				t.Errorf("sblFetchGate(%v, %q) run = %v, want %v", tc.now, tc.lastFetchDay, got, tc.wantRun)
			}
			if reason != tc.wantSkipReason {
				t.Errorf("sblFetchGate(%v, %q) reason = %q, want %q", tc.now, tc.lastFetchDay, reason, tc.wantSkipReason)
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
