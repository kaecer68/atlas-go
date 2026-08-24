package marketdata

import (
	"context"
	"fmt"
)

// MSCIRebalanceCalendarProvider is a static provider of MSCI quarterly index
// review dates for the Taiwan market (MSCI 季度調整).
//
// Schedule (public, stable since 2008):
//   - 4 reviews per year: February (quarterly), May (semiannual),
//     August (quarterly), November (semiannual).
//   - MSCI announces the review ~20 days ahead; the rebalance takes effect at
//     the close of the last business day of the review month (Taiwan time).
//     Index funds execute their rebalancing trades on the effective day and
//     the following session — the classic MSCI effect day for TWSE stocks.
//
// Effective dates below are the last TWSE business day of Feb/May/Aug/Nov
// (weekends and TWSE holidays excluded; verified against the TWSE holiday
// calendar 2026-08-24 by k3 audit: 2023-02-24/2025-02-27/2025-05-29/2026-02-26
// are the pre-holiday trading days for 228/端午 closures). These are the
// dates that matter for
// event-driven mispricing backtests (charter C4/C16).
//
// Source: MSCI index calendar / public announcements (rich01, stockfeel,
// storm, technews coverage for 2023-2026; 2026-08 announced 08-13, effective
// 2026-08-31 — verified live 2026-08-24).
//
// Maturity: evolving (static table; refresh annually)
type MSCIRebalanceCalendarProvider struct{}

// NewMSCIRebalanceCalendarProvider creates the static MSCI provider.
func NewMSCIRebalanceCalendarProvider() *MSCIRebalanceCalendarProvider {
	return &MSCIRebalanceCalendarProvider{}
}

// Name returns the provider name.
func (p *MSCIRebalanceCalendarProvider) Name() string {
	return "msci_static"
}

// msciQuarterlyReviewDates holds the effective dates (ISO YYYY-MM-DD) of MSCI
// quarterly index reviews. One entry per review: announcement month + the
// effective date (last TWSE business day of the review month).
type msciReviewEntry struct {
	date string // effective date, ISO YYYY-MM-DD
}

var msciReviewEntries = []msciReviewEntry{
	// 2023
	{date: "2023-02-24"},
	{date: "2023-05-31"},
	{date: "2023-08-31"},
	{date: "2023-11-30"},
	// 2024
	{date: "2024-02-29"},
	{date: "2024-05-31"},
	{date: "2024-08-30"},
	{date: "2024-11-29"},
	// 2025
	{date: "2025-02-27"},
	{date: "2025-05-29"},
	{date: "2025-08-29"},
	{date: "2025-11-28"},
	// 2026
	{date: "2026-02-26"},
	{date: "2026-05-29"},
	{date: "2026-08-31"},
	{date: "2026-11-30"}, // scheduled (derived from trading calendar)
}

// FetchEvents returns MSCI quarterly rebalance events for the given year.
// Years without a static entry (before 2023) return an empty slice.
func (p *MSCIRebalanceCalendarProvider) FetchEvents(ctx context.Context, year int) ([]CalendarProviderData, error) {
	var events []CalendarProviderData
	for _, e := range msciReviewEntries {
		if len(e.date) != 10 || !hasYearPrefix(e.date, year) {
			continue
		}
		events = append(events, CalendarProviderData{
			Date:        e.date,
			EventType:   "msci_rebalance",
			Name:        fmt.Sprintf("MSCI 季度調整 %s", e.date),
			Symbol:      "",
			Direction:   "mixed",
			Weight:      0.9,
			Description: "MSCI 季度/半年度指數調整生效日（收盤後生效）",
			Source:      "msci_static",
		})
	}
	return events, nil
}

// hasYearPrefix reports whether an ISO date string starts with the given year.
func hasYearPrefix(isoDate string, year int) bool {
	if len(isoDate) < 4 {
		return false
	}
	return isoDate[0:4] == fmt.Sprintf("%04d", year)
}
