package apigateway

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeReplayCSV creates a minimal replay CSV (same schema as
// data/replay/tw_extended_90days.csv) whose latest date is the given date,
// and returns its path.
func writeReplayCSV(t *testing.T, latestDate string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.csv")
	content := "Date,Code,Name,TradeVolume,Open,High,Low,Close\n" +
		"2026-07-27,2330,台積電,1000,599,601,598,600\n" +
		latestDate + ",2330,台積電,1000,599,601,598,600\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTWSEChannelAdapter_Metadata(t *testing.T) {
	a := &TWSEChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "twse_replay" {
		t.Errorf("ChannelID = %q, want twse_replay", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "TWSE 證交所" {
		t.Errorf("Platform = %q, want TWSE 證交所", m.Platform)
	}
	if m.APIFormat != "json" {
		t.Errorf("APIFormat = %q, want json", m.APIFormat)
	}
	if m.Path != "www.twse.com.tw" {
		t.Errorf("Path = %q, want www.twse.com.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTWSEChannelAdapter_Fetch(t *testing.T) {
	// N1 S3a：Fetch 讀本地 replay CSV，不打 live TWSE。
	path := writeReplayCSV(t, "2026-07-28")
	adapter := NewTWSEChannelAdapter(path)
	res, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
	if res.Meta.ChannelID != "twse_replay" {
		t.Errorf("ChannelID = %q, want twse_replay", res.Meta.ChannelID)
	}
	// Data must be the latest-date quotes from the CSV.
	var quotes []map[string]any
	if err := json.Unmarshal(res.Data, &quotes); err != nil {
		t.Fatalf("unmarshal fetch data: %v", err)
	}
	if len(quotes) != 1 {
		t.Fatalf("quotes = %d, want 1 (latest date only)", len(quotes))
	}
	if quotes[0]["symbol"] != "2330.TW" {
		t.Errorf("symbol = %v, want 2330.TW", quotes[0]["symbol"])
	}
	if quotes[0]["last"] != float64(600) {
		t.Errorf("last = %v, want 600 (close of latest date)", quotes[0]["last"])
	}
}

func TestTWSEChannelAdapter_Fetch_MissingFile(t *testing.T) {
	// N1 S3a：缺檔回明確錯誤（replay data missing），而非打 live TWSE。
	adapter := NewTWSEChannelAdapter(filepath.Join(t.TempDir(), "does-not-exist.csv"))
	_, err := adapter.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for missing replay file")
	}
	if !strings.Contains(err.Error(), "replay") {
		t.Errorf("expected replay-related error, got %v", err)
	}
}

func TestTWSEChannelAdapter_HealthCheck(t *testing.T) {
	// Fresh CSV (yesterday) → ok.
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	adapter := NewTWSEChannelAdapter(writeReplayCSV(t, yesterday))
	status, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

func TestTWSEChannelAdapter_HealthCheck_WarnWhenSomewhatStale(t *testing.T) {
	// 7 days old → warn (mirrors monitoring checkReplayHealth thresholds).
	stale := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	adapter := NewTWSEChannelAdapter(writeReplayCSV(t, stale))
	status, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "warn" {
		t.Errorf("Status = %q, want warn", status.Status)
	}
}

func TestTWSEChannelAdapter_HealthCheck_Stale(t *testing.T) {
	// N1 S3a：舊檔回明確錯誤（stale），不打 live TWSE。
	stale := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	adapter := NewTWSEChannelAdapter(writeReplayCSV(t, stale))
	status, err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error for stale replay data")
	}
	if status.Status != "error" {
		t.Errorf("Status = %q, want error", status.Status)
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Errorf("expected stale error, got %v", err)
	}
}

func TestTWSEChannelAdapter_HealthCheck_MissingFile(t *testing.T) {
	adapter := NewTWSEChannelAdapter(filepath.Join(t.TempDir(), "does-not-exist.csv"))
	status, err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error for missing replay file")
	}
	if status.Status != "error" {
		t.Errorf("Status = %q, want error", status.Status)
	}
}

func TestTWSEChannelAdapter_RateLimit(t *testing.T) {
	adapter := NewTWSEChannelAdapter(writeReplayCSV(t, "2026-07-28"))
	if adapter.RateLimit() == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
