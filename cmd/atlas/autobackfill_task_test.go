package main

// auto_backfill 的缺口回補 / CSV→JSONL 轉檔解耦測試（2026-09-27）。
//
// 驗收三態：
//   (a) 無缺口但 JSONL 落後   ⇒ 轉檔發生、JSONL 追上（本檔 TestAutoBackfill_NoGapStillConvertsJSONL
//                                與 TestRunAutoBackfillAt_NoGapConvertsStaleJSONL）
//   (b) 無缺口且 JSONL 已最新 ⇒ 不轉檔、不重寫（mtime / size 不變）
//   (c) 轉檔失敗仍非致命，且日誌三態可辨（state=converted / up_to_date / failed）

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// autoBackfillTaskForTest 取得註冊後的 auto_backfill task body（與 production
// 完全相同的那顆 closure），讓測試走 registration → task 的真實路徑。
func autoBackfillTaskForTest(t *testing.T, cfg config.Config) *apigateway.ScheduledTask {
	t.Helper()
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerOperationsTasks(operationsDeps{taskMgr: mgr, cfg: cfg})
	task, ok := mgr.Get("auto_backfill")
	if !ok {
		t.Fatal("auto_backfill must be registered")
	}
	return task
}

// autoBackfillWindowEndDate 是測試自己的 oracle：復刻 window end 推導
// （Taipei；15:30 前算前一交易日；週末回退到週五）。
//
// 刻意不重用 implementation helper：修法前的程式碼沒有可重用的 helper，
// 而獨立 oracle 才驗得出「實作真的沒缺口」。
func autoBackfillWindowEndDate(now time.Time) (string, time.Duration) {
	if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
		now = now.In(tz)
	}
	// 距 15:30 與跨日的距離（分鐘）：太近就跳過，避免 task 內部 time.Now()
	// 與本測試推導不一致造成 flake。
	mins := float64(now.Hour()*60+now.Minute()) + float64(now.Second())/60
	dist := 3 * time.Minute
	for _, boundary := range []float64{15*60 + 30, 0, 24 * 60} {
		d := time.Duration(absFloat(boundary-mins)) * time.Minute
		if d < dist {
			dist = d
		}
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if now.Hour() < 15 || (now.Hour() == 15 && now.Minute() < 30) {
		end = end.AddDate(0, 0, -1)
	}
	for end.Weekday() == time.Saturday || end.Weekday() == time.Sunday {
		end = end.AddDate(0, 0, -1)
	}
	return end.Format(replayDateLayout), dist
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// TestAutoBackfill_NoGapStillConvertsJSONL — 驗收 (a) 的**結構性回歸釘**。
//
// 舊碼把轉檔寫在「有缺口」分支內（`if start.After(end) { return nil }` 之後），
// 因此只要每日 cron 正常落地（＝常態無缺口），auto_backfill 每天在 return nil
// 離開、永遠不轉檔 ⇒ CSV 前進而 JSONL 凍結（2026-08-24 ~ 2026-09-24 實證）。
//
// 本測試刻意讓「無缺口」成立（CSV 最新日 = 本 tick 的 window end），並斷言
// JSONL 仍追上 CSV。修法前必須 FAIL，修法後必須 PASS。
func TestAutoBackfill_NoGapStillConvertsJSONL(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "tw_extended_90days.csv")
	jsonlPath := replayJSONLPath(csvPath)

	noGapDate, boundaryDist := autoBackfillWindowEndDate(time.Now())
	if boundaryDist < 3*time.Minute {
		t.Skipf("距離 15:30／跨日不到 3 分鐘（%v）——task 內部 time.Now() 可能跨過邊界，跳過避免 flake", boundaryDist)
	}
	writeReplayCSV(t, csvPath, noGapDate)
	writeReplayJSONL(t, jsonlPath, "2026-08-24")

	// production 形狀：絕對路徑（容器內 /app/data/replay/tw_extended_90days.csv）。
	task := autoBackfillTaskForTest(t, config.Config{WorkDir: dir, ReplayDataPath: csvPath})
	buf := captureLog(t)

	if err := task.Task(t.Context()); err != nil {
		t.Fatalf("auto_backfill tick returned error: %v", err)
	}

	got, err := replayLatestDate(jsonlPath)
	if err != nil {
		t.Fatalf("JSONL unreadable after tick: %v\n--- log ---\n%s", err, buf.String())
	}
	if got.Format(replayDateLayout) != noGapDate {
		t.Fatalf("no-gap tick did NOT convert CSV→JSONL: jsonl latest = %s, want %s (CSV latest)\n--- log ---\n%s",
			got.Format(replayDateLayout), noGapDate, buf.String())
	}
	if !strings.Contains(buf.String(), "state="+string(replayConversionConverted)) {
		t.Fatalf("log must report state=%s\n--- log ---\n%s", replayConversionConverted, buf.String())
	}
}

// TestAutoBackfill_NoGapLogsSkipAndStillEvaluatesConversion — 無缺口時必須留下
// 「跳過回補、仍在評估轉檔」的可辨識痕跡（舊碼是靜默 return nil）。
func TestAutoBackfill_NoGapLogsSkipAndStillEvaluatesConversion(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "tw_extended_90days.csv")
	jsonlPath := replayJSONLPath(csvPath)

	noGapDate, boundaryDist := autoBackfillWindowEndDate(time.Now())
	if boundaryDist < 3*time.Minute {
		t.Skipf("距離 15:30／跨日不到 3 分鐘（%v）", boundaryDist)
	}
	writeReplayCSV(t, csvPath, noGapDate)
	// JSONL 已最新 ⇒ 應記 up_to_date（zero work）。
	writeReplayJSONL(t, jsonlPath, noGapDate)

	before, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}

	task := autoBackfillTaskForTest(t, config.Config{WorkDir: dir, ReplayDataPath: csvPath})
	buf := captureLog(t)
	if err := task.Task(t.Context()); err != nil {
		t.Fatalf("auto_backfill tick returned error: %v", err)
	}

	after, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatalf("up-to-date JSONL must NOT be rewritten: mtime %v→%v size %d→%d\n--- log ---\n%s",
			before.ModTime(), after.ModTime(), before.Size(), after.Size(), buf.String())
	}
	if !strings.Contains(buf.String(), "state="+string(replayConversionUpToDate)) {
		t.Fatalf("log must report state=%s\n--- log ---\n%s", replayConversionUpToDate, buf.String())
	}
}

// --- 決定性測試（注入 clock / runner）：驗收 (b) (c) -----------------------------

// noGapNow 是「確定無缺口」的 injected clock：2026-09-23（週三）08:00 UTC
// = 16:00 Taipei（盤後）。搭配 CSV 最新日 2026-09-23 ⇒ start=09-24 > end=09-23。
//
// 註：即使 runtime 沒有 tzdata（LoadLocation 失敗），now 留在 08:00 UTC，
// hour<15 ⇒ end=09-22，start=09-24 > end 仍然無缺口 —— 兩種環境都成立。
var noGapNow = time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)

// recordingRunner 記錄回補呼叫；非空回傳值會讓 task 失敗（用來證明「沒被呼叫」）。
func recordingRunner(calls *[][]string) backfillRunner {
	return func(_ context.Context, workDir, csvPath, startStr, endStr string) ([]byte, error) {
		*calls = append(*calls, []string{workDir, csvPath, startStr, endStr})
		return []byte("stub backfill"), nil
	}
}

func noGapFixture(t *testing.T) (config.Config, string, string) {
	t.Helper()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "tw_extended_90days.csv")
	return config.Config{WorkDir: dir, ReplayDataPath: csvPath}, csvPath, replayJSONLPath(csvPath)
}

// TestRunAutoBackfillAt_NoGapConvertsStaleJSONL — 驗收 (a)：無缺口 + JSONL 落後
// ⇒ 不跑回補，但轉檔必須發生、JSONL 追上 CSV。
func TestRunAutoBackfillAt_NoGapConvertsStaleJSONL(t *testing.T) {
	cfg, csvPath, jsonlPath := noGapFixture(t)
	writeReplayCSV(t, csvPath, "2026-09-23")
	writeReplayJSONL(t, jsonlPath, "2026-08-24")

	var calls [][]string
	buf := captureLog(t)
	if err := runAutoBackfillAt(t.Context(), cfg, noGapNow, recordingRunner(&calls)); err != nil {
		t.Fatalf("task returned error: %v", err)
	}

	if len(calls) != 0 {
		t.Fatalf("no-gap tick must NOT run daily-replay-sync; got %d call(s): %v", len(calls), calls)
	}
	got, err := replayLatestDate(jsonlPath)
	if err != nil {
		t.Fatalf("JSONL unreadable: %v", err)
	}
	if got.Format(replayDateLayout) != "2026-09-23" {
		t.Fatalf("JSONL not caught up: got %s want 2026-09-23\n--- log ---\n%s", got.Format(replayDateLayout), buf.String())
	}
	logs := buf.String()
	if !strings.Contains(logs, "no gap") {
		t.Fatalf("no-gap tick must log the skip explicitly\n--- log ---\n%s", logs)
	}
	if !strings.Contains(logs, "state="+string(replayConversionConverted)) {
		t.Fatalf("no-gap tick must convert and log state=converted\n--- log ---\n%s", logs)
	}
}

// TestRunAutoBackfillAt_UpToDateJSONLIsNotRewritten — 驗收 (b)：無缺口且 JSONL 已最新
// ⇒ **不轉檔、不重寫**（mtime 與大小不變）。
func TestRunAutoBackfillAt_UpToDateJSONLIsNotRewritten(t *testing.T) {
	cfg, csvPath, jsonlPath := noGapFixture(t)
	writeReplayCSV(t, csvPath, "2026-09-23")
	writeReplayJSONL(t, jsonlPath, "2026-09-23")

	before, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeBytes, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	buf := captureLog(t)
	if err := runAutoBackfillAt(t.Context(), cfg, noGapNow, recordingRunner(&calls)); err != nil {
		t.Fatalf("task returned error: %v", err)
	}

	after, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	afterBytes, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("up-to-date JSONL was rewritten: mtime %v → %v\n--- log ---\n%s", before.ModTime(), after.ModTime(), buf.String())
	}
	if after.Size() != before.Size() || !bytes.Equal(beforeBytes, afterBytes) {
		t.Fatalf("up-to-date JSONL content/size changed: size %d → %d\n--- log ---\n%s", before.Size(), after.Size(), buf.String())
	}
	logs := buf.String()
	if !strings.Contains(logs, "state="+string(replayConversionUpToDate)) {
		t.Fatalf("must log state=up_to_date\n--- log ---\n%s", logs)
	}
	if strings.Contains(logs, "state="+string(replayConversionConverted)) {
		t.Fatalf("must not report a conversion on an up-to-date JSONL\n--- log ---\n%s", logs)
	}
}

// TestRunAutoBackfillAt_GapStillBackfillsAndConverts — 有缺口時：回補照跑（語意不變），
// 且轉檔閘門同樣評估（兩條路徑都有轉檔）。
func TestRunAutoBackfillAt_GapStillBackfillsAndConverts(t *testing.T) {
	cfg, csvPath, jsonlPath := noGapFixture(t)
	writeReplayCSV(t, csvPath, "2026-09-15")
	writeReplayJSONL(t, jsonlPath, "2026-08-24")

	var calls [][]string
	buf := captureLog(t)
	if err := runAutoBackfillAt(t.Context(), cfg, noGapNow, recordingRunner(&calls)); err != nil {
		t.Fatalf("task returned error: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("gap tick must run daily-replay-sync exactly once; got %d: %v", len(calls), calls)
	}
	if calls[0][2] != "2026-09-16" {
		t.Fatalf("backfill start = %q, want 2026-09-16 (first weekday after 2026-09-15)", calls[0][2])
	}
	if calls[0][1] != cfg.ReplayDataPath {
		t.Fatalf("backfill csv arg = %q, want %q", calls[0][1], cfg.ReplayDataPath)
	}
	got, err := replayLatestDate(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.Format(replayDateLayout) != "2026-09-15" {
		t.Fatalf("JSONL not converted on the gap path: %s\n--- log ---\n%s", got.Format(replayDateLayout), buf.String())
	}
	if !strings.Contains(buf.String(), "state="+string(replayConversionConverted)) {
		t.Fatalf("must log state=converted\n--- log ---\n%s", buf.String())
	}
}

// TestSyncReplayJSONLIfStale_ThreeStatesAreDistinguishable — 驗收 (c)：三態日誌互斥且可辨。
func TestSyncReplayJSONLIfStale_ThreeStatesAreDistinguishable(t *testing.T) {
	dir := t.TempDir()

	newPair := func(t *testing.T, csvDate, jsonlDate string) (string, string) {
		t.Helper()
		d := t.TempDir()
		csvPath := filepath.Join(d, "tw_extended_90days.csv")
		writeReplayCSV(t, csvPath, csvDate)
		jsonlPath := replayJSONLPath(csvPath)
		if jsonlDate != "" {
			writeReplayJSONL(t, jsonlPath, jsonlDate)
		}
		return csvPath, jsonlPath
	}

	t.Run("converted", func(t *testing.T) {
		csvPath, jsonlPath := newPair(t, "2026-09-24", "2026-08-24")
		buf := captureLog(t)
		if got := syncReplayJSONLIfStale(csvPath, jsonlPath); got != replayConversionConverted {
			t.Fatalf("state = %q, want converted\n--- log ---\n%s", got, buf.String())
		}
		assertExactlyOneState(t, buf.String(), string(replayConversionConverted))
	})

	t.Run("up_to_date", func(t *testing.T) {
		csvPath, jsonlPath := newPair(t, "2026-09-24", "2026-09-24")
		buf := captureLog(t)
		if got := syncReplayJSONLIfStale(csvPath, jsonlPath); got != replayConversionUpToDate {
			t.Fatalf("state = %q, want up_to_date\n--- log ---\n%s", got, buf.String())
		}
		assertExactlyOneState(t, buf.String(), string(replayConversionUpToDate))
	})

	t.Run("failed_write_blocked", func(t *testing.T) {
		// CSV 可讀、JSONL 落後，但產物路徑被一個目錄佔住 ⇒ 轉檔必然失敗。
		csvPath, jsonlPath := newPair(t, "2026-09-24", "")
		if err := os.Mkdir(jsonlPath, 0o755); err != nil {
			t.Fatal(err)
		}
		buf := captureLog(t)
		if got := syncReplayJSONLIfStale(csvPath, jsonlPath); got != replayConversionFailed {
			t.Fatalf("state = %q, want failed\n--- log ---\n%s", got, buf.String())
		}
		assertExactlyOneState(t, buf.String(), string(replayConversionFailed))
		if !strings.Contains(buf.String(), "non-fatal") {
			t.Fatalf("conversion failure must be logged as non-fatal\n--- log ---\n%s", buf.String())
		}
	})

	t.Run("failed_csv_unreadable", func(t *testing.T) {
		buf := captureLog(t)
		missing := filepath.Join(dir, "missing.csv")
		if got := syncReplayJSONLIfStale(missing, replayJSONLPath(missing)); got != replayConversionFailed {
			t.Fatalf("state = %q, want failed\n--- log ---\n%s", got, buf.String())
		}
		if !strings.Contains(buf.String(), "csv_latest_date_unreadable") {
			t.Fatalf("CSV unreadable must be named in the log\n--- log ---\n%s", buf.String())
		}
		if _, err := os.Stat(replayJSONLPath(missing)); err == nil {
			t.Fatal("must NOT create a JSONL when the CSV latest date is unknown")
		}
	})
}

// assertExactlyOneState 確認一份日誌只帶三態之一（避免「同時說成功與失敗」）。
func assertExactlyOneState(t *testing.T, logs, want string) {
	t.Helper()
	seen := map[string]bool{}
	for _, s := range []replayConversionState{replayConversionConverted, replayConversionUpToDate, replayConversionFailed} {
		if strings.Contains(logs, "state="+string(s)) {
			seen[string(s)] = true
		}
	}
	if len(seen) != 1 || !seen[want] {
		t.Fatalf("expected exactly one state token %q in log; saw %v\n--- log ---\n%s", want, seen, logs)
	}
}

// TestSyncReplayJSONLIfStale_FailureIsNonFatalAtTaskLevel — 驗收 (c)：轉檔失敗不得讓
// auto_backfill tick 變成 error（BTM 不得因資料檔問題進入失敗處理）。
func TestSyncReplayJSONLIfStale_FailureIsNonFatalAtTaskLevel(t *testing.T) {
	cfg, csvPath, jsonlPath := noGapFixture(t)
	writeReplayCSV(t, csvPath, "2026-09-23")
	if err := os.Mkdir(jsonlPath, 0o755); err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	buf := captureLog(t)
	if err := runAutoBackfillAt(t.Context(), cfg, noGapNow, recordingRunner(&calls)); err != nil {
		t.Fatalf("conversion failure must be non-fatal, got error: %v", err)
	}
	if !strings.Contains(buf.String(), "state="+string(replayConversionFailed)) {
		t.Fatalf("must log state=failed\n--- log ---\n%s", buf.String())
	}
}

// TestReplayConversionGate_AlignsWithFreshnessMetrics — 閘門與 #2063 的可觀測性
// 不得漂移：指標說「CSV 比 JSONL 新」時閘門必須轉檔；轉完指標變相等、閘門必須回
// up_to_date（zero work）。兩者共用 replayLatestDate / replayJSONLPath。
func TestReplayConversionGate_AlignsWithFreshnessMetrics(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "tw_extended_90days.csv")
	jsonlPath := replayJSONLPath(csvPath)
	writeReplayCSV(t, csvPath, "2026-09-24")
	writeReplayJSONL(t, jsonlPath, "2026-08-24")

	body := replayMetricsBody(t, func() *monitoring.MetricsCollector {
		c := monitoring.NewMetricsCollector()
		exportReplayFreshnessMetrics(csvPath, c, time.Now())
		return c
	}())
	csvV := metricValue(t, body, MetricReplayCSVLatestDate)
	jsonlV := metricValue(t, body, MetricReplayJSONLLatestDate)
	if csvV <= jsonlV {
		t.Fatalf("fixture must show csv newer than jsonl; got csv=%v jsonl=%v", csvV, jsonlV)
	}

	buf := captureLog(t)
	if got := syncReplayJSONLIfStale(csvPath, jsonlPath); got != replayConversionConverted {
		t.Fatalf("metrics say CSV is newer ⇒ gate must convert; got %q\n--- log ---\n%s", got, buf.String())
	}

	c := monitoring.NewMetricsCollector()
	exportReplayFreshnessMetrics(csvPath, c, time.Now())
	after := replayMetricsBody(t, c)
	if got, want := metricValue(t, after, MetricReplayJSONLLatestDate), metricValue(t, after, MetricReplayCSVLatestDate); got != want {
		t.Fatalf("after conversion the two series must be equal; jsonl=%v csv=%v", got, want)
	}
	if got := syncReplayJSONLIfStale(csvPath, jsonlPath); got != replayConversionUpToDate {
		t.Fatalf("metrics say the two dates are equal ⇒ gate must skip; got %q", got)
	}
}

// metricValue 讀 /metrics body 上一條無 label gauge 的值。
func metricValue(t *testing.T, body, name string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, name+" "); ok {
			// MetricsCollector 以浮點格式輸出（`1790208000.000000`）。
			v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
			if err != nil {
				t.Fatalf("parse %s from %q: %v", name, line, err)
			}
			return v
		}
	}
	t.Fatalf("metric %s not found in /metrics body:\n%s", name, body)
	return 0
}

// TestResolveReplayPaths_UsesSharedJSONLDerivation — 產物路徑只有一條推導
// （與觀測端 replayJSONLPath 相同），且相對路徑以 workDir 為基準。
func TestResolveReplayPaths_UsesSharedJSONLDerivation(t *testing.T) {
	csv, jsonl := resolveReplayPaths("/app", "data/replay/tw_extended_90days.csv")
	if csv != "/app/data/replay/tw_extended_90days.csv" {
		t.Fatalf("csv = %q", csv)
	}
	if jsonl != replayJSONLPath(csv) || jsonl != "/app/data/replay/tw_extended_90days.jsonl" {
		t.Fatalf("jsonl = %q (want the shared replayJSONLPath derivation)", jsonl)
	}

	absCSV := "/tmp/x/tw_extended_90days.csv"
	if csv, jsonl = resolveReplayPaths("/app", absCSV); csv != absCSV || jsonl != "/tmp/x/tw_extended_90days.jsonl" {
		t.Fatalf("absolute path must not be re-rooted: csv=%q jsonl=%q", csv, jsonl)
	}
}
