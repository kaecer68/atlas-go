package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ExchangeRateProvider uses open.er-api.com for USD/TWD (Frankfurter doesn't include TWD).
// Reference: USD/TWD on 2026-04-29 ≈ 32.45, USD/JPY ≈ 156.42.

func TestExchangeRateProvider_Name(t *testing.T) {
	if got := NewExchangeRateProvider().Name(); got != "exchange_rate_api" {
		t.Errorf("Name() = %q, want exchange_rate_api", got)
	}
}

func TestExchangeRateProvider_FetchSnapshot_BothRates(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("expected User-Agent header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"result": "success",
			"base_code": "USD",
			"rates": {
				"TWD": 32.45,
				"JPY": 156.42,
				"EUR": 0.92
			}
		}`))
	}))
	defer ts.Close()

	p := NewExchangeRateProvider()
	p.latestURL = ts.URL

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot error: %v", err)
	}
	if snap.USD_TWD.Symbol != "USD/TWD=X" {
		t.Errorf("USD_TWD.Symbol = %q, want USD/TWD=X", snap.USD_TWD.Symbol)
	}
	if snap.USD_TWD.Value != 32.45 {
		t.Errorf("USD_TWD.Value = %v, want 32.45", snap.USD_TWD.Value)
	}
	if snap.USD_TWD.ChangePct != 0 {
		t.Errorf("USD_TWD.ChangePct = %v, want 0 (free tier no historical)", snap.USD_TWD.ChangePct)
	}
	if snap.JPY.Symbol != "JPY=X" {
		t.Errorf("JPY.Symbol = %q, want JPY=X", snap.JPY.Symbol)
	}
	if snap.JPY.Value != 156.42 {
		t.Errorf("JPY.Value = %v, want 156.42", snap.JPY.Value)
	}
	if snap.RecordedAt == 0 {
		t.Error("RecordedAt should be set")
	}
}

func TestExchangeRateProvider_FetchSnapshot_OnlyTWD(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"result": "success",
			"base_code": "USD",
			"rates": {"TWD": 32.45}
		}`))
	}))
	defer ts.Close()

	p := NewExchangeRateProvider()
	p.latestURL = ts.URL

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot error: %v", err)
	}
	if snap.USD_TWD.Value != 32.45 {
		t.Errorf("USD_TWD.Value = %v, want 32.45", snap.USD_TWD.Value)
	}
	if snap.JPY.Symbol != "" {
		t.Errorf("JPY.Symbol = %q, want empty (JPY not in response)", snap.JPY.Symbol)
	}
}

func TestExchangeRateProvider_FetchSnapshot_APIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":"error","error-type":"unsupported-code"}`))
	}))
	defer ts.Close()

	p := NewExchangeRateProvider()
	p.latestURL = ts.URL

	_, err := p.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when API returns non-success result")
	}
	if !strings.Contains(err.Error(), "exchangerate API error") {
		t.Errorf("error %q should mention exchangerate API error", err.Error())
	}
}

func TestExchangeRateProvider_FetchSnapshot_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	p := NewExchangeRateProvider()
	p.latestURL = ts.URL

	_, err := p.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error for 503 response")
	}
}

func TestExchangeRateProvider_FetchSnapshot_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not-json`))
	}))
	defer ts.Close()

	p := NewExchangeRateProvider()
	p.latestURL = ts.URL

	if _, err := p.FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestExchangeRateProvider_FetchSnapshot_ZeroRate(t *testing.T) {
	// Zero rates should be skipped (not populate MacroDataPoint).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"result": "success",
			"base_code": "USD",
			"rates": {"TWD": 0, "JPY": 0}
		}`))
	}))
	defer ts.Close()

	p := NewExchangeRateProvider()
	p.latestURL = ts.URL

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot error: %v", err)
	}
	if snap.USD_TWD.Symbol != "" {
		t.Errorf("USD_TWD.Symbol = %q, want empty (zero rate skipped)", snap.USD_TWD.Symbol)
	}
	if snap.JPY.Symbol != "" {
		t.Errorf("JPY.Symbol = %q, want empty (zero rate skipped)", snap.JPY.Symbol)
	}
}
