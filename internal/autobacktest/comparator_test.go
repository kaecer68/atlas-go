package autobacktest

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func TestComparatorComparePortfolioNoData(t *testing.T) {
	dir := t.TempDir()
	cmp := NewComparator(dir)

	comp, err := cmp.ComparePortfolio()
	if err != nil {
		t.Fatalf("ComparePortfolio with no data: expected no error, got %v", err)
	}
	if comp.ShortTermAvg != 0 || comp.LongTermAvg != 0 {
		t.Fatalf("expected zero averages with no data, got short=%f long=%f", comp.ShortTermAvg, comp.LongTermAvg)
	}
}

func TestComparatorComparePortfolio(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewStore(dir).(*ledger.Store)

	for i := 0; i < 20; i++ {
		session := domain.ReplaySession{ID: "pfolio-" + string(rune('0'+i))}
		pv := 100000.0 + float64(i)*1000.0
		summary := domain.SessionSummary{
			SessionID:      session.ID,
			Regime:         domain.RegimeRiskOn,
			PortfolioValue: pv,
			EndingCash:     50000.0,
			RecordedAt:     time.Now().Add(time.Duration(-20+i) * time.Hour),
		}
		if err := store.RecordSessionSummary(session, summary); err != nil {
			t.Fatalf("RecordSessionSummary[%d]: %v", i, err)
		}
	}

	cmp := NewComparator(dir)
	comp, err := cmp.ComparePortfolio()
	if err != nil {
		t.Fatalf("ComparePortfolio: %v", err)
	}

	if comp.ShortTermAvg == 0 {
		t.Errorf("expected non-zero ShortTermAvg, got 0")
	}
	if comp.LongTermAvg == 0 {
		t.Errorf("expected non-zero LongTermAvg, got 0")
	}
	if comp.Delta != comp.ShortTermAvg-comp.LongTermAvg {
		t.Errorf("Delta mismatch: got %f, want %f", comp.Delta, comp.ShortTermAvg-comp.LongTermAvg)
	}
	if comp.LongTermAvg != 0 && comp.DeltaPct == 0 {
		t.Errorf("expected non-zero DeltaPct when LongTermAvg != 0")
	}
}

func TestComparatorCompareSharpeNoData(t *testing.T) {
	dir := t.TempDir()
	cmp := NewComparator(dir)

	comp, err := cmp.CompareSharpe()
	if err != nil {
		t.Fatalf("CompareSharpe with no data: expected no error, got %v", err)
	}
	if comp.ShortTermAvg != 0 || comp.LongTermAvg != 0 {
		t.Fatalf("expected zero averages with no data, got short=%f long=%f", comp.ShortTermAvg, comp.LongTermAvg)
	}
}

func TestComparatorCompareSharpe(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewStore(dir).(*ledger.Store)

	for i := 0; i < 20; i++ {
		session := domain.ReplaySession{ID: "sharpe-" + string(rune('0'+i))}
		var outs []domain.RecommendationOutcome
		for j := 0; j < 5; j++ {
			outs = append(outs, domain.RecommendationOutcome{
				AgentID:       "agent1",
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

	cmp := NewComparator(dir)
	comp, err := cmp.CompareSharpe()
	if err != nil {
		t.Fatalf("CompareSharpe: %v", err)
	}

	if comp.ShortTermAvg == 0 {
		t.Errorf("expected non-zero ShortTermAvg after recording outcomes, got 0")
	}
}

func TestComparatorRecentRegimes(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewStore(dir).(*ledger.Store)

	regimes := []domain.Regime{domain.RegimeRiskOn, domain.RegimeRiskOff, domain.RegimeNeutral, domain.RegimeRiskOn, domain.RegimeRiskOff}
	for i := 0; i < len(regimes); i++ {
		session := domain.ReplaySession{ID: "regime-" + string(rune('0'+i))}
		summary := domain.SessionSummary{
			SessionID:      session.ID,
			Regime:         regimes[i],
			PortfolioValue: 100000.0,
			EndingCash:     50000.0,
			RecordedAt:     time.Now().Add(time.Duration(-5+i) * time.Hour),
		}
		if err := store.RecordSessionSummary(session, summary); err != nil {
			t.Fatalf("RecordSessionSummary[%d]: %v", i, err)
		}
	}

	cmp := NewComparator(dir)
	recent, err := cmp.RecentRegimes()
	if err != nil {
		t.Fatalf("RecentRegimes: %v", err)
	}
	if len(recent) != len(regimes) {
		t.Fatalf("expected %d regimes, got %d", len(regimes), len(recent))
	}
}

func TestComparatorRecentRegimesEmpty(t *testing.T) {
	dir := t.TempDir()
	cmp := NewComparator(dir)

	recent, err := cmp.RecentRegimes()
	if err != nil {
		t.Fatalf("RecentRegimes with no data: expected no error, got %v", err)
	}
	if recent == nil {
		t.Fatalf("expected nil slice for empty regimes, got %v", recent)
	}
}
