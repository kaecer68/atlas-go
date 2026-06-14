package monitoring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordChannelFetch(t *testing.T) {
	tmpDir := t.TempDir()

	RecordChannelFetch(tmpDir, "test_channel", "ok", "", 100, 50)

	healthPath := filepath.Join(tmpDir, "channel_health.json")
	data, err := os.ReadFile(healthPath)
	if err != nil {
		t.Fatalf("failed to read health file: %v", err)
	}

	var wrapper struct {
		Channels map[string]*ChannelHealthRecord `json:"channels"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	record, ok := wrapper.Channels["test_channel"]
	if !ok {
		t.Fatal("test_channel not found in health record")
	}
	if record.Status != "ok" {
		t.Fatalf("expected status 'ok', got %s", record.Status)
	}
	if record.RateLimitRemaining != 100 {
		t.Fatalf("expected rate_limit_remaining 100, got %d", record.RateLimitRemaining)
	}
	if record.LatencyMs != 50 {
		t.Fatalf("expected latency_ms 50, got %d", record.LatencyMs)
	}
}

func TestRecordChannelFetch_MultipleChannels(t *testing.T) {
	tmpDir := t.TempDir()

	RecordChannelFetch(tmpDir, "channel_a", "ok", "", 100, 50)
	RecordChannelFetch(tmpDir, "channel_b", "error", "rate limit exceeded", 0, 0)

	healthPath := filepath.Join(tmpDir, "channel_health.json")
	data, _ := os.ReadFile(healthPath)
	var wrapper struct {
		Channels map[string]*ChannelHealthRecord `json:"channels"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("unmarshal wrapper: %v", err)
	}

	if len(wrapper.Channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(wrapper.Channels))
	}

	recA := wrapper.Channels["channel_a"]
	if recA == nil || recA.Status != "ok" {
		t.Fatal("channel_a should have status ok")
	}

	recB := wrapper.Channels["channel_b"]
	if recB == nil || recB.Status != "error" {
		t.Fatal("channel_b should have status error")
	}
	if recB.LastError != "rate limit exceeded" {
		t.Fatalf("expected last_error 'rate limit exceeded', got %s", recB.LastError)
	}
}

func TestRecordChannelFetch_WithErrors(t *testing.T) {
	tmpDir := t.TempDir()

	RecordChannelFetch(tmpDir, "err_channel", "error", "connection timeout", 5, 2000)

	healthPath := filepath.Join(tmpDir, "channel_health.json")
	data, _ := os.ReadFile(healthPath)
	var wrapper struct {
		Channels map[string]*ChannelHealthRecord `json:"channels"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("unmarshal wrapper: %v", err)
	}

	rec := wrapper.Channels["err_channel"]
	if rec == nil {
		t.Fatal("err_channel not found")
	}
	if rec.Status != "error" {
		t.Fatalf("expected status 'error', got %s", rec.Status)
	}
	if rec.LastError != "connection timeout" {
		t.Fatalf("expected last_error 'connection timeout', got %s", rec.LastError)
	}
	if rec.RateLimitRemaining != 5 {
		t.Fatalf("expected rate_limit_remaining 5, got %d", rec.RateLimitRemaining)
	}
	if rec.LatencyMs != 2000 {
		t.Fatalf("expected latency_ms 2000, got %d", rec.LatencyMs)
	}
}

func TestChannelHealthRecord_LastDataAt(t *testing.T) {
	dataAt := "2026-05-13T10:11:12Z"
	input := ChannelHealthRecord{LastDataAt: dataAt}

	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}

	var got ChannelHealthRecord
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}

	if got.LastDataAt != dataAt {
		t.Fatalf("expected last_data_at %q, got %q", dataAt, got.LastDataAt)
	}
}

func TestWithLastDataAt(t *testing.T) {
	timestamp := time.Date(2026, 5, 13, 10, 11, 12, 0, time.UTC)
	rec := &ChannelHealthRecord{}

	WithLastDataAt(timestamp)(rec)

	if rec.LastDataAt != timestamp.Format(time.RFC3339) {
		t.Fatalf("expected last_data_at %q, got %q", timestamp.Format(time.RFC3339), rec.LastDataAt)
	}
}

func TestChannelHealthStore_RecordAndGet(t *testing.T) {
	dir := t.TempDir()
	store := NewChannelHealthStore(dir)

	if err := store.Record("twse", "ok", "", WithLatencyMs(100)); err != nil {
		t.Fatalf("Record: %v", err)
	}

	rec := store.Get("twse")
	if rec == nil {
		t.Fatal("expected record")
	}
	if rec.Status != "ok" {
		t.Errorf("status = %q, want ok", rec.Status)
	}
	if rec.LatencyMs != 100 {
		t.Errorf("latency = %d, want 100", rec.LatencyMs)
	}
}

func TestChannelHealthStore_Record_Error(t *testing.T) {
	dir := t.TempDir()
	store := NewChannelHealthStore(dir)

	if err := store.Record("fugle", "error", "timeout", WithRecordsFetched(0)); err != nil {
		t.Fatalf("Record: %v", err)
	}

	rec := store.Get("fugle")
	if rec == nil || rec.Status != "error" {
		t.Fatalf("expected error record, got %+v", rec)
	}
	if rec.LastError != "timeout" {
		t.Errorf("last_error = %q, want timeout", rec.LastError)
	}
}

func TestChannelHealthStore_LoadExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "channel_health.json")
	payload := `{"channels":{"twse":{"status":"ok","last_fetch_at":"2026-01-01T00:00:00Z"}}}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewChannelHealthStore(dir)
	rec := store.Get("twse")
	if rec == nil || rec.Status != "ok" {
		t.Fatalf("expected loaded record, got %+v", rec)
	}
}

func TestChannelHealthStore_Alerts(t *testing.T) {
	dir := t.TempDir()
	store := NewChannelHealthStore(dir)

	if err := store.Record("twse", "ok", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Record("fugle", "error", "down"); err != nil {
		t.Fatal(err)
	}
	if err := store.Record("finmind", "inactive", ""); err != nil {
		t.Fatal(err)
	}

	alerts := store.Alerts()
	if len(alerts) != 1 || alerts[0].ChannelID != "fugle" {
		t.Errorf("expected 1 alert for fugle, got %+v", alerts)
	}
}

func TestChannelHealthStore_SyncAllToDB_NoPool(t *testing.T) {
	dir := t.TempDir()
	store := NewChannelHealthStore(dir)
	if err := store.Record("twse", "ok", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SyncAllToDB(); err == nil {
		t.Error("expected error when pool nil")
	}
}

func TestRecordChannelFetchWithPool_NoPool(t *testing.T) {
	dir := t.TempDir()
	RecordChannelFetchWithPool(dir, "twse", "ok", "", nil, WithLastDataAt(time.Now()))

	store := NewChannelHealthStore(dir)
	if store.Get("twse") == nil {
		t.Error("expected record")
	}
}
