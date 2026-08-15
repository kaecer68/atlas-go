package apigateway

import (
	"testing"
	"time"
)

// TestChannelHealthStore_Alerts_FilterStale verifies that non-ok records
// older than the configured staleThreshold are filtered from Alerts().
//
// Real-world context: dashboard showed a 27-day-old `fubon` "no such host"
// error alongside fresh channel states. The stale record masked the real
// channel health — without filtering, Alerts() returns every historical
// error and the dashboard can't distinguish "currently broken" from
// "was broken once".
func TestChannelHealthStore_Alerts_FilterStale(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	s.WithNowFunc(func() time.Time { return now })
	s.WithStaleThreshold(1 * time.Hour)

	// Inject stale record: error timestamp is 2 hours before "now".
	s.mu.Lock()
	s.data["ch_stale_2h"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
		LastError:   "old dns failure",
	}
	// Inject stale record from 27 days ago (the actual production scenario).
	s.data["ch_stale_27d"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-27 * 24 * time.Hour).Format(time.RFC3339),
		LastError:   "very old transient failure",
	}
	s.mu.Unlock()

	alerts := s.Alerts()
	if len(alerts) != 0 {
		t.Errorf("expected stale alerts to be filtered, got %d: %+v", len(alerts), alerts)
	}
}

// TestChannelHealthStore_Alerts_KeepFresh verifies that non-ok records
// within the staleThreshold window still surface as alerts.
func TestChannelHealthStore_Alerts_KeepFresh(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	s.WithNowFunc(func() time.Time { return now })
	s.WithStaleThreshold(1 * time.Hour)

	s.mu.Lock()
	s.data["ch_fresh_30m"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-30 * time.Minute).Format(time.RFC3339),
		LastError:   "recent failure",
	}
	s.data["ch_fresh_59m"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-59 * time.Minute).Format(time.RFC3339),
		LastError:   "boundary case",
	}
	s.mu.Unlock()

	alerts := s.Alerts()
	if len(alerts) != 2 {
		t.Errorf("expected 2 fresh alerts, got %d: %+v", len(alerts), alerts)
	}
}

// TestChannelHealthStore_Alerts_ZeroThresholdDisabled verifies that
// setting staleThreshold to 0 disables filtering entirely — useful for
// historical audit / debugging where all records are wanted.
func TestChannelHealthStore_Alerts_ZeroThresholdDisabled(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	s.WithNowFunc(func() time.Time { return now })
	s.WithStaleThreshold(0) // disable filter

	s.mu.Lock()
	s.data["ch_30d_old"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
		LastError:   "very old",
	}
	s.data["ch_ok"] = &ChannelHealthRecord{
		Status:    "ok",
		LastError: "",
	}
	s.mu.Unlock()

	alerts := s.Alerts()
	if len(alerts) != 1 || alerts[0].ChannelID != "ch_30d_old" {
		t.Errorf("expected 1 old alert when filter disabled, got %+v", alerts)
	}
}

// TestChannelHealthStore_Alerts_BoundaryEdge verifies the > comparison:
// exactly at threshold = included; threshold + 1ns = filtered.
func TestChannelHealthStore_Alerts_BoundaryEdge(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	s.WithNowFunc(func() time.Time { return now })
	s.WithStaleThreshold(1 * time.Hour)

	s.mu.Lock()
	// Exactly 1 hour old: should be included (boundary inclusive)
	s.data["ch_at_threshold"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		LastError:   "boundary",
	}
	// 1 hour + 1 nanosecond old: should be filtered
	s.data["ch_over_threshold"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-1*time.Hour - time.Nanosecond).Format(time.RFC3339),
		LastError:   "just over",
	}
	s.mu.Unlock()

	alerts := s.Alerts()
	if len(alerts) != 1 || alerts[0].ChannelID != "ch_at_threshold" {
		t.Errorf("expected exactly ch_at_threshold to surface (boundary inclusive), got %+v", alerts)
	}
}

// TestChannelHealthStore_Alerts_UnparseableTimestampKept verifies the
// defensive fallback: if LastFetchAt cannot be parsed, the record is
// shown rather than silently filtered. Prefer false-positive alerts
// over silently missing a real failure.
func TestChannelHealthStore_Alerts_UnparseableTimestampKept(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	s.WithNowFunc(func() time.Time { return now })
	s.WithStaleThreshold(1 * time.Hour)

	s.mu.Lock()
	s.data["ch_bad_ts"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: "not-a-rfc3339-timestamp",
		LastError:   "bad timestamp",
	}
	s.mu.Unlock()

	alerts := s.Alerts()
	if len(alerts) != 1 || alerts[0].ChannelID != "ch_bad_ts" {
		t.Errorf("expected unparseable timestamp record to be kept (defensive), got %+v", alerts)
	}
}

// TestChannelHealthStore_Alerts_OKAndInactiveExcluded verifies the
// existing semantics: "ok" and "inactive" records never appear in
// Alerts() regardless of age.
func TestChannelHealthStore_Alerts_OKAndInactiveExcluded(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	s.WithNowFunc(func() time.Time { return now })
	s.WithStaleThreshold(1 * time.Hour)

	s.mu.Lock()
	s.data["ch_ok"] = &ChannelHealthRecord{
		Status:      "ok",
		LastFetchAt: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
	}
	s.data["ch_inactive"] = &ChannelHealthRecord{
		Status:      "inactive",
		LastFetchAt: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
	}
	s.data["ch_error_fresh"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Format(time.RFC3339),
		LastError:   "current",
	}
	s.mu.Unlock()

	alerts := s.Alerts()
	if len(alerts) != 1 || alerts[0].ChannelID != "ch_error_fresh" {
		t.Errorf("expected only ch_error_fresh to surface, got %+v", alerts)
	}
}

// TestChannelHealthStore_Alerts_DefaultThresholdApplied verifies that
// the constructor wires DefaultStaleThreshold (1 hour) by default —
// no explicit WithStaleThreshold call needed for production use.
func TestChannelHealthStore_Alerts_DefaultThresholdApplied(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	if s.staleThreshold != DefaultStaleThreshold {
		t.Errorf("expected default staleThreshold = %v, got %v",
			DefaultStaleThreshold, s.staleThreshold)
	}
	if DefaultStaleThreshold != 1*time.Hour {
		t.Errorf("expected DefaultStaleThreshold = 1 hour, got %v", DefaultStaleThreshold)
	}
}

// TestChannelHealthStore_WithStaleThreshold_Chaining verifies the
// setter returns *ChannelHealthStore for fluent API.
func TestChannelHealthStore_WithStaleThreshold_Chaining(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)
	got := s.WithStaleThreshold(2 * time.Hour).WithNowFunc(time.Now)
	if got != s {
		t.Errorf("With* methods must return the store for chaining")
	}
	if s.staleThreshold != 2*time.Hour {
		t.Errorf("WithStaleThreshold did not apply")
	}
	if s.nowFunc == nil {
		t.Errorf("WithNowFunc did not apply")
	}
}

// TestChannelHealthStore_Record_OKClearsStaleErrors verifies that a healthy
// (status=ok) Record() clears the Errors slice left over from a previous
// failure, so the dashboard stops showing stale error text for channels that
// have since recovered (e.g. market_volume / day_trading).
func TestChannelHealthStore_Record_OKClearsStaleErrors(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	if err := s.Record("market_volume", "error", "connection refused"); err != nil {
		t.Fatalf("record error: %v", err)
	}
	rec := s.Get("market_volume")
	if rec == nil || len(rec.Errors) == 0 {
		t.Fatalf("expected stale error to be recorded, got %+v", rec)
	}

	if err := s.Record("market_volume", "ok", ""); err != nil {
		t.Fatalf("record ok: %v", err)
	}
	rec = s.Get("market_volume")
	if rec == nil {
		t.Fatal("expected record after ok")
	}
	if rec.LastError != "" {
		t.Errorf("LastError = %q, want empty after ok", rec.LastError)
	}
	if len(rec.Errors) != 0 {
		t.Errorf("Errors = %v, want empty after ok (stale error text must be cleared)", rec.Errors)
	}
}
