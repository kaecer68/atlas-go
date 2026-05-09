package orders

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	Svc *service.OrderService
}

func (h *Handlers) getService() *service.OrderService {
	return h.Svc
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/orders", shared.Get(h.HandleListOrders))
	mux.Handle("GET /api/dashboard/orders/", shared.Get(h.HandleGetOrder))
}

func (h *Handlers) HandleListOrders(r *http.Request) (int, any) {
	svc := h.getService()
	if svc == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "order service unavailable"}
	}

	params := service.OrderFilterParams{
		Status:   r.URL.Query().Get("status"),
		Symbol:   r.URL.Query().Get("symbol"),
		Side:     r.URL.Query().Get("side"),
		DateFrom: r.URL.Query().Get("date_from"),
		DateTo:   r.URL.Query().Get("date_to"),
	}

	var page, pageSize int
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed := parseInt(p); parsed > 0 {
			page = parsed
		}
	}
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if parsed := parseInt(ps); parsed > 0 {
			pageSize = parsed
		}
	}
	params.Page = page
	params.PageSize = pageSize

	filter := service.ParseOrderFilter(params)
	resp, err := svc.ListOrders(filter)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": "list orders failed"}
	}
	return http.StatusOK, resp
}

func (h *Handlers) HandleGetOrder(r *http.Request) (int, any) {
	svc := h.getService()
	if svc == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "order service unavailable"}
	}

	path := r.URL.Path
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		return http.StatusBadRequest, map[string]string{"error": "missing order id"}
	}
	id := parts[len(parts)-1]
	if id == "" || id == "orders" {
		return http.StatusBadRequest, map[string]string{"error": "missing order id"}
	}

	resp, err := svc.GetOrderDetail(id)
	if err != nil {
		if _, ok := err.(*service.OrderNotFoundError); ok {
			return http.StatusNotFound, map[string]string{"error": "order not found"}
		}
		return http.StatusInternalServerError, map[string]string{"error": "get order failed"}
	}
	return http.StatusOK, resp
}

func parseInt(s string) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return 0
}
