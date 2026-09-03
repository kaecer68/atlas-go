package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// outcomePeriodFixture builds one buy recommendation + a quote for the given
// as-of date (mirrors the fixtures used across system_passed_guards_test.go).
func outcomePeriodFixture(symbol string) ([]domain.Recommendation, []domain.Recommendation, []domain.Quote) {
	recs := []domain.Recommendation{{
		Agent:      "financials-desk-01",
		Skill:      "financials_desk",
		Layer:      domain.LayerStyle,
		Symbol:     symbol,
		Side:       domain.SideBuy,
		Conviction: 60,
		Reason:     "period join test",
	}}
	quotes := []domain.Quote{{Symbol: symbol, Open: 68, Last: 68.5, IsTradable: true}}
	return recs, recs, quotes
}

func mapPeriodResolver(m map[string]periodStub) PeriodResolver {
	return func(date string) (domain.MarketPeriod, string, bool) {
		if s, ok := m[date]; ok {
			return domain.MarketPeriod(s.period), s.source, true
		}
		return "", "", false
	}
}

type periodStub struct {
	period string
	source string
}

// TestBuildSyntheticOutcomesPeriodJoin covers the three resolver states for
// the synthetic outcome path (Phase 2 PR-2a): a live period row fills
// market_period + market_period_source="live"; a synthetic (backfilled)
// period row fills the period with source="synthetic"; an unknown trading
// day leaves both fields empty (the matrix "unknown" cell).
func TestBuildSyntheticOutcomesPeriodJoin(t *testing.T) {
	asOf := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	date := asOf.Format("2006-01-02")

	tests := []struct {
		name       string
		stub       map[string]periodStub
		wantPeriod string
		wantSource string
	}{
		{name: "live row", stub: map[string]periodStub{date: {period: "bull", source: "live"}}, wantPeriod: "bull", wantSource: "live"},
		{name: "synthetic row", stub: map[string]periodStub{date: {period: "black_swan", source: "synthetic"}}, wantPeriod: "black_swan", wantSource: "synthetic"},
		{name: "no period row", stub: nil, wantPeriod: "", wantSource: ""},
		{name: "nil resolver", stub: nil, wantPeriod: "", wantSource: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resolver PeriodResolver
			if tt.stub != nil {
				resolver = mapPeriodResolver(tt.stub)
			}
			raw, final, quotes := outcomePeriodFixture("2881.TW")
			outcomes := buildSyntheticOutcomes(raw, final, quotes, asOf, string(domain.RegimeRiskOn), resolver)
			if len(outcomes) != 1 {
				t.Fatalf("expected 1 outcome, got %d", len(outcomes))
			}
			if outcomes[0].MarketPeriod != tt.wantPeriod {
				t.Errorf("MarketPeriod = %q, want %q", outcomes[0].MarketPeriod, tt.wantPeriod)
			}
			if outcomes[0].MarketPeriodSource != tt.wantSource {
				t.Errorf("MarketPeriodSource = %q, want %q", outcomes[0].MarketPeriodSource, tt.wantSource)
			}
		})
	}
}

// TestBuildReplayOutcomesPeriodJoin covers the replay outcome path: same join
// semantics through buildReplayOutcomes (which needs a Dataset for the
// forward return).
func TestBuildReplayOutcomesPeriodJoin(t *testing.T) {
	asOf := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	date := asOf.Format("2006-01-02")
	next := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	ds := &replay.Dataset{
		ByDate: map[string]map[string]domain.DailyBar{
			date:                      {"2881.TW": {Date: asOf, Symbol: "2881.TW", Close: 68}},
			next.Format("2006-01-02"): {"2881.TW": {Date: next, Symbol: "2881.TW", Close: 69.36}},
		},
		Dates: []time.Time{asOf, next},
	}

	raw, final, quotes := outcomePeriodFixture("2881.TW")
	outcomes := buildReplayOutcomes(raw, final, quotes, asOf, string(domain.RegimeRiskOn), ds,
		mapPeriodResolver(map[string]periodStub{date: {period: "turnaround_up", source: "live"}}))
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].MarketPeriod != "turnaround_up" {
		t.Errorf("MarketPeriod = %q, want turnaround_up", outcomes[0].MarketPeriod)
	}
	if outcomes[0].MarketPeriodSource != "live" {
		t.Errorf("MarketPeriodSource = %q, want live", outcomes[0].MarketPeriodSource)
	}
}

// stubHistoricalStore implements ledger.HistoricalStore with only
// LoadPeriodByDateAll wired; every other method is promoted from the embedded
// nil interface and must never be called by the resolver.
type stubHistoricalStore struct {
	ledger.HistoricalStore
	rows map[string]ledger.PeriodRow
}

func (s *stubHistoricalStore) LoadPeriodByDateAll(_ context.Context, date string) (ledger.PeriodRow, bool, error) {
	row, ok := s.rows[date]
	return row, ok, nil
}

// TestHistoricalPeriodResolver checks the store adapter: is_synthetic=0 →
// "live", is_synthetic=1 → "synthetic", missing date → not ok.
func TestHistoricalPeriodResolver(t *testing.T) {
	store := &stubHistoricalStore{rows: map[string]ledger.PeriodRow{
		"2026-04-01": {Date: "2026-04-01", Period: "plateau", IsSynthetic: 0},
		"2020-06-15": {Date: "2020-06-15", Period: "black_swan", IsSynthetic: 1},
	}}
	resolver := HistoricalPeriodResolver(store)
	if resolver == nil {
		t.Fatal("HistoricalPeriodResolver(nil-capable store) returned nil")
	}

	period, source, ok := resolver("2026-04-01")
	if !ok || string(period) != "plateau" || source != "live" {
		t.Errorf("live row: got (%s, %s, %v), want (plateau, live, true)", period, source, ok)
	}
	period, source, ok = resolver("2020-06-15")
	if !ok || string(period) != "black_swan" || source != "synthetic" {
		t.Errorf("synthetic row: got (%s, %s, %v), want (black_swan, synthetic, true)", period, source, ok)
	}
	if _, _, ok := resolver("2019-01-01"); ok {
		t.Error("missing date should report not ok")
	}
	if resolver := HistoricalPeriodResolver(nil); resolver != nil {
		t.Error("HistoricalPeriodResolver(nil) should return nil")
	}
}

// TestResolveOutcomePeriodNilResolver locks the legacy behavior: a nil
// resolver produces empty period/source (outcomes keep no classification).
func TestResolveOutcomePeriodNilResolver(t *testing.T) {
	period, source := resolveOutcomePeriod(nil, "2026-04-01")
	if period != "" || source != "" {
		t.Errorf("nil resolver: got (%q, %q), want empty", period, source)
	}
}
