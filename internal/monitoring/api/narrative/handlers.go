package narrative

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

type Handlers struct {
	Svc *service.NarrativeService
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/narrative/events", h.HandleNarrativeEvents)
	mux.HandleFunc("/api/narrative/chains", h.HandleNarrativeChains)
	mux.HandleFunc("/api/narrative/models", h.HandleNarrativeModels)
	mux.HandleFunc("/api/narrative/templates", h.HandleNarrativeTemplates)
	mux.HandleFunc("/api/narrative/seasonal", h.HandleSeasonalAnalysis)
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

func (h *Handlers) HandleNarrativeEvents(w http.ResponseWriter, r *http.Request) {
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
	events := h.Svc.DetectEvents(data)
	shared.WriteJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (h *Handlers) HandleNarrativeChains(w http.ResponseWriter, r *http.Request) {
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
	chains := h.Svc.MatchChains(events)
	shared.WriteJSON(w, http.StatusOK, map[string]any{"chains": chains})
}

func (h *Handlers) HandleNarrativeModels(w http.ResponseWriter, r *http.Request) {
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
	models := h.Svc.GetActiveModels(themes)
	shared.WriteJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (h *Handlers) HandleNarrativeTemplates(w http.ResponseWriter, r *http.Request) {
	templates := h.Svc.GetTemplates()
	shared.WriteJSON(w, http.StatusOK, map[string]any{"templates": templates})
}

func (h *Handlers) HandleSeasonalAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	now := time.Now()
	month := now.Month()

	shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"month": month.String(),
		"note":  "seasonal patterns are embedded in narrative engine",
	})
}
