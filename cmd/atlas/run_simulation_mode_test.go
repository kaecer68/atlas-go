package main

import (
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

// TestRunSimulationMode_InvalidDateFormat locks the early-exit error path
// of runSimulationMode. Before accessing rt.MetricsCollector/Repository
// (which require a fully bootstrapped runtime), the function must validate
// dateOverride against the "2006-01-02" format and return a wrapped error.
//
// This is a safety net for #611 sub-issue-2 refactor (cmd/atlas/ package
// split): the error wrapping contract must remain stable so callers can
// programmatically detect malformed date inputs.
func TestRunSimulationMode_InvalidDateFormat(t *testing.T) {
	cases := []struct {
		name      string
		dateInput string
	}{
		{"slash_format", "2026/06/22"},
		{"us_format", "06/22/2026"},
		{"compact", "20260622"},
		{"empty_after_check", " "},
		{"month_day_swap", "22-06-2026"},
		{"trailing_garbage", "2026-06-22xyz"},
		{"two_digit_year", "26-06-22"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Pass nil rt — date validation happens BEFORE rt access.
			err := runSimulationMode(nil, configStub(t), nil, false, tc.dateInput)
			if err == nil {
				t.Fatalf("expected error for date %q, got nil", tc.dateInput)
			}
			if !strings.Contains(err.Error(), "invalid date format") {
				t.Errorf("error %q must contain 'invalid date format'", err.Error())
			}
			if !strings.Contains(err.Error(), "2006-01-02") {
				t.Errorf("error %q must mention expected format '2006-01-02'", err.Error())
			}
		})
	}
}

// TestRunSimulationMode_EmptyDateReachesRuntimeAccess locks the boundary
// between "early date validation" and "runtime field access". With an
// empty dateOverride, the date-check block is skipped and the function
// proceeds to `collector := rt.MetricsCollector` (main.go:2916). With a
// nil rt, that line panics — proving the function REQUIRES a non-nil
// Runtime when dateOverride is empty.
//
// This is a safety net for #611 sub-issue-2 refactor: the contract
// "runSimulationMode with empty date requires non-nil rt" must remain
// stable. If refactor accidentally inserts an early return for empty
// date (returning a friendlier error), that would be a behavioral
// change and this test should be updated intentionally, not silently.
func TestRunSimulationMode_EmptyDateReachesRuntimeAccess(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected nil-pointer panic at rt.MetricsCollector access, got no panic")
		}
		t.Logf("confirmed: empty-date path reaches rt.MetricsCollector (panic: %v)", r)
	}()

	_ = runSimulationMode(nil, configStub(t), nil, false, "")
}

// configStub returns a minimal Config that allows runSimulationMode to reach
// the date-validation step without filesystem dependencies. rt is nil so
// the call will fail downstream (which is the whole point of these tests).
func configStub(t *testing.T) config.Config {
	return config.Config{
		WorkDir:        t.TempDir(),
		LedgerDir:      t.TempDir(),
		BrokerMode:     "dry-run",
		BrokerAdapter:  "guarded",
		BrokerSigner:   "hmac-sha256",
		ReplayDataPath: "samples/replay/twse_stock_day_all_sample.csv",
	}
}
