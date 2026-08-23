package marketdata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

// unlimitedLimiter replaces the provider's real 5s/req limiter in tests.
func unlimitedLimiter() *rate.Limiter {
	return rate.NewLimiter(rate.Inf, 1)
}

func TestFetchInstitutionalFuturesDaily_OK(t *testing.T) {
	body, err := os.ReadFile("testdata/taifex_institutional_20260716.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	p := NewTAIFEXProvider()
	p.baseURL = srv.URL
	p.rateLimiter = unlimitedLimiter()

	daily, err := p.FetchInstitutionalFuturesDaily(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if daily.Date != "20260716" {
		t.Errorf("date=%s, want 20260716", daily.Date)
	}
	// Verified fixture values for 臺股期貨 20260716:
	if daily.Foreign.OINet != -84453 {
		t.Errorf("foreign OI net=%d, want -84453", daily.Foreign.OINet)
	}
	if daily.InvestmentTrust.OINet != 76264 {
		t.Errorf("investment_trust OI net=%d, want 76264", daily.InvestmentTrust.OINet)
	}
	if daily.Dealer.OINet != 657 {
		t.Errorf("dealer OI net=%d, want 657", daily.Dealer.OINet)
	}
	if daily.Foreign.OILong != 5895 || daily.Foreign.OIShort != 90348 {
		t.Errorf("foreign OI long/short=%d/%d, want 5895/90348", daily.Foreign.OILong, daily.Foreign.OIShort)
	}
	if daily.Foreign.TradeNet != -3207 {
		t.Errorf("foreign trade net=%d, want -3207", daily.Foreign.TradeNet)
	}
}

func TestFetchInstitutionalFuturesDaily_MissingTrader(t *testing.T) {
	// Fixture trimmed to only 外資 — should report missing rows.
	onlyForeign := `[
		{"Date":"20260716","ContractCode":"臺股期貨","Item":"外資及陸資",
		 "TradingVolume(Long)":"1","TradingVolume(Short)":"2","TradingVolume(Net)":"-1",
		 "OpenInterest(Long)":"3","OpenInterest(Short)":"4","OpenInterest(Net)":"-1"}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(onlyForeign))
	}))
	defer srv.Close()

	p := NewTAIFEXProvider()
	p.baseURL = srv.URL
	p.rateLimiter = unlimitedLimiter()

	_, err := p.FetchInstitutionalFuturesDaily(context.Background())
	if err == nil || !strings.Contains(err.Error(), "missing trader rows") {
		t.Fatalf("expected missing-trader error, got %v", err)
	}
}

func TestFetchInstitutionalFuturesDaily_NegativeParsing(t *testing.T) {
	// Net fields must parse negative strings correctly (parseInt64 used).
	body := `[
		{"Date":"20260716","ContractCode":"臺股期貨","Item":"外資及陸資",
		 "TradingVolume(Long)":"10","TradingVolume(Short)":"20","TradingVolume(Net)":"-10",
		 "OpenInterest(Long)":"5","OpenInterest(Short)":"90","OpenInterest(Net)":"-85"},
		{"Date":"20260716","ContractCode":"臺股期貨","Item":"投信",
		 "TradingVolume(Long)":"1","TradingVolume(Short)":"0","TradingVolume(Net)":"1",
		 "OpenInterest(Long)":"80","OpenInterest(Short)":"5","OpenInterest(Net)":"75"},
		{"Date":"20260716","ContractCode":"臺股期貨","Item":"自營商",
		 "TradingVolume(Long)":"6","TradingVolume(Short)":"4","TradingVolume(Net)":"2",
		 "OpenInterest(Long)":"5","OpenInterest(Short)":"4","OpenInterest(Net)":"1"}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	p := NewTAIFEXProvider()
	p.baseURL = srv.URL
	p.rateLimiter = unlimitedLimiter()
	d, err := p.FetchInstitutionalFuturesDaily(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if d.Foreign.OINet != -85 {
		t.Errorf("negative OINet parse failed: got %d", d.Foreign.OINet)
	}
	if d.Foreign.TradeNet != -10 {
		t.Errorf("negative trade net parse failed: got %d", d.Foreign.TradeNet)
	}
}

// TestFetchInstitutionalFuturesDaily_SchemaGap_ReturnsErrTAIFEXSchema is the
// P0-3 test: a renamed/non-numeric field on a 臺股期貨 row must surface
// typed ErrTAIFEXSchema instead of silently parsing to 0.
func TestFetchInstitutionalFuturesDaily_SchemaGap_ReturnsErrTAIFEXSchema(t *testing.T) {
	badField := `[
		{"Date":"20260716","ContractCode":"臺股期貨","Item":"外資及陸資",
		 "TradingVolume(Long)":"1","TradingVolume(Short)":"2","TradingVolume(Net)":"-1",
		 "OpenInterest(Long)":"--","OpenInterest(Short)":"4","OpenInterest(Net)":"-1"},
		{"Date":"20260716","ContractCode":"臺股期貨","Item":"投信",
		 "TradingVolume(Long)":"1","TradingVolume(Short)":"2","TradingVolume(Net)":"-1",
		 "OpenInterest(Long)":"3","OpenInterest(Short)":"4","OpenInterest(Net)":"-1"},
		{"Date":"20260716","ContractCode":"臺股期貨","Item":"自營商",
		 "TradingVolume(Long)":"1","TradingVolume(Short)":"2","TradingVolume(Net)":"-1",
		 "OpenInterest(Long)":"3","OpenInterest(Short)":"4","OpenInterest(Net)":"-1"}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(badField))
	}))
	defer srv.Close()

	p := NewTAIFEXProvider()
	p.baseURL = srv.URL
	p.rateLimiter = unlimitedLimiter()

	_, err := p.FetchInstitutionalFuturesDaily(context.Background())
	if err == nil {
		t.Fatal("expected ErrTAIFEXSchema for non-numeric OpenInterest(Long), got nil")
	}
	if !errors.Is(err, ErrTAIFEXSchema) {
		t.Errorf("err = %v, want wrapped ErrTAIFEXSchema", err)
	}
}
