// Package monitoring — auto_quote_backfill BTM task.
//
// This task reads fundamentals.json daily, filters to stocks with valid
// fundamental data (PE > 0), and backfills any missing historical daily
// bars via FugleClient.GetHistoricalCandles. Stocks already covered
// (≥90 days of data) are skipped to keep the task idempotent.
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
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// QuoteBackfillDeps holds the dependencies for auto_quote_backfill.
type QuoteBackfillDeps struct {
	FugleClient *marketdata.FugleClient
	QuoteStore  ledger.QuoteStore
	WorkDir     string
}

// NewQuoteBackfillRunner returns a BTM-compatible runner (func(ctx) error)
// that backfills missing historical quotes for stocks in fundamentals.json
// that pass basic fundamental health checks (PE > 0).
func NewQuoteBackfillRunner(deps QuoteBackfillDeps) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if deps.FugleClient == nil {
			log.Printf("[quote_backfill] skipped: FugleClient not configured")
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

		from := "2026-01-01"
		to := time.Now().Format("2006-01-02")
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Now()
		fetched := 0
		skipped := 0
		failed := 0

		for _, sym := range symbols {
			select {
			case <-ctx.Done():
				log.Printf("[quote_backfill] cancelled after %d fetched, %d skipped, %d failed",
					fetched, skipped, failed)
				return ctx.Err()
			default:
			}

			// Check if already covered.
			existing, err := deps.QuoteStore.LoadQuotes(sym+".TW", start, end)
			if err == nil && len(existing) >= 90 {
				skipped++
				continue
			}

			bars, err := deps.FugleClient.GetHistoricalCandles(ctx, sym, from, to)
			if err != nil {
				failed++
				continue
			}
			if len(bars) < 2 {
				continue
			}
			if err := deps.QuoteStore.RecordQuotes(bars); err != nil {
				log.Printf("[quote_backfill] RecordQuotes failed for %s: %v", sym, err)
				failed++
				continue
			}
			fetched++
		}

		log.Printf("[quote_backfill] complete: %d fetched, %d skipped, %d failed",
			fetched, skipped, failed)
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
