// Package monitoring — auto_quote_backfill BTM task.
//
// This task reads fundamentals.json daily, filters to stocks with valid
// fundamental data (PE > 0), and backfills any missing historical daily
// bars via FinMind's TaiwanStockPrice dataset.
//
// Data source priority (documented per CONSTITUTION.md):
//  1. TWSE OpenAPI — no historical range API (daily quotes only)
//  2. Fubon — no historical candles API (intraday quotes only)
//  3. FinMind — per-day GetStockPrice (usable for background, too slow for
//     on-demand; used HERE for BTM backfill to preserve Fugle free-tier quota)
//  4. Fugle — historical/candles range API (used in HandleTechnical on-demand
//     fallback and warmupQuotes because one call covers the full window)
//
// The task is idempotent and incremental: it only fetches days not already
// present in the quote store. Rate limiting is handled by FinMindClient's
// shared rate limiter (600 req/hr free tier).
//
// Registration (in main.go):
//
//	mgr.RegisterRunner("auto_quote_backfill", monitoring.NewQuoteBackfillRunner(deps))
package monitoring

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// QuoteBackfillDeps holds the dependencies for auto_quote_backfill.
type QuoteBackfillDeps struct {
	FinMindClient *marketdata.FinMindClient
	QuoteStore    ledger.QuoteStore
	WorkDir       string
}

// defaultBackfillStart is the earliest date we backfill from.
const defaultBackfillStart = "2026-01-01"

// backfillQuotaStopRemaining is the FinMind daily-quota floor at which the
// backfill task stops pulling more data (P1-12). The scheduler gate keeps a
// headroom budget for the live channel (auto_cycle_update / channel health)
// instead of letting a cold-start backfill burn the whole 14400/day ceiling.
const backfillQuotaStopRemaining = 200

// NewQuoteBackfillRunner returns a BTM-compatible runner (func(ctx) error)
// that backfills missing historical quotes via FinMind for stocks in
// fundamentals.json with PE > 0.
func NewQuoteBackfillRunner(deps QuoteBackfillDeps) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if deps.FinMindClient == nil {
			log.Printf("[quote_backfill] skipped: FinMindClient not configured")
			return nil
		}
		if deps.QuoteStore == nil {
			log.Printf("[quote_backfill] skipped: QuoteStore not configured")
			return nil
		}

		fundPath := filepath.Join(deps.WorkDir, "data", "fundamentals.json")
		symbols, err := loadFundamentalSymbols(fundPath)
		if err != nil {
			return err
		}
		log.Printf("[quote_backfill] loaded %d symbols with PE>0 from fundamentals.json", len(symbols))

		backfillStart, _ := time.Parse("2006-01-02", defaultBackfillStart)
		today := time.Now().Truncate(24 * time.Hour)
		lookbackStart := today.AddDate(0, 0, -90)
		fetched := 0
		skipped := 0
		failed := 0
		totalBars := 0

		for _, sym := range symbols {
			select {
			case <-ctx.Done():
				log.Printf("[quote_backfill] cancelled after %d fetched (%d bars), %d skipped, %d failed",
					fetched, totalBars, skipped, failed)
				return ctx.Err()
			default:
			}

			// P1-12: scheduler quota gate — stop early when the daily budget
			// is nearly gone so the live channel keeps its headroom.
			if deps.FinMindClient.QuotaRemaining() < backfillQuotaStopRemaining {
				log.Printf("[quote_backfill] stopping early: FinMind quota remaining %d < %d (reserving headroom for live channel)",
					deps.FinMindClient.QuotaRemaining(), backfillQuotaStopRemaining)
				break
			}

			qsSymbol := sym + ".TW"

			// Determine which date range we need to fill.
			existing, _ := deps.QuoteStore.LoadQuotes(qsSymbol, backfillStart, today)
			if len(existing) >= 90 {
				skipped++
				continue
			}

			// Build set of existing dates for fast lookup.
			have := make(map[string]bool, len(existing))
			var latestDate time.Time
			for _, b := range existing {
				ds := b.Date.Format("2006-01-02")
				have[ds] = true
				if b.Date.After(latestDate) {
					latestDate = b.Date
				}
			}

			// Determine fetch range.
			fetchStart := backfillStart
			if !latestDate.IsZero() && latestDate.After(backfillStart) {
				fetchStart = latestDate.AddDate(0, 0, 1)
			}
			// Cap: only backfill up to lookbackStart for the initial run
			// (90-day window); ongoing runs naturally fill recent days.
			if fetchStart.Before(lookbackStart) {
				fetchStart = lookbackStart
			}
			fetchEnd := today

			if fetchStart.After(fetchEnd) {
				skipped++
				continue
			}

			// Fetch day-by-day via FinMind.
			var bars []domain.DailyBar
			for d := fetchStart; !d.After(fetchEnd); d = d.AddDate(0, 0, 1) {
				ds := d.Format("2006-01-02")
				if have[ds] {
					continue
				}
				// Skip weekends (no trading data).
				if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
					continue
				}

				q, ferr := deps.FinMindClient.GetStockPrice(ctx, sym, ds)
				if ferr != nil {
					// Non-trading day or API error — skip gracefully.
					continue
				}
				if q.Symbol == "" {
					continue
				}
				bars = append(bars, domain.DailyBar{
					Symbol: qsSymbol,
					Date:   d,
					Open:   q.Open,
					High:   q.High,
					Low:    q.Low,
					Close:  q.Last,
					Volume: q.Volume,
					Source: "finmind_backfill",
				})
			}

			if len(bars) == 0 {
				continue
			}

			// Sort bars by date for deterministic storage.
			sort.Slice(bars, func(i, j int) bool {
				return bars[i].Date.Before(bars[j].Date)
			})

			if err := deps.QuoteStore.RecordQuotes(bars); err != nil {
				log.Printf("[quote_backfill] RecordQuotes failed for %s: %v", sym, err)
				failed++
				continue
			}
			totalBars += len(bars)
			fetched++
		}

		log.Printf("[quote_backfill] complete: %d symbols fetched (%d bars), %d skipped, %d failed",
			fetched, totalBars, skipped, failed)
		return nil
	}
}

// loadFundamentalSymbols reads fundamentals.json and returns symbols
// (without .TW suffix) that have PE > 0 (valid fundamental data).
func loadFundamentalSymbols(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]struct {
		PE float64 `json:"PE"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var symbols []string
	for sym, f := range raw {
		if f.PE > 0 {
			symbols = append(symbols, strings.TrimSuffix(sym, ".TW"))
		}
	}
	return symbols, nil
}
