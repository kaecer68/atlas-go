package industry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	ind "github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

type Handlers struct {
	Svc             *service.IndustryService
	SectorAllocator sectorallocation.WeightEngine
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/industry-classification", shared.Get(h.HandleIndustryClassification))
	mux.Handle("GET /api/dashboard/industry-seasonality", shared.Get(h.HandleIndustrySeasonality))
	mux.Handle("GET /api/dashboard/industry-seasonality-calendar", shared.Get(h.HandleIndustrySeasonalityCalendar))
	mux.Handle("GET /api/dashboard/industry-cycle", shared.Get(h.HandleIndustryCycle))
	// Deprecated: internal linkage data; not for web UI or MCP.
	mux.Handle("GET /api/dashboard/industry-linkage", shared.Get(h.HandleIndustryLinkage))
	// Deprecated: internal risk surface; not for web UI or MCP.
	mux.Handle("GET /api/dashboard/industry-risk", shared.Get(h.HandleIndustryRisk))
	mux.Handle("GET /api/dashboard/industry-overview", shared.Get(h.HandleIndustryOverview))
	mux.Handle("POST /api/dashboard/industry-shock-simulation", shared.Post(h.HandleShockSimulation))
	mux.Handle("GET /api/dashboard/industry-graph", shared.Get(h.HandleIndustryGraph))
	mux.Handle("GET /api/dashboard/industry-detail", shared.Get(h.HandleIndustryDetail))
	mux.Handle("GET /api/dashboard/cycle-status-card", shared.Get(h.HandleCycleStatusCard))
	// Deprecated: internal calibration; not for web UI or MCP.
	mux.Handle("GET /api/dashboard/industry-calibration", shared.Get(h.HandleIndustryCalibration))
	mux.Handle("GET /api/dashboard/calendar-events", shared.Get(h.HandleCalendarEvents)) // DEPRECATED: use /api/events/calendar
	// Deprecated: internal ODM channel data; not for web UI or MCP.
	mux.Handle("GET /api/dashboard/industry-odm-channel", shared.Get(h.HandleODMChannel))
	// Deprecated: internal data aggregator; not for web UI or MCP.
	mux.Handle("GET /api/dashboard/industry-data-aggregator", shared.Get(h.HandleDataAggregator))
	// Deprecated: internal seasonal health; not for web UI or MCP.
	mux.Handle("GET /api/dashboard/industry-seasonal-health", shared.Get(h.HandleSeasonalHealth))
	// Deprecated: internal correlation loader; not for web UI or MCP.
	mux.Handle("GET /api/dashboard/industry-correlation-loader", shared.Get(h.HandleCorrelationLoader))
	mux.Handle("GET /api/dashboard/sector-allocation-plan", shared.Get(h.HandleSectorAllocationPlan))
	mux.Handle("GET /api/industry/sectors", shared.Get(h.HandleSectorList))
	mux.Handle("GET /api/industry/sector-lookup", shared.Get(h.HandleSectorLookup))
}

func (h *Handlers) HandleIndustryClassification(r *http.Request) (int, any) {
	result := h.Svc.GetClassificationTree()

	return http.StatusOK, map[string]any{
		"industries": result,
		"count":      len(result),
	}
}

func (h *Handlers) HandleIndustrySeasonality(r *http.Request) (int, any) {
	industryID := r.URL.Query().Get("industry")
	now := time.Now()

	active, historical, adjustment := h.Svc.GetSeasonalPatterns(industryID, now)

	var activePatterns []map[string]any
	for _, p := range active {
		activePatterns = append(activePatterns, map[string]any{
			"id":                  p.ID,
			"name":                p.Name,
			"description":         p.Description,
			"start_month":         p.StartMonth,
			"start_day":           p.StartDay,
			"end_month":           p.EndMonth,
			"end_day":             p.EndDay,
			"historical_accuracy": p.HistoricalAccuracy,
			"avg_market_return":   p.AvgMarketReturn,
			// Deprecated: remove after 2026-Q3 migration window.
			"typical_return":      p.AvgMarketReturn,
			"affected_industries": p.AffectedIndustries,
		})
	}

	var historicalPatterns []map[string]any
	for _, p := range historical {
		historicalPatterns = append(historicalPatterns, map[string]any{
			"id":                  p.ID,
			"name":                p.Name,
			"name_en":             p.NameEN,
			"description":         p.Description,
			"start_month":         p.StartMonth,
			"start_day":           p.StartDay,
			"end_month":           p.EndMonth,
			"end_day":             p.EndDay,
			"historical_accuracy": p.HistoricalAccuracy,
			"avg_market_return":   p.AvgMarketReturn,
			// Deprecated: remove after 2026-Q3 migration window.
			"typical_return":     p.AvgMarketReturn,
			"adjustment_factor":  p.AdjustmentFactor,
			"favored_industries": p.FavoredIndustries,
			"avoided_industries": p.AvoidedIndustries,
			"impact":             p.Impact,
		})
	}

	// Build adjustment breakdown
	var breakdown map[string]any
	if industryID != "" {
		bd := h.Svc.GetAdjustmentBreakdown(industryID, now)
		if bd != nil {
			breakdown = map[string]any{
				"direct_match": bd.DirectMatch,
				"supply_chain": bd.SupplyChain,
				"narrative":    bd.Narrative,
				"dynamic_env":  bd.DynamicEnv,
				"composite":    bd.Composite,
			}
		}
	}

	// Get active narrative themes for seasonality context
	var narrativeThemes []string
	if industryID != "" {
		narrativeThemes = h.Svc.GetActiveNarrativeThemes(industryID)
	}

	return http.StatusOK, map[string]any{
		"current_date":         now.Format("2006-01-02"),
		"active_patterns":      activePatterns,
		"pattern_count":        len(activePatterns),
		"adjustment":           adjustment,
		"adjustment_breakdown": breakdown,
		"narrative_themes":     narrativeThemes,
		"all_patterns":         historicalPatterns,
		"total_pattern_count":  len(historicalPatterns),
		"calibration_evidence": h.Svc.GetCalibrationEvidence(),
	}
}

func (h *Handlers) HandleIndustrySeasonalityCalendar(r *http.Request) (int, any) {
	industryID := r.URL.Query().Get("industry")
	now := time.Now()
	months := h.Svc.GetSeasonalCalendar(industryID, now.Year())

	return http.StatusOK, map[string]any{
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
		var allPositions []map[string]any
		for _, pos := range positions {
			allPositions = append(allPositions, map[string]any{
				"industry":             pos.Industry,
				"name":                 pos.Name,
				"business_cycle":       pos.BusinessCycle,
				"inventory_cycle":      pos.InventoryCycle,
				"capex_cycle":          pos.CapexCycle,
				"confidence":           pos.Confidence,
				"updated_at":           pos.UpdatedAt,
				"is_favorable":         pos.IsFavorable,
				"phase_score":          pos.PhaseScore,
				"trend":                pos.Trend,
				"confidence_breakdown": pos.ConfidenceBreakdown,
				"threshold_evidence":   pos.ThresholdEvidence,
				"evidence":             pos.Evidence,
				"narrative_theme":      pos.NarrativeTheme,
			})
		}
		return http.StatusOK, map[string]any{
			"industries": allPositions,
			"count":      len(allPositions),
		}
	}

	pos := positions[0]
	return http.StatusOK, map[string]any{
		"industry":             pos.Industry,
		"business_cycle":       pos.BusinessCycle,
		"inventory_cycle":      pos.InventoryCycle,
		"capex_cycle":          pos.CapexCycle,
		"confidence":           pos.Confidence,
		"updated_at":           pos.UpdatedAt,
		"is_favorable":         pos.IsFavorable,
		"phase_score":          pos.PhaseScore,
		"trend":                pos.Trend,
		"confidence_breakdown": pos.ConfidenceBreakdown,
		"threshold_evidence":   pos.ThresholdEvidence,
		"evidence":             pos.Evidence,
		"narrative_theme":      pos.NarrativeTheme,
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

	return http.StatusOK, map[string]any{
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

	return http.StatusOK, map[string]any{
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

	var industries []map[string]any
	for _, o := range overviews {
		industries = append(industries, map[string]any{
			"id":                  o.ID,
			"name":                o.Name,
			"base_weight":         o.BaseWeight,
			"adjusted_weight":     o.AdjustedWeight,
			"cycle_phase":         o.CyclePhase,
			"inventory_cycle":     o.InventoryCycle,
			"capex_cycle":         o.CapexCycle,
			"cycle_confidence":    o.CycleConfidence,
			"is_favorable":        o.IsFavorable,
			"seasonal_patterns":   o.SeasonalPatterns,
			"linkage_score":       o.LinkageScore,
			"cycle_multiplier":    o.CycleMultiplier,
			"seasonal_multiplier": o.SeasonalMultiplier,
			"linkage_multiplier":  o.LinkageMultiplier,
			"adjustment_log":      o.AdjustmentLog,
		})
	}

	return http.StatusOK, map[string]any{
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

	var impactList []map[string]any
	for _, impact := range impacts {
		impactList = append(impactList, map[string]any{
			"industry": impact.Industry,
			"impact":   impact.Impact,
		})
	}

	return http.StatusOK, map[string]any{
		"source":       req.SourceIndustry,
		"shock":        req.ShockMagnitude,
		"max_depth":    req.MaxDepth,
		"impacts":      impactList,
		"impact_count": len(impactList),
	}
}

func (h *Handlers) HandleIndustryGraph(r *http.Request) (int, any) {
	nodes, edges := h.Svc.GetIndustryGraph()

	var nodeList []map[string]any
	for _, n := range nodes {
		nodeList = append(nodeList, map[string]any{
			"id":                  n.ID,
			"systemic_importance": n.SystemicImportance,
			"upstream_count":      n.UpstreamCount,
			"downstream_count":    n.DownstreamCount,
		})
	}

	var edgeList []map[string]any
	for _, e := range edges {
		edgeList = append(edgeList, map[string]any{
			"source":      e.Source,
			"target":      e.Target,
			"correlation": e.Correlation,
			"strength":    e.Strength,
		})
	}

	return http.StatusOK, map[string]any{
		"nodes": nodeList,
		"edges": edgeList,
	}
}

func (h *Handlers) HandleCycleStatusCard(r *http.Request) (int, any) {
	industryID := r.URL.Query().Get("industry")
	now := time.Now()

	var card any
	var err error
	if industryID != "" {
		card, err = h.Svc.BuildIndustryCycleStatusCard(now, industryID)
	} else {
		card, err = h.Svc.BuildCycleStatusCard(now)
	}
	if err != nil {
		return http.StatusInternalServerError, map[string]any{
			"error": err.Error(),
		}
	}
	return http.StatusOK, map[string]any{
		"card": card,
	}
}

func (h *Handlers) HandleIndustryCalibration(r *http.Request) (int, any) {
	if h.Svc == nil {
		return http.StatusInternalServerError, map[string]string{"error": "service not available"}
	}

	metrics := h.Svc.GetCalibrationMetrics()
	if metrics == nil {
		return http.StatusOK, map[string]any{
			"calibrated": false,
			"message":    "no calibration data available",
			"layers":     []map[string]any{},
		}
	}

	var layers []map[string]any
	for layer, m := range metrics {
		layers = append(layers, map[string]any{
			"layer":           layer,
			"total_signals":   m.TotalSignals,
			"correct_signals": m.CorrectSignals,
			"accuracy":        m.Accuracy,
			"last_updated":    m.LastUpdated,
		})
	}

	cal := h.Svc.CycleCalibration
	return http.StatusOK, map[string]any{
		"calibrated":    true,
		"outcome_count": cal.GetOutcomeCount(),
		"layers":        layers,
	}
}

func (h *Handlers) HandleCalendarEvents(r *http.Request) (int, any) {
	if h.Svc == nil || h.Svc.EventCalendar == nil {
		return http.StatusInternalServerError, map[string]string{"error": "calendar service not available"}
	}

	events := h.Svc.EventCalendar.GetAllEvents()

	type eventDTO struct {
		ID                  string   `json:"id"`
		Name                string   `json:"name"`
		EventType           string   `json:"event_type"`
		Description         string   `json:"description"`
		Direction           string   `json:"direction"`
		BaseWeight          float64  `json:"base_weight"`
		Active              bool     `json:"active"`
		StartDate           string   `json:"start_date"`
		EndDate             string   `json:"end_date"`
		PeakDate            string   `json:"peak_date"`
		DecayDays           int      `json:"decay_days"`
		AffectedIndustries  []string `json:"affected_industries"`
		SentimentAdjustment float64  `json:"sentiment_adjustment"`
		DataSource          string   `json:"data_source"`
		EvidenceQuality     string   `json:"evidence_quality"`
		GeneratedAt         string   `json:"generated_at"`
	}

	result := make([]eventDTO, 0, len(events))
	for _, evt := range events {
		result = append(result, eventDTO{
			ID:                  evt.ID,
			Name:                evt.Name,
			EventType:           evt.EventType,
			Description:         evt.Description,
			Direction:           evt.Direction,
			BaseWeight:          evt.BaseWeight,
			Active:              evt.Active,
			StartDate:           evt.StartDate.Format("2006-01-02"),
			EndDate:             evt.EndDate.Format("2006-01-02"),
			PeakDate:            evt.PeakDate.Format("2006-01-02"),
			DecayDays:           evt.DecayDays,
			AffectedIndustries:  evt.AffectedIndustries,
			SentimentAdjustment: evt.SentimentAdjustment,
			DataSource:          string(evt.DataSource),
			EvidenceQuality:     string(evt.EvidenceQuality),
			GeneratedAt:         evt.GeneratedAt.Format(time.RFC3339),
		})
	}

	return http.StatusOK, map[string]any{
		"events": result,
		"count":  len(result),
	}
}

func (h *Handlers) HandleODMChannel(r *http.Request) (int, any) {
	snap := h.Svc.GetODMChannelSnapshot(r.Context())
	return http.StatusOK, snap
}

func (h *Handlers) HandleDataAggregator(r *http.Request) (int, any) {
	summary := h.Svc.GetDataAggregatorSummary()
	return http.StatusOK, summary
}

func (h *Handlers) HandleSeasonalHealth(r *http.Request) (int, any) {
	health, err := h.Svc.GetSeasonalHealth()
	if err != nil {
		return http.StatusOK, map[string]any{
			"error": err.Error(),
		}
	}
	return http.StatusOK, health
}

func (h *Handlers) HandleCorrelationLoader(r *http.Request) (int, any) {
	replayPath := config.Load().ReplayDataPath
	sectorSymbolsPath := ""
	meta := h.Svc.GetCorrelationLoaderMetadata(replayPath, sectorSymbolsPath)
	return http.StatusOK, meta
}

func (h *Handlers) HandleSectorAllocationPlan(r *http.Request) (int, any) {
	snap, err := h.Svc.GetLatestSectorAllocation(r.Context())
	if err != nil {
		return http.StatusServiceUnavailable, map[string]any{
			"error":           "snapshot_unavailable",
			"message":         err.Error(),
			"fallback_reason": "snapshot_unavailable",
		}
	}
	if snap == nil {
		// No simulation session has closed yet — this is expected in
		// staging/dev with no replay data. Return an empty plan with
		// fallback_reason so the frontend can show a meaningful state.
		return http.StatusOK, sectorallocation.SectorAllocationSnapshot{
			FallbackReason: "no_simulation_session",
		}
	}
	return http.StatusOK, snap
}

// --- Sector taxonomy handlers (E-06: HTTP proxy for MCP-in-memory tools) ---

// HandleSectorList returns the full 20-sector taxonomy.
// Mirrors the MCP industry_sector_list tool.
func (h *Handlers) HandleSectorList(r *http.Request) (int, any) {
	all := ind.AllSectors()
	repr := ind.DefaultRepresentativeStocks()
	sectors := make([]map[string]any, 0, len(all))
	for _, id := range all {
		syms := repr[id]
		if syms == nil {
			syms = []string{}
		}
		sectors = append(sectors, map[string]any{
			"id":            string(id),
			"display_zh":    ind.DisplayZH(id),
			"stock_symbols": syms,
		})
	}
	return http.StatusOK, map[string]any{"sectors": sectors}
}

// HandleSectorLookup looks up a sector by symbol or sector name/alias.
// Mirrors the MCP industry_sector_lookup tool.
func (h *Handlers) HandleSectorLookup(r *http.Request) (int, any) {
	symbol := r.URL.Query().Get("symbol")
	sector := r.URL.Query().Get("sector")

	if symbol == "" && sector == "" {
		return http.StatusBadRequest, map[string]string{"error": "provide either symbol or sector query parameter"}
	}

	var secID ind.SectorID
	if symbol != "" {
		secID = ind.ClassifyBySymbol(symbol)
		if secID == "" {
			return http.StatusOK, map[string]any{
				"found":   false,
				"warning": fmt.Sprintf("symbol %q not found in representative stocks", symbol),
			}
		}
	} else {
		var ok bool
		secID, ok = ind.SectorIDFromString(sector)
		if !ok {
			return http.StatusOK, map[string]any{
				"found":   false,
				"warning": fmt.Sprintf("sector %q not recognized", sector),
			}
		}
	}

	syms := ind.DefaultRepresentativeStocks()[secID]
	if syms == nil {
		syms = []string{}
	}
	return http.StatusOK, map[string]any{
		"found": true,
		"sector": map[string]any{
			"id":            string(secID),
			"display_zh":    ind.DisplayZH(secID),
			"stock_symbols": syms,
		},
	}
}
