package live

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// MockLiveAdapter 是實盤 adapter 契約的測試實作，不會觸發真實下單。
type MockLiveAdapter struct{}

func NewMockLiveAdapter() *MockLiveAdapter {
	return &MockLiveAdapter{}
}

func (a *MockLiveAdapter) SubmitOrder(_ context.Context, order domain.Order) (BrokerResult, error) {
	return BrokerResult{
		OrderID:   fmt.Sprintf("mocklive-%s-%d", order.Symbol, order.Quantity),
		Status:    "filled",
		FillPrice: order.Price,
		Reason:    "mock live adapter execution",
	}, nil
}
