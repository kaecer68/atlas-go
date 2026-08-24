package marketdata

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestTAIFEXFetchFutures_BadField_ReturnsErrTAIFEXSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// LastPrice renamed → empty.
		_, _ = w.Write([]byte(`[
			{"Date":"20260811","Contract":"TX","Open":"100","High":"110","Low":"99","LastPrice":"","Volume":"10000","SettlementPrice":"105","PreviousSettlementPrice":"101"}
		]`))
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	_, err := p.FetchFutures(context.Background())
	if err == nil {
		t.Fatal("FetchFutures with empty LastPrice = nil error, want ErrTAIFEXSchema")
	}
	if !errors.Is(err, ErrTAIFEXSchema) {
		t.Errorf("err = %v, want wrapped ErrTAIFEXSchema", err)
	}
}

// TestTAIFEXFetchPCR_PicksLatestDate (P2-16) verifies FetchPCR selects the row
// with the MAXIMUM Date instead of assuming rawList[0] is the newest. The
// upstream row order is not a documented contract — a sorting change would
// previously serve stale PCR data silently.
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
