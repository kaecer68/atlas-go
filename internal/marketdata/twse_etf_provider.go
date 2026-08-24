package marketdata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/constants"
)

// ETFStats holds daily ETF net subscription statistics from TWSE.
type ETFStats struct {
	Date            string `json:"date"`
	NetSubscription int64  `json:"net_subscription"`
	TotalNAV        int64  `json:"total_nav"`
	SubscriberCount int64  `json:"subscriber_count"`
}

// A05 typed errors（2026-08-10 audit）：adapter 需要區分「正常休市無資料」
// 與「真實上游故障」，否則 403/timeout 會被 7 天 fallback 偽裝成假日 stale，
// 永遠不觸發 circuit breaker。
var (
	// ErrETFNoTradingData：上游正常回覆但最近 7 天無交易資料（假日/休市）。
	// adapter 允許轉成 stale，不觸發 circuit breaker。
	ErrETFNoTradingData = fmt.Errorf("twse_etf: no trading data in last 7 days: %w", ErrNoData)
	// ErrETFUpstream：transport/HTTP 層失敗（timeout、DNS、4xx/5xx）。
	// 必須觸發 circuit breaker。
	ErrETFUpstream = fmt.Errorf("twse_etf: upstream failure: %w", ErrUpstream)
	// ErrETFSchema：回應無法解析（WAF 頁面、schema 改變、非預期格式）。
	// 必須觸發 circuit breaker。
	ErrETFSchema = fmt.Errorf("twse_etf: schema mismatch: %w", ErrSchema)
)

// TWSEETFProvider fetches Taiwan ETF net subscription data from TWSE.
//
// ⚠️ 資料源狀態（2026-08-10 實測）：`www.twse.com.tw/exchangeReport/TWT44U`
// （全市場 ETF 申購贖回淨額彙總報表）已移除 — HTTP 307 → page-not-found.html
// (404)，任何日期/參數皆同；對照組 STOCK_DAY_ALL 200 證明非 IP rate-limit。
// ETF 投資人資訊（NAV/PCF/折溢價）仍公開於 ETFortune，但申購贖回淨額
// 無等價公開替代（TWSE OpenAPI opendata 44 個 dataset 無此項、FinMind 僅
// ETF 持股）。見 `internal/monitoring/known_issues.go` `twse_etf_upstream_60d`。
// 消費者 `rsi_tw_calculator.subC3` 已停用（回 0 + IsFallback）。
type TWSEETFProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// NewTWSEETFProvider creates a new TWSE ETF provider.
func NewTWSEETFProvider() *TWSEETFProvider {
	return &TWSEETFProvider{
		client:      httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:     constants.TWSEBaseURL,
		rateLimiter: getTWSESharedLimiter(), // P1-13: shared TWSE bucket
	}
}

// SetHTTPClient sets a custom HTTP client for tests.
func (p *TWSEETFProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.client = client
	}
}

// SetRateLimiter overrides the rate limiter (for testing).
func (p *TWSEETFProvider) SetRateLimiter(lim *rate.Limiter) {
	if lim != nil {
		p.rateLimiter = lim
	}
}

// Name returns the provider name.
func (p *TWSEETFProvider) Name() string {
	return "twse_etf"
}

// FetchLatest retrieves the most recent ETF net subscription statistics.
// A05：7 天掃描遇到 hard error（upstream/schema）立即以該錯誤回傳（保真），
// 只有全部日期都是「正常無資料」才回 ErrETFNoTradingData。
func (p *TWSEETFProvider) FetchLatest(ctx context.Context) (*ETFStats, error) {
	now := time.Now().UTC()
	for i := range 7 {
		dateStr := now.AddDate(0, 0, -i).Format("20060102")
		stats, err := p.fetchDate(ctx, dateStr)
		if err == nil {
			return stats, nil
		}
		// 遇到 hard error 直接回傳，不繼續 fallback — 後續日期幾乎必然
		// 得到相同結果，且 hard error 必須即時反映到 circuit breaker。
		if !errors.Is(err, ErrETFNoTradingData) {
			return nil, err
		}
	}
	return nil, ErrETFNoTradingData
}

func (p *TWSEETFProvider) fetchDate(ctx context.Context, dateStr string) (*ETFStats, error) {
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/exchangeReport/TWT44U?response=json&date=%s", p.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.client.Do(req)
	if err != nil {
		// transport failure：DNS/timeout/連線拒絕
		return nil, fmt.Errorf("%w: http request: %v", ErrETFUpstream, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 非 2xx：403/429/5xx 都是 upstream 問題（IP rate-limit 推測的實際情況）
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: http status %d", ErrETFUpstream, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrETFUpstream, err)
	}

	var apiResp twseETFResponse
	if err := DecodeJSON(bytes.NewReader(body), resp.Header.Get("Content-Type"), &apiResp); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", ErrETFSchema, err)
	}

	// stat!=OK 或無 tables = 該日無資料（假日/休市）→ ErrETFNoTradingData
	if apiResp.Stat != "OK" || len(apiResp.Tables) == 0 {
		return nil, fmt.Errorf("%w: stat=%s tables=%d", ErrETFNoTradingData, apiResp.Stat, len(apiResp.Tables))
	}

	// ETF net subscription data is typically in the summary row of the first table.
	marketTable := apiResp.Tables[0]
	if len(marketTable.Data) == 0 {
		return nil, fmt.Errorf("%w: empty ETF data", ErrETFNoTradingData)
	}

	// Aggregate across all ETF rows to compute totals.
	var netSubTotal int64
	var navTotal int64
	var subscriberTotal int64

	for _, row := range marketTable.Data {
		if len(row) < 4 {
			continue
		}
		// Typical TWSE TWT44U columns:
		// [0] ETF name, [1] Net Subscription (units),
		// [2] Total NAV, [3] Subscriber Count
		netSubTotal += parseTWSEInt(row[1])
		navTotal += parseTWSEInt(row[2])

		// Subscriber count from a later column if available
		if len(row) > 3 {
			subscriberTotal += parseTWSEInt(row[3])
		}
	}

	stats := &ETFStats{
		Date:            dateStr,
		NetSubscription: netSubTotal,
		TotalNAV:        navTotal,
		SubscriberCount: subscriberTotal,
	}

	return stats, nil
}

type twseETFResponse struct {
	Stat   string         `json:"stat"`
	Date   string         `json:"date"`
	Tables []twseETFTable `json:"tables"`
}

type twseETFTable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
	Total  int        `json:"total"`
}
