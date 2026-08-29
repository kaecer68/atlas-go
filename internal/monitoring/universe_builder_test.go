package monitoring

import (
	"context"
	"errors"
	"maps"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/screener"
)

// ============================================================================
//  Mocks
// ============================================================================

// mockMapper implements SymbolIndustryMapper.
type mockMapper struct {
	classifications map[string]*IndustryClassification
	byIndustry      map[string][]string
}

func (m *mockMapper) GetClassification(symbol string) (*IndustryClassification, bool) {
	if m == nil || m.classifications == nil {
		return nil, false
	}
	cls, ok := m.classifications[symbol]
	return cls, ok
}

func (m *mockMapper) GetSymbolsByIndustry(industryID string) []string {
	if m == nil || m.byIndustry == nil {
		return nil
	}
	return m.byIndustry[industryID]
}

// mockTree implements ClassificationTreeAccessor. IndustryFilter does not call
// into the tree inside Filter(), so all methods return empty values.
type mockTree struct{}

func (m *mockTree) GetSegment(string) (IndustrySegment, bool) { return IndustrySegment{}, false }
func (m *mockTree) GetChildren(string) []IndustrySegment      { return nil }
func (m *mockTree) GetLevel1() []IndustrySegment              { return nil }
func (m *mockTree) GetPath(string) []IndustrySegment          { return nil }

// mockSupplyChain implements SupplyChainAccessor.
type mockSupplyChain struct {
	downstream map[string][]string
	upstream   map[string][]string
}

func (m *mockSupplyChain) GetDownstream(symbol string) []string {
	if m == nil || m.downstream == nil {
		return nil
	}
	return m.downstream[symbol]
}

func (m *mockSupplyChain) GetUpstream(symbol string) []string {
	if m == nil || m.upstream == nil {
		return nil
	}
	return m.upstream[symbol]
}

// mockScreener implements screener.Screener.
type mockScreener struct {
	// passAll: when true, all symbols are returned as passed.
	passAll bool
	// reject: per-symbol reject map; entries return false.
	reject map[string]bool
	// returnErr: when non-nil, ScreenUniverse returns this error (used to
	// verify the error-fallback path in ScoringScreener.Rank).
	returnErr error
}

func (m *mockScreener) Screen(_ context.Context, symbol string, _ domain.ScreeningCriteria, _ map[string]domain.Quote) (bool, error) {
	if m.returnErr != nil {
		return false, m.returnErr
	}
	if m.passAll {
		return true, nil
	}
	return !m.reject[symbol], nil
}

func (m *mockScreener) ScreenDetailed(_ context.Context, symbol string, _ domain.ScreeningCriteria, _ map[string]domain.Quote) (screener.ScreenResult, error) {
	if m.passAll {
		return screener.ScreenResult{Passed: true}, nil
	}
	if m.reject[symbol] {
		return screener.ScreenResult{Passed: false, Reason: "mock reject", Criterion: "mock"}, nil
	}
	return screener.ScreenResult{Passed: true}, nil
}

func (m *mockScreener) ScreenUniverse(_ context.Context, symbols []string, _ domain.ScreeningCriteria, _ map[string]domain.Quote) ([]string, error) {
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	if m.passAll {
		out := make([]string, len(symbols))
		copy(out, symbols)
		return out, nil
	}
	passed := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		if !m.reject[sym] {
			passed = append(passed, sym)
		}
	}
	return passed, nil
}

// mockFactorEng implements FactorScoreProvider. Symbols in the noScores set
// return nil (interpreted as "no prior factor scores" by scoreAndRank).
type mockFactorEng struct {
	scores   map[string]map[string]float64
	noScores map[string]bool
}

func (m *mockFactorEng) CalculateAllScores(symbol string, _ map[string]domain.Quote, _ ...any) map[string]float64 {
	if m.noScores[symbol] {
		return nil
	}
	if s, ok := m.scores[symbol]; ok {
		out := make(map[string]float64, len(s))
		maps.Copy(out, s)
		return out
	}
	return nil
}

// mockRiskMgr implements RiskManager.
type mockRiskMgr struct {
	var95         float64
	contributions map[string]float64
	contribErrors map[string]error
}

func (m *mockRiskMgr) VaRContribution(symbol string) (float64, error) {
	if err, ok := m.contribErrors[symbol]; ok {
		return 0, err
	}
	return m.contributions[symbol], nil
}

func (m *mockRiskMgr) VaR95() float64 { return m.var95 }

// mockQuoteProv implements QuoteProvider.
type mockQuoteProv struct {
	quotes map[string]domain.Quote
	err    error
}

func (m *mockQuoteProv) GetQuotes(_ context.Context, _ time.Time, symbols []string) ([]domain.Quote, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]domain.Quote, 0, len(symbols))
	for _, sym := range symbols {
		if q, ok := m.quotes[sym]; ok {
			out = append(out, q)
		}
	}
	return out, nil
}

// mockPriceHistory implements HistoricalPriceProvider.
type mockPriceHistory struct {
	series map[string][]float64
}

func (m *mockPriceHistory) GetCloseSeries(symbol string) []float64 {
	if m == nil || m.series == nil {
		return nil
	}
	return m.series[symbol]
}

// ============================================================================
//  Test helpers
// ============================================================================

// alternatingReturns builds n returns of the form [+v, -v, +v, -v, ...] which
// produce a sample standard deviation of exactly v (mean = 0).
func alternatingReturns(v float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		if i%2 == 0 {
			out[i] = v
		} else {
			out[i] = -v
		}
	}
	return out
}

// pricesFromReturns turns a start price and a series of simple returns into
// a close-price series with len(returns)+1 entries.
func pricesFromReturns(start float64, returns []float64) []float64 {
	prices := make([]float64, len(returns)+1)
	prices[0] = start
	for i, r := range returns {
		prices[i+1] = prices[i] * (1 + r)
	}
	return prices
}

// ============================================================================
//  TestIndustryFilter_Filter
// ============================================================================

func TestIndustryFilter_Filter(t *testing.T) {
	type tc struct {
		name               string
		input              []string
		mapper             *mockMapper
		tree               *mockTree
		chain              *mockSupplyChain
		targetLevel1       []string
		excludeCyclicalLow bool
		expandDepth        int
		want               []string // nil ⇒ expect nil; otherwise compare element-wise
	}
	tests := []tc{
		{
			name:  "empty symbols returns nil",
			input: []string{},
			mapper: &mockMapper{
				classifications: map[string]*IndustryClassification{
					"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "semiconductor"}},
				},
			},
			want: nil,
		},
		{
			name:   "nil mapper returns nil",
			input:  []string{"2330", "2317"},
			mapper: nil,
			want:   nil,
		},
		{
			name:  "basic filter with .TW normalization",
			input: []string{"2330.TW", "2317.TW"},
			mapper: &mockMapper{
				classifications: map[string]*IndustryClassification{
					"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "semiconductor"}},
					"2317": {Symbol: "2317", Level1: IndustrySegment{ID: "tech"}},
				},
			},
			want: []string{"2330", "2317"},
		},
		{
			name:  "dedup of duplicate symbols with and without .TW",
			input: []string{"2330", "2330.TW", "2330"},
			mapper: &mockMapper{
				classifications: map[string]*IndustryClassification{
					"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "semiconductor"}},
				},
			},
			want: []string{"2330"},
		},
		{
			name:  "cyclicality low excluded when flag set",
			input: []string{"2330", "1101"},
			mapper: &mockMapper{
				classifications: map[string]*IndustryClassification{
					"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "tech", Cyclicality: "high"}},
					"1101": {Symbol: "1101", Level1: IndustrySegment{ID: "cement", Cyclicality: "low"}},
				},
			},
			excludeCyclicalLow: true,
			want:               []string{"2330"},
		},
		{
			name:  "cyclicality low kept when flag not set",
			input: []string{"1101"},
			mapper: &mockMapper{
				classifications: map[string]*IndustryClassification{
					"1101": {Symbol: "1101", Level1: IndustrySegment{ID: "cement", Cyclicality: "low"}},
				},
			},
			excludeCyclicalLow: false,
			want:               []string{"1101"},
		},
		{
			name:  "target industry filtering restricts accepted ids",
			input: []string{"2330", "1101", "2317"},
			mapper: &mockMapper{
				classifications: map[string]*IndustryClassification{
					"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "semiconductor"}},
					"1101": {Symbol: "1101", Level1: IndustrySegment{ID: "cement"}},
					"2317": {Symbol: "2317", Level1: IndustrySegment{ID: "semiconductor"}},
				},
			},
			targetLevel1: []string{"semiconductor"},
			want:         []string{"2330", "2317"},
		},
		{
			name:  "semiconductor supply chain expansion at depth 2",
			input: []string{"2330"},
			mapper: &mockMapper{
				classifications: map[string]*IndustryClassification{
					"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "semiconductor"}},
				},
				byIndustry: map[string][]string{
					"wafer_fab": {"2375", "8069"},
				},
			},
			chain: &mockSupplyChain{
				downstream: map[string][]string{
					"semiconductor": {"wafer_fab"},
				},
			},
			expandDepth: 2,
			want:        []string{"2330", "2375", "8069"},
		},
		{
			name:  "symbol not found in mapper is dropped",
			input: []string{"9999", "2330", "8888"},
			mapper: &mockMapper{
				classifications: map[string]*IndustryClassification{
					"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "tech"}},
				},
			},
			want: []string{"2330"},
		},
		{
			name:  "non-semiconductor symbol does not trigger expansion",
			input: []string{"2317"},
			mapper: &mockMapper{
				classifications: map[string]*IndustryClassification{
					"2317": {Symbol: "2317", Level1: IndustrySegment{ID: "tech"}},
				},
				byIndustry: map[string][]string{
					"any_industry": {"should_not_appear"},
				},
			},
			chain: &mockSupplyChain{
				downstream: map[string][]string{
					"tech": {"any_industry"},
				},
			},
			expandDepth: 2,
			want:        []string{"2317"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &IndustryFilter{
				Mapper:                 tt.mapper,
				Tree:                   tt.tree,
				SupplyChain:            tt.chain,
				TargetLevel1:           tt.targetLevel1,
				ExcludeCyclicalityLow:  tt.excludeCyclicalLow,
				ExpandSupplyChainDepth: tt.expandDepth,
			}
			got := f.Filter(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (got=%v want=%v)", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("index %d: got %q want %q (full got=%v want=%v)", i, got[i], tt.want[i], got, tt.want)
				}
			}
		})
	}
}

// ============================================================================
//  TestScoringScreener_Rank
// ============================================================================

func TestScoringScreener_Rank(t *testing.T) {
	now := time.Now()

	// Common quote specs reused across cases.
	highQuote := func(sym string) domain.Quote {
		return domain.Quote{
			Symbol: sym,
			Last:   100.0,
			Volume: 200_000, // approxTWD = 20M > 10M floor
			AsOf:   now,
		}
	}
	lowVolumeQuote := func(sym string) domain.Quote {
		return domain.Quote{
			Symbol: sym,
			Last:   50.0,
			Volume: 100_000, // approxTWD = 5M < 10M floor → dropped
			AsOf:   now,
		}
	}
	lowPriceQuote := func(sym string) domain.Quote {
		return domain.Quote{
			Symbol: sym,
			Last:   5.0, // below PriceMin (default 10.0)
			Volume: 5_000_000,
			AsOf:   now,
		}
	}
	staleQuote := func(sym string) domain.Quote {
		return domain.Quote{
			Symbol: sym,
			Last:   100.0,
			Volume: 200_000,
			AsOf:   now.Add(-60 * 24 * time.Hour), // > 30 days old → freshness downgrade
		}
	}

	// Standard factor-score set for the "weighted scoring" verification.
	// Expected score with default weights (0.15, 0.10, 0.15, 0.15, 0.20, 0.20):
	//   100*0.15 + 80*0.10 + 60*0.15 + 40*0.15 + 20*0.20 + 10*0.20
	// = 15 + 8 + 9 + 6 + 4 + 2 = 44.0
	fullScores := map[string]float64{
		"pe":           100,
		"pb":           80,
		"volume":       60,
		"momentum":     40,
		"quality":      20,
		"foreign_flow": 10,
	}

	// Two industry buckets for the concentration cap test.
	industryMapper := &mockMapper{
		classifications: map[string]*IndustryClassification{
			"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "tech"}},
			"2454": {Symbol: "2454", Level1: IndustrySegment{ID: "tech"}},
			"2881": {Symbol: "2881", Level1: IndustrySegment{ID: "finance"}},
			"2882": {Symbol: "2882", Level1: IndustrySegment{ID: "finance"}},
		},
	}

	type tc struct {
		name       string
		universe   []string
		quotes     map[string]domain.Quote
		screener   *mockScreener
		factorEng  *mockFactorEng
		industry   SymbolIndustryMapper
		weights    ScreenerWeights
		topN       int
		concenCap  float64
		wantLen    int // expected length of result (-1 ⇒ check scores instead)
		wantScores map[string]float64
		checkFresh bool // when true, verify ScoreFresh of first result
		freshWant  bool // expected ScoreFresh
	}

	tests := []tc{
		{
			name:     "empty universe returns empty slice",
			universe: []string{},
			quotes:   map[string]domain.Quote{},
			screener: &mockScreener{passAll: true},
			factorEng: &mockFactorEng{scores: map[string]map[string]float64{
				"2330": fullScores,
			}},
			wantLen: 0,
		},
		{
			name:     "volume filter drops low-volume symbol",
			universe: []string{"2330", "1101"},
			quotes: map[string]domain.Quote{
				"2330": highQuote("2330"),
				"1101": lowVolumeQuote("1101"),
			},
			screener: &mockScreener{passAll: true},
			factorEng: &mockFactorEng{scores: map[string]map[string]float64{
				"2330": fullScores,
				"1101": fullScores,
			}},
			wantLen: 1,
		},
		{
			name:     "price filter drops sub-PriceMin symbol",
			universe: []string{"2330", "LOW"},
			quotes: map[string]domain.Quote{
				"2330": highQuote("2330"),
				"LOW":  lowPriceQuote("LOW"),
			},
			screener: &mockScreener{passAll: true},
			factorEng: &mockFactorEng{scores: map[string]map[string]float64{
				"2330": fullScores,
				"LOW":  fullScores,
			}},
			wantLen: 1,
		},
		{
			name:     "screener error falls back to survivors from volume/price filters",
			universe: []string{"2330", "1101"},
			quotes: map[string]domain.Quote{
				"2330": highQuote("2330"),
				"1101": highQuote("1101"),
			},
			screener: &mockScreener{returnErr: errors.New("boom")},
			factorEng: &mockFactorEng{scores: map[string]map[string]float64{
				"2330": fullScores,
				"1101": fullScores,
			}},
			// Set industry mapper to keep both symbols in different buckets
			// so the default concentration cap does not drop one of them.
			industry: &mockMapper{
				classifications: map[string]*IndustryClassification{
					"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "tech"}},
					"1101": {Symbol: "1101", Level1: IndustrySegment{ID: "cement"}},
				},
			},
			concenCap: 1.0,
			wantLen:   2,
		},
		{
			name:     "weighted scoring uses expected weight math",
			universe: []string{"2330"},
			quotes:   map[string]domain.Quote{"2330": highQuote("2330")},
			screener: &mockScreener{passAll: true},
			factorEng: &mockFactorEng{scores: map[string]map[string]float64{
				"2330": fullScores,
			}},
			wantLen: 1,
		},
		{
			name:     "freshness downgrade sets score to 30 and ScoreFresh false",
			universe: []string{"2330"},
			quotes:   map[string]domain.Quote{"2330": staleQuote("2330")},
			screener: &mockScreener{passAll: true},
			factorEng: &mockFactorEng{scores: map[string]map[string]float64{
				"2330": fullScores,
			}},
			wantLen:    1,
			checkFresh: true,
			freshWant:  false,
			wantScores: map[string]float64{"2330": 30.0},
		},
		{
			name:     "fresh quote keeps ScoreFresh true",
			universe: []string{"2330"},
			quotes:   map[string]domain.Quote{"2330": highQuote("2330")},
			screener: &mockScreener{passAll: true},
			factorEng: &mockFactorEng{scores: map[string]map[string]float64{
				"2330": fullScores,
			}},
			wantLen:    1,
			checkFresh: true,
			freshWant:  true,
		},
		{
			name:     "symbols without factor scores are excluded",
			universe: []string{"2330", "9999"},
			quotes: map[string]domain.Quote{
				"2330": highQuote("2330"),
				"9999": highQuote("9999"),
			},
			screener: &mockScreener{passAll: true},
			factorEng: &mockFactorEng{
				scores: map[string]map[string]float64{
					"2330": fullScores,
				},
				noScores: map[string]bool{"9999": true},
			},
			wantLen: 1,
		},
		{
			name:     "concentration cap trims over-represented industry to top per bucket",
			universe: []string{"2330", "2454", "2881", "2882"},
			quotes: map[string]domain.Quote{
				"2330": highQuote("2330"),
				"2454": highQuote("2454"),
				"2881": highQuote("2881"),
				"2882": highQuote("2882"),
			},
			screener: &mockScreener{passAll: true},
			factorEng: &mockFactorEng{scores: map[string]map[string]float64{
				"2330": fullScores,
				"2454": {"pe": 50, "pb": 50, "volume": 50, "momentum": 50, "quality": 50, "foreign_flow": 50},
				"2881": fullScores,
				"2882": {"pe": 50, "pb": 50, "volume": 50, "momentum": 50, "quality": 50, "foreign_flow": 50},
			}},
			industry:  industryMapper,
			concenCap: 0.25,
			wantLen:   2, // 1 per industry (0.25 * 4 = 1)
		},
		{
			name:     "TopN cuts result to configured size",
			universe: []string{"2330", "2454", "2881", "2882", "1101"},
			quotes: map[string]domain.Quote{
				"2330": highQuote("2330"),
				"2454": highQuote("2454"),
				"2881": highQuote("2881"),
				"2882": highQuote("2882"),
				"1101": highQuote("1101"),
			},
			screener: &mockScreener{passAll: true},
			factorEng: &mockFactorEng{scores: map[string]map[string]float64{
				"2330": fullScores,
				"2454": fullScores,
				"2881": fullScores,
				"2882": fullScores,
				"1101": fullScores,
			}},
			industry:  industryMapper,
			concenCap: 0.5, // 0.5*5 = 2 per industry — single industry falls back to 1
			topN:      3,
			wantLen:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &ScoringScreener{
				Screener:                 tt.screener,
				FactorEng:                tt.factorEng,
				IndustryMapper:           tt.industry,
				Weights:                  tt.weights,
				TopN:                     tt.topN,
				MaxIndustryConcentration: tt.concenCap,
				VolumeFloorTWD:           10_000_000,
				PriceMin:                 10.0,
				FactorScoreMaxAge:        30 * 24 * time.Hour,
			}
			if s.Weights.PE == 0 && s.Weights.PB == 0 {
				s.Weights = DefaultScreenerWeights()
			}
			if s.MaxIndustryConcentration == 0 {
				s.MaxIndustryConcentration = 0.25
			}
			if s.TopN == 0 {
				s.TopN = 50
			}

			got := s.Rank(tt.universe, tt.quotes)
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d (got=%v)", len(got), tt.wantLen, got)
			}

			// Verify weighted scoring math (single-symbol case).
			if tt.name == "weighted scoring uses expected weight math" && len(got) == 1 {
				expected := 100*0.15 + 80*0.10 + 60*0.15 + 40*0.15 + 20*0.20 + 10*0.20
				if math.Abs(got[0].Score-expected) > 1e-9 {
					t.Errorf("Score = %v, want %v", got[0].Score, expected)
				}
			}

			// Verify freshness flag where requested.
			if tt.checkFresh && len(got) == 1 {
				if got[0].ScoreFresh != tt.freshWant {
					t.Errorf("ScoreFresh = %v, want %v", got[0].ScoreFresh, tt.freshWant)
				}
			}

			// Verify per-symbol score values where specified.
			for sym, wantScore := range tt.wantScores {
				var found bool
				for _, r := range got {
					if r.Symbol == sym {
						if math.Abs(r.Score-wantScore) > 1e-9 {
							t.Errorf("Score[%s] = %v, want %v", sym, r.Score, wantScore)
						}
						found = true
						break
					}
				}
				if !found {
					t.Errorf("symbol %s missing from result (got %v)", sym, got)
				}
			}
		})
	}
}

// ============================================================================
//  TestRiskExclusionFilter_Filter
// ============================================================================

func TestRiskExclusionFilter_Filter(t *testing.T) {
	now := time.Now()

	// Helper: build a price series of n prices that produces a specific
	// realized volatility (alternating returns [+v, -v, ...]).
	pricesWithVol := func(vol float64, n int) []float64 {
		return pricesFromReturns(100, alternatingReturns(vol, n-1))
	}

	// Build a steep drawdown series: 100 → 50 → 110. Max DD = (100-50)/100 = 0.50.
	steepDD := []float64{100, 95, 80, 65, 50, 60, 110}
	// A series with no significant drawdown: monotonic-ish rise.
	shallowDD := []float64{100, 102, 101, 103, 105, 104, 106}

	// Quote with dailyAmount >> 5M floor.
	liquidQuote := func(sym string) domain.Quote {
		return domain.Quote{
			Symbol: sym,
			Last:   100.0,
			Volume: 200_000, // 20M TWD
			AsOf:   now,
		}
	}
	// Quote with dailyAmount << 5M floor.
	illiquidQuote := func(sym string) domain.Quote {
		return domain.Quote{
			Symbol: sym,
			Last:   10.0,
			Volume: 100_000, // 1M TWD
			AsOf:   now,
		}
	}

	type tc struct {
		name        string
		symbols     []string
		riskMgr     *mockRiskMgr
		quoteProv   *mockQuoteProv
		priceHist   *mockPriceHistory
		wantErr     bool
		wantResults []RiskExclusionResult // exact-match expectation (len must equal len(symbols))
	}
	tests := []tc{
		{
			name:      "empty symbols returns nil, nil",
			symbols:   []string{},
			riskMgr:   &mockRiskMgr{var95: 4.0, contributions: map[string]float64{}},
			quoteProv: &mockQuoteProv{quotes: map[string]domain.Quote{}},
			priceHist: &mockPriceHistory{series: map[string][]float64{}},
			wantErr:   false,
			// wantResults is empty — the function should return nil slices.
		},
		{
			name:    "VaR contribution above threshold is excluded",
			symbols: []string{"2330", "1101"},
			riskMgr: &mockRiskMgr{
				var95: 4.0, // |VaR95| = 4, n=2 → portfolioAvgVar = 2
				contributions: map[string]float64{
					"2330": 3.0, // threshold = 2*2=4, 3 ≤ 4 → pass
					"1101": 5.0, // threshold = 4, 5 > 4 → fail
				},
			},
			quoteProv: &mockQuoteProv{quotes: map[string]domain.Quote{
				"2330": liquidQuote("2330"),
				"1101": liquidQuote("1101"),
			}},
			priceHist: &mockPriceHistory{series: map[string][]float64{
				"2330": shallowDD,
				"1101": shallowDD,
			}},
			wantResults: []RiskExclusionResult{
				{Symbol: "2330", Passed: true, FailReasons: nil, HighRisk: false},
				{Symbol: "1101", Passed: false, FailReasons: []string{"var_contribution"}, HighRisk: false},
			},
		},
		{
			name:    "volatility above 2x median fails volatility check",
			symbols: []string{"LOW_VOL", "MED_VOL", "HIGH_VOL"},
			riskMgr: &mockRiskMgr{
				var95: 0,
				contributions: map[string]float64{
					"LOW_VOL": 0, "MED_VOL": 0, "HIGH_VOL": 0,
				},
			},
			quoteProv: &mockQuoteProv{quotes: map[string]domain.Quote{
				"LOW_VOL":  liquidQuote("LOW_VOL"),
				"MED_VOL":  liquidQuote("MED_VOL"),
				"HIGH_VOL": liquidQuote("HIGH_VOL"),
			}},
			priceHist: &mockPriceHistory{series: map[string][]float64{
				"LOW_VOL": pricesWithVol(0.02, 31),
				"MED_VOL": pricesWithVol(0.10, 31),
				// median=0.10, threshold=0.20; 0.30 > 0.20 → volatility fail.
				// Alternating ±30% also drives a deep drawdown, so the
				// drawdown check independently flags HighRisk=true.
				"HIGH_VOL": pricesWithVol(0.30, 31),
			}},
			wantResults: []RiskExclusionResult{
				{Symbol: "LOW_VOL", Passed: true, HighRisk: false},
				{Symbol: "MED_VOL", Passed: true, HighRisk: false},
				{Symbol: "HIGH_VOL", Passed: false, HighRisk: true, FailReasons: []string{"volatility"}},
			},
		},
		{
			name:    "drawdown above 30% flags HighRisk but does not fail",
			symbols: []string{"OK_DD", "STEEP_DD"},
			riskMgr: &mockRiskMgr{
				var95: 0,
				contributions: map[string]float64{
					"OK_DD":    0,
					"STEEP_DD": 0,
				},
			},
			quoteProv: &mockQuoteProv{quotes: map[string]domain.Quote{
				"OK_DD":    liquidQuote("OK_DD"),
				"STEEP_DD": liquidQuote("STEEP_DD"),
			}},
			priceHist: &mockPriceHistory{series: map[string][]float64{
				"OK_DD":    shallowDD,
				"STEEP_DD": steepDD,
			}},
			wantResults: []RiskExclusionResult{
				{Symbol: "OK_DD", Passed: true, HighRisk: false},
				{Symbol: "STEEP_DD", Passed: true, HighRisk: true},
			},
		},
		{
			name:    "liquidity below 5M TWD fails liquidity check",
			symbols: []string{"LIQ", "ILLIQ"},
			riskMgr: &mockRiskMgr{
				var95: 0,
				contributions: map[string]float64{
					"LIQ":   0,
					"ILLIQ": 0,
				},
			},
			quoteProv: &mockQuoteProv{quotes: map[string]domain.Quote{
				"LIQ":   liquidQuote("LIQ"),
				"ILLIQ": illiquidQuote("ILLIQ"),
			}},
			priceHist: &mockPriceHistory{series: map[string][]float64{
				"LIQ":   shallowDD,
				"ILLIQ": shallowDD,
			}},
			wantResults: []RiskExclusionResult{
				{Symbol: "LIQ", Passed: true},
				{Symbol: "ILLIQ", Passed: false, FailReasons: []string{"liquidity"}},
			},
		},
		{
			name:    "all providers nil → every symbol passes",
			symbols: []string{"S1", "S2", "S3"},
			riskMgr: nil,
			// quoteProv and priceHist left as zero values (nil).
			wantResults: []RiskExclusionResult{
				{Symbol: "S1", Passed: true, HighRisk: false},
				{Symbol: "S2", Passed: true, HighRisk: false},
				{Symbol: "S3", Passed: true, HighRisk: false},
			},
		},
		{
			name:    "mixed pass and fail across checks",
			symbols: []string{"GOOD", "VAR_FAIL", "LIQ_FAIL", "BOTH_FAIL"},
			riskMgr: &mockRiskMgr{
				var95: 4.0, // avg = 1.0, threshold = 2.0
				contributions: map[string]float64{
					"GOOD":      0.5, // pass
					"VAR_FAIL":  3.0, // > 2.0 threshold
					"LIQ_FAIL":  0.5, // pass VaR
					"BOTH_FAIL": 3.0, // fail VaR
				},
			},
			quoteProv: &mockQuoteProv{quotes: map[string]domain.Quote{
				"GOOD":      liquidQuote("GOOD"),
				"VAR_FAIL":  liquidQuote("VAR_FAIL"),
				"LIQ_FAIL":  illiquidQuote("LIQ_FAIL"),
				"BOTH_FAIL": illiquidQuote("BOTH_FAIL"),
			}},
			priceHist: &mockPriceHistory{series: map[string][]float64{
				"GOOD":      shallowDD,
				"VAR_FAIL":  shallowDD,
				"LIQ_FAIL":  shallowDD,
				"BOTH_FAIL": shallowDD,
			}},
			wantResults: []RiskExclusionResult{
				{Symbol: "GOOD", Passed: true},
				{Symbol: "VAR_FAIL", Passed: false, FailReasons: []string{"var_contribution"}},
				{Symbol: "LIQ_FAIL", Passed: false, FailReasons: []string{"liquidity"}},
				{Symbol: "BOTH_FAIL", Passed: false, FailReasons: []string{"var_contribution", "liquidity"}},
			},
		},
		{
			name:    "quote fetch error propagates as error",
			symbols: []string{"S1"},
			riskMgr: &mockRiskMgr{
				var95: 1.0,
				contributions: map[string]float64{
					"S1": 0.5,
				},
			},
			quoteProv: &mockQuoteProv{err: errors.New("provider offline")},
			priceHist: &mockPriceHistory{series: map[string][]float64{
				"S1": shallowDD,
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rf *RiskExclusionFilter
			if tt.riskMgr == nil && tt.quoteProv == nil && tt.priceHist == nil {
				rf = NewRiskExclusionFilter(nil, nil, nil)
			} else {
				var qp QuoteProvider
				if tt.quoteProv != nil {
					qp = tt.quoteProv
				}
				var ph HistoricalPriceProvider
				if tt.priceHist != nil {
					ph = tt.priceHist
				}
				rf = NewRiskExclusionFilter(tt.riskMgr, qp, ph)
			}

			results, err := rf.Filter(tt.symbols)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (results=%v)", results)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tt.symbols) == 0 {
				if results != nil {
					t.Fatalf("expected nil results for empty input, got %v", results)
				}
				return
			}
			if len(results) != len(tt.wantResults) {
				t.Fatalf("len(results)=%d, want %d (got=%v)", len(results), len(tt.wantResults), results)
			}
			for i, want := range tt.wantResults {
				got := results[i]
				if got.Symbol != want.Symbol {
					t.Errorf("idx %d: Symbol = %q, want %q", i, got.Symbol, want.Symbol)
				}
				if got.Passed != want.Passed {
					t.Errorf("idx %d (%s): Passed = %v, want %v", i, got.Symbol, got.Passed, want.Passed)
				}
				if got.HighRisk != want.HighRisk {
					t.Errorf("idx %d (%s): HighRisk = %v, want %v", i, got.Symbol, got.HighRisk, want.HighRisk)
				}
				if !sameStringSet(got.FailReasons, want.FailReasons) {
					t.Errorf("idx %d (%s): FailReasons = %v, want %v", i, got.Symbol, got.FailReasons, want.FailReasons)
				}
			}
		})
	}
}

// sameStringSet returns true when both slices contain the same elements
// regardless of order. Treats nil and empty as equal.
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}
