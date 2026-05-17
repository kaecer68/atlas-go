package industry

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// DataAggregator fetches per-stock financial metrics from FinMind and
// computes industry-level weighted averages to feed CycleTracker.
type DataAggregator struct {
	tracker *CycleTracker
	tree    *ClassificationTree
	finmind *marketdata.FinMindClient
}

// NewDataAggregator creates a DataAggregator. Pass nil for finmindClient if
// FinMind is unavailable; AggregateAllIndustries will be a no-op.
func NewDataAggregator(tracker *CycleTracker, tree *ClassificationTree, finmind *marketdata.FinMindClient) *DataAggregator {
	return &DataAggregator{
		tracker: tracker,
		tree:    tree,
		finmind: finmind,
	}
}

// AggregateAllIndustries iterates over every Level-1 industry and calls
// AggregateIndustry for each. Errors on individual industries are logged
// and do not stop the rest.
func (a *DataAggregator) AggregateAllIndustries(ctx context.Context) error {
	if a.finmind == nil {
		logging.Warn("data_aggregator", "no_finmind_client", "err", "FinMind client is nil — skipping aggregation")
		return nil
	}

	industries := a.tree.GetLevel1()
	if len(industries) == 0 {
		return fmt.Errorf("data_aggregator: no Level-1 industries in classification tree")
	}

	var aggregateErr error
	for _, seg := range industries {
		if len(seg.RepresentativeStocks) == 0 {
			continue
		}
		if err := a.AggregateIndustry(ctx, seg.ID); err != nil {
			logging.Warn("data_aggregator", "industry_aggregate_failed",
				"industry", seg.ID, "err", err)
			aggregateErr = err
		}
	}
	return aggregateErr
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
		return fmt.Errorf("data_aggregator: no valid data for industry %q", industryID)
	}

	metrics := IndustryMetrics{
		IndustryID:    industryID,
		DataFreshness: FreshLive,
		Timestamp:     now,
	}
	if revCount > 0 {
		metrics.RevenueGrowthYoY = revSum / float64(revCount)
	}
	if profitCount > 0 {
		metrics.ProfitGrowthYoY = profitSum / float64(profitCount)
	}

	a.tracker.UpdatePosition(industryID, metrics)
	logging.Info("data_aggregator", "industry_updated",
		"industry", industryID,
		"rev_growth", fmt.Sprintf("%.2f", metrics.RevenueGrowthYoY),
		"profit_growth", fmt.Sprintf("%.2f", metrics.ProfitGrowthYoY),
		"rev_stocks", revCount, "profit_stocks", profitCount,
	)
	return nil
}

func (a *DataAggregator) fetchRevenueYoY(ctx context.Context, symbol string, now time.Time) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	current, err := a.finmind.GetMonthRevenue(ctx, symbol, now.Year(), int(now.Month()))
	if err != nil {
		return 0, fmt.Errorf("finmind: %w", err)
	}

	prevYear := now.Year() - 1
	prev, err := a.finmind.GetMonthRevenue(ctx, symbol, prevYear, int(now.Month()))
	if err != nil {
		return 0, fmt.Errorf("finmind prior year: %w", err)
	}

	if prev == 0 {
		return 0, fmt.Errorf("prior year revenue is zero for %s", symbol)
	}

	growth := (current - prev) / math.Abs(prev)
	return clampGrowth(growth), nil
}

func (a *DataAggregator) fetchProfitYoY(ctx context.Context, symbol string, now time.Time) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	quarter := ((int(now.Month()) - 1) / 3) + 1
	if quarter == 1 {
		quarter = 4
	}

	currentData, err := a.finmind.GetFinancialStatements(ctx, symbol, now.Year(), quarter)
	if err != nil {
		return 0, fmt.Errorf("finmind financial statements: %w", err)
	}

	currentProfit := extractProfit(currentData)

	prevData, err := a.finmind.GetFinancialStatements(ctx, symbol, now.Year()-1, quarter)
	if err != nil {
		return 0, fmt.Errorf("finmind prior year financials: %w", err)
	}

	prevProfit := extractProfit(prevData)
	if prevProfit == 0 {
		return 0, fmt.Errorf("prior year profit is zero for %s", symbol)
	}

	growth := (currentProfit - prevProfit) / math.Abs(prevProfit)
	return clampGrowth(growth), nil
}

func clampGrowth(v float64) float64 {
	const maxGrowth = 5.0  // 500% YoY growth cap (sanity check for extreme outliers)
	const minGrowth = -1.0 // -100% YoY growth floor
	if v > maxGrowth {
		return maxGrowth
	} else if v < minGrowth {
		return minGrowth
	}
	return v
}

func extractProfit(data map[string]float64) float64 {
	for _, key := range []string{"本期淨利", "本期稅後淨利", "net_profit", "NetIncome"} {
		if v, ok := data[key]; ok && v != 0 {
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
	return os.WriteFile(configPath, out, 0o644)
}
