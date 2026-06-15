package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPerformanceService_NewPerformanceService(t *testing.T) {
	svc := NewPerformanceService("/tmp/ledger")
	if svc == nil {
		t.Fatal("NewPerformanceService returned nil")
	}
	if svc.ledgerDir != "/tmp/ledger" {
		t.Errorf("ledgerDir = %q, want /tmp/ledger", svc.ledgerDir)
	}
}

func TestPerformanceService_GetPerformanceReport_EmptyLedger(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(tmpDir)

	report, err := svc.GetPerformanceReport("30d")
	if err != nil {
		t.Fatalf("GetPerformanceReport error = %v", err)
	}
	if report == nil {
		t.Fatal("GetPerformanceReport returned nil report")
	}
}

func TestPerformanceService_GetPerformanceReport_InvalidPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(tmpDir)

	report, err := svc.GetPerformanceReport("invalid")
	if err != nil {
		t.Fatalf("GetPerformanceReport error = %v", err)
	}
	if report == nil {
		t.Fatal("GetPerformanceReport returned nil report")
	}
}

func TestPerformanceService_GetAgentContributions_EmptyLedger(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(tmpDir)

	agents, err := svc.GetAgentContributions("90d")
	if err != nil {
		t.Fatalf("GetAgentContributions error = %v", err)
	}
	if agents == nil {
		t.Fatal("GetAgentContributions returned nil")
	}
}

func TestPerformanceService_GetRegimeBreakdown_EmptyLedger(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(tmpDir)

	breakdown, err := svc.GetRegimeBreakdown("1y")
	if err != nil {
		t.Fatalf("GetRegimeBreakdown error = %v", err)
	}
	if breakdown == nil {
		t.Fatal("GetRegimeBreakdown returned nil")
	}
}

func TestPerformanceService_GetPerformanceReport_AllPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(tmpDir)

	report, err := svc.GetPerformanceReport("all")
	if err != nil {
		t.Fatalf("GetPerformanceReport('all') error = %v", err)
	}
	if report == nil {
		t.Fatal("GetPerformanceReport('all') returned nil report")
	}
}

func TestPerformanceService_GetAgentContributions_AllPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(tmpDir)

	agents, err := svc.GetAgentContributions("all")
	if err != nil {
		t.Fatalf("GetAgentContributions('all') error = %v", err)
	}
	if agents == nil {
		t.Fatal("GetAgentContributions('all') returned nil")
	}
}

func TestPerformanceService_GetRegimeBreakdown_AllPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(tmpDir)

	breakdown, err := svc.GetRegimeBreakdown("all")
	if err != nil {
		t.Fatalf("GetRegimeBreakdown('all') error = %v", err)
	}
	if breakdown == nil {
		t.Fatal("GetRegimeBreakdown('all') returned nil")
	}
}

func TestPerformanceService_GetPerformanceReport_NonExistentDir(t *testing.T) {
	nonExistent := filepath.Join(os.TempDir(), "nonexistent-ledger-dir-xyz123")
	svc := NewPerformanceService(nonExistent)

	report, err := svc.GetPerformanceReport("30d")
	if err != nil {
		t.Fatalf("GetPerformanceReport error = %v", err)
	}
	if report == nil {
		t.Fatal("GetPerformanceReport returned nil")
	}
}

func TestPerformanceService_GetAgentContributions_NonExistentDir(t *testing.T) {
	nonExistent := filepath.Join(os.TempDir(), "nonexistent-ledger-dir-xyz456")
	svc := NewPerformanceService(nonExistent)

	agents, err := svc.GetAgentContributions("30d")
	if err != nil {
		t.Fatalf("GetAgentContributions error = %v", err)
	}
	if agents == nil {
		t.Fatal("GetAgentContributions returned nil")
	}
}

func TestPerformanceService_GetRegimeBreakdown_NonExistentDir(t *testing.T) {
	nonExistent := filepath.Join(os.TempDir(), "nonexistent-ledger-dir-xyz789")
	svc := NewPerformanceService(nonExistent)

	breakdown, err := svc.GetRegimeBreakdown("30d")
	if err != nil {
		t.Fatalf("GetRegimeBreakdown error = %v", err)
	}
	if breakdown == nil {
		t.Fatal("GetRegimeBreakdown returned nil")
	}
}
