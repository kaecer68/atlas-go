package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/logging"
	"golang.org/x/time/rate"
)

// TWSEMarginBalanceProvider fetches Taiwan margin balance data from TWSE.
type TWSEMarginBalanceProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
	storageDir  string
}

// NewTWSEMarginBalanceProvider creates a new TWSE margin balance provider.
// Pass an empty storageDir to skip saving margin data to disk.
func NewTWSEMarginBalanceProvider(storageDir string) *TWSEMarginBalanceProvider {
	return &TWSEMarginBalanceProvider{
		client:      httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:     "https://www.twse.com.tw",
		rateLimiter: rate.NewLimiter(rate.Every(5*time.Second), 1),
		storageDir:  storageDir,
	}
}

// Name returns the provider name.
func (t *TWSEMarginBalanceProvider) Name() string {
	return "twse_margin_balance"
}

// FetchSnapshot retrieves the latest margin balance data.
func (t *TWSEMarginBalanceProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	return t.FetchSnapshotForDate(ctx, time.Now().UTC())
}

// FetchSnapshotForDate retrieves margin balance data for a specific date.
func (t *TWSEMarginBalanceProvider) FetchSnapshotForDate(ctx context.Context, date time.Time) (MacroDataSnapshot, error) {
	for i := range 7 {
		dateStr := date.AddDate(0, 0, -i).Format("20060102")
		balance, shortBalance, changePct, shortChangePct, err := t.fetchDateExpanded(ctx, dateStr)
		if err == nil {
			if err := t.saveMargin(dateStr, balance, shortBalance, changePct, shortChangePct); err != nil {
				logging.Warn("twse_margin_provider", "save_margin_warning", logging.Err(err))
			}
			ts := time.Now().Unix()
			return MacroDataSnapshot{
				RetailMarginBalance: MacroDataPoint{
					Symbol:    "TAIWAN_MARGIN_BALANCE",
					Value:     balance,
					ChangePct: changePct,
					Timestamp: ts,
				},
				RetailShortBalance: MacroDataPoint{
					Symbol:    "TAIWAN_SHORT_BALANCE",
					Value:     shortBalance,
					ChangePct: shortChangePct,
					Timestamp: ts,
				},
				RecordedAt: ts,
			}, nil
		}
	}
	return MacroDataSnapshot{}, fmt.Errorf("no TWSE margin balance data available in the last 7 days")
}

func (t *TWSEMarginBalanceProvider) fetchDateExpanded(ctx context.Context, dateStr string) (float64, float64, float64, float64, error) {
	if err := t.rateLimiter.Wait(ctx); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/en/exchangeReport/MI_MARGN?response=json&date=%s&selectType=MS", t.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("read body: %w", err)
	}

	var apiResp twseMarginResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Stat != "OK" || len(apiResp.Tables) == 0 {
		return 0, 0, 0, 0, fmt.Errorf("TWSE API returned no data: stat=%s tables=%d", apiResp.Stat, len(apiResp.Tables))
	}

	var marginTable, shortTable *twseMarginTable
	for i := range apiResp.Tables {
		table := &apiResp.Tables[i]
		if marginTable == nil && tableHasField(table, "融資") {
			marginTable = table
		}
		if shortTable == nil && (tableHasField(table, "融券") || strings.Contains(table.Title, "融券")) {
			shortTable = table
		}
	}
	if marginTable == nil {
		marginTable = &apiResp.Tables[0]
	}

	balance, prevBalance, err := extractCurrentAndPreviousValue(*marginTable, "今日餘額", "昨日餘額", 5, 4)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	changePct := percentChange(balance, prevBalance)

	shortBalance, shortPrevBalance := 0.0, 0.0
	shortChangePct := 0.0
	if shortTable != nil {
		shortBalance, shortPrevBalance, err = extractCurrentAndPreviousValue(*shortTable, "今日餘額", "昨日餘額", 5, 4)
		if err == nil {
			shortChangePct = percentChange(shortBalance, shortPrevBalance)
		}
	}

	return balance, shortBalance, changePct, shortChangePct, nil
}

func (t *TWSEMarginBalanceProvider) saveMargin(dateStr string, balance, shortBalance, changePct, shortChangePct float64) error {
	if t.storageDir == "" {
		return nil
	}
	if err := os.MkdirAll(t.storageDir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data := map[string]interface{}{
		"date":             dateStr,
		"margin_balance":   balance,
		"short_balance":    shortBalance,
		"change_pct":       changePct,
		"short_change_pct": shortChangePct,
	}
	out, _ := json.MarshalIndent(data, "", "  ")
	return os.WriteFile(filepath.Join(t.storageDir, dateStr+"_margin.json"), out, 0o644)
}

func tableHasField(table *twseMarginTable, fieldName string) bool {
	for _, field := range table.Fields {
		if strings.Contains(field, fieldName) {
			return true
		}
	}
	return strings.Contains(table.Title, fieldName)
}

func extractValueByFieldName(table twseMarginTable, fieldName string, fallbackIdx int) (string, bool) {
	if len(table.Data) < 3 {
		return "", false
	}
	for i, field := range table.Fields {
		if strings.Contains(field, fieldName) && i < len(table.Data[2]) {
			return table.Data[2][i], true
		}
	}
	if fallbackIdx >= 0 && fallbackIdx < len(table.Data[2]) {
		return table.Data[2][fallbackIdx], true
	}
	return "", false
}

func extractCurrentAndPreviousValue(table twseMarginTable, currentFieldName, prevFieldName string, currentFallbackIdx, prevFallbackIdx int) (float64, float64, error) {
	currentRaw, ok := extractValueByFieldName(table, currentFieldName, currentFallbackIdx)
	if !ok {
		return 0, 0, fmt.Errorf("TWSE API missing current value for %s", currentFieldName)
	}
	prevRaw, ok := extractValueByFieldName(table, prevFieldName, prevFallbackIdx)
	if !ok {
		return 0, 0, fmt.Errorf("TWSE API missing previous value for %s", prevFieldName)
	}
	return float64(parseTWSEInt(currentRaw)) / 1e5, float64(parseTWSEInt(prevRaw)) / 1e5, nil
}

func percentChange(current, previous float64) float64 {
	if previous == 0 {
		return 0
	}
	return (current - previous) / previous * 100
}

type twseMarginResponse struct {
	Stat   string            `json:"stat"`
	Date   string            `json:"date"`
	Tables []twseMarginTable `json:"tables"`
}

type twseMarginTable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
	Total  int        `json:"total"`
}
