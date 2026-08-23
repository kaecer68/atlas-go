package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/sim"
)

func TestRecordSessionSummary_PersistsRiskCommentary(t *testing.T) {
	baseDir := t.TempDir()
	sys := &System{
		SystemCore: &SystemCore{
			sim: SimulationCore{
				cfg:      config.Config{PrimaryMarket: "TW", LedgerDir: baseDir},
				provider: marketdata.NewMockProvider(),
				engine:   sim.NewEngine(domain.SimulationConstraints{StartingCash: 1_000_000}),
				registry: SeedRegistry(),
				session:  domain.ReplaySession{ID: "test-session"},
				ledger:   ledger.NewStore(baseDir),
			},
			plugins: NewPluginRegistry(),
		},
	}

	result := domain.SimulationResult{
		Regime:         domain.RegimeRiskOn,
		EndingCash:     100_000,
		PortfolioValue: 1_000_000,
		RiskCommentary: "test commentary XYZ",
	}

	if err := sys.RecordSessionSummary(result, nil); err != nil {
		t.Fatalf("RecordSessionSummary error: %v", err)
	}

	summaryPath := filepath.Join(baseDir, "sessions", sys.Sim().session.ID, "summary.json")
	bytes, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary.json: %v", err)
	}

	var got domain.SessionSummary
	if err := json.Unmarshal(bytes, &got); err != nil {
		t.Fatalf("unmarshal summary.json: %v", err)
	}

	if got.RiskCommentary != result.RiskCommentary {
		t.Fatalf("risk_commentary mismatch: got %q want %q", got.RiskCommentary, result.RiskCommentary)
	}
}

// TestSaveSessionPositions_EmptyWritesEmptyFile covers BL-02: an empty (no
// positions) session must still write an empty positions.json, so a stale
// positions.json from an earlier session is never mistaken for the current
// session's holdings.
func TestSaveSessionPositions_EmptyWritesEmptyFile(t *testing.T) {
	baseDir := t.TempDir()
	sys := &System{
		SystemCore: &SystemCore{
			sim: SimulationCore{
				cfg:      config.Config{LedgerDir: baseDir},
				provider: marketdata.NewMockProvider(),
				engine:   sim.NewEngine(domain.SimulationConstraints{StartingCash: 1_000_000}),
				registry: SeedRegistry(),
				session:  domain.ReplaySession{ID: "session-test-empty"},
				ledger:   ledger.NewStore(baseDir),
			},
			plugins: NewPluginRegistry(),
		},
	}

	// Empty positions slice → must write empty positions.json.
	sys.saveSessionPositions("session-test-empty", nil)

	path := filepath.Join(baseDir, "sessions", "session-test-empty", "positions.json")
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected empty positions.json to be written, read error: %v", err)
	}
	var got []domain.Position
	if err := json.Unmarshal(bytes, &got); err != nil {
		t.Fatalf("unmarshal empty positions.json: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty positions slice, got %d positions", len(got))
	}
}
