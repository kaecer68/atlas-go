package eventquality

import (
	"testing"
	"time"
)

func crossSourceFixedNow() time.Time {
	return time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
}

func TestCrossSourceStore_FirstSourceReturnsPending(t *testing.T) {
	s := NewCrossSourceStore(0)
	s.SetClock(crossSourceFixedNow)

	status := s.Record("twse_provider", "earnings_release", "2330", crossSourceFixedNow())
	if status != StatusPending {
		t.Errorf("first source: got %q, want %q", status, StatusPending)
	}
}

func TestCrossSourceStore_SecondSourceReturnsConfirmed(t *testing.T) {
	s := NewCrossSourceStore(0)
	s.SetClock(crossSourceFixedNow)

	_ = s.Record("twse_provider", "earnings_release", "2330", crossSourceFixedNow())
	status := s.Record("finmind_provider", "earnings_release", "2330", crossSourceFixedNow())
	if status != StatusConfirmed {
		t.Errorf("second source: got %q, want %q", status, StatusConfirmed)
	}
}

func TestCrossSourceStore_SameSourceDoesNotDoubleCount(t *testing.T) {
	s := NewCrossSourceStore(0)
	s.SetClock(crossSourceFixedNow)

	_ = s.Record("twse_provider", "earnings_release", "2330", crossSourceFixedNow())
	status := s.Record("twse_provider", "earnings_release", "2330", crossSourceFixedNow())
	if status != StatusPending {
		t.Errorf("same source twice: got %q, want %q", status, StatusPending)
	}
}

func TestCrossSourceStore_DifferentKeyIsIndependent(t *testing.T) {
	s := NewCrossSourceStore(0)
	s.SetClock(crossSourceFixedNow)

	_ = s.Record("twse_provider", "earnings_release", "2330", crossSourceFixedNow())
	status := s.Record("finmind_provider", "msci_rebalance", "2330", crossSourceFixedNow())
	if status != StatusPending {
		t.Errorf("different theme: got %q, want %q", status, StatusPending)
	}

	status2 := s.Record("finmind_provider", "earnings_release", "2454", crossSourceFixedNow())
	if status2 != StatusPending {
		t.Errorf("different symbol: got %q, want %q", status2, StatusPending)
	}
}

func TestCrossSourceStore_StatusWithoutRecording(t *testing.T) {
	s := NewCrossSourceStore(0)
	s.SetClock(crossSourceFixedNow)

	_ = s.Record("twse_provider", "earnings_release", "2330", crossSourceFixedNow())
	_ = s.Record("finmind_provider", "earnings_release", "2330", crossSourceFixedNow())

	status := s.Status("earnings_release", "2330", crossSourceFixedNow())
	if status != StatusConfirmed {
		t.Errorf("status after two sources: got %q, want %q", status, StatusConfirmed)
	}

	status2 := s.Status("unknown_theme", "2330", crossSourceFixedNow())
	if status2 != "" {
		t.Errorf("unrecorded key: got %q, want empty", status2)
	}
}

func TestCrossSourceStore_TTLExpiry(t *testing.T) {
	clock := crossSourceFixedNow()
	s := NewCrossSourceStore(0) // default 7-day TTL
	s.SetClock(func() time.Time { return clock })

	_ = s.Record("twse_provider", "earnings_release", "2330", clock)

	clock = clock.Add(8 * 24 * time.Hour) // 8 days later
	_ = s.Record("finmind_provider", "earnings_release", "2330", clock)

	// First source entry expired, so tally should be 1 not 2
	status := s.Status("earnings_release", "2330", clock)
	if status != StatusPending {
		t.Errorf("after TTL expiry of first source: got %q, want %q", status, StatusPending)
	}
}

func TestCrossSourceStore_CrossSourceKeyFormat(t *testing.T) {
	got := crossSourceKey("earnings_release", "2330",
		time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC))
	want := "earnings_release|2330|2026-07-13"
	if got != want {
		t.Errorf("key = %q, want %q", got, want)
	}
}
