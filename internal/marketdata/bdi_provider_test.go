package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestBDIProvider_Name(t *testing.T) {
	p := NewBDIProvider()
	if got := p.Name(); got != "bdi" {
		t.Errorf("Name() = %q, want %q", got, "bdi")
	}
}

func TestBDIProvider_FetchSnapshot_Success(t *testing.T) {
	mockResponse := `{"QuickQuoteResult":{"QuickQuote":[{"symbol":".BADI","last":"1234.00","change_pct":"2.15","last_time_msec":"1730000000000"}]}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	SetBDILimiterForTest(rate.NewLimiter(rate.Inf, 0))
	t.Cleanup(func() { SetBDILimiterForTest(rate.NewLimiter(rate.Every(5*time.Second), 1)) })

	p := NewBDIProvider()
	p.endpoint = server.URL

	ctx := context.Background()
	snap, err := p.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.Bdi.Symbol != ".BADI" {
		t.Errorf("Bdi.Symbol = %q, want %q", snap.Bdi.Symbol, ".BADI")
	}
	if snap.Bdi.Value != 1234.00 {
		t.Errorf("Bdi.Value = %v, want %v", snap.Bdi.Value, 1234.00)
	}
	if snap.Bdi.ChangePct != 2.15 {
		t.Errorf("Bdi.ChangePct = %v, want %v", snap.Bdi.ChangePct, 2.15)
	}
	if snap.Bdi.Timestamp != 1730000000 {
		t.Errorf("Bdi.Timestamp = %v, want %v", snap.Bdi.Timestamp, 1730000000)
	}
}

func TestBDIProvider_FetchSnapshot_NegativeChangePct(t *testing.T) {
	mockResponse := `{"QuickQuoteResult":{"QuickQuote":[{"symbol":".BADI","last":"1100.00","change_pct":"-3.50","last_time_msec":"1730000000000"}]}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()
	SetBDILimiterForTest(rate.NewLimiter(rate.Inf, 0))
	t.Cleanup(func() { SetBDILimiterForTest(rate.NewLimiter(rate.Every(5*time.Second), 1)) })

	p := NewBDIProvider()
	p.endpoint = server.URL

	ctx := context.Background()
	snap, err := p.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.Bdi.Value != 1100.00 {
		t.Errorf("Bdi.Value = %v, want %v", snap.Bdi.Value, 1100.00)
	}
	if snap.Bdi.ChangePct != -3.50 {
		t.Errorf("Bdi.ChangePct = %v, want %v", snap.Bdi.ChangePct, -3.50)
	}
}

func TestBDIProvider_FetchSnapshot_EmptyQuote(t *testing.T) {
	mockResponse := `{"QuickQuoteResult":{"QuickQuote":[]}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	p := NewBDIProvider()
	SetBDILimiterForTest(rate.NewLimiter(rate.Inf, 0))
	t.Cleanup(func() { SetBDILimiterForTest(rate.NewLimiter(rate.Every(5*time.Second), 1)) })

	p.endpoint = server.URL

	ctx := context.Background()
	_, err := p.FetchSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error on empty QuickQuote array")
	}
	if !strings.Contains(err.Error(), "empty QuickQuote") {
		t.Errorf(`expected "empty QuickQuote" error, got: %v`, err)
	}
}

func TestBDIProvider_FetchSnapshot_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	SetBDILimiterForTest(rate.NewLimiter(rate.Inf, 0))
	t.Cleanup(func() { SetBDILimiterForTest(rate.NewLimiter(rate.Every(5*time.Second), 1)) })

	p := NewBDIProvider()
	p.endpoint = server.URL

	ctx := context.Background()
	_, err := p.FetchSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

func TestBDIProvider_FetchSnapshot_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	SetBDILimiterForTest(rate.NewLimiter(rate.Inf, 0))
	t.Cleanup(func() { SetBDILimiterForTest(rate.NewLimiter(rate.Every(5*time.Second), 1)) })

	p := NewBDIProvider()
	p.endpoint = server.URL

	ctx := context.Background()
	_, err := p.FetchSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error on non-200 status")
	}
	if !strings.Contains(err.Error(), "http status 500") {
		t.Errorf("expected status error, got: %v", err)
	}
}

func TestBDIProvider_FetchSnapshot_MissingLastField(t *testing.T) {
	mockResponse := `{"QuickQuoteResult":{"QuickQuote":[{"symbol":".BADI","change_pct":"2.15","last_time_msec":"1730000000000"}]}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	SetBDILimiterForTest(rate.NewLimiter(rate.Inf, 0))
	t.Cleanup(func() { SetBDILimiterForTest(rate.NewLimiter(rate.Every(5*time.Second), 1)) })

	p := NewBDIProvider()
	p.endpoint = server.URL

	ctx := context.Background()
	_, err := p.FetchSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error on missing price field")
	}
	if !strings.Contains(err.Error(), "missing last price") {
		t.Errorf(`expected "missing last price" error, got: %v`, err)
	}
}

func TestBDIProvider_ImplementsInterface(t *testing.T) {
	var _ MacroDataProvider = NewBDIProvider()
}
