package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExchangeRateProvider_FetchSnapshot(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"success","base_code":"USD","rates":{"TWD":32.15,"JPY":149.5}}`))
	}))
	defer ts.Close()

	provider := NewExchangeRateProvider()
	provider.client = ts.Client()
	provider.endpoint = ts.URL + "/v6/latest/USD"

	snap, err := provider.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}

	if snap.USD_TWD.Symbol != "USD/TWD=X" {
		t.Errorf("USD_TWD symbol = %q, want USD/TWD=X", snap.USD_TWD.Symbol)
	}
	if snap.USD_TWD.Value != 32.15 {
		t.Errorf("USD_TWD value = %f, want 32.15", snap.USD_TWD.Value)
	}

	if snap.JPY.Symbol != "JPY=X" {
		t.Errorf("JPY symbol = %q, want JPY=X", snap.JPY.Symbol)
	}
	if snap.JPY.Value != 149.5 {
		t.Errorf("JPY value = %f, want 149.5", snap.JPY.Value)
	}

	if snap.RecordedAt == 0 {
		t.Error("RecordedAt should be set")
	}
}

func TestExchangeRateProvider_ZeroRate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"success","base_code":"USD","rates":{"TWD":0,"JPY":0}}`))
	}))
	defer ts.Close()

	provider := NewExchangeRateProvider()
	provider.client = ts.Client()
	provider.endpoint = ts.URL + "/v6/latest/USD"

	snap, err := provider.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}

	if snap.USD_TWD.Symbol != "" {
		t.Error("expected USD_TWD to be empty when rate is zero")
	}
	if snap.JPY.Symbol != "" {
		t.Error("expected JPY to be empty when rate is zero")
	}
}

func TestExchangeRateProvider_MissingRate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"success","base_code":"USD","rates":{"EUR":0.92}}`))
	}))
	defer ts.Close()

	provider := NewExchangeRateProvider()
	provider.client = ts.Client()
	provider.endpoint = ts.URL + "/v6/latest/USD"

	snap, err := provider.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}

	if snap.USD_TWD.Symbol != "" {
		t.Error("expected USD_TWD to be empty when rate is missing")
	}
	if snap.JPY.Symbol != "" {
		t.Error("expected JPY to be empty when rate is missing")
	}
}

func TestExchangeRateProvider_Unavailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	provider := NewExchangeRateProvider()
	provider.client = ts.Client()
	provider.endpoint = ts.URL + "/v6/latest/USD"

	_, err := provider.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when service unavailable")
	}
}

func TestExchangeRateProvider_APIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"error","error-type":"unknown-code"}`))
	}))
	defer ts.Close()

	provider := NewExchangeRateProvider()
	provider.client = ts.Client()
	provider.endpoint = ts.URL + "/v6/latest/USD"

	_, err := provider.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when API returns error result")
	}
}

func TestExchangeRateProvider_ImplementsInterface(t *testing.T) {
	var _ MacroDataProvider = NewExchangeRateProvider()
}
