package shared

import (
	"encoding/json"
	"testing"
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

// TestFactorScoreBreakdownOmitzero locks in omitzero on the six optional
// FactorScoreItem fields: a zero FactorScoreItem must be omitted, mirroring
// the always-present required factors (momentum/value/quality/...).
func TestFactorScoreBreakdownOmitzero(t *testing.T) {
	cases := []struct {
		name string
		key  string
		set  func(*FactorScoreBreakdown)
	}{
		{name: "narrative", key: "narrative", set: func(b *FactorScoreBreakdown) { b.Narrative = FactorScoreItem{Formula: "narrative"} }},
		{name: "industry_cycle", key: "industry_cycle", set: func(b *FactorScoreBreakdown) { b.IndustryCycle = FactorScoreItem{Formula: "industry_cycle"} }},
		{name: "precious_metals", key: "precious_metals", set: func(b *FactorScoreBreakdown) { b.PreciousMetals = FactorScoreItem{Formula: "precious_metals"} }},
		{name: "etf", key: "etf", set: func(b *FactorScoreBreakdown) { b.ETF = FactorScoreItem{Formula: "etf"} }},
		{name: "linkage", key: "linkage", set: func(b *FactorScoreBreakdown) { b.Linkage = FactorScoreItem{Formula: "linkage"} }},
		{name: "tsmc", key: "tsmc", set: func(b *FactorScoreBreakdown) { b.TSMC = FactorScoreItem{Formula: "tsmc"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name+"-zero", func(t *testing.T) {
			assertJSONKey(t, tc.key, FactorScoreBreakdown{}, false)
		})
		t.Run(tc.name+"-nonzero", func(t *testing.T) {
			v := FactorScoreBreakdown{}
			tc.set(&v)
			assertJSONKey(t, tc.key, v, true)
		})
	}
}
