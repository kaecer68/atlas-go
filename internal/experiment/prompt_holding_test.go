package experiment

import "testing"

func TestPromptMentionsHoldingPeriod(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		want   bool
	}{
		{"holding_period_keyword", "Strategy uses holding_period=10", true},
		{"max_holding_days_keyword", "Constraint: max_holding_days=20", true},
		{"holding_days_text", "Exit after 15 holding days", true},
		{"max_holding_text", "Max holding 30 days for swing trades", true},
		{"exit_rule_keyword", "Strategy exit_rule: close after 5 days", true},
		{"no_mention", "Strategy uses momentum and trend following only", false},
		{"case_insensitive", "HOLDING_PERIOD constraint applied", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := promptMentionsHoldingPeriod(tc.prompt)
			if got != tc.want {
				t.Errorf("promptMentionsHoldingPeriod(%q) = %v, want %v", tc.prompt, got, tc.want)
			}
		})
	}
}
