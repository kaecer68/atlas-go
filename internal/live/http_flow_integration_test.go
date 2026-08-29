package live

import (
	"context"
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

func TestGuardedToHTTPFlowIntegration(t *testing.T) {
	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 100}

	guardedOnlyMgr := NewOrderManager(NewGuardedLiveBroker(nil), nil, 0, 0, nil)
	err := guardedOnlyMgr.Run(context.Background(), order)
	if err == nil {
		t.Fatalf("expected guarded broker rejection")
	}
	if !strings.Contains(err.Error(), "adapter") {
		t.Fatalf("unexpected guarded broker error: %v", err)
	}

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Header.Get("X-Signature") == "" {
			t.Fatalf("missing signature header")
		}
		if r.Header.Get("X-Key-Id") != "kid-int-1" {
			t.Fatalf("unexpected key id header: %s", r.Header.Get("X-Key-Id"))
		}
		if r.Header.Get("X-Request-Timestamp") == "" || r.Header.Get("X-Request-Nonce") == "" {
			t.Fatalf("missing anti-replay headers")
		}

		if n == 1 {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "placed", "order_id": "oid-int-1"})
	}))
	defer server.Close()

	storePath := filepath.Join(t.TempDir(), "nonce-store.json")
	nonceStore, err := livestore.BuildNonceReplayStore("file", storePath)
	if err != nil {
		t.Fatalf("build nonce store: %v", err)
	}

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:              server.URL,
		APIKey:               "k1",
		APISecret:            "s1",
		KeyID:                "kid-int-1",
		MaxAttempts:          2,
		RetryableStatusCodes: []int{http.StatusTooManyRequests},
		MaxClockSkew:         30 * time.Second,
		Now:                  func() time.Time { return time.Date(2026, time.April, 11, 23, 30, 0, 0, time.UTC) },
		CurrentTime:          func() time.Time { return time.Date(2026, time.April, 11, 23, 30, 5, 0, time.UTC) },
		Nonce:                func() string { return "nonce-int-1" },
		NonceTTL:             5 * time.Minute,
		NonceStore:           nonceStore,
		Client:               server.Client(),
		Signer:               "hmac-sha256",
	})

	httpMgr := NewOrderManager(NewGuardedLiveBroker(adapter), nil, 0, 0, nil)
	if err := httpMgr.Run(context.Background(), order); err != nil {
		t.Fatalf("http manager run failed: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}

	err = httpMgr.Run(context.Background(), order)
	if err == nil {
		t.Fatalf("expected replay nonce error")
	}
	if !strings.Contains(err.Error(), "nonce replay") {
		t.Fatalf("unexpected nonce replay error: %v", err)
	}
}

func TestHTTPFlowIntegrationRejectsClockSkew(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "placed"})
	}))
	defer server.Close()

	adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
		BaseURL:      server.URL,
		APIKey:       "k1",
		APISecret:    "s1",
		KeyID:        "kid-int-2",
		MaxAttempts:  1,
		MaxClockSkew: 5 * time.Second,
		Now:          func() time.Time { return time.Date(2026, time.April, 11, 23, 40, 0, 0, time.UTC) },
		CurrentTime:  func() time.Time { return time.Date(2026, time.April, 11, 23, 41, 0, 0, time.UTC) },
		Client:       server.Client(),
	})

	mgr := NewOrderManager(NewGuardedLiveBroker(adapter), nil, 0, 0, nil)
	err := mgr.Run(context.Background(), domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 100})
	if err == nil {
		t.Fatalf("expected clock skew error")
	}
	if !strings.Contains(err.Error(), "outside allowed skew") {
		t.Fatalf("unexpected clock skew error: %v", err)
	}
}
