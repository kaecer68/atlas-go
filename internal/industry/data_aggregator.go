package industry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	configpkg "github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// DefaultFetchFallbackAttempts 是 fetchRevenueYoY / fetchProfitYoY 月份/季度 fallback 的
// 最大嘗試次數。從當前月/季度往前 fallback 直到找到 valid (current, prev year) 對。
//
// 為何預設 3：
//   - 涵蓋台灣月營收 publish lag（TWSE 月營收通常次月 10 號前 publish）— 8 月初跑會抓 7 月，7 月初跑會抓 6 月
//   - 涵蓋 quarterly financial statement filing lag (45 days after quarter end)
//   - 對 FinMind daily quota 用量：每 +1 attempt × 2 calls × 35 stocks = +70 calls per run
//     (4 runs/day → +280 calls/day，佔 14400 daily quota 約 2%)
//
// 若要拉長到 4-6 個月應對邊緣 publish 延遲，需先驗證：
//  1. production 的 "no_data" kind metric 是否在月初高峰（1-10 號）顯著上升
//  2. FinMind daily quota tracker 是否有餘裕
//
// 詳見 docs/investigations/2026-08-05-auto-cycle-update-quota-misconception.md。
const DefaultFetchFallbackAttempts = 3

// IndustryAggregateStatus 記錄單一 industry 的聚合結果，供排程任務回報
// data-health（#A03）。Succeeded=false 時 Error 為失敗原因。
type IndustryAggregateStatus struct {
	IndustryID string `json:"industry_id"`
	Succeeded  bool   `json:"succeeded"`
	Error      string `json:"error,omitempty"`
}

// AggregateReport 記錄一次 AggregateAllIndustries 的整體結果，讓
// auto_cycle_update 排程任務能填寫 ScheduledTask data-health 欄位
// （LastDataAsOf / LastNewSamples / NoProgressReason），取代「只有
// last_error 字串」的被動監控。
type AggregateReport struct {
	Attempted  int                       `json:"attempted"`
	Succeeded  int                       `json:"succeeded"`
	UpdatedAt  time.Time                 `json:"updated_at"`
	Industries []IndustryAggregateStatus `json:"industries"`
}

// DataAggregator fetches per-stock financial metrics from FinMind and
// computes industry-level weighted averages to feed CycleTracker.
type DataAggregator struct {
	tracker       *CycleTracker
	tree          *ClassificationTree
	finmind       *marketdata.FinMindClient
	recordFailure func(industryID, kind string)
}

// NewDataAggregator creates a DataAggregator. Pass nil for finmindClient if
// FinMind is unavailable; AggregateAllIndustries will be a no-op.
//
// recordFailure is called once per AggregateIndustry failure (per industry).
// It receives a stable industry ID and a `kind` value (see monitoring.MetricDataAggregatorFailures
// for the closed enum). Pass nil to disable failure telemetry — useful in tests and
// in the early-bootstrap path where the Prometheus collector has not yet been built.
// The dependency is injected as a function (rather than a *monitoring.MetricsCollector)
// to avoid an import cycle: `internal/monitoring` already imports `internal/industry`,
// so importing the other direction would deadlock the Go build.
func NewDataAggregator(tracker *CycleTracker, tree *ClassificationTree, finmind *marketdata.FinMindClient, recordFailure func(industryID, kind string)) *DataAggregator {
	return &DataAggregator{
		tracker:       tracker,
		tree:          tree,
		finmind:       finmind,
		recordFailure: recordFailure,
	}
}

// AggregateAllIndustries iterates over every Level-1 industry and calls
// AggregateIndustry for each. Errors on individual industries are logged
// and do not stop the rest. 僅在全部失敗時回傳 error（保留原簽名給既有
// caller）；需要 per-industry 明細請用 AggregateAllIndustriesReport。
func (a *DataAggregator) AggregateAllIndustries(ctx context.Context) error {
	_, err := a.AggregateAllIndustriesReport(ctx)
	return err
}

// AggregateAllIndustriesReport 與 AggregateAllIndustries 相同，但回傳
// AggregateReport 記錄每個 industry 的成敗明細（#A03）。排程任務用它
// 填寫 ScheduledTask data-health，讓 dashboard 能看到
// 「成功 N/M industries、哪些失敗」而非只有一筆 last_error。
func (a *DataAggregator) AggregateAllIndustriesReport(ctx context.Context) (*AggregateReport, error) {
	report := &AggregateReport{UpdatedAt: time.Now()}
	if a.finmind == nil {
		logging.Warn("data_aggregator", "no_finmind_client", "err", "FinMind client is nil — skipping aggregation")
		return report, nil
	}

	industries := a.tree.GetLevel1()
	if len(industries) == 0 {
		return report, fmt.Errorf("data_aggregator: no Level-1 industries in classification tree")
	}

	var aggregateErr error
	for _, seg := range industries {
		if len(seg.RepresentativeStocks) == 0 {
			continue
		}
		report.Attempted++
		status := IndustryAggregateStatus{IndustryID: seg.ID}
		if err := a.AggregateIndustry(ctx, seg.ID); err != nil {
			logging.Warn("data_aggregator", "industry_aggregate_failed",
				"industry", seg.ID, "err", err)
			status.Error = err.Error()
			report.Industries = append(report.Industries, status)
			aggregateErr = err
			continue
		}
		status.Succeeded = true
		report.Industries = append(report.Industries, status)
		report.Succeeded++
	}
	// Partial failure (e.g. FinMind quota exhausted for some symbols) must not
	// fail the whole scheduled task; only a total failure is an error.
	if report.Attempted > 0 && report.Succeeded == 0 {
		return report, aggregateErr
	}
	return report, nil
}

// AggregateIndustry fetches financial data for a single industry's
// representative stocks, computes the mean RevenueGrowthYoY and
// ProfitGrowthYoY, and calls CycleTracker.UpdatePosition.
func (a *DataAggregator) AggregateIndustry(ctx context.Context, industryID string) error {
	if a.finmind == nil {
		return fmt.Errorf("data_aggregator: no FinMind client")
	}

	seg, ok := a.tree.GetSegment(industryID)
	if !ok {
		return fmt.Errorf("data_aggregator: unknown industry %q", industryID)
	}

	stocks := seg.RepresentativeStocks
	if len(stocks) == 0 {
		return fmt.Errorf("data_aggregator: industry %q has no representative stocks", industryID)
	}

	now := time.Now()
	revSum, revCount := 0.0, 0
	profitSum, profitCount := 0.0, 0

	for _, symbol := range stocks {
		revYoY, err := a.fetchRevenueYoY(ctx, symbol, now)
		if err != nil {
			logging.Warn("data_aggregator", "revenue_fetch_failed",
				"symbol", symbol, "industry", industryID, "err", err)
			continue
		}
		revSum += revYoY
		revCount++
	}

	for _, symbol := range stocks {
		profitYoY, err := a.fetchProfitYoY(ctx, symbol, now)
		if err != nil {
			continue
		}
		profitSum += profitYoY
		profitCount++
	}

	if revCount == 0 && profitCount == 0 {
		return a.recordIndustryFailure(industryID, fmt.Errorf("data_aggregator: no valid data for industry %q", industryID))
	}

	metrics := IndustryMetrics{
		IndustryID: industryID,
		Timestamp:  now,
	}
	if revCount > 0 {
		metrics.RevenueGrowthYoY = revSum / float64(revCount)
	}
	if profitCount > 0 {
		metrics.ProfitGrowthYoY = profitSum / float64(profitCount)
	}

	a.tracker.UpdatePosition(industryID, metrics)
	logging.Info(
		"data_aggregator", "industry_updated",
		"industry", industryID,
		"rev_growth", fmt.Sprintf("%.2f", metrics.RevenueGrowthYoY),
		"profit_growth", fmt.Sprintf("%.2f", metrics.ProfitGrowthYoY),
		"rev_stocks", revCount, "profit_stocks", profitCount,
	)
	return nil
}

// recordIndustryFailure 統一處理 AggregateIndustry 的失敗：log warning、emit metric、return error。
// recordFailure callback 為 nil 時（test / bootstrap 早期）只 log 不 emit。
func (a *DataAggregator) recordIndustryFailure(industryID string, err error) error {
	kind := classifyFinMindError(err)
	logging.Warn("data_aggregator", "industry_aggregate_failed",
		"industry", industryID, "kind", kind, "err", err)
	if a.recordFailure != nil {
		a.recordFailure(industryID, kind)
	}
	return err
}

// fetchRevenueYoY tries to compute YoY revenue growth from the most recent
// available monthly data. It starts with the current month and falls back up to
// DefaultFetchFallbackAttempts months to handle the publication lag.
//
// Context timeout is 10s (not 5s): the shared FinMind rate limiter grants one
// token every 6s (600/hr). With a 5s ctx, rateLimiter.Wait(ctx) always fails
// once the burst is exhausted (Issue #1465 P1.10 — the 02:16 UTC 8/6 round
// failed 11 industries with 0 server 402s for exactly this reason). 10s
// covers one token interval plus request latency.
func (a *DataAggregator) fetchRevenueYoY(ctx context.Context, symbol string, now time.Time) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	year := now.Year()
	month := int(now.Month())

	for attempt := 0; attempt < DefaultFetchFallbackAttempts; attempt++ {
		current, err := a.finmind.GetMonthRevenue(ctx, symbol, year, month)
		if err != nil {
			// Rate-limit (local 5s ctx vs 6s token) or server 402 are
			// NOT "no data" — propagate so the metric classifies
			// correctly instead of falling back into a misleading
			// "no data in last N months" (Issue #1465 finding 2).
			if isFinMindQuotaOrRateLimited(err) {
				return 0, err
			}
			// Try previous month
			month--
			if month == 0 {
				month = 12
				year--
			}
			continue
		}

		prevYear := year - 1
		prev, err := a.finmind.GetMonthRevenue(ctx, symbol, prevYear, month)
		if err != nil {
			month--
			if month == 0 {
				month = 12
				year--
			}
			continue
		}

		if prev == 0 {
			month--
			if month == 0 {
				month = 12
				year--
			}
			continue
		}

		growth := (current - prev) / math.Abs(prev)
		return clampGrowth(growth), nil
	}

	return 0, fmt.Errorf("finmind revenue: no data for %s in last %d months", symbol, DefaultFetchFallbackAttempts)
}

// fetchProfitYoY tries to compute YoY profit growth from the most recent
// available quarterly financial statements.
func (a *DataAggregator) fetchProfitYoY(ctx context.Context, symbol string, now time.Time) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Q1=(1,2,3), Q2=(4,5,6), Q3=(7,8,9), Q4=(10,11,12)
	quarter := ((int(now.Month()) - 1) / 3) + 1
	year := now.Year()

	for attempt := 0; attempt < DefaultFetchFallbackAttempts; attempt++ {
		currentData, err := a.finmind.GetFinancialStatements(ctx, symbol, year, quarter)
		if err != nil {
			// Propagate quota/rate-limit errors instead of swallowing into
			// "no data" fallback (Issue #1465 finding 2) — same as
			// fetchRevenueYoY.
			if isFinMindQuotaOrRateLimited(err) {
				return 0, err
			}
			quarter--
			if quarter == 0 {
				quarter = 4
				year--
			}
			continue
		}

		currentProfit := extractProfit(currentData)

		prevData, err := a.finmind.GetFinancialStatements(ctx, symbol, year-1, quarter)
		if err != nil {
			quarter--
			if quarter == 0 {
				quarter = 4
				year--
			}
			continue
		}

		prevProfit := extractProfit(prevData)
		if prevProfit == 0 {
			quarter--
			if quarter == 0 {
				quarter = 4
				year--
			}
			continue
		}

		growth := (currentProfit - prevProfit) / math.Abs(prevProfit)
		return clampGrowth(growth), nil
	}

	return 0, fmt.Errorf("finmind profit: no data for %s in last %d quarters", symbol, DefaultFetchFallbackAttempts)
}

func clampGrowth(v float64) float64 {
	if v > 5.0 {
		return 5.0
	}
	if v < -1.0 {
		return -1.0
	}
	return v
}

func extractProfit(data map[string]float64) float64 {
	for _, k := range []string{"本期淨利", "NetIncome", "NetIncomeLoss"} {
		if v, ok := data[k]; ok {
			return v
		}
	}
	return 0
}

func RecalibrateThresholds(revenuePath, configPath string) error {
	results, err := CalibrateThresholdsFromFile(revenuePath)
	if err != nil {
		return fmt.Errorf("recalibrate: %w", err)
	}
	if len(results) == 0 {
		return fmt.Errorf("recalibrate: no data available")
	}
	return writeCalibratedConfig(configPath, results)
}

func writeCalibratedConfig(configPath string, results []CalibrationResult) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	industryCfg, _ := config["industry"].(map[string]any)
	if industryCfg == nil {
		industryCfg = make(map[string]any)
		config["industry"] = industryCfg
	}
	ct, _ := industryCfg["cycle_thresholds"].(map[string]any)
	if ct == nil {
		ct = make(map[string]any)
		industryCfg["cycle_thresholds"] = ct
	}
	ct["source"] = "percentile_based"
	ct["calibrated_at"] = time.Now().Format(time.RFC3339)
	delete(ct, "todo")
	value, _ := ct["value"].(map[string]any)
	if value == nil {
		value = make(map[string]any)
		ct["value"] = value
	}
	for _, r := range results {
		value[r.IndustryID] = map[string]float64{
			"expansion_revenue_pct": math.Round(r.P75*10000) / 10000,
			"expansion_profit_pct":  math.Round(r.P75*10000) / 10000,
			"recovery_revenue_pct":  math.Round(r.P50*10000) / 10000,
			"recovery_profit_pct":   math.Round(r.P50*10000) / 10000,
			"mature_revenue_pct":    math.Round(r.P25*10000) / 10000,
			"mature_profit_pct":     math.Round(r.P25*10000) / 10000,
		}
	}
	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	out = append(out, '\n')
	return configpkg.LockedWriteFileWithRollback(configPath, out)
}

// classifyFinMindError 把 FinMind API / transport error 分類到 monitoring.MetricDataAggregatorFailures 的
// kind label 值域。值域固定（quota/rate_limited/no_data/parse_error/transport/unknown）以避免 cardinality 爆炸。
//
// 判斷順序（最具體到最廣）：
//  1. errors.Is(err, marketdata.ErrQuotaExhausted) → "quota"
//  2. errors.Is(err, marketdata.ErrRateLimited)    → "rate_limited"
//  3. 字串匹配 "no month revenue data" / "no data" / "no valid data" → "no_data"
//  4. 字串匹配 "cannot parse" / "decode"            → "parse_error"
//  5. 字串匹配 "http request" / "i/o timeout" / "context deadline"
//     / "connection refused" / "no such host"      → "transport"
//  6. 其餘 → "unknown"
//
// 為何優先 errors.Is：marketdata.FinMindClient.fetchDataset 用 fmt.Errorf("finmind: %w", ErrQuotaExhausted)
// 包裝 sentinel error（finmind_client.go:179），errors.Is 可穿透 wrap chain 識別。
// isFinMindQuotaOrRateLimited reports whether err is a quota/rate-limit
// condition that must NOT be treated as "no data":
//   - local rate limiter Wait failure (ErrRateLimited, 5s ctx vs 6s token)
//   - server-side 402 (finmind_client.go fetchDataset non-2xx body)
//
// Issue #1465 P1.10: without this, both conditions were swallowed by the
// month/quarter fallback loop and surfaced as misleading
// "no data in last N months" → metric kind=no_data.
func isFinMindQuotaOrRateLimited(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, marketdata.ErrRateLimited) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "Requests reach the upper limit") ||
		strings.Contains(msg, "status 402")
}

func classifyFinMindError(err error) string {
	if err == nil {
		return "unknown"
	}
	if errors.Is(err, marketdata.ErrQuotaExhausted) {
		return "quota"
	}
	if errors.Is(err, marketdata.ErrRateLimited) {
		return "rate_limited"
	}
	// Server-side 402 (Requests reach the upper limit) is a quota
	// condition — classify as quota so alert rules can share the bucket
	// with ErrQuotaExhausted (Issue #1465 HF-1b).
	if isFinMindQuotaOrRateLimited(err) {
		return "quota"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no month revenue data"),
		strings.Contains(msg, "no data"),
		strings.Contains(msg, "no valid data"):
		return "no_data"
	case strings.Contains(msg, "cannot parse"),
		strings.Contains(msg, "decode"):
		return "parse_error"
	case strings.Contains(msg, "http request"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "context deadline"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "no such host"):
		return "transport"
	default:
		return "unknown"
	}
}
