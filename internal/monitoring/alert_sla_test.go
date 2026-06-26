package monitoring

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestAlertStore_Acknowledge_RecordsLatency verifies that the Acknowledge
// method records how long it took to acknowledge the alert (in seconds).
// Per Decision 9 (alert-redesign-v2.md Part 3.7): AcknowledgedWithinSec
// is the key metric for SLA compliance — a CRITICAL alert not acknowledged
// within 30 min becomes a meta-alert.
func TestAlertStore_Acknowledge_RecordsLatency(t *testing.T) {
	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	now := time.Now()
	// Backdate the alert to 5 minutes ago so the latency is non-zero.
	if err := store.Save(domain.AlertRecord{
		ID:        "alert-1",
		Timestamp: now.Add(-5 * time.Minute),
		Rule:      "drawdown",
		Severity:  "critical",
		Status:    domain.AlertStatusTriggered,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Acknowledge now.
	if err := store.Acknowledge("alert-1", "user1"); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}
	all, _ := store.LoadAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(all))
	}
	got := all[0]
	if got.AcknowledgedWithinSec == nil {
		t.Fatal("expected AcknowledgedWithinSec to be set, got nil")
	}
	// 5 minutes = 300 seconds ± a few seconds of test latency.
	if *got.AcknowledgedWithinSec < 290 || *got.AcknowledgedWithinSec > 310 {
		t.Errorf("expected latency ~300s, got %d", *got.AcknowledgedWithinSec)
	}
}

// TestAlertStore_GetSLAStats_PerSeverityBuckets verifies SLA stats aggregate
// by severity bucket (critical/error/warning), counting breaches of the
// per-severity SLA threshold. The endpoint exposes 3 metrics:
//   - alert_ack_latency_p50 / p95
//   - sla_compliance_rate
//
// Per alert-redesign-v2.md Part 3.7.
func TestAlertStore_GetSLAStats_PerSeverityBuckets(t *testing.T) {
	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	now := time.Now()
	// Critical alert acknowledged in 10min (compliant with 30min SLA)
	store.Save(domain.AlertRecord{
		ID:        "a1",
		Timestamp: now.Add(-10 * time.Minute),
		Rule:      "drawdown",
		Severity:  "critical",
		Status:    domain.AlertStatusTriggered,
	})
	store.Acknowledge("a1", "u1")
	// Critical alert acknowledged in 45min (VIOLATES 30min SLA)
	store.Save(domain.AlertRecord{
		ID:        "a2",
		Timestamp: now.Add(-45 * time.Minute),
		Rule:      "drawdown",
		Severity:  "critical",
		Status:    domain.AlertStatusTriggered,
	})
	store.Acknowledge("a2", "u2")
	// Unacknowledged critical alert (counts as violation for compliance rate)
	store.Save(domain.AlertRecord{
		ID:        "a3",
		Timestamp: now.Add(-50 * time.Minute),
		Rule:      "drawdown",
		Severity:  "critical",
		Status:    domain.AlertStatusTriggered,
	})
	// Error alert acknowledged in 30min (compliant with 2h SLA)
	store.Save(domain.AlertRecord{
		ID:        "a4",
		Timestamp: now.Add(-30 * time.Minute),
		Rule:      "channel_health",
		Severity:  "error",
		Status:    domain.AlertStatusTriggered,
	})
	store.Acknowledge("a4", "u4")

	stats := store.GetSLAStats(
		SLAParameters{
			CriticalSec: 30 * 60,      // 30 min
			ErrorSec:    2 * 60 * 60,  // 2 hours
			WarningSec:  24 * 60 * 60, // 24 hours
		},
	)
	// Critical bucket: 3 alerts (a1 compliant 600s, a2 violation 2700s,
	// a3 unacknowledged → treated as violation). Compliance = 1/3 ≈ 33%.
	if stats.Critical.ComplianceRate < 0.3 || stats.Critical.ComplianceRate > 0.4 {
		t.Errorf("expected critical compliance ~0.33, got %f", stats.Critical.ComplianceRate)
	}
	if stats.Critical.Total != 3 {
		t.Errorf("expected critical total=3, got %d", stats.Critical.Total)
	}
	if stats.Critical.Violations != 2 {
		t.Errorf("expected critical violations=2 (a2, a3), got %d", stats.Critical.Violations)
	}
	// Error bucket: 1 alert compliant.
	if stats.Error.ComplianceRate != 1.0 {
		t.Errorf("expected error compliance=1.0, got %f", stats.Error.ComplianceRate)
	}
	// Warning bucket: 0 alerts.
	if stats.Warning.Total != 0 {
		t.Errorf("expected warning total=0, got %d", stats.Warning.Total)
	}
}

// TestAlertStore_GetSLAStats_AggregateLatencyP50P95 verifies the latency
// percentile computation across all acknowledged alerts.
func TestAlertStore_GetSLAStats_AggregateLatencyP50P95(t *testing.T) {
	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	now := time.Now()
	// Create 5 acknowledged alerts with different latencies: 100, 200,
	// 300, 400, 500 seconds. p50 ≈ 300, p95 ≈ 500.
	for i, sec := range []int{100, 200, 300, 400, 500} {
		_ = i
		store.Save(domain.AlertRecord{
			ID:        string(rune('a' + i)),
			Timestamp: now.Add(-time.Duration(sec) * time.Second),
			Rule:      "test",
			Severity:  "warning",
			Status:    domain.AlertStatusTriggered,
		})
		store.Acknowledge(string(rune('a'+i)), "u")
	}
	stats := store.GetSLAStats(SLAParameters{WarningSec: 24 * 3600})
	if stats.AggregateLatencyP50 < 280 || stats.AggregateLatencyP50 > 320 {
		t.Errorf("expected p50 ~300, got %d", stats.AggregateLatencyP50)
	}
	if stats.AggregateLatencyP95 < 480 || stats.AggregateLatencyP95 > 520 {
		t.Errorf("expected p95 ~500, got %d", stats.AggregateLatencyP95)
	}
}

// TestAlertStore_GetSLAStats_EmptyStore verifies zero counts on empty store.
func TestAlertStore_GetSLAStats_EmptyStore(t *testing.T) {
	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	stats := store.GetSLAStats(SLAParameters{CriticalSec: 1800, ErrorSec: 7200, WarningSec: 86400})
	if stats.Critical.Total != 0 || stats.Error.Total != 0 || stats.Warning.Total != 0 {
		t.Errorf("expected all zeros, got %+v", stats)
	}
	if stats.AggregateLatencyP50 != 0 || stats.AggregateLatencyP95 != 0 {
		t.Errorf("expected zero latencies, got p50=%d p95=%d",
			stats.AggregateLatencyP50, stats.AggregateLatencyP95)
	}
}
