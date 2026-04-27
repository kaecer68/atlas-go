package narrative

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

type Handlers struct {
	WorkDir         string
	NarrativeEngine *narrative.NarrativeEngine
	ReportGenerator *narrative.ReportGenerator
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/narrative/events", h.HandleNarrativeEvents)
	mux.HandleFunc("/api/narrative/chains", h.HandleNarrativeChains)
	mux.HandleFunc("/api/narrative/models", h.HandleNarrativeModels)
	mux.HandleFunc("/api/narrative/templates", h.HandleNarrativeTemplates)
	mux.HandleFunc("/api/narrative/seasonal", h.HandleSeasonalAnalysis)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
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
	events := h.NarrativeEngine.DetectEvents(data)
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
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
	events := h.NarrativeEngine.DetectEvents(data)
	chains := h.NarrativeEngine.MatchChains(events)
	writeJSON(w, http.StatusOK, map[string]any{"chains": chains})
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
	events := h.NarrativeEngine.DetectEvents(data)
	themes := make([]string, len(events))
	for i, e := range events {
		themes[i] = e.Theme
	}

	replayPath := filepath.Join(h.WorkDir, "data/replay/tw_extended_90days.csv")
	if err := h.NarrativeEngine.EvaluateModels(replayPath); err != nil {
		log.Printf("[DashboardAPI] EvaluateModels warning: %v", err)
	}

	models := h.NarrativeEngine.ActiveModels(themes)
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (h *Handlers) HandleNarrativeTemplates(w http.ResponseWriter, r *http.Request) {
	kb := narrative.NewKnowledgeBase()
	templates := kb.ListTemplates()
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates})
}

func (h *Handlers) HandleSeasonalAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	now := time.Now()
	month := now.Month()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"month": month.String(),
		"note":  "seasonal patterns are embedded in narrative engine",
	})
}
