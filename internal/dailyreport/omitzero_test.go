package dailyreport

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

// TestReportOmitzero locks in omitzero on Report.RevisedAt: a fresh report
// (workflow state all zero) must not serialize revised_at, matching the
// "legacy persisted reports stay byte-compatible" contract.
func TestReportOmitzero(t *testing.T) {
	cases := []struct {
		name string
		key  string
		set  func(*Report)
	}{
		{name: "revised_at", key: "revised_at", set: func(r *Report) { r.RevisedAt = time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC) }},
	}
	for _, tc := range cases {
		t.Run(tc.name+"-zero", func(t *testing.T) {
			assertJSONKey(t, tc.key, Report{}, false)
		})
		t.Run(tc.name+"-nonzero", func(t *testing.T) {
			v := Report{}
			tc.set(&v)
			assertJSONKey(t, tc.key, v, true)
		})
	}
}

// TestTrackedClaimOmitzero locks in omitzero on TrackedClaim.VerifiedAt.
func TestTrackedClaimOmitzero(t *testing.T) {
	cases := []struct {
		name string
		key  string
		set  func(*TrackedClaim)
	}{
		{name: "verified_at", key: "verified_at", set: func(c *TrackedClaim) { c.VerifiedAt = time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC) }},
	}
	for _, tc := range cases {
		t.Run(tc.name+"-zero", func(t *testing.T) {
			assertJSONKey(t, tc.key, TrackedClaim{}, false)
		})
		t.Run(tc.name+"-nonzero", func(t *testing.T) {
			v := TrackedClaim{}
			tc.set(&v)
			assertJSONKey(t, tc.key, v, true)
		})
	}
}
