package domain

import "testing"

func TestFormatNTD(t *testing.T) {
	tests := []struct {
		name     string
		amount   float64
		expected string
	}{
		{name: "positive", amount: 1234.56, expected: "NT$1,234.56"},
		{name: "negative", amount: -100, expected: "NT$-100.00"},
		{name: "zero", amount: 0, expected: "NT$0.00"},
		{name: "large value", amount: 1234567890.99, expected: "NT$1,234,567,890.99"},
		{name: "small decimal", amount: 0.99, expected: "NT$0.99"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatNTD(tt.amount)
			if got != tt.expected {
				t.Errorf("FormatNTD(%v) = %q, want %q", tt.amount, got, tt.expected)
			}
		})
	}
}
