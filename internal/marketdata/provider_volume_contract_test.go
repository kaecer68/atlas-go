package marketdata

// provider_volume_contract_test.go — the #1987 single-unit contract, pinned at
// every provider boundary: domain.Quote.Volume is ALWAYS 成交股數 (shares),
// no matter which unit the upstream wire uses.
//
// Providers whose wire reports 成交張數 (lots) — Fugle intraday/quote, the
// Fubon proxy pass-through, and the Fugle WebSocket trades channel — multiply
// by domain.SharesPerLot before constructing the Quote. Providers already
// reporting shares (TWSE, TPEx, FinMind, mock) pass the value through.
//
// Consumers MUST NOT re-convert by Source: the monitoring package's old
// quoteVolumeLotSources translation was removed with this change because it
// double-converted after the boundary fix.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestContractSharesPerLot pins the factor itself: 1 張 = 1,000 股 (Taiwan
// regular-board convention, including ETFs — 0050 et al. trade in 1,000-share
// lots like any regular-board stock).
func TestContractSharesPerLot(t *testing.T) {
	if domain.SharesPerLot != 1000 {
		t.Fatalf("domain.SharesPerLot = %d, want 1000", domain.SharesPerLot)
	}
}

// TestContractFugleQuoteVolumeIsShares: wire tradeVolume=12,345 (lots) →
// Quote.Volume = 12,345,000 (shares).
func TestContractFugleQuoteVolumeIsShares(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"symbol":"2330","closePrice":680,"openPrice":670,"highPrice":685,"lowPrice":668,"lastPrice":680,"total":{"tradeVolume":12345}}`))
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	q, err := c.GetQuote(context.Background(), "2330")
	if err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if q.Volume != 12_345_000 {
		t.Errorf("Quote.Volume = %d, want 12345000 (12,345 lots x 1000)", q.Volume)
	}
	if q.Source != "fugle" {
		t.Errorf("Source = %q, want fugle", q.Source)
	}
}

// TestContractFubonQuoteVolumeIsShares: proxy volume=1,000 (lots) →
// Quote.Volume = 1,000,000 (shares), on BOTH the single and batch paths.
func TestContractFubonQuoteVolumeIsShares(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/quotes" {
			_, _ = w.Write([]byte(`[{"symbol":"2330","last":1000,"open":990,"high":1005,"low":985,"volume":1000,"is_open":true,"is_close":false,"source":"fubon"},` +
				`{"symbol":"2317","last":200,"open":198,"high":201,"low":197,"volume":2000,"is_open":true,"is_close":false,"source":"fubon"}]`))
			return
		}
		_, _ = w.Write([]byte(`{"symbol":"2330","last":1000,"open":990,"high":1005,"low":985,"volume":1000,"is_open":true,"is_close":false,"source":"fubon"}`))
	}))
	defer srv.Close()

	c := NewFubonClient()
	c.proxyURL = srv.URL
	c.intradayLimiter = rate.NewLimiter(rate.Inf, 1)

	q, err := c.GetQuote(context.Background(), "2330")
	if err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if q.Volume != 1_000_000 {
		t.Errorf("single: Quote.Volume = %d, want 1000000 (1,000 lots x 1000)", q.Volume)
	}

	// Two symbols force the batch /quotes path (one symbol routes to GetQuote).
	qs, err := c.GetQuotes(context.Background(), []string{"2330", "2317"})
	if err != nil {
		t.Fatalf("GetQuotes: %v", err)
	}
	if len(qs) != 2 {
		t.Fatalf("GetQuotes returned %d quotes, want 2", len(qs))
	}
	if qs[0].Volume != 1_000_000 {
		t.Errorf("batch[0]: Quote.Volume = %d, want 1000000 (1,000 lots x 1000)", qs[0].Volume)
	}
	if qs[1].Volume != 2_000_000 {
		t.Errorf("batch[1]: Quote.Volume = %d, want 2000000 (2,000 lots x 1000)", qs[1].Volume)
	}
}

// TestContractMockQuoteVolumeIsShares pins the mock provider's share-scale
// fixture so it stays consistent with the contract (its Source is exempt from
// conversion because it already emits shares).
func TestContractMockQuoteVolumeIsShares(t *testing.T) {
	m := NewMockProvider()
	qs, err := m.GetQuotes(context.Background(), time.Now(), []string{"2330"})
	if err != nil {
		t.Fatalf("GetQuotes: %v", err)
	}
	if len(qs) != 1 {
		t.Fatalf("GetQuotes returned %d quotes, want 1", len(qs))
	}
	if qs[0].Volume != 15_000_000 {
		t.Errorf("mock Quote.Volume = %d, want 15000000 (shares)", qs[0].Volume)
	}
}

// TestContractTWSEQuoteVolumeIsShares pins that the first-party TWSE path is
// the reference implementation of the contract: 成交股數 passes through
// unchanged (no conversion at the boundary).
func TestContractTWSEQuoteVolumeIsShares(t *testing.T) {
	q, err := convertToQuoteForTest(TWSEQuote{
		Code:         "2330",
		ClosingPrice: "1566.00",
		OpeningPrice: "1550.00",
		HighestPrice: "1580.00",
		LowestPrice:  "1545.00",
		TradeVolume:  "22,871,974",
	})
	if err != nil {
		t.Fatalf("convertToQuote: %v", err)
	}
	if q.Volume != 22_871_974 {
		t.Errorf("TWSE Quote.Volume = %d, want 22871974 (成交股數, unchanged)", q.Volume)
	}
}
