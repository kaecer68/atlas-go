// Package monitoring hosts Layer 1 (IndustryFilter) and Layer 2 (ScoringScreener)
// of the SmartUniverseBuilder. Together they narrow the broad TWSE/TPEX symbol
// universe (~1800 names) down to a Top-N ranked candidate pool driven by the
// SP4 §9 parameter table.
//
// Layer 1 enforces Level-1 industry membership, cyclicality preference, and
// semiconductor supply-chain expansion.
// Layer 2 runs volume + price filters, delegates binary pass/fail to the
// screener.Engine, applies weighted factor scoring, enforces an industry
// concentration cap, and returns the top N symbols.
package monitoring

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/screener"
)

// ── Local mirrors (avoid monitoring ↔ marketdata circular import) ────────────

// SymbolIndustryMapper defines the minimal contract required for symbol→industry
// lookups. Implementations are expected to be nil-safe and may normalize symbols
// internally; IndustryFilter will normalize to the .TW-stripped form before
// calling.
type SymbolIndustryMapper interface {
	GetClassification(symbol string) (*IndustryClassification, bool)
	GetSymbolsByIndustry(industryID string) []string
}

// IndustryClassification is a local mirror of industry.IndustryClassification
// keeping only the fields the SmartUniverseBuilder consumes.
type IndustryClassification struct {
	Symbol string          `json:"symbol"`
	Level1 IndustrySegment `json:"level1"`
	Level2 IndustrySegment `json:"level2"`
	Level3 IndustrySegment `json:"level3"`
}

// IndustrySegment mirrors industry.IndustrySegment with the subset of fields
// consumed here. Cyclicality is kept as a string to avoid importing industry.Cyclicality.
type IndustrySegment struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	NameEN               string   `json:"name_en"`
	Level                int      `json:"level"`
	ParentID             string   `json:"parent_id,omitempty"`
	Weight               float64  `json:"weight,omitempty"`
	Cyclicality          string   `json:"cyclicality,omitempty"`           // "high" | "medium" | "low"
	RepresentativeStocks []string `json:"representative_stocks,omitempty"` // mirror of industry.IndustrySegment.RepresentativeStocks
}

// ClassificationTreeAccessor mirrors industry.ClassificationTree methods we
// need so callers can supply either the real tree or a test stub.
type ClassificationTreeAccessor interface {
	GetSegment(id string) (IndustrySegment, bool)
	GetChildren(id string) []IndustrySegment
	GetLevel1() []IndustrySegment
	GetPath(id string) []IndustrySegment
}

// SupplyChainAccessor mirrors industry.SupplyChainGraph methods.
type SupplyChainAccessor interface {
	GetDownstream(symbol string) []string
	GetUpstream(symbol string) []string
}

// ── Layer 1: IndustryFilter ──────────────────────────────────────────────────

// IndustryFilter narrows a broad symbol universe (~1800) down to a candidate
// pool (400-600) by Level-1 industry membership, cyclicality preference, and
// supply-chain expansion for semiconductor-related symbols.
type IndustryFilter struct {
	// Mapper resolves a symbol to its IndustryClassification.
	Mapper SymbolIndustryMapper
	// Tree supplies segment metadata for cyclicality lookups (reserved).
	Tree ClassificationTreeAccessor
	// SupplyChain is consulted for downstream expansion when a symbol belongs
	// to the semiconductor industry.
	SupplyChain SupplyChainAccessor
	// TargetLevel1 restricts accepted symbols to those whose Level-1 industry
	// ID is in this list. An empty list disables the filter.
	TargetLevel1 []string
	// ExcludeCyclicalityLow drops industries whose Cyclicality == "low".
	ExcludeCyclicalityLow bool
	// ExpandSupplyChainDepth sets how many tiers downstream of the
	// semiconductor Level-1 are expanded. Defaults to 2 when zero.
	ExpandSupplyChainDepth int
}

// NewIndustryFilter returns an IndustryFilter with the canonical defaults from
// SP4 §9 (depth=2, cyclicality-low not excluded, no target list).
func NewIndustryFilter(mapper SymbolIndustryMapper, tree ClassificationTreeAccessor, chain SupplyChainAccessor) *IndustryFilter {
	return &IndustryFilter{
		Mapper:                 mapper,
		Tree:                   tree,
		SupplyChain:            chain,
		ExpandSupplyChainDepth: 2,
	}
}

// Filter returns the deduplicated, .TW-normalized symbol list that passes the
// Layer-1 industry constraints. Symbols the mapper cannot classify are dropped
// silently (callers see this as a smaller output, not an error).
func (f *IndustryFilter) Filter(symbols []string) []string {
	if f.Mapper == nil {
		return nil
	}
	depth := f.ExpandSupplyChainDepth
	if depth == 0 {
		depth = 2
	}
	targetSet := make(map[string]bool, len(f.TargetLevel1))
	for _, id := range f.TargetLevel1 {
		targetSet[id] = true
	}
	seen := make(map[string]bool)
	var result []string
	add := func(sym string) {
		norm := normalizeSymbol(sym)
		if seen[norm] {
			return
		}
		seen[norm] = true
		result = append(result, norm)
	}
	// Hoist BFS allocations outside the symbol loop so visited/queue/nextDepth
	// slices and maps are reused across semiconductor expansions.
	var visited map[string]bool
	var queue, nextDepth []string
	for _, sym := range symbols {
		cls, ok := f.Mapper.GetClassification(normalizeSymbol(sym))
		if !ok {
			continue
		}
		if len(targetSet) > 0 && !targetSet[cls.Level1.ID] {
			continue
		}
		if f.ExcludeCyclicalityLow && strings.EqualFold(cls.Level1.Cyclicality, "low") {
			continue
		}
		add(sym)
		if isSemiconductor(cls) && f.SupplyChain != nil && depth > 0 {
			visited = map[string]bool{cls.Level1.ID: true}
			queue = append(queue[:0], cls.Level1.ID)
			curDepth := 0
			for len(queue) > 0 && curDepth < depth {
				nextDepth = nextDepth[:0]
				for _, id := range queue {
					for _, downID := range f.SupplyChain.GetDownstream(id) {
						if visited[downID] {
							continue
						}
						visited[downID] = true
						nextDepth = append(nextDepth, downID)
						for _, ds := range f.Mapper.GetSymbolsByIndustry(downID) {
							add(ds)
						}
					}
				}
				queue = nextDepth
				curDepth++
			}
		}
	}
	return result
}

// semiconductorMarkers is the set of strings used to detect semiconductor
// industry membership in Chinese or English industry names/IDs.
var semiconductorMarkers = []string{"半導體", "semiconductor"}

// isSemiconductor returns true when the symbol belongs to the semiconductor
// Level-1 or Level-2 industry (matches Chinese and English names defensively).
func isSemiconductor(cls *IndustryClassification) bool {
	if cls == nil {
		return false
	}
	if strings.EqualFold(cls.Level1.ID, "semiconductor") || strings.EqualFold(cls.Level2.ID, "semiconductor") {
		return true
	}
	for _, marker := range semiconductorMarkers {
		if strings.Contains(strings.ToLower(cls.Level1.Name), marker) {
			return true
		}
	}
	return strings.Contains(strings.ToLower(cls.Level1.NameEN), "semiconductor")
}

// normalizeSymbol strips a trailing ".TW" suffix so 2330 and 2330.TW are
// treated as the same canonical internal form.
func normalizeSymbol(symbol string) string {
	return strings.TrimSuffix(symbol, ".TW")
}

// ── Adapters for real industry types ─────────────────────────────────────────

// AdaptClassificationTree wraps *industry.ClassificationTree so it satisfies
// ClassificationTreeAccessor using local IndustrySegment values.
func AdaptClassificationTree(tree *industry.ClassificationTree) ClassificationTreeAccessor {
	return &classificationTreeAdapter{tree: tree}
}

type classificationTreeAdapter struct {
	tree *industry.ClassificationTree
}

func (a *classificationTreeAdapter) GetSegment(id string) (IndustrySegment, bool) {
	seg, ok := a.tree.GetSegment(id)
	if !ok || seg == nil {
		return IndustrySegment{}, false
	}
	return adaptSegment(seg), true
}

func (a *classificationTreeAdapter) GetChildren(id string) []IndustrySegment {
	children := a.tree.GetChildren(id)
	out := make([]IndustrySegment, 0, len(children))
	for _, c := range children {
		if c == nil {
			continue
		}
		out = append(out, adaptSegment(c))
	}
	return out
}

func (a *classificationTreeAdapter) GetLevel1() []IndustrySegment {
	segs := a.tree.GetLevel1()
	out := make([]IndustrySegment, 0, len(segs))
	for _, s := range segs {
		if s == nil {
			continue
		}
		out = append(out, adaptSegment(s))
	}
	return out
}

func (a *classificationTreeAdapter) GetPath(id string) []IndustrySegment {
	segs := a.tree.GetPath(id)
	out := make([]IndustrySegment, 0, len(segs))
	for _, s := range segs {
		if s == nil {
			continue
		}
		out = append(out, adaptSegment(s))
	}
	return out
}

func adaptSegment(seg *industry.IndustrySegment) IndustrySegment {
	// Defensive copy of representative stocks to avoid sharing the backing array.
	stocks := make([]string, len(seg.RepresentativeStocks))
	copy(stocks, seg.RepresentativeStocks)
	return IndustrySegment{
		ID:                   seg.ID,
		Name:                 seg.Name,
		NameEN:               seg.NameEN,
		Level:                int(seg.Level),
		ParentID:             seg.ParentID,
		Weight:               seg.Weight,
		Cyclicality:          string(seg.Cyclicality),
		RepresentativeStocks: stocks,
	}
}

// AdaptSupplyChainGraph wraps *industry.SupplyChainGraph so it satisfies
// SupplyChainAccessor. Downstream/upstream queries return a single tier by
// default; callers needing deeper traversal should walk the graph directly.
func AdaptSupplyChainGraph(graph *industry.SupplyChainGraph) SupplyChainAccessor {
	return &supplyChainAdapter{graph: graph}
}

type supplyChainAdapter struct {
	graph *industry.SupplyChainGraph
}

func (a *supplyChainAdapter) GetDownstream(symbol string) []string {
	return a.graph.GetDownstreamChain(symbol, 1)
}

func (a *supplyChainAdapter) GetUpstream(symbol string) []string {
	return a.graph.GetUpstreamChain(symbol, 1)
}

// ── Layer 2: ScoringScreener ─────────────────────────────────────────────────

// ScreenerWeights holds the SP4 §9 default weights (sum = 0.95 — missing
// factor scores contribute 0, so the weighted total stays rankable).
type ScreenerWeights struct {
	PE          float64 `json:"pe"`           // 0.15
	PB          float64 `json:"pb"`           // 0.10
	Volume      float64 `json:"volume"`       // 0.15
	Momentum    float64 `json:"momentum"`     // 0.15
	Quality     float64 `json:"quality"`      // 0.20
	ForeignFlow float64 `json:"foreign_flow"` // 0.20
}

// DefaultScreenerWeights returns the SP4 §9 default weights.
func DefaultScreenerWeights() ScreenerWeights {
	return ScreenerWeights{PE: 0.15, PB: 0.10, Volume: 0.15, Momentum: 0.15, Quality: 0.20, ForeignFlow: 0.20}
}

// RankedSymbol is the Layer-2 output for one candidate.
type RankedSymbol struct {
	Symbol          string             `json:"symbol"`
	Score           float64            `json:"score"`
	Industry        string             `json:"industry"`
	FactorBreakdown map[string]float64 `json:"factor_breakdown"`
	ScoreFresh      bool               `json:"score_fresh"`
}

// FactorScoreProvider is the minimal contract ScoringScreener needs from a
// factor engine. The variadic extras slot lets implementations consume bridge
// inputs without breaking the simple call sites.
//
// Note: ScoringScreener does not propagate extras to the engine — the engine
// is expected to use its own injected providers.
type FactorScoreProvider interface {
	CalculateAllScores(symbol string, quotes map[string]domain.Quote, extras ...any) map[string]float64
}

// ScoringScreener ranks a candidate universe with 6-factor weighted scoring,
// applies an industry concentration cap, and returns the top-N symbols.
type ScoringScreener struct {
	// Screener performs binary pass/fail. Required.
	Screener screener.Screener
	// FactorEng supplies per-symbol factor scores. Required.
	FactorEng FactorScoreProvider
	// IndustryMapper is used to populate RankedSymbol.Industry and to
	// enforce the concentration cap. Optional; nil ⇒ all symbols group under
	// "unknown".
	IndustryMapper SymbolIndustryMapper
	// Weights holds the 6-factor weight table. Use DefaultScreenerWeights().
	Weights ScreenerWeights
	// TopN is the number of symbols returned after ranking. Default 150.
	TopN int
	// MaxIndustryConcentration caps the share of one industry in the ranked
	// output (0-1]. Default 0.40.
	MaxIndustryConcentration float64
	// VolumeFloorTWD is the minimum approximate daily TWD volume (quote.Volume
	// * quote.Last). Default 10,000,000.
	VolumeFloorTWD float64
	// PriceMin is the minimum last price in TWD. Default 10.0.
	PriceMin float64
	// FactorScoreMaxAge downgrades factor scores older than this to 30/100.
	// Default 30 days.
	FactorScoreMaxAge time.Duration
	// ScreeningCriteria is passed verbatim to Screener.ScreenUniverse.
	ScreeningCriteria domain.ScreeningCriteria
}

// NewScoringScreener returns a ScoringScreener populated with the SP4 §9
// defaults. Callers may override individual fields on the returned struct.
func NewScoringScreener(scr screener.Screener, factorEng FactorScoreProvider) *ScoringScreener {
	return &ScoringScreener{
		Screener:                 scr,
		FactorEng:                factorEng,
		Weights:                  DefaultScreenerWeights(),
		TopN:                     150,
		MaxIndustryConcentration: 0.40,
		VolumeFloorTWD:           10_000_000,
		PriceMin:                 10.0,
		FactorScoreMaxAge:        30 * 24 * time.Hour,
	}
}

// Rank scores the candidate universe and returns the top-N ranked symbols
// with the industry concentration cap applied.
//
// Steps:
//  1. Volume filter (quote.Volume * quote.Last vs VolumeFloorTWD).
//  2. Price filter (quote.Last vs PriceMin).
//  3. Binary pass/fail via Screener.ScreenUniverse.
//  4. Per-symbol factor scoring via FactorEng.
//  5. Weighted aggregation using Weights; freshness downgrade applied.
//  6. Sort descending by Score.
//  7. Apply MaxIndustryConcentration cap.
//  8. Take TopN.
func (s *ScoringScreener) Rank(universe []string, quotes map[string]domain.Quote) []RankedSymbol {
	normalizedQuotes := normalizeQuotes(quotes)

	// Pre-compute normalized universe once so downstream filters avoid
	// repeated normalizeSymbol calls inside tight loops.
	normalizedUniverse := make([]string, len(universe))
	for i, sym := range universe {
		normalizedUniverse[i] = normalizeSymbol(sym)
	}

	survivors := s.applyVolumeAndPriceFilters(normalizedUniverse, normalizedQuotes)

	// Binary pass/fail via the injected screener.
	// Per screener contract errors are non-fatal; fall back to the survivors.
	ctx := context.Background()
	passed, err := s.Screener.ScreenUniverse(ctx, survivors, s.ScreeningCriteria, normalizedQuotes)
	if err != nil {
		passed = survivors
	}

	ranked := s.scoreAndRank(passed, normalizedQuotes)
	ranked = s.ApplyConcentrationCap(ranked)
	if s.TopN > 0 && len(ranked) > s.TopN {
		ranked = ranked[:s.TopN]
	}
	return ranked
}

// applyVolumeAndPriceFilters drops symbols that lack quotes or fail the
// approximate-TWD-volume or last-price thresholds.
func (s *ScoringScreener) applyVolumeAndPriceFilters(universe []string, quotes map[string]domain.Quote) []string {
	var survivors []string
	for _, sym := range universe {
		q, ok := quotes[sym]
		if !ok {
			continue
		}
		// Approximate TWD volume: Volume * Last. Proper ADV would require
		// HistoricalPrices; see SP4 §9.
		approxTWD := float64(q.Volume) * q.Last
		if approxTWD < s.VolumeFloorTWD || q.Last < s.PriceMin {
			continue
		}
		survivors = append(survivors, sym)
	}
	return survivors
}

// scoreAndRank produces RankedSymbol entries with weighted scores and
// freshness flags. Symbols without prior factor scores are excluded entirely
// (not downgraded) per SP4 §9.
func (s *ScoringScreener) scoreAndRank(passed []string, quotes map[string]domain.Quote) []RankedSymbol {
	ranked := make([]RankedSymbol, 0, len(passed))
	for _, sym := range passed {
		rawScores := s.FactorEng.CalculateAllScores(sym, quotes)
		if len(rawScores) == 0 {
			continue
		}
		breakdown := map[string]float64{
			"pe":           pickScore(rawScores, "pe", "value"),
			"pb":           pickScore(rawScores, "pb"),
			"volume":       pickScore(rawScores, "volume", "liquidity"),
			"momentum":     pickScore(rawScores, "momentum"),
			"quality":      pickScore(rawScores, "quality"),
			"foreign_flow": pickScore(rawScores, "foreign_flow", "institutional_sentiment"),
		}
		weighted := breakdown["pe"]*s.Weights.PE +
			breakdown["pb"]*s.Weights.PB +
			breakdown["volume"]*s.Weights.Volume +
			breakdown["momentum"]*s.Weights.Momentum +
			breakdown["quality"]*s.Weights.Quality +
			breakdown["foreign_flow"]*s.Weights.ForeignFlow

		// Freshness proxy: the most recent quote timestamp associated with the
		// symbol. If the proxy predates FactorScoreMaxAge, the score is
		// downgraded to 30/100.
		recordedAt := time.Time{}
		if q, ok := quotes[sym]; ok {
			recordedAt = q.AsOf
		}
		score, fresh := s.CheckFactorScoreFreshness(weighted, recordedAt)

		ranked = append(ranked, RankedSymbol{
			Symbol:          sym,
			Score:           score,
			Industry:        s.inferIndustry(sym),
			FactorBreakdown: breakdown,
			ScoreFresh:      fresh,
		})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
	return ranked
}

// ApplyConcentrationCap removes the lowest-scoring symbols from any industry
// whose count exceeds MaxIndustryConcentration * len(ranked). The returned
// slice is re-sorted descending by Score.
func (s *ScoringScreener) ApplyConcentrationCap(ranked []RankedSymbol) []RankedSymbol {
	if len(ranked) == 0 {
		return ranked
	}
	maxCount := max(int(s.MaxIndustryConcentration*float64(len(ranked))), 1)
	byIndustry := make(map[string][]RankedSymbol, len(ranked))
	for _, r := range ranked {
		byIndustry[r.Industry] = append(byIndustry[r.Industry], r)
	}
	kept := make([]RankedSymbol, 0, len(ranked))
	for _, list := range byIndustry {
		if len(list) <= maxCount {
			kept = append(kept, list...)
			continue
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Score > list[j].Score })
		kept = append(kept, list[:maxCount]...)
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Score > kept[j].Score })
	return kept
}

// CheckFactorScoreFreshness returns (30.0, false) when recordedAt predates
// FactorScoreMaxAge; otherwise it returns the original score and true.
// A zero recordedAt is treated as fresh.
func (s *ScoringScreener) CheckFactorScoreFreshness(score float64, recordedAt time.Time) (float64, bool) {
	if !recordedAt.IsZero() && time.Since(recordedAt) > s.FactorScoreMaxAge {
		return 30.0, false
	}
	return score, true
}

func pickScore(scores map[string]float64, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := scores[k]; ok {
			return v
		}
	}
	return 0
}

func (s *ScoringScreener) inferIndustry(symbol string) string {
	if s.IndustryMapper == nil {
		return "unknown"
	}
	if cls, ok := s.IndustryMapper.GetClassification(symbol); ok {
		return cls.Level1.ID
	}
	return "unknown"
}

func normalizeQuotes(quotes map[string]domain.Quote) map[string]domain.Quote {
	out := make(map[string]domain.Quote, len(quotes))
	for k, v := range quotes {
		out[normalizeSymbol(k)] = v
	}
	return out
}

// ── Adapter for portfolio.FactorEngine ───────────────────────────────────────

// AdaptFactorEngine wraps a *portfolio.FactorEngine as a FactorScoreProvider.
// The engine emits scores on [-1, 1]; this adapter maps them to [0, 100] and
// renames factor keys to the names ScoringScreener expects.
//
// Note: the minimal FactorScoreProvider interface intentionally omits
// fundamental data and ADV history, so precise valuation/liquidity scoring is
// not possible without a richer provider implementation.
func AdaptFactorEngine(fe *portfolio.FactorEngine) FactorScoreProvider {
	return &factorEngineAdapter{fe: fe}
}

type factorEngineAdapter struct {
	fe *portfolio.FactorEngine
}

func (a *factorEngineAdapter) CalculateAllScores(symbol string, quotes map[string]domain.Quote, _ ...any) map[string]float64 {
	if a.fe == nil {
		return nil
	}
	engineScores := a.fe.CalculateAllScores(symbol, quotes, nil, nil, nil)
	out := make(map[string]float64, len(engineScores))
	for ft, v := range engineScores {
		out[string(ft)] = scaleTo100(v)
	}
	// Map engine factor names to the keys ScoringScreener expects.
	if v, ok := out[string(portfolio.FactorMomentum)]; ok {
		out["momentum"] = v
	}
	if v, ok := out[string(portfolio.FactorValue)]; ok {
		out["value"] = v
	}
	if v, ok := out[string(portfolio.FactorQuality)]; ok {
		out["quality"] = v
	}
	if v, ok := out[string(portfolio.FactorInstSent)]; ok {
		out["foreign_flow"] = v
		out["institutional_sentiment"] = v
	}
	if v, ok := out[string(portfolio.FactorLiquidity)]; ok {
		out["liquidity"] = v
	}
	return out
}

func scaleTo100(v float64) float64 {
	return (v + 1.0) * 50.0
}
