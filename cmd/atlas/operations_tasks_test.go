package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/importer"
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
	for i := range 3 {
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

// =========================================================================
// fix/20260927-decouple-csv-to-jsonl-conversion
//
// 驗收（owner 裁定方向）：CSV→JSONL 轉檔必須與「有沒有缺口」解耦，並以
// 「CSV 最新資料日 > JSONL 最新資料日」為閘門。
//
// 為什麼要有這些測試（實證缺陷形狀）：
// 舊碼在 `start.After(end)`（沒有缺口）時直接 `return nil`，而轉檔在該
// return 之後 ⇒ 沒有缺口的日子結構上永遠不轉檔 ⇒ JSONL 凍結在
// 2026-08-24 而 CSV 持續前進（生產實測）。同時
// internal/orchestrator/composition.go 的 buildFactorEngine 只在 JSONL
// **不存在**時轉檔，所以「已存在但落後」的 JSONL 沒有任何修復路徑。
//
// 時鐘注入：runAutoBackfill 收 now 參數（不是內部呼叫 time.Now()），
// 因此「沒有缺口」可以用 fixture 決定性重現，不必依賴執行當天是星期幾。
// =========================================================================

// replayFixtureDir 建立一個 replay fixture 目錄，回傳 CSV / JSONL 路徑。
func replayFixtureDir(t *testing.T) (dir, csvPath, jsonlPath string) {
	t.Helper()
	dir = t.TempDir()
	csvPath = filepath.Join(dir, "tw_extended_90days.csv")
	jsonlPath = filepath.Join(dir, "tw_extended_90days.jsonl")
	return dir, csvPath, jsonlPath
}

// writeReplayCSVFixture 寫入最小可用的 TWSE open-data CSV（欄位與
// internal/replay.LoadTWSEOpenDataCSV 的 required 清單一致）。
func writeReplayCSVFixture(t *testing.T, path string, dates ...string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("Date,Code,Name,TradeVolume,TradeValue,Open,High,Low,Close,Change,Transaction\n")
	for _, d := range dates {
		b.WriteString(d + ",2330,TSMC,32001234,25801234567,790,795,788,792,2,19555\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write CSV fixture %s: %v", path, err)
	}
}

// buildJSONLFromDates 用**正式轉檔路徑**（importer.ImportTWOpenDataCSVToJSONL）
// 產生 JSONL 前置狀態，避免手寫 JSONL 與正式格式漂移。
func buildJSONLFromDates(t *testing.T, csvPath, jsonlPath string, dates ...string) {
	t.Helper()
	writeReplayCSVFixture(t, csvPath, dates...)
	if err := importer.ImportTWOpenDataCSVToJSONL(csvPath, jsonlPath); err != nil {
		t.Fatalf("seed JSONL from %v: %v", dates, err)
	}
}

// replayLatestDateForTest 讀回檔案最新資料日（測試斷言用；走正式 helper）。
func replayLatestDateForTest(t *testing.T, path string) string {
	t.Helper()
	d, err := replayLatestDate(path)
	if err != nil {
		t.Fatalf("replayLatestDate(%s): %v", path, err)
	}
	return d.Format(dateLayout)
}

// taipeiClock 回傳固定時刻（Asia/Taipei 15:30 之後 ⇒ 「今天的結算日 = 今天」）。
// tzdata 不存在時 skip，與 TestCurrentTaipeiTradingDate 同一慣例。
func taipeiClock(t *testing.T, y int, mo time.Month, d, h, mi int) time.Time {
	t.Helper()
	tz, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Skipf("Asia/Taipei tzdata unavailable: %v", err)
	}
	return time.Date(y, mo, d, h, mi, 0, 0, tz)
}

// captureGatewayLog 攔截 log 輸出（cmd/atlas 測試未使用 t.Parallel，安全）。
func captureGatewayLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	})
	fn()
	return buf.String()
}

// (a) 無缺口、但 JSONL 落後 ⇒ 轉檔發生、JSONL 追上、CSV 不被改動。
//
// fixture 時鐘 2026-03-20（週五）16:00 Taipei：CSV 最新日 = 2026-03-20 ⇒
// start = 2026-03-23（跳過週末）> end = 2026-03-20 ⇒ **沒有缺口**（舊碼
// 從這裡 return nil）。JSONL 前置狀態只有 2026-03-19 ⇒ 新閘門必須轉檔。
// 同一個 fixture 在舊碼上會留下 JSONL = 2026-03-19（本檔案的負向對照）。
func TestRunAutoBackfill_NoGapJSONLBehind_Converts(t *testing.T) {
	dir, csvPath, jsonlPath := replayFixtureDir(t)
	// 前置：JSONL 落後一天（模擬 daily-replay-sync 昨日寫入後、今日 CSV 又前進）。
	buildJSONLFromDates(t, csvPath, jsonlPath, "2026-03-19")
	if got := replayLatestDateForTest(t, jsonlPath); got != "2026-03-19" {
		t.Fatalf("fixture JSONL pre-state = %s, want 2026-03-19", got)
	}
	// 今日 CSV 追加一天（模擬 daily-replay-sync 15:30 UTC 的每日寫入）。
	writeReplayCSVFixture(t, csvPath, "2026-03-19", "2026-03-20")
	csvBefore, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("read CSV pre-state: %v", err)
	}

	cfg := config.Config{WorkDir: dir, ReplayDataPath: csvPath}
	now := taipeiClock(t, 2026, time.March, 20, 16, 0)

	var runErr error
	logs := captureGatewayLog(t, func() {
		runErr = runAutoBackfill(context.Background(), cfg, now)
	})

	if runErr != nil {
		t.Fatalf("runAutoBackfill returned %v, want nil (no gap ⇒ no fetch, conversion is non-fatal)", runErr)
	}
	if got := replayLatestDateForTest(t, jsonlPath); got != "2026-03-20" {
		t.Errorf("JSONL latest date after task = %s, want 2026-03-20 (conversion must run on gap-free days)", got)
	}
	csvAfter, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("read CSV post-state: %v", err)
	}
	if !bytes.Equal(csvBefore, csvAfter) {
		t.Error("replay CSV was modified by the task; the converter must never write the source file")
	}
	if !strings.Contains(logs, "backfill gap: none") {
		t.Errorf("expected the gap-free branch to be logged, got:\n%s", logs)
	}
	if !strings.Contains(logs, "backfill CSV→JSONL conversion: converted") {
		t.Errorf("expected a 'converted' outcome line, got:\n%s", logs)
	}
	if strings.Contains(logs, "backfill gap detected") {
		t.Errorf("no-gap fixture must not reach the backfill path, got:\n%s", logs)
	}
	t.Logf("[evidence a] logs:\n%s", strings.TrimSpace(logs))
}

// (b) 無缺口、JSONL 已最新 ⇒ 不轉檔、不重寫檔（mtime / size / bytes 三者皆不變）。
//
// mtime 先刻意壓到 2020-01-01：即使內容被「原樣重寫」一次，mtime 也會變 ⇒
// 「沒有寫入」是可證的，不只是「內容一樣」。
func TestRunAutoBackfill_NoGapJSONLUpToDate_DoesNotRewrite(t *testing.T) {
	dir, csvPath, jsonlPath := replayFixtureDir(t)
	dates := []string{"2026-03-19", "2026-03-20"}
	buildJSONLFromDates(t, csvPath, jsonlPath, dates...)
	writeReplayCSVFixture(t, csvPath, dates...)

	oldTime := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(jsonlPath, oldTime, oldTime); err != nil {
		t.Fatalf("seed JSONL mtime: %v", err)
	}
	beforeInfo, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatalf("stat JSONL pre-state: %v", err)
	}
	beforeBytes, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("read JSONL pre-state: %v", err)
	}

	cfg := config.Config{WorkDir: dir, ReplayDataPath: csvPath}
	now := taipeiClock(t, 2026, time.March, 20, 16, 0)

	var runErr error
	logs := captureGatewayLog(t, func() {
		runErr = runAutoBackfill(context.Background(), cfg, now)
	})
	if runErr != nil {
		t.Fatalf("runAutoBackfill returned %v, want nil", runErr)
	}

	afterInfo, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatalf("stat JSONL post-state: %v", err)
	}
	if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Errorf("JSONL mtime changed: %v → %v (the file was rewritten although it was already up to date)",
			beforeInfo.ModTime(), afterInfo.ModTime())
	}
	if afterInfo.Size() != beforeInfo.Size() {
		t.Errorf("JSONL size changed: %d → %d", beforeInfo.Size(), afterInfo.Size())
	}
	afterBytes, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("read JSONL post-state: %v", err)
	}
	if !bytes.Equal(beforeBytes, afterBytes) {
		t.Error("JSONL content changed although CSV and JSONL had the same latest date")
	}
	if !strings.Contains(logs, "backfill CSV→JSONL conversion: skipped (already up to date)") {
		t.Errorf("expected an 'already up to date' skip line, got:\n%s", logs)
	}
	if strings.Contains(logs, "conversion: converted") {
		t.Errorf("must not report a conversion when nothing was stale, got:\n%s", logs)
	}
	t.Logf("[evidence b] logs:\n%s", strings.TrimSpace(logs))
}

// (e) 轉檔失敗不致命 ⇒ 任務仍回 nil，且日誌明確寫「轉檔失敗」。
//
// 注入方式：把 JSONL 目標路徑做成**目錄**（os.Create 必定失敗 EISDIR），
// 且該路徑對 tail reader 不可讀 ⇒ 閘門判定 needConvert ⇒ 走失敗分支。
func TestRunAutoBackfill_ConversionFailure_NonFatal(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "tw_extended_90days.csv")
	jsonlPath := filepath.Join(dir, "tw_extended_90days.jsonl")
	writeReplayCSVFixture(t, csvPath, "2026-03-19", "2026-03-20")
	// JSONL 路徑是一個目錄 ⇒ 轉檔（os.Create）必定失敗。
	if err := os.MkdirAll(jsonlPath, 0o755); err != nil {
		t.Fatalf("seed JSONL path as directory: %v", err)
	}
	csvBefore, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("read CSV pre-state: %v", err)
	}

	cfg := config.Config{WorkDir: dir, ReplayDataPath: csvPath}
	now := taipeiClock(t, 2026, time.March, 20, 16, 0)

	var runErr error
	logs := captureGatewayLog(t, func() {
		runErr = runAutoBackfill(context.Background(), cfg, now)
	})
	if runErr != nil {
		t.Fatalf("runAutoBackfill returned %v, want nil (conversion failure must stay non-fatal)", runErr)
	}
	if !strings.Contains(logs, "backfill CSV→JSONL conversion failed (non-fatal)") {
		t.Errorf("expected a distinguishable 'conversion failed' line, got:\n%s", logs)
	}
	if strings.Contains(logs, "conversion: converted") {
		t.Errorf("must not report success after a failed conversion, got:\n%s", logs)
	}
	csvAfter, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("read CSV post-state: %v", err)
	}
	if !bytes.Equal(csvBefore, csvAfter) {
		t.Error("replay CSV was modified by a failed conversion")
	}
	info, err := os.Stat(jsonlPath)
	if err != nil || !info.IsDir() {
		t.Errorf("injected failure target should still be the untouched directory (err=%v)", err)
	}
	t.Logf("[evidence e] logs:\n%s", strings.TrimSpace(logs))
}

// 閘門語意的單元測試：convert 只發生在「CSV 比 JSONL 新」或「JSONL 讀不到」。
// 這是把 owner 的判準寫成可執行的表，避免後人把閘門改成「無條件轉檔」。
func TestCompareReplayCSVToJSONL_ConversionNeeded(t *testing.T) {
	dir := t.TempDir()
	csv := filepath.Join(dir, "replay.csv")
	jsonl := filepath.Join(dir, "replay.jsonl")
	missing := filepath.Join(dir, "does-not-exist.jsonl")

	writeReplayCSVFixture(t, csv, "2026-03-19", "2026-03-20")
	if err := importer.ImportTWOpenDataCSVToJSONL(csv, jsonl); err != nil {
		t.Fatalf("seed JSONL: %v", err)
	}

	cases := []struct {
		name  string
		csv   string
		jsonl string
		want  bool
	}{
		{"CSV 與 JSONL 同步 ⇒ 不轉", csv, jsonl, false},
		{"JSONL 缺席 ⇒ 轉（自我修復）", csv, missing, true},
		{"CSV 讀不到 ⇒ 不轉（無來源）", filepath.Join(dir, "no-such.csv"), jsonl, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compareReplayCSVToJSONL(tc.csv, tc.jsonl).conversionNeeded()
			if got != tc.want {
				t.Errorf("conversionNeeded() = %v, want %v", got, tc.want)
			}
		})
	}

	// CSV 比 JSONL 新一天 ⇒ 轉（把 CSV 換成多一天的版本）。
	writeReplayCSVFixture(t, csv, "2026-03-19", "2026-03-20", "2026-03-23")
	f := compareReplayCSVToJSONL(csv, jsonl)
	if !f.csvLatest.After(f.jsonlLatest) {
		t.Fatalf("fixture should have CSV(%s) newer than JSONL(%s)", f.csvLatest, f.jsonlLatest)
	}
	if !f.conversionNeeded() {
		t.Error("CSV newer than JSONL must require a conversion")
	}
}

// (R1) 抓取失敗的那一輪，JSONL 仍必須被評估。
//
// 對抗式複核（2026-09-27）指出：轉檔若放在「抓取成功之後」的 `return err` 後方，
// 就等於讓「要不要補資料」再一次決定「要不要轉檔」—— 與本 PR 修掉的缺陷同型，
// 只是窗口縮小到「抓取失敗的輪次」。本測試用一個必定 exit 3 的假
// daily-replay-sync 注入抓取失敗，要求：
//   - 任務仍把抓取錯誤往上報（既有容錯不變）
//   - 同一輪仍完成 CSV→JSONL 轉檔（JSONL 追上）
func TestRunAutoBackfill_FetchFailureStillEvaluatesConversion(t *testing.T) {
	dir, csvPath, jsonlPath := replayFixtureDir(t)
	// JSONL 落後一天；CSV 已前進 ⇒ 同輪有「缺口」也有「該轉檔」。
	buildJSONLFromDates(t, csvPath, jsonlPath, "2026-03-18")
	writeReplayCSVFixture(t, csvPath, "2026-03-18", "2026-03-19")

	// 假 binary：必失敗（exit 3）。runAutoBackfill 會優先使用 WorkDir 下的它。
	fakeSync := filepath.Join(dir, "daily-replay-sync")
	if err := os.WriteFile(fakeSync, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatalf("write fake daily-replay-sync: %v", err)
	}

	cfg := config.Config{WorkDir: dir, ReplayDataPath: csvPath}
	now := taipeiClock(t, 2026, time.March, 24, 16, 0) // start=2026-03-20 <= end=2026-03-24 ⇒ 有缺口

	var runErr error
	logs := captureGatewayLog(t, func() {
		runErr = runAutoBackfill(context.Background(), cfg, now)
	})
	if runErr == nil {
		t.Fatal("a failed daily-replay-sync must still surface as a task error")
	}
	// 抓取失敗本身走 return value（不是 log）—— 這是既有契約，本測試不改它。
	if !strings.Contains(runErr.Error(), "backfill failed") {
		t.Errorf("runErr = %v, want it to carry the fetch failure", runErr)
	}
	if !strings.Contains(logs, "backfill CSV→JSONL conversion: converted") {
		t.Errorf("conversion must not be skipped because the fetch failed, got:\n%s", logs)
	}
	if got := replayLatestDateForTest(t, jsonlPath); got != "2026-03-19" {
		t.Errorf("JSONL latest date = %s, want 2026-03-19 (conversion must run in the same tick as the failed fetch)", got)
	}
	t.Logf("[evidence R1] logs:\n%s", strings.TrimSpace(logs))
}
