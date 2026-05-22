package orders

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/live"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

func TestHandleListOrders_Empty(t *testing.T) {
	h := &Handlers{Svc: nil}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/orders", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleListOrders(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleListOrders_WithService_Empty(t *testing.T) {
	om := live.NewOrderManager(nil, nil, 0, 0, nil)
	svc := service.NewOrderService(om)
	h := &Handlers{Svc: svc}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/orders", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleListOrders(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp service.OrderListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if resp.Orders == nil {
		t.Error("orders is nil")
	}
	if len(resp.Orders) != 0 {
		t.Errorf("len(orders) = %d, want 0", len(resp.Orders))
	}
}

func TestHandleListOrders_WithOrders(t *testing.T) {
	om := live.NewOrderManager(nil, nil, 0, 0, nil)

	om.RecordOrder(live.OrderRecord{
		OrderID:    "order-001",
		Symbol:     "2330",
		Side:       "buy",
		Quantity:   100,
		Price:      900.0,
		Status:     "filled",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		BrokerMode: "dry-run",
		FillPrice:  900.0,
		Events: []live.OrderEvent{
			{Status: "placed", Timestamp: time.Now()},
			{Status: "filled", Timestamp: time.Now()},
		},
	})

	svc := service.NewOrderService(om)
	h := &Handlers{Svc: svc}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/orders", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleListOrders(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp service.OrderListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(resp.Orders) != 1 {
		t.Errorf("len(orders) = %d, want 1", len(resp.Orders))
	}
	if resp.Orders[0].OrderID != "order-001" {
		t.Errorf("order_id = %s, want order-001", resp.Orders[0].OrderID)
	}
}

func TestHandleListOrders_FilterBySymbol(t *testing.T) {
	om := live.NewOrderManager(nil, nil, 0, 0, nil)

	om.RecordOrder(live.OrderRecord{
		OrderID:    "order-001",
		Symbol:     "2330",
		Side:       "buy",
		Quantity:   100,
		Price:      900.0,
		Status:     "filled",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		BrokerMode: "dry-run",
	})
	om.RecordOrder(live.OrderRecord{
		OrderID:    "order-002",
		Symbol:     "2311",
		Side:       "buy",
		Quantity:   200,
		Price:      150.0,
		Status:     "filled",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		BrokerMode: "dry-run",
	})

	svc := service.NewOrderService(om)
	h := &Handlers{Svc: svc}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/orders?symbol=2330", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleListOrders(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp service.OrderListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(resp.Orders) != 1 {
		t.Errorf("len(orders) = %d, want 1", len(resp.Orders))
	}
	if resp.Orders[0].Symbol != "2330" {
		t.Errorf("symbol = %s, want 2330", resp.Orders[0].Symbol)
	}
}

func TestHandleListOrders_Pagination(t *testing.T) {
	om := live.NewOrderManager(nil, nil, 0, 0, nil)

	for i := 1; i <= 25; i++ {
		om.RecordOrder(live.OrderRecord{
			OrderID:    "order-" + string(rune('a'+i-1)),
			Symbol:     "2330",
			Side:       "buy",
			Quantity:   100,
			Price:      900.0,
			Status:     "filled",
			CreatedAt:  time.Now().Add(-time.Duration(i) * time.Hour),
			UpdatedAt:  time.Now(),
			BrokerMode: "dry-run",
		})
	}

	svc := service.NewOrderService(om)
	h := &Handlers{Svc: svc}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/orders?page=2&page_size=10", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleListOrders(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp service.OrderListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(resp.Orders) != 10 {
		t.Errorf("len(orders) = %d, want 10", len(resp.Orders))
	}
	if resp.Page != 2 {
		t.Errorf("page = %d, want 2", resp.Page)
	}
	if resp.PageSize != 10 {
		t.Errorf("page_size = %d, want 10", resp.PageSize)
	}
}

func TestHandleListOrders_PageBeyondTotal(t *testing.T) {
	om := live.NewOrderManager(nil, nil, 0, 0, nil)

	om.RecordOrder(live.OrderRecord{
		OrderID:    "order-001",
		Symbol:     "2330",
		Side:       "buy",
		Quantity:   100,
		Price:      900.0,
		Status:     "filled",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		BrokerMode: "dry-run",
	})

	svc := service.NewOrderService(om)
	h := &Handlers{Svc: svc}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/orders?page=100&page_size=20", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleListOrders(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp service.OrderListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(resp.Orders) != 0 {
		t.Errorf("len(orders) = %d, want 0", len(resp.Orders))
	}
}

func TestHandleGetOrder_NotFound(t *testing.T) {
	om := live.NewOrderManager(nil, nil, 0, 0, nil)
	svc := service.NewOrderService(om)
	h := &Handlers{Svc: svc}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/orders/nonexistent", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleGetOrder(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleGetOrder_Found(t *testing.T) {
	om := live.NewOrderManager(nil, nil, 0, 0, nil)

	om.RecordOrder(live.OrderRecord{
		OrderID:    "order-001",
		Symbol:     "2330",
		Side:       "buy",
		Quantity:   100,
		Price:      900.0,
		Status:     "filled",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		BrokerMode: "dry-run",
		FillPrice:  900.0,
		Events: []live.OrderEvent{
			{Status: "placed", Timestamp: time.Now()},
			{Status: "filled", Timestamp: time.Now()},
		},
	})

	svc := service.NewOrderService(om)
	h := &Handlers{Svc: svc}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/orders/order-001", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleGetOrder(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp service.OrderDetailResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if resp.Order.OrderID != "order-001" {
		t.Errorf("order_id = %s, want order-001", resp.Order.OrderID)
	}
	if resp.Order.Symbol != "2330" {
		t.Errorf("symbol = %s, want 2330", resp.Order.Symbol)
	}
	if len(resp.Order.Events) != 2 {
		t.Errorf("len(events) = %d, want 2", len(resp.Order.Events))
	}
}

func TestHandleGetOrder_MissingID(t *testing.T) {
	om := live.NewOrderManager(nil, nil, 0, 0, nil)
	h := &Handlers{Svc: service.NewOrderService(om)}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/orders/", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleGetOrder(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleListOrders_MethodNotAllowed(t *testing.T) {
	h := &Handlers{Svc: nil}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/orders", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleListOrders(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}
