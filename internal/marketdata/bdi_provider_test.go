package marketdata

import (
	"context"
	"errors"
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

// TestBDIProvider_FetchSnapshot_MissingLastField covers the CNBC payload seen
// in production since 2026-09-20T08:35Z: HTTP 200, JSON shape intact, but the
// `.BADI` quote carries no `last` and open/high/low are all "0.00". The
// provider must type this as ErrEmptyQuote so the gateway records warn and the
// circuit breaker treats it as a no-op, while the human-readable message stays
// unchanged ("missing last price") for existing diagnostics/log greps.
func TestBDIProvider_FetchSnapshot_MissingLastField(t *testing.T) {
	mockResponse := `{"QuickQuoteResult":{"QuickQuote":[{"symbol":".BADI","open":"0.00","high":"0.00","low":"0.00","change_pct":"","last_time_msec":"","provider":"CNBC Quote Cache"}]}}`

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
	if !errors.Is(err, ErrEmptyQuote) {
		t.Errorf("err = %v, want wrapped ErrEmptyQuote (empty quote must be typed, not a generic failure)", err)
	}
	// An empty quote is NOT a no-data/off-hours condition and NOT a schema or
	// transport failure: confusing it with either would hide the outage or page
	// on it. Pin the separation.
	if errors.Is(err, ErrNoData) {
		t.Error("empty quote must NOT classify as ErrNoData (that would record the channel as ok/waiting)")
	}
	if errors.Is(err, ErrUpstream) || errors.Is(err, ErrSchema) {
		t.Error("empty quote must NOT classify as ErrUpstream/ErrSchema (those trip breakers and page as error)")
	}
}

// TestBDIProvider_FetchSnapshot_HardFailuresAreNotErrEmptyQuote guards the
// boundary of the 2026-09-23 fix: HTTP errors, JSON parse failures and an empty
// QuickQuote array are genuine breakage and must keep the ordinary error
// semantics (no ErrEmptyQuote), so callers still open breakers and page.
func TestBDIProvider_FetchSnapshot_HardFailuresAreNotErrEmptyQuote(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantSub string
	}{
		{
			name:    "http 500",
			status:  http.StatusInternalServerError,
			body:    "boom",
			wantSub: "http status 500",
		},
		{
			name:    "invalid json",
			status:  http.StatusOK,
			body:    `invalid json`,
			wantSub: "unmarshal",
		},
		{
			name:    "empty QuickQuote array",
			status:  http.StatusOK,
			body:    `{"QuickQuoteResult":{"QuickQuote":[]}}`,
			wantSub: "empty QuickQuote",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			SetBDILimiterForTest(rate.NewLimiter(rate.Inf, 0))
			t.Cleanup(func() { SetBDILimiterForTest(rate.NewLimiter(rate.Every(5*time.Second), 1)) })

			p := NewBDIProvider()
			p.endpoint = server.URL

			_, err := p.FetchSnapshot(context.Background())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("err = %v, want message containing %q", err, tt.wantSub)
			}
			if errors.Is(err, ErrEmptyQuote) {
				t.Errorf("err = %v must NOT be ErrEmptyQuote: %s is real breakage and must stay actionable", err, tt.name)
			}
		})
	}
}

func TestBDIProvider_ImplementsInterface(t *testing.T) {
	var _ MacroDataProvider = NewBDIProvider()
}
