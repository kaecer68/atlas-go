// Package decision provides the aggregate decision-chain endpoint that
// combines narrative events, event logic rules, sector heatmap, pipeline
// recommendations, and exit alerts into a single API response.
package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

// symbolNameMap resolves common TW stock symbols to Chinese names.
// Keep in sync with shared_web/static/js/names.js STOCK_NAME_MAP.
var symbolNameMap = map[string]string{
	"0050.TW":  "元大台灣50",
	"0056.TW":  "元大高股息",
	"00878.TW": "國泰永續高股息",
	"1101.TW":  "台泥",
	"1216.TW":  "統一",
	"1301.TW":  "台塑",
	"1303.TW":  "南亞",
	"1326.TW":  "台化",
	"1402.TW":  "遠東新",
	"2002.TW":  "中鋼",
	"2105.TW":  "正新",
	"2207.TW":  "和泰車",
	"2303.TW":  "聯電",
	"2308.TW":  "台達電",
	"2317.TW":  "鴻海",
	"2327.TW":  "國巨",
	"2330.TW":  "台積電",
	"2357.TW":  "華碩",
	"2379.TW":  "瑞昱",
	"2382.TW":  "廣達",
	"2395.TW":  "研華",
	"2408.TW":  "南亞科",
	"2412.TW":  "中華電",
	"2454.TW":  "聯發科",
	"2880.TW":  "華南金",
	"2881.TW":  "富邦金",
	"2882.TW":  "國泰金",
	"2883.TW":  "開發金",
	"2884.TW":  "玉山金",
	"2885.TW":  "元大金",
	"2886.TW":  "兆豐金",
	"2887.TW":  "台新金",
	"2891.TW":  "中信金",
	"2892.TW":  "第一金",
	"3008.TW":  "大立光",
	"3045.TW":  "台灣大",
	"3711.TW":  "日月光投控",
	"4904.TW":  "遠傳",
	"5880.TW":  "台灣金控",
	"6505.TW":  "台塑化",
}

func resolveSymbolName(symbol string) string {
	if name, ok := symbolNameMap[symbol]; ok {
		return name
	}
	// Try adding .TW suffix if missing.
	if !strings.HasSuffix(symbol, ".TW") {
		if name, ok := symbolNameMap[symbol+".TW"]; ok {
			return name
		}
	}
	return symbol
}

// Handlers provides HTTP handlers for the decision-chain aggregation API.
type Handlers struct {
	NarrativeEng *narrative.NarrativeEngine

	IndustrySvc      *service.IndustryService
	PipelineSvc      *service.PipelineService
	MacroProvider    marketdata.MacroDataProvider
	MacroIngestor    *narrative.MacroIngestor
	WorkDir          string
	LedgerDir        string
	StrategyRegistry *strategy_techniques.Registry
}

// StrategyFrameSummary is the API-facing projection of a strategy_techniques
// StrategyFrame for the decision-chain /strategies response block. It carries
// the new fields (Layer, Themes, Risk, Attribution) and uses snake_case
// JSON to match the rest of the dashboard API (see monitoring/AGENTS.md).
type StrategyFrameSummary struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Layer           string   `json:"layer"`
	Summary         string   `json:"summary"`
	Themes          []string `json:"themes"`
	Direction       string   `json:"direction"`
	Risk            string   `json:"risk"`
	HitRate         float64  `json:"hit_rate"`
	Status          string   `json:"status"`
	Attribution     []string `json:"attribution"`
	AffectedSectors []string `json:"affected_sectors"`
}

// CoreIndicators is the explicit "4 leading indicators" view that the
// 5-layer framework's short-term judgment depends on. Units match
// MacroDataSnapshot: ForeignCapitalNetTWD is in TWD millions, the others
// are percent change. Fields are *float64 so missing macro data serializes as null.
type CoreIndicators struct {
	ForeignCapitalNetTWD *float64 `json:"foreign_capital_net_twd,omitempty"`
	TSMADRpct            *float64 `json:"tsm_adr_pct,omitempty"`
	NVDApct              *float64 `json:"nvda_pct,omitempty"`
	DXYpct               *float64 `json:"dxy_pct,omitempty"`
}

// ExitAlert represents a position that warrants an exit consideration.
type ExitAlert struct {
	Symbol     string   `json:"symbol"`
	Name       string   `json:"name"`
	DaysHeld   int      `json:"days_held"`
	PnlPct     *float64 `json:"pnl_pct,omitempty"`
	Suggestion string   `json:"suggestion"`
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
	Symbol        string   `json:"symbol"`
	Name          string   `json:"name"`
	Action        string   `json:"action"`
	Price         float64  `json:"price"`
	TargetPrice   float64  `json:"target_price"`
	StopLossPrice float64  `json:"stop_loss_price"`
	Confidence    float64  `json:"confidence"`
	Reasons       []string `json:"reasons"`
}

// RegisterRoutes registers the decision-chain endpoint on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/decision-chain", shared.Get(h.HandleDecisionChain))
}

// buildMarketNarrativeData maps a MacroDataSnapshot to MarketNarrativeData.
func buildMarketNarrativeData(snap *marketdata.MacroDataSnapshot) narrative.MarketNarrativeData {
	if snap == nil {
		return narrative.MarketNarrativeData{}
	}
	return narrative.MarketNarrativeData{
		US10YChangeBps:             snap.US10Y.ChangePct * 100,
		DXYChangePct:               snap.DXY.ChangePct,
		VIXLevel:                   snap.VIX.Value,
		USD_TWD_ChangePct:          snap.USD_TWD.ChangePct,
		OilChangePct:               snap.Oil.ChangePct,
		GoldChangePct:              snap.Gold.ChangePct,
		GoldLevel:                  snap.Gold.Value,
		JPY_ChangePct:              snap.JPY.ChangePct,
		JPYLevel:                   snap.JPY.Value,
		CPIYoY:                     snap.CPIYoY.Value,
		BDIChangePct:               snap.Bdi.ChangePct,
		CopperChangePct:            snap.Copper.ChangePct,
		ExportElectronicsChangePct: snap.ExportElectronics.ChangePct,
		SOXIndexChangePct:          snap.SOXIndex.ChangePct,
		DRAMSpotPriceChangePct:     snap.DRAMSpotPrice.ChangePct,
		SPXIndexChangePct:          snap.SPXIndex.ChangePct,
		NDXIndexChangePct:          snap.NDXIndex.ChangePct,
		DJIIndexChangePct:          snap.DJIIndex.ChangePct,
		TSMADRChangePct:            snap.TSMADR.ChangePct,
	}
}

// buildPremarketData builds the PremarketData block from a MacroDataSnapshot.
func buildPremarketData(snap *marketdata.MacroDataSnapshot) *PremarketData {
	if snap == nil {
		return nil
	}
	return &PremarketData{
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
}

// HandleDecisionChain aggregates narrative events, event logic rules, sector
// heatmap, pipeline recommendations, and exit alerts into a single response.
func (h *Handlers) HandleDecisionChain(r *http.Request) (int, any) {
	var (
		events       []narrative.NarrativeEvent
		industries   []service.IndustryOverview
		pipelineData *service.RecommendationPipelineData
		exitAlerts   []ExitAlert
	)

	// Fetch macro snapshot FIRST — it is required for accurate narrative detection.
	var macroSnap *marketdata.MacroDataSnapshot
	if h.MacroProvider != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if snap, err := h.MacroProvider.FetchSnapshot(ctx); err == nil {
			macroSnap = &snap
		}
	}

	g, ctx := errgroup.WithContext(r.Context())

	// 1. Narrative events — detect from ACTUAL market data snapshot.
	g.Go(func() error {
		_ = ctx
		data := buildMarketNarrativeData(macroSnap)
		if h.NarrativeEng != nil {
			events = h.NarrativeEng.DetectEvents(data)
		}
		return nil
	})

	// 2. Industry overview — sector heatmap data.
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

	// 5. Exit alerts — computed from live portfolio state.
	g.Go(func() error {
		exitAlerts = h.computeExitAlerts()
		return nil
	})

	// Wait for all goroutines; individual errors are non-fatal above.
	_ = g.Wait()

	// --- Build response ---

	// Events: split by calendar day (not rolling 24h window).
	now := time.Now()
	nowYear, nowMonth, nowDay := now.Date()
	var todayEvents, recentEvents []narrative.NarrativeEvent
	for _, e := range events {
		eYear, eMonth, eDay := e.Timestamp.Date()
		if eYear == nowYear && eMonth == nowMonth && eDay == nowDay {
			todayEvents = append(todayEvents, e)
		} else if e.Timestamp.After(now.Add(-7 * 24 * time.Hour)) {
			recentEvents = append(recentEvents, e)
		}
	}

	// Sector heatmap: pass numeric confidence alongside label.
	type heatmapEntry struct {
		Sector          string   `json:"sector"`
		Confidence      string   `json:"confidence"`
		ConfidenceScore float64  `json:"confidence_score"`
		Reasons         []string `json:"reasons"`
	}
	heatmap := make([]heatmapEntry, 0, len(industries))
	for _, ind := range industries {
		confidence := "low"
		score := ind.CycleConfidence
		if ind.IsFavorable {
			confidence = "high"
		} else if ind.CycleConfidence >= 0.5 {
			confidence = "medium"
		}
		reasons := make([]string, 0, 3)
		if ind.IsFavorable {
			reasons = append(reasons, "產業處於有利階段")
		}
		if ind.CyclePhase != "" {
			reasons = append(reasons, "週期: "+ind.CyclePhase)
		}
		if len(ind.SeasonalPatterns) > 0 {
			reasons = append(reasons, "季節性: "+ind.SeasonalPatterns[0])
		}
		heatmap = append(heatmap, heatmapEntry{
			Sector:          ind.Name,
			Confidence:      confidence,
			ConfidenceScore: score,
			Reasons:         reasons,
		})
	}

	// Recommendations: include all directions (buy / sell / short).
	recs := make([]RecEntry, 0)
	if pipelineData != nil {
		for _, item := range pipelineData.Items {
			entry := RecEntry{
				Symbol:        item.Symbol,
				Name:          resolveSymbolName(item.Symbol),
				Action:        item.Side,
				Price:         item.Price,
				TargetPrice:   item.TargetPrice,
				StopLossPrice: item.StopLossPrice,
				Confidence:    float64(item.Conviction) / 100.0,
				Reasons:       []string{item.Reason},
			}
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
			Premarket: buildPremarketData(macroSnap),
		},
		"sector_heatmap":  heatmap,
		"recommendations": recs,
		"exit_alerts":     exitAlerts,
		"strategies":      h.buildStrategiesSummary(),
		"core_indicators": h.buildCoreIndicators(macroSnap),
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

		pnl := pos.PnlPct
		alerts = append(alerts, ExitAlert{
			Symbol:     pos.Symbol,
			Name:       resolveSymbolName(pos.Symbol),
			DaysHeld:   -1, // TODO: not tracked in current position DTO; derive from ledger/trade history
			PnlPct:     &pnl,
			Suggestion: suggestion,
		})
	}
	return alerts
}

// buildStrategiesSummary projects the strategy_techniques.Registry into
// the active-only StrategyFrameSummary list. Returns nil if the registry
// is unset; this keeps the legacy eventlogic paths working for the
// migration window (eventlogic is retired in Wave 5 cleanup).
func (h *Handlers) buildStrategiesSummary() []StrategyFrameSummary {
	if h.StrategyRegistry == nil {
		return nil
	}
	frames := h.StrategyRegistry.All()
	out := make([]StrategyFrameSummary, 0, len(frames))
	for _, f := range frames {
		if f.Status != strategy_techniques.StatusActive {
			continue
		}
		out = append(out, StrategyFrameSummary{
			ID:              f.ID,
			Name:            f.Name,
			Layer:           f.Layer.String(),
			Summary:         f.Summary,
			Themes:          f.Themes,
			Direction:       f.Direction.String(),
			Risk:            f.Risk.String(),
			HitRate:         f.HitRate,
			Status:          f.Status.String(),
			Attribution:     f.Attribution,
			AffectedSectors: f.Sectors,
		})
	}
	return out
}

// buildCoreIndicators exposes the 4 leading indicators (ForeignInvestorNet,
// TSMADR, NVDA, DXY) used by the strategy_techniques seeds. Frontend can
// highlight them as a "core 4" strip on the strategy techniques dashboard.
// Missing individual indicators serialize as null instead of zero so the UI
// shows "—" rather than a misleading 0.00.
func (h *Handlers) buildCoreIndicators(snap *marketdata.MacroDataSnapshot) *CoreIndicators {
	// Prefer the canonical macro ingestor snapshot if available; it survives
	// transient live-provider failures (e.g., TWSE closed on weekends) and is
	// the same source used by /api/macro/capital-flow/latest.
	if h.MacroIngestor != nil {
		path := filepath.Join(h.MacroIngestor.SnapshotDir(), "latest.json")
		if data, err := os.ReadFile(path); err == nil {
			var ingestorSnap marketdata.MacroDataSnapshot
			if err := json.Unmarshal(data, &ingestorSnap); err == nil {
				indicators := &CoreIndicators{}
				if ingestorSnap.ForeignInvestorNet.Symbol != "" {
					indicators.ForeignCapitalNetTWD = &ingestorSnap.ForeignInvestorNet.Value
				}
				if ingestorSnap.TSMADR.Symbol != "" {
					indicators.TSMADRpct = &ingestorSnap.TSMADR.ChangePct
				}
				if ingestorSnap.NVDA.Symbol != "" {
					indicators.NVDApct = &ingestorSnap.NVDA.ChangePct
				}
				if ingestorSnap.DXY.Symbol != "" {
					indicators.DXYpct = &ingestorSnap.DXY.ChangePct
				}
				if indicators.ForeignCapitalNetTWD != nil || indicators.TSMADRpct != nil ||
					indicators.NVDApct != nil || indicators.DXYpct != nil {
					return indicators
				}
			}
		}
	}

	if snap == nil {
		return nil
	}
	indicators := &CoreIndicators{}
	if snap.ForeignInvestorNet.Symbol != "" {
		indicators.ForeignCapitalNetTWD = &snap.ForeignInvestorNet.Value
	}
	if snap.TSMADR.Symbol != "" {
		indicators.TSMADRpct = &snap.TSMADR.ChangePct
	}
	if snap.NVDA.Symbol != "" {
		indicators.NVDApct = &snap.NVDA.ChangePct
	}
	if snap.DXY.Symbol != "" {
		indicators.DXYpct = &snap.DXY.ChangePct
	}
	if indicators.ForeignCapitalNetTWD == nil && indicators.TSMADRpct == nil &&
		indicators.NVDApct == nil && indicators.DXYpct == nil {
		return nil
	}
	return indicators
}
