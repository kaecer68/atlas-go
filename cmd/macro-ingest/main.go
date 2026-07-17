package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

func main() {
	workDir := flag.String("work-dir", ".", "working directory")
	flag.Parse()

	stateDir := filepath.Join(*workDir, "data/state")
	snapshotDir := filepath.Join(stateDir, "macro")
	capitalFlowDir := filepath.Join(stateDir, "capital_flow")
	exportDir := filepath.Join(stateDir, "export")

	var pool *pgxpool.Pool
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		migrationsPath := filepath.Join(*workDir, "sql/migrations")
		if _, err := os.Stat(migrationsPath); err == nil {
			var dbErr error
			pool, dbErr = db.Init(context.Background(), dsn, migrationsPath)
			if dbErr != nil {
				log.Printf("[DB] failed to initialize database: %v", dbErr)
			} else {
				log.Printf("[DB] connected and migrations applied")
				defer pool.Close()
			}
		}
	}

	provider := marketdata.NewCompositeMacroProvider(
		marketdata.NewYahooFinanceMacroProvider(),
		marketdata.NewFrankfurterFXProvider(),
		marketdata.NewTWSECapitalFlowProvider(capitalFlowDir),
		marketdata.NewExportStatisticsProvider(exportDir),
		marketdata.NewTWSEMarginBalanceProvider(""),
		marketdata.NewSOXIndexProvider(),
		marketdata.NewSPXIndexProvider(),
		marketdata.NewNDXIndexProvider(),
		marketdata.NewDJIIndexProvider(),
		marketdata.NewTSMADRProvider(),
		marketdata.NewTAIEXIndexProvider(),
		marketdata.NewNVDAProvider(),
		marketdata.NewAAPLProvider(),
		marketdata.NewMSFTProvider(),
		marketdata.NewTaiwanVolatilityProvider(),
	)

	ingestor := narrative.NewMacroIngestor(provider, snapshotDir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log.Println("[MacroIngest] Starting macro data ingestion...")
	events, snap, err := ingestor.Ingest(ctx)
	if err != nil {
		// All providers failed — FetchSnapshot only returns error when every provider errors.
		log.Printf("[MacroIngest] All providers failed: %v", err)
		monitoring.RecordChannelFetchWithPool(stateDir, "us_yahoo", "error", err.Error(), pool)
		monitoring.RecordChannelFetchWithPool(stateDir, "frankfurter_fx", "error", err.Error(), pool)
	} else {
		// Per-provider status: the composite snapshot already records which
		// sub-providers failed (FailedChannels). Surface partial failures as
		// "degraded" for the affected channel instead of masking both as "ok"
		// (fix manifest #B08).
		failed := make(map[string]bool, len(snap.FailedChannels))
		for _, name := range snap.FailedChannels {
			failed[name] = true
		}
		recordChannel := func(channelID, providerName string) {
			if failed[providerName] {
				monitoring.RecordChannelFetchWithPool(stateDir, channelID, "degraded", "provider fetch failed (partial composite failure)", pool)
			} else {
				monitoring.RecordChannelFetchWithPool(stateDir, channelID, "ok", "", pool)
			}
		}
		recordChannel("us_yahoo", "yahoo_finance")
		recordChannel("frankfurter_fx", "frankfurter_fx")
		if len(snap.FailedChannels) > 0 {
			log.Printf("[MacroIngest] partial failure: providers failed=%v", snap.FailedChannels)
		}
	}
	log.Printf("[MacroIngest] Ingested %d events, snapshot recorded_at=%d", len(events), snap.RecordedAt)
}
