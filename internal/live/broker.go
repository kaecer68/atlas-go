package live

import (
	"context"
	"fmt"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// BrokerResult 表示券商執行結果。
type BrokerResult struct {
	OrderID   string
	Status    string
	FillPrice float64
	Reason    string
}

// Broker 定義最小下單能力，預設由 dry-run 實作提供安全行為。
type Broker interface {
	SubmitOrder(ctx context.Context, order domain.Order) (BrokerResult, error)
	Mode() string
}

// DryRunBroker 永遠不送真實委託，僅回傳可稽核結果。
type DryRunBroker struct{}

func NewDryRunBroker() *DryRunBroker {
	return &DryRunBroker{}
}

func (b *DryRunBroker) Mode() string {
	return "dry-run"
}

func (b *DryRunBroker) SubmitOrder(_ context.Context, order domain.Order) (BrokerResult, error) {
	if strings.TrimSpace(order.Symbol) == "" {
		return BrokerResult{}, fmt.Errorf("validate order: symbol is required")
	}
	if order.Quantity <= 0 {
		return BrokerResult{}, fmt.Errorf("validate order: quantity must be positive")
	}
	if order.Price <= 0 {
		return BrokerResult{}, fmt.Errorf("validate order: price must be positive")
	}
	if order.Side != domain.SideBuy && order.Side != domain.SideSell {
		return BrokerResult{}, fmt.Errorf("validate order: unsupported side %q", order.Side)
	}

	return BrokerResult{
		OrderID:   fmt.Sprintf("dryrun-%s-%d", order.Symbol, order.Quantity),
		Status:    "filled",
		FillPrice: order.Price,
		Reason:    "dry-run execution",
	}, nil
}
