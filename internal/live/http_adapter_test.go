package live

import (
	"context"
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
		Timeout:     2 * time.Second,
		MaxAttempts: 2,
		Client:      server.Client(),
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
