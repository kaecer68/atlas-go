package industry

import (
	"testing"
)

// TestEventCalendar_DefaultRulesHaveEventType 驗證 PR-A minor polish：
// 修前 10 個 build*Event helper 都漏 EventType: rule.EventType 欄位，
// 導致 active_events response event_type=""。
// 修法：sed 一次插入 EventType: rule.EventType 到所有 10 個 helper。
func TestEventCalendar_DefaultRulesHaveEventType(t *testing.T) {
	ec := NewEventCalendarWithProvider(nil)
	if ec == nil {
		t.Fatal("expected non-nil calendar")
	}

	events := ec.GetAllEvents()
	if len(events) == 0 {
		t.Fatal("expected default-rule events to be loaded")
	}

	seenTypes := make(map[string]bool)
	for _, evt := range events {
		if evt.EventType == "" {
			t.Errorf("event %q (start=%s) has empty EventType — build*Event helper missing rule.EventType propagation",
				evt.Name, evt.StartDate.Format("2006-01-02"))
		}
		seenTypes[evt.EventType] = true
	}

	expectedTypes := []string{
		"ex_dividend", "shareholder_meeting", "window_dressing",
		"msci_rebalance", "financial_report", "investor_conference",
		"monthly_revenue", "futures_settlement", "position_building",
	}
	for _, want := range expectedTypes {
		if !seenTypes[want] {
			t.Errorf("expected at least one event with EventType=%q, got types: %v", want, seenTypes)
		}
	}
}
