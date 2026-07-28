package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

func TestChannelHealthStore_PassesOptionsAndPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := apigateway.NewChannelHealthStoreWithPool(tmpDir, nil)
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
	if adapter.Get("adapter_channel") == nil {
		t.Fatal("expected singleton store to remain usable")
	}
}

// =============================================================================
// channelHealthStore unit tests
// =============================================================================

func TestChannelHealthStore_Record_NewChannel(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
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
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
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
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	stamp := time.Date(2026, 5, 27, 14, 30, 0, 0, time.UTC)
	err := store.Record(
		"ch-opts", "ok", "",
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
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	rec := store.Get("no-such-channel")
	if rec != nil {
		t.Fatalf("expected nil for unrecorded channel, got %+v", rec)
	}
}

func TestChannelHealthStore_Get_ReturnsCopyNotPointerToInternalData(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
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
	store1 := apigateway.NewChannelHealthStoreWithPool(tmpDir, nil)
	if err := store1.Record("ch-a", "ok", ""); err != nil {
		t.Fatalf("store1 record ch-a: %v", err)
	}
	if err := store1.Record("ch-b", "error", "connection refused"); err != nil {
		t.Fatalf("store1 record ch-b: %v", err)
	}

	// Create a new store pointing at the same directory — must load data from file
	store2 := apigateway.NewChannelHealthStoreWithPool(tmpDir, nil)
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
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
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
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
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
		{"warn", "待更新"},
		{"expected_delay", "正常延遲"},
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
			adapter := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
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

// =============================================================================
// MarkDegraded tests
// =============================================================================

func TestChannelHealthStore_RecordDegraded(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := apigateway.NewChannelHealthStoreWithPool(tmpDir, nil)
	if err := adapter.Record("tsmc_revenue", "degraded", "cache_fallback"); err != nil {
		t.Fatalf("MarkDegraded: %v", err)
	}
	rec := adapter.Get("tsmc_revenue")
	if rec == nil {
		t.Fatal("expected record after MarkDegraded")
	}
	if rec.Status != "degraded" {
		t.Fatalf("expected status degraded, got %q", rec.Status)
	}
	if rec.LastError != "cache_fallback" {
		t.Fatalf("expected last_error cache_fallback, got %q", rec.LastError)
	}
}

// =============================================================================
// NewDataChannelService (0% → covered)
// =============================================================================

func TestNewDataChannelService(t *testing.T) {
	svc := NewDataChannelService("/tmp/workdir", nil, nil, nil, nil, nil, "fugle-key", "fubon-key", "finmind-key", "tej-key")
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.WorkDir != "/tmp/workdir" {
		t.Errorf("expected WorkDir /tmp/workdir, got %q", svc.WorkDir)
	}
	if svc.FugleAPIKey != "fugle-key" {
		t.Errorf("expected FugleAPIKey fugle-key, got %q", svc.FugleAPIKey)
	}
	if svc.healthStore == nil {
		t.Error("expected healthStore to be initialized")
	}
}

// =============================================================================
// GetChannelStatus (0% → covered)
// =============================================================================

func TestGetChannelStatus(t *testing.T) {
	svc := NewDataChannelService("/tmp/workdir", nil, nil, nil, nil, nil, "", "", "", "")
	ch, err := svc.GetChannelStatus(context.Background(), "fugle")
	if err != nil {
		t.Fatalf("expected no error for valid channel, got %v", err)
	}
	if ch.ChannelID != "fugle" {
		t.Errorf("expected channelID fugle, got %q", ch.ChannelID)
	}
}

func TestGetChannelStatus_NotFound(t *testing.T) {
	svc := NewDataChannelService("/tmp/workdir", nil, nil, nil, nil, nil, "", "", "", "")
	_, err := svc.GetChannelStatus(context.Background(), "nonexistent_channel")
	if err == nil {
		t.Fatal("expected error for nonexistent channel")
	}
}

// =============================================================================
// GetAllChannelStatuses (0% → covered)
// =============================================================================

func TestGetAllChannelStatuses(t *testing.T) {
	svc := NewDataChannelService("/tmp/workdir", nil, nil, nil, nil, nil, "fugle-key", "fubon-key", "finmind-key", "tej-key")
	channels, err := svc.GetAllChannelStatuses(context.Background())
	if err != nil {
		t.Fatalf("GetAllChannelStatuses: %v", err)
	}
	if len(channels) == 0 {
		t.Fatal("expected at least one channel")
	}
	seen := make(map[string]bool)
	for _, c := range channels {
		if c.ChannelID == "" {
			t.Error("channel with empty ChannelID")
		}
		if seen[c.ChannelID] {
			t.Errorf("duplicate channelID %q", c.ChannelID)
		}
		seen[c.ChannelID] = true
	}
}

// =============================================================================
// GetAlerts (0% → covered)
// =============================================================================

func TestGetAlerts_NoAlerts(t *testing.T) {
	svc := NewDataChannelService("/tmp/workdir", nil, nil, nil, nil, nil, "", "", "", "")
	alerts, err := svc.GetAlerts(context.Background())
	if err != nil {
		t.Fatalf("GetAlerts: %v", err)
	}
	if alerts == nil {
		t.Error("expected non-nil alerts slice, got nil")
	}
}

func TestGetAlerts_WithErrorChannel(t *testing.T) {
	adapter := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	_ = adapter.Record("fugle", "error", "connection refused")
	svc := &DataChannelService{
		WorkDir:     "/tmp/workdir",
		healthStore: adapter,
	}
	alerts, err := svc.GetAlerts(context.Background())
	if err != nil {
		t.Fatalf("GetAlerts: %v", err)
	}
	if len(alerts) == 0 {
		t.Error("expected at least one alert for error channel")
	}
}

// =============================================================================
// statusText (0% → covered)
// =============================================================================

func TestStatusText(t *testing.T) {
	tests := []struct {
		status string
		expect string
	}{
		{"ok", "正常"},
		{"warn", "待更新"},
		{"error", "異常"},
		{"inactive", "未啟用"},
		{"unknown", "未知"},
	}
	for _, tt := range tests {
		got := statusText(tt.status)
		if got != tt.expect {
			t.Errorf("statusText(%q) = %q, want %q", tt.status, got, tt.expect)
		}
	}
}

// =============================================================================
// classifyErrorSeverity
// =============================================================================

func TestClassifyErrorSeverity(t *testing.T) {
	tests := []struct {
		name   string
		errMsg string
		expect string
	}{
		// Empty / no error
		{name: "empty string", errMsg: "", expect: ""},

		// info: off-hours / no-data — expected, not actionable
		{name: "info: no data available", errMsg: "margin fetch: no TWSE margin balance data available in the last 7 days", expect: ErrorSeverityInfo},
		{name: "info: no capital flow data", errMsg: "capital_flow fetch: no TWSE capital flow data available in the last 7 days", expect: ErrorSeverityInfo},

		// warn: transient network / rate-limit — usually self-healing
		{name: "warn: rate limit", errMsg: "rate limit: context deadline exceeded", expect: ErrorSeverityWarn},
		{name: "warn: context deadline", errMsg: "context deadline exceeded (Client.Timeout exceeded)", expect: ErrorSeverityWarn},
		{name: "warn: timeout", errMsg: "request timeout after 30s", expect: ErrorSeverityWarn},
		{name: "warn: connection reset", errMsg: "read tcp: connection reset by peer", expect: ErrorSeverityWarn},

		// error: infra down — requires operator action
		{name: "error: connection refused", errMsg: "dial tcp 127.0.0.1:18081: connection refused", expect: ErrorSeverityError},
		{name: "error: no such host", errMsg: "lookup api.example.com: no such host", expect: ErrorSeverityError},
		{name: "error: dial tcp", errMsg: "dial tcp 10.0.0.1:443: i/o timeout", expect: ErrorSeverityError},

		// error: config / registration — needs admin fix
		{name: "error: channel not registered", errMsg: "channel not registered: us_yahoo", expect: ErrorSeverityError},
		{name: "error: not found", errMsg: "config key not found", expect: ErrorSeverityError},
		{name: "error: invalid", errMsg: "invalid parameter: start_date", expect: ErrorSeverityError},

		// critical: auth / credential — urgent
		{name: "critical: api key", errMsg: "api key missing or invalid", expect: ErrorSeverityCritical},
		{name: "critical: unauthorized", errMsg: "unauthorized: token expired", expect: ErrorSeverityCritical},
		{name: "critical: 401", errMsg: "HTTP 401 Unauthorized", expect: ErrorSeverityCritical},
		{name: "critical: 403", errMsg: "HTTP 403 Forbidden", expect: ErrorSeverityCritical},
		{name: "critical: forbidden", errMsg: "access forbidden by policy", expect: ErrorSeverityCritical},

		// default: unknown falls to warn
		{name: "default: unknown error", errMsg: "something unexpected happened", expect: ErrorSeverityWarn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyErrorSeverity(tt.errMsg)
			if got != tt.expect {
				t.Errorf("classifyErrorSeverity(%q) = %q, want %q", tt.errMsg, got, tt.expect)
			}
		})
	}
}

func TestClassifyErrorSeverity_RulePriority(t *testing.T) {
	got := classifyErrorSeverity("invalid api key")
	if got != ErrorSeverityCritical {
		t.Errorf("expected critical (auth beats config), got %q", got)
	}

	got2 := classifyErrorSeverity("connection refused: no data available in the last hour")
	if got2 != ErrorSeverityError {
		t.Errorf("expected error (infra beats info), got %q", got2)
	}

	got3 := classifyErrorSeverity("rate limit exceeded: HTTP 403")
	if got3 != ErrorSeverityWarn {
		t.Errorf("expected warn (rate limit before 403), got %q", got3)
	}
}

func TestClassifyErrorSeverity_RealWorldMessages(t *testing.T) {
	// Real messages observed in channel_health.json
	tests := []struct {
		name   string
		errMsg string
		expect string
	}{
		{
			name:   "twse_margin off-hours",
			errMsg: "margin fetch: no TWSE margin balance data available in the last 7 days",
			expect: ErrorSeverityInfo,
		},
		{
			name:   "us_yahoo unregistered",
			errMsg: "channel not registered: us_yahoo",
			expect: ErrorSeverityError,
		},
		{
			name:   "rate limit timeout",
			errMsg: "rate limit: context deadline exceeded",
			expect: ErrorSeverityWarn,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyErrorSeverity(tt.errMsg)
			if got != tt.expect {
				t.Errorf("classifyErrorSeverity(%q) = %q, want %q", tt.errMsg, got, tt.expect)
			}
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		substrs []string
		expect  bool
	}{
		{name: "single match", s: "hello world", substrs: []string{"world"}, expect: true},
		{name: "no match", s: "hello world", substrs: []string{"foo", "bar"}, expect: false},
		{name: "empty input", s: "", substrs: []string{"anything"}, expect: false},
		{name: "empty substrs", s: "hello", substrs: nil, expect: false},
		{name: "substring match", s: "connection refused", substrs: []string{"refused"}, expect: true},
		{name: "case sensitive", s: "Rate Limit", substrs: []string{"rate limit"}, expect: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsAny(tt.s, tt.substrs...)
			if got != tt.expect {
				t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.substrs, got, tt.expect)
			}
		})
	}
}

// =============================================================================
// Enabled merge tests (channels.json → DataChannel.Enabled field)
// =============================================================================

func TestDataChannelService_GetAllChannelStatuses_EnabledMergedFromChannelsJSON(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "data/state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	channelsState := map[string]any{
		"twse_replay": map[string]any{"enabled": true, "updated_at": "2026-01-01T00:00:00Z"},
		"fugle":       map[string]any{"enabled": false, "updated_at": "2026-01-01T00:00:00Z"},
		"fubon":       map[string]any{"enabled": false, "updated_at": "2026-01-01T00:00:00Z"},
	}
	stateBytes, err := json.Marshal(channelsState)
	if err != nil {
		t.Fatalf("marshal channels.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "channels.json"), stateBytes, 0o644); err != nil {
		t.Fatalf("write channels.json: %v", err)
	}

	svc := NewDataChannelService(tmpDir, nil, nil, nil, nil, nil, "fugleKey", "fubonKey", "finmindKey", "tejKey")
	channels, err := svc.GetAllChannelStatuses(context.Background())
	if err != nil {
		t.Fatalf("GetAllChannelStatuses: %v", err)
	}
	byID := make(map[string]DataChannel, len(channels))
	for _, c := range channels {
		byID[c.ChannelID] = c
	}

	// Case A: explicit enabled=true in JSON → field true
	if c, ok := byID["twse_replay"]; !ok {
		t.Errorf("expected twse_replay in response (missing)")
	} else if !c.Enabled {
		t.Errorf("expected twse_replay enabled=true (per channels.json), got false")
	}
	// Case B: explicit enabled=false in JSON → field false
	if c, ok := byID["fugle"]; !ok {
		t.Errorf("expected fugle in response (missing)")
	} else if c.Enabled {
		t.Errorf("expected fugle enabled=false (per channels.json), got true")
	}
	if c, ok := byID["fubon"]; !ok {
		t.Errorf("expected fubon in response (missing)")
	} else if c.Enabled {
		t.Errorf("expected fubon enabled=false (per channels.json), got true")
	}
	// Case C: absent from channels.json → default-on (true)
	if c, ok := byID["us_yahoo"]; !ok {
		t.Errorf("expected us_yahoo in response (missing)")
	} else if !c.Enabled {
		t.Errorf("expected us_yahoo enabled=true (default-on for absent keys), got false")
	}
}

func TestDataChannelService_GetAllChannelStatuses_NoChannelsJSON_DefaultsEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewDataChannelService(tmpDir, nil, nil, nil, nil, nil, "fugleKey", "fubonKey", "finmindKey", "tejKey")
	channels, err := svc.GetAllChannelStatuses(context.Background())
	if err != nil {
		t.Fatalf("GetAllChannelStatuses: %v", err)
	}
	if len(channels) == 0 {
		t.Fatal("expected at least one channel even with no channels.json")
	}
	for _, c := range channels {
		if !c.Enabled {
			t.Errorf("expected channel %q enabled=true (default-on when channels.json missing), got false", c.ChannelID)
		}
	}
}

func TestDataChannelService_GetAllChannelStatuses_MalformedChannelsJSON_DefaultsEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "data/state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "channels.json"), []byte("not-valid-json{"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	svc := NewDataChannelService(tmpDir, nil, nil, nil, nil, nil, "fugleKey", "fubonKey", "finmindKey", "tejKey")
	channels, err := svc.GetAllChannelStatuses(context.Background())
	if err != nil {
		t.Fatalf("GetAllChannelStatuses: %v", err)
	}
	for _, c := range channels {
		if !c.Enabled {
			t.Errorf("expected channel %q enabled=true (graceful default-on when channels.json malformed), got false", c.ChannelID)
		}
	}
}
