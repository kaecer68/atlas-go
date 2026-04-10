package live

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestHTTPBrokerAdapterSubmitOrderSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/orders" {
			t.Fatalf("path = %s, want /orders", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "k1" {
			t.Fatalf("missing api key header")
		}
		sig := r.Header.Get("X-Signature")
		if !strings.HasPrefix(sig, "placeholder-") {
			t.Fatalf("unexpected signature header: %s", sig)
		}
		if r.Header.Get("X-Signature-Method") != "placeholder" {
			t.Fatalf("unexpected signature method: %s", r.Header.Get("X-Signature-Method"))
		}
		if r.Header.Get("X-Signature-Version") != "v1" {
			t.Fatalf("unexpected signature version: %s", r.Header.Get("X-Signature-Version"))
		}
		if r.Header.Get("X-Key-Id") != "kid-1" {
			t.Fatalf("unexpected key id header: %s", r.Header.Get("X-Key-Id"))
		}
		if r.Header.Get("X-Request-Timestamp") != "2026-04-11T21:00:00Z" {
			t.Fatalf("unexpected request timestamp header: %s", r.Header.Get("X-Request-Timestamp"))
		}
		if r.Header.Get("X-Request-Nonce") != "nonce-fixed-1" {
			t.Fatalf("unexpected request nonce header: %s", r.Header.Get("X-Request-Nonce"))
		}
		if got := r.Header.Get("X-Idempotency-Key"); !strings.HasPrefix(got, "atlas-") {
			t.Fatalf("unexpected idempotency key header: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"order_id":   "oid-100",
			"status":     "filled",
			"fill_price": 99.5,
		})
	}))
	defer server.Close()

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		KeyID:       "kid-1",
		Timeout:     2 * time.Second,
		MaxAttempts: 2,
		Client:      server.Client(),
		Now: func() time.Time {
			return time.Date(2026, time.April, 11, 21, 0, 0, 0, time.UTC)
		},
		Nonce: func() string { return "nonce-fixed-1" },
	})

	res, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 10, Price: 99.5})
	if err != nil {
		t.Fatalf("SubmitOrder error: %v", err)
	}
	if res.OrderID != "oid-100" {
		t.Fatalf("order id = %q, want oid-100", res.OrderID)
	}
	if res.Status != "filled" {
		t.Fatalf("status = %q, want filled", res.Status)
	}
}

func TestHTTPBrokerAdapterHMACSignerSetsMethodAndVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Signature-Method") != "hmac-sha256" {
			t.Fatalf("unexpected signature method: %s", r.Header.Get("X-Signature-Method"))
		}
		if r.Header.Get("X-Signature-Version") != "v1" {
			t.Fatalf("unexpected signature version: %s", r.Header.Get("X-Signature-Version"))
		}
		expected := expectedHMACSignature(t, []byte("{\"price\":1,\"quantity\":1,\"reason\":\"\",\"side\":\"BUY\",\"symbol\":\"2330\"}"), "s1", "2026-04-11T21:00:01Z", "nonce-fixed-2")
		if got := r.Header.Get("X-Signature"); got != expected {
			t.Fatalf("unexpected hmac signature value: got=%s want=%s", got, expected)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "placed"})
	}))
	defer server.Close()

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		KeyID:       "kid-hmac-1",
		Timeout:     2 * time.Second,
		MaxAttempts: 1,
		Client:      server.Client(),
		Signer:      "hmac-sha256",
		Now: func() time.Time {
			return time.Date(2026, time.April, 11, 21, 0, 1, 0, time.UTC)
		},
		Nonce: func() string { return "nonce-fixed-2" },
	})

	_, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1})
	if err != nil {
		t.Fatalf("SubmitOrder error: %v", err)
	}
}

func expectedHMACSignature(t *testing.T, payload []byte, secret string, ts string, nonce string) string {
	t.Helper()
	h := hmac.New(sha256.New, []byte(secret))
	_, err := h.Write(canonicalSignInput(payload, secret, ts, nonce))
	if err != nil {
		t.Fatalf("hmac write failed: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestHTTPBrokerAdapterNoRetryOnBadRequest(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "invalid order", http.StatusBadRequest)
	}))
	defer server.Close()

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		Timeout:     2 * time.Second,
		MaxAttempts: 3,
		Client:      server.Client(),
	})

	_, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d, want 1 for non-retryable 400", got)
	}
}

func TestHTTPBrokerAdapterNoRetryOnNotImplementedByDefault(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "not implemented", http.StatusNotImplemented)
	}))
	defer server.Close()

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		Timeout:     2 * time.Second,
		MaxAttempts: 3,
		Client:      server.Client(),
	})

	_, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d, want 1 for default non-retryable 501", got)
	}
}

func TestHTTPBrokerAdapterCustomRetryMatrixRetriesOnConfiguredCode(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			http.Error(w, "not implemented yet", http.StatusNotImplemented)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "placed"})
	}))
	defer server.Close()

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:              server.URL,
		APIKey:               "k1",
		APISecret:            "s1",
		Timeout:              2 * time.Second,
		MaxAttempts:          2,
		RetryableStatusCodes: []int{http.StatusNotImplemented},
		Client:               server.Client(),
	})

	res, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1})
	if err != nil {
		t.Fatalf("SubmitOrder error: %v", err)
	}
	if res.Status != "placed" {
		t.Fatalf("status = %q, want placed", res.Status)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestHTTPBrokerAdapterRetriesOnTooManyRequests(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "placed"})
	}))
	defer server.Close()

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		Timeout:     2 * time.Second,
		MaxAttempts: 2,
		Client:      server.Client(),
	})

	res, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1})
	if err != nil {
		t.Fatalf("SubmitOrder error: %v", err)
	}
	if res.Status != "placed" {
		t.Fatalf("status = %q, want placed", res.Status)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestHTTPBrokerAdapterRetriesOnServerError(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "placed"})
	}))
	defer server.Close()

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		Timeout:     2 * time.Second,
		MaxAttempts: 2,
		Client:      server.Client(),
	})

	res, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1})
	if err != nil {
		t.Fatalf("SubmitOrder error: %v", err)
	}
	if res.Status != "placed" {
		t.Fatalf("status = %q, want placed", res.Status)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestHTTPBrokerAdapterRejectsMissingBaseURL(t *testing.T) {
	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{APIKey: "k1"})
	_, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "base url") {
		t.Fatalf("unexpected error: %v", err)
	}
}
