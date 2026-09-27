package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// writeReplayCSV 寫一個最小但格式正確的 replay CSV（header + 1 列）。
// 欄位名必須與 internal/replay.LoadTWSEOpenDataCSV 的 required 清單一致。
func writeReplayCSV(t *testing.T, path, date string) {
	t.Helper()
	body := "Date,Code,Name,TradeVolume,Open,High,Low,Close\n" +
		date + ",2330,台積電,1000,600.00,610.00,595.00,605.00\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeReplayJSONL 寫一個最小但格式正確的轉檔 JSONL（最後一列即最新資料日）。
func writeReplayJSONL(t *testing.T, path, date string) {
	t.Helper()
	row := `{"date":"` + date + `T00:00:00Z","symbol":"2330.TW","close":605}` + "\n"
	if err := os.WriteFile(path, []byte(row), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replayMetricsBody(t *testing.T, collector *monitoring.MetricsCollector) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	monitoring.PrometheusHandler(collector).ServeHTTP(rec, req)
	return rec.Body.String()
}

// TestExportReplayFreshnessMetrics_EmitsBothDates — 正常狀態：CSV 與 JSONL 都有，
// 兩個序列都輸出，且值 = 該資料日的 UTC 00:00（不是讀取時間、也不是 1970）。
func TestExportReplayFreshnessMetrics_EmitsBothDates(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "tw_extended_90days.csv")
	writeReplayCSV(t, csvPath, "2026-09-24")
	writeReplayJSONL(t, replayJSONLPath(csvPath), "2026-09-24")

	collector := monitoring.NewMetricsCollector()
	now := time.Date(2026, 9, 27, 1, 50, 0, 0, time.UTC)
	exportReplayFreshnessMetrics(csvPath, collector, now)
	body := replayMetricsBody(t, collector)

	// 2026-09-24T00:00:00Z = 1785024000（UTC 00:00 的 Unix 秒）
	wantDate := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC).Unix()
	mustContain := []string{
		`atlas_replay_csv_latest_date_timestamp_seconds ` + itoa(wantDate),
		`atlas_replay_jsonl_latest_date_timestamp_seconds ` + itoa(wantDate),
		`atlas_replay_freshness_checked_timestamp_seconds ` + itoa(now.Unix()),
	}
	for _, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Fatalf("missing /metrics line %q\n--- body ---\n%s", want, body)
		}
	}
}

// TestExportReplayFreshnessMetrics_JSONLUnreadableEmitsNoSeries — JSONL 讀不到時
// **不得**輸出該序列（尤其不得輸出 0：0 會被讀成 1970 年＝落後 56 年）。
// 「CSV 有、JSONL 沒有」由規則的 unless 子句告警（見 promtool 測試檔）。
func TestExportReplayFreshnessMetrics_JSONLUnreadableEmitsNoSeries(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "tw_extended_90days.csv")
	writeReplayCSV(t, csvPath, "2026-09-24")
	// 刻意不寫 JSONL

	collector := monitoring.NewMetricsCollector()
	exportReplayFreshnessMetrics(csvPath, collector, time.Date(2026, 9, 27, 1, 50, 0, 0, time.UTC))
	body := replayMetricsBody(t, collector)

	if !strings.Contains(body, "atlas_replay_csv_latest_date_timestamp_seconds ") {
		t.Fatalf("CSV date series must be emitted even when the JSONL is missing\n--- body ---\n%s", body)
	}
	if strings.Contains(body, "atlas_replay_jsonl_latest_date_timestamp_seconds") {
		t.Fatalf("JSONL series must NOT be emitted when the file is unreadable\n--- body ---\n%s", body)
	}
}

// TestExportReplayFreshnessMetrics_CSVUnreadableEmitsNoDateSeries — CSV 讀不到時
// 只留心跳；不謊報任何資料日。
func TestExportReplayFreshnessMetrics_CSVUnreadableEmitsNoDateSeries(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "missing.csv")

	collector := monitoring.NewMetricsCollector()
	now := time.Date(2026, 9, 27, 1, 50, 0, 0, time.UTC)
	exportReplayFreshnessMetrics(csvPath, collector, now)
	body := replayMetricsBody(t, collector)

	if strings.Contains(body, "atlas_replay_csv_latest_date_timestamp_seconds") {
		t.Fatalf("CSV date series must not be emitted when the CSV is unreadable\n--- body ---\n%s", body)
	}
	if !strings.Contains(body, `atlas_replay_freshness_checked_timestamp_seconds `+itoa(now.Unix())) {
		t.Fatalf("heartbeat must still be emitted\n--- body ---\n%s", body)
	}
}

// TestReplayJSONLPath_MatchesWriter — 這條推導必須與寫入端
// （cmd/atlas/operations_tasks.go auto_backfill）完全相同，否則會變成
// 「檢查 A、寫入 B」。
func TestReplayJSONLPath_MatchesWriter(t *testing.T) {
	if got := replayJSONLPath("/app/data/replay/tw_extended_90days.csv"); got != "/app/data/replay/tw_extended_90days.jsonl" {
		t.Fatalf("replayJSONLPath = %q", got)
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

// TestHealthStatusValue_DegradedIsNotOk — AtlasReplaySyncNotLanding 規則的前提。
//
// 「跑了、但資料沒落地」在寫入端記的是 degraded（cmd/daily-replay-sync/main.go），
// 經 DeriveChannelStatus 透傳後由 healthStatusValue 映射成 4（未映射值）。規則只看
// `> 0`，所以映射值本身可以調整；但若有一天有人把 degraded 映射成 0（=ok），
// 那條規則會**靜默失效** —— 這個測試就是那個守門（degraded 不可以是 ok）。
func TestHealthStatusValue_DegradedIsNotOk(t *testing.T) {
	if got := healthStatusValue(apigateway.StatusDegraded); got == 0 {
		t.Fatalf("degraded 在 atlas_channel_health_status 上不得為 0（ok）; got %v", got)
	}
	if got := healthStatusValue(apigateway.StatusOK); got != 0 {
		t.Fatalf("ok 必須是 0; got %v", got)
	}
}
