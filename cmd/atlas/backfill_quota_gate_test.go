package main

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TestBackfillQuotaAllowed covers the FinMind quota reserve gate for the
// auto_sbl_tdcc_history_backfill task (fix/20260906-finmind-quota-reserve).
// The backfill must defer (return false) when the shared FinMindClient's
// remaining daily quota is below finmindBackfillQuotaReserve, so the live
// scheduled fetches never starve — 2026-09-04 incident: the backfill burned
// the full 14,400-call quota and live fetches failed all day.
func TestBackfillQuotaAllowed(t *testing.T) {
	// Fresh quota — backfill allowed. Read the ceiling from marketdata so this
	// test tracks the value the client actually enforces.
	full := marketdata.FinMindDailyLimit()
	if !backfillQuotaAllowed(full) {
		t.Fatalf("full quota (%d) should allow backfill", full)
	}
	// Boundary: exactly at the reserve — allowed (inclusive).
	if !backfillQuotaAllowed(finmindBackfillQuotaReserve) {
		t.Fatalf("remaining == reserve (%d) should allow backfill", finmindBackfillQuotaReserve)
	}
	// Just below the reserve — deferred.
	if backfillQuotaAllowed(finmindBackfillQuotaReserve - 1) {
		t.Fatalf("remaining < reserve (%d) should defer backfill", finmindBackfillQuotaReserve)
	}
	// Quota exhausted (the 2026-09-04 state) — deferred.
	if backfillQuotaAllowed(0) {
		t.Fatalf("exhausted quota should defer backfill")
	}
}

// TestFinmindBackfillQuotaReserveSanity pins the reserve value: large enough
// to cover a day of live-channel FinMind calls with headroom, small enough
// that the backfill still makes progress every hour once above the reserve.
func TestFinmindBackfillQuotaReserveSanity(t *testing.T) {
	if finmindBackfillQuotaReserve <= 0 {
		t.Fatalf("reserve must be positive, got %d", finmindBackfillQuotaReserve)
	}
	if finmindBackfillQuotaReserve > 1000 {
		t.Fatalf("reserve too large (%d): backfill would stall most of the day", finmindBackfillQuotaReserve)
	}
	// The reserve is a slice of the LOCAL ceiling: if it ever reaches the
	// ceiling the backfill can never run, and the gate would silently become a
	// kill switch. (fix/finmind-quota-honor-402-r — the ceiling is now 12,000,
	// set below the ~12,500 point where FinMind actually refuses.)
	if finmindBackfillQuotaReserve >= marketdata.FinMindDailyLimit() {
		t.Fatalf("reserve %d must stay below the daily ceiling %d", finmindBackfillQuotaReserve, marketdata.FinMindDailyLimit())
	}
}
