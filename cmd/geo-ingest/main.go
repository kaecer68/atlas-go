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
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

func main() {
	workDir := flag.String("work-dir", ".", "working directory")
	flag.Parse()

	stateDir := filepath.Join(*workDir, "data/state")
	geoDir := filepath.Join(stateDir, "geopolitical")

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

	provider := geopolitical.NewCompositeGeopoliticalProvider(
		geopolitical.NewRSSGeopoliticalProvider(),
		geopolitical.NewGDELTGeopoliticalProvider(),
	)

	store := geopolitical.NewGeopoliticalStore(geoDir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log.Println("[GeoIngest] Starting geopolitical data ingestion...")
	score, err := provider.FetchScore(ctx)
	if err != nil {
		monitoring.RecordChannelFetchWithPool(stateDir, "geopolitical", "error", err.Error(), pool)
		log.Fatalf("fetch geopolitical score failed: %v", err)
	}

	if err := store.Save(score); err != nil {
		monitoring.RecordChannelFetchWithPool(stateDir, "geopolitical", "error", err.Error(), pool)
		log.Fatalf("save geopolitical score failed: %v", err)
	}

	monitoring.RecordChannelFetchWithPool(stateDir, "geopolitical", "ok", "", pool)
	log.Printf("[GeoIngest] Ingested geopolitical score: intensity=%.2f, sources=%v", score.Intensity, score.Sources)
}
