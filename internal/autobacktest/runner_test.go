package autobacktest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func TestNewRunnerWithEventBus(t *testing.T) {
	cfg := config.Config{LedgerDir: t.TempDir()}
	runner := NewRunnerWithEventBus(cfg, nil)
	if runner == nil {
		t.Fatal("NewRunnerWithEventBus returned nil")
	}
	if runner.cfg.LedgerDir != cfg.LedgerDir {
		t.Error("LedgerDir not set correctly")
	}
}

func TestRunnerRecordSnapshot(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewStore(dir).(*ledger.Store)

	// Seed ledger with data needed by recordSnapshot:
	//   - outcomes (for VaR + scorecard computation)
	//   - session summaries (for portfolio value + drawdown)
	for i := range 25 {
		session := domain.ReplaySession{ID: fmt.Sprintf("sess-snap-%02d", i)}
		summary := domain.SessionSummary{
			SessionID:      session.ID,
			Regime:         domain.RegimeRiskOn,
			PortfolioValue: 100000.0 + float64(i)*100.0,
			EndingCash:     50000.0,
		}
		if err := store.RecordSessionSummary(session, summary); err != nil {
			t.Fatalf("RecordSessionSummary[%d]: %v", i, err)
		}

		agentID := fmt.Sprintf("agent-snap-%02d", i)
		var outs []domain.RecommendationOutcome
		for range 5 {
			outs = append(outs, domain.RecommendationOutcome{
				AgentID:       agentID,
				Symbol:        "2330",
				Side:          domain.SideBuy,
				Layer:         domain.LayerSector,
				Conviction:    1,
				Window:        "1d",
				ForwardReturn: 0.01,
				Hit:           true,
			})
		}
		if err := store.RecordSessionOutcomes(session, outs); err != nil {
			t.Fatalf("RecordSessionOutcomes[%d]: %v", i, err)
		}
	}

	// Record global outcomes so LoadOutcomes works
	var allOutcomes []domain.RecommendationOutcome
	for i := range 25 {
		agentID := fmt.Sprintf("agent-snap-%02d", i)
		for range 5 {
			allOutcomes = append(allOutcomes, domain.RecommendationOutcome{
				AgentID:       agentID,
				Symbol:        "2330",
				Side:          domain.SideBuy,
				Layer:         domain.LayerSector,
				Conviction:    1,
				Window:        "1d",
				ForwardReturn: 0.01,
				Hit:           true,
			})
		}
	}
	if err := store.RecordOutcomes(allOutcomes); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	cfg := config.Config{LedgerDir: dir}
	runner := NewRunner(cfg)

	snapDate := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	if err := runner.recordSnapshot(snapDate); err != nil {
		t.Fatalf("recordSnapshot: %v", err)
	}

	// Verify snapshot was written to JSONL
	snapFile := filepath.Join(dir, "autobacktest", "snapshots.jsonl")
	data, err := os.ReadFile(snapFile)
	if err != nil {
		t.Fatalf("read snapshots file: %v", err)
	}

	var snap AutoSnapshot
	if err := json.Unmarshal(data[:len(data)-1], &snap); err != nil {
		// trim trailing newline from JSONL
		lines := filepath.SplitList(string(data))
		_ = lines
		t.Fatalf("unmarshal snapshot: %v (raw: %q)", err, string(data))
	}

	if !snap.Date.Equal(snapDate) {
		t.Errorf("Date: got %v, want %v", snap.Date, snapDate)
	}
	if snap.PortfolioVal == 0 {
		t.Error("PortfolioVal should be non-zero")
	}
	if snap.SignalCount < 0 {
		t.Errorf("SignalCount should be >= 0, got %d", snap.SignalCount)
	}

	// Verify LatestN can read back the snapshot
	hist := NewHistory(dir)
	snaps, err := hist.LatestN(1)
	if err != nil {
		t.Fatalf("LatestN(1): %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if !snaps[0].Date.Equal(snapDate) {
		t.Errorf("LatestN Date: got %v, want %v", snaps[0].Date, snapDate)
	}
}
