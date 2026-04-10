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

// LiveExecutionAdapter 定義實盤 adapter 的最小能力。
type LiveExecutionAdapter interface {
	SubmitOrder(ctx context.Context, order domain.Order) (BrokerResult, error)
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
	if err := validateOrder(order); err != nil {
		return BrokerResult{}, err
	}

	return BrokerResult{
		OrderID:   fmt.Sprintf("dryrun-%s-%d", order.Symbol, order.Quantity),
		Status:    "filled",
		FillPrice: order.Price,
		Reason:    "dry-run execution",
	}, nil
}

// GuardedLiveBroker 是 Phase 6 的實盤骨架：未配置 adapter 時永遠拒單。
type GuardedLiveBroker struct {
	adapter LiveExecutionAdapter
}

func NewGuardedLiveBroker(adapter LiveExecutionAdapter) *GuardedLiveBroker {
	return &GuardedLiveBroker{adapter: adapter}
}

func (b *GuardedLiveBroker) Mode() string {
	return "live"
}

func (b *GuardedLiveBroker) SubmitOrder(ctx context.Context, order domain.Order) (BrokerResult, error) {
	if err := validateOrder(order); err != nil {
		return BrokerResult{}, err
	}

	if b.adapter == nil {
		return BrokerResult{
			Status: "rejected",
			Reason: "live broker adapter not configured",
		}, nil
	}

	result, err := b.adapter.SubmitOrder(ctx, order)
	if err != nil {
		return BrokerResult{}, fmt.Errorf("live adapter submit order: %w", err)
	}
	if strings.TrimSpace(result.Status) == "" {
		result.Status = "placed"
	}
	return result, nil
}

func validateOrder(order domain.Order) error {
	if strings.TrimSpace(order.Symbol) == "" {
		return fmt.Errorf("validate order: symbol is required")
	}
	if order.Quantity <= 0 {
		return fmt.Errorf("validate order: quantity must be positive")
	}
	if order.Price <= 0 {
		return fmt.Errorf("validate order: price must be positive")
	}
	if order.Side != domain.SideBuy && order.Side != domain.SideSell {
		return fmt.Errorf("validate order: unsupported side %q", order.Side)
	}
	return nil
}
