package marketdata

import (
	"testing"
)

func TestDayTradingProvider_Name(t *testing.T) {
	p := NewDayTradingProvider()
	if p.Name() != "twse_day_trading" {
		t.Errorf("Name() = %q, want %q", p.Name(), "twse_day_trading")
	}
}

func TestParseTWSEInt(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"1,234,567", 1234567},
		{"", 0},
		{"0", 0},
		{"100", 100},
		{"abc", 0},
	}

	for _, tc := range tests {
		result := parseTWSEInt(tc.input)
		if result != tc.expected {
			t.Errorf("parseTWSEInt(%q) = %d, want %d", tc.input, result, tc.expected)
		}
	}
}

func TestParseTWSEPercent(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"1.5", 0.015},
		{"", 0},
		{"0", 0},
		{"100", 1.0},
		{"-5.0", -0.05},
		{"abc", 0},
	}

	for _, tc := range tests {
		result := parseTWSEPercent(tc.input)
		if result != tc.expected {
			t.Errorf("parseTWSEPercent(%q) = %f, want %f", tc.input, result, tc.expected)
		}
	}
}

func TestNewDayTradingProvider(t *testing.T) {
	p := NewDayTradingProvider()
	if p == nil {
		t.Fatal("NewDayTradingProvider returned nil")
	}
	if p.Name() != "twse_day_trading" {
		t.Errorf("Name() = %q, want %q", p.Name(), "twse_day_trading")
	}
}
