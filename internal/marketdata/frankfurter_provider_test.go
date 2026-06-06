package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFrankfurterFXProvider_FetchSnapshot(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.Path, "/latest") {
			_, _ = w.Write([]byte(`{"date":"2026-05-13","base":"USD","rates":{"JPY":149.5}}`))
		} else {
			_, _ = w.Write([]byte(`{"date":"2026-05-12","base":"USD","rates":{"JPY":148.0}}`))
		}
	}))
	defer ts.Close()

	provider := NewFrankfurterFXProvider()
	provider.client = ts.Client()
	provider.latestURL = ts.URL + "/v2/latest?from=USD&to=JPY"
	provider.baseURL = ts.URL + "/v2"

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
	if snap.JPY.ChangePct == 0 {
		t.Errorf("JPY ChangePct should be non-zero (149.5 vs historical 148.0), got %f", snap.JPY.ChangePct)
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
	provider.latestURL = ts.URL + "/v2/latest"
	provider.baseURL = ts.URL + "/v2"

	_, err := provider.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when service unavailable")
	}
}

func TestFrankfurterFXProvider_ImplementsInterface(t *testing.T) {
	var _ MacroDataProvider = NewFrankfurterFXProvider()
}

func TestFrankfurterFXProvider_ChangePctOnFlatDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.Path, "/latest") {
			_, _ = w.Write([]byte(`{"date":"2026-05-13","base":"USD","rates":{"JPY":150.0}}`))
		} else {
			_, _ = w.Write([]byte(`{"date":"2026-05-12","base":"USD","rates":{"JPY":150.0}}`))
		}
	}))
	defer ts.Close()

	provider := NewFrankfurterFXProvider()
	provider.client = ts.Client()
	provider.latestURL = ts.URL + "/v2/latest?from=USD&to=JPY"
	provider.baseURL = ts.URL + "/v2"

	snap, err := provider.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}
	if snap.JPY.ChangePct != 0 {
		t.Errorf("expected ChangePct=0 when rate unchanged (150 vs 150), got %f", snap.JPY.ChangePct)
	}
}

func TestFrankfurterFXProvider_HistoricalAPIFailsGracefully(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.Path, "/latest") {
			_, _ = w.Write([]byte(`{"date":"2026-05-13","base":"USD","rates":{"JPY":150.0}}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	provider := NewFrankfurterFXProvider()
	provider.client = ts.Client()
	provider.latestURL = ts.URL + "/v2/latest?from=USD&to=JPY"
	provider.baseURL = ts.URL + "/v2"

	snap, err := provider.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot should succeed even when historical API fails: %v", err)
	}
	if snap.JPY.ChangePct != 0 {
		t.Errorf("expected ChangePct=0 when historical API returns 404, got %f", snap.JPY.ChangePct)
	}
}
