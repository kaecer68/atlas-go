// Package marketdata: finmind_government_bank_provider.go
//
// FinMindGovernmentBankProvider fetches 八大行庫 (8 core government banks) daily
// buy/sell from FinMind's TaiwanStockGovernmentBankBuySell dataset — Sponsor
// tier, data range 2021-06-30 ~ now (docs/data-sources.md:216-217). This is
// the #1740 D06 R5 backfill path for 2021-06-30..2024-12-31, where HiStock's
// broker8 page history stops (~2024-06, histock_broker8.go:15).
//
// API shape (verified live 2026-08-30 with the Sponsor token):
//   - The dataset does NOT accept data_id ("parameter data_id don't provide on
//     TaiwanStockGovernmentBankBuySell dataset"); one request returns the WHOLE
//     market for a date range (~10k rows/day = every stock × 8 banks).
//   - Row schema: date, stock_id, bank_name, buy, sell, buy_amount, sell_amount.
//     buy/sell are in shares (股); buy_amount/sell_amount are in TWD (元)
//     (cross-checked: 2330 buy_amount/buy ≈ 593.5 on 2021-07-01, close 593).
//   - bank_name ∈ {合庫, 土銀, 臺銀, 台企銀, 彰銀, 第一, 兆豐, 華南} — the same 8
//     core banks as coreBankBranches (HiStock uses the short-name variants
//     台銀/第一金/兆豐銀/華南永昌; the codes are identical).
//   - Non-trading days (holidays, before 2021-06-30) return status 200 with an
//     empty data array — the same no-data contract as HiStock empty pages.
//
// The provider aggregates the tw50Symbols universe by bank_name (per the v1
// methodology docs/specs/government-force-proxy-spec.md §3) and writes the
// standard government_flow outputs (YYYYMMDD.json + YYYYMMDD_brokers.json)
// via the same shapes as GovernmentBrokerAggregator, so downstream consumers
// are agnostic of the upstream (HiStock vs FinMind).
//
// Rate tier: Sponsor = 6000 req/hr → one token per 600ms (burst 100). The
// provider owns a dedicated FinMindClient so the shared free-tier bucket
// (600/hr, burst 60) is untouched, and a dedicated DailyQuotaTracker
// (finmind_sponsor, 144,000/day = 6000×24) so sponsor volume is not conflated
// with the free-tier 14,400/day budget. fetchDataset supplies the shared
// fetchWithRetry (429/5xx backoff), the daily-quota gate and the provider
// breaker (402/quota/no-data do NOT trip it).
package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/logging"
)

const (
	// finmindGovBankDataset is the Sponsor-tier FinMind dataset that publishes
	// the 8 core government banks' daily buy/sell by stock.
	finmindGovBankDataset = "TaiwanStockGovernmentBankBuySell"

	// Sponsor tier: 6000 req/hr → one token per 600ms; burst ≤ 100 (task
	// constraint: rate.Every(600ms), burst within 100).
	finmindGovBankRateLimit = 6000
	finmindGovBankBurst     = 100

	// finmindGovBankDailyLimit is the daily ceiling for the sponsor tracker:
	// 6000/hr × 24 = 144,000/day.
	finmindGovBankDailyLimit = 144000

	// finmindGovBankRawURL documents the upstream endpoint in output files.
	finmindGovBankRawURL = "https://api.finmindtrade.com/api/v4/data?dataset=TaiwanStockGovernmentBankBuySell"
)

// finmindGovBankCodes maps the FinMind bank_name values to the canonical
// 4-digit broker codes used by coreBankBranches / BrokerDailyDetail.
// The names are FinMind's short forms of the same 8 banks (臺銀 traditional
// script, 第一/兆豐/華南 short, 台企銀 as-is).
var finmindGovBankCodes = map[string]string{
	"合庫":  "8060",
	"土銀":  "8030",
	"臺銀":  "8040",
	"台企銀": "8010",
	"彰銀":  "8064",
	"第一":  "8011",
	"兆豐":  "8061",
	"華南":  "8080",
}

// govBankSymbolSet is the client-side universe filter (tw50Symbols), built
// once so per-day aggregation is a map lookup instead of a slice scan.
var govBankSymbolSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(tw50Symbols))
	for _, s := range tw50Symbols {
		set[s] = struct{}{}
	}
	return set
}()

// GovernmentBankDay is one trading day's aggregated 八大行庫 flow across the
// tw50Symbols universe.
type GovernmentBankDay struct {
	Date     string              // YYYYMMDD
	TotalNet int64               // TWD, positive = net buy
	Banks    []BrokerDailyDetail // per-bank aggregation, sorted by code
}

// FinMindGovernmentBankProvider fetches and aggregates the FinMind sponsor
// GovernmentBankBuySell dataset.
type FinMindGovernmentBankProvider struct {
	client *FinMindClient
}

// NewFinMindGovernmentBankProvider creates the sponsor-tier provider with its
// own token bucket (6000/hr, burst 100), DailyQuotaTracker (finmind_sponsor)
// and circuit breaker. apiKey is the Sponsor FinMind token; stateDir is where
// the daily-quota state file is persisted (default "data/state").
func NewFinMindGovernmentBankProvider(apiKey, stateDir string) *FinMindGovernmentBankProvider {
	if stateDir == "" {
		stateDir = "data/state"
	}
	tracker := NewDailyQuotaTracker("finmind_sponsor", stateDir, finmindGovBankDailyLimit)
	GlobalQuotaRegistry().Register("finmind_sponsor", tracker)
	return &FinMindGovernmentBankProvider{
		client: &FinMindClient{
			apiKey:       apiKey,
			httpClient:   httpclient.NewFactory().NewClient(30 * time.Second),
			rateLimiter:  rate.NewLimiter(rate.Every(time.Hour/finmindGovBankRateLimit), finmindGovBankBurst),
			quotaTracker: tracker,
			retryCfg:     defaultRetryConfig(),
			breaker:      newProviderBreaker("finmind_sponsor", defaultCircuitBreakerConfig()),
		},
	}
}

// NewFinMindGovernmentBankProviderWithClient wires an existing FinMindClient
// (test injection / shared-client reuse). The caller is responsible for the
// rate limiter and quota tracker configuration.
func NewFinMindGovernmentBankProviderWithClient(client *FinMindClient) *FinMindGovernmentBankProvider {
	return &FinMindGovernmentBankProvider{client: client}
}

// Client exposes the underlying FinMindClient (observability/tests).
func (p *FinMindGovernmentBankProvider) Client() *FinMindClient {
	return p.client
}

// SetHTTPClient overrides the HTTP client (tests only).
func (p *FinMindGovernmentBankProvider) SetHTTPClient(c *http.Client) {
	if c != nil && p.client != nil {
		p.client.SetHTTPClient(c)
	}
}

// FetchDay fetches the whole market for one date, aggregates the tw50Symbols
// universe by bank_name and returns the day's reading. A non-trading day
// (holiday / before the dataset start) returns (nil, nil) — the same no-data
// contract as the HiStock path — so callers can distinguish "no data" from a
// real upstream failure.
func (p *FinMindGovernmentBankProvider) FetchDay(ctx context.Context, date time.Time) (*GovernmentBankDay, error) {
	dateStr := date.Format("2006-01-02")
	// NOTE: this dataset rejects data_id; empty string returns the whole market.
	rows, err := p.client.fetchDataset(ctx, finmindGovBankDataset, "", dateStr, dateStr)
	if err != nil {
		return nil, fmt.Errorf("finmind government bank fetch %s: %w", dateStr, err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	bankAgg := make(map[string]*BrokerDailyDetail, len(finmindGovBankCodes))
	var totalNet int64
	for _, row := range rows {
		stockID, _ := row["stock_id"].(string)
		if _, ok := govBankSymbolSet[stockID]; !ok {
			continue
		}
		bankName, _ := row["bank_name"].(string)
		code, ok := finmindGovBankCodes[bankName]
		if !ok {
			logging.Warn("finmind_government_bank", "unknown_bank_name",
				"bank_name", bankName, "date", dateStr)
			continue
		}
		buyAmt := finmindGovFloat(row["buy_amount"])
		sellAmt := finmindGovFloat(row["sell_amount"])

		acc := bankAgg[code]
		if acc == nil {
			acc = &BrokerDailyDetail{Code: code, Name: coreBankBranches[code], Type: "gov"}
			bankAgg[code] = acc
		}
		acc.Buy += buyAmt
		acc.Sell += sellAmt
		acc.Net += buyAmt - sellAmt
		totalNet += buyAmt - sellAmt
	}

	banks := make([]BrokerDailyDetail, 0, len(bankAgg))
	for _, acc := range bankAgg {
		banks = append(banks, *acc)
	}
	sort.Slice(banks, func(i, j int) bool { return banks[i].Code < banks[j].Code })

	// Partial-upstream guard: the API returns 200 with rows on some days but
	// only a subset of the market (verified 2023-04-06: 1,872 rows; 2023-10-25:
	// 4,249 rows — TW50 constituents absent, vs ~10k+ rows on normal days).
	// Writing a fake total_net=0 file for such days would pollute the signal
	// ("zero government flow"), so treat them as no-data (nil, nil) with a
	// warning instead.
	if len(banks) == 0 {
		logging.Warn("finmind_government_bank", "partial_or_empty_universe",
			"date", date.Format("20060102"),
			"rows", len(rows),
			"hint", "API returned rows but none matched tw50Symbols x 8 banks; treating as no-data")
		return nil, nil
	}

	return &GovernmentBankDay{
		Date:     date.Format("20060102"),
		TotalNet: totalNet,
		Banks:    banks,
	}, nil
}

// finmindGovFloat converts the API's float64 amount (TWD) to int64 with
// rounding. Non-numeric / missing values decode to 0 (FinMind always returns
// numbers; a schema change surfaces via warnFinMindDatasetFingerprint).
func finmindGovFloat(v any) int64 {
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return int64(math.Round(f))
}

// BackfillDay fetches one date and writes the standard government_flow files:
// YYYYMMDD.json (GovernmentFlowReading) + YYYYMMDD_brokers.json (per-bank
// detail). Returns (nil, nil) on no-data days. Output shapes are byte-for-byte
// compatible with the HiStock writer so GovernmentFlowProvider and channel
// consumers behave identically.
func (p *FinMindGovernmentBankProvider) BackfillDay(ctx context.Context, date time.Time, outputDir string) (*GovernmentFlowReading, error) {
	day, err := p.FetchDay(ctx, date)
	if err != nil {
		return nil, err
	}
	if day == nil {
		return nil, nil
	}
	reading := &GovernmentFlowReading{
		Date:     day.Date,
		TotalNet: day.TotalNet,
		// 我們自己從上游分點明細聚合 → broker-aggregate（v1 方法論口徑；
		// HiStock 是 media-curated 口徑，兩者 universe 不同但 schema 相同）。
		Source: "broker-aggregate",
		RawURL: finmindGovBankRawURL,
	}
	if err := p.writeReading(outputDir, *reading); err != nil {
		return nil, fmt.Errorf("finmind government bank write reading: %w", err)
	}
	if err := p.writeBrokerDetails(outputDir, day.Date, day.Banks); err != nil {
		return nil, fmt.Errorf("finmind government bank write brokers: %w", err)
	}
	return reading, nil
}

// writeReading writes YYYYMMDD.json atomically (tmp + rename), mirroring
// GovernmentBrokerAggregator.writeReading.
func (p *FinMindGovernmentBankProvider) writeReading(outputDir string, r GovernmentFlowReading) error {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	path := filepath.Join(outputDir, r.Date+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// writeBrokerDetails writes YYYYMMDD_brokers.json with the same payload shape
// as GovernmentBrokerAggregator.writeBrokerDetails.
func (p *FinMindGovernmentBankProvider) writeBrokerDetails(outputDir, dateStr string, banks []BrokerDailyDetail) error {
	payload := struct {
		Date    string              `json:"date"`
		Source  string              `json:"source"`
		Brokers []BrokerDailyDetail `json:"brokers"`
	}{
		Date:    dateStr,
		Source:  "broker-aggregate",
		Brokers: banks,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal broker details: %w", err)
	}
	path := filepath.Join(outputDir, dateStr+"_brokers.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
