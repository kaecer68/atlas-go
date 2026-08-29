package anomaly

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

// TestStoredAnomalyOmitzero locks in omitzero on StoredAnomaly.AckedAt:
// an un-acked anomaly must not serialize acked_at.
func TestStoredAnomalyOmitzero(t *testing.T) {
	cases := []struct {
		name string
		key  string
		set  func(*StoredAnomaly)
	}{
		{name: "acked_at", key: "acked_at", set: func(s *StoredAnomaly) { s.AckedAt = time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC) }},
	}
	for _, tc := range cases {
		t.Run(tc.name+"-zero", func(t *testing.T) {
			assertJSONKey(t, tc.key, StoredAnomaly{}, false)
		})
		t.Run(tc.name+"-nonzero", func(t *testing.T) {
			v := StoredAnomaly{}
			tc.set(&v)
			assertJSONKey(t, tc.key, v, true)
		})
	}
}
