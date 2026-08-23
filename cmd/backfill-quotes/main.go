package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func main() {
	csvPath := flag.String("csv", constants.ReplayCSVPath, "source replay CSV path")
	storeBackend := flag.String("store-backend", getEnvDefault("STORE_BACKEND", "jsonl"), "primary quote store backend: jsonl or sqlite")
	ledgerDir := flag.String("ledger-dir", getEnvDefault("LEDGER_DIR", "data/state"), "jsonl quote store base directory")
	sqlitePath := flag.String("sqlite-path", getEnvDefault("SQLITE_PATH", "data/state/atlas.db"), "sqlite quote store path")
	writeSQLite := flag.Bool("write-sqlite", true, "also backfill the sqlite store (in addition to the primary backend)")
	dryRun := flag.Bool("dry-run", false, "print summary without writing")
	flag.Parse()

	ds, err := replay.LoadTWSEOpenDataCSV(*csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load replay csv: %v\n", err)
		os.Exit(1)
	}

	var stores []ledger.QuoteStore
	switch *storeBackend {
	case "sqlite":
		store, err := ledger.NewSQLiteQuoteStoreFromPath(*sqlitePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open sqlite quote store: %v\n", err)
			os.Exit(1)
		}
		stores = append(stores, store)
	default:
		stores = append(stores, ledger.NewJSONLQuoteStore(*ledgerDir))
	}
	if *writeSQLite && *storeBackend != "sqlite" {
		store, err := ledger.NewSQLiteQuoteStoreFromPath(*sqlitePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open sqlite quote store: %v\n", err)
			os.Exit(1)
		}
		stores = append(stores, store)
	}

	var bars []domain.DailyBar
	for _, bySymbol := range ds.ByDate {
		for _, bar := range bySymbol {
			// The replay CSV uses "2330.TW" but /api/stock/technical expects bare "2330".
			bar.Symbol = strings.TrimSuffix(bar.Symbol, ".TW")
			bars = append(bars, bar)
		}
	}

	// Skip weekends and Taiwan public holidays before writing: a closed-market
	// row in the replay CSV is a copy of the previous trading day and poisons
	// ForwardReturn duplicate detection downstream (2026-08-23 replay bug).
	bars, skipped := filterTradingDays(bars)

	fmt.Printf("loaded %d daily bars from %s (%d non-trading-day rows skipped)\n", len(bars), *csvPath, skipped)
	if *dryRun {
		return
	}

	for _, store := range stores {
		if err := store.RecordQuotes(bars); err != nil {
			fmt.Fprintf(os.Stderr, "record quotes: %v\n", err)
			os.Exit(1)
		}
	}
	log.Printf("backfilled %d quotes into %d quote store(s) (backend=%s, write-sqlite=%v)", len(bars), len(stores), *storeBackend, *writeSQLite)
}

func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// filterTradingDays removes bars on non-trading days (weekends and Taiwan
// public holidays per marketdata.IsTaiwanTradingDay) and returns the kept
// bars plus the number of removed rows. Same guard as
// cmd/cron-quote-backfill and cmd/daily-replay-sync runGapBackfill.
func filterTradingDays(bars []domain.DailyBar) ([]domain.DailyBar, int) {
	kept := bars[:0]
	skipped := 0
	for _, bar := range bars {
		if !marketdata.IsTaiwanTradingDay(bar.Date) {
			skipped++
			continue
		}
		kept = append(kept, bar)
	}
	return kept, skipped
}
