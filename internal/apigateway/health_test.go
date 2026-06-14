package apigateway

import (
	"context"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func newTestHealthStore(t *testing.T) *UnifiedHealthStore {
	t.Helper()
	return NewUnifiedHealthStore(t.TempDir(), nil)
}

func TestNewUnifiedHealthStore(t *testing.T) {
	store := NewUnifiedHealthStore(t.TempDir(), nil)
	if store == nil {
		t.Fatal("NewUnifiedHealthStore returned nil")
	}
	if store.store == nil {
		t.Fatal("underlying store is nil")
	}
}

func TestUnifiedHealthStore_Record(t *testing.T) {
	s := newTestHealthStore(t)
	err := s.Record("test_channel", "ok", "", monitoring.WithLatencyMs(100))
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	rec := s.store.Get("test_channel")
	if rec == nil {
		t.Fatal("Get returned nil after Record")
	}
	if rec.Status != "ok" {
		t.Errorf("Status = %q, want ok", rec.Status)
	}
	if rec.LatencyMs != 100 {
		t.Errorf("LatencyMs = %d, want 100", rec.LatencyMs)
	}
}

func TestUnifiedHealthStore_Record_Error(t *testing.T) {
	s := newTestHealthStore(t)
	err := s.Record("test_channel", "error", "connection refused")
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	rec := s.store.Get("test_channel")
	if rec == nil {
		t.Fatal("Get returned nil after Record")
	}
	if rec.Status != "error" {
		t.Errorf("Status = %q, want error", rec.Status)
	}
	if rec.LastError != "connection refused" {
		t.Errorf("LastError = %q, want connection refused", rec.LastError)
	}
}

func TestUnifiedHealthStore_Get(t *testing.T) {
	s := newTestHealthStore(t)
	// Get on empty store returns nil
	rec := s.Get("nonexistent")
	if rec != nil {
		t.Error("Get on empty store should return nil")
	}
	// After record, Get returns the record
	_ = s.Record("ch1", "ok", "")
	rec = s.Get("ch1")
	if rec == nil {
		t.Fatal("Get returned nil after Record")
	}
	if rec.Status != "ok" {
		t.Errorf("Status = %q, want ok", rec.Status)
	}
}

func TestUnifiedHealthStore_Alerts(t *testing.T) {
	s := newTestHealthStore(t)
	// Empty store has no alerts
	alerts := s.Alerts()
	if len(alerts) != 0 {
		t.Errorf("Alerts on empty store = %d, want 0", len(alerts))
	}
	// Record an error - should appear in alerts
	_ = s.Record("ch1", "error", "timeout")
	alerts = s.Alerts()
	if len(alerts) != 1 {
		t.Fatalf("Alerts = %d, want 1", len(alerts))
	}
	if alerts[0].ChannelID != "ch1" {
		t.Errorf("Alert ChannelID = %q, want ch1", alerts[0].ChannelID)
	}
	if alerts[0].Status != "error" {
		t.Errorf("Alert Status = %q, want error", alerts[0].Status)
	}
	if alerts[0].Error != "timeout" {
		t.Errorf("Alert Error = %q, want timeout", alerts[0].Error)
	}
	// Ok status should not appear in alerts
	_ = s.Record("ch2", "ok", "")
	alerts = s.Alerts()
	if len(alerts) != 1 {
		t.Errorf("Alerts after ok record = %d, want 1", len(alerts))
	}
}

func TestUnifiedHealthStore_RecordChannelFetch_Error(t *testing.T) {
	s := newTestHealthStore(t)
	RecordChannelFetch(s, "ch1", nil, errors.New("fetch failed"))
	rec := s.Get("ch1")
	if rec == nil {
		t.Fatal("Get returned nil after RecordChannelFetch error")
	}
	if rec.Status != "error" {
		t.Errorf("Status = %q, want error", rec.Status)
	}
	if rec.LastError != "fetch failed" {
		t.Errorf("LastError = %q, want fetch failed", rec.LastError)
	}
}

func TestUnifiedHealthStore_RecordChannelFetch_Success(t *testing.T) {
	s := newTestHealthStore(t)
	result := &FetchResult{
		Meta: FetchMetadata{
			LatencyMs:          42,
			RateLimitRemaining: 99,
		},
	}
	RecordChannelFetch(s, "ch1", result, nil)
	rec := s.Get("ch1")
	if rec == nil {
		t.Fatal("Get returned nil after RecordChannelFetch success")
	}
	if rec.Status != "ok" {
		t.Errorf("Status = %q, want ok", rec.Status)
	}
}

func TestUnifiedHealthStore_RecordChannelFetch_Success_ZeroRateLimit(t *testing.T) {
	s := newTestHealthStore(t)
	result := &FetchResult{
		Meta: FetchMetadata{
			LatencyMs:          15,
			RateLimitRemaining: 0,
		},
	}
	RecordChannelFetch(s, "ch1", result, nil)
	rec := s.Get("ch1")
	if rec == nil {
		t.Fatal("Get returned nil after RecordChannelFetch success")
	}
	if rec.Status != "ok" {
		t.Errorf("Status = %q, want ok", rec.Status)
	}
}

func TestUnifiedHealthStore_StatusSummary(t *testing.T) {
	s := newTestHealthStore(t)
	summary := s.StatusSummary()
	if summary == nil {
		t.Fatal("StatusSummary returned nil")
	}
	// channelIDs() returns all known channels, so summary should be non-empty
	if len(summary) == 0 {
		t.Error("StatusSummary returned empty map")
	}
	// All channels should have "unknown" status initially
	foundUnknown := false
	for _, hs := range summary {
		if hs.Status == "unknown" {
			foundUnknown = true
			break
		}
	}
	if !foundUnknown {
		t.Error("StatusSummary should contain unknown status for unrecorded channels")
	}
}

func TestUnifiedHealthStore_StatusSummary_AfterRecord(t *testing.T) {
	s := newTestHealthStore(t)
	_ = s.Record("us_yahoo", "ok", "")
	summary := s.StatusSummary()
	// us_yahoo is in channelIDs, so it should be in the summary
	if hs, ok := summary["us_yahoo"]; ok {
		if hs.Status != "ok" {
			t.Errorf("us_yahoo status = %q, want ok", hs.Status)
		}
	}
}

func TestUnifiedHealthStore_CheckHealth(t *testing.T) {
	s := newTestHealthStore(t)
	g := newTestGateway(t)

	// CheckHealth with registered channels iterates channelIDs
	// and calls providers' HealthCheck
	results := s.CheckHealth(context.Background(), g.registry)
	if results == nil {
		t.Fatal("CheckHealth returned nil")
	}
	// Results should contain entries for all channelIDs
	if len(results) == 0 {
		t.Error("CheckHealth returned empty results")
	}
}

func TestAlert_Fields(t *testing.T) {
	a := Alert{
		ChannelID: "ch1",
		Status:    "error",
		Error:     "timeout",
		FetchAt:   "2026-06-14T12:00:00Z",
	}
	if a.ChannelID != "ch1" {
		t.Errorf("ChannelID = %q, want ch1", a.ChannelID)
	}
	if a.Status != "error" {
		t.Errorf("Status = %q, want error", a.Status)
	}
	if a.Error != "timeout" {
		t.Errorf("Error = %q, want timeout", a.Error)
	}
	if a.FetchAt != "2026-06-14T12:00:00Z" {
		t.Errorf("FetchAt = %q, want 2026-06-14T12:00:00Z", a.FetchAt)
	}
}

func TestHealthSummary_Fields(t *testing.T) {
	hs := HealthSummary{
		ChannelID: "ch1",
		Status:    "ok",
		LastFetch: "2026-06-14T12:00:00Z",
		LastError: "none",
	}
	if hs.ChannelID != "ch1" {
		t.Errorf("ChannelID = %q, want ch1", hs.ChannelID)
	}
	if hs.Status != "ok" {
		t.Errorf("Status = %q, want ok", hs.Status)
	}
	if hs.LastFetch != "2026-06-14T12:00:00Z" {
		t.Errorf("LastFetch = %q, want 2026-06-14T12:00:00Z", hs.LastFetch)
	}
	if hs.LastError != "none" {
		t.Errorf("LastError = %q, want none", hs.LastError)
	}
}
