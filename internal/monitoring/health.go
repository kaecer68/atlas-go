package monitoring

import (
	"context"
	"fmt"
	"time"

	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// HealthStatus 健康状态
type HealthStatus int

const (
	HealthStatusHealthy HealthStatus = iota
	HealthStatusDegraded
	HealthStatusUnhealthy
)

func (s HealthStatus) String() string {
	switch s {
	case HealthStatusHealthy:
		return "HEALTHY"
	case HealthStatusDegraded:
		return "DEGRADED"
	case HealthStatusUnhealthy:
		return "UNHEALTHY"
	default:
		return "UNKNOWN"
	}
}

// HealthChecker 健康检查器
type HealthChecker struct {
	monitor    *Monitor
	provider   marketdata.Provider
	stateStore *livestore.StateStore
	interval   time.Duration
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(monitor *Monitor, provider marketdata.Provider, stateStore *livestore.StateStore) *HealthChecker {
	return &HealthChecker{
		monitor:    monitor,
		provider:   provider,
		stateStore: stateStore,
		interval:   30 * time.Second,
	}
}

// SetInterval 设置检查间隔
func (h *HealthChecker) SetInterval(interval time.Duration) {
	h.interval = interval
}

// Start 启动健康检查
func (h *HealthChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	// 立即执行一次检查
	h.checkAll()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.checkAll()
		}
	}
}

// checkAll 执行所有健康检查
func (h *HealthChecker) checkAll() {
	h.checkDataProvider()
	h.checkStateStore()
}

// checkDataProvider 检查数据提供者健康状态
func (h *HealthChecker) checkDataProvider() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 尝试获取行情（使用常见股票代码）
	_, err := h.provider.GetQuotes(ctx, time.Now(), []string{"0050", "2330"})
	if err != nil {
		h.monitor.Warning("data_provider", fmt.Sprintf("Data provider check failed: %v", err), map[string]any{
			"provider": h.provider.Name(),
			"error":    err.Error(),
		})
	} else {
		h.monitor.Info("data_provider", "Data provider healthy", map[string]any{
			"provider": h.provider.Name(),
		})
	}
}

// checkStateStore 检查状态存储健康状态
func (h *HealthChecker) checkStateStore() {
	if h.stateStore == nil {
		h.monitor.Warning("state_store", "State store not initialized", nil)
		return
	}

	// 检查状态存储是否可写（通过获取投资组合信息）
	portfolio := h.stateStore.GetPortfolio()
	// PortfolioState 是值类型，不能直接比较 nil，通过 LastUpdated 判断是否有效
	if portfolio.LastUpdated.IsZero() && portfolio.Cash == 0 {
		h.monitor.Error("state_store", "Failed to retrieve valid portfolio from state store", nil)
		return
	}

	h.monitor.Info("state_store", "State store healthy", map[string]any{
		"cash":      portfolio.Cash,
		"positions": len(h.stateStore.GetPositions()),
	})
}

// GetStatus 获取整体健康状态
func (h *HealthChecker) GetStatus() HealthStatus {
	// 简化的健康状态判断
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := h.provider.GetQuotes(ctx, time.Now(), []string{"0050"})
	if err != nil {
		return HealthStatusUnhealthy
	}

	return HealthStatusHealthy
}
