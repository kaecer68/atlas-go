package service

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// seedMatrixStore builds an in-memory SQLite outcome store holding session
// outcomes (LoadOutcomesFromSessions reads session rows when no JSONL is
// configured) covering two agents across two periods.
func seedMatrixStore(t *testing.T) ledger.OutcomeStore {
	t.Helper()
	db, err := ledger.OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	store := ledger.NewSQLiteOutcomeStore(db)

	mk := func(agent, period, source string, ret float64, hit bool, recorded time.Time) domain.RecommendationOutcome {
		return domain.RecommendationOutcome{
			AgentID: agent, Symbol: "2330", Side: domain.SideBuy, Conviction: 80,
			Window: "2026-04-01", RecordedAt: recorded, PassedGuards: true,
			ForwardReturn: ret, Hit: hit,
			MarketPeriod: period, MarketPeriodSource: source,
		}
	}
	day := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	var outcomes []domain.RecommendationOutcome
	// 30 wins for agent-a in bull.
	for i := 0; i < 30; i++ {
		outcomes = append(outcomes, mk("agent-a", "bull", "live", 0.02, true, day))
	}
	// 20 outcomes for agent-b in plateau (< 30 → insufficient).
	for i := 0; i < 20; i++ {
		outcomes = append(outcomes, mk("agent-b", "plateau", "live", 0.01, true, day))
	}
	session := domain.ReplaySession{ID: "session-20260401-daily", SessionDate: day}
	if err := store.RecordSessionOutcomes(session, outcomes); err != nil {
		t.Fatalf("record: %v", err)
	}
	return store
}

func TestPeriodMatrixService_Matrix(t *testing.T) {
	svc := NewPeriodMatrixServiceWithStore(seedMatrixStore(t))
	m, err := svc.Matrix()
	if err != nil {
		t.Fatalf("matrix: %v", err)
	}
	if m == nil {
		t.Fatal("nil matrix")
	}
	if m.MinSamples != portfolio.PeriodMatrixMinSamplesDefault {
		t.Errorf("MinSamples = %d, want 30", m.MinSamples)
	}
	if len(m.Periods) != 7 {
		t.Errorf("periods len = %d, want 7", len(m.Periods))
	}
	// agent-a/bull ok with n=30 win_rate 1.0.
	found := false
	for _, c := range m.Cells {
		if c.AgentID == "agent-a" && c.MarketPeriod == "bull" {
			found = true
			if c.Status != portfolio.PeriodCellStatusOK || c.SampleCount != 30 {
				t.Fatalf("agent-a/bull cell = %+v", c)
			}
			if c.WinRate == nil || *c.WinRate != 1.0 {
				t.Errorf("agent-a/bull win_rate = %v", c.WinRate)
			}
		}
		if c.AgentID == "agent-b" && c.MarketPeriod == "plateau" {
			if c.Status != portfolio.PeriodCellStatusInsufficientData || c.SampleCount != 20 {
				t.Errorf("agent-b/plateau cell = %+v", c)
			}
			if c.WinRate != nil {
				t.Error("insufficient cell must not carry win_rate")
			}
		}
	}
	if !found {
		t.Error("agent-a/bull cell missing")
	}
}

func TestPeriodMatrixService_CacheTTL(t *testing.T) {
	store := seedMatrixStore(t)
	svc := NewPeriodMatrixServiceWithStore(store)

	first, err := svc.Matrix()
	if err != nil {
		t.Fatalf("first matrix: %v", err)
	}
	// Within TTL the cached pointer is returned without re-reading the store.
	second, err := svc.Matrix()
	if err != nil {
		t.Fatalf("second matrix: %v", err)
	}
	if first != second {
		t.Error("expected cached matrix pointer within TTL")
	}
	_ = context.Background()
}
