package eventdriven

import (
	"encoding/json"
	"testing"
	"time"
)

// assertJSONKey verifies that marshaling v either contains key (wantPresent)
// or omits it. Used to lock in omitzero behavior for zero-value fields.
func assertJSONKey(t *testing.T, key string, v any, wantPresent bool) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", b, err)
	}
	_, ok := m[key]
	if ok != wantPresent {
		t.Errorf("key %q present=%v, want %v (marshaled: %s)", key, ok, wantPresent, b)
	}
}

// TestEventCalendarItemOmitzero locks in omitzero on the zero-value time
// fields peak_date / generated_at.
func TestEventCalendarItemOmitzero(t *testing.T) {
	cases := []struct {
		name string
		key  string
		set  func(*EventCalendarItem)
	}{
		{name: "peak_date", key: "peak_date", set: func(e *EventCalendarItem) { e.PeakDate = time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC) }},
		{name: "generated_at", key: "generated_at", set: func(e *EventCalendarItem) { e.GeneratedAt = time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC) }},
	}
	for _, tc := range cases {
		t.Run(tc.name+"-zero", func(t *testing.T) {
			assertJSONKey(t, tc.key, EventCalendarItem{}, false)
		})
		t.Run(tc.name+"-nonzero", func(t *testing.T) {
			v := EventCalendarItem{}
			tc.set(&v)
			assertJSONKey(t, tc.key, v, true)
		})
	}
}
