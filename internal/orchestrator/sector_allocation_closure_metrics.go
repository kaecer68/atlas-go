// Package orchestrator provides sector-allocation-closure dark-launch metrics.
//
// The SA11.B SACMetrics emitter exposes 11 structured events for the
// observation window. Each event carries feature=sector_allocation_closure
// and version=sa.0.1.
package orchestrator

import (
	"log/slog"
)

// SACMetrics exposes the 11 SAC observation-window events.
//
// Wiring status (#1944 Batch 4, item N-A1): StrategyEvolver.ApplySectorRotation
// — the path that actually builds and stores a sector-allocation policy, called
// from orchestrator.SystemCore on the sector-rotation session path — emits
// snapshot.start / snapshot.current / snapshot.target / snapshot.fallback /
// snapshot.end / policy.consumed / policy.applied. The remaining four have no
// honest call site on that path yet and are listed in
// docs/reference/inert-registry.md §Batch 4 (snapshot.projection needs the
// clamped-sector count the projector does not return; legacy.read belongs to the
// legacy compat reader; fallback.count and rollback.drill are other subsystems).
type SACMetrics struct {
	log      *slog.Logger
	observer SACLifecycleObserver
}

// NewSACMetrics creates a metrics emitter. A nil logger falls back to
// slog.Default(), so composition code can wire the emitter without threading a
// logger through.
func NewSACMetrics(log *slog.Logger) *SACMetrics { return &SACMetrics{log: log} }

// SACLifecycleObserver optionally mirrors every emitted event into a test seam.
type SACLifecycleObserver interface {
	ObserveSACEvent(name string, fields map[string]any)
}

// WithObserver attaches an observer that sees every emitted event.
func (m *SACMetrics) WithObserver(o SACLifecycleObserver) *SACMetrics {
	if m != nil {
		m.observer = o
	}
	return m
}

// logger returns the emitter's logger, defaulting to slog.Default so a nil
// logger can never panic the rotation path.
func (m *SACMetrics) logger() *slog.Logger {
	if m == nil || m.log == nil {
		return slog.Default()
	}
	return m.log
}

// emit logs one event and mirrors it to the observer when wired. A nil
// *SACMetrics is a true no-op: StrategyEvolver.WithSACMetrics(nil) keeps the
// rotation path silent.
func (m *SACMetrics) emit(name string, fields ...any) {
	if m == nil {
		return
	}
	m.logger().Info(name, fields...)
	if m.observer == nil {
		return
	}
	kv := make(map[string]any, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		key, _ := fields[i].(string)
		if key == "" {
			continue
		}
		kv[key] = fields[i+1]
	}
	m.observer.ObserveSACEvent(name, kv)
}

// EmitSnapshotStart fires on snapshot computation begin.
func (m *SACMetrics) EmitSnapshotStart(sessionID string) {
	m.emit("sac.snapshot.start", "session_id", sessionID)
}

// EmitSnapshotTarget fires after target weights are computed.
func (m *SACMetrics) EmitSnapshotTarget(sessionID string, sectorCount int, sum float64) {
	m.emit("sac.snapshot.target", "session_id", sessionID, "sector_count", sectorCount, "sum", sum)
}

// EmitSnapshotCurrent fires after current exposure computation.
func (m *SACMetrics) EmitSnapshotCurrent(sessionID string, complete bool, totalValue float64) {
	m.emit("sac.snapshot.current", "session_id", sessionID, "complete", complete, "total_value", totalValue)
}

// EmitSnapshotFallback fires when a fallback reason is recorded.
func (m *SACMetrics) EmitSnapshotFallback(sessionID, reason string) {
	m.emit("sac.snapshot.fallback", "session_id", sessionID, "reason", reason)
}

// EmitSnapshotProjection fires after constraint projection.
func (m *SACMetrics) EmitSnapshotProjection(sessionID string, clamped int) {
	m.emit("sac.snapshot.projection", "session_id", sessionID, "clamped_sectors", clamped)
}

// EmitSnapshotEnd fires on snapshot computation complete.
func (m *SACMetrics) EmitSnapshotEnd(sessionID string, ok bool) {
	m.emit("sac.snapshot.end", "session_id", sessionID, "ok", ok)
}

// EmitPolicyApplied fires when a policy is applied with a receipt.
func (m *SACMetrics) EmitPolicyApplied(sessionID, receiptID string, changedSectors int) {
	m.emit("sac.policy.applied", "session_id", sessionID, "receipt", receiptID, "changed_sectors", changedSectors)
}

// EmitPolicyConsumed fires when a policy is consumed by an allocator.
func (m *SACMetrics) EmitPolicyConsumed(sessionID, policyID string) {
	m.emit("sac.policy.consumed", "session_id", sessionID, "policy_id", policyID)
}

// EmitLegacyRead fires on legacy compat reader access.
func (m *SACMetrics) EmitLegacyRead(key string) {
	m.emit("sac.legacy.read", "key", key)
}

// EmitFallbackCount fires accumulation of fallback counts.
func (m *SACMetrics) EmitFallbackCount(sessionID string, count int) {
	m.emit("sac.fallback.count", "session_id", sessionID, "count", count)
}

// EmitRollbackDrill fires on rollback drill execution.
func (m *SACMetrics) EmitRollbackDrill(sessionID, drillID string) {
	m.emit("sac.rollback.drill", "session_id", sessionID, "drill_id", drillID)
}
