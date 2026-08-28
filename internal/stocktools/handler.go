// Package stocktools exposes per-symbol Taiwan stock endpoints used by atlas-mcp.
package stocktools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// Deps holds the data providers required by the stocktools handlers.
type Deps struct {
	FugleClient  *marketdata.FugleClient
	TWSEQuote    marketdata.Provider // *TWSEOpenAPIProvider; interface for test injection
	Fundamentals *portfolio.FundamentalProvider
	CapitalFlow  *marketdata.TWSECapitalFlowProvider
	QuoteStore   ledger.QuoteStore
	// Revenue is the per-symbol monthly revenue provider. Optional —
	// when nil, GET /api/stock/monthly_revenue returns 503. Production
	// injects *marketdata.TSMCRevenueProvider (the same provider used by
	// the macro channel — it now exposes FetchSnapshotForSymbolAt /
	// QuotaRemaining; see internal/marketdata/tsmc_revenue_provider.go).
	// Reusing the existing provider avoids adding a second FinMind
	// client, preserves the 14400/day QuotaRegistry, and keeps the macro
	// channel's FetchSnapshot behavior unchanged (regression test in
	// tsmc_revenue_provider_test.go).
	//
	// Declared as an interface (not the concrete type) for testability,
	// mirroring the existing `TWSEQuote marketdata.Provider` pattern in
	// this struct — the quota-exhausted 503 path is only reachable via
	// a fake provider from the stocktools test package.
	//
	// Coverage: stocktools Fundamentals-based coverage guard is NOT
	// consulted for this endpoint (PR #1477 scope is chips/fundamentals/
	// technical/quote, not monthly_revenue). FinMind's
	// TaiwanStockMonthRevenue dataset covers TWSE 上市 + TPEX 上櫃 + 興櫃,
	// so the handler returns 200 + revenue for symbols like 3131 / 3587
	// / 6640 even though the other 4 stocktools endpoints correctly mark
	// them as NOT_COVERED. This is an intentional scope exception.
	Revenue MonthlyRevenueProvider
	// WinRate is the read-only stockpicker win-rate store provider
	// (PR 3c). Optional — when nil, GET /api/stock/win_rate returns 503.
	// Production injects *SQLiteWinRateProvider backed by the job-local
	// stockpicker ledger (data/state/atlas.db or ATLAS_MCP_STOCKPICKER_DB,
	// opened read-only); tests may inject a fake to exercise error paths
	// without opening SQLite.
	WinRate WinRateProvider
}

// MonthlyRevenueProvider is the minimal interface the
// /api/stock/monthly_revenue handler depends on. *TSMCRevenueProvider
// satisfies it; tests may inject a fake to exercise the quota-exhausted
// 503 path (which a real provider cannot reach from outside the
// marketdata package).
type MonthlyRevenueProvider interface {
	// FetchMonthlyRevenue returns the (year, month) revenue reading for a
	// symbol with YoY% and MoM%.
	FetchMonthlyRevenue(ctx context.Context, symbol string, year, month int) (marketdata.MonthlyRevenuePoint, error)
	// QuotaRemaining returns the number of FinMind API calls remaining
	// today (0 when no tracker configured).
	QuotaRemaining() int
}

// Handler serves per-symbol Taiwan stock endpoints.
type Handler struct {
	deps Deps
	// nowFunc 供 HandleQuote/HandleTechnical 判斷交易日；測試可注入
	// 固定時間（manifest Phase C — 非交易日標記的確定性測試）。
	nowFunc func() time.Time
}

// NewHandler creates a Handler from the given dependencies.
func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps, nowFunc: time.Now}
}

// now 回傳注入的時鐘（預設 time.Now）。
func (h *Handler) now() time.Time {
	if h.nowFunc != nil {
		return h.nowFunc()
	}
	return time.Now()
}

func RegisterRoutes(mux *http.ServeMux, deps Deps) {
	h := NewHandler(deps)
	mux.Handle("GET /api/stock/quote", shared.Get(h.HandleQuote))
	mux.Handle("GET /api/stock/fundamentals", shared.Get(h.HandleFundamentals))
	mux.Handle("GET /api/stock/chips", shared.Get(h.HandleChips))
	mux.Handle("GET /api/stock/technical", shared.Get(h.HandleTechnical))
	mux.Handle("GET /api/stock/sector-median-pe", shared.Get(h.HandleSectorMedianPE))
	mux.Handle("GET /api/stock/coverage", shared.Get(h.HandleCoverage))
	mux.Handle("GET /api/stock/monthly_revenue", shared.Get(h.HandleMonthlyRevenue))
	mux.Handle("GET /api/stock/win_rate", shared.Get(h.HandleWinRate))
}

// normalizeFundamentalsSymbol maps an API input symbol to the Yahoo-suffix
// format used by data/fundamentals.json, decoupling the storage layout from
// the API contract. Empty input and other exchange suffixes are passed through.
func normalizeFundamentalsSymbol(s string) string {
	if s == "" {
		return s
	}
	for _, suf := range []string{".TW", ".US", ".HK", ".JP", ".CN"} {
		if len(s) > len(suf) && s[len(s)-len(suf):] == suf {
			return s
		}
	}
	return s + ".TW"
}

// HandleQuote returns the latest intraday quote for a single symbol.
// It tries Fugle first (5s timeout), then falls back to TWSE OpenAPI (5s timeout).
// P0-1 fix (2026-07-26): separate timeout budgets + log failures + source annotation.
// Manifest Phase B2/C (2026-08-10): Fugle 200 但資料殘缺（closePrice-only）
// 視為失敗 → fallback TWSE；全部 provider 殘缺 → 200 + complete:false 明確
// 訊號（非「看似成功但殘缺的 200」）；非交易日 → trading_day:false 標記
// （不再因 TWSE 空快照回誤導的 503）。
func (h *Handler) HandleQuote(r *http.Request) (int, any) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		return http.StatusBadRequest, map[string]string{"error": "symbol is required"}
	}
	// Quote handler is intentionally NOT short-circuited by the coverage
	// guard: Fugle covers OTC quotes in real-world production even when
	// the TWSE fundamentals snapshot has no entry. Frontend and MCP
	// callers discover out-of-scope for chips/fundamentals through the
	// dedicated `GET /api/stock/coverage?symbol=X` endpoint or the
	// 200 + `coverage_note` field returned by the other 3 handlers.

	if h.deps.FugleClient == nil && h.deps.TWSEQuote == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "quote provider not configured"}
	}

	tradingDay := marketdata.IsTaiwanTradingDay(h.now())

	// Attempt Fugle with a short timeout first.
	if h.deps.FugleClient != nil {
		fugleCtx, fugleCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer fugleCancel()
		q, err := h.deps.FugleClient.GetQuote(fugleCtx, symbol)
		if err == nil {
			if marketdata.QuoteComplete(q) {
				q.Complete = boolPtr(true)
				q.TradingDay = boolPtr(tradingDay)
				return http.StatusOK, q
			}
			// Manifest Phase B2: Fugle 200 但 OHLC 殘缺（closePrice-only）
			// — 視為失敗 fallback，不靜默回傳殘缺 200。
			slog.Warn("stocktools: fugle quote incomplete, falling back to TWSE",
				"symbol", symbol)
		} else {
			// Log the failure so it's observable (previously silently discarded).
			slog.Warn("stocktools: fugle quote failed, falling back to TWSE",
				"symbol", symbol,
				"err", err)
		}
	}

	// Fallback: TWSE OpenAPI with its own timeout budget.
	if h.deps.TWSEQuote != nil {
		// Independent 10s budget for the fallback: context.WithoutCancel drops
		// the parent request deadline (which Fugle may have already consumed)
		// while preserving request cancellation propagation. Without this, a
		// slow Fugle response eats the fallback's time and TWSE fails with
		// context deadline exceeded (SK-22 endpoint-2 audit). 10s was chosen
		// because the TWSE STOCK_DAY_ALL fallback payload is ~300KB and can
		// take more than 5s under production network load.
		twseCtx, twseCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
		defer twseCancel()
		quotes, err := h.deps.TWSEQuote.GetQuotes(twseCtx, h.now(), []string{symbol})
		if err != nil {
			if errors.Is(err, marketdata.ErrTWSEQuoteNotFound) {
				// 所有 provider 都無此 symbol → by-design 政策語義
				//（文件問題 4 / 驗收 SOP 2）：200 + coverage_note，
				// 客戶端可區分「不在資料範圍」與「atlas 壞了」。
				return http.StatusOK, map[string]any{
					"symbol":        symbol,
					"last":          0,
					"complete":      false,
					"coverage_note": CoverageNoteNotCovered,
				}
			}
			slog.Error("stocktools: TWSE quote fallback failed",
				"symbol", symbol,
				"err", err)
			return http.StatusServiceUnavailable, map[string]string{"error": err.Error()}
		}
		if len(quotes) == 0 {
			return http.StatusNotFound, map[string]string{"error": "symbol not found"}
		}
		q := quotes[0]
		q.Complete = boolPtr(marketdata.QuoteComplete(q))
		q.TradingDay = boolPtr(tradingDay)
		return http.StatusOK, q
	}
	return http.StatusServiceUnavailable, map[string]string{"error": "quote provider failed"}
}

// boolPtr 回傳指向 b 的指標（domain.Quote 的 Complete/TradingDay 標記用；
// nil = 未評估，false 是明確訊號，故不能省略 false）。
func boolPtr(b bool) *bool { return &b }

// HandleSectorMedianPE returns the median P/E for a given sector; 0 if no data.
func (h *Handler) HandleSectorMedianPE(r *http.Request) (int, any) {
	sector := r.URL.Query().Get("sector")
	if sector == "" {
		return http.StatusBadRequest, map[string]string{"error": "sector is required"}
	}
	if h.deps.Fundamentals == nil || !h.deps.Fundamentals.HasData() {
		return http.StatusServiceUnavailable, map[string]string{"error": "fundamentals data not loaded"}
	}
	median := h.deps.Fundamentals.SectorMedianPE(sector)
	return http.StatusOK, map[string]any{
		"sector":    sector,
		"median_pe": median,
	}
}

// HandleFundamentals returns fundamental metrics for a single symbol.
func (h *Handler) HandleFundamentals(r *http.Request) (int, any) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		return http.StatusBadRequest, map[string]string{"error": "symbol is required"}
	}
	if h.deps.Fundamentals != nil && h.deps.Fundamentals.HasData() {
		cov := LookupCoverage(symbol, h.deps.Fundamentals)
		if !cov.Covered {
			return notCoveredResponse(symbol, cov, nil)
		}
	}

	if h.deps.Fundamentals == nil || !h.deps.Fundamentals.HasData() {
		return http.StatusServiceUnavailable, map[string]string{"error": "fundamentals data not loaded"}
	}
	data := h.deps.Fundamentals.Get(normalizeFundamentalsSymbol(symbol))
	if data.Sector == "" {
		data.Sector = string(industry.ClassifyBySymbol(symbol))
	}
	return http.StatusOK, data
}

// HandleChips returns institutional investor flow for a single symbol.
func (h *Handler) HandleChips(r *http.Request) (int, any) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		return http.StatusBadRequest, map[string]string{"error": "symbol is required"}
	}
	if h.deps.Fundamentals != nil && h.deps.Fundamentals.HasData() {
		cov := LookupCoverage(symbol, h.deps.Fundamentals)
		if !cov.Covered {
			// Short-circuit out-of-scope to avoid the upstream 7-day fallback
			// loop colliding with the handler's 15s context deadline (P0-1.
			return notCoveredResponse(symbol, cov, nil)
		}
	}

	if h.deps.CapitalFlow == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "capital flow provider not configured"}
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("20060102")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	flow, err := h.fetchLatestSymbolFlow(ctx, symbol, date)
	if err != nil {
		return http.StatusServiceUnavailable, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, flow
}

func (h *Handler) fetchLatestSymbolFlow(ctx context.Context, symbol, dateStr string) (marketdata.SymbolFlow, error) {
	// Try up to 7 trading days back to find the most recent data.
	for i := 0; i < 7; i++ {
		d, err := time.Parse("20060102", dateStr)
		if err != nil {
			return marketdata.SymbolFlow{}, err
		}
		d = d.AddDate(0, 0, -i)
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		ds := d.Format("20060102")
		flow, err := h.deps.CapitalFlow.FetchSymbolFlow(ctx, symbol, ds)
		if err == nil {
			return flow, nil
		}
	}
	return marketdata.SymbolFlow{}, context.Canceled
}

// HandleTechnical returns simple technical indicators for a single symbol.
// When the quote store has insufficient data, it falls back to fetching
// historical candles from Fugle on-demand, then caches the result.
func (h *Handler) HandleTechnical(r *http.Request) (int, any) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		return http.StatusBadRequest, map[string]string{"error": "symbol is required"}
	}
	if h.deps.Fundamentals != nil && h.deps.Fundamentals.HasData() {
		cov := LookupCoverage(symbol, h.deps.Fundamentals)
		if !cov.Covered {
			return notCoveredResponse(symbol, cov, map[string]any{"technical": map[string]any{"empty": true}})
		}
	}

	if h.deps.QuoteStore == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "quote store not configured"}
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 90
	}
	if days > 365 {
		days = 365
	}
	end := h.now()
	start := end.AddDate(0, 0, -days)
	// Normalize: QuoteStore uses ".TW" suffix; API input is bare symbol.
	qsSymbol := symbol
	if !strings.Contains(symbol, ".") {
		qsSymbol = symbol + ".TW"
	}
	bars, err := h.deps.QuoteStore.LoadQuotes(qsSymbol, start, end)
	if err != nil {
		return http.StatusServiceUnavailable, map[string]string{"error": err.Error()}
	}
	if len(bars) < 2 {
		// On-demand fallback: try Fugle historical candles.
		if h.deps.FugleClient != nil {
			from := start.Format("2006-01-02")
			to := end.Format("2006-01-02")
			fetched, fetchErr := h.deps.FugleClient.GetHistoricalCandles(r.Context(), symbol, from, to)
			if fetchErr == nil && len(fetched) >= 2 {
				// Cache for future requests.
				if recErr := h.deps.QuoteStore.RecordQuotes(fetched); recErr != nil {
					slog.Warn("stocktools: on-demand quote cache failed", "symbol", symbol, "err", recErr)
				}
				bars = fetched
			}
		}
	}
	if len(bars) < 2 {
		return http.StatusServiceUnavailable, map[string]string{"error": "insufficient historical quote data"}
	}
	tech := computeTechnical(bars)
	// Manifest Phase C：非交易日明確標記（假日機制接入 technical 路徑），
	// 避免 SMA/RSI 被誤讀為當日訊號。
	if !marketdata.IsTaiwanTradingDay(h.now()) {
		tech["trading_day"] = false
	}
	return http.StatusOK, tech
}

func computeTechnical(bars []domain.DailyBar) map[string]any {
	// Defensive sort: QuoteStore implementations may return bars in any order,
	// but SMA/RSI and the "latest" bar must be chronological.
	sort.Slice(bars, func(i, j int) bool {
		return bars[i].Date.Before(bars[j].Date)
	})

	closes := make([]float64, len(bars))
	for i, b := range bars {
		closes[i] = b.Close
	}
	latest := bars[len(bars)-1]
	return map[string]any{
		"symbol": latest.Symbol,
		"date":   latest.Date.Format("2006-01-02"),
		"close":  latest.Close,
		"volume": latest.Volume,
		"sma20":  sma(closes, 20),
		"sma50":  sma(closes, 50),
		"rsi14":  rsi(closes, 14),
	}
}

func sma(values []float64, n int) float64 {
	if len(values) < n {
		return 0
	}
	sum := 0.0
	for i := len(values) - n; i < len(values); i++ {
		sum += values[i]
	}
	return math.Round(sum/float64(n)*100) / 100
}

func rsi(values []float64, n int) float64 {
	if len(values) < n+1 {
		return 0
	}
	var gains, losses float64
	for i := len(values) - n; i < len(values); i++ {
		diff := values[i] - values[i-1]
		if diff > 0 {
			gains += diff
		} else {
			losses -= diff
		}
	}
	if losses == 0 {
		return 100
	}
	rs := gains / losses
	return math.Round((100-100/(1+rs))*100) / 100
}

// monthlyRevenueMinQuota is the minimum remaining FinMind daily quota
// required before HandleMonthlyRevenue will attempt a per-symbol lookup.
// A single FetchSnapshotForSymbol call may issue up to 3 FinMind requests
// (current month + same month prior year + same month prior month for
// MoM), so the handler fails-soft at 3 to avoid returning a partial
// response mid-quota. Tuned against finmindDailyLimit=14400 in
// internal/marketdata/finmind_client.go:41.
const monthlyRevenueMinQuota = 3

// HandleMonthlyRevenue returns the most recent published monthly revenue
// for a single symbol along with YoY% and MoM%. Query parameters:
//
//	symbol (required)         — 4–6 digit Taiwan code, e.g. 2330 / 3131 / 6640
//	year  (optional)          — reporting year, default = most recent closed month
//	month (optional)          — reporting month 1–12, default = most recent closed month
//
// The default (year, month) is computed as "last month" relative to now
// because TWSE/TPEX publish prior-month revenue around the 10th of the
// next month.
//
// Quota-aware: 503 with explicit error message when the FinMind daily
// budget is below monthlyRevenueMinQuota remaining. This is fail-soft
// to prevent the handler from issuing up to 3 calls and partially
// exhausting the budget (causing later requests to fail with the
// generic 14400/day exhausted error).
func (h *Handler) HandleMonthlyRevenue(r *http.Request) (int, any) {
	if h.deps.Revenue == nil {
		return http.StatusServiceUnavailable, map[string]string{
			"error": "monthly revenue provider not configured",
		}
	}
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		return http.StatusBadRequest, map[string]string{
			"error": "symbol is required",
		}
	}
	// Strip suffix variants the caller may pass (.TW / .TWO) — the
	// provider's FetchSnapshotForSymbol takes a bare 4–6 digit code.
	symbol = strings.TrimSuffix(strings.TrimSuffix(symbol, ".TW"), ".TWO")

	// Fail-soft quota check before any FinMind call. FetchMonthlyRevenue
	// issues up to 3 FinMind requests (current + year-ago for YoY +
	// month-ago for MoM). Requiring at least monthlyRevenueMinQuota
	// remaining avoids running out mid-lookup and returning a partial
	// response.
	if remaining := h.deps.Revenue.QuotaRemaining(); remaining < monthlyRevenueMinQuota {
		return http.StatusServiceUnavailable, map[string]any{
			"error":              "finmind daily quota nearly exhausted, retry tomorrow",
			"quota_remaining":    remaining,
			"quota_min_required": monthlyRevenueMinQuota,
			"symbol":             symbol,
		}
	}

	year, month, err := parseRevenueYearMonth(r, time.Now())
	if err != nil {
		return http.StatusBadRequest, map[string]string{"error": err.Error()}
	}
	// Use a dedicated 15s context so a slow FinMind response cannot
	// block the handler beyond the existing stocktools 15s budget
	// used by chips (handler.go:185). The provider's underlying
	// FinMind client has a 30s timeout (finmind_client.go:130) but
	// that's a safety net — 15s aligns with the stocktools convention.
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	pt, err := h.deps.Revenue.FetchMonthlyRevenue(ctx, symbol, year, month)
	if err != nil {
		return http.StatusServiceUnavailable, map[string]string{
			"error":  err.Error(),
			"symbol": symbol,
		}
	}
	return http.StatusOK, pt
}

func parseRevenueYearMonth(r *http.Request, now time.Time) (int, int, error) {
	yearStr := r.URL.Query().Get("year")
	monthStr := r.URL.Query().Get("month")

	if yearStr == "" && monthStr == "" {
		// Default: most recent closed month. If today is in the first
		// 10 days of a month, TWSE/TPEX may not have published prior
		// month yet (FinMind may still have it cached from previous
		// run); callers can pass year=YYYY&month=MM to override.
		last := now.AddDate(0, -1, 0)
		return last.Year(), int(last.Month()), nil
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 1990 || year > 2100 {
		return 0, 0, fmt.Errorf("invalid year %q (expected 1990-2100)", yearStr)
	}
	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		return 0, 0, fmt.Errorf("invalid month %q (expected 1-12)", monthStr)
	}
	return year, month, nil
}

// HandleWinRate returns the persisted Phase-4 stockpicker win-rate
// aggregates for a single symbol (read-only; never recomputes). Query
// parameters:
//
//	symbol         (required) — Taiwan stock code, e.g. 2330
//	condition_id   (optional) — filter to one condition, e.g. foreign-3d-net-buy
//	rolling_window (optional) — rolling window label, e.g. 120d; default 120d
//
// The response mirrors the MCP stock_get_win_rate contract: 200 +
// found=false (+ message) when the symbol has no stored data — "no data"
// is informational, not a 5xx, matching the coverage/quote convention.
// Like monthly_revenue, this endpoint is NOT short-circuited by the
// Fundamentals coverage guard: the stockpicker universe (quote symbols)
// can include TPEX codes the 4 TWSE-scoped endpoints mark NOT_COVERED.
func (h *Handler) HandleWinRate(r *http.Request) (int, any) {
	if h.deps.WinRate == nil {
		return http.StatusServiceUnavailable, map[string]string{
			"error": "win rate store not configured",
		}
	}
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		return http.StatusBadRequest, map[string]string{
			"error": "symbol is required",
		}
	}
	symbol = stripSymbolSuffix(symbol)

	window := r.URL.Query().Get("rolling_window")
	if window == "" {
		window = defaultWinRateWindow
	}
	conditionID := r.URL.Query().Get("condition_id")

	out := WinRateResponse{
		Symbol:        symbol,
		RollingWindow: window,
		Conditions:    []WinRateCondition{},
	}

	sources, err := h.deps.WinRate.Sources(r.Context(), symbol, window)
	if err != nil {
		return http.StatusServiceUnavailable, map[string]string{"error": err.Error()}
	}
	if conditionID != "" {
		want := winRateSourcePrefix + conditionID
		filtered := sources[:0]
		for _, s := range sources {
			if s == want {
				filtered = append(filtered, s)
			}
		}
		sources = filtered
	}

	for _, source := range sources {
		summary, found, err := h.deps.WinRate.LoadWinRate(r.Context(), symbol, source, window)
		if err != nil {
			return http.StatusServiceUnavailable, map[string]string{"error": err.Error()}
		}
		if !found {
			continue
		}
		out.Found = true
		cond := WinRateCondition{
			ConditionID:       strings.TrimPrefix(source, winRateSourcePrefix),
			Source:            source,
			Observations:      summary.Observations,
			Hits:              summary.Hits,
			WinRate:           summary.WinRate,
			WilsonLower:       summary.WilsonLower,
			WilsonUpper:       summary.WilsonUpper,
			Confidence:        summary.Confidence,
			CalibrationStatus: string(summary.CalibrationStatus),
			NetCostRate:       summary.NetCostRate,
			AvgForwardReturn:  summary.AvgForwardReturn,
			UpdatedAt:         summary.UpdatedAt,
		}
		if start, end, ok := h.deps.WinRate.OutcomeDateRange(r.Context(), symbol, source); ok {
			cond.DataStart = start
			cond.DataEnd = end
		}
		out.Conditions = append(out.Conditions, cond)
	}

	if !out.Found {
		scope := ""
		if conditionID != "" {
			scope = ", condition " + conditionID
		}
		out.Message = fmt.Sprintf("no stockpicker win-rate data for symbol %s (window %s%s)",
			symbol, window, scope)
	}
	return http.StatusOK, out
}
