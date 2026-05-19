package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFrankfurterFXProvider_FetchSnapshot(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"date":"2026-05-13","base":"USD","rates":{"JPY":149.5}}`))
	}))
	defer ts.Close()

	provider := NewFrankfurterFXProvider()
	provider.client = ts.Client()
	provider.endpoint = ts.URL + "/v2/latest?from=USD&to=JPY"

	snap, err := provider.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
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

func TestFrankfurterFXProvider_Unavailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	provider := NewFrankfurterFXProvider()
	provider.client = ts.Client()
	provider.endpoint = ts.URL + "/v2/latest"

	_, err := provider.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when service unavailable")
	}
}

func TestFrankfurterFXProvider_ImplementsInterface(t *testing.T) {
	var _ MacroDataProvider = NewFrankfurterFXProvider()
}
