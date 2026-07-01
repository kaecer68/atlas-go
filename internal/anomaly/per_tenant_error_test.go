package anomaly

import (
	"context"
	"testing"
	"time"
)

func TestPerTenantErrorDetector_Name(t *testing.T) {
	d := NewPerTenantErrorDetector(DefaultConfig())
	if got := d.Name(); got != "per_tenant_error" {
		t.Errorf("Name = %q, want per_tenant_error", got)
	}
}

func TestPerTenantErrorDetector_Detect_returnsAnomalyForSpikingTenant(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	d := NewPerTenantErrorDetector(DefaultConfig())
	d.now = func() time.Time { return now }

	entries := []AuditEntryV2{}
	// Baseline: tenant_x has 1 error every 5 minutes.
	for i := 0; i < 288; i++ {
		entries = append(entries, AuditEntryV2{
			TS:       now.Add(-time.Duration(i*5+10) * time.Minute).Format(time.RFC3339),
			TenantID: "tenant_x",
			Tool:     "tool_a",
			Status:   "error",
		})
	}
	// Current window: tenant_x has 10 errors.
	for i := 0; i < 10; i++ {
		entries = append(entries, AuditEntryV2{
			TS:       now.Add(-time.Duration(i) * time.Second).Format(time.RFC3339),
			TenantID: "tenant_x",
			Tool:     "tool_a",
			Status:   "error",
		})
	}

	got, err := d.Detect(context.Background(), entries)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Detect = %d anomalies, want 1", len(got))
	}
	if got[0].Type != "per_tenant_error" {
		t.Errorf("Anomaly.Type = %q, want per_tenant_error", got[0].Type)
	}
	if got[0].TenantID != "tenant_x" {
		t.Errorf("Anomaly.TenantID = %q, want tenant_x", got[0].TenantID)
	}
}

func TestPerTenantErrorDetector_Detect_skipsAnonymousTenant(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	d := NewPerTenantErrorDetector(DefaultConfig())
	d.now = func() time.Time { return now }

	entries := []AuditEntryV2{}
	for i := 0; i < 288; i++ {
		entries = append(entries, AuditEntryV2{
			TS:     now.Add(-time.Duration(i*5+10) * time.Minute).Format(time.RFC3339),
			Tool:   "tool_a",
			Status: "error",
		})
	}
	for i := 0; i < 10; i++ {
		entries = append(entries, AuditEntryV2{
			TS:     now.Add(-time.Duration(i) * time.Second).Format(time.RFC3339),
			Tool:   "tool_a",
			Status: "error",
		})
	}

	got, err := d.Detect(context.Background(), entries)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Detect = %d anomalies, want 0 for anonymous tenants", len(got))
	}
}

func TestPerTenantErrorDetector_Detect_respectsContextCancellation(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	d := NewPerTenantErrorDetector(DefaultConfig())
	d.now = func() time.Time { return now }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := d.Detect(ctx, []AuditEntryV2{})
	if err == nil {
		t.Error("Detect did not return context error")
	}
}
