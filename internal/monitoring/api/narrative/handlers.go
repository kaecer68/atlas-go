package narrative

import (
	"fmt"
	"net/http"
	"time"

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
	mux.Handle("GET /api/narrative/stress-index/current", shared.Get(h.HandleStressIndexCurrent))
	mux.Handle("GET /api/narrative/stress-index/history", shared.Get(h.HandleStressIndexHistory))
	mux.Handle("GET /api/narrative/stress-index/thresholds", shared.Get(h.HandleStressIndexThresholds))
}

func parseFloatQuery(r *http.Request, key string, defaultVal float64) float64 {
	if v := r.URL.Query().Get(key); v != "" {
		var f float64
		if _, err := fmt.Sscanf(v, "%f", &f); err == nil {
			return f
		}
	}
	return defaultVal
}

func (h *Handlers) HandleNarrativeEvents(r *http.Request) (int, any) {
	data := narrative.MarketNarrativeData{
		US10YChangeBps:                parseFloatQuery(r, "us10y_change_bps", 15),
		DXYChangePct:                  parseFloatQuery(r, "dxy_change_pct", 2.0),
		VIXLevel:                      parseFloatQuery(r, "vix_level", 30),
		USD_TWD_ChangePct:             parseFloatQuery(r, "usd_twd_change_pct", 0),
		OilChangePct:                  parseFloatQuery(r, "oil_change_pct", 6.0),
		GoldChangePct:                 parseFloatQuery(r, "gold_change_pct", 2.5),
		JPY_ChangePct:                 parseFloatQuery(r, "jpy_change_pct", 3.0),
		AICapexSentiment:              parseFloatQuery(r, "ai_capex_sentiment", 0.8),
		GeopoliticalGPR:               parseFloatQuery(r, "geopolitical_gpr", 160),
		RetailInstitutionalDivergence: parseFloatQuery(r, "retail_divergence", 0),
		MarginZScore:                  parseFloatQuery(r, "margin_zscore", 0),
	}
	return http.StatusOK, map[string]any{"events": h.Svc.DetectEvents(data)}
}

func (h *Handlers) HandleNarrativeChains(r *http.Request) (int, any) {
	data := narrative.MarketNarrativeData{
		US10YChangeBps:    parseFloatQuery(r, "us10y_change_bps", 15),
		DXYChangePct:      parseFloatQuery(r, "dxy_change_pct", 2.0),
		VIXLevel:          parseFloatQuery(r, "vix_level", 30),
		USD_TWD_ChangePct: parseFloatQuery(r, "usd_twd_change_pct", 0),
		OilChangePct:      parseFloatQuery(r, "oil_change_pct", 6.0),
		GoldChangePct:     parseFloatQuery(r, "gold_change_pct", 2.5),
		JPY_ChangePct:     parseFloatQuery(r, "jpy_change_pct", 3.0),
		AICapexSentiment:  parseFloatQuery(r, "ai_capex_sentiment", 0.8),
		GeopoliticalGPR:   parseFloatQuery(r, "geopolitical_gpr", 160),
	}
	events := h.Svc.DetectEvents(data)
	return http.StatusOK, map[string]any{"chains": h.Svc.MatchChains(events)}
}

func (h *Handlers) HandleNarrativeModels(r *http.Request) (int, any) {
	data := narrative.MarketNarrativeData{
		US10YChangeBps:    parseFloatQuery(r, "us10y_change_bps", 15),
		DXYChangePct:      parseFloatQuery(r, "dxy_change_pct", 2.0),
		VIXLevel:          parseFloatQuery(r, "vix_level", 30),
		USD_TWD_ChangePct: parseFloatQuery(r, "usd_twd_change_pct", 0),
		OilChangePct:      parseFloatQuery(r, "oil_change_pct", 6.0),
		GoldChangePct:     parseFloatQuery(r, "gold_change_pct", 2.5),
		JPY_ChangePct:     parseFloatQuery(r, "jpy_change_pct", 3.0),
		AICapexSentiment:  parseFloatQuery(r, "ai_capex_sentiment", 0.8),
		GeopoliticalGPR:   parseFloatQuery(r, "geopolitical_gpr", 160),
	}
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
	Theme               string  `json:"theme"`
	HistoricalAvgReturn float64 `json:"historical_avg_return"`
	CurrentReturn       float64 `json:"current_return"`
	ExpectationGap      float64 `json:"expectation_gap"`
	AlreadyPricedIn     bool    `json:"already_priced_in"`
}

func (h *Handlers) HandleSeasonalAnalysis(r *http.Request) (int, any) {
	now := time.Now()

	currentReturn := 0.0
	calc := marketdata.NewTAIEXReturnCalculator()
	if ret, err := calc.Get1MonthReturn(r.Context()); err == nil {
		currentReturn = ret
	}

	if h.IndustryService != nil {
		active, historical, adjustment := h.IndustryService.GetSeasonalPatterns("", now)
		expectations := make([]SeasonalExpectation, len(active))
		for i, p := range active {
			gap := currentReturn - p.TypicalReturn
			expectations[i] = SeasonalExpectation{
				Theme:               p.Name,
				HistoricalAvgReturn: p.TypicalReturn,
				CurrentReturn:       currentReturn,
				ExpectationGap:      gap,
				AlreadyPricedIn:     currentReturn > p.TypicalReturn,
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

func (h *Handlers) HandleStressIndexCurrent(r *http.Request) (int, any) {
	idx := h.Svc.GetCurrentStressIndex()
	return http.StatusOK, map[string]any{
		"score":      idx.Score,
		"regime":     idx.Regime,
		"components": idx.Components,
		"timestamp":  idx.Timestamp,
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
