package monitoring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
	json.Unmarshal(data, &wrapper)

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
	json.Unmarshal(data, &wrapper)

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
