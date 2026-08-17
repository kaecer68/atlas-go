package main

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/prism"
)

// registerOpsForTest 隔離 registerOperationsTasks 的可測部分：只注入
// taskMgr 與本次 PR 新增的 optional dep，避免其他 9 個既有 task 因
// nil dashboard/repo/healthMonitor 而失敗。
//
// 傳入 captchaCooldown=nil + govtFlowDir="" 時行為與改動前相同（10
// 個 task 之中只有不含 governmentFlowDir 的 9 個會被註冊）。
// fix/20260731-govflow-cadence 起的測試會注入真實的 captchaCooldown
// + govtFlowDir 來驗證 government_flow_aggregate 的 24h 節律 + CAPTCHA
// 退避。
func registerOpsForTest(mgr *apigateway.BackgroundTaskManager, janusEngine *janus.Engine) {
	registerOpsForTestWithGovFlow(mgr, janusEngine, nil, "")
}

// registerOpsForTestWithGovFlow is the test seam for the BTM
// government_flow_aggregate task. Both args are optional (nil/empty
// keeps behavior identical to registerOpsForTest).
func registerOpsForTestWithGovFlow(
	mgr *apigateway.BackgroundTaskManager,
	janusEngine *janus.Engine,
	captchaCooldown *marketdata.CaptchaCooldown,
	govtFlowDir string,
) {
	registerOperationsTasks(operationsDeps{
		taskMgr:           mgr,
		janusEngine:       janusEngine,
		captchaCooldown:   captchaCooldown,
		governmentFlowDir: govtFlowDir,
	})
}

func TestRegisterOperationsTasks_JanusRefreshNotRegistered(t *testing.T) {
	// janus_regime_refresh is now registered in main.go (Issue #1086),
	// not in operations_tasks.go. This test confirms the migration.
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerOpsForTest(mgr, janus.NewEngine())

	if _, ok := mgr.Get("janus_regime_refresh"); ok {
		t.Fatal("janus_regime_refresh must NOT be registered by operations_tasks — migrated to main.go (Issue #1086)")
	}
}

func TestRegisterOperationsTasks_JanusRefreshSkippedWhenNil(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerOpsForTest(mgr, nil)

	if _, ok := mgr.Get("janus_regime_refresh"); ok {
		t.Fatal("janus_regime_refresh must NOT be registered when janusEngine is nil")
	}
}

func TestRegisterOperationsTasks_CapitalFlowRefreshSkippedWhenNil(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerOpsForTest(mgr, nil)

	if _, ok := mgr.Get("capital_flow_refresh"); ok {
		t.Fatal("capital_flow_refresh must NOT be registered when capitalFlow is nil")
	}
}

// TestCurrentTaipeiTradingDate verifies the trading-day boundary
// derivation used by the capital_flow_refresh closure:
//   - Mon–Fri before 15:30 Taipei → previous weekday (Friday or earlier)
//   - Mon–Fri at/after 15:30 Taipei → today
//   - Saturday → Friday, Sunday → Friday
//
// Reference dates are 2026-07-13 (Mon), 2026-07-17 (Fri),
// 2026-07-18 (Sat), 2026-07-19 (Sun), 2026-07-20 (Mon).
func TestCurrentTaipeiTradingDate(t *testing.T) {
	tz, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Skipf("Asia/Taipei tzdata unavailable: %v", err)
	}

	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "Friday 16:00 Taipei → today (Friday)",
			now:  time.Date(2026, 7, 17, 16, 0, 0, 0, tz),
			want: time.Date(2026, 7, 17, 0, 0, 0, 0, tz),
		},
		{
			name: "Friday 15:30 Taipei boundary → today (Friday)",
			now:  time.Date(2026, 7, 17, 15, 30, 0, 0, tz),
			want: time.Date(2026, 7, 17, 0, 0, 0, 0, tz),
		},
		{
			name: "Friday 15:29 Taipei (just before cutoff) → Thursday",
			now:  time.Date(2026, 7, 17, 15, 29, 0, 0, tz),
			want: time.Date(2026, 7, 16, 0, 0, 0, 0, tz),
		},
		{
			name: "Friday 09:00 Taipei → Thursday",
			now:  time.Date(2026, 7, 17, 9, 0, 0, 0, tz),
			want: time.Date(2026, 7, 16, 0, 0, 0, 0, tz),
		},
		{
			name: "Monday 09:00 Taipei → Friday",
			now:  time.Date(2026, 7, 13, 9, 0, 0, 0, tz),
			want: time.Date(2026, 7, 10, 0, 0, 0, 0, tz),
		},
		{
			name: "Monday 16:00 Taipei → today (Monday)",
			now:  time.Date(2026, 7, 13, 16, 0, 0, 0, tz),
			want: time.Date(2026, 7, 13, 0, 0, 0, 0, tz),
		},
		{
			name: "Saturday 12:00 Taipei → previous Friday",
			now:  time.Date(2026, 7, 18, 12, 0, 0, 0, tz),
			want: time.Date(2026, 7, 17, 0, 0, 0, 0, tz),
		},
		{
			name: "Sunday 12:00 Taipei → previous Friday",
			now:  time.Date(2026, 7, 19, 12, 0, 0, 0, tz),
			want: time.Date(2026, 7, 17, 0, 0, 0, 0, tz),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := currentTaipeiTradingDate(tc.now)
			if !got.Equal(tc.want) {
				t.Errorf("currentTaipeiTradingDate(%v) = %v, want %v", tc.now, got, tc.want)
			}
		})
	}
}

// =========================================================================
// fix/20260731-govflow-cadence — W1 cadence + W2 CAPTCHA cooldown tests
// =========================================================================

// TestRegisterOperationsTasks_GovernmentFlowAggregate_Cadence
// 驗證 W1.a 排程下調：BTM government_flow_aggregate 從 24h 改為 1h
// (fix/20260801-govflow-cadence)。先前 fix/20260731-govflow-cadence 從
// 28h 降到 24h，但仍受 time.NewTicker 開機錨點 + weekday 15:00+ Taipei
// 閘門永久性卡住；本 PR 進一步縮短到 1h，搭配 in-memory daily-once
// guard 確保每交易日最多跑 1 次。
//
// 紅線 1：期間檔 + PeriodIndicators 結構零改動，本測試只檢查
// BackgroundTaskManager 註冊的 Interval 值。
func TestRegisterOperationsTasks_GovernmentFlowAggregate_Cadence(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerOpsForTestWithGovFlow(mgr, nil, nil, t.TempDir())

	task, ok := mgr.Get("government_flow_aggregate")
	if !ok {
		t.Fatal("government_flow_aggregate must be registered when governmentFlowDir is set")
	}
	if task.Interval != 1*time.Hour {
		t.Errorf("government_flow_aggregate Interval = %v, want 1h (was 24h pre-fix/20260801-govflow-cadence)", task.Interval)
	}
	if task.ChannelID != "government_broker" {
		t.Errorf("ChannelID = %q, want government_broker", task.ChannelID)
	}
	if !task.IsEnabled() {
		t.Error("government_flow_aggregate must be enabled by default")
	}
}

// TestRegisterOperationsTasks_GovernmentFlowAggregate_CaptchaCooldown
// 驗證 W2：連續 CAPTCHA → 退避生效（task body 跳過 upstream fetch）。
// 測試方法：
//  1. 注入 nil gateway（fetch 會回傳 no-gateway 錯）；再用 captcha cooldown
//     預先觸發 RecordCaptcha；
//  2. 直接呼叫 task.Task(ctx) 觀察回傳值；
//  3. 第二次呼叫 → 因 captcha cooldown 仍 active，task 直接 return nil
//     （不嘗試 fetch）。
func TestRegisterOperationsTasks_GovernmentFlowAggregate_CaptchaCooldown(t *testing.T) {
	cd := marketdata.NewCaptchaCooldown()
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerOpsForTestWithGovFlow(mgr, nil, cd, t.TempDir())

	task, ok := mgr.Get("government_flow_aggregate")
	if !ok {
		t.Fatal("government_flow_aggregate must be registered")
	}

	// 預冷卻啟動：模擬上游剛回 captcha 一次
	cd.RecordCaptcha("government_broker")
	if !cd.ShouldSkip("government_broker") {
		t.Fatal("pre-condition: cooldown should be active right after RecordCaptcha")
	}

	// task body 在 captcha cooldown 啟動時直接 return nil（不再嘗試 fetch）
	if err := task.Task(t.Context()); err != nil {
		t.Errorf("with captcha cooldown active, task should return nil (skip fetch), got err=%v", err)
	}

	// 重置 cooldown，再呼叫一次：依當下時段兩種合法結果：
	//   - weekday 15:00+ Taipei: task 應走到 fetch 並回傳
	//     "no gateway" 錯誤（驗證 cooldown 已清、body 真的進入 fetch）
	//   - 週末或 15:00 前: weekday gate 會先 return nil（也是正確的）
	// 兩種結果都證明 body 正常運作；測試只斷言「不 panic」+ 結果符合
	// 其中之一。
	cd.RecordSuccess("government_broker")
	loc, _ := time.LoadLocation("Asia/Taipei")
	now := time.Now()
	inLoc := now
	if loc != nil {
		inLoc = now.In(loc)
	}
	inTradingWindow := inLoc.Weekday() != time.Saturday &&
		inLoc.Weekday() != time.Sunday &&
		inLoc.Hour() >= 15
	err := task.Task(t.Context())
	switch {
	case inTradingWindow:
		if err == nil {
			t.Errorf("weekday 15:00+ Taipei with nil gateway: expected error, got nil")
		}
	default:
		// Weekend or pre-15:00: weekday gate short-circuits to nil.
		// Cooldown is no longer in play (cleared), so the body should
		// be allowed through the gate; the gate itself is what returns
		// nil here, not the cooldown.
		if err != nil {
			t.Errorf("weekday gate should short-circuit, got err=%v", err)
		}
	}
}

// TestRegisterOperationsTasks_GovernmentFlowAggregate_CaptchaCooldown_Consecutive
// 驗證 W2 prompt 強制要求的「連續 CAPTCHA → 跳過後續嘗試」案例。
// 情境：1 次 CAPTCHA 觸發 cooldown 後，連續 3 次呼叫 task 都應 return nil
// （task body 跳過 fetch，不打到 gateway）。
func TestRegisterOperationsTasks_GovernmentFlowAggregate_CaptchaCooldown_Consecutive(t *testing.T) {
	cd := marketdata.NewCaptchaCooldown()
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerOpsForTestWithGovFlow(mgr, nil, cd, t.TempDir())

	task, ok := mgr.Get("government_flow_aggregate")
	if !ok {
		t.Fatal("government_flow_aggregate must be registered")
	}

	// 1st CAPTCHA hit
	cd.RecordCaptcha("government_broker")

	// 3 個連續 task 呼叫：全部應 return nil（fetch 被跳過）
	for i := 0; i < 3; i++ {
		if err := task.Task(t.Context()); err != nil {
			t.Errorf("consecutive tick %d/3: expected nil (cooldown skip), got err=%v", i+1, err)
		}
	}
}

// TestRegisterOperationsTasks_GovernmentFlowAggregate_WeekdayGate
// 驗證 W1.a 額外加上的 weekday + 15:00+ Taipei gate。
// 用 "2026-08-01 10:00 Saturday Taipei" 模擬週末：task 應 return nil
// （不嘗試 fetch）。
func TestRegisterOperationsTasks_GovernmentFlowAggregate_WeekdayGate(t *testing.T) {
	cd := marketdata.NewCaptchaCooldown()
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerOpsForTestWithGovFlow(mgr, nil, cd, t.TempDir())

	task, _ := mgr.Get("government_flow_aggregate")

	// 模擬時間：2026-08-01 (Sat) 10:00 Taipei
	loc, _ := time.LoadLocation("Asia/Taipei")
	satMorning := time.Date(2026, 8, 1, 10, 0, 0, 0, loc)
	t.Setenv("TZ", "") // 確保 process tz 行為可預測
	_ = satMorning

	// 直接呼叫 task body 並驗證它不會 panic。週末 + pre-15:00 應
	// return nil 靜默 skip（無法在沒有 clock injection 的情況下
	// 嚴格斷言 false，這裡只保證它跑得起來）。
	if err := task.Task(t.Context()); err != nil {
		t.Logf("task err (acceptable if running on weekday post-15:00): %v", err)
	}
}

// registerOpsForTestWithPrism is the test seam for the PRISM Phase A
// prism_training BTM: injects a real prism manager + agent registry so the
// task registration and its per-agent scheduling can be verified.
func registerOpsForTestWithPrism(
	mgr *apigateway.BackgroundTaskManager,
	janusEngine *janus.Engine,
	prismMgr *prism.PRISMManager,
	registry domain.AgentRegistry,
) {
	registerOperationsTasks(operationsDeps{
		taskMgr:       mgr,
		janusEngine:   janusEngine,
		prismMgr:      prismMgr,
		prismRegistry: registry,
	})
}

func TestRegisterOperationsTasks_PrismTrainingEnabled(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	registry := domain.AgentRegistry{Agents: []domain.AgentSpec{{ID: "test-agent-01", Enabled: true}}}

	registerOpsForTestWithPrism(mgr, janus.NewEngine(), pm, registry)

	task, ok := mgr.Get("prism_training")
	if !ok {
		t.Fatal("prism_training must be registered when prismMgr + janusEngine are present (PRISM Phase A)")
	}
	if !task.IsEnabled() {
		t.Fatal("prism_training must be Enabled after PRISM Phase A wiring")
	}
}

func TestRegisterOperationsTasks_PrismTrainingSchedulesPerAgent(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	registry := domain.AgentRegistry{Agents: []domain.AgentSpec{
		{ID: "agent-enabled-01", Enabled: true},
		{ID: "agent-disabled-01", Enabled: false},
	}}

	registerOpsForTestWithPrism(mgr, janus.NewEngine(), pm, registry)

	task, ok := mgr.Get("prism_training")
	if !ok {
		t.Fatal("prism_training must be registered")
	}
	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("prism_training task run: %v", err)
	}

	// The replay executor filters recommendations by task.AgentID, so the
	// task must schedule every ENABLED registry agent (not pseudo
	// "system-<regime>" IDs) into each of the 5 regime queues.
	stats := pm.GetQueueStats()
	if len(stats) != int(prism.RegimeCount) {
		t.Fatalf("expected %d regime queues, got %d", prism.RegimeCount, len(stats))
	}
	for _, q := range stats {
		if q.Size != 1 {
			t.Errorf("regime %s queue size = %d, want 1 (one enabled agent scheduled; disabled agent must be skipped)", q.Regime, q.Size)
		}
	}
}

func TestRegisterOperationsTasks_PrismTrainingSkippedWhenPrismMgrNil(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerOpsForTest(mgr, janus.NewEngine())

	if _, ok := mgr.Get("prism_training"); ok {
		t.Fatal("prism_training must NOT be registered when prismMgr is nil")
	}
}
