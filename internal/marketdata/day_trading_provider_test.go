package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/time/rate"
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

// TestDayTradingProvider_FetchLatest_PreservesLastError verifies that when
// every date in the 7-day window fails, FetchLatest returns the stable
// "no TWSE day trading data available in the last 7 days" prefix PLUS the
// last underlying error (diagnosability — the previous version swallowed
// per-date errors entirely).
func TestDayTradingProvider_FetchLatest_PreservesLastError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	p := NewDayTradingProvider()
	p.SetHTTPClient(srv.Client())
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	// Point baseURL at the test server via the exported-for-tests hook.
	p.SetBaseURL(srv.URL)

	_, err := p.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("FetchLatest() error = nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "no TWSE day trading data available in the last 7 days") {
		t.Errorf("error missing stable prefix: %q", msg)
	}
	if !strings.Contains(msg, "last error") {
		t.Errorf("error missing last-error detail: %q", msg)
	}
}
