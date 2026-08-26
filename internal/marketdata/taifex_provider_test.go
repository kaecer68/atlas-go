package marketdata

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestTAIFEXFetchPCR_UnitConversion verifies that TAIFEX PutCallRatio API
// percentages (e.g. "110.43" = 110.43%) are converted to ratios (1.1043)
// before being stored in PCRStats. Audit A13 (2026-08-12): the raw API field
// name carries a "%" suffix; without /100 the retail subA5 threshold mapping
// (1.5/1.0/0.8) always matched the top band, pinning weekly_pcr to 0.9.
func TestTAIFEXFetchPCR_UnitConversion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/PutCallRatio" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"Date":"20260811","PutVolume":"100","CallVolume":"200","PutCallVolumeRatio%":"110.43","PutOI":"50","CallOI":"40","PutCallOIRatio%":"123.58"}
		]`))
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	stats, err := p.FetchPCR(context.Background())
	if err != nil {
		t.Fatalf("FetchPCR error: %v", err)
	}
	if got, want := stats.PutCallVolumeRatio, 1.1043; got != want {
		t.Errorf("PutCallVolumeRatio = %v, want %v (110.43%% → 1.1043)", got, want)
	}
	if got, want := stats.PutCallOIRatio, 1.2358; got != want {
		t.Errorf("PutCallOIRatio = %v, want %v (123.58%% → 1.2358)", got, want)
	}
}

// TestTAIFEXFetchPCR_GzipResponse verifies that when the upstream returns a
// gzip-encoded body (as requested by Accept-Encoding: gzip), the provider
// transparently decompresses it before JSON parsing.
func TestTAIFEXFetchPCR_GzipResponse(t *testing.T) {
	payload := `[
		{"Date":"20260811","PutVolume":"100","CallVolume":"200","PutCallVolumeRatio%":"110.43","PutOI":"50","CallOI":"40","PutCallOIRatio%":"123.58"}
	]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept-Encoding"); got != "gzip" {
			t.Errorf("Accept-Encoding = %q, want gzip", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, _ = gz.Write([]byte(payload))
		_ = gz.Close()
		_, _ = w.Write(buf.Bytes())
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	stats, err := p.FetchPCR(context.Background())
	if err != nil {
		t.Fatalf("FetchPCR error: %v", err)
	}
	if got, want := stats.PutCallVolumeRatio, 1.1043; got != want {
		t.Errorf("PutCallVolumeRatio = %v, want %v (gzip decompressed)", got, want)
	}
}

// TestTAIFEXFetchPCR_BOMPrefixedJSON verifies that when the upstream response
// is prefixed with a UTF-8 BOM (0xEF 0xBB 0xBF), the provider strips it
// before JSON parsing. Without stripping, json.Decoder sees the BOM as an
// invalid character ('ï' / 'ï') and fails.
func TestTAIFEXFetchPCR_BOMPrefixedJSON(t *testing.T) {
	payload := []byte(`[
		{"Date":"20260811","PutVolume":"100","CallVolume":"200","PutCallVolumeRatio%":"110.43","PutOI":"50","CallOI":"40","PutCallOIRatio%":"123.58"}
	]`)
	bomPayload := append([]byte{0xEF, 0xBB, 0xBF}, payload...)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bomPayload)
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	stats, err := p.FetchPCR(context.Background())
	if err != nil {
		t.Fatalf("FetchPCR error: %v", err)
	}
	if got, want := stats.PutCallVolumeRatio, 1.1043; got != want {
		t.Errorf("PutCallVolumeRatio = %v, want %v (BOM stripped)", got, want)
	}
}

// TestTAIFEXProvider_ClientTimeout verifies the upstream timeout was raised to
// 30s (openapi.taifex.com.tw can exceed the old 20s budget under load).
func TestTAIFEXProvider_ClientTimeout(t *testing.T) {
	p := NewTAIFEXProvider()
	if p.client == nil {
		t.Fatal("NewTAIFEXProvider client is nil")
	}
	if got, want := p.client.Timeout, 30*time.Second; got != want {
		t.Errorf("client.Timeout = %v, want %v", got, want)
	}
}

// TestTAIFEXFetchPCR_ZeroRatio ensures a "0.00" percentage maps to 0 ratio
// (not NaN or fallback-triggering large value).
func TestTAIFEXFetchPCR_ZeroRatio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"Date":"20260811","PutVolume":"0","CallVolume":"0","PutCallVolumeRatio%":"0.00","PutOI":"0","CallOI":"0","PutCallOIRatio%":"0.00"}
		]`))
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	stats, err := p.FetchPCR(context.Background())
	if err != nil {
		t.Fatalf("FetchPCR error: %v", err)
	}
	if stats.PutCallVolumeRatio != 0 {
		t.Errorf("PutCallVolumeRatio = %v, want 0", stats.PutCallVolumeRatio)
	}
}

// ---------------------------------------------------------------------------
// P0-3: typed ErrTAIFEXSchema — required-field validation. A missing or
// non-numeric field must surface ErrTAIFEXSchema (errors.Is-detectable)
// instead of silently parsing to 0 ("0 data, nil error" hid schema changes).
// ---------------------------------------------------------------------------

func TestTAIFEXFetchPCR_EmptyField_ReturnsErrTAIFEXSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// PutVolume is empty — upstream renamed/removed the column.
		_, _ = w.Write([]byte(`[
			{"Date":"20260811","PutVolume":"","CallVolume":"200","PutCallVolumeRatio%":"110.43","PutOI":"50","CallOI":"40","PutCallOIRatio%":"123.58"}
		]`))
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	_, err := p.FetchPCR(context.Background())
	if err == nil {
		t.Fatal("FetchPCR with empty PutVolume = nil error, want ErrTAIFEXSchema")
	}
	if !errors.Is(err, ErrTAIFEXSchema) {
		t.Errorf("err = %v, want wrapped ErrTAIFEXSchema", err)
	}
}

func TestTAIFEXFetchPCR_MissingRatioField_ReturnsErrTAIFEXSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// PutCallVolumeRatio% field renamed → absent from JSON.
		_, _ = w.Write([]byte(`[
			{"Date":"20260811","PutVolume":"100","CallVolume":"200","PutOI":"50","CallOI":"40","PutCallOIRatio%":"123.58"}
		]`))
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	_, err := p.FetchPCR(context.Background())
	if err == nil {
		t.Fatal("FetchPCR with missing ratio field = nil error, want ErrTAIFEXSchema")
	}
	if !errors.Is(err, ErrTAIFEXSchema) {
		t.Errorf("err = %v, want wrapped ErrTAIFEXSchema", err)
	}
}

func TestTAIFEXFetchRetailFuturesOI_BadField_ReturnsErrTAIFEXSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Top10Buy is a renamed/non-numeric field.
		_, _ = w.Write([]byte(`[
			{"Date":"20260811","Contract":"TX","SettlementMonth":"999912","TypeOfTraders":"0","Top5Buy":"1000","Top5Sell":"900","Top10Buy":"--","Top10Sell":"800","OIOfMarket":"50000"}
		]`))
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	_, err := p.FetchRetailFuturesOI(context.Background())
	if err == nil {
		t.Fatal("FetchRetailFuturesOI with bad Top10Buy = nil error, want ErrTAIFEXSchema")
	}
	if !errors.Is(err, ErrTAIFEXSchema) {
		t.Errorf("err = %v, want wrapped ErrTAIFEXSchema", err)
	}
}

func TestTAIFEXFetchPCR_PicksLatestDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Deliberately out of order: newest row NOT first.
		_, _ = w.Write([]byte(`[
			{"Date":"20260810","PutVolume":"100","CallVolume":"200","PutCallVolumeRatio%":"90.00","PutOI":"50","CallOI":"40","PutCallOIRatio%":"80.00"},
			{"Date":"20260813","PutVolume":"500","CallVolume":"600","PutCallVolumeRatio%":"150.00","PutOI":"250","CallOI":"140","PutCallOIRatio%":"160.00"},
			{"Date":"20260811","PutVolume":"200","CallVolume":"300","PutCallVolumeRatio%":"110.00","PutOI":"80","CallOI":"60","PutCallOIRatio%":"100.00"}
		]`))
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	stats, err := p.FetchPCR(context.Background())
	if err != nil {
		t.Fatalf("FetchPCR error: %v", err)
	}
	if got, want := stats.Date, "20260813"; got != want {
		t.Errorf("Date = %q, want %q (latest by Date, not rawList[0])", got, want)
	}
	if got, want := stats.PutVolume, int64(500); got != want {
		t.Errorf("PutVolume = %d, want %d (must come from the 20260813 row)", got, want)
	}
	if got, want := stats.PutCallVolumeRatio, 1.5; got != want {
		t.Errorf("PutCallVolumeRatio = %v, want %v", got, want)
	}
}

// TestTAIFEXFetchPCR_AllEmptyDates (P2-16) verifies that a payload where every
// row has an empty Date surfaces a typed schema error instead of a nil
// dereference (latest stays nil after the max-Date scan).
func TestTAIFEXFetchPCR_AllEmptyDates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"Date":"","PutVolume":"100","CallVolume":"200","PutCallVolumeRatio%":"90.00","PutOI":"50","CallOI":"40","PutCallOIRatio%":"80.00"}
		]`))
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	_, err := p.FetchPCR(context.Background())
	if err == nil {
		t.Fatal("expected schema error for all-empty Date rows")
	}
	if !errors.Is(err, ErrTAIFEXSchema) {
		t.Fatalf("err = %v, want errors.Is(err, ErrTAIFEXSchema)", err)
	}
}

// TestTAIFEXFetchRetailFuturesOI_CSVFormat verifies the 2026-08-26 upstream
// format change: OpenInterestOfLargeTradersFutures now serves a BOM-prefixed
// CSV (日期,契約,商品名稱(契約名稱),到期月份(週別),交易人類別,前五大/前十大
// 交易人買方/賣方數量,全市場未沖銷部位數) instead of JSON. The provider must
// fall back to CSV parsing so the taifex_daily channel keeps working.
func TestTAIFEXFetchRetailFuturesOI_CSVFormat(t *testing.T) {
	csvBody := "\xef\xbb\xbf" + strings.Join([]string{
		"日期,契約,商品名稱(契約名稱),到期月份(週別),交易人類別,前五大交易人買方數量,前五大交易人賣方數量,前十大交易人買方數量,前十大交易人賣方數量,全市場未沖銷部位數",
		"20260825,TX,臺股期貨,202609,0,30000,28000,45000,42000,120000",
		"20260825,TX,臺股期貨,999912,0,35000,33000,52000,51000,150000",
		"20260825,TX,臺股期貨,999912,1,4000,3000,8000,7000,150000",
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte(csvBody))
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	oi, err := p.FetchRetailFuturesOI(context.Background())
	if err != nil {
		t.Fatalf("FetchRetailFuturesOI() CSV error = %v", err)
	}
	// TX all-months (999912) all traders (type 0): Top5 35000/33000,
	// Top10 52000/51000, market OI 150000.
	if oi.Top5LongOI != 35000 || oi.Top5ShortOI != 33000 {
		t.Errorf("Top5 = %d/%d, want 35000/33000", oi.Top5LongOI, oi.Top5ShortOI)
	}
	if oi.Top10LongOI != 52000 || oi.Top10ShortOI != 51000 {
		t.Errorf("Top10 = %d/%d, want 52000/51000", oi.Top10LongOI, oi.Top10ShortOI)
	}
	if oi.TotalMarketOI != 150000 {
		t.Errorf("TotalMarketOI = %d, want 150000", oi.TotalMarketOI)
	}
	// retail = market - top10
	if oi.RetailLongOI != 150000-52000 || oi.RetailShortOI != 150000-51000 {
		t.Errorf("Retail = %d/%d, want %d/%d", oi.RetailLongOI, oi.RetailShortOI, 150000-52000, 150000-51000)
	}
}
