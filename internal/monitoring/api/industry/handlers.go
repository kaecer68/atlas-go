package industry

import (
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

type Handlers struct {
	Classifier     *industry.ClassificationTree
	SeasonalEngine *industry.SeasonalEngine
	CycleTracker   *industry.CycleTracker
	LinkageAnalyzer *industry.LinkageAnalyzer
	RiskMonitor    *industry.RiskMonitor
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
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (h *Handlers) HandleIndustryClassification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	tree := h.Classifier
	segments := tree.GetAllSegments()

	var result []map[string]interface{}
	for _, seg := range segments {
		if seg.ParentID == "" {
			children := tree.GetChildren(seg.ID)
			var childList []map[string]interface{}
			for _, child := range children {
				grandchildren := tree.GetChildren(child.ID)
				var grandchildList []map[string]interface{}
				for _, gc := range grandchildren {
					grandchildList = append(grandchildList, map[string]interface{}{
						"id":          gc.ID,
						"name":        gc.Name,
						"weight":      gc.Weight,
						"description": gc.Description,
					})
				}
				childList = append(childList, map[string]interface{}{
					"id":          child.ID,
					"name":        child.Name,
					"weight":      child.Weight,
					"description": child.Description,
					"children":    grandchildList,
				})
			}
			result = append(result, map[string]interface{}{
				"id":          seg.ID,
				"name":        seg.Name,
				"weight":      seg.Weight,
				"description": seg.Description,
				"children":    childList,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"industries": result,
		"count":      len(result),
	})
}

func (h *Handlers) HandleIndustrySeasonality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")
	now := time.Now()

	patterns := h.SeasonalEngine.DetectCurrentPatterns(now)
	var activePatterns []map[string]interface{}
	for _, p := range patterns {
		if industryID == "" || p.IsRelevantForIndustry(industryID) {
			activePatterns = append(activePatterns, map[string]interface{}{
				"id":                  p.ID,
				"name":                p.Name,
				"description":         p.Description,
				"start_month":         p.StartMonth,
				"start_day":           p.StartDay,
				"end_month":           p.EndMonth,
				"end_day":             p.EndDay,
				"historical_accuracy": p.HistoricalAccuracy,
				"typical_return":      p.TypicalReturn(),
				"affected_industries": p.AffectedIndustries,
			})
		}
	}

	var adjustment float64
	if industryID != "" {
		adjustment = h.SeasonalEngine.GetPatternAdjustment(industryID, now)
	}

	allPatterns := h.SeasonalEngine.GetAllPatterns()
	var historicalPatterns []map[string]interface{}
	for _, p := range allPatterns {
		if industryID == "" || p.IsRelevantForIndustry(industryID) {
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
				"typical_return":      p.TypicalReturn(),
				"adjustment_factor":   p.AdjustmentFactor,
				"favored_industries":  p.FavoredIndustries,
				"avoided_industries":  p.AvoidedIndustries,
				"impact": func() string {
					if industryID == "" {
						return ""
					}
					impact, _ := h.SeasonalEngine.GetIndustryImpact(p.ID, industryID)
					return impact
				}(),
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")
	now := time.Now()
	calendar := h.SeasonalEngine.GenerateCalendar(now.Year())

	var months []map[string]interface{}
	for m := 1; m <= 12; m++ {
		monthPatterns := calendar.ByMonth[m]
		var relevantPatterns []map[string]interface{}
		for _, p := range monthPatterns {
			if industryID == "" || p.IsRelevantForIndustry(industryID) {
				relevantPatterns = append(relevantPatterns, map[string]interface{}{
					"id":                  p.ID,
					"name":                p.Name,
					"historical_accuracy": p.HistoricalAccuracy,
					"typical_return":      p.TypicalReturn(),
					"adjustment_factor":   p.AdjustmentFactor,
				})
			}
		}
		months = append(months, map[string]interface{}{
			"month":    m,
			"patterns": relevantPatterns,
			"count":    len(relevantPatterns),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"year":     calendar.Year,
		"industry": industryID,
		"months":   months,
	})
}

func (h *Handlers) HandleIndustryCycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")

	if industryID == "" {
		var allPositions []map[string]interface{}
		for _, seg := range h.Classifier.GetAllSegments() {
			if seg.ParentID != "" {
				continue
			}
			if pos, ok := h.CycleTracker.GetPosition(seg.ID); ok {
				allPositions = append(allPositions, map[string]interface{}{
					"industry":        seg.ID,
					"name":            seg.Name,
					"business_cycle":  pos.BusinessCycle,
					"inventory_cycle": pos.InventoryCycle,
					"capex_cycle":     pos.CapexCycle,
					"confidence":      pos.Confidence,
					"updated_at":      pos.UpdatedAt,
					"is_favorable":    pos.IsFavorable(),
					"phase_score":     pos.GetPhaseScore(),
					"trend":           pos.GetTrend(),
				})
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"industries": allPositions,
			"count":      len(allPositions),
		})
		return
	}

	position, ok := h.CycleTracker.GetPosition(industryID)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "industry not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"industry":        industryID,
		"business_cycle":  position.BusinessCycle,
		"inventory_cycle": position.InventoryCycle,
		"capex_cycle":     position.CapexCycle,
		"confidence":      position.Confidence,
		"updated_at":      position.UpdatedAt,
		"is_favorable":    position.IsFavorable(),
		"phase_score":     position.GetPhaseScore(),
		"trend":           position.GetTrend(),
	})
}

func (h *Handlers) HandleIndustryLinkage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")
	if industryID == "" {
		writeJSONError(w, http.StatusBadRequest, "industry parameter required")
		return
	}

	graph := h.LinkageAnalyzer.GetSupplyChainGraph()
	upstream := graph.GetUpstream(industryID)
	downstream := graph.GetDownstream(industryID)

	correlations := h.LinkageAnalyzer.GetCorrelationMatrix().GetCorrelatedIndustries(industryID, 0.0)
	var correlationList []map[string]interface{}
	for otherIndustry, correlation := range correlations {
		strength := "low"
		if math.Abs(correlation) > 0.7 {
			strength = "high"
		} else if math.Abs(correlation) > 0.4 {
			strength = "medium"
		}
		correlationList = append(correlationList, map[string]interface{}{
			"industry":    otherIndustry,
			"correlation": correlation,
			"strength":    strength,
		})
	}

	score := h.LinkageAnalyzer.CalculateLinkageScore(industryID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"industry":      industryID,
		"upstream":      upstream,
		"downstream":    downstream,
		"correlations":  correlationList,
		"linkage_score": score,
	})
}

func (h *Handlers) HandleIndustryRisk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	symbol := r.URL.Query().Get("symbol")
	industryID := r.URL.Query().Get("industry")
	if symbol == "" && industryID == "" {
		writeJSONError(w, http.StatusBadRequest, "symbol or industry parameter required")
		return
	}

	var risks []industry.RiskEvent
	if symbol == "" && industryID != "" {
		risks = h.RiskMonitor.GetAllRisks("ALL", industryID, 0, 0)
	} else {
		risks = h.RiskMonitor.GetAllRisks(symbol, industryID, 0, 0)
	}
	var riskList []map[string]interface{}
	for _, risk := range risks {
		riskList = append(riskList, map[string]interface{}{
			"id":                 risk.ID,
			"type":               risk.Type,
			"severity":           risk.Severity,
			"description":        risk.Description,
			"impact_estimate":    risk.ImpactEstimate,
			"confidence":         risk.Confidence,
			"detected_at":        risk.DetectedAt,
			"recommended_action": risk.RecommendedAction,
		})
	}

	highest := h.RiskMonitor.GetHighestRisk(risks)
	var highestRisk map[string]interface{}
	if highest != nil {
		highestRisk = map[string]interface{}{
			"id":          highest.ID,
			"type":        highest.Type,
			"severity":    highest.Severity,
			"description": highest.Description,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"symbol":       symbol,
		"industry":     industryID,
		"risk_count":   len(riskList),
		"risks":        riskList,
		"highest_risk": highestRisk,
	})
}

func (h *Handlers) HandleIndustryOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	now := time.Now()
	tree := h.Classifier
	segments := tree.GetAllSegments()

	var industries []map[string]interface{}
	for _, seg := range segments {
		if seg.ParentID != "" {
			continue
		}

		cyclePos, ok := h.CycleTracker.GetPosition(seg.ID)
		if !ok {
			continue
		}

		patterns := h.SeasonalEngine.DetectCurrentPatterns(now)
		var activePatternNames []string
		for _, p := range patterns {
			if p.IsRelevantForIndustry(seg.ID) {
				activePatternNames = append(activePatternNames, p.Name)
			}
		}

		linkageScore := h.LinkageAnalyzer.CalculateLinkageScore(seg.ID)

		industries = append(industries, map[string]interface{}{
			"id":                seg.ID,
			"name":              seg.Name,
			"cycle_phase":       cyclePos.BusinessCycle,
			"inventory_cycle":   cyclePos.InventoryCycle,
			"capex_cycle":       cyclePos.CapexCycle,
			"cycle_confidence":  cyclePos.Confidence,
			"is_favorable":      cyclePos.IsFavorable(),
			"seasonal_patterns": activePatternNames,
			"linkage_score":     linkageScore,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"industries": industries,
		"count":      len(industries),
		"updated_at": now,
	})
}

func (h *Handlers) HandleShockSimulation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		SourceIndustry string  `json:"source_industry"`
		ShockMagnitude float64 `json:"shock_magnitude"`
		MaxDepth       int     `json:"max_depth"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.SourceIndustry == "" {
		writeJSONError(w, http.StatusBadRequest, "source_industry required")
		return
	}

	if req.MaxDepth <= 0 {
		req.MaxDepth = 3
	}

	impacts := h.LinkageAnalyzer.PropagateShock(req.SourceIndustry, req.ShockMagnitude, req.MaxDepth)

	var impactList []map[string]interface{}
	for industry, impact := range impacts {
		impactList = append(impactList, map[string]interface{}{
			"industry": industry,
			"impact":   impact,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"source":       req.SourceIndustry,
		"shock":        req.ShockMagnitude,
		"max_depth":    req.MaxDepth,
		"impacts":      impactList,
		"impact_count": len(impactList),
	})
}

func (h *Handlers) HandleIndustryGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cm := h.LinkageAnalyzer.GetCorrelationMatrix()

	var nodes []map[string]interface{}
	var edges []map[string]interface{}
	nodeSet := make(map[string]bool)

	allCorrelations := cm.GetAllCorrelations()
	for industryA, correlations := range allCorrelations {
		if !nodeSet[industryA] {
			nodeSet[industryA] = true
			score := h.LinkageAnalyzer.CalculateLinkageScore(industryA)
			nodes = append(nodes, map[string]interface{}{
				"id":                  industryA,
				"systemic_importance": score.SystemicImportance,
				"upstream_count":      score.UpstreamCount,
				"downstream_count":    score.DownstreamCount,
			})
		}

		for industryB, correlation := range correlations {
			if industryA >= industryB {
				continue
			}
			if !nodeSet[industryB] {
				nodeSet[industryB] = true
				score := h.LinkageAnalyzer.CalculateLinkageScore(industryB)
				nodes = append(nodes, map[string]interface{}{
					"id":                  industryB,
					"systemic_importance": score.SystemicImportance,
					"upstream_count":      score.UpstreamCount,
					"downstream_count":    score.DownstreamCount,
				})
			}

			strength := "low"
			if math.Abs(correlation) > 0.7 {
				strength = "high"
			} else if math.Abs(correlation) > 0.4 {
				strength = "medium"
			}

			edges = append(edges, map[string]interface{}{
				"source":      industryA,
				"target":      industryB,
				"correlation": correlation,
				"strength":    strength,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
	})
}
