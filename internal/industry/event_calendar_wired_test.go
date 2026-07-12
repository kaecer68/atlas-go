package industry

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TestNewEventCalendarWithProvider_LoadsDefaultRules 驗證
// Stage 1 PR#1 wired factory 的核心承諾：建立後 events 不為空。
// 對照 NewEventCalendar() 只載 annualRules，events slice 為空，需要 caller
// 自行呼叫 RefreshEvents（本測試若失敗 = 漏呼叫 RefreshEvents）。
func TestNewEventCalendarWithProvider_LoadsDefaultRules(t *testing.T) {
	ec := NewEventCalendarWithProvider(nil)
	if ec == nil {
		t.Fatal("expected non-nil EventCalendar")
	}

	events := ec.GetAllEvents()
	if len(events) == 0 {
		t.Fatal("wired factory must populate default-rule events (Stage 1 PR#1 root cause)")
	}
	t.Logf("wired factory loaded %d default-rule events", len(events))
}

// TestNewEventCalendarWithProvider_NilProviderIsSafe 確保 nil provider 不會 panic。
func TestNewEventCalendarWithProvider_NilProviderIsSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil provider must not panic, got: %v", r)
		}
	}()
	ec := NewEventCalendarWithProvider(nil)
	if ec == nil {
		t.Fatal("expected non-nil calendar")
	}
}

// TestNewEventCalendarWithProvider_EquivalentToManualRefresh 確保 wired factory
// 與 NewEventCalendar()+RefreshEvents(now) 產生等量的 default-rule 事件。
func TestNewEventCalendarWithProvider_EquivalentToManualRefresh(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)

	wired := NewEventCalendarWithProvider(nil)
	wiredEvents := wired.GetAllEvents()

	plain := NewEventCalendar()
	plain.RefreshEvents(now)
	plainEvents := plain.GetAllEvents()

	if len(wiredEvents) != len(plainEvents) {
		t.Errorf("wired factory (%d events) and manual RefreshEvents (%d events) should match for same year",
			len(wiredEvents), len(plainEvents))
	}
}

// TestNewEventCalendarWithProvider_AcceptsProviderWithoutCallingFetch 驗證 wired
// factory 收到 provider 時只持有 reference，不會同步呼叫 FetchEvents（會 block startup）。
// 用計數 provider 觀察 FetchEvents 被呼叫次數：應為 0。
func TestNewEventCalendarWithProvider_AcceptsProviderWithoutCallingFetch(t *testing.T) {
	counter := &countingProvider{}
	ec := NewEventCalendarWithProvider(counter)

	events := ec.GetAllEvents()
	if len(events) == 0 {
		t.Fatal("default-rule events should be loaded even when provider is given")
	}
	if counter.fetchCalls != 0 {
		t.Errorf("wired factory must NOT call provider.FetchEvents synchronously, got %d calls", counter.fetchCalls)
	}
}

// countingProvider 計數 FetchEvents 呼叫次數以驗證 wired factory 的設計約束。
type countingProvider struct {
	fetchCalls int
}

func (c *countingProvider) Name() string { return "counting" }
func (c *countingProvider) FetchEvents(_ context.Context, _ int) ([]marketdata.CalendarProviderData, error) {
	c.fetchCalls++
	return nil, nil
}
