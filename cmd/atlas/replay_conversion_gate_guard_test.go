package main

// 轉檔閘門的兩個守門測試（**test-only**，不改任何 production 行為）。
//
// 背景：#2079 把 auto_backfill 的 CSV→JSONL 轉檔與缺口判定解耦，並以
// 「CSV 最新資料日 > JSONL 最新資料日」為閘門。它自己的測試已覆蓋
// (a) 無缺口+落後 ⇒ 轉檔、(b) 無缺口+已最新 ⇒ 不重寫、(e) 轉檔失敗非致命、
// (R1) 抓取失敗那一輪仍轉檔。本檔只補兩個它刻意/順帶沒有的角度：
//
//   1. TestReplayConversionGate_AgreesWithFreshnessMetrics
//      閘門與 #2063 匯出的 atlas_replay_{csv,jsonl}_latest_date_timestamp_seconds
//      必須做**同一個判斷**。#2079 以註解宣告「共用 replayLatestDate 故不漂移」；
//      這裡把它變成可執行斷言：指標說 CSV 領先 ⇒ 閘門必須轉檔；轉完兩序列相等
//      ⇒ 閘門必須回「已最新、不重寫」。日後若有人改動指標語意或正規化（例如改讀
//      另一個檔、或把日期改成帶時分秒），只有這條測試會紅。
//
//   2. TestRunAutoBackfill_GapFetched_StillConverts
//      「有缺口 + 抓取成功 + 同輪仍轉檔」的正例。#2079 的 gap 路徑只有 R1 的
//      **失敗**案例（fake binary `exit 3`），沒有成功那一輪。這裡沿用它的風格：
//      在 WorkDir 放一支真的會被 exec 的假 daily-replay-sync（並把 argv 記下來），
//      因此同時釘住「binary 優先選徑 + 參數透傳 + 同輪轉檔」。
//
// 為什麼不需要 clock oracle：兩個測試都以 now 參數注入固定時刻（與 #2079 的
// taipeiClock 同慣例），不依賴執行當下時間 ⇒ 不會跨 15:30／跨日 flake。

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/importer"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// guardFixture 建立 replay fixture 目錄（CSV 與其轉檔 JSONL 的官方名稱）。
func guardFixture(t *testing.T) (dir, csvPath, jsonlPath string) {
	t.Helper()
	dir = t.TempDir()
	csvPath = filepath.Join(dir, "tw_extended_90days.csv")
	jsonlPath = filepath.Join(dir, "tw_extended_90days.jsonl")
	return dir, csvPath, jsonlPath
}

// guardWriteCSV 寫入最小但欄位齊全的 TWSE open-data CSV（欄位與
// internal/replay.LoadTWSEOpenDataCSV 的 required 清單一致）。
func guardWriteCSV(t *testing.T, path string, dates ...string) {
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

// guardSeedJSONLFromCSV 用**正式轉檔路徑**產生 JSONL 前置狀態（避免手寫 JSONL
// 與正式格式漂移）。
func guardSeedJSONLFromCSV(t *testing.T, csvPath, jsonlPath string, dates ...string) {
	t.Helper()
	guardWriteCSV(t, csvPath, dates...)
	if err := importer.ImportTWOpenDataCSVToJSONL(csvPath, jsonlPath); err != nil {
		t.Fatalf("seed JSONL from %v: %v", dates, err)
	}
}

// guardCaptureLog 攔截標準 logger 的輸出（cmd/atlas 的測試不使用 t.Parallel）。
func guardCaptureLog(t *testing.T, fn func()) string {
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

// guardMetricGauges 回傳 (csv, jsonl) 兩條 #2063 gauge 的當前值。
//
// 走 production 的 exportReplayFreshnessMetrics + /metrics handler ⇒ 斷言的是
// 「Prometheus 真的看到的值」，不是重算的日期。
func guardMetricGauges(t *testing.T, csvPath string) (csvValue, jsonlValue float64) {
	t.Helper()
	collector := monitoring.NewMetricsCollector()
	exportReplayFreshnessMetrics(csvPath, collector, time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC))

	rec := httptest.NewRecorder()
	monitoring.PrometheusHandler(collector).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()

	return guardMetricValue(t, body, MetricReplayCSVLatestDate),
		guardMetricValue(t, body, MetricReplayJSONLLatestDate)
}

// guardMetricValue 讀 /metrics body 上一條無 label gauge 的值
// （collector 以浮點格式輸出，例如 `1790208000.000000`）。
func guardMetricValue(t *testing.T, body, name string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, name+" "); ok {
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

// TestReplayConversionGate_AgreesWithFreshnessMetrics — 閘門 ↔ 觀測不得漂移。
//
// 三個階段：
//  1. 指標：csv > jsonl（JSONL 落後）⇒ 閘門必須要求轉檔，且轉檔真的發生。
//  2. 轉檔後指標：兩序列相等 ⇒ 閘門必須不再要求轉檔。
//  3. 同一狀態下再跑一次 ⇒ 檔案不得被重寫（zero work，mtime/size 不變）。
func TestReplayConversionGate_AgreesWithFreshnessMetrics(t *testing.T) {
	_, csvPath, jsonlPath := guardFixture(t)
	// JSONL 前置狀態落後一天（模擬「每日 cron 昨日寫入後、今日 CSV 又前進」）。
	guardSeedJSONLFromCSV(t, csvPath, jsonlPath, "2026-03-19")
	guardWriteCSV(t, csvPath, "2026-03-19", "2026-03-20")

	// --- 1. 指標說 CSV 領先 ⇒ 閘門必須轉檔 ---
	csvBefore, jsonlBefore := guardMetricGauges(t, csvPath)
	if csvBefore <= jsonlBefore {
		t.Fatalf("fixture must show csv(%v) newer than jsonl(%v)", csvBefore, jsonlBefore)
	}
	if !compareReplayCSVToJSONL(csvPath, jsonlPath).conversionNeeded() {
		t.Fatal("指標說 CSV 領先，但閘門拒絕轉檔 ⇒ 閘門與 atlas_replay_{csv,jsonl}_latest_date_timestamp_seconds 已經漂移")
	}
	logs := guardCaptureLog(t, func() {
		if err := syncReplayJSONLFromCSV(csvPath, jsonlPath); err != nil {
			t.Fatalf("conversion should succeed: %v", err)
		}
	})
	if !strings.Contains(logs, "conversion: converted") {
		t.Fatalf("expected a converted outcome, got:\n%s", logs)
	}
	if got := guardReplayLatestDate(t, jsonlPath); got != "2026-03-20" {
		t.Fatalf("JSONL latest date = %s, want 2026-03-20 (must catch up with the CSV)", got)
	}

	// --- 2. 指標說兩者相等 ⇒ 閘門必須不轉 ---
	csvAfter, jsonlAfter := guardMetricGauges(t, csvPath)
	if csvAfter != jsonlAfter {
		t.Fatalf("after conversion both series must be equal: csv=%v jsonl=%v", csvAfter, jsonlAfter)
	}
	if compareReplayCSVToJSONL(csvPath, jsonlPath).conversionNeeded() {
		t.Fatal("指標說兩個資料日相等，但閘門仍要求轉檔 ⇒ 閘門與觀測漂移")
	}

	// --- 3. zero work：不得重寫檔 ---
	before, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeBytes, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	skipLogs := guardCaptureLog(t, func() {
		if err := syncReplayJSONLFromCSV(csvPath, jsonlPath); err != nil {
			t.Fatalf("an up-to-date JSONL must be a no-op, got: %v", err)
		}
	})
	after, err := os.Stat(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	afterBytes, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() || !bytes.Equal(beforeBytes, afterBytes) {
		t.Fatalf("up-to-date JSONL must NOT be rewritten (mtime %v→%v size %d→%d)\n--- log ---\n%s",
			before.ModTime(), after.ModTime(), before.Size(), after.Size(), skipLogs)
	}
	if !strings.Contains(skipLogs, "skipped (already up to date)") {
		t.Fatalf("expected an explicit skip line, got:\n%s", skipLogs)
	}
	if strings.Contains(skipLogs, "conversion: converted") {
		t.Fatalf("must not report a conversion when nothing was stale, got:\n%s", skipLogs)
	}
}

// guardReplayLatestDate 讀回檔案最新資料日（走 production helper）。
func guardReplayLatestDate(t *testing.T, path string) string {
	t.Helper()
	d, err := replayLatestDate(path)
	if err != nil {
		t.Fatalf("replayLatestDate(%s): %v", path, err)
	}
	return d.Format(dateLayout)
}

// TestRunAutoBackfill_GapFetched_StillConverts — 有缺口 + 抓取**成功** ⇒ 同一輪
// 仍必須完成 CSV→JSONL 轉檔（R1 只涵蓋抓取失敗那一輪）。
//
// 假 daily-replay-sync 是真的被 exec 的（WorkDir 下優先選徑），並把 argv 逐行
// 記到檔案 ⇒ 同時驗證參數透傳（csv / backfill-start / backfill-end）。
func TestRunAutoBackfill_GapFetched_StillConverts(t *testing.T) {
	dir, csvPath, jsonlPath := guardFixture(t)
	// JSONL 落後一天；CSV 已多一天 ⇒ 同一輪同時有「缺口」與「該轉檔」。
	guardSeedJSONLFromCSV(t, csvPath, jsonlPath, "2026-03-18")
	guardWriteCSV(t, csvPath, "2026-03-18", "2026-03-19")

	argsFile := filepath.Join(dir, "sync-argv.txt")
	fakeSync := filepath.Join(dir, "daily-replay-sync")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + argsFile + "\necho fake-sync-ok\nexit 0\n"
	if err := os.WriteFile(fakeSync, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake daily-replay-sync: %v", err)
	}

	cfg := config.Config{WorkDir: dir, ReplayDataPath: csvPath}
	// 2026-03-24（週二）16:00 Taipei ⇒ end = 2026-03-24；CSV 最新 2026-03-19
	// ⇒ start = 2026-03-20（週五）≤ end ⇒ **有缺口**。
	now := time.Date(2026, time.March, 24, 16, 0, 0, 0, guardTaipeiLoc(t))

	var runErr error
	logs := guardCaptureLog(t, func() {
		runErr = runAutoBackfill(context.Background(), cfg, now)
	})

	if runErr != nil {
		t.Fatalf("a successful fetch must not fail the task: %v\n--- log ---\n%s", runErr, logs)
	}
	if !strings.Contains(logs, "backfill gap detected: 2026-03-20 to 2026-03-24") {
		t.Fatalf("expected the gap window 2026-03-20..2026-03-24 to be fetched, got:\n%s", logs)
	}
	if !strings.Contains(logs, "backfill success: fake-sync-ok") {
		t.Fatalf("expected a successful fetch line, got:\n%s", logs)
	}
	if !strings.Contains(logs, "conversion: converted") {
		t.Fatalf("the gap-free branch is not the only one that converts; a fetched round must convert too, got:\n%s", logs)
	}
	if got := guardReplayLatestDate(t, jsonlPath); got != "2026-03-19" {
		t.Fatalf("JSONL latest date = %s, want 2026-03-19 (conversion must run in the same tick as the fetch)", got)
	}

	// exec 選徑 + 參數透傳：WorkDir 下的 binary 優先，且 argv 逐項正確。
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("fake daily-replay-sync was not executed (no argv file): %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{"-csv", csvPath, "-backfill-start", "2026-03-20", "-backfill-end", "2026-03-24"}
	if len(got) != len(want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q (full: %q)", i, got[i], want[i], got)
		}
	}
}

// guardTaipeiLoc 回傳 Asia/Taipei；tzdata 缺席時跳過（與既有慣例相同）。
func guardTaipeiLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Skipf("Asia/Taipei tzdata unavailable: %v", err)
	}
	return loc
}
