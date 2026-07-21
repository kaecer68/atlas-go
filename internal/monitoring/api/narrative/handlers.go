package narrative

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

type Handlers struct {
	Svc             *service.NarrativeService
	IndustryService *service.IndustryService
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/narrative/events", shared.Get(h.HandleNarrativeEvents))
	mux.Handle("GET /api/narrative/chains", shared.Get(h.HandleNarrativeChains))
	mux.Handle("GET /api/narrative/models", shared.Get(h.HandleNarrativeModels))
	mux.Handle("GET /api/narrative/templates", shared.Get(h.HandleNarrativeTemplates))
	mux.Handle("GET /api/narrative/seasonal", shared.Get(h.HandleSeasonalAnalysis))
	mux.Handle("GET /api/narrative/bundle", shared.Get(h.HandleNarrativeBundle))
	// Deprecated: covered by /api/taiwan/stress-index. See docs/operations/tier-boundary.md.
	mux.Handle("GET /api/narrative/stress-index/current", shared.Get(h.HandleStressIndexCurrent))
	// Deprecated: covered by /api/taiwan/stress-index. See docs/operations/tier-boundary.md.
	mux.Handle("GET /api/narrative/stress-index/history", shared.Get(h.HandleStressIndexHistory))
	// Deprecated: covered by /api/taiwan/stress-index. See docs/operations/tier-boundary.md.
	mux.Handle("GET /api/narrative/stress-index/thresholds", shared.Get(h.HandleStressIndexThresholds))
	mux.Handle("GET /api/geopolitical/history", shared.Get(h.HandleGeopoliticalHistory))
	mux.Handle("GET /api/narrative/regime-mapping", shared.Get(h.HandleRegimeMapping))
}

func parseFloatQuery(r *http.Request, key string) float64 {
	if v := r.URL.Query().Get(key); v != "" {
		var f float64
		if _, err := fmt.Sscanf(v, "%f", &f); err == nil {
			return f
		}
	}
	return 0
}

// buildNarrativeData fetches real-time macro snapshot data and overlays
// query-param overrides.  Fields not available in the snapshot
// (RetailInstitutionalDivergence, MarginZScore, EarningsSurprisePct) remain
// at their query-param values (defaulting to 0).  GeopoliticalGPR is fetched
// from the geoProvider inside BuildMarketNarrativeData and can be overridden
// by query param.
func (h *Handlers) buildNarrativeData(ctx context.Context, r *http.Request) narrative.MarketNarrativeData {
	data := narrative.MarketNarrativeData{
		GeopoliticalGPR:               parseFloatQuery(r, "geopolitical_gpr"),
		RetailInstitutionalDivergence: parseFloatQuery(r, "retail_divergence"),
		MarginZScore:                  parseFloatQuery(r, "margin_zscore"),
		EarningsSurprisePct:           parseFloatQuery(r, "earnings_surprise_pct"),
	}

	if snapData, err := h.Svc.BuildMarketNarrativeData(ctx); err == nil {
		data.US10YChangeBps = snapData.US10YChangeBps
		data.DXYChangePct = snapData.DXYChangePct
		data.VIXLevel = snapData.VIXLevel
		data.USD_TWD_ChangePct = snapData.USD_TWD_ChangePct
		data.OilChangePct = snapData.OilChangePct
		data.GoldChangePct = snapData.GoldChangePct
		data.GoldLevel = snapData.GoldLevel
		data.JPY_ChangePct = snapData.JPY_ChangePct
		data.JPYLevel = snapData.JPYLevel
		data.AICapexSentiment = snapData.AICapexSentiment
		// Overlay geoProvider result with query-param override (manual override wins).
		if data.GeopoliticalGPR == 0 {
			data.GeopoliticalGPR = snapData.GeopoliticalGPR
		}
	} else {
		logging.Warn("narrative_handlers", "snapshot_fallback", logging.Err(err))
		// Graceful degradation: use query-param defaults for missing fields.
		data.US10YChangeBps = parseFloatQuery(r, "us10y_change_bps")
		data.DXYChangePct = parseFloatQuery(r, "dxy_change_pct")
		data.VIXLevel = parseFloatQuery(r, "vix_level")
		data.USD_TWD_ChangePct = parseFloatQuery(r, "usd_twd_change_pct")
		data.OilChangePct = parseFloatQuery(r, "oil_change_pct")
		data.GoldChangePct = parseFloatQuery(r, "gold_change_pct")
		data.GoldLevel = parseFloatQuery(r, "gold_level")
		data.JPY_ChangePct = parseFloatQuery(r, "jpy_change_pct")
		data.JPYLevel = parseFloatQuery(r, "jpy_level")
		data.AICapexSentiment = parseFloatQuery(r, "ai_capex_sentiment")
	}
	return data
}

func (h *Handlers) HandleNarrativeEvents(r *http.Request) (int, any) {
	data := h.buildNarrativeData(r.Context(), r)
	return http.StatusOK, map[string]any{"events": h.Svc.DetectEvents(data)}
}

func (h *Handlers) HandleNarrativeChains(r *http.Request) (int, any) {
	data := h.buildNarrativeData(r.Context(), r)
	events := h.Svc.DetectEvents(data)
	return http.StatusOK, map[string]any{"chains": h.Svc.MatchChains(events)}
}

func (h *Handlers) HandleNarrativeModels(r *http.Request) (int, any) {
	data := h.buildNarrativeData(r.Context(), r)
	events := h.Svc.DetectEvents(data)
	themes := make([]string, len(events))
	for i, e := range events {
		themes[i] = e.Theme
	}
	return http.StatusOK, map[string]any{"models": h.Svc.GetActiveModels(themes)}
}

func (h *Handlers) HandleNarrativeTemplates(r *http.Request) (int, any) {
	return http.StatusOK, map[string]any{"templates": h.Svc.GetTemplates()}
}

type SeasonalExpectation struct {
	Theme               string   `json:"theme"`
	HistoricalAvgReturn float64  `json:"historical_avg_return"`
	CurrentReturn       *float64 `json:"current_return"`
	ExpectationGap      float64  `json:"expectation_gap"`
	AlreadyPricedIn     bool     `json:"already_priced_in"`
}

// approxPatternDays returns the approximate duration in days for a seasonal
// pattern, using its start/end month/day fields.  This lets us match the
// current TAIEX return window to the pattern's historical window.
func approxPatternDays(p service.SeasonalPattern) int {
	start := p.StartMonth*30 + p.StartDay
	end := p.EndMonth*30 + p.EndDay
	days := end - start
	if days <= 0 {
		days += 365 // cross-year wrap
	}
	return days
}

// getPatternReturn fetches the TAIEX return for a duration matching the
// seasonal pattern's typical window. Returns nil when historical quote data is
// insufficient so the caller can distinguish "no data" from a genuine 0% return.
func getPatternReturn(ctx context.Context, calc *marketdata.TAIEXReturnCalculator, p service.SeasonalPattern) *float64 {
	days := approxPatternDays(p)
	if days <= 0 {
		days = 30
	}
	if ret, err := calc.GetNDayReturn(ctx, days); err == nil {
		return &ret
	}
	return nil
}

func (h *Handlers) HandleSeasonalAnalysis(r *http.Request) (int, any) {
	now := time.Now()

	if h.IndustryService != nil {
		active, historical, adjustment := h.IndustryService.GetSeasonalPatterns("", now)
		calc := marketdata.NewTAIEXReturnCalculator()
		expectations := make([]SeasonalExpectation, len(active))
		for i, p := range active {
			currentReturn := getPatternReturn(r.Context(), calc, p)
			var gap float64
			pricedIn := false
			if currentReturn != nil {
				gap = *currentReturn - p.AvgMarketReturn
				pricedIn = *currentReturn > p.AvgMarketReturn
			}
			expectations[i] = SeasonalExpectation{
				Theme:               p.Name,
				HistoricalAvgReturn: p.AvgMarketReturn,
				CurrentReturn:       currentReturn,
				ExpectationGap:      gap,
				AlreadyPricedIn:     pricedIn,
			}
		}
		return http.StatusOK, map[string]any{
			"month":               now.Month().String(),
			"active_patterns":     active,
			"all_patterns":        historical,
			"combined_adjustment": adjustment,
			"expectations":        expectations,
		}
	}

	return http.StatusOK, map[string]any{
		"month": now.Month().String(),
		"note":  "seasonal patterns are embedded in narrative engine",
	}
}

// HandleNarrativeBundle aggregates events, chains, models, templates, and
// seasonal analysis into a single response. Events are computed first (needed
// by chains/models), then the dependent calls run in parallel via errgroup.
func (h *Handlers) HandleNarrativeBundle(r *http.Request) (int, any) {
	data := h.buildNarrativeData(r.Context(), r)

	// Compute events first — needed by chains and models.
	events := h.Svc.DetectEvents(data)
	themes := make([]string, len(events))
	for i, e := range events {
		themes[i] = e.Theme
	}

	var (
		chains    []narrative.CausalChain
		models    []narrative.InvestmentModel
		templates []narrative.CausalTemplate
		seasonal  map[string]any
	)

	g, ctx := errgroup.WithContext(r.Context())

	// Chains: depends on events.
	g.Go(func() error {
		_ = ctx
		chains = h.Svc.MatchChains(events)
		return nil
	})

	// Models: depends on themes derived from events.
	g.Go(func() error {
		_ = ctx
		models = h.Svc.GetActiveModels(themes)
		return nil
	})

	// Templates: no dependency on events.
	g.Go(func() error {
		_ = ctx
		templates = h.Svc.GetTemplates()
		return nil
	})

	// Seasonal: independent computation.
	g.Go(func() error {
		now := time.Now()
		if h.IndustryService != nil {
			active, historical, adjustment := h.IndustryService.GetSeasonalPatterns("", now)
			calc := marketdata.NewTAIEXReturnCalculator()
			expectations := make([]SeasonalExpectation, len(active))
			for i, p := range active {
				currentReturn := getPatternReturn(ctx, calc, p)
				var gap float64
				pricedIn := false
				if currentReturn != nil {
					gap = *currentReturn - p.AvgMarketReturn
					pricedIn = *currentReturn > p.AvgMarketReturn
				}
				expectations[i] = SeasonalExpectation{
					Theme:               p.Name,
					HistoricalAvgReturn: p.AvgMarketReturn,
					CurrentReturn:       currentReturn,
					ExpectationGap:      gap,
					AlreadyPricedIn:     pricedIn,
				}
			}
			seasonal = map[string]any{
				"month":               now.Month().String(),
				"active_patterns":     active,
				"all_patterns":        historical,
				"combined_adjustment": adjustment,
				"expectations":        expectations,
			}
		} else {
			seasonal = map[string]any{
				"month": now.Month().String(),
				"note":  "seasonal patterns are embedded in narrative engine",
			}
		}
		return nil
	})

	_ = g.Wait()

	return http.StatusOK, map[string]any{
		"events":    events,
		"chains":    chains,
		"models":    models,
		"templates": templates,
		"seasonal":  seasonal,
	}
}

func (h *Handlers) HandleStressIndexCurrent(r *http.Request) (int, any) {
	idx := h.Svc.GetCurrentStressIndex()
	date := ""
	if idx.Timestamp != 0 {
		date = time.Unix(idx.Timestamp, 0).UTC().Format("2006-01-02")
	}
	return http.StatusOK, map[string]any{
		"score":      idx.Score,
		"regime":     idx.Regime,
		"components": idx.Components,
		"timestamp":  idx.Timestamp,
		"date":       date,
		"source":     "taiwan_calculator",
	}
}

func (h *Handlers) HandleStressIndexHistory(r *http.Request) (int, any) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := fmt.Sscanf(d, "%d", &days); err != nil || n != 1 {
			days = 30
		}
	}
	history := h.Svc.GetStressIndexHistory(days)
	return http.StatusOK, map[string]any{"history": history}
}

func (h *Handlers) HandleStressIndexThresholds(r *http.Request) (int, any) {
	thresholds := h.Svc.GetStressIndexThresholds()
	return http.StatusOK, map[string]any{
		"crisis": thresholds.Crisis,
		"high":   thresholds.High,
		"alert":  thresholds.Alert,
	}
}

// HandleGeopoliticalHistory serves GET /api/geopolitical/history.
func (h *Handlers) HandleGeopoliticalHistory(r *http.Request) (int, any) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := fmt.Sscanf(d, "%d", &days); err != nil || n != 1 {
			days = 30
		}
	}
	history := h.Svc.GetGeopoliticalHistory(days)
	return http.StatusOK, map[string]any{"history": history}
}

// HandleRegimeMapping serves GET /api/narrative/regime-mapping. It exposes the
// cross-walk between the two regime vocabularies that have ended up sharing the
// stress_index_history.regime and regime_history.regime SQLite columns
// (TaiwanStressCalculator "low/alert/high/crisis" vs Janus engine
// "RISK_ON/RISK_OFF/NEUTRAL/TRANSITIONAL"). See internal/narrative/regime_mapping.go
// and docs/manifests/2026-07-21-stress-history-and-regime-gaps.md (C03).
//
// The mapping table is bidirectional: each entry maps one stress-vocabulary
// token to a regime-vocabulary token and vice versa. Consumers can use it as
// a lookup without needing to call NormalizeRegime from Go.
func (h *Handlers) HandleRegimeMapping(r *http.Request) (int, any) {
	stressToRegime := make(map[string]string, len(narrative.StressVocabulary))
	for _, k := range narrative.StressVocabulary {
		if v, ok := narrative.RegimeVocabularyMapping[k]; ok {
			stressToRegime[k] = v
		}
	}
	regimeToStress := make(map[string]string, len(narrative.RegimeVocabulary))
	for _, k := range narrative.RegimeVocabulary {
		if v, ok := narrative.RegimeVocabularyMapping[k]; ok {
			regimeToStress[k] = v
		}
	}
	return http.StatusOK, map[string]any{
		"stress_to_regime":      stressToRegime,
		"regime_to_stress":      regimeToStress,
		"stress_vocabulary":     narrative.StressVocabulary,
		"regime_vocabulary":     narrative.RegimeVocabulary,
		"warning":               "the two systems measure different things; the mapping is approximate",
		"see_also_source_field": "TaiwanStressIndex.Source and RegimeRow.Source disambiguate origin per row",
	}
}
