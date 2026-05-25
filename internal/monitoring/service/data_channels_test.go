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

func TestDataChannelService_getHealthFromStore_NilRecordFallbacks(t *testing.T) {
	tests := []struct {
		name          string
		apiKey        string
		seedRecord    bool
		expectStatus  string
		expectUpdated string
		expectLastErr string
	}{
		{
			name:          "warn when api key exists but no record",
			apiKey:        "api-key-present",
			expectStatus:  "warn",
			expectUpdated: "API Key 已設定，等待首次健康檢查",
			expectLastErr: "",
		},
		{
			name:          "inactive when api key missing and no record",
			apiKey:        "",
			expectStatus:  "inactive",
			expectUpdated: "未設定 API Key",
			expectLastErr: "",
		},
		{
			name:          "returns stored ok record",
			apiKey:        "api-key-present",
			seedRecord:    true,
			expectStatus:  "ok",
			expectUpdated: "2026-05-25T08:28:08+08:00",
			expectLastErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewChannelHealthStoreAdapter(t.TempDir(), nil)
			if tt.seedRecord {
				if err := adapter.Record("fugle", "ok", ""); err != nil {
					t.Fatalf("seed record: %v", err)
				}
				rec := adapter.Get("fugle")
				if rec == nil {
					t.Fatal("expected seeded record")
				}
				tt.expectUpdated = rec.LastFetchAt
			}

			svc := &DataChannelService{healthStore: adapter}
			status, updated, lastErr := svc.getHealthFromStore("fugle", tt.apiKey)

			if status != tt.expectStatus {
				t.Fatalf("expected status %q, got %q", tt.expectStatus, status)
			}
			if updated != tt.expectUpdated {
				t.Fatalf("expected updated %q, got %q", tt.expectUpdated, updated)
			}
			if lastErr != tt.expectLastErr {
				t.Fatalf("expected last error %q, got %q", tt.expectLastErr, lastErr)
			}
		})
	}
}
