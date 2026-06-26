package monitoring

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestCleanupStaleHeartbeats_RemovesOnlyOldChannelHealthSummaries verifies
// that the one-time cleanup deletes only:
//   - alerts with Rule == "channel_health_summary"
//   - AND Timestamp older than (now - ttl)
//
// Alerts with other Rules or fresh Timestamps are preserved.
func TestCleanupStaleHeartbeats_RemovesOnlyOldChannelHealthSummaries(t *testing.T) {
	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	now := time.Now()
	alerts := []domain.AlertRecord{
		// Should be deleted: old channel_health_summary
		{ID: "old-1", Rule: "channel_health_summary", Timestamp: now.Add(-30 * time.Minute), Severity: "info"},
		{ID: "old-2", Rule: "channel_health_summary", Timestamp: now.Add(-25 * time.Minute), Severity: "info"},
		// Should be preserved: fresh channel_health_summary
		{ID: "fresh-1", Rule: "channel_health_summary", Timestamp: now.Add(-1 * time.Minute), Severity: "info"},
		// Should be preserved: old but different rule
		{ID: "old-drawdown", Rule: "drawdown", Timestamp: now.Add(-30 * time.Minute), Severity: "critical"},
		// Should be preserved: fresh drawdown
		{ID: "fresh-drawdown", Rule: "drawdown", Timestamp: now.Add(-1 * time.Minute), Severity: "critical"},
	}
	for _, a := range alerts {
		if err := store.Save(a); err != nil {
			t.Fatalf("Save %s: %v", a.ID, err)
		}
	}
	// Run cleanup with 5-minute TTL (matches default HeartbeatTTLMinutes).
	deleted, err := CleanupStaleHeartbeats(store, 5*time.Minute)
	if err != nil {
		t.Fatalf("CleanupStaleHeartbeats: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 alerts deleted, got %d", deleted)
	}
	// Verify remaining alerts
	remaining, _ := store.LoadAll()
	remainingIDs := make(map[string]bool)
	for _, a := range remaining {
		remainingIDs[a.ID] = true
	}
	expectedRemaining := []string{"fresh-1", "old-drawdown", "fresh-drawdown"}
	if len(remaining) != len(expectedRemaining) {
		t.Errorf("expected %d remaining, got %d (ids=%v)",
			len(expectedRemaining), len(remaining), remainingIDs)
	}
	for _, id := range expectedRemaining {
		if !remainingIDs[id] {
			t.Errorf("expected %s to be preserved, but it was deleted", id)
		}
	}
	for _, id := range []string{"old-1", "old-2"} {
		if remainingIDs[id] {
			t.Errorf("expected %s to be deleted, but it remains", id)
		}
	}
}

// TestCleanupStaleHeartbeats_EmptyStore verifies the cleanup is a no-op
// when the store is empty (returns 0, nil).
func TestCleanupStaleHeartbeats_EmptyStore(t *testing.T) {
	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	deleted, err := CleanupStaleHeartbeats(store, 5*time.Minute)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}

// stub to keep import (eventbus is referenced in comments only)
var _ = domain.AlertRecord{}
