package marketdata

import (
	"context"
	"fmt"
	"strings"
)

// FinMindFuturesInstitutionalProvider fetches 期貨三大法人買賣 (per-trader volume
// and open interest) from the FinMind TaiwanFuturesInstitutionalInvestors
// dataset.
//
// Background (D07, #1740 Road 1 §2.7-2.8): the TAIFEX OpenAPI institutional
// endpoint has no date parameter (it only serves the latest session) and the
// website CSV (futContractsDateDown) rejects dates <= 2023 with
// "日期時間錯誤 DateTime error". FinMind's TaiwanFuturesInstitutionalInvestors
// dataset is the only Free/Sponsor path to historical 三大法人 期貨 OI for
// 2021 (Free tier 已含 期貨三大法人買賣; Sponsor tier 亦含).
//
// Rate limit contract: the underlying FinMindClient owns the token bucket.
// Free tier = 600 req/hr (finmindRateLimit), Sponsor tier = 6000 req/hr.
// This provider adds NO second limiter (avoids the double-wait trap the
// taifex adapters had); backfill callers must keep total requests under the
// tier ceiling. A full-year range fetch is ONE API call (FinMind returns all
// dates in the range), so 2021 TX backfill = 1 call, far below 6000/hr.
//
// Data shape (verified live 2026-08-30): one row per (date, futures_id,
// institutional_investors). institutional_investors ∈ {自營商, 投信, 外資}
// (note: FinMind uses "外資" where TAIFEX OpenAPI uses "外資及陸資" — the
// provider maps both to TraderSide.Foreign). Amount fields are NT$ values;
// volume/OI fields are 口數 (contract counts, signed).
type FinMindFuturesInstitutionalProvider struct {
	client *FinMindClient
}

// finmindFuturesDataset is the FinMind dataset name for 期貨三大法人買賣.
// Do NOT use TaiwanFuturesInstitutionalTraders — that name is not in the
// FinMind v4 enum (422 at the API; verified 2026-08-30).
const finmindFuturesDataset = "TaiwanFuturesInstitutionalInvestors"

// NewFinMindFuturesInstitutionalProvider creates a provider backed by the
// given FinMindClient (typically GetSharedFinMindClient so the 600/6000 per
// hour bucket is shared across all FinMind consumers).
func NewFinMindFuturesInstitutionalProvider(client *FinMindClient) *FinMindFuturesInstitutionalProvider {
	return &FinMindFuturesInstitutionalProvider{client: client}
}

// FetchInstitutionalFuturesRange fetches per-trader volume + OI for one
// futures product over a date range (inclusive, YYYY-MM-DD). One FinMind API
// call returns every trading date in the range (verified: full 2021 TX = 732
// rows / 244 dates in a single response).
//
// futuresID is the FinMind futures_id (e.g. "TX", "MTX", "EXF", "FXF", "GTF",
// "TJF"). TAIFEX-website commodityIds are translated: TXF→TX (臺股期貨),
// MXF→MTX (小型臺指). Returns one InstitutionalFuturesDaily per trading date
// that has complete 外資/投信/自營商 rows; dates with no rows (holidays,
// products not yet listed) are skipped.
func (p *FinMindFuturesInstitutionalProvider) FetchInstitutionalFuturesRange(ctx context.Context, futuresID, startDate, endDate string) ([]InstitutionalFuturesDaily, error) {
	id := normalizeFinMindFuturesID(futuresID)
	if p.client == nil {
		return nil, fmt.Errorf("finmind taifex: nil client")
	}
	data, err := p.client.fetchDataset(ctx, finmindFuturesDataset, id, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("finmind taifex %s %s..%s: %w", id, startDate, endDate, err)
	}
	return parseInstitutionalFuturesRows(data, id)
}

// FetchInstitutionalFuturesForDate fetches one trading date for one futures
// product. Convenience wrapper over FetchInstitutionalFuturesRange.
// NormalizeFinMindFuturesID is the exported form of normalizeFinMindFuturesID
// for callers outside package marketdata (e.g. backfill CLIs that need to
// display the effective futures_id before fetching).
func NormalizeFinMindFuturesID(id string) string {
	return normalizeFinMindFuturesID(id)
}

func (p *FinMindFuturesInstitutionalProvider) FetchInstitutionalFuturesForDate(ctx context.Context, futuresID, date string) (*InstitutionalFuturesDaily, error) {
	rows, err := p.FetchInstitutionalFuturesRange(ctx, futuresID, date, date)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("finmind taifex %s: no institutional trader rows for %s", normalizeFinMindFuturesID(futuresID), date)
	}
	return &rows[0], nil
}

// normalizeFinMindFuturesID trims/uppercases a FinMind futures_id and
// translates TAIFEX-website commodityIds to FinMind futures_id:
//
//	TXF (臺股期貨, TAIFEX futContractsDateDown commodityId) → TX
//	MXF (小型臺指期貨, TAIFEX commodityId)                    → MTX
//
// Other codes pass through unchanged (TX, MTX, EXF, FXF, GTF, TJF, ...).
func normalizeFinMindFuturesID(id string) string {
	id = strings.ToUpper(strings.TrimSpace(id))
	switch id {
	case "TXF":
		return "TX"
	case "MXF":
		return "MTX"
	}
	return id
}

// parseInstitutionalFuturesRows groups raw FinMind rows by date and maps the
// three trader categories onto InstitutionalFuturesDaily. A date is emitted
// only when all three categories (外資/投信/自營商) are present — a missing
// category means the upstream schema changed or the API stopped serving the
// category (P0-3 style typed error, not silent partial data).
func parseInstitutionalFuturesRows(data []map[string]any, futuresID string) ([]InstitutionalFuturesDaily, error) {
	type dayAccum struct {
		date  string
		seen  map[string]bool
		daily InstitutionalFuturesDaily
	}
	byDate := make(map[string]*dayAccum)
	var order []string

	for i, item := range data {
		rowID, _ := item["futures_id"].(string)
		if rowID != "" && !strings.EqualFold(rowID, futuresID) {
			continue
		}
		date, _ := item["date"].(string)
		if date == "" {
			return nil, fmt.Errorf("finmind taifex: row %d missing date (futures_id=%v)", i, item["futures_id"])
		}
		cat, _ := item["institutional_investors"].(string)
		cat = strings.TrimSpace(cat)
		if cat == "" {
			return nil, fmt.Errorf("finmind taifex: row %d missing institutional_investors (date=%s)", i, date)
		}
		side, err := parseFinMindTraderSide(item)
		if err != nil {
			return nil, fmt.Errorf("finmind taifex %s %s %s: %w", futuresID, date, cat, err)
		}

		acc := byDate[date]
		if acc == nil {
			acc = &dayAccum{date: date, seen: map[string]bool{}}
			acc.daily.Date = date
			byDate[date] = acc
			order = append(order, date)
		}
		// Map FinMind category names to TraderSide slots. "外資" is FinMind's
		// name; "外資及陸資" is TAIFEX OpenAPI's — both map to Foreign.
		switch cat {
		case "外資", "外資及陸資":
			acc.daily.Foreign = side
			acc.seen["外資"] = true
		case "投信":
			acc.daily.InvestmentTrust = side
			acc.seen["投信"] = true
		case "自營商":
			acc.daily.Dealer = side
			acc.seen["自營商"] = true
		default:
			return nil, fmt.Errorf("finmind taifex: unknown institutional_investors %q (date=%s)", cat, date)
		}
	}

	result := make([]InstitutionalFuturesDaily, 0, len(order))
	var errs []string
	for _, date := range order {
		acc := byDate[date]
		if !acc.seen["外資"] || !acc.seen["投信"] || !acc.seen["自營商"] {
			errs = append(errs, fmt.Sprintf("%s missing trader rows (foreign=%v trust=%v dealer=%v)",
				date, acc.seen["外資"], acc.seen["投信"], acc.seen["自營商"]))
			continue
		}
		result = append(result, acc.daily)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("finmind taifex: incomplete days: %s", strings.Join(errs, "; "))
	}
	return result, nil
}

// parseFinMindTraderSide extracts one trader category's volume + OI fields
// from a raw FinMind row. Volume/OI fields are required (schema guard); the
// four NT$ amount fields are informational and parsed best-effort.
func parseFinMindTraderSide(item map[string]any) (TraderSide, error) {
	var side TraderSide
	var err error
	if side.TradeLong, err = finmindRowInt64(item, "long_deal_volume"); err != nil {
		return side, err
	}
	if side.TradeShort, err = finmindRowInt64(item, "short_deal_volume"); err != nil {
		return side, err
	}
	if side.OILong, err = finmindRowInt64(item, "long_open_interest_balance_volume"); err != nil {
		return side, err
	}
	if side.OIShort, err = finmindRowInt64(item, "short_open_interest_balance_volume"); err != nil {
		return side, err
	}
	side.TradeNet = side.TradeLong - side.TradeShort
	side.OINet = side.OILong - side.OIShort
	return side, nil
}

// finmindRowInt64 reads a numeric field from a decoded FinMind row. The
// client decodes JSON into map[string]any, so numbers arrive as float64;
// strings are accepted defensively (upstream schema drift), matching
// parseInt64OK semantics elsewhere in this package.
func finmindRowInt64(item map[string]any, field string) (int64, error) {
	v, ok := item[field]
	if !ok {
		return 0, fmt.Errorf("%w: missing %s", ErrTAIFEXSchema, field)
	}
	switch n := v.(type) {
	case float64:
		return int64(n), nil
	case int:
		return int64(n), nil
	case int64:
		return n, nil
	case string:
		parsed, ok := parseInt64OK(n)
		if !ok {
			return 0, fmt.Errorf("%w: %s=%q not parseable", ErrTAIFEXSchema, field, n)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%w: %s has unexpected type %T", ErrTAIFEXSchema, field, v)
	}
}
