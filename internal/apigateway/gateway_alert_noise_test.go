package apigateway

// 2026-09-03 告警降噪 regression tests: expected non-failure fetch outcomes
// (upstream has no new data yet / daily budget exhausted) must not mark the
// channel health record as "error", otherwise ChannelHealthStatusError
// alerts fire on every scheduled tick (tdcc weekly snapshot days / FinMind
// quota exhaustion are the production cases that motivated this).

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// registerFailingProvider registers a provider whose fetcher returns the
// given error for channelID.
func registerFailingProvider(g *Gateway, channelID string, err error) {
	g.registry.Register(channelID, &HTTPProvider{
		name:    channelID,
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: channelID},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return nil, err
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "error", LastError: err.Error()}, err
		},
	})
}

// TestGateway_Fetch_ErrNoData_RecordsWaiting verifies that an ErrNoData
// fetch outcome (e.g. "tdcc: no dispersion data for ... (weekly snapshot may
// not be published yet)", now wrapped with marketdata.ErrNoData) keeps the
// health record at status "ok" without advancing last_success — the channel
// is waiting for upstream publication, not failing.
func TestGateway_Fetch_ErrNoData_RecordsWaiting(t *testing.T) {
	g := newTestGateway(t)
	channelID := "tdcc_equity_dispersion"
	noDataErr := fmt.Errorf("tdcc: no dispersion data for 20260902 (weekly snapshot may not be published yet): %w", marketdata.ErrNoData)
	registerFailingProvider(g, channelID, noDataErr)

	_, err := g.Fetch(context.Background(), channelID)
	if err == nil {
		t.Fatal("Fetch should still surface the no-data error to callers")
	}
	if !errors.Is(err, marketdata.ErrNoData) {
		t.Fatalf("err = %v, want wrapped marketdata.ErrNoData", err)
	}

	rec := g.Health().Get(channelID)
	if rec == nil {
		t.Fatal("expected health record after fetch")
	}
	if rec.Status != "ok" {
		t.Errorf("Status = %q, want ok (waiting-for-data must not alert as error)", rec.Status)
	}
	if rec.LastSuccessAt != "" {
		t.Errorf("LastSuccessAt = %q, want empty (no data landed → last_success must not advance)", rec.LastSuccessAt)
	}
	if rec.LastError != "" {
		t.Errorf("LastError = %q, want empty on a waiting (ok) record", rec.LastError)
	}
}

// TestGateway_Fetch_ErrQuotaExhausted_RecordsWarn verifies that FinMind
// daily-quota exhaustion (twse_sbl channel) is recorded as "warn" — the
// 00:00 TW reset self-heals it, so it must not page as an error.
func TestGateway_Fetch_ErrQuotaExhausted_RecordsWarn(t *testing.T) {
	g := newTestGateway(t)
	channelID := "twse_sbl"
	quotaErr := fmt.Errorf("twse_sbl: finmind fetch 20260902: %w (used=14400, remaining=0)", marketdata.ErrQuotaExhausted)
	registerFailingProvider(g, channelID, quotaErr)

	_, err := g.Fetch(context.Background(), channelID)
	if err == nil {
		t.Fatal("Fetch should still surface the quota error to callers")
	}
	rec := g.Health().Get(channelID)
	if rec == nil {
		t.Fatal("expected health record after fetch")
	}
	if rec.Status != "warn" {
		t.Errorf("Status = %q, want warn (quota exhaustion is a waiting state, not an error)", rec.Status)
	}
	if rec.LastError == "" {
		t.Error("LastError should carry the quota reason for the channel page")
	}
	if !errors.Is(err, marketdata.ErrQuotaExhausted) {
		t.Fatalf("err = %v, want wrapped marketdata.ErrQuotaExhausted", err)
	}
}

// TestGateway_Fetch_ErrFugleQuotaExhausted_RecordsWarn mirrors the FinMind
// quota warn mapping for Fugle: the LOCAL daily-quota gate (2000/day, resets
// 00:00 UTC) is a budget condition, not an outage — 實證 2026-09-04 生產事故:
// 未映射時 fugle 額度耗盡全日以 error 級告警（ChannelHealthStatusError）。
func TestGateway_Fetch_ErrFugleQuotaExhausted_RecordsWarn(t *testing.T) {
	g := newTestGateway(t)
	channelID := "fugle"
	quotaErr := fmt.Errorf("fugle fetch: fugle: %w (used=2000, remaining=0)", marketdata.ErrFugleQuotaExhausted)
	registerFailingProvider(g, channelID, quotaErr)

	_, err := g.Fetch(context.Background(), channelID)
	if err == nil {
		t.Fatal("Fetch should still surface the quota error to callers")
	}
	rec := g.Health().Get(channelID)
	if rec == nil {
		t.Fatal("expected health record after fetch")
	}
	if rec.Status != "warn" {
		t.Errorf("Status = %q, want warn (fugle quota exhaustion is a waiting state, not an error)", rec.Status)
	}
	if rec.LastError == "" {
		t.Error("LastError should carry the quota reason for the channel page")
	}
}

// TestGateway_Fetch_RealError_StillRecordsError guards the classification
// boundary: only typed no-data/quota conditions are downgraded; ordinary
// upstream failures must keep recording "error" so ChannelHealthStatusError
// still fires on genuine outages.
func TestGateway_Fetch_RealError_StillRecordsError(t *testing.T) {
	g := newTestGateway(t)
	channelID := "us_yahoo"
	registerFailingProvider(g, channelID, errors.New("i/o timeout"))

	_, err := g.Fetch(context.Background(), channelID)
	if err == nil {
		t.Fatal("Fetch should return the upstream error")
	}
	rec := g.Health().Get(channelID)
	if rec == nil {
		t.Fatal("expected health record after fetch")
	}
	if rec.Status != "error" {
		t.Errorf("Status = %q, want error for a genuine upstream failure", rec.Status)
	}
}
