package narrative

import "testing"

func TestValidationFrameworkWithHistoricalCases(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil, "")
	cases := DefaultStressEventTestCases()

	report := ValidateAgainstCases(calc, cases)
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.Results) != len(cases) {
		t.Fatalf("expected %d results, got %d", len(cases), len(report.Results))
	}
	if len(report.FailedCases) == len(cases) {
		t.Fatalf("expected at least one passing case, got %v", report.FailedCases)
	}
	if report.OverallPassRate <= 0 || report.OverallPassRate > 1 {
		t.Fatalf("expected pass rate in (0,1], got %.2f", report.OverallPassRate)
	}

	for _, result := range report.Results {
		if result.ActualScore < 0 || result.ActualScore > 100 {
			t.Fatalf("case %q produced invalid score %.2f", result.CaseName, result.ActualScore)
		}
	}
}

func TestValidationFrameworkCasesAreReasonable(t *testing.T) {
	cases := DefaultStressEventTestCases()
	if len(cases) != 5 {
		t.Fatalf("expected 5 cases, got %d", len(cases))
	}

	for _, tc := range cases {
		if tc.Name == "" || tc.Date == "" || tc.ExpectedRegime == "" || tc.Rationale == "" {
			t.Fatalf("case has missing fields: %+v", tc)
		}
		if tc.Window <= 0 {
			t.Fatalf("case has invalid window: %+v", tc)
		}
	}
}
