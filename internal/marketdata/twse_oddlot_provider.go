package marketdata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/constants"
)

// OddLotStats holds after-hours odd-lot trading statistics from TWSE.
type OddLotStats struct {
	Date           string  `json:"date"`
	BuyVolume      int64   `json:"buy_volume"`
	SellVolume     int64   `json:"sell_volume"`
	ImbalanceRatio float64 `json:"imbalance_ratio"`
}

// ErrOddLotUpstreamRemoved reports that the TWSE odd-lot endpoint no longer
// serves odd-lot data. Confirmed 2026-08: BFI84U now returns the 停券預告表
// (margin suspension notice) report, and MI_INDEX type=ODDLOT returns an empty
// data set. Consumers should redirect to twse_capital_flow (see known_issues).
var ErrOddLotUpstreamRemoved = fmt.Errorf("twse_oddlot: upstream report removed (redirect to twse_capital_flow)")

// TWSEOddLotProvider fetches Taiwan after-hours odd-lot trading data from TWSE.
//
// ⚠️ 資料源已移除（2026-08 實測）：BFI84U 現在回傳「得為融資融券有價證券停券
// 預告表」而非零股交易資料；MI_INDEX type=ODDLOT 回傳空 data。上游無等價公開
// 替代，短期由 twse_capital_flow 代理（見 known_issues twse_oddlot_upstream_60d）。
type TWSEOddLotProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// NewTWSEOddLotProvider creates a new TWSE odd-lot provider.
func NewTWSEOddLotProvider() *TWSEOddLotProvider {
	return &TWSEOddLotProvider{
		client:      httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:     constants.TWSEBaseURL,
		rateLimiter: getTWSESharedLimiter(), // P1-13: shared TWSE bucket
	}
}

// SetHTTPClient sets a custom HTTP client for tests.
func (p *TWSEOddLotProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.client = client
	}
}

// Name returns the provider name.
func (p *TWSEOddLotProvider) Name() string {
	return "twse_oddlot"
}

// SetRateLimiter overrides the rate limiter (tests only; P1-13 shared-bucket
// tests use SetTWSESharedLimiterForTest instead).
func (p *TWSEOddLotProvider) SetRateLimiter(l *rate.Limiter) {
	if l != nil {
		p.rateLimiter = l
	}
}

// FetchLatest retrieves the most recent odd-lot trading statistics.
func (p *TWSEOddLotProvider) FetchLatest(ctx context.Context) (*OddLotStats, error) {
	now := time.Now().UTC()
	for i := range 7 {
		dateStr := now.AddDate(0, 0, -i).Format("20060102")
		stats, err := p.fetchDate(ctx, dateStr)
		if err == nil {
			return stats, nil
		}
		// Fail fast when the upstream report was removed: later dates fail
		// identically, and consumers must redirect to twse_capital_flow
		// instead of burning 7 HTTP round-trips against a dead endpoint.
		if errors.Is(err, ErrOddLotUpstreamRemoved) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: no TWSE odd-lot data available in the last 7 days", ErrNoData)
}

func (p *TWSEOddLotProvider) fetchDate(ctx context.Context, dateStr string) (*OddLotStats, error) {
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/exchangeReport/BFI84U?response=json&date=%s", p.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var apiResp twseOddLotResponse
	if err := DecodeJSON(bytes.NewReader(body), resp.Header.Get("Content-Type"), &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if apiResp.Stat != "OK" {
		return nil, fmt.Errorf("TWSE API returned stat=%s", apiResp.Stat)
	}
	if len(apiResp.Tables) == 0 {
		// New flat response shape (BFI84U now serves the 停券預告表 report
		// instead of odd-lot data). Report the removal so consumers redirect.
		return nil, ErrOddLotUpstreamRemoved
	}

	marketTable := apiResp.Tables[0]
	if len(marketTable.Data) == 0 {
		// MI_INDEX type=ODDLOT-style report with no rows — upstream removed.
		return nil, ErrOddLotUpstreamRemoved
	}

	// Aggregate per-stock odd-lot trading volumes.
	// BFI84U per-stock rows typically contain: symbol, name, trade_volume, trade_value, open, high, low, close, ...
	// We aggregate trade_volume (column index 2) across all stocks as total volume,
	// and compute buy/sell split from close-to-open price direction as a heuristic.
	var buyVolume int64
	var sellVolume int64

	for _, row := range marketTable.Data {
		if len(row) < 9 {
			continue
		}
		vol := parseTWSEInt(row[2])
		if vol <= 0 {
			continue
		}

		// Heuristic: if close > open, the net volume is buy-dominant;
		// if close < open, sell-dominant. Proportional to close/open ratio.
		openPrice := parseTWSEFloat(row[5])
		closePrice := parseTWSEFloat(row[8])
		if closePrice > openPrice && openPrice > 0 {
			buyVolume += vol
		} else if closePrice < openPrice && closePrice > 0 {
			sellVolume += vol
		} else {
			// Flat: split evenly
			buyVolume += vol / 2
			sellVolume += vol - vol/2
		}
	}

	stats := &OddLotStats{
		Date:       dateStr,
		BuyVolume:  buyVolume,
		SellVolume: sellVolume,
	}

	if buyVolume+sellVolume > 0 {
		stats.ImbalanceRatio = float64(buyVolume-sellVolume) / float64(buyVolume+sellVolume)
	}

	return stats, nil
}

// parseTWSEFloat parses a TWSE-formatted float string (may contain commas).
func parseTWSEFloat(s string) float64 {
	s = strings.ReplaceAll(s, ",", "")
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

type twseOddLotResponse struct {
	Stat   string            `json:"stat"`
	Date   string            `json:"date"`
	Tables []twseOddLotTable `json:"tables"`
}

type twseOddLotTable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
	Total  int        `json:"total"`
}
