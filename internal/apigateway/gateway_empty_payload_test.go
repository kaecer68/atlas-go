package apigateway

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestGateway_Fetch_EmptyPayloadRecordsDegradedForRemovedUpstream pins the
// write-path half of the 2026-09-24 channel-status-truth fix: a fetch that
// SUCCEEDS but carries an empty/stale payload must not be recorded as "ok" for
// a channel whose contract declares DegradedOnEmpty (twse_oddlot: BFI84U was
// repurposed in 2026-08, so no data can ever arrive). "ok" here was what let
// the channel page show 正常 for 17 days while the upstream was gone.
//
// The control case is just as important: channels whose adapters flag stale for
// routine non-trading-day no-data (twse_margin, twse_capital_flow) keep "ok",
// otherwise every weekend would light up as degraded.

func TestGateway_Fetch_EmptyPayloadRecordsDegradedForRemovedUpstream(t *testing.T) {
	cases := []struct {
		channel string
		want    string
	}{
		{"twse_oddlot", StatusDegraded}, // DegradedOnEmpty contract
		{"twse_margin", StatusOK},       // routine no-new-data
	}
	for _, tc := range cases {
		t.Run(tc.channel, func(t *testing.T) {
			g := newTestGateway(t)
			g.registry.Register(tc.channel, &stalePayloadProvider{channel: tc.channel})

			if _, err := g.Fetch(context.Background(), tc.channel); err != nil {
				t.Fatalf("Fetch failed: %v", err)
			}
			rec := g.Health().Get(tc.channel)
			if rec == nil {
				t.Fatal("no health record written for the fetch")
			}
			if rec.Status != tc.want {
				t.Errorf("recorded status = %q, want %q", rec.Status, tc.want)
			}
			if tc.want == StatusDegraded && rec.LastError == "" {
				t.Error("degraded record must carry the reason for the channel page")
			}
		})
	}
}

// stalePayloadProvider mimics an adapter whose upstream returned no usable
// payload: Fetch succeeds with FetchResult.Stale=true (no error, so the
// circuit breaker stays closed).
type stalePayloadProvider struct{ channel string }

func (p *stalePayloadProvider) Fetch(context.Context) (*FetchResult, error) {
	return &FetchResult{Stale: true, Meta: FetchMetadata{ChannelID: p.channel, Timestamp: time.Now(), LastError: "upstream removed"}}, nil
}

func (p *stalePayloadProvider) HealthCheck(context.Context) (HealthStatus, error) {
	return HealthStatus{Status: StatusOK, CheckType: "liveness"}, nil
}

func (p *stalePayloadProvider) RateLimit() *rate.Limiter { return rate.NewLimiter(rate.Inf, 1) }

func (p *stalePayloadProvider) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: p.channel}
}
