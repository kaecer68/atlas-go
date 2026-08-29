package live

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
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
		_ = json.NewEncoder(w).Encode(map[string]any{
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
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "placed"})
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
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
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
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1 for non-retryable 400", got)
	}
}

func TestHTTPBrokerAdapterNoRetryOnNotImplementedByDefault(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
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
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1 for default non-retryable 501", got)
	}
}

func TestHTTPBrokerAdapterCustomRetryMatrixRetriesOnConfiguredCode(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			http.Error(w, "not implemented yet", http.StatusNotImplemented)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "placed"})
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
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestHTTPBrokerAdapterRetriesOnTooManyRequests(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "placed"})
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
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestHTTPBrokerAdapterRetriesOnServerError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "placed"})
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
	if got := calls.Load(); got != 2 {
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

func TestHTTPBrokerAdapterRejectsReplayNonceWithinTTL(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "placed"})
	}))
	defer server.Close()

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		Timeout:     2 * time.Second,
		MaxAttempts: 1,
		Client:      server.Client(),
		Now: func() time.Time {
			return time.Date(2026, time.April, 11, 21, 5, 0, 0, time.UTC)
		},
		Nonce:    func() string { return "nonce-replayed-1" },
		NonceTTL: 5 * time.Minute,
	})

	if _, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1}); err != nil {
		t.Fatalf("first SubmitOrder error: %v", err)
	}
	if _, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1}); err == nil {
		t.Fatalf("expected replay nonce error, got nil")
	} else if !strings.Contains(err.Error(), "nonce replay") {
		t.Fatalf("unexpected replay nonce error: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}

func TestHTTPBrokerAdapterAllowsNonceReuseAfterTTL(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "placed"})
	}))
	defer server.Close()

	now := time.Date(2026, time.April, 11, 21, 6, 0, 0, time.UTC)
	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		Timeout:     2 * time.Second,
		MaxAttempts: 1,
		Client:      server.Client(),
		Now: func() time.Time {
			return now
		},
		Nonce:    func() string { return "nonce-reuse-1" },
		NonceTTL: 2 * time.Minute,
	})

	if _, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1}); err != nil {
		t.Fatalf("first SubmitOrder error: %v", err)
	}
	now = now.Add(3 * time.Minute)
	if _, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1}); err != nil {
		t.Fatalf("second SubmitOrder error: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestHTTPBrokerAdapterRejectsTimestampOutsideClockSkew(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "placed"})
	}))
	defer server.Close()

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:      server.URL,
		APIKey:       "k1",
		APISecret:    "s1",
		Timeout:      2 * time.Second,
		MaxAttempts:  1,
		Client:       server.Client(),
		Now:          func() time.Time { return time.Date(2026, time.April, 11, 21, 0, 0, 0, time.UTC) },
		CurrentTime:  func() time.Time { return time.Date(2026, time.April, 11, 21, 10, 0, 0, time.UTC) },
		MaxClockSkew: 30 * time.Second,
	})

	if _, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1}); err == nil {
		t.Fatalf("expected clock skew error, got nil")
	} else if !strings.Contains(err.Error(), "outside allowed skew") {
		t.Fatalf("unexpected clock skew error: %v", err)
	}
}

func TestHTTPBrokerAdapterRejectsHMACSignerWithoutSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "placed"})
	}))
	defer server.Close()

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "",
		Signer:      "hmac-sha256",
		MaxAttempts: 1,
		Client:      server.Client(),
	})

	_, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1})
	if err == nil {
		t.Fatalf("expected signer misconfigured error")
	}
	if !strings.Contains(err.Error(), "api secret") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPBrokerAdapterClassifiesSignatureErrors(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "bad signature", http.StatusUnauthorized)
	}))
	defer server.Close()

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		MaxAttempts: 2,
		Client:      server.Client(),
	})

	_, err := adapter.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "auth.signature_invalid") {
		t.Fatalf("unexpected classification error: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1 for non-retryable auth error", got)
	}
}

func TestHTTPBrokerAdapterKeyRotationUsesNewKeyID(t *testing.T) {
	keyIDs := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keyIDs = append(keyIDs, r.Header.Get("X-Key-Id"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "placed"})
	}))
	defer server.Close()

	adapterV1 := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		KeyID:       "kid-v1",
		MaxAttempts: 1,
		Client:      server.Client(),
	})
	if _, err := adapterV1.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1}); err != nil {
		t.Fatalf("adapter v1 submit failed: %v", err)
	}

	adapterV2 := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		KeyID:       "kid-v2",
		MaxAttempts: 1,
		Client:      server.Client(),
	})
	if _, err := adapterV2.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1}); err != nil {
		t.Fatalf("adapter v2 submit failed: %v", err)
	}

	if len(keyIDs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(keyIDs))
	}
	if keyIDs[0] != "kid-v1" || keyIDs[1] != "kid-v2" {
		t.Fatalf("unexpected key rotation sequence: %v", keyIDs)
	}
}

func TestHTTPBrokerAdapterRejectsReplayNonceAcrossRestartsWithFileStore(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "placed"})
	}))
	defer server.Close()

	storePath := filepath.Join(t.TempDir(), "nonce-replay.json")
	now := time.Date(2026, time.April, 11, 22, 30, 0, 0, time.UTC)

	adapterA := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		Timeout:     2 * time.Second,
		MaxAttempts: 1,
		Client:      server.Client(),
		Now:         func() time.Time { return now },
		Nonce:       func() string { return "nonce-persist-1" },
		NonceTTL:    10 * time.Minute,
		NonceStore:  livestore.NewFileNonceReplayStore(storePath),
	})
	if _, err := adapterA.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1}); err != nil {
		t.Fatalf("adapterA SubmitOrder error: %v", err)
	}

	adapterB := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:     server.URL,
		APIKey:      "k1",
		APISecret:   "s1",
		Timeout:     2 * time.Second,
		MaxAttempts: 1,
		Client:      server.Client(),
		Now:         func() time.Time { return now.Add(1 * time.Minute) },
		Nonce:       func() string { return "nonce-persist-1" },
		NonceTTL:    10 * time.Minute,
		NonceStore:  livestore.NewFileNonceReplayStore(storePath),
	})
	if _, err := adapterB.SubmitOrder(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 1}); err == nil {
		t.Fatalf("expected replay nonce error, got nil")
	} else if !strings.Contains(err.Error(), "nonce replay") {
		t.Fatalf("unexpected replay nonce error: %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}
