package main

// fix/20260801-govflow-cadence — 純函數 shouldRunGovFlow 與
// classifyGateSkip 的單元測試。
//
// 規格（依實作提示詞 W4）：
//   - 週五 14:59 → false
//   - 週五 15:00 → true
//   - 週六 16:00 → false
//   - 週日 16:00 → false
//   - 當日已成功 → false
//   - 跨日（昨日成功）→ true
//
// classifyGateSkip 對應的 reason 字串也要逐 case 驗證，避免 silent skip
// 復活。

import (
	"testing"
	"time"
)

func TestShouldRunGovFlow_TableDriven(t *testing.T) {
	tz, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Skipf("Asia/Taipei tzdata unavailable: %v", err)
	}

	cases := []struct {
		name            string
		now             time.Time
		lastSuccessDate string
		wantRun         bool
		wantReason      gateSkipReason
	}{
		{
			name:            "Friday 14:59 Taipei → false (before 15:00)",
			now:             time.Date(2026, 7, 17, 14, 59, 0, 0, tz),
			lastSuccessDate: "",
			wantRun:         false,
			wantReason:      gateReasonBefore15,
		},
		{
			name:            "Friday 15:00 Taipei → true (boundary)",
			now:             time.Date(2026, 7, 17, 15, 0, 0, 0, tz),
			lastSuccessDate: "",
			wantRun:         true,
			wantReason:      "",
		},
		{
			name:            "Friday 16:00 Taipei, no prior success → true",
			now:             time.Date(2026, 7, 17, 16, 0, 0, 0, tz),
			lastSuccessDate: "",
			wantRun:         true,
			wantReason:      "",
		},
		{
			name:            "Saturday 16:00 Taipei → false (weekend)",
			now:             time.Date(2026, 7, 18, 16, 0, 0, 0, tz),
			lastSuccessDate: "",
			wantRun:         false,
			wantReason:      gateReasonWeekend,
		},
		{
			name:            "Sunday 16:00 Taipei → false (weekend)",
			now:             time.Date(2026, 7, 19, 16, 0, 0, 0, tz),
			lastSuccessDate: "",
			wantRun:         false,
			wantReason:      gateReasonWeekend,
		},
		{
			name:            "Friday 15:00 Taipei, already succeeded today → false",
			now:             time.Date(2026, 7, 17, 15, 0, 0, 0, tz),
			lastSuccessDate: "2026-07-17",
			wantRun:         false,
			wantReason:      gateReasonAlreadyDoneToday,
		},
		{
			name:            "Friday 16:00 Taipei, already succeeded today → false",
			now:             time.Date(2026, 7, 17, 16, 0, 0, 0, tz),
			lastSuccessDate: "2026-07-17",
			wantRun:         false,
			wantReason:      gateReasonAlreadyDoneToday,
		},
		{
			name:            "Friday 15:00 Taipei, yesterday succeeded → true (cross-day)",
			now:             time.Date(2026, 7, 17, 15, 0, 0, 0, tz),
			lastSuccessDate: "2026-07-16",
			wantRun:         true,
			wantReason:      "",
		},
		{
			name:            "Monday 09:00 Taipei → false (before 15:00, weekend check passed)",
			now:             time.Date(2026, 7, 13, 9, 0, 0, 0, tz),
			lastSuccessDate: "",
			wantRun:         false,
			wantReason:      gateReasonBefore15,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRunGovFlow(tc.now, tc.lastSuccessDate)
			if got != tc.wantRun {
				t.Errorf("shouldRunGovFlow(now=%v, lastSuccessDate=%q) = %v, want %v",
					tc.now, tc.lastSuccessDate, got, tc.wantRun)
			}
			// 同步驗證 reason 分類：wantRun==true 時 reason 必須是空字串
			// （不該被 skip），wantRun==false 時 reason 必須具體。
			gotReason := classifyGateSkip(tc.now, tc.lastSuccessDate)
			if tc.wantRun {
				if gotReason != "" {
					t.Errorf("shouldRun=true but classifyGateSkip returned %q (want empty)", gotReason)
				}
			} else {
				if gotReason == "" {
					t.Errorf("shouldRun=false but classifyGateSkip returned empty (want %q)", tc.wantReason)
				}
				if gotReason != tc.wantReason {
					t.Errorf("classifyGateSkip = %q, want %q", gotReason, tc.wantReason)
				}
			}
		})
	}
}

// TestShouldRunGovFlow_StaggeredIndependenceSanity 驗證 gate 邏輯不耦合
// lastSuccessDate 的「哪個任務」語意：兩個任務各自的 lastSuccessDate
// 互不影響，純函數本來就不該有任何狀態，但明確測一次以防未來 refactor
// 引入 module-level state。
func TestShouldRunGovFlow_StaggeredIndependenceSanity(t *testing.T) {
	tz, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Skipf("Asia/Taipei tzdata unavailable: %v", err)
	}
	now := time.Date(2026, 7, 17, 15, 0, 0, 0, tz)

	// 任務 A 今天已成功；任務 B 沒記錄 → 兩者回傳必須不同。
	if shouldRunGovFlow(now, "2026-07-17") {
		t.Error("task-A-equivalent (lastSuccess=today) should be skipped")
	}
	if !shouldRunGovFlow(now, "") {
		t.Error("task-B-equivalent (lastSuccess=empty) should run")
	}
}

// TestTaipeiDateString 驗證跨時區正確性：UTC 時間 + Asia/Taipei location
// 必須把日期收斂到台北當天。這是「每日一次」守衛的正確性基石。
func TestTaipeiDateString(t *testing.T) {
	tz, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Skipf("Asia/Taipei tzdata unavailable: %v", err)
	}

	// 2026-07-17 23:30 UTC == 2026-07-18 07:30 Taipei → 應回傳 2026-07-18
	utcLateNight := time.Date(2026, 7, 17, 23, 30, 0, 0, time.UTC)
	if got := taipeiDateString(utcLateNight); got != "2026-07-18" {
		t.Errorf("taipeiDateString(UTC 23:30) = %q, want 2026-07-18 (next-day Taipei)", got)
	}

	// 2026-07-17 15:00 UTC == 2026-07-17 23:00 Taipei → 應回傳 2026-07-17
	utcEvening := time.Date(2026, 7, 17, 15, 0, 0, 0, time.UTC)
	if got := taipeiDateString(utcEvening); got != "2026-07-17" {
		t.Errorf("taipeiDateString(UTC 15:00) = %q, want 2026-07-17 (same-day Taipei)", got)
	}

	// 直接在台北時區構造的時間：無轉換，應原樣回傳
	taipeiDirect := time.Date(2026, 7, 17, 16, 0, 0, 0, tz)
	if got := taipeiDateString(taipeiDirect); got != "2026-07-17" {
		t.Errorf("taipeiDateString(tz-direct) = %q, want 2026-07-17", got)
	}
}
