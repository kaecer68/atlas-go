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

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
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
	client   *http.Client
	baseURL  string
	limiter  *rate.Limiter
	cacheDir string
}

// NewTWSESectorIndexProvider creates a new TWSE sector index provider.
func NewTWSESectorIndexProvider(cacheDir string) *TWSESectorIndexProvider {
	params := config.GetParametersConfig()
	timeoutSec := 30
	if params != nil {
		timeoutSec = params.Marketdata.TWSEAPITimeoutSec.Value
	}

	return &TWSESectorIndexProvider{
		client:   httpclient.NewFactory().NewClient(time.Duration(timeoutSec) * time.Second),
		baseURL:  "https://openapi.twse.com.tw/v1",
		limiter:  getTWSESharedLimiter(), // P1-13: shared TWSE bucket
		cacheDir: cacheDir,
	}
}

// SetHTTPClient sets a custom HTTP client for tests.
func (p *TWSESectorIndexProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.client = client
	}
}

// Name returns the provider name.
func (p *TWSESectorIndexProvider) Name() string {
	return "twse_sector_index"
}
// SetRateLimiter overrides the rate limiter (tests only; P1-13 shared-bucket
// tests use SetTWSESharedLimiterForTest instead).
func (p *TWSESectorIndexProvider) SetRateLimiter(l *rate.Limiter) {
	if l != nil {
		p.limiter = l
	}
}


// FetchSectorIndices fetches historical industry index data for the given date range.
// Returns a map of industry ID -> sorted daily index data.
// This method uses the legacy correlation-calibration mapping; for canonical L1
// sector data use FetchL1SectorIndices.
func (p *TWSESectorIndexProvider) FetchSectorIndices(ctx context.Context, startDate, endDate time.Time) (map[string][]SectorIndexData, error) {
	return p.fetchWithMapper(ctx, startDate, endDate, p.mapIndustryName)
}

// FetchL1SectorIndices is like FetchSectorIndices but maps TWSE industry names to
// the canonical L1 SectorID set defined in internal/industry/sector.go. Use this
// for C07 per-sector direction prediction and any other consumer that needs a
// stable 20-sector universe.
func (p *TWSESectorIndexProvider) FetchL1SectorIndices(ctx context.Context, startDate, endDate time.Time) (map[string][]SectorIndexData, error) {
	return p.fetchWithMapper(ctx, startDate, endDate, p.canonicalL1SectorID)
}

func (p *TWSESectorIndexProvider) fetchWithMapper(ctx context.Context, startDate, endDate time.Time, mapper func(string) string) (map[string][]SectorIndexData, error) {
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

		dailyData, err := p.fetchSingleDay(ctx, current, mapper)
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

type twseIndexItem struct {
	Index      string `json:"指數"`
	CloseIndex string `json:"收盤指數"`
	Change     string `json:"漲跌"`
	ChangePts  string `json:"漲跌點數"`
	ChangePct  string `json:"漲跌百分比"`
}

// fetchSingleDay fetches industry index data for a single trading day.
func (p *TWSESectorIndexProvider) fetchSingleDay(ctx context.Context, date time.Time, mapper func(string) string) (map[string]SectorIndexData, error) {
	dateStr := date.Format("20060102")
	endpoint := fmt.Sprintf("%s/exchangeReport/MI_INDEX?date=%s&type=MS&response=json", p.baseURL, dateStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error: status %d", resp.StatusCode)
	}

	var apiResp []twseIndexItem
	if err := DecodeJSON(resp.Body, resp.Header.Get("Content-Type"), &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	result := make(map[string]SectorIndexData)
	dateFormatted := date.Format("2006-01-02")

	for _, item := range apiResp {
		industryName := strings.TrimSpace(item.Index)
		closeIndex := strings.TrimSpace(item.CloseIndex)

		if closeIndex == "" {
			continue
		}
		indexValue, err := strconv.ParseFloat(strings.ReplaceAll(closeIndex, ",", ""), 64)
		if err != nil {
			continue
		}

		if indexValue == 0 {
			continue
		}

		industryID := mapper(industryName)
		if industryID == "" {
			continue
		}

		// Parse daily change percentage from TWSE response (e.g., "0.62" or "-0.43").
		changePct, _ := strconv.ParseFloat(strings.TrimSpace(item.ChangePct), 64)

		result[industryID] = SectorIndexData{
			Date:      dateFormatted,
			Industry:  industryID,
			Index:     indexValue,
			ReturnPct: changePct,
		}
	}

	return result, nil
}

// mapIndustryName maps TWSE OpenAPI v1 industry names (Chinese) to canonical SectorID strings.
// This mapping is preserved for backward compatibility with the original correlation-calibration
// consumer that expected IDs such as "ai_supply_chain" and "robotics".
func (p *TWSESectorIndexProvider) mapIndustryName(twseName string) string {
	mapping := map[string]string{
		"半導體類指數":     "semiconductor",
		"電腦及週邊設備類指數": "ai_supply_chain",
		"電子零組件類指數":   "electronics",
		"其他電子類指數":    "other_electronics",
		"航運類指數":      "shipping",
		"金融保險類指數":    "financials",
		"油電燃氣類指數":    "energy",
		"電機機械類指數":    "robotics",
	}

	if id, ok := mapping[twseName]; ok {
		return id
	}
	return ""
}

// canonicalL1SectorID maps TWSE OpenAPI v1 industry names to the canonical L1 SectorID
// set defined in internal/industry/sector.go. This is the stable 20-sector universe used
// by C07 per-sector direction prediction. Unrecognized TWSE names are dropped.
func (p *TWSESectorIndexProvider) canonicalL1SectorID(twseName string) string {
	mapping := map[string]string{
		"半導體類指數":     "semiconductor",
		"電腦及週邊設備類指數": "electronics",
		"電子零組件類指數":   "electronics",
		"其他電子類指數":    "other_electronics",
		"光電類指數":      "optoelectronics",
		"通信網路類指數":    "telecom",
		"航運類指數":      "shipping",
		"金融保險類指數":    "financials",
		"油電燃氣類指數":    "energy",
		"電機機械類指數":    "machinery",
		"電器電纜類指數":    "machinery",
		"水泥類指數":      "cement",
		"食品類指數":      "food",
		"塑膠類指數":      "plastics",
		"紡織纖維類指數":    "textiles",
		"鋼鐵類指數":      "steel",
		"汽車類指數":      "auto",
		"化學工業類指數":    "chemicals",
		"生技醫療類指數":    "biotech",
		"建材營造類指數":    "construction",
		"觀光類指數":      "tourism",
		"貿易百貨類指數":    "retail",
	}

	if id, ok := mapping[twseName]; ok {
		return id
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

		industryReturns := make([]float64, 0, max(0, len(indices)-1))
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
	if err := os.MkdirAll(p.cacheDir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	path := p.cacheFilePath(startDate, endDate)
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cache: %w", err)
	}

	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}

	return nil
}
