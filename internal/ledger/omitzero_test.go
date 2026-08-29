package ledger

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

// TestPeriodRowOmitzero locks in omitzero on PeriodRow.RecordedAt.
func TestPeriodRowOmitzero(t *testing.T) {
	cases := []struct {
		name string
		key  string
		set  func(*PeriodRow)
	}{
		{name: "recorded_at", key: "recorded_at", set: func(r *PeriodRow) { r.RecordedAt = time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC) }},
	}
	for _, tc := range cases {
		t.Run(tc.name+"-zero", func(t *testing.T) {
			assertJSONKey(t, tc.key, PeriodRow{}, false)
		})
		t.Run(tc.name+"-nonzero", func(t *testing.T) {
			v := PeriodRow{}
			tc.set(&v)
			assertJSONKey(t, tc.key, v, true)
		})
	}
}
