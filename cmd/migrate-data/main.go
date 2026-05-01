package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}
	log.Println("Migration completed successfully")
}

func run() error {
	var (
		migrateMetrics  = flag.Bool("metrics", false, "Migrate metrics data")
		migrateAlerts   = flag.Bool("alerts", false, "Migrate alerts data")
		migrateOutcomes = flag.Bool("outcomes", false, "Migrate recommendation outcomes")
		migrateAll      = flag.Bool("all", false, "Migrate all data")
	)
	flag.Parse()

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL not configured")
	}

	ctx := context.Background()
	pool, err := db.Init(ctx, cfg.DatabaseURL, cfg.MigrationsPath)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	defer pool.Close()

	stateDir := cfg.LedgerDir
	if stateDir == "" {
		stateDir = "data/state"
	}

	if *migrateAll || *migrateMetrics {
		if err := migrateMetricsData(ctx, pool, stateDir); err != nil {
			return fmt.Errorf("migrate metrics: %w", err)
		}
	}

	if *migrateAll || *migrateAlerts {
		if err := migrateAlertsData(ctx, pool, stateDir); err != nil {
			return fmt.Errorf("migrate alerts: %w", err)
		}
	}

	if *migrateAll || *migrateOutcomes {
		if err := migrateOutcomesData(ctx, pool, stateDir); err != nil {
			return fmt.Errorf("migrate outcomes: %w", err)
		}
	}

	return nil
}

func migrateMetricsData(ctx context.Context, pool *pgxpool.Pool, stateDir string) error {
	filePath := stateDir + "/metrics.jsonl"
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No metrics.jsonl found, skipping")
			return nil
		}
		return err
	}
	defer f.Close()

	type snapshot struct {
		ScreeningTotal     int64            `json:"screening_total"`
		ScreeningPassed    int64            `json:"screening_passed"`
		ScreeningRate      float64          `json:"screening_rate"`
		AlertsTriggered    int64            `json:"alerts_triggered"`
		AlertsAcknowledged int64            `json:"alerts_acknowledged"`
		AlertsByType       map[string]int64 `json:"alerts_by_type"`
		Timestamp          time.Time        `json:"timestamp"`
	}

	var count int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var s snapshot
		if err := json.Unmarshal(scanner.Bytes(), &s); err != nil {
			continue
		}

		metrics := map[string]float64{
			"screening_total":      float64(s.ScreeningTotal),
			"screening_passed":     float64(s.ScreeningPassed),
			"screening_rate":       s.ScreeningRate,
			"alerts_triggered":     float64(s.AlertsTriggered),
			"alerts_acknowledged":  float64(s.AlertsAcknowledged),
		}
		for alertType, count := range s.AlertsByType {
			metrics["alerts_"+alertType] = float64(count)
		}

		for name, value := range metrics {
			_, err := pool.Exec(ctx, `
				INSERT INTO metrics (time, metric_name, value, metadata)
				VALUES ($1, $2, $3, '{}')
			`, s.Timestamp, name, value)
			if err != nil {
				log.Printf("Warning: failed to insert metric %s: %v", name, err)
			}
		}
		count++
	}

	log.Printf("Migrated %d metrics snapshots", count)
	return scanner.Err()
}

func migrateAlertsData(ctx context.Context, pool *pgxpool.Pool, stateDir string) error {
	filePath := stateDir + "/alerts/alerts.jsonl"
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No alerts.jsonl found, skipping")
			return nil
		}
		return err
	}
	defer f.Close()

	var count int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var alert domain.AlertRecord
		if err := json.Unmarshal(scanner.Bytes(), &alert); err != nil {
			continue
		}

		_, err := pool.Exec(ctx, `
			INSERT INTO alerts (id, timestamp, rule, severity, message, value, threshold, acknowledged, acknowledged_at, acknowledged_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (id) DO NOTHING
		`, alert.ID, alert.Timestamp, alert.Rule, alert.Severity, alert.Message,
			alert.Value, alert.Threshold, alert.Acknowledged, alert.AcknowledgedAt, alert.AcknowledgedBy)
		if err != nil {
			log.Printf("Warning: failed to insert alert %s: %v", alert.ID, err)
		} else {
			count++
		}
	}

	log.Printf("Migrated %d alerts", count)
	return scanner.Err()
}

func migrateOutcomesData(ctx context.Context, pool *pgxpool.Pool, stateDir string) error {
	filePath := stateDir + "/recommendation_outcomes.jsonl"
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No recommendation_outcomes.jsonl found, skipping")
			return nil
		}
		return err
	}
	defer f.Close()

	var count int
	batchSize := 1000
	var batch []domain.RecommendationOutcome

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var outcome domain.RecommendationOutcome
		if err := json.Unmarshal(scanner.Bytes(), &outcome); err != nil {
			continue
		}
		batch = append(batch, outcome)

		if len(batch) >= batchSize {
			if err := insertOutcomeBatch(ctx, pool, batch); err != nil {
				log.Printf("Warning: failed to insert batch: %v", err)
			} else {
				count += len(batch)
			}
			batch = batch[:0]
		}
	}

	// Insert remaining
	if len(batch) > 0 {
		if err := insertOutcomeBatch(ctx, pool, batch); err != nil {
			log.Printf("Warning: failed to insert final batch: %v", err)
		} else {
			count += len(batch)
		}
	}

	log.Printf("Migrated %d recommendation outcomes", count)
	return scanner.Err()
}

func insertOutcomeBatch(ctx context.Context, pool *pgxpool.Pool, outcomes []domain.RecommendationOutcome) error {
	for _, o := range outcomes {
		metadata, _ := json.Marshal(o)
		_, err := pool.Exec(ctx, `
			INSERT INTO recommendation_outcomes (time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, o.RecordedAt, o.Window, o.Symbol, o.AgentID, string(o.Layer),
			o.Conviction, o.PassedGuards, o.GuardReason, o.Price, metadata)
		if err != nil {
			return err
		}
	}
	return nil
}
