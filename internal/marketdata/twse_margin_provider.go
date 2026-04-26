package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type TWSEBalanceProvider struct {
	client     *http.Client
	storageDir string
}

type TWSEBalance struct {
	Date          string  `json:"date"`
	MarginBalance float64 `json:"margin_balance"`
	ShortBalance  float64 `json:"short_balance"`
	MarginBuy     float64 `json:"margin_buy"`
	MarginSell    float64 `json:"margin_sell"`
	MarginNet     float64 `json:"margin_net"`
	ShortSell     float64 `json:"short_sell"`
	ShortCover    float64 `json:"short_cover"`
	ShortNet      float64 `json:"short_net"`
}

func NewTWSEBalanceProvider(storageDir string) *TWSEBalanceProvider {
	return &TWSEBalanceProvider{
		client:     &http.Client{Timeout: 20 * time.Second},
		storageDir: storageDir,
	}
}

func (t *TWSEBalanceProvider) Name() string {
	return "twse_margin"
}

func (t *TWSEBalanceProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	bal, err := t.fetchLatestTradingDay(ctx)
	if err != nil {
		return MacroDataSnapshot{}, err
	}

	if err := t.saveBalance(bal); err != nil {
		log.Printf("[TWSEBalanceProvider] saveBalance warning: %v", err)
	}

	balTime, _ := time.ParseInLocation("20060102", bal.Date, time.FixedZone("CST", 8*60*60))
	balTs := balTime.Unix()

	snap := MacroDataSnapshot{
		RecordedAt: balTs,
	}
	snap.RetailMarginBalance = MacroDataPoint{
		Symbol:    "TAIWAN_MARGIN_BALANCE",
		Value:     bal.MarginBalance,
		ChangePct: 0,
		Timestamp: balTs,
	}
	return snap, nil
}

func (t *TWSEBalanceProvider) fetchLatestTradingDay(ctx context.Context) (TWSEBalance, error) {
	now := time.Now().UTC()
	for i := 0; i < 7; i++ {
		dateStr := now.AddDate(0, 0, -i).Format("20060102")
		bal, err := t.fetchDate(ctx, dateStr)
		if err == nil {
			return bal, nil
		}
	}
	return TWSEBalance{}, fmt.Errorf("no TWSE margin balance data available in the last 7 days")
}

func (t *TWSEBalanceProvider) fetchDate(ctx context.Context, dateStr string) (TWSEBalance, error) {
	url := "https://openapi.twse.com.tw/v1/exchangeReport/MI_MARGN"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return TWSEBalance{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return TWSEBalance{}, fmt.Errorf("TWSE MI_MARGN request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TWSEBalance{}, fmt.Errorf("TWSE MI_MARGN HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TWSEBalance{}, fmt.Errorf("TWSE MI_MARGN read body: %w", err)
	}

	// New OpenAPI v1 returns a direct array of objects with Chinese field names.
	var records []marginRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return TWSEBalance{}, fmt.Errorf("TWSE MI_MARGN JSON parse error: %w", err)
	}
	if len(records) == 0 {
		return TWSEBalance{}, fmt.Errorf("TWSE MI_MARGN returned empty data")
	}

	var totalMarginBalance float64
	var totalShortBalance float64

	for _, r := range records {
		marginBal := parseTWDVolume(r.MarginBalance)
		shortBal := parseTWDVolume(r.ShortBalance)
		totalMarginBalance += marginBal
		totalShortBalance += shortBal
	}

	bal := TWSEBalance{
		Date:          dateStr,
		MarginBalance: totalMarginBalance / 1e9,
		ShortBalance:  totalShortBalance,
	}
	return bal, nil
}

// marginRecord represents a single row from the TWSE OpenAPI v1 MI_MARGN endpoint.
// Fields use Chinese keys matching the API response.
type marginRecord struct {
	Symbol          string `json:"股票代號"`
	Name            string `json:"股票名稱"`
	MarginBuy       string `json:"融資買進"`
	MarginSell      string `json:"融資賣出"`
	MarginCashRepay string `json:"融資現金償還"`
	MarginPrevBal   string `json:"融資前日餘額"`
	MarginBalance   string `json:"融資今日餘額"`
	MarginLimit     string `json:"融資限額"`
	ShortBuy        string `json:"融券買進"`
	ShortSell       string `json:"融券賣出"`
	ShortCashRepay  string `json:"融券現券償還"`
	ShortPrevBal    string `json:"融券前日餘額"`
	ShortBalance    string `json:"融券今日餘額"`
	ShortLimit      string `json:"融券限額"`
	Offset          string `json:"資券互抵"`
	Note            string `json:"註記"`
}

func (t *TWSEBalanceProvider) saveBalance(bal TWSEBalance) error {
	if err := os.MkdirAll(t.storageDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(t.storageDir, bal.Date+"_margin.json")
	data, err := json.MarshalIndent(bal, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
