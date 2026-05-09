package service

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/live"
)

type OrderService struct {
	om *live.OrderManager
}

func NewOrderService(om *live.OrderManager) *OrderService {
	return &OrderService{om: om}
}

type OrderListResponse struct {
	Orders   []live.OrderRecord `json:"orders"`
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}

type OrderDetailResponse struct {
	Order live.OrderRecord `json:"order"`
}

func (s *OrderService) ListOrders(filter live.OrderFilter) (*OrderListResponse, error) {
	if s.om == nil {
		return &OrderListResponse{Orders: []live.OrderRecord{}, Total: 0, Page: filter.Page, PageSize: filter.PageSize}, nil
	}

	orders, total, err := s.om.GetOrders(filter)
	if err != nil {
		return nil, err
	}

	return &OrderListResponse{
		Orders:   orders,
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

func (s *OrderService) GetOrderDetail(orderID string) (*OrderDetailResponse, error) {
	if s.om == nil {
		return nil, &OrderNotFoundError{OrderID: orderID}
	}

	order, err := s.om.GetOrder(orderID)
	if err != nil {
		return nil, &OrderNotFoundError{OrderID: orderID}
	}

	return &OrderDetailResponse{Order: *order}, nil
}

type OrderNotFoundError struct {
	OrderID string
}

func (e *OrderNotFoundError) Error() string {
	return "order not found: " + e.OrderID
}

func (e *OrderNotFoundError) NotFound() bool {
	return true
}

type OrderFilterParams struct {
	Status   string
	Symbol   string
	Side     string
	DateFrom string
	DateTo   string
	Page     int
	PageSize int
}

func ParseOrderFilter(params OrderFilterParams) live.OrderFilter {
	filter := live.OrderFilter{
		Page:     1,
		PageSize: 20,
	}

	if params.Page > 0 {
		filter.Page = params.Page
	}
	if params.PageSize > 0 {
		filter.PageSize = params.PageSize
	}
	if params.Status != "" {
		filter.Status = params.Status
	}
	if params.Symbol != "" {
		filter.Symbol = params.Symbol
	}
	if params.Side != "" {
		filter.Side = params.Side
	}
	if params.DateFrom != "" {
		if t, err := time.Parse("2006-01-02", params.DateFrom); err == nil {
			filter.DateFrom = t
		}
	}
	if params.DateTo != "" {
		if t, err := time.Parse("2006-01-02", params.DateTo); err == nil {
			filter.DateTo = t
		}
	}

	return filter
}
