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
	url := fmt.Sprintf(
		"https://www.twse.com.tw/rwd/zh/marginTradingMiantane?response=json&date=%s&selectType=ALL",
		dateStr,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return TWSEBalance{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return TWSEBalance{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TWSEBalance{}, fmt.Errorf("TWSE API HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TWSEBalance{}, err
	}

	var apiResp twseMarginResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return TWSEBalance{}, fmt.Errorf("TWSE margin JSON parse error: %w", err)
	}
	if apiResp.Stat != "OK" || len(apiResp.Data) == 0 {
		return TWSEBalance{}, fmt.Errorf("TWSE margin API returned: %s", apiResp.Stat)
	}

	// TWSE T86 or T4 response columns vary; we aggregate total margin balance.
	// Column mapping for margin trading (T4 / Miantane):
	// 0: 證券代號, 1: 證券名稱,
	// 2: 融資餘額 (元), 3: 融資買進 (元), 4: 融資賣出 (元), 5: 融資買賣斷 (元),
	// 6: 融券餘額 (股), 7: 融券買進 (股), 8: 融券賣出 (股), 9: 融券買賣斷 (股)
	var totalMarginBalance float64
	var totalShortBalance float64

	for _, row := range apiResp.Data {
		if len(row) < 7 {
			continue
		}
		marginBal := parseTWDVolume(row[2])
		shortBal := parseTWDVolume(row[6])
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

type twseMarginResponse struct {
	Stat string     `json:"stat"`
	Data [][]string `json:"data"`
}
