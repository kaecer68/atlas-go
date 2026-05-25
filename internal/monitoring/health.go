package monitoring

import (
	"context"

	livestore "github.com/kaecer68/atlas-go/internal/live/store"
)

// GatewayHealthChecker is the minimal interface HealthChecker needs from the Gateway
// to perform channel-level health checks. Defined here to avoid circular imports
// between the monitoring and apigateway packages.
type GatewayHealthChecker interface {
	// Summary returns a concise summary of all channel health statuses,
	// keyed by channel ID, for logging and alert purposes.
	Summary() map[string]string
}

// HealthChecker performs periodic system health checks.
// checkDataProvider() was removed as redundant — Gateway UnifiedHealthStore
// already tracks all 14 channels. Channel-level health is verified by
// background tasks (see cmd/atlas/main.go) that call gateway.Fetch().
// HealthChecker provides a hook (gateway) for future integration.
type HealthChecker struct {
	monitor    *Monitor
	stateStore *livestore.StateStore
	gateway    GatewayHealthChecker
}

// NewHealthChecker creates a health checker.
// Pass nil for stateStore in API mode (check will be skipped gracefully).
func NewHealthChecker(monitor *Monitor, stateStore *livestore.StateStore) *HealthChecker {
	return &HealthChecker{
		monitor:    monitor,
		stateStore: stateStore,
	}
}

// SetGateway wires the API Gateway for channel-level health monitoring.
// Safe to call with nil — channel checks are skipped when gateway is not available.
func (h *HealthChecker) SetGateway(gw GatewayHealthChecker) {
	h.gateway = gw
}

// RunOnce performs a single health check cycle for BTM integration.
func (h *HealthChecker) RunOnce(ctx context.Context) error {
	h.checkStateStore()
	h.checkGateway()
	return nil
}

// checkGateway verifies the API Gateway's channel health records.
// Logs a summary of all channel statuses when the gateway is wired.
func (h *HealthChecker) checkGateway() {
	if h.gateway == nil {
		return
	}
	summary := h.gateway.Summary()
	h.monitor.Info("gateway", "channel_health_summary", map[string]any{
		"channels": summary,
	})
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
