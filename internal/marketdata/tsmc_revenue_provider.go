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
	"strings"
	"time"
)

// TSMCRevenue holds monthly revenue data for TSMC.
type TSMCRevenue struct {
	Date         string  `json:"date"`
	Revenue      float64 `json:"revenue"`        // billions TWD
	YoYChangePct float64 `json:"yoy_change_pct"` // year-over-year change %
}

// TSMCRevenueProvider fetches TSMC monthly revenue from TWSE OpenAPI.
// TSMC revenue is the most direct proxy for AI capex spending in Taiwan.
type TSMCRevenueProvider struct {
	client     *http.Client
	storageDir string
	baseURL    string // kept for test compatibility; not used in production
}

// NewTSMCRevenueProvider creates a new TSMC revenue provider.
func NewTSMCRevenueProvider(storageDir string) *TSMCRevenueProvider {
	return &TSMCRevenueProvider{
		client:     &http.Client{Timeout: 20 * time.Second},
		storageDir: storageDir,
	}
}

// Name returns the provider name.
func (t *TSMCRevenueProvider) Name() string {
	return "tsmc_revenue"
}

// FetchSnapshot retrieves TSMC monthly revenue and returns a MacroDataSnapshot.
func (t *TSMCRevenueProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	rev, err := t.fetchLatestMonth(ctx)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("TSMC revenue fetch: %w", err)
	}

	if err := t.saveRevenue(rev); err != nil {
		log.Printf("[TSMCRevenueProvider] saveRevenue warning: %v", err)
	}

	revTime, _ := time.ParseInLocation("20060102", rev.Date+"01", time.FixedZone("CST", 8*60*60))
	revTs := revTime.Unix()

	snap := MacroDataSnapshot{
		RecordedAt: revTs,
	}
	snap.TSMCRevenue = MacroDataPoint{
		Symbol:    "TSMC_REVENUE",
		Value:     rev.Revenue,
		ChangePct: rev.YoYChangePct,
		Timestamp: revTs,
	}
	return snap, nil
}

func (t *TSMCRevenueProvider) fetchLatestMonth(ctx context.Context) (TSMCRevenue, error) {
	now := time.Now().UTC()
	for i := 0; i < 7; i++ {
		yearMonth := now.AddDate(0, -i, 0).Format("200601")
		rev, err := t.fetchMonth(ctx, yearMonth)
		if err == nil {
			return rev, nil
		}
		log.Printf("[TSMCRevenueProvider] month %s failed: %v", yearMonth, err)
	}
	return TSMCRevenue{}, fmt.Errorf("no TSMC revenue data available in the last 7 months")
}

func (t *TSMCRevenueProvider) fetchMonth(ctx context.Context, yearMonth string) (TSMCRevenue, error) {
	url := "https://openapi.twse.com.tw/v1/opendata/t187ap05_L"
	if t.baseURL != "" {
		url = t.baseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return TSMCRevenue{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return TSMCRevenue{}, fmt.Errorf("TSMC revenue HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TSMCRevenue{}, fmt.Errorf("TSMC revenue API HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TSMCRevenue{}, fmt.Errorf("TSMC revenue read body: %w", err)
	}

	var records []monthlyRevenueRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return TSMCRevenue{}, fmt.Errorf("TSMC revenue JSON parse: %w", err)
	}

	var tsmcRec monthlyRevenueRecord
	var found bool
	for _, r := range records {
		if r.CompanyCode == "2330" {
			tsmcRec = r
			found = true
			break
		}
	}
	if !found {
		return TSMCRevenue{}, fmt.Errorf("TSMC (2330) not found in monthly revenue list")
	}

	revenue := parseTWDVolume(tsmcRec.Revenue) / 1e6
	yoyChange := parseTWDVolume(tsmcRec.YoYChange)

	return TSMCRevenue{
		Date:         tsmcRec.YearMonth,
		Revenue:      revenue,
		YoYChangePct: yoyChange,
	}, nil
}

type monthlyRevenueRecord struct {
	ReportDate  string `json:"出表日期"`
	YearMonth   string `json:"資料年月"`
	CompanyCode string `json:"公司代號"`
	CompanyName string `json:"公司名稱"`
	Industry    string `json:"產業別"`
	Revenue     string `json:"營業收入-當月營收"`
	YoYChange   string `json:"營業收入-去年同月增減(%)"`
}

func (t *TSMCRevenueProvider) saveRevenue(rev TSMCRevenue) error {
	if err := os.MkdirAll(t.storageDir, 0o755); err != nil {
		return err
	}
	safeDate := strings.ReplaceAll(rev.Date, "/", "")
	path := filepath.Join(t.storageDir, safeDate+"_revenue.json")
	out, err := json.MarshalIndent(rev, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// TSMCRevenueProviderWithClient creates a provider with custom HTTP client (for testing).
func TSMCRevenueProviderWithClient(client *http.Client, storageDir string) *TSMCRevenueProvider {
	return &TSMCRevenueProvider{
		client:     client,
		storageDir: storageDir,
	}
}
