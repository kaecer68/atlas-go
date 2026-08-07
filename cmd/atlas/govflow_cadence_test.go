package main

// feat/20260807-v11-residual-smoke — govflow 純函數 + CAPTCHA recovery
// 鏈路的整合測試。
//
// 涵蓋 §附錄 E 綜合風險段第一條：CAPTCHA 解封後公股欄位復活路徑的
// 驗證閉環。
//
// 對應現實情境：
//   - 2026-08 起 TWSE bsr.twse.com.tw 啟用 CAPTCHA → PR #1421/#1424/#1437
//     加上 CaptchaCooldown 24h 退避、daily-once guard、weekday 15:00+ gate。
//   - 三條門檻的語意正確性原本只有 integration-level smoke
//     (operations_tasks_test.go TestRegisterOperationsTasks_*)
//     沒有純函數級 table-driven 覆蓋。
//   - 本檔補上：(1) shouldRunGovFlow / classifyGateSkip 純函數規格對照
//     table；(2) CaptchaCooldown 在 24h 邊界 + RecoverySuccess 的行為；
//     (3) channel-isolation 防 cross-channel 污染。
//
// 本檔不改 production code — 把既有 gate 行為用 table-driven 測試
// 定型，避免未來重構時靜默迴歸。

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// fixtureTaipei 以 +08:00 FixedZone 作為 production `taipeiLocation()`
// 的 deterministic 副本；不依賴 system tzdata（CI 環境可能缺 tz 資料
// 導致 time.LoadLocation fail → silent 退到 UTC，量產事故）。
//
// fix/20260801-govflow-cadence 規格假設 wall-clock at Asia/Taipei (= +08:00)。
var fixtureTaipei = time.FixedZone("Asia/Taipei", 8*3600)

// =========================================================================
// §1 shouldRunGovFlow / classifyGateSkip 純函數 — 規格對照表
// =========================================================================

// TestShouldRunGovFlow_TableDriven 把 fix/20260801-govflow-cadence 規格的
// 全部 gate 規則對照化為 table：
//
//   - 週末（Sat/Sun）→ false，reason=gate_weekend
//   - weekday 但 Hour < 15 → false，reason=gate_before_15
//   - weekday + Hour >= 15 + lastSuccessDate==今日台北 → false，
//     reason=already_done_today
//   - weekday + Hour >= 15 + lastSuccessDate==空 → true
//   - weekday + Hour >= 15 + lastSuccessDate==昨日 → true（recovery path）
//
// 每個 row 都是純函數 IO（無 I/O、無 global state）；用 FixedZone 不需。
func TestShouldRunGovFlow_TableDriven(t *testing.T) {
	cases := []struct {
		name            string
		now             time.Time
		lastSuccessDate string
		want            bool
		wantSkipReason  gateSkipReason
	}{
		{
			name: "weekday_Monday_pre_15_returns_false",
			now:  time.Date(2026, 8, 3, 9, 30, 0, 0, fixtureTaipei),
			want: false, wantSkipReason: gateReasonBefore15,
		},
		{
			name: "weekday_Monday_exactly_15_returns_true",
			now:  time.Date(2026, 8, 3, 15, 0, 0, 0, fixtureTaipei),
			want: true, wantSkipReason: "",
		},
		{
			name: "weekday_Monday_post_15_returns_true",
			now:  time.Date(2026, 8, 3, 16, 30, 0, 0, fixtureTaipei),
			want: true, wantSkipReason: "",
		},
		{
			name:            "weekday_already_done_today_returns_false",
			now:             time.Date(2026, 8, 3, 17, 30, 0, 0, fixtureTaipei),
			lastSuccessDate: "2026-08-03",
			want:            false, wantSkipReason: gateReasonAlreadyDoneToday,
		},
		{
			name: "weekday_prev_day_success_returns_true_recovery",
			// CAPTCHA 解封翌日場景：昨日已成功，今天尚未跑，
			// gate 應放行 → 真正 fetch 路徑被解開。
			now:             time.Date(2026, 8, 4, 15, 30, 0, 0, fixtureTaipei),
			lastSuccessDate: "2026-08-03",
			want:            true, wantSkipReason: "",
		},
		{
			name: "weekday_first_run_last_empty_returns_true",
			now:  time.Date(2026, 8, 4, 15, 30, 0, 0, fixtureTaipei),
			want: true, wantSkipReason: "",
		},
		{
			name: "Saturday_returns_false",
			now:  time.Date(2026, 8, 8, 16, 0, 0, 0, fixtureTaipei),
			want: false, wantSkipReason: gateReasonWeekend,
		},
		{
			name: "Sunday_returns_false",
			now:  time.Date(2026, 8, 9, 16, 0, 0, 0, fixtureTaipei),
			want: false, wantSkipReason: gateReasonWeekend,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRunGovFlow(tc.now, tc.lastSuccessDate)
			if got != tc.want {
				t.Errorf("shouldRunGovFlow(%v, %q) = %v, want %v",
					tc.now, tc.lastSuccessDate, got, tc.want)
			}
			if tc.want {
				return
			}
			// false 路徑才檢查 reason
			gotReason := classifyGateSkip(tc.now, tc.lastSuccessDate)
			if gotReason != tc.wantSkipReason {
				t.Errorf("classifyGateSkip(%v, %q) = %q, want %q",
					tc.now, tc.lastSuccessDate, gotReason, tc.wantSkipReason)
			}
		})
	}
}

// TestClassifyGateSkip_ShouldNotRun_EmptyReason:
// classifyGateSkip 在 shouldRun==true 時回傳空字串。語意不變性測試。
func TestClassifyGateSkip_ShouldNotRun_EmptyReason(t *testing.T) {
	now := time.Date(2026, 8, 4, 16, 0, 0, 0, fixtureTaipei) // Tuesday 16:00, no last
	if !shouldRunGovFlow(now, "") {
		t.Fatal("pre-condition violated: 16:00 weekday + empty last should be shouldRun=true")
	}
	if r := classifyGateSkip(now, ""); r != "" {
		t.Errorf("shouldRun=true → should not call classify; got %q", r)
	}
}

// =========================================================================
// §2 CaptchaCooldown 與 weekday gate 組合 — CAPTCHA recovery chain
// =========================================================================

// TestCaptchaRecovery_ExitsCooldownAfter24h:
// 模擬真實情境：CAPTCHA 在 T0 觸發，task body 24h 後（cooldown expired）
// 應重新允許 fetch。clock 用 atomic.Int64 注入，不依賴 time.Now。
func TestCaptchaRecovery_ExitsCooldownAfter24h(t *testing.T) {
	var nowUnix atomic.Int64
	start := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC) // Monday noon UTC
	nowUnix.Store(start.UnixNano())
	clock := func() time.Time { return time.Unix(0, nowUnix.Load()) }
	cd := marketdata.CaptchaCooldownWith(24*time.Hour, clock)

	if cd.ShouldSkip("government_broker") {
		t.Fatal("cold start: ShouldSkip must be false")
	}
	cd.RecordCaptcha("government_broker")
	if !cd.ShouldSkip("government_broker") {
		t.Fatal("post-captcha: ShouldSkip must be true")
	}

	// 推 23h59m：仍在 cooldown 內
	nowUnix.Store(start.Add(23*time.Hour + 59*time.Minute).UnixNano())
	if !cd.ShouldSkip("government_broker") {
		t.Fatal("at 23h59m: Still in cooldown — ShouldSkip must be true")
	}

	// 推過 24h：cooldown 過期 → 應放行
	nowUnix.Store(start.Add(24*time.Hour + 1*time.Minute).UnixNano())
	if cd.ShouldSkip("government_broker") {
		t.Fatal("at 24h+1m: cooldown expired — ShouldSkip must be false (CAPTCHA 解封 path)")
	}

	// 驗證 Until 仍在過期前（Until 用 cooldown-start + 24h；現在在 24h+1m，
	// 因此 Until 應 < clock()）
	untilUTC := cd.Until("government_broker").UTC()
	if !untilUTC.Before(clock()) {
		t.Errorf("until %v should be < now %v", untilUTC, clock())
	}
}

// TestCaptchaRecovery_RecordSuccessResetsCooldown:
// 連續 CAPTCHA → 一次 RecordSuccess → cooldown 立即清除，
// 不需等 24h。對應 real 情境：CAPTCHA 解封後第一次成功就解開退避。
func TestCaptchaRecovery_RecordSuccessResetsCooldown(t *testing.T) {
	var nowUnix atomic.Int64
	start := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	nowUnix.Store(start.UnixNano())
	clock := func() time.Time { return time.Unix(0, nowUnix.Load()) }
	cd := marketdata.CaptchaCooldownWith(24*time.Hour, clock)

	cd.RecordCaptcha("government_broker")
	if !cd.ShouldSkip("government_broker") {
		t.Fatal("post-captcha should be true")
	}

	// 1 分鐘後（遠小於 24h）手動清除 → 立即放行
	nowUnix.Store(start.Add(1 * time.Minute).UnixNano())
	cd.RecordSuccess("government_broker")
	if cd.ShouldSkip("government_broker") {
		t.Fatal("post-success: cooldown must clear immediately, even before 24h")
	}
}

// TestCaptchaRecovery_ReCaptureAfterRecovery:
// 解封後若再次撞 CAPTCHA，cooldown 應重新計算（不沿用舊的 until）。
func TestCaptchaRecovery_ReCaptureAfterRecovery(t *testing.T) {
	var nowUnix atomic.Int64
	start := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	nowUnix.Store(start.UnixNano())
	clock := func() time.Time { return time.Unix(0, nowUnix.Load()) }
	cd := marketdata.CaptchaCooldownWith(24*time.Hour, clock)

	cd.RecordCaptcha("government_broker")
	nowUnix.Store(start.Add(30 * time.Minute).UnixNano())
	cd.RecordSuccess("government_broker")
	if cd.ShouldSkip("government_broker") {
		t.Fatal("after 30min + success: ShouldSkip must be false")
	}

	// 第二次撞 captcha — cooldown 重新開始
	nowUnix.Store(start.Add(40 * time.Minute).UnixNano())
	cd.RecordCaptcha("government_broker")
	if !cd.ShouldSkip("government_broker") {
		t.Fatal("re-capture: ShouldSkip must be true")
	}
}

// =========================================================================
// §3 Channel Isolation — captcha cooldown 對其他頻道不影響
// =========================================================================

// TestCaptchaRecovery_ChannelIsolation:
// CAPTCHA 只冷卻對應頻道，不應汙染其他頻道。對應 real 情境：
// government_broker cooldown 期間，另一條 captcha-prone 頻道各自獨立。
func TestCaptchaRecovery_ChannelIsolation(t *testing.T) {
	var nowUnix atomic.Int64
	start := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	nowUnix.Store(start.UnixNano())
	clock := func() time.Time { return time.Unix(0, nowUnix.Load()) }
	cd := marketdata.CaptchaCooldownWith(24*time.Hour, clock)

	cd.RecordCaptcha("government_broker")
	if !cd.ShouldSkip("government_broker") {
		t.Fatal("government_broker in cooldown")
	}
	if cd.ShouldSkip("tw_vol") {
		t.Fatal("tw_vol must NOT inherit government_broker cooldown (channel isolation)")
	}
	if cd.ShouldSkip("any_new_channel") {
		t.Fatal("any-new-channel must NOT inherit prior cooldown")
	}
}
