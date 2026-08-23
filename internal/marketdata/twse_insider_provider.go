// Package marketdata — TWSE insider trading provider.
package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// InsiderReading holds a single daily insider trading record from TWSE.
type InsiderReading struct {
	Date         string `json:"date"`          // 出表日期 YYYYMMDD
	StockID      string `json:"stock_id"`      // 公司代號
	CompanyName  string `json:"company_name"`  // 公司名稱
	Role         string `json:"role"`          // 申報人身分（董事/經理人/大股東）
	Name         string `json:"name"`          // 姓名
	TransferType string `json:"transfer_type"` // 轉讓方式
	Shares       int64  `json:"shares"`        // 預定轉讓股數
	HeldShares   int64  `json:"held_shares"`   // 目前持有股數
}

// InsiderAggregate holds daily aggregate insider trading statistics.
type InsiderAggregate struct {
	Date          string `json:"date"`
	TotalDeclared int64  `json:"total_declared"` // 總申讓股數
	TotalStocks   int    `json:"total_stocks"`   // 有申讓的股票數
	TotalInsiders int    `json:"total_insiders"` // 申讓人數
	NetSentiment  int    `json:"net_sentiment"`  // 淨賣出人數（賣-買；負值=淨買入）
	DominantRole  string `json:"dominant_role"`  // 最多申讓的內部人類型
}

// TWSEClient  already defined in twse_openapi.go. Use GetSharedTWSEClient().

// TWSEInsiderProvider fetches insider trading data from TWSE OpenAPI.
type TWSEInsiderProvider struct {
	client     *http.Client
	baseURL    string
	limiter    *rate.Limiter
	storageDir string
}

var insiderOpenAPIURL = "https://openapi.twse.com.tw/v1/opendata/t187ap12_L"

// NewTWSEInsiderProvider creates a provider that fetches insider trading data.
// Pass an empty storageDir to skip persisting data to disk.
func NewTWSEInsiderProvider(storageDir string) *TWSEInsiderProvider {
	params := config.GetParametersConfig()
	return &TWSEInsiderProvider{
		client:     httpclient.NewFactory().NewClient(time.Duration(params.Marketdata.TWSEAPITimeoutSec.Value) * time.Second),
		baseURL:    constants.TWSEBaseURL,
		limiter:    getTWSESharedLimiter(), // P1-13: shared TWSE bucket
		storageDir: storageDir,
	}
}

// SetHTTPClient overrides the HTTP client (for tests).
func (p *TWSEInsiderProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.client = client
	}
}

// Name returns the provider name.
func (p *TWSEInsiderProvider) Name() string {
	return "twse_insider"
}

// SetRateLimiter overrides the rate limiter (tests only; P1-13 shared-bucket
// tests use SetTWSESharedLimiterForTest instead).
func (p *TWSEInsiderProvider) SetRateLimiter(l *rate.Limiter) {
	if l != nil {
		p.limiter = l
	}
}

// FetchLatest fetches the most recent insider trading daily report and
// returns an aggregate summary.
func (p *TWSEInsiderProvider) FetchLatest(ctx context.Context) (*InsiderAggregate, error) {
	if err := p.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("insider rate limit: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, insiderOpenAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("insider create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("insider fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("insider HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("insider read body: %w", err)
	}

	records, err := parseInsiderJSON(body)
	if err != nil {
		return nil, fmt.Errorf("insider parse: %w", err)
	}

	agg := aggregateInsiderRecords(records)

	// Persist for audit.
	if p.storageDir != "" {
		date := agg.Date
		if date == "" {
			date = time.Now().Format("20060102")
		}
		if err := p.saveInsiderData(date, agg, records); err != nil {
			logging.Warn("twse_insider", "save_warning", logging.Err(err))
		}
	}

	return agg, nil
}

// ─── JSON parsing ──────────────────────────────────────────────────────

// insiderJSONRow mirrors the TWSE OpenAPI response row format.
type insiderJSONRow struct {
	Date       string `json:"出表日期"`
	StockID    string `json:"公司代號"`
	Company    string `json:"公司名稱"`
	Role       string `json:"申報人身分"`
	Name       string `json:"姓名"`
	TransType  string `json:"預定轉讓方式及股數-轉讓方式"`
	TransShare string `json:"預定轉讓方式及股數-轉讓股數"`
	MaxDaily   string `json:"每日於盤中交易最大得轉讓股數"`
	Receiver   string `json:"受讓人"`
	HeldOwn    string `json:"目前持有股數-自有持股"`
	HeldTrust  string `json:"目前持有股數-保留運用決定權信託股數"`
}

func parseInsiderJSON(body []byte) ([]InsiderReading, error) {
	// TWSE OpenAPI returns either a JSON array or a JSON object with a data array.
	// Try array first.
	var rows []insiderJSONRow
	if err := json.Unmarshal(body, &rows); err == nil {
		if len(rows) == 0 {
			return nil, nil // legitimate: no filings today
		}
		return convertInsiderRows(rows), nil
	}

	// Try wrapped format (some TWSE endpoints use this).
	var wrapper struct {
		Data []insiderJSONRow `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && len(wrapper.Data) > 0 {
		return convertInsiderRows(wrapper.Data), nil
	}

	return nil, fmt.Errorf("insider: unable to parse response as JSON array or wrapped object")
}

func convertInsiderRows(rows []insiderJSONRow) []InsiderReading {
	records := make([]InsiderReading, 0, len(rows))
	for _, r := range rows {
		shares := parseTWSEInt(strings.TrimSpace(r.TransShare))
		held := parseTWSEInt(strings.TrimSpace(r.HeldOwn))
		if shares == 0 && held == 0 {
			continue // skip rows with no data
		}
		records = append(records, InsiderReading{
			Date:         normalizeROCYearDate(strings.TrimSpace(r.Date)),
			StockID:      strings.TrimSpace(r.StockID),
			CompanyName:  strings.TrimSpace(r.Company),
			Role:         strings.TrimSpace(r.Role),
			Name:         strings.TrimSpace(r.Name),
			TransferType: strings.TrimSpace(r.TransType),
			Shares:       shares,
			HeldShares:   held,
		})
	}
	return records
}

// normalizeROCYearDate converts a TWSE ROC year date string (YYYMMDD, e.g. "1150724")
// to Gregorian year YYYYMMDD (e.g. "20260724"). Returns the input unchanged if
// it doesn't match the 7-digit ROC pattern.
func normalizeROCYearDate(roc string) string {
	s := strings.TrimSpace(roc)
	if len(s) != 7 {
		return s // already gregorian or malformed, pass through
	}
	rocYear, err := strconv.Atoi(s[:3])
	if err != nil {
		return s
	}
	return fmt.Sprintf("%04d%s", rocYear+1911, s[3:])
}

// ─── Aggregation ───────────────────────────────────────────────────────

func aggregateInsiderRecords(records []InsiderReading) *InsiderAggregate {
	agg := &InsiderAggregate{}
	if len(records) == 0 {
		return agg
	}

	agg.Date = records[0].Date
	stockSet := make(map[string]bool)
	roleCount := make(map[string]int)
	sellCount := 0
	buyCount := 0

	for _, r := range records {
		agg.TotalDeclared += r.Shares
		stockSet[r.StockID] = true
		roleCount[r.Role]++

		// Net sentiment: shares declared for transfer = net selling signal.
		// The "預定轉讓" is a pre-declaration of intent to sell.
		// We count each distinct insider as one "selling" signal.
		// In future, we could also track buy-side from 受讓人 (receiver) data.
		sellCount++
	}

	agg.TotalStocks = len(stockSet)
	agg.TotalInsiders = sellCount + buyCount
	agg.NetSentiment = buyCount - sellCount // negative = net selling

	// Dominant role
	maxCount := 0
	for role, count := range roleCount {
		if count > maxCount {
			maxCount = count
			agg.DominantRole = role
		}
	}

	return agg
}

// ─── Persistence ───────────────────────────────────────────────────────

func (p *TWSEInsiderProvider) saveInsiderData(date string, agg *InsiderAggregate, records []InsiderReading) error {
	payload := struct {
		Date      string            `json:"date"`
		Aggregate *InsiderAggregate `json:"aggregate"`
		Records   []InsiderReading  `json:"records"`
	}{
		Date:      date,
		Aggregate: agg,
		Records:   records,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	path := fmt.Sprintf("%s/%s_insider.json", p.storageDir, date)
	return os.WriteFile(path, data, 0o644)
}
