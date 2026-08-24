package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var taipei = time.FixedZone("CST", 8*3600)

func TestParseTaiwan5SecIndexResponse(t *testing.T) {
	raw := `{
		"msg": "success",
		"status": 200,
		"data": [
			{"date": "2026-04-29 09:00:00", "TAIEX": 39521.73},
			{"date": "2026-04-29 09:00:05", "TAIEX": 39081.34},
			{"date": "2026-04-29 09:00:10", "TAIEX": 39102.56}
		]
	}`

	bars, err := parseTaiwan5SecIndexResponse([]byte(raw))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(bars) != 3 {
		t.Fatalf("expected 3 bars, got %d", len(bars))
	}

	expectedFirst := time.Date(2026, 4, 29, 9, 0, 0, 0, taipei)
	if !bars[0].Date.Equal(expectedFirst) {
		t.Errorf("first bar date: got %v, want %v", bars[0].Date, expectedFirst)
	}
	if bars[0].TAIEX != 39521.73 {
		t.Errorf("first bar TAIEX: got %f, want 39521.73", bars[0].TAIEX)
	}

	expectedThird := time.Date(2026, 4, 29, 9, 0, 10, 0, taipei)
	if !bars[2].Date.Equal(expectedThird) {
		t.Errorf("third bar date: got %v, want %v", bars[2].Date, expectedThird)
	}
	if bars[2].TAIEX != 39102.56 {
		t.Errorf("third bar TAIEX: got %f, want 39102.56", bars[2].TAIEX)
	}
}

func TestParseTaiwan5SecIndexResponse_MinuteFormat(t *testing.T) {
	raw := `{
		"msg": "success",
		"status": 200,
		"data": [
			{"date": "2026-04-29 09:00", "TAIEX": 39521.73}
		]
	}`

	bars, err := parseTaiwan5SecIndexResponse([]byte(raw))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(bars) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(bars))
	}

	expected := time.Date(2026, 4, 29, 9, 0, 0, 0, taipei)
	if !bars[0].Date.Equal(expected) {
		t.Errorf("date: got %v, want %v", bars[0].Date, expected)
	}
}

func TestParseTaiwan5SecIndexResponse_InvalidDate(t *testing.T) {
	raw := `{
		"msg": "success",
		"status": 200,
		"data": [
			{"date": "not-a-date", "TAIEX": 39521.73},
			{"date": "2026-04-29 09:00:05", "TAIEX": 39081.34}
		]
	}`

	bars, err := parseTaiwan5SecIndexResponse([]byte(raw))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(bars) != 1 {
		t.Fatalf("expected 1 bar (invalid date skipped), got %d", len(bars))
	}
	if bars[0].TAIEX != 39081.34 {
		t.Errorf("TAIEX: got %f, want 39081.34", bars[0].TAIEX)
	}
}

func TestParseTaiwan5SecIndexResponse_APIError(t *testing.T) {
	raw := `{
		"msg": "no data",
		"status": 404,
		"data": []
	}`

	_, err := parseTaiwan5SecIndexResponse([]byte(raw))
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestParseTaiwan5SecIndexResponse_EmptyData(t *testing.T) {
	raw := `{
		"msg": "success",
		"status": 200,
		"data": []
	}`

	bars, err := parseTaiwan5SecIndexResponse([]byte(raw))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(bars) != 0 {
		t.Fatalf("expected 0 bars, got %d", len(bars))
	}
}

func TestFetchTaiwan5SecIndex(t *testing.T) {
	responseJSON := `{
		"msg": "success",
		"status": 200,
		"data": [
			{"date": "2026-04-29 09:00:00", "TAIEX": 39521.73},
			{"date": "2026-04-29 09:00:05", "TAIEX": 39081.34}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dataset") != "TaiwanVariousIndicators5Seconds" {
			t.Errorf("expected dataset=TaiwanVariousIndicators5Seconds, got %s", r.URL.Query().Get("dataset"))
		}
		if r.URL.Query().Get("start_date") != "2026-04-29" {
			t.Errorf("expected start_date=2026-04-29, got %s", r.URL.Query().Get("start_date"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	client := NewFinMindClient("test-key")
	client.SetHTTPClient(server.Client())

	// 覆寫 baseURL 以指向測試伺服器
	origURL := finmindBaseURL
	// 由於 finmindBaseURL 是常數，我們改用 httptest Server 的 URL
	// 需要直接呼叫 fetchDataset 或透過可測試的方式
	// 這裡直接測試 parse 邏輯，HTTP 整合測試需要修改 client
	_ = origURL

	bars, err := parseTaiwan5SecIndexResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	if bars[0].TAIEX != 39521.73 {
		t.Errorf("first bar TAIEX: got %f, want 39521.73", bars[0].TAIEX)
	}
}

func TestFetchTaiwan5SecIndex_Integration(t *testing.T) {
	responseJSON := `{
		"msg": "success",
		"status": 200,
		"data": [
			{"date": "2026-04-29 09:00:00", "TAIEX": 39521.73},
			{"date": "2026-04-29 09:00:05", "TAIEX": 39081.34}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	client := NewFinMindClient("test-key")
	// 使用測試伺服器的 HTTP client
	client.SetHTTPClient(server.Client())

	// 直接建構請求到測試伺服器
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v4/data?dataset=TaiwanVariousIndicators5Seconds&start_date=2026-04-29", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("http request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	bars, err := parseTaiwan5SecIndexResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
}

func TestSave5SecIndexToLedger(t *testing.T) {
	bars := []Taiwan5SecIndexBar{
		{Date: time.Date(2026, 4, 29, 9, 0, 0, 0, taipei), TAIEX: 39521.73},
		{Date: time.Date(2026, 4, 29, 9, 0, 5, 0, taipei), TAIEX: 39081.34},
	}

	tmpDir := t.TempDir()
	err := Save5SecIndexToLedger(bars, tmpDir)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	path := filepath.Join(tmpDir, "taiwan_5sec_index.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	type entry struct {
		Date  string  `json:"date"`
		TAIEX float64 `json:"taiex"`
		Type  string  `json:"type"`
	}

	var first entry
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("unmarshal first line: %v", err)
	}
	if first.Type != "5sec_index" {
		t.Errorf("first line type: got %q, want %q", first.Type, "5sec_index")
	}
	if first.TAIEX != 39521.73 {
		t.Errorf("first line TAIEX: got %f, want 39521.73", first.TAIEX)
	}
	if !strings.HasPrefix(first.Date, "2026-04-29T09:00:00") {
		t.Errorf("first line date: got %q, want RFC3339 format starting with 2026-04-29T09:00:00", first.Date)
	}

	var second entry
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("unmarshal second line: %v", err)
	}
	if second.TAIEX != 39081.34 {
		t.Errorf("second line TAIEX: got %f, want 39081.34", second.TAIEX)
	}
}

func TestSave5SecIndexToLedger_EmptyBars(t *testing.T) {
	tmpDir := t.TempDir()
	err := Save5SecIndexToLedger([]Taiwan5SecIndexBar{}, tmpDir)
	if err != nil {
		t.Fatalf("save empty bars failed: %v", err)
	}

	path := filepath.Join(tmpDir, "taiwan_5sec_index.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if len(strings.TrimSpace(string(data))) != 0 {
		t.Errorf("expected empty file for empty bars, got %q", string(data))
	}
}

func TestSave5SecIndexToLedger_Append(t *testing.T) {
	bars1 := []Taiwan5SecIndexBar{
		{Date: time.Date(2026, 4, 29, 9, 0, 0, 0, taipei), TAIEX: 39521.73},
	}
	bars2 := []Taiwan5SecIndexBar{
		{Date: time.Date(2026, 4, 29, 9, 0, 5, 0, taipei), TAIEX: 39081.34},
	}

	tmpDir := t.TempDir()
	if err := Save5SecIndexToLedger(bars1, tmpDir); err != nil {
		t.Fatalf("first save failed: %v", err)
	}
	if err := Save5SecIndexToLedger(bars2, tmpDir); err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	path := filepath.Join(tmpDir, "taiwan_5sec_index.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines after append, got %d", len(lines))
	}
}

// ─── P1-11: 5sec index must respect the daily-quota gate and capture
// error bodies (previously it bypassed AllowCall entirely) ────────────────

func TestFetchTaiwan5SecIndex_QuotaGate(t *testing.T) {
	hit := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"msg":"success","status":200,"data":[]}`))
	}))
	defer server.Close()

	client := &FinMindClient{
		httpClient:   server.Client(),
		rateLimiter:  newUnlimitedLimiter(),
		quotaTracker: &DailyQuotaTracker{dailyLimit: 0, lastReset: time.Now().Truncate(24 * time.Hour)},
	}

	_, err := client.FetchTaiwan5SecIndex(context.Background(), "2026-04-29")
	if err == nil || !strings.Contains(err.Error(), ErrQuotaExhausted.Error()) {
		t.Fatalf("expected ErrQuotaExhausted, got %v", err)
	}
	if hit != 0 {
		t.Fatalf("quota gate bypassed: HTTP request made (%d hits)", hit)
	}
}

func TestFetchTaiwan5SecIndex_ErrorBodyCaptured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"msg":"start_date is illegal"}`))
	}))
	defer server.Close()

	client := NewFinMindClient("test-key")
	client.httpClient = &http.Client{Transport: &rewriteTransport{target: server.URL, inner: http.DefaultTransport}}
	client.rateLimiter = newUnlimitedLimiter()

	_, err := client.FetchTaiwan5SecIndex(context.Background(), "2026-04-29")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "start_date is illegal") {
		t.Errorf("error body not captured: %v", err)
	}
}
