package shared

import "testing"

func TestAdjustType_String(t *testing.T) {
	cases := []struct {
		value AdjustType
		want  string
	}{
		{AdjustCashDividend, "cash_dividend"},
		{AdjustStockDividend, "stock_dividend"},
		{AdjustCapitalReduction, "capital_reduction"},
		{AdjustType(999), "unknown"},
	}

	for _, tc := range cases {
		got := tc.value.String()
		if got != tc.want {
			t.Errorf("AdjustType(%d).String() = %q, want %q", tc.value, got, tc.want)
		}
	}
}
