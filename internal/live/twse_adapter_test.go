package live

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func setupTestTWSEAdapter(t *testing.T, handler http.HandlerFunc) (*TWSEBrokerAdapter, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	cb := NewCircuitBreaker("", "")
	cb.state = CircuitNormal
	adapter := NewTWSEBrokerAdapter(TWSEBrokerAdapterConfig{
		BaseURL:              ts.URL,
		APIKey:               "test-key",
		APISecret:            "test-secret",
		AccountID:            "ACC-001",
		Timeout:              5 * time.Second,
		MaxRequestsPerSecond: 100,
	}, cb)
	return adapter, ts
}

func TestTWSEBrokerAdapterSubmitOrderSuccess(t *testing.T) {
	adapter, _ := setupTestTWSEAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Error("missing or wrong API key header")
		}
		sig := r.Header.Get("X-Signature")
		if sig == "" {
			t.Error("missing signature header")
		}
		sigMethod := r.Header.Get("X-Signature-Method")
		if sigMethod != "hmac-sha256" {
			t.Errorf("expected hmac-sha256 signer, got %s", sigMethod)
		}
		ts := r.Header.Get("X-Request-Timestamp")
		if ts == "" {
			t.Error("missing timestamp header")
		}
		nonce := r.Header.Get("X-Request-Nonce")
		if nonce == "" {
			t.Error("missing nonce header")
		}

		resp := TWSEOrderResponse{
			OrderID:   "TWSE-20260509-0001",
			Status:    "submitted",
			FillPrice: 500.0,
			FilledQty: 0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1000,
		Price:    500.0,
		Reason:   "test",
	}

	result, err := adapter.SubmitOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("SubmitOrder failed: %v", err)
	}
	if result.OrderID != "TWSE-20260509-0001" {
		t.Errorf("unexpected order id: %s", result.OrderID)
	}
	if result.Status != "submitted" {
		t.Errorf("unexpected status: %s", result.Status)
	}
	if result.FillPrice != 500.0 {
		t.Errorf("unexpected fill price: %f", result.FillPrice)
	}
}

func TestTWSEBrokerAdapterSubmitOrderRejected(t *testing.T) {
	adapter, _ := setupTestTWSEAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TWSEOrderResponse{
			Status:       "rejected",
			RejectReason: "insufficient_margin",
		})
	})

	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1000,
		Price:    500.0,
		Reason:   "test",
	}

	_, err := adapter.SubmitOrder(context.Background(), order)
	if err == nil {
		t.Fatal("expected error on rejected order")
	}
	if !strings.Contains(err.Error(), "API error") {
		t.Fatalf("expected API error message, got: %v", err)
	}
}

func TestTWSEBrokerAdapterSubmitOrderTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer ts.Close()

	cb := NewCircuitBreaker("", "")
	cb.state = CircuitNormal
	adapter := NewTWSEBrokerAdapter(TWSEBrokerAdapterConfig{
		BaseURL:              ts.URL,
		APIKey:               "test-key",
		APISecret:            "test-secret",
		AccountID:            "ACC-001",
		Timeout:              100 * time.Millisecond,
		MaxRequestsPerSecond: 100,
	}, cb)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1000,
		Price:    500.0,
		Reason:   "test",
	}

	_, err := adapter.SubmitOrder(ctx, order)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestTWSEBrokerAdapterCircuitBreakerBlocksOrder(t *testing.T) {
	cb := NewCircuitBreaker("", "")
	cb.state = CircuitHalted

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called when circuit breaker is active")
	}))
	defer ts.Close()

	adapter := NewTWSEBrokerAdapter(TWSEBrokerAdapterConfig{
		BaseURL:              ts.URL,
		APIKey:               "test-key",
		APISecret:            "test-secret",
		AccountID:            "ACC-001",
		Timeout:              5 * time.Second,
		MaxRequestsPerSecond: 100,
	}, cb)

	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1000,
		Price:    500.0,
		Reason:   "test",
	}

	result, err := adapter.SubmitOrder(context.Background(), order)
	if err == nil {
		t.Fatal("expected circuit breaker error")
	}
	if result.Status != "rejected" {
		t.Errorf("expected rejected status, got %s", result.Status)
	}
	if !strings.Contains(err.Error(), "circuit breaker") {
		t.Errorf("expected circuit breaker error, got: %v", err)
	}
	if !strings.Contains(result.Reason, "circuit breaker") {
		t.Errorf("expected circuit breaker reason, got: %s", result.Reason)
	}
}

func TestTWSEBrokerAdapterCircuitBreakerAllowsSellWhenPaused(t *testing.T) {
	cb := NewCircuitBreaker("", "")
	cb.state = CircuitPaused

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TWSEOrderResponse{
			OrderID:   "TWSE-20260509-0002",
			Status:    "submitted",
			FillPrice: 480.0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	adapter := NewTWSEBrokerAdapter(TWSEBrokerAdapterConfig{
		BaseURL:              ts.URL,
		APIKey:               "test-key",
		APISecret:            "test-secret",
		AccountID:            "ACC-001",
		Timeout:              5 * time.Second,
		MaxRequestsPerSecond: 100,
	}, cb)

	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideSell,
		Quantity: 500,
		Price:    480.0,
		Reason:   "test",
	}

	result, err := adapter.SubmitOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("expected sell order to pass circuit breaker in paused state: %v", err)
	}
	if result.Status != "submitted" {
		t.Errorf("expected submitted status, got %s", result.Status)
	}
}

func TestTWSEBrokerAdapterSignatureValidation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig := r.Header.Get("X-Signature")
		if sig == "" || strings.HasPrefix(sig, "placeholder-") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(TWSEOrderResponse{
				Status:       "rejected",
				RejectReason: "invalid_signature",
			})
			return
		}
		resp := TWSEOrderResponse{
			OrderID:   "TWSE-20260509-0003",
			Status:    "submitted",
			FillPrice: 600.0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	cb := NewCircuitBreaker("", "")
	cb.state = CircuitNormal
	adapter := NewTWSEBrokerAdapter(TWSEBrokerAdapterConfig{
		BaseURL:              ts.URL,
		APIKey:               "test-key",
		APISecret:            "test-secret",
		AccountID:            "ACC-001",
		Timeout:              5 * time.Second,
		MaxRequestsPerSecond: 100,
	}, cb)

	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1000,
		Price:    600.0,
		Reason:   "test",
	}

	result, err := adapter.SubmitOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("SubmitOrder with valid signature failed: %v", err)
	}
	if result.OrderID != "TWSE-20260509-0003" {
		t.Errorf("unexpected order id: %s", result.OrderID)
	}
}

func TestTWSEBrokerAdapterQueryOrderStatus(t *testing.T) {
	adapter, _ := setupTestTWSEAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/orders/TWSE-001") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TWSETradeResult{
			OrderID:   "TWSE-001",
			Status:    TWSEStatusFilled,
			FillPrice: 510.0,
			FilledQty: 1000,
			RemainQty: 0,
			Symbol:    "2330",
			Side:      "B",
		})
	})

	status, err := adapter.QueryOrderStatus(context.Background(), "TWSE-001")
	if err != nil {
		t.Fatalf("QueryOrderStatus failed: %v", err)
	}
	if status.Status != TWSEStatusFilled {
		t.Errorf("expected filled, got %s", status.Status)
	}
	if status.FillPrice != 510.0 {
		t.Errorf("unexpected fill price: %f", status.FillPrice)
	}
	if status.FilledQty != 1000 {
		t.Errorf("unexpected filled qty: %d", status.FilledQty)
	}
}

func TestTWSEBrokerAdapterCancelOrder(t *testing.T) {
	adapter, _ := setupTestTWSEAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/orders/TWSE-002") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})

	err := adapter.CancelOrder(context.Background(), "TWSE-002")
	if err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}
}

func TestTWSEBrokerAdapterCancelOrderNotFound(t *testing.T) {
	adapter, _ := setupTestTWSEAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error":"order not found"}`)
	})

	err := adapter.CancelOrder(context.Background(), "TWSE-999")
	if err == nil {
		t.Fatal("expected error for not-found cancel")
	}
}

func TestTWSEBrokerAdapterEmptyOrderID(t *testing.T) {
	cb := NewCircuitBreaker("", "")
	cb.state = CircuitNormal
	adapter := NewTWSEBrokerAdapter(TWSEBrokerAdapterConfig{
		BaseURL:              "http://localhost",
		APIKey:               "test-key",
		APISecret:            "test-secret",
		AccountID:            "ACC-001",
		Timeout:              5 * time.Second,
		MaxRequestsPerSecond: 100,
	}, cb)

	_, err := adapter.QueryOrderStatus(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty order ID")
	}

	err = adapter.CancelOrder(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty order ID")
	}
}

func TestTWSEBrokerAdapterNilCircuitBreaker(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TWSEOrderResponse{
			OrderID:   "TWSE-20260509-0004",
			Status:    "submitted",
			FillPrice: 500.0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	adapter := NewTWSEBrokerAdapter(TWSEBrokerAdapterConfig{
		BaseURL:              ts.URL,
		APIKey:               "test-key",
		APISecret:            "test-secret",
		AccountID:            "ACC-001",
		Timeout:              5 * time.Second,
		MaxRequestsPerSecond: 100,
	}, nil)

	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1000,
		Price:    500.0,
		Reason:   "test",
	}

	result, err := adapter.SubmitOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("SubmitOrder with nil circuit breaker failed: %v", err)
	}
	if result.OrderID != "TWSE-20260509-0004" {
		t.Errorf("unexpected order id: %s", result.OrderID)
	}
}

func TestValidateConfig_MissingBaseURL(t *testing.T) {
	adapter := NewTWSEBrokerAdapter(TWSEBrokerAdapterConfig{
		APIKey:    "key",
		APISecret: "secret",
		AccountID: "ACC-001",
	}, nil)
	err := adapter.validateConfig()
	if err == nil {
		t.Fatal("expected error for missing base URL")
	}
	if !strings.Contains(err.Error(), "base_url") {
		t.Errorf("expected base_url error, got: %v", err)
	}
}

func TestValidateConfig_MissingAPIKey(t *testing.T) {
	adapter := NewTWSEBrokerAdapter(TWSEBrokerAdapterConfig{
		BaseURL:   "http://localhost",
		APISecret: "secret",
		AccountID: "ACC-001",
	}, nil)
	err := adapter.validateConfig()
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestValidateConfig_MissingAPISecret(t *testing.T) {
	adapter := NewTWSEBrokerAdapter(TWSEBrokerAdapterConfig{
		BaseURL:   "http://localhost",
		APIKey:    "key",
		AccountID: "ACC-001",
	}, nil)
	err := adapter.validateConfig()
	if err == nil {
		t.Fatal("expected error for missing API secret")
	}
}

func TestValidateConfig_MissingAccountID(t *testing.T) {
	adapter := NewTWSEBrokerAdapter(TWSEBrokerAdapterConfig{
		BaseURL:   "http://localhost",
		APIKey:    "key",
		APISecret: "secret",
	}, nil)
	err := adapter.validateConfig()
	if err == nil {
		t.Fatal("expected error for missing account ID")
	}
}

func TestMapSide_Buy(t *testing.T) {
	if s := mapSide(domain.SideBuy); s != "B" {
		t.Errorf("expected B, got %s", s)
	}
}

func TestMapSide_Sell(t *testing.T) {
	if s := mapSide(domain.SideSell); s != "S" {
		t.Errorf("expected S, got %s", s)
	}
}

func TestMapSide_Unknown(t *testing.T) {
	// Unknown side defaults to "B"
	if s := mapSide("invalid_side"); s != "B" {
		t.Errorf("expected B for unknown side, got %s", s)
	}
}
