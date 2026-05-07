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
		marketdata.NewTWSECapitalFlowProvider(capitalFlowDir),
		marketdata.NewExportStatisticsProvider(exportDir),
	)

	ingestor := narrative.NewMacroIngestor(provider, snapshotDir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log.Println("[MacroIngest] Starting macro data ingestion...")
	events, snap, err := ingestor.Ingest(ctx)
	if err != nil {
		monitoring.RecordChannelFetchWithPool(stateDir, "us_yahoo", "error", err.Error(), pool)
		monitoring.RecordChannelFetchWithPool(stateDir, "jpy_yahoo", "error", err.Error(), pool)
		log.Fatalf("ingest failed: %v", err)
	}

	monitoring.RecordChannelFetchWithPool(stateDir, "us_yahoo", "ok", "", pool)
	monitoring.RecordChannelFetchWithPool(stateDir, "jpy_yahoo", "ok", "", pool)
	log.Printf("[MacroIngest] Ingested %d events, snapshot recorded_at=%d", len(events), snap.RecordedAt)
}
