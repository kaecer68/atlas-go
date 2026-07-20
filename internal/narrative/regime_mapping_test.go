package narrative

import "testing"

// TestRegimeMapping_AllCombinations covers C03: every stress vocabulary token
// has a regime mapping, every regime vocabulary token has a stress mapping,
// and no entry is the identity (which would imply the vocabularies are the
// same — they aren't). This locks in the bidirectional coverage so future
// vocabulary additions can't silently break the cross-walk.
func TestRegimeMapping_AllCombinations(t *testing.T) {
	for _, s := range StressVocabulary {
		mapped, ok := RegimeVocabularyMapping[s]
		if !ok {
			t.Errorf("stress vocabulary token %q has no regime mapping", s)
		}
		if mapped == s {
			t.Errorf("stress token %q maps to itself — vocabularies should differ", s)
		}
	}
	for _, r := range RegimeVocabulary {
		mapped, ok := RegimeVocabularyMapping[r]
		if !ok {
			t.Errorf("regime vocabulary token %q has no stress mapping", r)
		}
		if mapped == r {
			t.Errorf("regime token %q maps to itself — vocabularies should differ", r)
		}
	}
}

// TestNormalizeRegime covers C03: NormalizeRegime should return the
// regime-vocabulary form for inputs from either vocabulary, and pass through
// unknown values unchanged so future vocabulary additions don't get silently
// rewritten.
func TestNormalizeRegime(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"low", "RISK_ON"},
		{"alert", "NEUTRAL"},
		{"high", "RISK_OFF"},
		{"crisis", "RISK_OFF"},
		{"RISK_ON", "RISK_ON"},
		{"RISK_OFF", "RISK_OFF"},
		{"NEUTRAL", "NEUTRAL"},
		{"TRANSITIONAL", "TRANSITIONAL"},
		{"unknown_future_token", "unknown_future_token"},
		{"", ""},
	}
	for _, tc := range cases {
		got := NormalizeRegime(tc.in)
		if got != tc.want {
			t.Errorf("NormalizeRegime(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
