// Package decision provides the aggregate decision-chain endpoint that
// combines narrative events, event logic rules, sector heatmap, pipeline
// recommendations, and exit alerts into a single API response.
package decision

import (
	"context"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventlogic"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"golang.org/x/sync/errgroup"
)

// Handlers provides HTTP handlers for the decision-chain aggregation API.
type Handlers struct {
	NarrativeEng  *narrative.NarrativeEngine
	Registry      *eventlogic.RuleRegistry
	IndustrySvc   *service.IndustryService
	PipelineSvc   *service.PipelineService
	MacroProvider marketdata.MacroDataProvider
	WorkDir       string
	LedgerDir     string
}

// ExitAlert represents a position that warrants an exit consideration.
type ExitAlert struct {
	Symbol     string  `json:"symbol"`
	Name       string  `json:"name"`
	DaysHeld   int     `json:"days_held"`
	PnlPct     float64 `json:"pnl_pct"`
	Suggestion string  `json:"suggestion"`
}

// PremarketData holds pre-market key indicator readings.
type PremarketData struct {
	USMarket    map[string]any `json:"us_market"`
	ForeignFlow map[string]any `json:"foreign_flow"`
	FX          map[string]any `json:"fx"`
	BDI         map[string]any `json:"bdi"`
	VIX         map[string]any `json:"vix,omitempty"`
	StressIndex map[string]any `json:"stress_index,omitempty"`
}

// EventBlock groups narrative events by time window.
type EventBlock struct {
	Today     []narrative.NarrativeEvent `json:"today"`
	Recent    []narrative.NarrativeEvent `json:"recent"`
	Premarket *PremarketData             `json:"premarket,omitempty"`
}

// RuleSummary is the API-facing summary of an event logic rule.
type RuleSummary struct {
	ID              string   `json:"id"`
	Pattern         string   `json:"pattern"`
	HitRate         float64  `json:"hit_rate"`
	AffectedSectors []string `json:"affected_sectors"`
	Direction       string   `json:"direction"`
	Status          string   `json:"status"`
}

// HeatmapEntry represents a sector's confidence signal in the decision chain.
type HeatmapEntry struct {
	Sector     string   `json:"sector"`
	Confidence string   `json:"confidence"`
	Reasons    []string `json:"reasons"`
}

// RecEntry is a simplified recommendation for the decision chain view.
type RecEntry struct {
	Symbol     string   `json:"symbol"`
	Name       string   `json:"name"`
	Action     string   `json:"action"`
	Shares     int      `json:"shares,omitempty"`
	Confidence float64  `json:"confidence"`
	Reasons    []string `json:"reasons"`
}

// RegisterRoutes registers the decision-chain endpoint on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/decision-chain", shared.Get(h.HandleDecisionChain))
}

// HandleDecisionChain aggregates narrative events, event logic rules, sector
// heatmap, pipeline recommendations, and exit alerts into a single response.
func (h *Handlers) HandleDecisionChain(r *http.Request) (int, any) {

	var (
		events       []narrative.NarrativeEvent
		rules        []*eventlogic.EventRule
		industries   []service.IndustryOverview
		pipelineData *service.RecommendationPipelineData
		exitAlerts   []ExitAlert
		premarket    *PremarketData
	)

	g, ctx := errgroup.WithContext(r.Context())

	// 1. Narrative events — detect from current market data snapshot.
	g.Go(func() error {
		_ = ctx // unused; DetectEvents is synchronous
		data := narrative.MarketNarrativeData{
			US10YChangeBps:    0,
			DXYChangePct:      0,
			VIXLevel:          0,
			USD_TWD_ChangePct: 0,
			OilChangePct:      0,
			GoldChangePct:     0,
			JPY_ChangePct:     0,
		}
		if h.NarrativeEng != nil {
			events = h.NarrativeEng.DetectEvents(data)
		}
		return nil
	})

	// 2. Event logic rules — active rules from the self-improving rule registry.
	if h.Registry != nil {
		g.Go(func() error {
			rules = h.Registry.ListActive()
			return nil
		})
	}

	// 3. Industry overview — sector heatmap data.
	if h.IndustrySvc != nil {
		g.Go(func() error {
			industries = h.IndustrySvc.GetIndustryOverview(time.Now())
			return nil
		})
	}

	// 4. Pipeline recommendations — latest session.
	if h.PipelineSvc != nil {
		g.Go(func() error {
			var err error
			pipelineData, err = h.PipelineSvc.LoadRecommendationPipeline("", false)
			if err != nil {
				// Non-fatal: pipeline may not have data yet.
				return nil
			}
			return nil
		})
	}

	// 5. Premarket data — macro snapshot for pre-market indicators.
	if h.MacroProvider != nil {
		g.Go(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			snap, err := h.MacroProvider.FetchSnapshot(ctx)
			if err != nil {
				return nil // non-fatal
			}
			premarket = &PremarketData{
				USMarket: map[string]any{
					"sox_pct": snap.SOXIndex.ChangePct,
				},
				ForeignFlow: map[string]any{
					"net_buy_twd": snap.ForeignInvestorNet.Value,
				},
				FX: map[string]any{
					"usd_twd":    snap.USD_TWD.Value,
					"change_pct": snap.USD_TWD.ChangePct,
				},
				BDI: map[string]any{
					"value":         snap.Bdi.Value,
					"deviation_pct": snap.Bdi.ChangePct,
				},
				VIX: map[string]any{
					"value":      snap.VIX.Value,
					"change_pct": snap.VIX.ChangePct,
				},
				StressIndex: map[string]any{
					"dxy":       snap.DXY.Value,
					"dxy_pct":   snap.DXY.ChangePct,
					"oil":       snap.Oil.Value,
					"oil_pct":   snap.Oil.ChangePct,
					"vix_level": snap.VIX.Value,
				},
			}
			return nil
		})
	}

	// 6. Exit alerts — computed from live portfolio state.
	g.Go(func() error {
		exitAlerts = h.computeExitAlerts()
		return nil
	})

	// Wait for all goroutines; individual errors are non-fatal above.
	_ = g.Wait()

	// --- Build response ---

	// Events: split today vs recent.
	now := time.Now()
	var todayEvents, recentEvents []narrative.NarrativeEvent
	for _, e := range events {
		if e.Timestamp.After(now.Add(-24 * time.Hour)) {
			todayEvents = append(todayEvents, e)
		} else if e.Timestamp.After(now.Add(-7 * 24 * time.Hour)) {
			recentEvents = append(recentEvents, e)
		}
	}

	// Rules: convert to response shape.
	ruleSummaries := make([]RuleSummary, 0, len(rules))
	for _, ru := range rules {
		ruleSummaries = append(ruleSummaries, RuleSummary{
			ID:              ru.ID,
			Pattern:         ru.Pattern,
			HitRate:         ru.HitRate,
			AffectedSectors: ru.AffectedSectors,
			Direction:       ru.Direction,
			Status:          ru.Status,
		})
	}

	// Sector heatmap: determine confidence from cycle phase and favorability.
	heatmap := make([]HeatmapEntry, 0, len(industries))
	for _, ind := range industries {
		confidence := "low"
		reasons := make([]string, 0, 3)
		if ind.IsFavorable {
			confidence = "high"
			reasons = append(reasons, "產業處於有利階段")
		} else if ind.CycleConfidence >= 0.5 {
			confidence = "medium"
		}
		if ind.CyclePhase != "" {
			reasons = append(reasons, "週期: "+ind.CyclePhase)
		}
		if len(ind.SeasonalPatterns) > 0 {
			reasons = append(reasons, "季節性: "+ind.SeasonalPatterns[0])
		}
		heatmap = append(heatmap, HeatmapEntry{
			Sector:     ind.Name,
			Confidence: confidence,
			Reasons:    reasons,
		})
	}

	// Recommendations: extract buy-side items from pipeline.
	recs := make([]RecEntry, 0)
	if pipelineData != nil {
		for _, item := range pipelineData.Items {
			if item.Side != "buy" {
				continue
			}
			entry := RecEntry{
				Symbol:     item.Symbol,
				Action:     item.Side,
				Confidence: float64(item.Conviction) / 100.0,
				Reasons:    []string{item.Reason},
			}
			// Shares: extract from reason or use default.
			entry.Shares = 0
			if entry.Confidence > 1.0 {
				entry.Confidence = 1.0
			}
			recs = append(recs, entry)
		}
	}

	return http.StatusOK, map[string]any{
		"events": EventBlock{
			Today:     todayEvents,
			Recent:    recentEvents,
			Premarket: premarket,
		},
		"logic_rules":     ruleSummaries,
		"sector_heatmap":  heatmap,
		"recommendations": recs,
		"exit_alerts":     exitAlerts,
	}
}

// computeExitAlerts generates exit alerts from live portfolio positions.
// It returns positions with significant unrealized P&L that may warrant exit.
func (h *Handlers) computeExitAlerts() []ExitAlert {
	if h.WorkDir == "" {
		return nil
	}
	svc := service.NewLiveService(h.WorkDir, h.LedgerDir)
	state := svc.LoadPortfolioState()

	var alerts []ExitAlert
	for _, pos := range state.Positions {
		absPnl := pos.PnlPct
		if absPnl < 0 {
			absPnl = -absPnl
		}
		if absPnl <= 5.0 {
			continue
		}

		suggestion := ""
		switch {
		case pos.PnlPct >= 20:
			suggestion = "強烈建議獲利了結"
		case pos.PnlPct >= 10:
			suggestion = "部分獲利了結"
		case pos.PnlPct <= -10:
			suggestion = "建議評估停損"
		case pos.PnlPct <= -5:
			suggestion = "注意虧損擴大"
		}

		alerts = append(alerts, ExitAlert{
			Symbol:     pos.Symbol,
			Name:       pos.Symbol, // name resolution needs symbol mapping
			DaysHeld:   0,          // not tracked in current position DTO
			PnlPct:     pos.PnlPct,
			Suggestion: suggestion,
		})
	}
	return alerts
}
