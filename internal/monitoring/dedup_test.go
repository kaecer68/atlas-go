package monitoring

import (
	"testing"
	"time"
)

func TestNewAlertDeduplicator(t *testing.T) {
	d := NewAlertDeduplicator(5*time.Minute, nil)
	if d == nil {
		t.Fatal("NewAlertDeduplicator returned nil")
	}
}

func TestAlertDeduplicator_DedupWithinWindow(t *testing.T) {
	d := NewAlertDeduplicator(5*time.Minute, nil)

	d.Track("key-1")

	result, err := d.Check("key-1")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !result.Skip {
		t.Error("expected Skip=true for tracked key within window")
	}
}

func TestAlertDeduplicator_DedupAfterWindow(t *testing.T) {
	window := 1 * time.Millisecond
	d := NewAlertDeduplicator(window, nil)

	d.Track("key-expired")

	// Manually set the timestamp to well before the window
	d.mu.Lock()
	d.recent["key-expired"] = time.Now().Add(-1 * time.Hour)
	d.mu.Unlock()

	result, err := d.Check("key-expired")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Skip {
		t.Error("expected Skip=false for key outside window")
	}
}

func TestAlertDeduplicator_CheckNotFound(t *testing.T) {
	d := NewAlertDeduplicator(5*time.Minute, nil)

	result, err := d.Check("never-tracked")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Skip {
		t.Error("expected Skip=false for untracked key")
	}
}

func TestAlertDeduplicator_CheckWithNilStore(t *testing.T) {
	d := NewAlertDeduplicator(5*time.Minute, nil)

	d.Track("nil-store-key")

	result, err := d.Check("nil-store-key")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !result.Skip {
		t.Error("expected Skip=true for tracked key with nil store")
	}
	if result.ExistingAlertID != "" {
		t.Errorf("expected empty ExistingAlertID with nil store, got %q", result.ExistingAlertID)
	}
	if result.NewCount != 0 {
		t.Errorf("expected zero NewCount with nil store, got %d", result.NewCount)
	}
}

func TestAlertDeduplicator_Cleanup(t *testing.T) {
	window := 1 * time.Millisecond
	d := NewAlertDeduplicator(window, nil)

	// Add an entry and manually expire it
	d.Track("old-key")
	d.Track("fresh-key")

	d.mu.Lock()
	d.recent["old-key"] = time.Now().Add(-1 * time.Hour)
	d.mu.Unlock()

	d.cleanup()

	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.recent["old-key"]; ok {
		t.Error("cleanup should have removed expired key 'old-key'")
	}
	if _, ok := d.recent["fresh-key"]; !ok {
		t.Error("cleanup should have kept fresh key 'fresh-key'")
	}
}
