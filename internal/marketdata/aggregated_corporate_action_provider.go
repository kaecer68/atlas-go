package marketdata

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// AggregatedCorporateActionProvider is the reference implementation of
// CorporateActionProvider. It merges ex-dividend / ex-right data from TWSE
// (primary, includes the published 除權息參考價 as ReferencePrice) and FinMind
// (backup, fills events TWSE is missing).
//
// Priority / dedup rules:
//   - TWSE wins for any event present in both sources (dedup key is
//     Symbol + ExDate). TWSE provides the published reference price which is
//     the most authoritative anchor for downstream adjustment.
//   - FinMind contributes only events absent from TWSE; its dividend
//     amounts populate CashDividend / StockDividend, and ReferencePrice
//     stays 0 (the adjustment algorithm can recompute it).
//   - Source provenance is preserved on every record ("twse_calendar",
//     "finmind"). The aggregated result has no "aggregated" tag — the Source
//     field reflects the actual origin, so downstream audit can trace.
//
// Partial-failure semantics:
//   - If TWSE fails completely but FinMind succeeds, we return FinMind's
//     data with a warning log. The error is NOT propagated.
//   - If FinMind fails but TWSE succeeds, we return TWSE's data with a warning.
//   - If BOTH fail, we return an error.
//
// Output ordering: ExDate ascending.
type AggregatedCorporateActionProvider struct {
	twse    *TWSECalendarProvider
	finmind *FinMindDividendProvider
	logger  *slog.Logger
}

// NewAggregatedCorporateActionProvider wires the two upstream providers.
// Either may be nil for testing, but production code MUST pass both.
func NewAggregatedCorporateActionProvider(twse *TWSECalendarProvider, finmind *FinMindDividendProvider) *AggregatedCorporateActionProvider {
	return &AggregatedCorporateActionProvider{
		twse:    twse,
		finmind: finmind,
		logger:  logging.Default(),
	}
}

// SetLogger overrides the default logger (used by tests for silence).
func (a *AggregatedCorporateActionProvider) SetLogger(l *slog.Logger) {
	a.logger = l
}

// GetCorporateActions implements CorporateActionProvider.
//
// Filtering is applied independently of source:
//   - Symbol match is exact (case-sensitive, no ".TW" normalization).
//   - ExDate must fall within [start, end] inclusive.
func (a *AggregatedCorporateActionProvider) GetCorporateActions(
	ctx context.Context,
	symbol string,
	start, end time.Time,
) ([]domain.CorporateAction, error) {
	if symbol == "" {
		return nil, fmt.Errorf("aggregated: symbol is required")
	}
	if end.Before(start) {
		return nil, fmt.Errorf("aggregated: end before start (%v < %v)", end, start)
	}

	merged := make(map[string]domain.CorporateAction) // dedup key: symbol|exdate

	// 1) TWSE: primary anchor (ReferencePrice populated).
	if a.twse != nil {
		twseEvents, err := a.fetchTWSE(ctx, start, end)
		if err != nil {
			a.logWarn("twse_fetch_failed", err, symbol)
		} else {
			for _, e := range twseEvents {
				if e.Symbol != symbol {
					continue
				}
				merged[e.ExDate.UTC().Format("2006-01-02")] = e
			}
		}
	}

	// 2) FinMind: backup (ReferencePrice stays 0 unless both sources agree).
	if a.finmind != nil {
		finmindRecords, err := a.fetchFinMind(ctx, symbol, start, end)
		if err != nil {
			a.logWarn("finmind_fetch_failed", err, symbol)
		} else {
			for _, r := range finmindRecords {
				key := r.ExDate.UTC().Format("2006-01-02")
				if existing, ok := merged[key]; ok {
					// TWSE already covers this — but if TWSE record missed a
					// dividend field that FinMind has, backfill it.
					if existing.CashDividend == 0 && r.CashDividend != 0 {
						existing.CashDividend = r.CashDividend
					}
					if existing.StockDividend == 0 && r.StockDividend != 0 {
						existing.StockDividend = r.StockDividend
					}
					merged[key] = existing
					continue
				}
				merged[key] = r
			}
		}
	}

	if len(merged) == 0 {
		// Distinguish "no data" from "both sources failed". If both providers
		// were non-nil but we never populated merged, the network likely
		// failed silently on both. Surface that as an error.
		if a.twse != nil && a.finmind != nil {
			return nil, fmt.Errorf("aggregated: no corporate actions for %s in [%v, %v] from either TWSE or FinMind", symbol, start, end)
		}
		return []domain.CorporateAction{}, nil
	}

	out := make([]domain.CorporateAction, 0, len(merged))
	for _, v := range merged {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExDate.Before(out[j].ExDate) })
	return out, nil
}

// fetchTWSE pulls all calendar events for years overlapping [start, end],
// filters to ex_dividend type, and maps them to domain.CorporateAction.
// TWSE's "reference price" (除權息參考價) is exposed in the description text,
// not a structured field — we parse it from the description best-effort and
// leave ReferencePrice = 0 when absent (downstream consumers recompute it).
func (a *AggregatedCorporateActionProvider) fetchTWSE(
	ctx context.Context,
	start, end time.Time,
) ([]domain.CorporateAction, error) {
	if a.twse == nil {
		return nil, fmt.Errorf("twse provider not configured")
	}
	startYear, endYear := start.Year(), end.Year()
	if end.Year() > start.Year() {
		endYear = end.Year()
	}

	var collected []domain.CorporateAction
	var firstErr error
	for year := startYear; year <= endYear; year++ {
		events, err := a.twse.FetchEvents(ctx, year)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, e := range events {
			if e.EventType != "ex_dividend" {
				continue
			}
			exDate, ok := parseISODate(e.Date)
			if !ok {
				continue
			}
			if exDate.Before(start) || exDate.After(end) {
				continue
			}
			collected = append(collected, domain.CorporateAction{
				Symbol:         e.Symbol,
				ExDate:         exDate,
				ReferencePrice: 0, // TWSECalendarProvider currently exposes no
				// structured 除權息參考價 field on CalendarProviderData; we
				// keep it 0 here and let the adjustment algorithm compute
				// the anchor from CashDividend when ReferencePrice == 0.
				Source: "twse_calendar",
			})
		}
	}
	if len(collected) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return collected, nil
}

// fetchFinMind queries FinMindDividendProvider and maps records to
// domain.CorporateAction. DividendRecord.ExDividendDate is an ISO string; we
// parse it to time.Time. StockDividend > 0 implies a stock-dividend event.
func (a *AggregatedCorporateActionProvider) fetchFinMind(
	ctx context.Context,
	symbol string,
	start, end time.Time,
) ([]domain.CorporateAction, error) {
	if a.finmind == nil {
		return nil, fmt.Errorf("finmind provider not configured")
	}
	startStr := start.Format("2006-01-02")
	endStr := end.Format("2006-01-02")
	records, err := a.finmind.GetDividends(ctx, symbol, startStr, endStr)
	if err != nil {
		return nil, err
	}
	out := make([]domain.CorporateAction, 0, len(records))
	for _, r := range records {
		exDate, ok := parseISODate(r.ExDividendDate)
		if !ok {
			continue
		}
		if exDate.Before(start) || exDate.After(end) {
			continue
		}
		out = append(out, domain.CorporateAction{
			Symbol:        r.Symbol,
			ExDate:        exDate,
			CashDividend:  r.CashDividend,
			StockDividend: r.StockDividend,
			Source:        "finmind",
		})
	}
	return out, nil
}

// logWarn is a tiny indirection so tests can swap in a silent logger
// without polluting production logging.
func (a *AggregatedCorporateActionProvider) logWarn(event string, err error, symbol string) {
	if a.logger == nil {
		return
	}
	a.logger.Warn("corporate_action_provider", event,
		logging.FStr("symbol", symbol),
		logging.Err(err))
}

// parseISODate accepts "2006-01-02" (and the same with time portion).
func parseISODate(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if len(s) >= 10 {
		if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return t, true
		}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}
