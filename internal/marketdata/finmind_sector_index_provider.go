package marketdata

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// FinMindSectorIndexProvider fetches Taiwan industry index daily closes from
// the FinMind "TaiwanStockEvery5SecondsIndex" dataset (每5秒指數統計 / 台灣類股股價表).
//
// Background (D08, #1740 Phase 1 Road 1 §2.3): TWSE OpenAPI MI_INDEX ignores
// the date parameter (latest-only, G2), so 2021 sector history is unreachable
// through the TWSE channel; the 33 polluted per-day files were deleted by
// #1752 (110 → 77). FinMind's 5-second index table carries the TWSE/TPEx
// industry index series back to 2005-01-03 (Backer/Sponsor tier). The last
// print of the session (13:30:00) is the official daily close.
//
// Fetch strategy: one request per trading day WITHOUT data_id returns the
// whole market (every index series) for that day — ~178k 5-second rows.
// Only kind=twse rows are kept; the 18 English series names are mapped
// one-to-one onto the canonical L1 SectorID set
// (SectorIndexReader.canonicalSectorIDs, internal/industry/sector.go).
// Daily returns are computed pairwise from consecutive trading-day closes.
//
// Rate limit: goes through the shared FinMindClient token bucket
// (Free 600/hr, Sponsor 6000/hr — see finmind_client.go), so a full 2021
// backfill (~250 requests) stays far below either ceiling.
type FinMindSectorIndexProvider struct {
	client *FinMindClient

	mu sync.Mutex
	// closesByDate caches whole-day close maps (canonical industry ID →
	// close) keyed by YYYY-MM-DD, including empty maps for non-trading
	// days, so the previous-trading-day lookup never re-fetches the API.
	closesByDate map[string]map[string]float64
}

// finmindSectorIndexDataset is the FinMind dataset carrying the TWSE/TPEx
// industry index series (每5秒指數統計). Single day per request; each row is a
// 5-second print: date, time, stock_id (English series name), price, kind
// (twse | tpex).
const finmindSectorIndexDataset = "TaiwanStockEvery5SecondsIndex"

// finmindSectorSeries maps the FinMind English series names (kind=twse) to the
// canonical L1 SectorID set. One source series per canonical ID, matching the
// 18-ID universe in SectorIndexReader.canonicalSectorIDs. Series not listed
// here (TAIEX, TPExIndex, sub-indices, 玻璃陶瓷/造紙/橡膠/觀光/文化創意 etc.) are
// intentionally dropped — they are not part of the canonical 18.
var finmindSectorSeries = map[string]string{
	"Automobile":                   "auto",
	"BiotechnologyMedicalCare":     "biotech",
	"Cement":                       "cement",
	"BuildingMaterialConstruction": "construction",
	"Electronic":                   "electronics",
	"OilGasElectricity":            "energy",
	"FinancialInsurance":           "financials",
	"Food":                         "food",
	"ElectricMachinery":            "machinery",
	"Optoelectronic":               "optoelectronics",
	"OtherElectronic":              "other_electronics",
	"Plastics":                     "plastics",
	"TradingConsumersGoods":        "retail",
	"Semiconductor":                "semiconductor",
	"ShippingTransportation":       "shipping",
	"IronSteel":                    "steel",
	"CommunicationsInternet":       "telecom",
	"Textiles":                     "textiles",
}

// finmindSectorPrevLookbackDays bounds how far back the provider walks to find
// the previous trading day's close (covers Lunar New Year closures + weekends).
const finmindSectorPrevLookbackDays = 14

// NewFinMindSectorIndexProvider creates a provider on the shared FinMind
// client (single token bucket + daily quota gate across all call sites).
func NewFinMindSectorIndexProvider(apiKey string) *FinMindSectorIndexProvider {
	return &FinMindSectorIndexProvider{
		client:       GetSharedFinMindClient(apiKey),
		closesByDate: make(map[string]map[string]float64),
	}
}

// NewFinMindSectorIndexProviderWithClient creates a provider on a caller-owned
// client (tests). Use NewFinMindSectorIndexProvider in production.
func NewFinMindSectorIndexProviderWithClient(client *FinMindClient) *FinMindSectorIndexProvider {
	return &FinMindSectorIndexProvider{
		client:       client,
		closesByDate: make(map[string]map[string]float64),
	}
}

// Name returns the provider name.
func (p *FinMindSectorIndexProvider) Name() string {
	return "finmind_sector_index"
}

// FetchSectorIndices returns daily sector index data for every trading day in
// [startDate, endDate] as map[canonical industry][]SectorIndexData (one entry
// per industry per trading day). Non-trading days are skipped. This satisfies
// backfill.SectorIndexFetcher so the shared backfill wrapper can write
// per-day files.
func (p *FinMindSectorIndexProvider) FetchSectorIndices(ctx context.Context, startDate, endDate time.Time) (map[string][]SectorIndexData, error) {
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		return nil, fmt.Errorf("load Asia/Taipei: %w", err)
	}

	result := make(map[string][]SectorIndexData)
	current := startDate
	for !current.After(endDate) {
		day := current.In(loc)
		dayStr := day.Format("2006-01-02")

		closes, err := p.dayCloses(ctx, day)
		if err != nil {
			return nil, fmt.Errorf("finmind sector index %s: %w", dayStr, err)
		}
		if len(closes) == 0 {
			// Non-trading day (weekend / holiday) — FinMind returns an
			// empty data array; skip.
			current = current.AddDate(0, 0, 1)
			continue
		}

		prev, err := p.previousTradingDayCloses(ctx, day)
		if err != nil {
			return nil, fmt.Errorf("finmind sector index prev-day %s: %w", dayStr, err)
		}

		for industry, idx := range closes {
			item := SectorIndexData{
				Date:     dayStr,
				Industry: industry,
				Index:    idx,
			}
			if prev != nil {
				if pc, ok := prev[industry]; ok && pc > 0 {
					item.ReturnPct = (idx/pc - 1) * 100
				}
			}
			result[industry] = append(result[industry], item)
		}

		current = current.AddDate(0, 0, 1)
	}
	return result, nil
}

// dayCloses fetches the whole-day 5-second index table for day (one FinMind
// request), keeps the last print per twse series, maps series names to
// canonical IDs, and caches the result (empty map for non-trading days).
func (p *FinMindSectorIndexProvider) dayCloses(ctx context.Context, day time.Time) (map[string]float64, error) {
	dayStr := day.Format("2006-01-02")

	p.mu.Lock()
	if cached, ok := p.closesByDate[dayStr]; ok {
		p.mu.Unlock()
		return cached, nil
	}
	p.mu.Unlock()

	rows, err := p.client.fetchDataset(ctx, finmindSectorIndexDataset, "", dayStr, dayStr)
	if err != nil {
		return nil, err
	}

	closes := make(map[string]float64)
	lastTime := make(map[string]string) // canonical ID → latest time seen
	for _, row := range rows {
		kind, _ := row["kind"].(string)
		if kind != "twse" {
			continue
		}
		stockID, _ := row["stock_id"].(string)
		industry, ok := finmindSectorSeries[stockID]
		if !ok {
			continue
		}
		t, _ := row["time"].(string)
		if prev, seen := lastTime[industry]; seen && t < prev {
			continue
		}
		price, ok := row["price"].(float64)
		if !ok || price <= 0 {
			continue
		}
		lastTime[industry] = t
		closes[industry] = price
	}

	// 空結果 = 非交易日（假日），不 warn；0 < got < 18 才是真異常（交易日缺產業）。
	if len(closes) > 0 && len(closes) != len(finmindSectorSeries) {
		logging.Warn("marketdata", "finmind_sector_series_partial",
			"date", dayStr,
			"got", len(closes),
			"want", len(finmindSectorSeries))
	}

	p.mu.Lock()
	p.closesByDate[dayStr] = closes
	p.mu.Unlock()
	return closes, nil
}

// previousTradingDayCloses returns the most recent cached close map strictly
// before day. When the cache has nothing yet (cold start), it walks back up to
// finmindSectorPrevLookbackDays calendar days fetching and caching each day
// until a trading day is found.
func (p *FinMindSectorIndexProvider) previousTradingDayCloses(ctx context.Context, day time.Time) (map[string]float64, error) {
	dayStr := day.Format("2006-01-02")
	if prev := p.cachedClosesBefore(dayStr); prev != nil {
		return prev, nil
	}
	cursor := day.AddDate(0, 0, -1)
	for i := 0; i < finmindSectorPrevLookbackDays; i++ {
		closes, err := p.dayCloses(ctx, cursor)
		if err != nil {
			return nil, err
		}
		if len(closes) > 0 {
			return closes, nil
		}
		cursor = cursor.AddDate(0, 0, -1)
	}
	return nil, nil
}

// cachedClosesBefore returns the most recent cached non-empty close map for a
// date strictly before dayStr, or nil when none exists.
func (p *FinMindSectorIndexProvider) cachedClosesBefore(dayStr string) map[string]float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	var best string
	var bestCloses map[string]float64
	for date, closes := range p.closesByDate {
		if len(closes) == 0 {
			continue
		}
		if date < dayStr && date > best {
			best = date
			bestCloses = closes
		}
	}
	return bestCloses
}
