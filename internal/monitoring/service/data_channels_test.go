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

// =============================================================================
// channelHealthStore unit tests
// =============================================================================

func TestChannelHealthStore_Record_NewChannel(t *testing.T) {
	store := newChannelHealthStore(t.TempDir(), nil)
	err := store.Record("ch-new", "ok", "")
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	rec := store.Get("ch-new")
	if rec == nil {
		t.Fatal("expected record to be retrievable after Record")
	}
	if rec.Status != "ok" {
		t.Fatalf("expected status ok, got %q", rec.Status)
	}
	if rec.LastFetchAt == "" {
		t.Fatal("expected LastFetchAt to be set")
	}
	if rec.LastError != "" {
		t.Fatalf("expected LastError empty on ok status, got %q", rec.LastError)
	}
}

func TestChannelHealthStore_Record_UpdateClearsErrorOnOk(t *testing.T) {
	store := newChannelHealthStore(t.TempDir(), nil)
	// Record error first
	if err := store.Record("ch-errclr", "error", "something broke"); err != nil {
		t.Fatalf("record error: %v", err)
	}
	rec := store.Get("ch-errclr")
	if rec.Status != "error" || rec.LastError != "something broke" {
		t.Fatalf("expected error status with message, got %q / %q", rec.Status, rec.LastError)
	}
	// Then record ok — LastError must be cleared, LastSuccessAt must be set
	if err := store.Record("ch-errclr", "ok", ""); err != nil {
		t.Fatalf("record ok: %v", err)
	}
	rec2 := store.Get("ch-errclr")
	if rec2.Status != "ok" {
		t.Fatalf("expected status ok, got %q", rec2.Status)
	}
	if rec2.LastError != "" {
		t.Fatalf("expected LastError cleared on ok, got %q", rec2.LastError)
	}
	if rec2.LastSuccessAt == "" {
		t.Fatal("expected LastSuccessAt to be set on ok transition")
	}
}

func TestChannelHealthStore_Record_WithOptions(t *testing.T) {
	store := newChannelHealthStore(t.TempDir(), nil)
	stamp := time.Date(2026, 5, 27, 14, 30, 0, 0, time.UTC)
	err := store.Record("ch-opts", "ok", "",
		WithLastDataAt(stamp),
		WithLatencyMs(789),
	)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	rec := store.Get("ch-opts")
	if rec == nil {
		t.Fatal("expected record")
	}
	if rec.LastDataAt != stamp.Format(time.RFC3339) {
		t.Fatalf("expected LastDataAt %q, got %q", stamp.Format(time.RFC3339), rec.LastDataAt)
	}
	if rec.LatencyMs != 789 {
		t.Fatalf("expected LatencyMs 789, got %d", rec.LatencyMs)
	}
}

func TestChannelHealthStore_Get_UnrecordedChannel(t *testing.T) {
	store := newChannelHealthStore(t.TempDir(), nil)
	rec := store.Get("no-such-channel")
	if rec != nil {
		t.Fatalf("expected nil for unrecorded channel, got %+v", rec)
	}
}

func TestChannelHealthStore_Get_ReturnsCopyNotPointerToInternalData(t *testing.T) {
	store := newChannelHealthStore(t.TempDir(), nil)
	if err := store.Record("ch-copy", "ok", ""); err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	rec := store.Get("ch-copy")
	if rec == nil {
		t.Fatal("expected record")
	}
	// Mutate the returned copy
	rec.Status = "tampered"
	rec.LatencyMs = 99999
	rec.LastError = "injected"
	// Get again — must return original values
	rec2 := store.Get("ch-copy")
	if rec2 == nil {
		t.Fatal("expected record")
	}
	if rec2.Status != "ok" {
		t.Fatalf("mutation leaked: expected status ok, got %q", rec2.Status)
	}
	if rec2.LatencyMs != 0 {
		t.Fatalf("mutation leaked: expected LatencyMs 0, got %d", rec2.LatencyMs)
	}
	if rec2.LastError != "" {
		t.Fatalf("mutation leaked: expected LastError empty, got %q", rec2.LastError)
	}
}

func TestChannelHealthStore_PersistenceRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	// Create store, record two channels — data saved to channel_health.json on disk
	store1 := newChannelHealthStore(tmpDir, nil)
	if err := store1.Record("ch-a", "ok", ""); err != nil {
		t.Fatalf("store1 record ch-a: %v", err)
	}
	if err := store1.Record("ch-b", "error", "connection refused"); err != nil {
		t.Fatalf("store1 record ch-b: %v", err)
	}

	// Create a new store pointing at the same directory — must load data from file
	store2 := newChannelHealthStore(tmpDir, nil)
	recA := store2.Get("ch-a")
	if recA == nil {
		t.Fatal("store2: expected ch-a loaded from persisted file")
	}
	if recA.Status != "ok" {
		t.Fatalf("store2 ch-a: expected status ok, got %q", recA.Status)
	}

	recB := store2.Get("ch-b")
	if recB == nil {
		t.Fatal("store2: expected ch-b loaded from persisted file")
	}
	if recB.Status != "error" {
		t.Fatalf("store2 ch-b: expected status error, got %q", recB.Status)
	}
	if recB.LastError != "connection refused" {
		t.Fatalf("store2 ch-b: expected LastError 'connection refused', got %q", recB.LastError)
	}
}

func TestRecordOption_WithLastDataAt(t *testing.T) {
	stamp := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	opt := WithLastDataAt(stamp)
	rec := &ChannelHealthRecord{}
	opt(rec)
	expected := stamp.Format(time.RFC3339)
	if rec.LastDataAt != expected {
		t.Fatalf("expected %q, got %q", expected, rec.LastDataAt)
	}
}

func TestRecordOption_WithLatencyMs(t *testing.T) {
	opt := WithLatencyMs(2500)
	rec := &ChannelHealthRecord{}
	opt(rec)
	if rec.LatencyMs != 2500 {
		t.Fatalf("expected 2500, got %d", rec.LatencyMs)
	}
}

func TestChannelHealthStore_Record_EmptyChannelID(t *testing.T) {
	store := newChannelHealthStore(t.TempDir(), nil)
	if err := store.Record("", "warn", ""); err != nil {
		t.Fatalf("Record with empty channelID should not error: %v", err)
	}
	rec := store.Get("")
	if rec == nil {
		t.Fatal("expected record for empty channelID")
	}
	if rec.Status != "warn" {
		t.Fatalf("expected status warn, got %q", rec.Status)
	}
}

func TestChannelHealthStore_Record_MultipleChannelsAllStored(t *testing.T) {
	store := newChannelHealthStore(t.TempDir(), nil)
	entries := map[string]string{
		"alpha":   "ok",
		"beta":    "warn",
		"gamma":   "error",
		"delta":   "ok",
		"epsilon": "warn",
	}
	for id, status := range entries {
		if err := store.Record(id, status, ""); err != nil {
			t.Fatalf("Record %s: %v", id, err)
		}
	}
	for id, expectedStatus := range entries {
		rec := store.Get(id)
		if rec == nil {
			t.Fatalf("expected record for %s", id)
		}
		if rec.Status != expectedStatus {
			t.Fatalf("%s: expected status %q, got %q", id, expectedStatus, rec.Status)
		}
	}
	// Also verify no spurious extra channels exist
	nilRec := store.Get("zeta")
	if nilRec != nil {
		t.Fatalf("expected nil for unrecorded channel 'zeta', got %+v", nilRec)
	}
}

func TestStatusText_AllStatuses(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"ok", "正常"},
		{"warn", "延遲"},
		{"error", "異常"},
		{"partial", "部分異常"},
		{"inactive", "未啟用"},
		{"unknown", "未知"},
		{"", "未知"},
	}
	for _, tt := range tests {
		got := StatusText(tt.status)
		if got != tt.expected {
			t.Errorf("StatusText(%q) = %q, want %q", tt.status, got, tt.expected)
		}
	}
}

func TestLastErrorStr(t *testing.T) {
	t.Run("nil record returns empty", func(t *testing.T) {
		if got := lastErrorStr(nil); got != "" {
			t.Errorf("lastErrorStr(nil) = %q, want empty", got)
		}
	})
	t.Run("record with error returns error", func(t *testing.T) {
		rec := &ChannelHealthRecord{LastError: "connection refused"}
		if got := lastErrorStr(rec); got != "connection refused" {
			t.Errorf("lastErrorStr = %q, want 'connection refused'", got)
		}
	})
	t.Run("record without error returns empty", func(t *testing.T) {
		rec := &ChannelHealthRecord{}
		if got := lastErrorStr(rec); got != "" {
			t.Errorf("lastErrorStr = %q, want empty", got)
		}
	})
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
