package apigateway

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

// TestTaskStatusOmitzero locks in omitzero on the zero-value time fields:
// a zero TaskStatus must not serialize last_data_as_of / last_persisted_at,
// and a non-zero value must keep the key.
func TestTaskStatusOmitzero(t *testing.T) {
	cases := []struct {
		name string
		key  string
		set  func(*TaskStatus)
	}{
		{name: "last_data_as_of", key: "last_data_as_of", set: func(s *TaskStatus) { s.LastDataAsOf = time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC) }},
		{name: "last_persisted_at", key: "last_persisted_at", set: func(s *TaskStatus) { s.LastPersistedAt = time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC) }},
	}
	for _, tc := range cases {
		t.Run(tc.name+"-zero", func(t *testing.T) {
			assertJSONKey(t, tc.key, TaskStatus{}, false)
		})
		t.Run(tc.name+"-nonzero", func(t *testing.T) {
			v := TaskStatus{}
			tc.set(&v)
			assertJSONKey(t, tc.key, v, true)
		})
	}
}

// TestDataStateOmitzero locks in omitzero on DataState.RecordedAt.
func TestDataStateOmitzero(t *testing.T) {
	cases := []struct {
		name string
		key  string
		set  func(*DataState)
	}{
		{name: "recorded_at", key: "recorded_at", set: func(s *DataState) { s.RecordedAt = time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC) }},
	}
	for _, tc := range cases {
		t.Run(tc.name+"-zero", func(t *testing.T) {
			assertJSONKey(t, tc.key, DataState{}, false)
		})
		t.Run(tc.name+"-nonzero", func(t *testing.T) {
			v := DataState{}
			tc.set(&v)
			assertJSONKey(t, tc.key, v, true)
		})
	}
}
