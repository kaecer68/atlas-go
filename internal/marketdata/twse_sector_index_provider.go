package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
	"golang.org/x/time/rate"
)

// SectorIndexData holds a single day's index value for an industry.
type SectorIndexData struct {
	Date      string  `json:"date"`       // YYYY-MM-DD
	Industry  string  `json:"industry"`   // industry ID
	Index     float64 `json:"index"`      // index value
	ReturnPct float64 `json:"return_pct"` // daily return %
}

// TWSESectorIndexProvider fetches Taiwan industry index data from TWSE.
// Industry indices are used for empirical correlation calibration between sectors.
type TWSESectorIndexProvider struct {
	client     *http.Client
	baseURL    string
	limiter    *rate.Limiter
	cacheDir   string
	industries map[string]string // industry ID -> TWSE industry code
}

// NewTWSESectorIndexProvider creates a new TWSE sector index provider.
func NewTWSESectorIndexProvider(cacheDir string) *TWSESectorIndexProvider {
	params := config.GetParametersConfig()
	timeoutSec := 30
	apiRate := 0.2 // 1 req per 5 seconds default
	burst := 1
	if params != nil {
		timeoutSec = params.Marketdata.TWSEAPITimeoutSec.Value
		apiRate = params.Marketdata.TWSEAPIRateLimit.Value
		burst = params.Marketdata.TWSEAPIRateBurst.Value
	}

	return &TWSESectorIndexProvider{
		client:   httpclient.NewFactory().NewClient(time.Duration(timeoutSec) * time.Second),
		baseURL:  "https://www.twse.com.tw",
		limiter:  rate.NewLimiter(rate.Limit(apiRate), burst),
		cacheDir: cacheDir,
		industries: map[string]string{
			"semiconductor":   "XX", // Placeholder - TWSE industry codes needed
			"electronics":     "XX",
			"shipping":        "XX",
			"financials":      "XX",
			"energy":          "XX",
			"ai_supply_chain": "XX",
			"robotics":        "XX",
		},
	}
}

// Name returns the provider name.
func (p *TWSESectorIndexProvider) Name() string {
	return "twse_sector_index"
}

// FetchSectorIndices fetches historical industry index data for the given date range.
// Returns a map of industry ID -> sorted daily index data.
func (p *TWSESectorIndexProvider) FetchSectorIndices(ctx context.Context, startDate, endDate time.Time) (map[string][]SectorIndexData, error) {
	if p.cacheDir != "" {
		if cached, err := p.loadFromCache(startDate, endDate); err == nil && len(cached) > 0 {
			logging.Info("marketdata", "sector_index_cache_hit",
				logging.FStr("start", startDate.Format("2006-01-02")),
				logging.FStr("end", endDate.Format("2006-01-02")))
			return cached, nil
		}
	}

	result := make(map[string][]SectorIndexData)
	current := startDate
	for !current.After(endDate) {
		if err := p.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait: %w", err)
		}

		dailyData, err := p.fetchSingleDay(ctx, current)
		if err != nil {
			logging.Warn("marketdata", "sector_index_fetch_failed",
				logging.FStr("date", current.Format("2006-01-02")),
				logging.Err(err))
			current = current.AddDate(0, 0, 1)
			continue
		}

		for industry, data := range dailyData {
			result[industry] = append(result[industry], data)
		}

		current = current.AddDate(0, 0, 1)
	}

	if p.cacheDir != "" {
		if err := p.saveToCache(result, startDate, endDate); err != nil {
			logging.Warn("marketdata", "sector_index_cache_save_failed", logging.Err(err))
		}
	}

	return result, nil
}

// fetchSingleDay fetches industry index data for a single trading day.
func (p *TWSESectorIndexProvider) fetchSingleDay(ctx context.Context, date time.Time) (map[string]SectorIndexData, error) {
	dateStr := date.Format("20060102")
	endpoint := fmt.Sprintf("%s/rwd/en/afterTrading/MI_INDEX?date=%s&type=MS&response=json", p.baseURL, dateStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error: status %d", resp.StatusCode)
	}

	var apiResp struct {
		Stat   string     `json:"stat"`
		Date   string     `json:"date"`
		Title  string     `json:"title"`
		Fields []string   `json:"fields"`
		Data   [][]string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if apiResp.Stat != "OK" {
		return nil, fmt.Errorf("api stat not OK: %s", apiResp.Stat)
	}

	result := make(map[string]SectorIndexData)
	dateFormatted := date.Format("2006-01-02")

	for _, row := range apiResp.Data {
		if len(row) < 2 {
			continue
		}
		industryName := strings.TrimSpace(row[0])
		indexValue, err := strconv.ParseFloat(strings.ReplaceAll(row[1], ",", ""), 64)
		if err != nil {
			continue
		}

		industryID := p.mapIndustryName(industryName)
		if industryID == "" {
			continue
		}

		result[industryID] = SectorIndexData{
			Date:     dateFormatted,
			Industry: industryID,
			Index:    indexValue,
		}
	}

	return result, nil
}

// mapIndustryName maps TWSE industry names to internal industry IDs.
func (p *TWSESectorIndexProvider) mapIndustryName(twseName string) string {
	mapping := map[string]string{
		"Semiconductors":              "semiconductor",
		"Electronic Parts/Components": "electronics",
		"Shipping and Transportation": "shipping",
		"Financial and Insurance":     "financials",
		"Oil, Gas and Electricity":    "energy",
		"Computer and Peripheral":     "ai_supply_chain",
		"Electric Machinery":          "robotics",
	}

	for twse, internal := range mapping {
		if strings.Contains(twseName, twse) {
			return internal
		}
	}
	return ""
}

// CalculateReturns computes daily returns from index values.
func (p *TWSESectorIndexProvider) CalculateReturns(data map[string][]SectorIndexData) map[string][]float64 {
	returns := make(map[string][]float64)

	for industry, indices := range data {
		if len(indices) < 2 {
			continue
		}

		industryReturns := make([]float64, 0, len(indices)-1)
		for i := 1; i < len(indices); i++ {
			prev := indices[i-1].Index
			curr := indices[i].Index
			if prev > 0 {
				dailyReturn := (curr - prev) / prev
				industryReturns = append(industryReturns, dailyReturn)
				indices[i].ReturnPct = dailyReturn * 100
			}
		}
		returns[industry] = industryReturns
	}

	return returns
}

// cacheFilePath returns the cache file path for a date range.
func (p *TWSESectorIndexProvider) cacheFilePath(startDate, endDate time.Time) string {
	filename := fmt.Sprintf("sector_indices_%s_%s.json",
		startDate.Format("20060102"),
		endDate.Format("20060102"))
	return filepath.Join(p.cacheDir, filename)
}

// loadFromCache loads sector index data from disk cache.
func (p *TWSESectorIndexProvider) loadFromCache(startDate, endDate time.Time) (map[string][]SectorIndexData, error) {
	path := p.cacheFilePath(startDate, endDate)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cache: %w", err)
	}

	var cached map[string][]SectorIndexData
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("decode cache: %w", err)
	}

	return cached, nil
}

// saveToCache saves sector index data to disk cache.
func (p *TWSESectorIndexProvider) saveToCache(data map[string][]SectorIndexData, startDate, endDate time.Time) error {
	if err := os.MkdirAll(p.cacheDir, 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	path := p.cacheFilePath(startDate, endDate)
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cache: %w", err)
	}

	if err := os.WriteFile(path, encoded, 0644); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}

	return nil
}
