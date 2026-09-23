package apigateway

// 2026-09-23 bdi fix regression tests.
//
// 生產事故 (2026-09-20T08:35Z → 09-23): CNBC answered the Baltic Dry Index
// symbol `.BADI` with a structurally valid quote that carried no price (no
// `last`, open/high/low "0.00", provider "CNBC Quote Cache"). Every 5m tick
// therefore produced "bdi: missing last price field", the gateway counted it
// as a failure, and after CircuitBreakerFailureThreshold ticks the bdi breaker
// opened — from then on channel_fetch_log showed
// "circuit breaker open for channel bdi" and the channel page showed a raw
// error, while task_liveness.macro_cache_bdi accumulated 800+ consecutive
// failures.
//
// Required semantics after the fix:
//   - the empty-quote sentinel is recorded as "warn" (never "error");
//   - it never accrues circuit-breaker failures (no-op, not a success reset);
//   - genuine failures (transport / HTTP / parse / empty payload array) keep
//     the old semantics and still open the breaker;
//   - no data lands, so last_success must not advance (no silent "ok").

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TestGateway_Fetch_ErrEmptyQuote_RecordsWarnAndKeepsBreakerClosed is the
// regression guard for the breaker-open half of the incident: an upstream that
// politely answers 200 with no price must not be able to open the channel.
func TestGateway_Fetch_ErrEmptyQuote_RecordsWarnAndKeepsBreakerClosed(t *testing.T) {
	g := newTestGateway(t)
	const channelID = "bdi"
	emptyErr := fmt.Errorf("bdi: missing last price field: %w", marketdata.ErrEmptyQuote)
	registerFailingProvider(g, channelID, emptyErr)

	// Fetch more times than the breaker threshold: an empty quote must never
	// accumulate enough failures to open the channel.
	for i := range CircuitBreakerFailureThreshold + 2 {
		_, err := g.Fetch(context.Background(), channelID)
		if err == nil {
			t.Fatalf("tick %d: Fetch must still surface the empty-quote error to callers", i)
		}
		if !errors.Is(err, marketdata.ErrEmptyQuote) {
			t.Fatalf("tick %d: err = %v, want wrapped marketdata.ErrEmptyQuote", i, err)
		}
	}

	st := g.BreakerStatus()[channelID]
	if st.State != "closed" {
		t.Errorf("breaker state = %q, want closed (expected-empty must not trip the breaker)", st.State)
	}
	if st.Failures != 0 {
		t.Errorf("breaker failures = %d, want 0 (expected-empty is a breaker no-op)", st.Failures)
	}

	rec := g.Health().Get(channelID)
	if rec == nil {
		t.Fatal("expected health record after fetch")
	}
	if rec.Status != "warn" {
		t.Errorf("Status = %q, want warn (empty quote is an upstream data outage, not an atlas error)", rec.Status)
	}
	if rec.LastError == "" {
		t.Error("LastError must carry the empty-quote reason so the channel page explains the yellow state")
	}
	if rec.LastSuccessAt != "" {
		t.Errorf("LastSuccessAt = %q, want empty (no data landed, so freshness must not advance)", rec.LastSuccessAt)
	}
	if rec.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0 (warn is not a failure streak)", rec.ConsecutiveFailures)
	}
}

// TestGateway_Fetch_ErrEmptyQuote_DoesNotDisableRealFailureDetection proves the
// no-op is scoped to the typed sentinel: after a long run of empty quotes, a
// genuine upstream failure still opens the breaker, so the fix cannot be used
// (or misread) as "stop paging for bdi".
func TestGateway_Fetch_ErrEmptyQuote_DoesNotDisableRealFailureDetection(t *testing.T) {
	g := newTestGateway(t)
	const channelID = "bdi"
	registerFailingProvider(g, channelID,
		fmt.Errorf("bdi: missing last price field: %w", marketdata.ErrEmptyQuote))

	for i := range CircuitBreakerFailureThreshold * 2 {
		if _, err := g.Fetch(context.Background(), channelID); err == nil {
			t.Fatalf("empty-quote tick %d: expected error", i)
		}
	}
	if st := g.BreakerStatus()[channelID]; st.State != "closed" {
		t.Fatalf("precondition: breaker should still be closed after empty quotes, got %q", st.State)
	}

	// The upstream really breaks now (transport/HTTP-level failure).
	registerFailingProvider(g, channelID, fmt.Errorf("bdi fetch: %w", marketdata.ErrUpstream))
	for i := range CircuitBreakerFailureThreshold {
		if _, err := g.Fetch(context.Background(), channelID); err == nil {
			t.Fatalf("genuine failure tick %d: Fetch must return the upstream error", i)
		}
	}

	st := g.BreakerStatus()[channelID]
	if st.State != "open" {
		t.Errorf("breaker state = %q, want open after %d genuine failures (empty-quote no-op must not mask real outages)",
			st.State, CircuitBreakerFailureThreshold)
	}
	if rec := g.Health().Get(channelID); rec == nil || rec.Status != "error" {
		t.Errorf("health status = %+v, want error for a genuine repeat failure", rec)
	}
}

// TestGateway_Fetch_ErrNoData_DoesNotAccumulateBreakerFailures closes the
// taxonomy gap the P1-9 comment in marketdata/errors.go already promised
// ("ErrNoData ... must NOT trip a circuit breaker"): waiting-for-publication
// channels (tdcc weekly snapshot, taiex weekend/pre-market) no longer
// accumulate breaker failures either, while still keeping status "ok".
func TestGateway_Fetch_ErrNoData_DoesNotAccumulateBreakerFailures(t *testing.T) {
	g := newTestGateway(t)
	const channelID = "tdcc_equity_dispersion"
	noDataErr := fmt.Errorf("tdcc: no dispersion data for 20260918 (weekly snapshot may not be published yet): %w", marketdata.ErrNoData)
	registerFailingProvider(g, channelID, noDataErr)

	for i := range CircuitBreakerFailureThreshold + 2 {
		if _, err := g.Fetch(context.Background(), channelID); !errors.Is(err, marketdata.ErrNoData) {
			t.Fatalf("tick %d: err = %v, want wrapped marketdata.ErrNoData", i, err)
		}
	}

	st := g.BreakerStatus()[channelID]
	if st.State != "closed" || st.Failures != 0 {
		t.Errorf("breaker = %+v, want closed with 0 failures (ErrNoData is an expected state, not an outage)", st)
	}
	if rec := g.Health().Get(channelID); rec == nil || rec.Status != "ok" {
		t.Errorf("health record = %+v, want status ok (waiting state is unchanged by this fix)", rec)
	}
}
