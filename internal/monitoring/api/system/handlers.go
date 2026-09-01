package system

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/retail"
)

// DayTradingFetcher fetches day trading statistics. Set by the constructor when Gateway is available.
type DayTradingFetcher func(ctx context.Context) (*marketdata.DayTradingStats, error)

// TaifexFetcher fetches TAIFEX PCR and retail futures OI data in a single call.
type TaifexFetcher func(ctx context.Context) (pcr *marketdata.PCRStats, futuresOI *marketdata.RetailFuturesOI, err error)

// OddLotFetcher fetches TWSE odd-lot trading statistics.
type OddLotFetcher func(ctx context.Context) (*marketdata.OddLotStats, error)

// ETFFetcher fetches TWSE ETF subscription/redemption data.
type ETFFetcher func(ctx context.Context) (*marketdata.ETFStats, error)

// GeopoliticalRiskFetcher fetches the current geopolitical risk reading
// normalized to [0, 1]. Returns 0 on error (defense-first).
type GeopoliticalRiskFetcher func(ctx context.Context) float64

type Handlers struct {
	Svc                     *service.SystemService
	DayTradingFetcher       DayTradingFetcher
	TaifexFetcher           TaifexFetcher
	OddLotFetcher           OddLotFetcher
	ETFFetcher              ETFFetcher
	GeopoliticalRiskFetcher GeopoliticalRiskFetcher
}

func NewHandlers(svc *service.SystemService) *Handlers {
	// Seed the shared RSI-tw calculator's margin history from persisted margin
	// cache files at startup so the A1 z-score has a historical baseline right
	// away (not only after the retail-sentiment API is first called). No-op if
	// already seeded; missing dir is logged and falls back to in-memory only.
	if svc != nil && svc.WorkDir != "" {
		marginDir := filepath.Join(svc.WorkDir, constants.StateMargin)
		if n, err := retail.GetCalculator().BackfillMarginHistory(marginDir); err != nil {
			logging.Warn("retail_sentiment", "margin_history_backfill_failed",
				logging.Err(err), logging.FStr("margin_dir", marginDir))
		} else if n > 0 {
			logging.Info("retail_sentiment", "margin_history_backfilled",
				logging.FInt("entries", n), logging.FStr("margin_dir", marginDir))
		}
	}
	return &Handlers{Svc: svc}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/phase3-status", shared.Get(h.HandlePhase3Status))
	mux.Handle("GET /api/dashboard/system-health", shared.Get(h.HandleSystemHealth))
	mux.Handle("GET /api/dashboard/clamping-events", shared.Get(h.HandleClampingEvents))
	mux.Handle("GET /api/dashboard/conviction-clamping-events", shared.Get(h.HandleConvictionClampingEvents))
	mux.Handle("GET /api/dashboard/capital-phase", shared.Get(h.HandleCapitalPhase))
	mux.Handle("GET /api/dashboard/retail-sentiment", shared.Get(h.HandleRetailSentiment))
}

func (h *Handlers) HandlePhase3Status(r *http.Request) (int, any) {
	metrics, err := h.Svc.LoadPhase3Status()
	if err != nil {
		// File not yet created (e.g. CI / first-run) is not an error — return empty state.
		return http.StatusOK, orchestrator.Phase3Metrics{}
	}
	return http.StatusOK, metrics
}

func (h *Handlers) HandleSystemHealth(r *http.Request) (int, any) {
	health, err := h.Svc.LoadSystemHealth()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load system health: %v", err)}
	}
	return http.StatusOK, health
}

func (h *Handlers) HandleClampingEvents(r *http.Request) (int, any) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			if parsed > 1000 {
				parsed = 1000
			}
			limit = parsed
		}
	}

	events, err := h.Svc.LoadClampingEvents(limit)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load clamping events: %v", err)}
	}

	return http.StatusOK, map[string]any{
		"events": events,
		"count":  len(events),
	}
}

func (h *Handlers) HandleConvictionClampingEvents(r *http.Request) (int, any) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			if parsed > 1000 {
				parsed = 1000
			}
			limit = parsed
		}
	}

	events, err := h.Svc.LoadConvictionClampingEvents(limit)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load conviction clamping events: %v", err)}
	}

	return http.StatusOK, map[string]any{
		"events": events,
		"count":  len(events),
	}
}

// HandleCapitalPhase serves the capital phase snapshot backed by REAL data
// (#1785-A: the old handler constructed a throwaway controller per request —
// every field was permanently zero while the portfolio page showed
// NT$2.99M net value from a different source).
//
// Data sources (same ones the risk-exposure endpoint uses):
//   - sessions/<id>/summary.json series → portfolio values → rolling Sharpe
//     (last 30 sessions) and max drawdown over the session history
//   - live portfolio state (cash + positions) → total/reserve/deployed capital
//   - first session date → days-in-phase
//
// 口徑 note: the session series comes from independent daily simulations
// (each starts from the same initial cash), so the drawdown reflects the
// cross-session distribution, not a continuous equity curve — the frontend
// labels it accordingly.
func (h *Handlers) HandleCapitalPhase(r *http.Request) (int, any) {
	cfg := domain.DefaultCapitalPhaseConfig()
	snap := domain.CapitalSnapshot{
		Phase:          cfg.CurrentPhase,
		PhaseStartDate: cfg.PhaseStartDate,
	}

	if h.Svc == nil {
		return http.StatusOK, snap
	}

	// Portfolio value series from session summaries.
	sessionsDir := filepath.Join(h.Svc.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err == nil {
		type sessionEntry struct {
			name  string
			value float64
			cash  float64
			date  time.Time
		}
		var series []sessionEntry
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(sessionsDir, entry.Name(), "summary.json"))
			if err != nil {
				continue
			}
			var summary domain.SessionSummary
			if json.Unmarshal(data, &summary) != nil || summary.SessionID == "" {
				continue
			}
			series = append(series, sessionEntry{
				name:  entry.Name(),
				value: summary.PortfolioValue,
				cash:  summary.EndingCash,
				date:  summary.RecordedAt,
			})
		}
		sort.Slice(series, func(i, j int) bool { return series[i].name < series[j].name })

		if len(series) > 0 {
			first := series[0]
			if !first.date.IsZero() {
				snap.PhaseStartDate = first.date
				snap.DaysInPhase = int(time.Since(first.date).Hours() / 24)
			}
			snap.TotalCapital = series[len(series)-1].value
			snap.ReserveCash = series[len(series)-1].cash
			snap.DeployedCapital = snap.TotalCapital - snap.ReserveCash

			// Daily returns over the bounded recent window (last 30 sessions)
			// for the rolling Sharpe; max drawdown over the full series.
			window := series
			if len(window) > 30 {
				window = window[len(window)-30:]
			}
			returns := make([]float64, 0, len(window))
			for i := 1; i < len(window); i++ {
				if window[i-1].value > 0 {
					returns = append(returns, (window[i].value-window[i-1].value)/window[i-1].value)
				}
			}
			if len(returns) > 1 {
				mean := 0.0
				for _, rr := range returns {
					mean += rr
				}
				mean /= float64(len(returns))
				variance := 0.0
				for _, rr := range returns {
					variance += (rr - mean) * (rr - mean)
				}
				variance /= float64(len(returns) - 1)
				if std := math.Sqrt(variance); std > 0 {
					snap.RollingSharpe = (mean / std) * math.Sqrt(252) // annualized
				}
			}
			peak := series[0].value
			for _, s := range series {
				if s.value > peak {
					peak = s.value
				}
				if peak > 0 {
					if dd := (peak - s.value) / peak; dd > snap.MaxDrawdown {
						snap.MaxDrawdown = dd
					}
				}
			}
		}
	} else {
		logging.Warn("system_handler", "capital_phase_sessions_unreadable", logging.Err(err))
	}

	// Live portfolio state refines cash/positions when available (same source
	// as the risk-exposure endpoint's net value).
	liveBase := filepath.Join(h.Svc.WorkDir, livestore.DefaultLiveStateBasePath)
	if portfolio, err := livestore.LoadLastPortfolioState(liveBase); err == nil {
		var totalMV float64
		if positions, err := livestore.LoadLastPositions(liveBase); err == nil {
			for _, p := range positions {
				totalMV += p.MarketValue
			}
		}
		snap.TotalCapital = portfolio.Cash + totalMV
		snap.ReserveCash = portfolio.Cash
		snap.DeployedCapital = totalMV
	}

	return http.StatusOK, snap
}

type RetailSentimentResponse struct {
	SentimentScore         *float64                   `json:"sentiment_score"`
	MarginChangePct        *float64                   `json:"margin_change_pct"`
	MarginBalance          *float64                   `json:"margin_balance"`
	ShortBalance           *float64                   `json:"short_balance"`
	ShortChangePct         *float64                   `json:"short_change_pct"`
	DayTradingRatio        *float64                   `json:"day_trading_ratio"`
	MarginPercentile       *float64                   `json:"margin_percentile"`
	ExtremeReading         string                     `json:"extreme_reading"`
	Score                  *float64                   `json:"score"`
	ChangePct              *float64                   `json:"change_pct"`
	Interpretation         string                     `json:"interpretation"`
	CompositeSentiment     *float64                   `json:"composite_sentiment"`
	RetailFuturesOI        *float64                   `json:"retail_futures_oi,omitempty"`
	ETFNetSubscription     *float64                   `json:"etf_net_subscription,omitempty"`
	SentimentSubIndicators *domain.RSITwSubIndicators `json:"sentiment_sub_indicators,omitempty"`
	FetcherStatus          FetcherStatus              `json:"fetcher_status"`
}

type FetcherStatus struct {
	DayTrading       string `json:"day_trading"`
	Taifex           string `json:"taifex"`
	OddLot           string `json:"odd_lot"`
	ETF              string `json:"etf"`
	GeopoliticalRisk string `json:"geopolitical_risk"`
}

func extremeReadingFromScore(score float64) string {
	switch {
	case score >= 0.5:
		return "frenzy"
	case score <= -0.5:
		return "fear"
	default:
		return "neutral"
	}
}

func (h *Handlers) HandleRetailSentiment(r *http.Request) (int, any) {
	snap, err := loadLatestMacroSnapshot(h.Svc.WorkDir)
	if err != nil {
		return http.StatusOK, RetailSentimentResponse{
			ExtremeReading: "neutral",
			Interpretation: "no macro snapshot available",
			FetcherStatus:  FetcherStatus{DayTrading: "no_data", Taifex: "no_data", OddLot: "no_data", ETF: "no_data", GeopoliticalRisk: "no_data"},
		}
	}

	var marginPercentile *float64
	if snap.RetailMarginBalance.Symbol != "" && snap.RetailMarginBalance.Value > 0 {
		p := calculateMarginPercentile(h.Svc.WorkDir, snap.RetailMarginBalance.Value)
		marginPercentile = &p
	}

	var dayTradingRatio *float64
	var dtStats *retail.DayTradingStats
	var pcrData *marketdata.PCRStats
	var futuresOIData *marketdata.RetailFuturesOI
	var oddLotData *marketdata.OddLotStats
	var etfData *marketdata.ETFStats
	var retailFuturesOI *float64
	var etfNetSubscription *float64
	fetcherStatus := FetcherStatus{DayTrading: "not_available", Taifex: "not_available", OddLot: "not_available", ETF: "not_available", GeopoliticalRisk: "not_available"}

	fetchCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex

	if h.DayTradingFetcher != nil {
		wg.Go(func() {
			if stats, err := h.DayTradingFetcher(fetchCtx); err == nil {
				ratio := stats.VolumeRatio
				dtStatsLocal := &retail.DayTradingStats{
					Volume:      float64(stats.DayTradingVolume),
					VolumeRatio: stats.VolumeRatio,
				}
				mu.Lock()
				dayTradingRatio = &ratio
				dtStats = dtStatsLocal
				fetcherStatus.DayTrading = "ok"
				mu.Unlock()
			} else {
				mu.Lock()
				fetcherStatus.DayTrading = "error"
				mu.Unlock()
			}
		})
	}
	if h.TaifexFetcher != nil {
		wg.Go(func() {
			if pcr, futures, err := h.TaifexFetcher(fetchCtx); err == nil {
				var foi *float64
				if futures != nil {
					v := futures.RetailLongPct - futures.RetailShortPct
					foi = &v
				}
				mu.Lock()
				pcrData = pcr
				futuresOIData = futures
				retailFuturesOI = foi
				fetcherStatus.Taifex = "ok"
				mu.Unlock()
			} else {
				mu.Lock()
				fetcherStatus.Taifex = "error"
				mu.Unlock()
			}
		})
	}
	if h.OddLotFetcher != nil {
		wg.Go(func() {
			if data, err := h.OddLotFetcher(fetchCtx); err == nil {
				mu.Lock()
				oddLotData = data
				fetcherStatus.OddLot = "ok"
				mu.Unlock()
			} else {
				mu.Lock()
				fetcherStatus.OddLot = "error"
				mu.Unlock()
			}
		})
	}
	if h.ETFFetcher != nil {
		wg.Go(func() {
			if data, err := h.ETFFetcher(fetchCtx); err == nil {
				var sub *float64
				if data != nil {
					v := float64(data.NetSubscription)
					sub = &v
				}
				mu.Lock()
				etfData = data
				etfNetSubscription = sub
				fetcherStatus.ETF = "ok"
				mu.Unlock()
			} else {
				mu.Lock()
				fetcherStatus.ETF = "error"
				mu.Unlock()
			}
		})
	}

	geoRisk := 0.0
	if h.GeopoliticalRiskFetcher != nil {
		wg.Go(func() {
			g := h.GeopoliticalRiskFetcher(fetchCtx)
			mu.Lock()
			geoRisk = g
			if geoRisk > 0 {
				fetcherStatus.GeopoliticalRisk = "ok"
			} else {
				fetcherStatus.GeopoliticalRisk = "no_data"
			}
			mu.Unlock()
		})
	} else {
		fetcherStatus.GeopoliticalRisk = "not_available"
	}

	wg.Wait()

	calc := retail.GetCalculator()
	calc.SetParams(config.GetParametersConfig().RSITw)

	// Lazy one-time backfill (idempotent; already seeded at NewHandlers for the
	// production server, no-op here when that happened): read the persisted
	// margin history so A1 z-score is computed from real history after restart.
	// Missing dir is not fatal — the in-memory rolling window fallback applies.
	if _, err := calc.BackfillMarginHistory(filepath.Join(h.Svc.WorkDir, constants.StateMargin)); err != nil {
		logging.Warn("retail_sentiment", "margin_history_backfill_failed",
			logging.Err(err), logging.FStr("margin_dir", filepath.Join(h.Svc.WorkDir, constants.StateMargin)))
	}

	rsiInput := retail.RSITwInput{
		MarginBalance:      snap.RetailMarginBalance.Value,
		MarginPercentile:   getFloatOrZero(marginPercentile, func(p *float64) float64 { return *p }),
		DayTrading:         dtStats,
		VIXLevel:           snap.VIX.Value,
		ForeignInvestorNet: snap.ForeignInvestorNet.Value,
		DomesticFundNet:    snap.DomesticFundNet.Value,
		GeopoliticalRisk:   geoRisk,
		PutCallRatio:       getFloatOrZero(pcrData, func(p *marketdata.PCRStats) float64 { return p.PutCallVolumeRatio }),
		OddLotImbalance:    getFloatOrZero(oddLotData, func(o *marketdata.OddLotStats) float64 { return o.ImbalanceRatio }),
		RetailFuturesPct:   getFloatOrZero(futuresOIData, func(f *marketdata.RetailFuturesOI) float64 { return f.RetailLongPct - f.RetailShortPct }),
		ETFNetSubscription: getFloatOrZero(etfData, func(e *marketdata.ETFStats) float64 { return float64(e.NetSubscription) }),
	}

	rsiResult := calc.ComputeFinal(rsiInput)
	calc.UpdateHistory(rsiInput)

	interpretation := interpretRetailSentiment(rsiResult.Score)
	resp := RetailSentimentResponse{
		ExtremeReading:         extremeReadingFromScore(rsiResult.Score),
		Interpretation:         interpretation,
		SentimentSubIndicators: convertRSITwSubIndicators(rsiResult),
		FetcherStatus:          fetcherStatus,
		RetailFuturesOI:        retailFuturesOI,
		ETFNetSubscription:     etfNetSubscription,
		DayTradingRatio:        dayTradingRatio,
		MarginPercentile:       marginPercentile,
	}
	if snap.RetailMarginBalance.Symbol != "" {
		resp.SentimentScore = new(rsiResult.Score)
		resp.MarginChangePct = new(snap.RetailMarginBalance.ChangePct / 100)
		resp.MarginBalance = new(snap.RetailMarginBalance.Value)
		resp.Score = new(rsiResult.Score)
		resp.ChangePct = new(snap.RetailMarginBalance.ChangePct)
		resp.CompositeSentiment = new(rsiResult.Score)
	}
	if snap.RetailShortBalance.Symbol != "" {
		resp.ShortBalance = new(snap.RetailShortBalance.Value)
		resp.ShortChangePct = new(snap.RetailShortBalance.ChangePct)
	}
	return http.StatusOK, resp
}

// convertRSITwSubIndicators maps the retail calculator's flat sub-indicator map
// into the structured domain types for the API response.
func convertRSITwSubIndicators(result retail.RSITwSnapshot) *domain.RSITwSubIndicators {
	subs := result.SubIndicators
	if len(subs) == 0 {
		return nil
	}

	catA := &domain.RSITwCategoryA{AScore: result.PartAScore}
	if v, ok := subs["a3_margin_maint"]; ok {
		catA.MarginMaintenanceZ = v.ZScore
		if v.IsFallback {
			catA.IsFallback = true
			catA.FallbackFields = append(catA.FallbackFields, "a3_margin_maint")
		}
	}
	if v, ok := subs["a2_day_trading"]; ok {
		catA.DayTradingZ = v.ZScore
		if v.IsFallback {
			catA.IsFallback = true
			catA.FallbackFields = append(catA.FallbackFields, "a2_day_trading")
		}
	}
	if v, ok := subs["a1_margin_z"]; ok {
		catA.MarginBalanceZ = v.ZScore
		if v.IsFallback {
			catA.IsFallback = true
			catA.FallbackFields = append(catA.FallbackFields, "a1_margin_z")
		}
	}
	if v, ok := subs["a4_vix_map"]; ok {
		catA.VIXRiskScore = v.ZScore
		if v.IsFallback {
			catA.IsFallback = true
			catA.FallbackFields = append(catA.FallbackFields, "a4_vix_map")
		}
	}
	if v, ok := subs["a5_pcr_proxy"]; ok {
		// FIX-5: WeeklyPCR 是實際 PCR 比值（v.Value），不是映射後的分數
		// （v.ZScore 0.9/0.7/0.5/0.1），否則散戶情緒頁顯示的數字會誤導。
		catA.WeeklyPCR = v.Value
		if v.IsFallback {
			catA.IsFallback = true
			catA.FallbackFields = append(catA.FallbackFields, "a5_pcr_proxy")
		}
	}
	if v, ok := subs["a6_odd_lot"]; ok {
		catA.OddLotImbalance = v.ZScore
		if v.IsFallback {
			catA.IsFallback = true
			catA.FallbackFields = append(catA.FallbackFields, "a6_odd_lot")
		}
	}

	catC := &domain.RSITwCategoryC{CScore: result.PartCScore}
	if v, ok := subs["c1_futures_oi"]; ok {
		catC.FuturesRetailOI = v.ZScore
		if v.IsFallback {
			catC.IsFallback = true
			catC.FallbackFields = append(catC.FallbackFields, "c1_futures_oi")
		}
	}
	if v, ok := subs["c2_inst_flow"]; ok {
		catC.BrokerFlowScore = v.ZScore
		if v.IsFallback {
			catC.IsFallback = true
			catC.FallbackFields = append(catC.FallbackFields, "c2_inst_flow")
		}
	}
	if v, ok := subs["c3_etf_sub"]; ok {
		catC.ETFSubscriptionScore = v.ZScore
		if v.IsFallback {
			catC.IsFallback = true
			catC.FallbackFields = append(catC.FallbackFields, "c3_etf_sub")
		}
	}

	catD := &domain.RSITwCategoryD{
		AdjustmentFactor: result.AdjustmentFactor,
		DMultiplier:      result.AdjustmentFactor,
	}
	// Audit A04 (2026-08-12): 填充 D 觸發事件 — 先前 active_events 永遠為 null，
	// 前端固定顯示「無觸發事件」，與實際乘數（如 0.85 = D1 地緣政治觸發）矛盾。
	if v, ok := subs["d1_geopolitical"]; ok && v.ZScore != 1.0 {
		catD.ActiveEvents = append(catD.ActiveEvents, "地緣政治風險")
	}
	if v, ok := subs["d2_vix_spike"]; ok && v.ZScore != 1.0 {
		catD.ActiveEvents = append(catD.ActiveEvents, "VIX 飆升")
	}
	if v, ok := subs["d3_credit_control"]; ok && v.ZScore != 1.0 {
		catD.ActiveEvents = append(catD.ActiveEvents, "信貸緊縮")
	}
	if v, ok := subs["d1_geopolitical"]; ok && v.IsFallback {
		catD.IsFallback = true
	}
	if v, ok := subs["d3_credit_control"]; ok && v.IsFallback {
		catD.IsFallback = true
	}
	if v, ok := subs["d4_flash_crash"]; ok && v.IsFallback {
		catD.IsFallback = true
	}

	return &domain.RSITwSubIndicators{
		CategoryA: catA,
		CategoryC: catC,
		CategoryD: catD,
	}
}

func loadLatestMacroSnapshot(workDir string) (marketdata.MacroDataSnapshot, error) {
	path := filepath.Join(workDir, "data/state/macro/latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return marketdata.MacroDataSnapshot{}, err
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return marketdata.MacroDataSnapshot{}, err
	}
	return snap, nil
}

func calculateMarginPercentile(workDir string, currentValue float64) float64 {
	if currentValue <= 0 {
		return 0
	}

	pattern := filepath.Join(workDir, constants.StateMacro, "*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return 0
	}

	var values []float64
	for _, path := range matches {
		if filepath.Base(path) == "latest.json" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var snap marketdata.MacroDataSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		if snap.RetailMarginBalance.Symbol != "" && snap.RetailMarginBalance.Value > 0 {
			values = append(values, snap.RetailMarginBalance.Value)
		}
	}

	if len(values) < 2 {
		return 0.5
	}

	lessThan := 0
	for _, v := range values {
		if v < currentValue {
			lessThan++
		}
	}

	return float64(lessThan) / float64(len(values))
}

func interpretRetailSentiment(score float64) string {
	switch {
	case score >= 0.8:
		return "extremely bullish retail sentiment"
	case score >= 0.5:
		return "bullish retail sentiment"
	case score >= 0.2:
		return "mildly bullish retail sentiment"
	case score > -0.2:
		return "neutral retail sentiment"
	case score > -0.5:
		return "mildly bearish retail sentiment"
	case score > -0.8:
		return "bearish retail sentiment"
	default:
		return "extremely bearish retail sentiment"
	}
}

func getFloatOrZero[T any](data *T, fn func(*T) float64) float64 {
	if data == nil {
		return 0
	}
	return fn(data)
}
