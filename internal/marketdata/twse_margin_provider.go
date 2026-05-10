package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// TWSEMarginBalanceProvider fetches Taiwan margin balance data from TWSE.
type TWSEMarginBalanceProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// NewTWSEMarginBalanceProvider creates a new TWSE margin balance provider.
func NewTWSEMarginBalanceProvider() *TWSEMarginBalanceProvider {
	return &TWSEMarginBalanceProvider{
		client:      &http.Client{Timeout: 20 * time.Second},
		baseURL:     "https://www.twse.com.tw",
		rateLimiter: rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// Name returns the provider name.
func (t *TWSEMarginBalanceProvider) Name() string {
	return "twse_margin_balance"
}

// FetchSnapshot retrieves the latest margin balance data.
func (t *TWSEMarginBalanceProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	now := time.Now().UTC()
	for i := range 7 {
		dateStr := now.AddDate(0, 0, -i).Format("20060102")
		balance, changePct, err := t.fetchDate(ctx, dateStr)
		if err == nil {
			ts := time.Now().Unix()
			return MacroDataSnapshot{
				RetailMarginBalance: MacroDataPoint{
					Symbol:    "TAIWAN_MARGIN_BALANCE",
					Value:     balance,
					ChangePct: changePct,
					Timestamp: ts,
				},
				RecordedAt: ts,
			}, nil
		}
	}
	return MacroDataSnapshot{}, fmt.Errorf("no TWSE margin balance data available in the last 7 days")
}

func (t *TWSEMarginBalanceProvider) fetchDate(ctx context.Context, dateStr string) (float64, float64, error) {
	if err := t.rateLimiter.Wait(ctx); err != nil {
		return 0, 0, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/en/exchangeReport/MI_MARGN?response=json&date=%s&selectType=MS", t.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, fmt.Errorf("read body: %w", err)
	}

	var apiResp twseMarginResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return 0, 0, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Stat != "OK" || len(apiResp.Tables) == 0 {
		return 0, 0, fmt.Errorf("TWSE API returned no data: stat=%s tables=%d", apiResp.Stat, len(apiResp.Tables))
	}

	marketTable := apiResp.Tables[0]
	if len(marketTable.Data) == 0 {
		return 0, 0, fmt.Errorf("TWSE API returned empty market data")
	}

	if len(marketTable.Data) < 3 {
		return 0, 0, fmt.Errorf("TWSE API returned insufficient data rows: %d", len(marketTable.Data))
	}

	valueRow := marketTable.Data[2]
	if len(valueRow) < 6 {
		return 0, 0, fmt.Errorf("TWSE API returned incomplete value row: %d fields", len(valueRow))
	}

	balance := float64(parseTWSEInt(valueRow[5])) / 1e5
	prevBalance := float64(parseTWSEInt(valueRow[4])) / 1e5
	
	changePct := 0.0
	if prevBalance > 0 {
		changePct = (balance - prevBalance) / prevBalance * 100
	}

	return balance, changePct, nil
}

type twseMarginResponse struct {
	Stat   string                `json:"stat"`
	Date   string                `json:"date"`
	Tables []twseMarginTable     `json:"tables"`
}

type twseMarginTable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
	Total  int        `json:"total"`
}
