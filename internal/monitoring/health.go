package monitoring

import (
	"context"

	livestore "github.com/kaecer68/atlas-go/internal/live/store"
)

// HealthChecker performs periodic system health checks.
// checkDataProvider() was removed as redundant — Gateway UnifiedHealthStore
// already tracks all 14 channels. Only checkStateStore() remains.
type HealthChecker struct {
	monitor    *Monitor
	stateStore *livestore.StateStore
}

// NewHealthChecker creates a health checker.
// Pass nil for stateStore in API mode (check will be skipped gracefully).
func NewHealthChecker(monitor *Monitor, stateStore *livestore.StateStore) *HealthChecker {
	return &HealthChecker{
		monitor:    monitor,
		stateStore: stateStore,
	}
}

// RunOnce performs a single health check cycle for BTM integration.
func (h *HealthChecker) RunOnce(ctx context.Context) error {
	h.checkStateStore()
	return nil
}

// checkStateStore verifies the live state store is operational.
// Only meaningful in live mode; gracefully no-ops in API mode.
func (h *HealthChecker) checkStateStore() {
	if h.stateStore == nil {
		return
	}

	portfolio := h.stateStore.GetPortfolio()
	if portfolio.LastUpdated.IsZero() && portfolio.Cash == 0 {
		h.monitor.Error("state_store", "Failed to retrieve valid portfolio from state store", nil)
		return
	}

	h.monitor.Info("state_store", "State store healthy", map[string]any{
		"cash":      portfolio.Cash,
		"positions": len(h.stateStore.GetPositions()),
	})
}
