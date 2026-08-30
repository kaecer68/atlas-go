package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

// finmindTestClient builds a FinMindClient pointed at an httptest server
// with an infinite rate limiter. URL rewriting follows the
// finmind_client_extra_test.go rewriteTransport pattern (httptest.Server.Client
// does NOT rewrite the host for plain HTTP servers).
func finmindTestClient(srv *httptest.Server) *FinMindClient {
	c := NewFinMindClient("test-key")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: srv.URL, inner: http.DefaultTransport},
	}
	c.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	return c
}

// finmindTXFixture is a live-shaped TaiwanFuturesInstitutionalInvestors
// response for TX on two dates (captured from the FinMind API 2026-08-30;
// numbers match the 2021-06-15 verified probe).
const finmindTXFixture = `{
  "msg": "success",
  "status": 200,
  "data": [
    {"futures_id":"TX","date":"2021-06-15","institutional_investors":"自營商",
     "long_deal_volume":12723,"long_deal_amount":43965958,"short_deal_volume":13576,"short_deal_amount":47020457,
     "long_open_interest_balance_volume":27610,"long_open_interest_balance_amount":94662383,
     "short_open_interest_balance_volume":3330,"short_open_interest_balance_amount":11438517},
    {"futures_id":"TX","date":"2021-06-15","institutional_investors":"投信",
     "long_deal_volume":18124,"long_deal_amount":62664244,"short_deal_volume":17461,"short_deal_amount":60396085,
     "long_open_interest_balance_volume":19164,"long_open_interest_balance_amount":66201347,
     "short_open_interest_balance_volume":17861,"short_open_interest_balance_amount":61699913},
    {"futures_id":"TX","date":"2021-06-15","institutional_investors":"外資",
     "long_deal_volume":77043,"long_deal_amount":266651525,"short_deal_volume":79201,"short_deal_amount":273983321,
     "long_open_interest_balance_volume":21584,"long_open_interest_balance_amount":74661544,
     "short_open_interest_balance_volume":52853,"short_open_interest_balance_amount":181960351},
    {"futures_id":"TX","date":"2021-06-16","institutional_investors":"自營商",
     "long_deal_volume":1,"long_deal_amount":2,"short_deal_volume":3,"short_deal_amount":4,
     "long_open_interest_balance_volume":5,"long_open_interest_balance_amount":6,
     "short_open_interest_balance_volume":7,"short_open_interest_balance_amount":8},
    {"futures_id":"TX","date":"2021-06-16","institutional_investors":"投信",
     "long_deal_volume":9,"long_deal_amount":10,"short_deal_volume":11,"short_deal_amount":12,
     "long_open_interest_balance_volume":13,"long_open_interest_balance_amount":14,
     "short_open_interest_balance_volume":15,"short_open_interest_balance_amount":16},
    {"futures_id":"TX","date":"2021-06-16","institutional_investors":"外資",
     "long_deal_volume":17,"long_deal_amount":18,"short_deal_volume":19,"short_deal_amount":20,
     "long_open_interest_balance_volume":21,"long_open_interest_balance_amount":22,
     "short_open_interest_balance_volume":23,"short_open_interest_balance_amount":24}
  ]
}`

func TestFinMindFuturesInstitutionalProvider_Range(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("dataset"); got != "TaiwanFuturesInstitutionalInvestors" {
			t.Errorf("dataset=%q, want TaiwanFuturesInstitutionalInvestors", got)
		}
		if got := r.URL.Query().Get("data_id"); got != "TX" {
			t.Errorf("data_id=%q, want TX", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(finmindTXFixture))
	}))
	defer srv.Close()

	p := NewFinMindFuturesInstitutionalProvider(finmindTestClient(srv))
	rows, err := p.FetchInstitutionalFuturesRange(context.Background(), "tx", "2021-06-15", "2021-06-16")
	if err != nil {
		t.Fatalf("fetch range: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d days, want 2", len(rows))
	}
	if rows[0].Date != "2021-06-15" {
		t.Errorf("date=%s, want 2021-06-15", rows[0].Date)
	}
	// Verified live values for TX 2021-06-15 (D07 acceptance: OI rows > 0).
	f := rows[0].Foreign
	if f.OILong != 21584 || f.OIShort != 52853 {
		t.Errorf("foreign OI long/short=%d/%d, want 21584/52853", f.OILong, f.OIShort)
	}
	if f.OINet != -31269 {
		t.Errorf("foreign OI net=%d, want -31269", f.OINet)
	}
	if f.TradeNet != -2158 {
		t.Errorf("foreign trade net=%d, want -2158", f.TradeNet)
	}
	if rows[0].InvestmentTrust.OINet != 1303 {
		t.Errorf("trust OI net=%d, want 1303", rows[0].InvestmentTrust.OINet)
	}
	if rows[0].Dealer.OINet != 24280 {
		t.Errorf("dealer OI net=%d, want 24280", rows[0].Dealer.OINet)
	}
}

func TestFinMindFuturesInstitutionalProvider_SingleDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(finmindTXFixture))
	}))
	defer srv.Close()

	p := NewFinMindFuturesInstitutionalProvider(finmindTestClient(srv))
	d, err := p.FetchInstitutionalFuturesForDate(context.Background(), "TX", "2021-06-15")
	if err != nil {
		t.Fatalf("fetch single date: %v", err)
	}
	if d.Date != "2021-06-15" || d.Foreign.OINet != -31269 {
		t.Errorf("unexpected single-date result: date=%s foreign_oi_net=%d", d.Date, d.Foreign.OINet)
	}
}

func TestFinMindFuturesInstitutionalProvider_TXFAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("data_id"); got != "TX" {
			t.Errorf("data_id=%q, want TX (TXF alias should translate)", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(finmindTXFixture))
	}))
	defer srv.Close()

	p := NewFinMindFuturesInstitutionalProvider(finmindTestClient(srv))
	if _, err := p.FetchInstitutionalFuturesForDate(context.Background(), "TXF", "2021-06-15"); err != nil {
		t.Fatalf("TXF alias fetch: %v", err)
	}
}

func TestFinMindFuturesInstitutionalProvider_MissingTrader(t *testing.T) {
	// Only 外資 + 投信 on 2021-06-15 → typed error, no silent partial data.
	body := `{
	  "msg":"success","status":200,"data":[
	    {"futures_id":"TX","date":"2021-06-15","institutional_investors":"外資",
	     "long_deal_volume":1,"short_deal_volume":2,
	     "long_open_interest_balance_volume":3,"short_open_interest_balance_volume":4},
	    {"futures_id":"TX","date":"2021-06-15","institutional_investors":"投信",
	     "long_deal_volume":5,"short_deal_volume":6,
	     "long_open_interest_balance_volume":7,"short_open_interest_balance_volume":8}
	  ]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := NewFinMindFuturesInstitutionalProvider(finmindTestClient(srv))
	_, err := p.FetchInstitutionalFuturesRange(context.Background(), "TX", "2021-06-15", "2021-06-15")
	if err == nil || !strings.Contains(err.Error(), "missing trader rows") {
		t.Fatalf("expected missing-trader error, got %v", err)
	}
}

func TestFinMindFuturesInstitutionalProvider_SchemaGap(t *testing.T) {
	// A renamed volume field must surface ErrTAIFEXSchema instead of 0s.
	body := `{
	  "msg":"success","status":200,"data":[
	    {"futures_id":"TX","date":"2021-06-15","institutional_investors":"外資",
	     "long_deal_volume":"n/a","short_deal_volume":2,
	     "long_open_interest_balance_volume":3,"short_open_interest_balance_volume":4},
	    {"futures_id":"TX","date":"2021-06-15","institutional_investors":"投信",
	     "long_deal_volume":5,"short_deal_volume":6,
	     "long_open_interest_balance_volume":7,"short_open_interest_balance_volume":8},
	    {"futures_id":"TX","date":"2021-06-15","institutional_investors":"自營商",
	     "long_deal_volume":9,"short_deal_volume":10,
	     "long_open_interest_balance_volume":11,"short_open_interest_balance_volume":12}
	  ]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := NewFinMindFuturesInstitutionalProvider(finmindTestClient(srv))
	_, err := p.FetchInstitutionalFuturesRange(context.Background(), "TX", "2021-06-15", "2021-06-15")
	if err == nil || !strings.Contains(err.Error(), "not parseable") {
		t.Fatalf("expected schema-gap error, got %v", err)
	}
}

func TestFinMindFuturesInstitutionalProvider_Empty(t *testing.T) {
	// Empty data (holiday / not-yet-listed product) → empty list, nil error.
	body := `{"msg":"success","status":200,"data":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := NewFinMindFuturesInstitutionalProvider(finmindTestClient(srv))
	rows, err := p.FetchInstitutionalFuturesRange(context.Background(), "EXF", "2021-06-15", "2021-06-15")
	if err != nil {
		t.Fatalf("empty fetch: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d rows, want 0", len(rows))
	}
}
