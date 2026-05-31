package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
)

// PCRStats holds the put/call ratio data from TAIFEX.
type PCRStats struct {
	Date               string  `json:"date"`
	PutVolume          int64   `json:"put_volume"`
	CallVolume         int64   `json:"call_volume"`
	PutCallVolumeRatio float64 `json:"put_call_volume_ratio"`
	PutOI              int64   `json:"put_oi"`
	CallOI             int64   `json:"call_oi"`
	PutCallOIRatio     float64 `json:"put_call_oi_ratio"`
}

// RetailFuturesOI holds the retail trader open interest breakdown for TX futures.
// Retail OI is derived from total market OI minus Top10 large-trader positions.
type RetailFuturesOI struct {
	Date           string  `json:"date"`
	Top5LongOI     int64   `json:"top5_long_oi"`
	Top5ShortOI    int64   `json:"top5_short_oi"`
	Top10LongOI    int64   `json:"top10_long_oi"`
	Top10ShortOI   int64   `json:"top10_short_oi"`
	TotalMarketOI  int64   `json:"total_market_oi"`
	RetailLongOI   int64   `json:"retail_long_oi"`
	RetailShortOI  int64   `json:"retail_short_oi"`
	RetailLongPct  float64 `json:"retail_long_pct"`
	RetailShortPct float64 `json:"retail_short_pct"`
}

// TAIFEXProvider fetches derivatives market data from the TAIFEX OpenAPI.
// Data is free and requires no API key.
type TAIFEXProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// NewTAIFEXProvider creates a new TAIFEX data provider.
func NewTAIFEXProvider() *TAIFEXProvider {
	return &TAIFEXProvider{
		client:      httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:     "https://openapi.taifex.com.tw/v1",
		rateLimiter: rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// Name returns the provider name.
func (t *TAIFEXProvider) Name() string {
	return "taifex"
}

// FetchPCR retrieves the most recent available put/call ratio data.
func (t *TAIFEXProvider) FetchPCR(ctx context.Context) (*PCRStats, error) {
	if err := t.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := t.baseURL + "/PutCallRatio"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("taifex pcr http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read pcr body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("taifex pcr api error: status %d, body=%s", resp.StatusCode, string(body))
	}

	var rawList []taifexPCRRaw
	if err := json.Unmarshal(body, &rawList); err != nil {
		return nil, fmt.Errorf("unmarshal pcr response: %w", err)
	}

	if len(rawList) == 0 {
		return nil, fmt.Errorf("taifex pcr api returned empty list")
	}

	raw := rawList[0]
	stats := &PCRStats{
		Date:               raw.Date,
		PutVolume:          parseInt64(raw.PutVolume),
		CallVolume:         parseInt64(raw.CallVolume),
		PutCallVolumeRatio: parseFloat64(raw.PutCallVolumeRatioPct),
		PutOI:              parseInt64(raw.PutOI),
		CallOI:             parseInt64(raw.CallOI),
		PutCallOIRatio:     parseFloat64(raw.PutCallOIRatioPct),
	}

	return stats, nil
}

// FetchRetailFuturesOI retrieves retail trader open interest for TX futures.
// Uses the large trader data to derive retail (small trader) share:
//
//	retail = total market OI - top10 large trader OI
func (t *TAIFEXProvider) FetchRetailFuturesOI(ctx context.Context) (*RetailFuturesOI, error) {
	if err := t.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := t.baseURL + "/OpenInterestOfLargeTradersFutures"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("taifex large trader http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read large trader body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("taifex large trader api error: status %d, body=%s", resp.StatusCode, string(body))
	}

	var rawList []taifexLargeTraderRaw
	if err := json.Unmarshal(body, &rawList); err != nil {
		return nil, fmt.Errorf("unmarshal large trader response: %w", err)
	}

	// Find the latest TX futures all-months record for all traders (TypeOfTraders="0").
	// SettlementMonth "999912" = all months combined; "0" = all trader types.
	var latest *taifexLargeTraderRaw
	var latestDate string
	for i := range rawList {
		r := &rawList[i]
		if r.Contract != "TX" || r.SettlementMonth != "999912" || r.TypeOfTraders != "0" {
			continue
		}
		if r.Date > latestDate {
			latestDate = r.Date
			latest = r
		}
	}

	if latest == nil {
		return nil, fmt.Errorf("taifex large trader api: no TX all-months record found")
	}

	top5Long := parseInt64(latest.Top5Buy)
	top5Short := parseInt64(latest.Top5Sell)
	top10Long := parseInt64(latest.Top10Buy)
	top10Short := parseInt64(latest.Top10Sell)
	totalOI := parseInt64(latest.OIOfMarket)

	retailLong := totalOI - top10Long
	retailShort := totalOI - top10Short
	if retailLong < 0 {
		retailLong = 0
	}
	if retailShort < 0 {
		retailShort = 0
	}

	retailLongPct := safePercent(retailLong, totalOI)
	retailShortPct := safePercent(retailShort, totalOI)

	return &RetailFuturesOI{
		Date:           latest.Date,
		Top5LongOI:     top5Long,
		Top5ShortOI:    top5Short,
		Top10LongOI:    top10Long,
		Top10ShortOI:   top10Short,
		TotalMarketOI:  totalOI,
		RetailLongOI:   retailLong,
		RetailShortOI:  retailShort,
		RetailLongPct:  retailLongPct,
		RetailShortPct: retailShortPct,
	}, nil
}

// --- raw API response types ---

type taifexPCRRaw struct {
	Date                  string `json:"Date"`
	PutVolume             string `json:"PutVolume"`
	CallVolume            string `json:"CallVolume"`
	PutCallVolumeRatioPct string `json:"PutCallVolumeRatio%"`
	PutOI                 string `json:"PutOI"`
	CallOI                string `json:"CallOI"`
	PutCallOIRatioPct     string `json:"PutCallOIRatio%"`
}

type taifexLargeTraderRaw struct {
	Date            string `json:"Date"`
	Contract        string `json:"Contract"`
	ContractName    string `json:"ContractName"`
	SettlementMonth string `json:"SettlementMonth"`
	TypeOfTraders   string `json:"TypeOfTraders"`
	Top5Buy         string `json:"Top5Buy"`
	Top5Sell        string `json:"Top5Sell"`
	Top10Buy        string `json:"Top10Buy"`
	Top10Sell       string `json:"Top10Sell"`
	OIOfMarket      string `json:"OIOfMarket"`
}

// --- shared helpers ---

func parseInt64(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseFloat64(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func safePercent(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}
