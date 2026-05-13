package service

import (
	"path/filepath"
	"testing"
	"time"
)

func TestChannelHealthStoreAdapter_PassesOptionsAndSingleton(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := NewChannelHealthStoreAdapter(tmpDir, nil)
	stamp := time.Date(2026, 5, 13, 10, 11, 12, 0, time.UTC)

	if err := adapter.Record("adapter_channel", "warn", "slow", WithLastDataAt(stamp), WithLatencyMs(1234)); err != nil {
		t.Fatalf("record: %v", err)
	}

	rec := adapter.Get("adapter_channel")
	if rec == nil {
		t.Fatal("expected record")
	}
	if rec.LastDataAt != stamp.Format(time.RFC3339) {
		t.Fatalf("expected last_data_at %q, got %q", stamp.Format(time.RFC3339), rec.LastDataAt)
	}
	if rec.LatencyMs != 1234 {
		t.Fatalf("expected latency_ms 1234, got %d", rec.LatencyMs)
	}
	if adapter.store == nil {
		t.Fatal("expected singleton store to be initialized")
	}
	if got := filepath.Base(adapter.store.path); got != "channel_health.json" {
		t.Fatalf("expected channel_health.json, got %s", got)
	}
	if adapter.Get("adapter_channel") == nil {
		t.Fatal("expected singleton store to remain usable")
	}
}
