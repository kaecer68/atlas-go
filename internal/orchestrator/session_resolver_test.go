package orchestrator

import (
	"errors"
	"testing"
	"time"
)

func TestReplayNextSessionResolver_Normal(t *testing.T) {
	dateStr := "2026-07-20"
	nextDate, _ := time.Parse("2006-01-02", dateStr)
	fn := func() (time.Time, bool) { return nextDate, true }
	r := NewReplayNextSessionResolver(fn)

	asOf, _ := time.Parse("2006-01-02", "2026-07-17")
	got, err := r.NextTradingSession(asOf)
	if err != nil {
		t.Fatalf("NextTradingSession: %v", err)
	}
	if got.Format("2006-01-02") != "2026-07-20" {
		t.Errorf("expected 2026-07-20, got %s", got.Format("2006-01-02"))
	}
}

func TestReplayNextSessionResolver_Exhausted(t *testing.T) {
	fn := func() (time.Time, bool) { return time.Time{}, false }
	r := NewReplayNextSessionResolver(fn)

	asOf, _ := time.Parse("2006-01-02", "2026-07-17")
	_, err := r.NextTradingSession(asOf)
	if err == nil {
		t.Fatal("expected error for exhausted dataset")
	}
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Errorf("expected ErrSessionUnavailable, got %v", err)
	}
}

func TestReplayNextSessionResolver_NotAfterAsOf(t *testing.T) {
	past, _ := time.Parse("2006-01-02", "2026-07-16")
	fn := func() (time.Time, bool) { return past, true }
	r := NewReplayNextSessionResolver(fn)

	asOf, _ := time.Parse("2006-01-02", "2026-07-17")
	_, err := r.NextTradingSession(asOf)
	if err == nil {
		t.Fatal("expected error when next is not after as_of")
	}
}

func TestNoOpNextSessionResolver(t *testing.T) {
	r := &NoOpNextSessionResolver{}
	_, err := r.NextTradingSession(time.Now())
	if err == nil {
		t.Fatal("expected error from no-op resolver")
	}
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Errorf("expected ErrSessionUnavailable, got %v", err)
	}
}
