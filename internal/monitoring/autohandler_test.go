package monitoring

import (
	"testing"
	"time"
)

func TestNewAutoHandler(t *testing.T) {
	t.Run("with nil store", func(t *testing.T) {
		h := NewAutoHandler(nil, nil)
		if h == nil {
			t.Fatal("NewAutoHandler returned nil")
		}
		if h.alertStore != nil {
			t.Error("expected nil alertStore")
		}
		if h.rules == nil {
			t.Error("rules should be non-nil empty slice")
		}
		if len(h.rules) != 0 {
			t.Errorf("rules len = %d, want 0", len(h.rules))
		}
	})

	t.Run("with store and rules", func(t *testing.T) {
		store := newTestStore(t)
		rules := []SuppressRule{
			{Category: "db", Duration: 5 * time.Minute},
			{Category: "network", Duration: 10 * time.Minute},
		}
		h := NewAutoHandler(store, rules)
		if h.alertStore != store {
			t.Error("alertStore mismatch")
		}
		if len(h.rules) != 2 {
			t.Fatalf("rules len = %d, want 2", len(h.rules))
		}
		if h.rules[0].Category != "db" {
			t.Errorf("rules[0].Category = %q, want db", h.rules[0].Category)
		}
		if h.rules[1].Duration != 10*time.Minute {
			t.Errorf("rules[1].Duration = %v, want 10m", h.rules[1].Duration)
		}
	})

	t.Run("nil rules becomes empty slice", func(t *testing.T) {
		h := NewAutoHandler(nil, nil)
		if h.rules == nil {
			t.Error("rules should not be nil after construction")
		}
		if len(h.rules) != 0 {
			t.Errorf("rules len = %d, want 0", len(h.rules))
		}
	})
}

func TestAutoHandler_HandleInfo_AutoAcknowledges(t *testing.T) {
	store := newTestStore(t)

	// Pre-save an alert record so Acknowledge can find it by ID.
	alertID := "info-alert-1"
	rec := makeAlert(alertID)
	if err := store.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	h := NewAutoHandler(store, nil)
	h.Handle(Alert{
		ID:        alertID,
		Level:     AlertLevelInfo,
		Category:  "test",
		Message:   "info message",
		Timestamp: time.Now(),
	})

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("LoadAll len = %d, want 1", len(records))
	}
	if !records[0].Acknowledged {
		t.Error("INFO alert should be auto-acknowledged")
	}
	if records[0].AcknowledgedBy != "auto-handler" {
		t.Errorf("AcknowledgedBy = %q, want auto-handler", records[0].AcknowledgedBy)
	}
}

func TestAutoHandler_HandleWarning_NoAutoAck(t *testing.T) {
	store := newTestStore(t)

	alertID := "warn-alert-1"
	rec := makeAlert(alertID)
	if err := store.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	h := NewAutoHandler(store, nil)
	h.Handle(Alert{
		ID:        alertID,
		Level:     AlertLevelWarning,
		Category:  "test",
		Message:   "warning message",
		Timestamp: time.Now(),
	})

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("LoadAll len = %d, want 1", len(records))
	}
	if records[0].Acknowledged {
		t.Error("WARNING alert should NOT be auto-acknowledged")
	}
}

func TestAutoHandler_HandleSuppressed(t *testing.T) {
	store := newTestStore(t)

	alertID := "suppressed-alert-1"
	rec := makeAlert(alertID)
	if err := store.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Manually set a suppression entry for "test-category" that is still active.
	h := NewAutoHandler(store, nil)
	h.mu.Lock()
	h.suppressUntil["test-category"] = time.Now().Add(1 * time.Hour)
	h.mu.Unlock()

	h.Handle(Alert{
		ID:        alertID,
		Level:     AlertLevelInfo,
		Category:  "test-category",
		Message:   "INFO alerts are always auto-acknowledged even when suppressed",
		Timestamp: time.Now(),
	})

	// INFO alerts are auto-acknowledged regardless of suppression.
	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("LoadAll len = %d, want 1", len(records))
	}
	if !records[0].Acknowledged {
		t.Error("INFO alert should be auto-acknowledged even when category is suppressed")
	}
}

func TestAutoHandler_HandleNilStore_NoPanic(t *testing.T) {
	h := NewAutoHandler(nil, nil)

	// Must not panic when store is nil and alert is INFO.
	h.Handle(Alert{
		ID:        "nil-store-alert",
		Level:     AlertLevelInfo,
		Category:  "test",
		Message:   "should not panic",
		Timestamp: time.Now(),
	})
}

func TestAutoHandler_SuppressRuleExpiry(t *testing.T) {
	store := newTestStore(t)
	alertID := "expiry-alert"

	// Pre-save so Acknowledge can find it.
	rec := makeAlert(alertID)
	if err := store.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	h := NewAutoHandler(store, nil)

	// Set suppression that has already expired.
	h.mu.Lock()
	h.suppressUntil["expired-cat"] = time.Now().Add(-1 * time.Second)
	h.mu.Unlock()

	// Expired suppression should not block the alert.
	h.Handle(Alert{
		ID:        alertID,
		Level:     AlertLevelInfo,
		Category:  "expired-cat",
		Message:   "should go through",
		Timestamp: time.Now(),
	})

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("LoadAll len = %d, want 1", len(records))
	}
	if !records[0].Acknowledged {
		t.Error("alert should be auto-acknowledged after suppression expired")
	}

	// Verify the expired entry was cleaned up.
	h.mu.Lock()
	_, exists := h.suppressUntil["expired-cat"]
	h.mu.Unlock()
	if exists {
		t.Error("expired suppression entry should have been removed")
	}
}

func TestAutoHandler_HandleError_NoAutoAck(t *testing.T) {
	store := newTestStore(t)

	alertID := "error-alert-1"
	rec := makeAlert(alertID)
	if err := store.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	h := NewAutoHandler(store, nil)
	h.Handle(Alert{
		ID:        alertID,
		Level:     AlertLevelError,
		Category:  "test",
		Message:   "error message",
		Timestamp: time.Now(),
	})

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if records[0].Acknowledged {
		t.Error("ERROR alert should NOT be auto-acknowledged")
	}
}

func TestAutoHandler_HandleCritical_NoAutoAck(t *testing.T) {
	store := newTestStore(t)

	alertID := "critical-alert-1"
	rec := makeAlert(alertID)
	if err := store.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	h := NewAutoHandler(store, nil)
	h.Handle(Alert{
		ID:        alertID,
		Level:     AlertLevelCritical,
		Category:  "test",
		Message:   "critical message",
		Timestamp: time.Now(),
	})

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if records[0].Acknowledged {
		t.Error("CRITICAL alert should NOT be auto-acknowledged")
	}
}
