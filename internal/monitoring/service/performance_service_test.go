package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

func TestPerformanceService_NewPerformanceService(t *testing.T) {
	svc := NewPerformanceService(ledger.NewStore("/tmp/ledger"), "/tmp/ledger")
	if svc == nil {
		t.Fatal("NewPerformanceService returned nil")
	}
	if svc.ledgerDir != "/tmp/ledger" {
		t.Errorf("ledgerDir = %q, want /tmp/ledger", svc.ledgerDir)
	}
}

func TestPerformanceService_GetPerformanceReport_EmptyLedger(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)

	report, err := svc.GetPerformanceReport("30d")
	if err != nil {
		t.Fatalf("GetPerformanceReport error = %v", err)
	}
	if report == nil {
		t.Fatal("GetPerformanceReport returned nil report")
	}
	if report.TotalReturn != 0.0 {
		t.Errorf("TotalReturn = %v, want 0.0 for empty ledger", report.TotalReturn)
	}
	if report.TotalTrades != 0 {
		t.Errorf("TotalTrades = %d, want 0 for empty ledger", report.TotalTrades)
	}
}

func TestPerformanceService_GetPerformanceReport_InvalidPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)

	report, err := svc.GetPerformanceReport("invalid")
	if err != nil {
		t.Fatalf("GetPerformanceReport error = %v", err)
	}
	if report == nil {
		t.Fatal("GetPerformanceReport returned nil report")
	}
	if report.TotalReturn != 0.0 {
		t.Errorf("TotalReturn = %v, want 0.0 for empty ledger", report.TotalReturn)
	}
}

func TestPerformanceService_GetAgentContributions_EmptyLedger(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)

	agents, err := svc.GetAgentContributions("90d")
	if err != nil {
		t.Fatalf("GetAgentContributions error = %v", err)
	}
	if agents == nil {
		t.Fatal("GetAgentContributions returned nil")
	}
	if len(agents) != 0 {
		t.Errorf("len(agents) = %d, want 0 for empty ledger", len(agents))
	}
}

func TestPerformanceService_GetRegimeBreakdown_EmptyLedger(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)

	breakdown, err := svc.GetRegimeBreakdown("1y")
	if err != nil {
		t.Fatalf("GetRegimeBreakdown error = %v", err)
	}
	if breakdown == nil {
		t.Fatal("GetRegimeBreakdown returned nil")
	}
	if len(breakdown.Regimes) != 0 {
		t.Errorf("len(Regimes) = %d, want 0 for empty ledger", len(breakdown.Regimes))
	}
}

func TestPerformanceService_GetPerformanceReport_AllPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)

	report, err := svc.GetPerformanceReport("all")
	if err != nil {
		t.Fatalf("GetPerformanceReport('all') error = %v", err)
	}
	if report == nil {
		t.Fatal("GetPerformanceReport('all') returned nil report")
	}
	if report.TotalReturn != 0.0 {
		t.Errorf("TotalReturn = %v, want 0.0 for empty ledger", report.TotalReturn)
	}
}

func TestPerformanceService_GetAgentContributions_AllPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)

	agents, err := svc.GetAgentContributions("all")
	if err != nil {
		t.Fatalf("GetAgentContributions('all') error = %v", err)
	}
	if agents == nil {
		t.Fatal("GetAgentContributions('all') returned nil")
	}
	if len(agents) != 0 {
		t.Errorf("len(agents) = %d, want 0 for empty ledger", len(agents))
	}
}

func TestPerformanceService_GetRegimeBreakdown_AllPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)

	breakdown, err := svc.GetRegimeBreakdown("all")
	if err != nil {
		t.Fatalf("GetRegimeBreakdown('all') error = %v", err)
	}
	if breakdown == nil {
		t.Fatal("GetRegimeBreakdown('all') returned nil")
	}
	if len(breakdown.Regimes) != 0 {
		t.Errorf("len(Regimes) = %d, want 0 for empty ledger", len(breakdown.Regimes))
	}
}

func TestPerformanceService_GetPerformanceReport_NonExistentDir(t *testing.T) {
	nonExistent := filepath.Join(os.TempDir(), "nonexistent-ledger-dir-xyz123")
	svc := NewPerformanceService(ledger.NewStore(nonExistent), nonExistent)

	report, err := svc.GetPerformanceReport("30d")
	if err != nil {
		t.Fatalf("GetPerformanceReport error = %v", err)
	}
	if report == nil {
		t.Fatal("GetPerformanceReport returned nil")
	}
	if report.TotalReturn != 0.0 {
		t.Errorf("TotalReturn = %v, want 0.0 for non-existent dir", report.TotalReturn)
	}
}

func TestPerformanceService_GetAgentContributions_NonExistentDir(t *testing.T) {
	nonExistent := filepath.Join(os.TempDir(), "nonexistent-ledger-dir-xyz456")
	svc := NewPerformanceService(ledger.NewStore(nonExistent), nonExistent)

	agents, err := svc.GetAgentContributions("30d")
	if err != nil {
		t.Fatalf("GetAgentContributions error = %v", err)
	}
	if agents == nil {
		t.Fatal("GetAgentContributions returned nil")
	}
	if len(agents) != 0 {
		t.Errorf("len(agents) = %d, want 0 for non-existent dir", len(agents))
	}
}

func TestPerformanceService_GetRegimeBreakdown_NonExistentDir(t *testing.T) {
	nonExistent := filepath.Join(os.TempDir(), "nonexistent-ledger-dir-xyz789")
	svc := NewPerformanceService(ledger.NewStore(nonExistent), nonExistent)

	breakdown, err := svc.GetRegimeBreakdown("30d")
	if err != nil {
		t.Fatalf("GetRegimeBreakdown error = %v", err)
	}
	if breakdown == nil {
		t.Fatal("GetRegimeBreakdown returned nil")
	}
	if len(breakdown.Regimes) != 0 {
		t.Errorf("len(Regimes) = %d, want 0 for non-existent dir", len(breakdown.Regimes))
	}
}
