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
	"github.com/kaecer68/atlas-go/internal/repository"
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

	exportProvider := marketdata.NewExportStatisticsProvider(exportDir)
	if pool != nil {
		// Mirror each successful export fetch to PostgreSQL export_statistics
		// through the same DualWriteRepository pipeline used by the atlas
		// runtime (best-effort; provider logs WARN and never fails the fetch).
		exportProvider.SetExportStatsSaver(repository.NewDualWriteRepository(pool, nil, nil, nil, nil, nil, nil))
	}

	provider := marketdata.NewCompositeMacroProvider(
		marketdata.NewYahooFinanceMacroProvider(),
		marketdata.NewFrankfurterFXProvider(),
		marketdata.NewTWSECapitalFlowProvider(capitalFlowDir),
		exportProvider,
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
		// channelID list must mirror register_adapters.go so the dashboard
		// /api/dashboard/data-channels page reflects the same health for each
		// registered channel. tw_vol was added in feat/tw-vol-channel-2026-07-22.
		recordChannel("us_yahoo", "yahoo_finance")
		recordChannel("frankfurter_fx", "frankfurter_fx")
		recordChannel("twse_capital_flow", "twse_capital_flow")
		recordChannel("export_statistics", "export_statistics")
		recordChannel("twse_margin", "twse_margin")
		recordChannel("sox_index", "sox_index")
		recordChannel("us_spx", "us_spx")
		recordChannel("us_ndx", "us_ndx")
		recordChannel("us_dji", "us_dji")
		recordChannel("tsm_adr", "tsm_adr")
		recordChannel("taiex_index", "taiex_index")
		recordChannel("us_nvda", "us_nvda")
		recordChannel("us_aapl", "us_aapl")
		recordChannel("us_msft", "us_msft")
		recordChannel("tw_vol", "tw_vol")
		if len(snap.FailedChannels) > 0 {
			log.Printf("[MacroIngest] partial failure: providers failed=%v", snap.FailedChannels)
		}
	}
	log.Printf("[MacroIngest] Ingested %d events, snapshot recorded_at=%d", len(events), snap.RecordedAt)
}
