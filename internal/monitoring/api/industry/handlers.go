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
	mux.Handle("GET /api/dashboard/industry-classification", shared.Get(h.HandleIndustryClassification))
	mux.Handle("GET /api/dashboard/industry-seasonality", shared.Get(h.HandleIndustrySeasonality))
	mux.Handle("GET /api/dashboard/industry-seasonality-calendar", shared.Get(h.HandleIndustrySeasonalityCalendar))
	mux.Handle("GET /api/dashboard/industry-cycle", shared.Get(h.HandleIndustryCycle))
	mux.Handle("GET /api/dashboard/industry-linkage", shared.Get(h.HandleIndustryLinkage))
	mux.Handle("GET /api/dashboard/industry-risk", shared.Get(h.HandleIndustryRisk))
	mux.Handle("GET /api/dashboard/industry-overview", shared.Get(h.HandleIndustryOverview))
	mux.Handle("POST /api/dashboard/industry-shock-simulation", shared.Post(h.HandleShockSimulation))
	mux.Handle("GET /api/dashboard/industry-graph", shared.Get(h.HandleIndustryGraph))
	mux.Handle("GET /api/dashboard/industry-detail", shared.Get(h.HandleIndustryDetail))
}

func (h *Handlers) HandleIndustryClassification(r *http.Request) (int, any) {
	result := h.Svc.GetClassificationTree()

	return http.StatusOK, map[string]interface{}{
		"industries": result,
		"count":      len(result),
	}
}

func (h *Handlers) HandleIndustrySeasonality(r *http.Request) (int, any) {
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

	return http.StatusOK, map[string]interface{}{
		"current_date":        now.Format("2006-01-02"),
		"active_patterns":     activePatterns,
		"pattern_count":       len(activePatterns),
		"adjustment":          adjustment,
		"all_patterns":        historicalPatterns,
		"total_pattern_count": len(historicalPatterns),
	}
}

func (h *Handlers) HandleIndustrySeasonalityCalendar(r *http.Request) (int, any) {
	industryID := r.URL.Query().Get("industry")
	now := time.Now()
	months := h.Svc.GetSeasonalCalendar(industryID, now.Year())

	return http.StatusOK, map[string]interface{}{
		"year":     now.Year(),
		"industry": industryID,
		"months":   months,
	}
}

func (h *Handlers) HandleIndustryCycle(r *http.Request) (int, any) {
	industryID := r.URL.Query().Get("industry")

	positions, ok := h.Svc.GetCyclePositions(industryID)
	if !ok {
		return http.StatusNotFound, map[string]string{"error": "industry not found"}
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
		return http.StatusOK, map[string]interface{}{
			"industries": allPositions,
			"count":      len(allPositions),
		}
	}

	pos := positions[0]
	return http.StatusOK, map[string]interface{}{
		"industry":        pos.Industry,
		"business_cycle":  pos.BusinessCycle,
		"inventory_cycle": pos.InventoryCycle,
		"capex_cycle":     pos.CapexCycle,
		"confidence":      pos.Confidence,
		"updated_at":      pos.UpdatedAt,
		"is_favorable":    pos.IsFavorable,
		"phase_score":     pos.PhaseScore,
		"trend":           pos.Trend,
	}
}

func (h *Handlers) HandleIndustryLinkage(r *http.Request) (int, any) {
	industryID := r.URL.Query().Get("industry")
	if industryID == "" {
		return http.StatusBadRequest, map[string]string{"error": "industry parameter required"}
	}

	info, err := h.Svc.GetLinkageInfo(industryID)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}

	return http.StatusOK, map[string]interface{}{
		"industry":      info.Industry,
		"upstream":      info.Upstream,
		"downstream":    info.Downstream,
		"correlations":  info.Correlations,
		"linkage_score": info.LinkageScore,
	}
}

func (h *Handlers) HandleIndustryRisk(r *http.Request) (int, any) {
	symbol := r.URL.Query().Get("symbol")
	industryID := r.URL.Query().Get("industry")
	if symbol == "" && industryID == "" {
		return http.StatusBadRequest, map[string]string{"error": "symbol or industry parameter required"}
	}

	info := h.Svc.GetRiskInfo(symbol, industryID)

	return http.StatusOK, map[string]interface{}{
		"symbol":       info.Symbol,
		"industry":     info.Industry,
		"risk_count":   info.RiskCount,
		"risks":        info.Risks,
		"highest_risk": info.HighestRisk,
	}
}

func (h *Handlers) HandleIndustryOverview(r *http.Request) (int, any) {
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

	return http.StatusOK, map[string]interface{}{
		"industries": industries,
		"count":      len(industries),
		"updated_at": now,
	}
}

func (h *Handlers) HandleIndustryDetail(r *http.Request) (int, any) {
	industryID := r.URL.Query().Get("industry")
	if industryID == "" {
		return http.StatusBadRequest, map[string]string{"error": "industry parameter required"}
	}

	now := time.Now()
	detail, err := h.Svc.GetIndustryDetail(industryID, now)
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": err.Error()}
	}

	return http.StatusOK, detail
}

func (h *Handlers) HandleShockSimulation(r *http.Request) (int, any) {
	var req struct {
		SourceIndustry string  `json:"source_industry"`
		ShockMagnitude float64 `json:"shock_magnitude"`
		MaxDepth       int     `json:"max_depth"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()}
	}

	if req.SourceIndustry == "" {
		return http.StatusBadRequest, map[string]string{"error": "source_industry required"}
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

	return http.StatusOK, map[string]interface{}{
		"source":       req.SourceIndustry,
		"shock":        req.ShockMagnitude,
		"max_depth":    req.MaxDepth,
		"impacts":      impactList,
		"impact_count": len(impactList),
	}
}

func (h *Handlers) HandleIndustryGraph(r *http.Request) (int, any) {
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

	return http.StatusOK, map[string]interface{}{
		"nodes": nodeList,
		"edges": edgeList,
	}
}
