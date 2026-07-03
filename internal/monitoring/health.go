package monitoring

import (
	"context"

	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/logging"
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
// HealthChecker provides hooks (gateway, collector) for future integration.
type HealthChecker struct {
	monitor    *Monitor
	stateStore *livestore.StateStore
	gateway    GatewayHealthChecker
	collector  *MetricsCollector
}

// NewHealthChecker creates a health checker.
// Pass nil for stateStore in API mode (check will be skipped gracefully).
func NewHealthChecker(monitor *Monitor, stateStore *livestore.StateStore) *HealthChecker {
	return &HealthChecker{
		monitor:    monitor,
		stateStore: stateStore,
	}
}

// SetCollector wires the Prometheus MetricsCollector so checkGateway can
// emit per-channel error counters (atlas_channel_health_errors_total).
// Safe to call with nil — metric emission is skipped when collector is not available.
func (h *HealthChecker) SetCollector(c *MetricsCollector) {
	h.collector = c
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
// Logs a structured summary to the logging system — does NOT create alerts
// (channel health is tracked by ChannelHealthStore, not the alert stream).
// For each channel whose status != "ok", emits a counter increment to the
// Prometheus collector so external alerts can fire on sustained errors.
func (h *HealthChecker) checkGateway() {
	if h.gateway == nil {
		return
	}
	summary := h.gateway.Summary()
	logging.Info("health", "channel_health_summary",
		"channels", summary,
	)
	if h.collector == nil {
		return
	}
	for channel, status := range summary {
		if status == "ok" {
			continue
		}
		RecordChannelHealthError(h.collector, channel)
	}
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
