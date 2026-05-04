package industry

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	Svc *service.IndustryService
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/industry-classification", h.HandleIndustryClassification)
	mux.HandleFunc("/api/dashboard/industry-seasonality", h.HandleIndustrySeasonality)
	mux.HandleFunc("/api/dashboard/industry-seasonality-calendar", h.HandleIndustrySeasonalityCalendar)
	mux.HandleFunc("/api/dashboard/industry-cycle", h.HandleIndustryCycle)
	mux.HandleFunc("/api/dashboard/industry-linkage", h.HandleIndustryLinkage)
	mux.HandleFunc("/api/dashboard/industry-risk", h.HandleIndustryRisk)
	mux.HandleFunc("/api/dashboard/industry-overview", h.HandleIndustryOverview)
	mux.HandleFunc("/api/dashboard/industry-shock-simulation", h.HandleShockSimulation)
	mux.HandleFunc("/api/dashboard/industry-graph", h.HandleIndustryGraph)

	mux.HandleFunc("/api/industry/classification", h.HandleIndustryClassification)
	mux.HandleFunc("/api/industry/seasonality", h.HandleIndustrySeasonality)
	mux.HandleFunc("/api/industry/seasonality/calendar", h.HandleIndustrySeasonalityCalendar)
	mux.HandleFunc("/api/industry/cycle", h.HandleIndustryCycle)
	mux.HandleFunc("/api/industry/linkage", h.HandleIndustryLinkage)
	mux.HandleFunc("/api/industry/risk", h.HandleIndustryRisk)
	mux.HandleFunc("/api/industry/overview", h.HandleIndustryOverview)
	mux.HandleFunc("/api/industry/detail", h.HandleIndustryDetail)
	mux.HandleFunc("/api/industry/shock-simulation", h.HandleShockSimulation)
	mux.HandleFunc("/api/industry/graph", h.HandleIndustryGraph)
}

func (h *Handlers) HandleIndustryClassification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	result := h.Svc.GetClassificationTree()

	shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"industries": result,
		"count":      len(result),
	})
}

func (h *Handlers) HandleIndustrySeasonality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")
	now := time.Now()

	active, historical, adjustment := h.Svc.GetSeasonalPatterns(industryID, now)

	var activePatterns []map[string]interface{}
	for _, p := range active {
		activePatterns = append(activePatterns, map[string]interface{}{
			"id":                  p.ID,
			"name":                p.Name,
			"description":         p.Description,
			"start_month":         p.StartMonth,
			"start_day":           p.StartDay,
			"end_month":           p.EndMonth,
			"end_day":             p.EndDay,
			"historical_accuracy": p.HistoricalAccuracy,
			"typical_return":      p.TypicalReturn,
			"affected_industries": p.AffectedIndustries,
		})
	}

	var historicalPatterns []map[string]interface{}
	for _, p := range historical {
		historicalPatterns = append(historicalPatterns, map[string]interface{}{
			"id":                  p.ID,
			"name":                p.Name,
			"name_en":             p.NameEN,
			"description":         p.Description,
			"start_month":         p.StartMonth,
			"start_day":           p.StartDay,
			"end_month":           p.EndMonth,
			"end_day":             p.EndDay,
			"historical_accuracy": p.HistoricalAccuracy,
			"typical_return":      p.TypicalReturn,
			"adjustment_factor":   p.AdjustmentFactor,
			"favored_industries":  p.FavoredIndustries,
			"avoided_industries":  p.AvoidedIndustries,
			"impact":              p.Impact,
		})
	}

	shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"current_date":        now.Format("2006-01-02"),
		"active_patterns":     activePatterns,
		"pattern_count":       len(activePatterns),
		"adjustment":          adjustment,
		"all_patterns":        historicalPatterns,
		"total_pattern_count": len(historicalPatterns),
	})
}

func (h *Handlers) HandleIndustrySeasonalityCalendar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")
	now := time.Now()
	months := h.Svc.GetSeasonalCalendar(industryID, now.Year())

	shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"year":     now.Year(),
		"industry": industryID,
		"months":   months,
	})
}

func (h *Handlers) HandleIndustryCycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")

	positions, ok := h.Svc.GetCyclePositions(industryID)
	if !ok {
		shared.WriteJSONError(w, http.StatusNotFound, "industry not found")
		return
	}

	if industryID == "" {
		var allPositions []map[string]interface{}
		for _, pos := range positions {
			allPositions = append(allPositions, map[string]interface{}{
				"industry":        pos.Industry,
				"name":            pos.Name,
				"business_cycle":  pos.BusinessCycle,
				"inventory_cycle": pos.InventoryCycle,
				"capex_cycle":     pos.CapexCycle,
				"confidence":      pos.Confidence,
				"updated_at":      pos.UpdatedAt,
				"is_favorable":    pos.IsFavorable,
				"phase_score":     pos.PhaseScore,
				"trend":           pos.Trend,
			})
		}
		shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"industries": allPositions,
			"count":      len(allPositions),
		})
		return
	}

	pos := positions[0]
	shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"industry":        pos.Industry,
		"business_cycle":  pos.BusinessCycle,
		"inventory_cycle": pos.InventoryCycle,
		"capex_cycle":     pos.CapexCycle,
		"confidence":      pos.Confidence,
		"updated_at":      pos.UpdatedAt,
		"is_favorable":    pos.IsFavorable,
		"phase_score":     pos.PhaseScore,
		"trend":           pos.Trend,
	})
}

func (h *Handlers) HandleIndustryLinkage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")
	if industryID == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "industry parameter required")
		return
	}

	info, err := h.Svc.GetLinkageInfo(industryID)
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"industry":      info.Industry,
		"upstream":      info.Upstream,
		"downstream":    info.Downstream,
		"correlations":  info.Correlations,
		"linkage_score": info.LinkageScore,
	})
}

func (h *Handlers) HandleIndustryRisk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	symbol := r.URL.Query().Get("symbol")
	industryID := r.URL.Query().Get("industry")
	if symbol == "" && industryID == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "symbol or industry parameter required")
		return
	}

	info := h.Svc.GetRiskInfo(symbol, industryID)

	shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"symbol":       info.Symbol,
		"industry":     info.Industry,
		"risk_count":   info.RiskCount,
		"risks":        info.Risks,
		"highest_risk": info.HighestRisk,
	})
}

func (h *Handlers) HandleIndustryOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	now := time.Now()
	overviews := h.Svc.GetIndustryOverview(now)

	var industries []map[string]interface{}
	for _, o := range overviews {
		industries = append(industries, map[string]interface{}{
			"id":                o.ID,
			"name":              o.Name,
			"cycle_phase":       o.CyclePhase,
			"inventory_cycle":   o.InventoryCycle,
			"capex_cycle":       o.CapexCycle,
			"cycle_confidence":  o.CycleConfidence,
			"is_favorable":      o.IsFavorable,
			"seasonal_patterns": o.SeasonalPatterns,
			"linkage_score":     o.LinkageScore,
		})
	}

	shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"industries": industries,
		"count":      len(industries),
		"updated_at": now,
	})
}

func (h *Handlers) HandleIndustryDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")
	if industryID == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "industry parameter required")
		return
	}

	now := time.Now()
	detail, err := h.Svc.GetIndustryDetail(industryID, now)
	if err != nil {
		shared.WriteJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	shared.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handlers) HandleShockSimulation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		SourceIndustry string  `json:"source_industry"`
		ShockMagnitude float64 `json:"shock_magnitude"`
		MaxDepth       int     `json:"max_depth"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.SourceIndustry == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "source_industry required")
		return
	}

	if req.MaxDepth <= 0 {
		req.MaxDepth = 3
	}

	impacts := h.Svc.PropagateShock(req.SourceIndustry, req.ShockMagnitude, req.MaxDepth)

	var impactList []map[string]interface{}
	for _, impact := range impacts {
		impactList = append(impactList, map[string]interface{}{
			"industry": impact.Industry,
			"impact":   impact.Impact,
		})
	}

	shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"source":       req.SourceIndustry,
		"shock":        req.ShockMagnitude,
		"max_depth":    req.MaxDepth,
		"impacts":      impactList,
		"impact_count": len(impactList),
	})
}

func (h *Handlers) HandleIndustryGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	nodes, edges := h.Svc.GetIndustryGraph()

	var nodeList []map[string]interface{}
	for _, n := range nodes {
		nodeList = append(nodeList, map[string]interface{}{
			"id":                  n.ID,
			"systemic_importance": n.SystemicImportance,
			"upstream_count":      n.UpstreamCount,
			"downstream_count":    n.DownstreamCount,
		})
	}

	var edgeList []map[string]interface{}
	for _, e := range edges {
		edgeList = append(edgeList, map[string]interface{}{
			"source":      e.Source,
			"target":      e.Target,
			"correlation": e.Correlation,
			"strength":    e.Strength,
		})
	}

	shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"nodes": nodeList,
		"edges": edgeList,
	})
}
