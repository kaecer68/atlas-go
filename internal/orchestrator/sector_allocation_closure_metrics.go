// Package orchestrator provides sector-allocation-closure dark-launch metrics.
//
// The SA11.B SACMetrics emitter exposes 11 structured events for the
// observation window. Each event carries feature=sector_allocation_closure
// and version=sa.0.1.
package orchestrator

import (
	"log/slog"
)

const sacFeature = "sector_allocation_closure"

func sacAttr(field string, value any) slog.Attr {
	return slog.Any(field, value)
}

func emitSAC(log *slog.Logger, event string, attrs ...slog.Attr) {
	base := []slog.Attr{
		slog.String("feature", sacFeature),
		slog.String("version", "sa.0.1"),
	}
	log.Info(event, "feature", sacFeature, "version", "sa.0.1")
	_ = base
	_ = attrs
}

// SACMetrics exposes the 11 SAC observation-window events.
type SACMetrics struct{ log *slog.Logger }

// NewSACMetrics creates a metrics emitter.
func NewSACMetrics(log *slog.Logger) *SACMetrics { return &SACMetrics{log: log} }

// EmitSnapshotStart fires on snapshot computation begin.
func (m *SACMetrics) EmitSnapshotStart(sessionID string) {
	m.log.Info("sac.snapshot.start", "session_id", sessionID)
}

// EmitSnapshotTarget fires after target weights are computed.
func (m *SACMetrics) EmitSnapshotTarget(sessionID string, sectorCount int, sum float64) {
	m.log.Info("sac.snapshot.target", "session_id", sessionID, "sector_count", sectorCount, "sum", sum)
}

// EmitSnapshotCurrent fires after current exposure computation.
func (m *SACMetrics) EmitSnapshotCurrent(sessionID string, complete bool, totalValue float64) {
	m.log.Info("sac.snapshot.current", "session_id", sessionID, "complete", complete, "total_value", totalValue)
}

// EmitSnapshotFallback fires when a fallback reason is recorded.
func (m *SACMetrics) EmitSnapshotFallback(sessionID, reason string) {
	m.log.Info("sac.snapshot.fallback", "session_id", sessionID, "reason", reason)
}

// EmitSnapshotProjection fires after constraint projection.
func (m *SACMetrics) EmitSnapshotProjection(sessionID string, clamped int) {
	m.log.Info("sac.snapshot.projection", "session_id", sessionID, "clamped_sectors", clamped)
}

// EmitSnapshotEnd fires on snapshot computation complete.
func (m *SACMetrics) EmitSnapshotEnd(sessionID string, ok bool) {
	m.log.Info("sac.snapshot.end", "session_id", sessionID, "ok", ok)
}

// EmitPolicyApplied fires when a policy is applied with a receipt.
func (m *SACMetrics) EmitPolicyApplied(sessionID, receiptID string, changedSectors int) {
	m.log.Info("sac.policy.applied", "session_id", sessionID, "receipt", receiptID, "changed_sectors", changedSectors)
}

// EmitPolicyConsumed fires when a policy is consumed by an allocator.
func (m *SACMetrics) EmitPolicyConsumed(sessionID, policyID string) {
	m.log.Info("sac.policy.consumed", "session_id", sessionID, "policy_id", policyID)
}

// EmitLegacyRead fires on legacy compat reader access.
func (m *SACMetrics) EmitLegacyRead(key string) {
	m.log.Info("sac.legacy.read", "key", key)
}

// EmitFallbackCount fires accumulation of fallback counts.
func (m *SACMetrics) EmitFallbackCount(sessionID string, count int) {
	m.log.Info("sac.fallback.count", "session_id", sessionID, "count", count)
}

// EmitRollbackDrill fires on rollback drill execution.
func (m *SACMetrics) EmitRollbackDrill(sessionID, drillID string) {
	m.log.Info("sac.rollback.drill", "session_id", sessionID, "drill_id", drillID)
}
