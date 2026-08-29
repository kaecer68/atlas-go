package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}
	log.Println("Migration completed successfully")
}

func run() error {
	var (
		migrateMetrics           = flag.Bool("metrics", false, "Migrate metrics data")
		migrateAlerts            = flag.Bool("alerts", false, "Migrate alerts data")
		migrateOutcomes          = flag.Bool("outcomes", false, "Migrate recommendation outcomes")
		migrateScreening         = flag.Bool("screening", false, "Migrate screening rejects")
		migrateSummaries         = flag.Bool("summaries", false, "Migrate session summaries")
		migrateInterventions     = flag.Bool("interventions", false, "Migrate human interventions")
		migrateCapitalFlow       = flag.Bool("capitalflow", false, "Migrate capital flow data")
		migrateExportStats       = flag.Bool("exportstats", false, "Migrate export statistics")
		migrateHistorical        = flag.Bool("historical", false, "Migrate historical tables (regime/stress/geo/period) from SQLite atlas.db")
		migrateQuotes            = flag.Bool("quotes", false, "Migrate quotes from SQLite atlas.db")
		migrateDetectorScans     = flag.Bool("detector-scans", false, "Migrate detector_scan_log from SQLite atlas.db")
		migrateOutcomesSQLite    = flag.Bool("outcomes-sqlite", false, "Migrate outcomes from SQLite atlas.db (SQLite is the only complete source)")
		migrateScreeningSQLite   = flag.Bool("screening-sqlite", false, "Migrate screening rejects from SQLite atlas.db")
		migrateTradesSQLite      = flag.Bool("trades-sqlite", false, "Migrate trades from SQLite atlas.db (absent from JSONL — E5)")
		migrateExperimentsSQLite = flag.Bool("experiments-sqlite", false, "Migrate experiments from SQLite atlas.db (C1: id space disjoint from JSONL)")
		migrateSummariesSQLite   = flag.Bool("summaries-sqlite", false, "Migrate session summaries from SQLite atlas.db (C2: sessions missing from JSONL)")
		outcomesSQLiteSessions   = flag.Bool("outcomes-sqlite-sessions", false, "Backfill SQLite session-scoped outcomes (session_id != '') preserving session_id (A01)")
		remapOutcomeSessionsFlag = flag.Bool("remap-outcome-sessions", false, "Remap PG date-format session_id to session-YYYYMMDD-daily (A01)")
		sqlitePath               = flag.String("sqlite-path", "data/state/atlas.db", "source SQLite atlas.db path for -historical")
		migrateAll               = flag.Bool("all", false, "Migrate all data")
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

	if *migrateAll || *migrateScreening {
		if err := migrateScreeningRejectsData(ctx, pool, stateDir); err != nil {
			return fmt.Errorf("migrate screening rejects: %w", err)
		}
	}

	if *migrateAll || *migrateSummaries {
		if err := migrateSessionSummariesData(ctx, pool, stateDir); err != nil {
			return fmt.Errorf("migrate session summaries: %w", err)
		}
	}

	if *migrateAll || *migrateInterventions {
		if err := migrateHumanInterventionsData(ctx, pool, stateDir); err != nil {
			return fmt.Errorf("migrate human interventions: %w", err)
		}
	}

	if *migrateAll || *migrateCapitalFlow {
		if err := migrateCapitalFlowData(ctx, pool, stateDir); err != nil {
			return fmt.Errorf("migrate capital flow: %w", err)
		}
	}

	if *migrateAll || *migrateExportStats {
		if err := migrateExportStatsData(ctx, pool, stateDir); err != nil {
			return fmt.Errorf("migrate export stats: %w", err)
		}
	}

	if *migrateAll || *migrateHistorical {
		if err := migrateHistoricalData(ctx, pool, *sqlitePath); err != nil {
			return fmt.Errorf("migrate historical: %w", err)
		}
	}

	if *migrateAll || *migrateQuotes {
		if err := migrateQuotesData(ctx, pool, *sqlitePath); err != nil {
			return fmt.Errorf("migrate quotes: %w", err)
		}
	}

	if *migrateAll || *migrateDetectorScans {
		if err := migrateDetectorScansData(ctx, pool, *sqlitePath); err != nil {
			return fmt.Errorf("migrate detector scans: %w", err)
		}
	}

	if *migrateAll || *migrateOutcomesSQLite {
		if err := migrateOutcomesSQLiteData(ctx, pool, *sqlitePath); err != nil {
			return fmt.Errorf("migrate outcomes sqlite: %w", err)
		}
	}

	if *migrateAll || *outcomesSQLiteSessions {
		if err := migrateOutcomesSQLiteSessions(ctx, pool, *sqlitePath); err != nil {
			return fmt.Errorf("migrate outcomes sqlite sessions: %w", err)
		}
	}

	if *migrateAll || *remapOutcomeSessionsFlag {
		if err := remapOutcomeSessions(ctx, pool); err != nil {
			return fmt.Errorf("remap outcome sessions: %w", err)
		}
	}

	if *migrateAll || *migrateScreeningSQLite {
		if err := migrateScreeningSQLiteData(ctx, pool, *sqlitePath); err != nil {
			return fmt.Errorf("migrate screening sqlite: %w", err)
		}
	}

	if *migrateAll || *migrateTradesSQLite {
		if err := migrateTradesSQLiteData(ctx, pool, *sqlitePath); err != nil {
			return fmt.Errorf("migrate trades sqlite: %w", err)
		}
	}

	if *migrateAll || *migrateExperimentsSQLite {
		if err := migrateExperimentsSQLiteData(ctx, pool, *sqlitePath); err != nil {
			return fmt.Errorf("migrate experiments sqlite: %w", err)
		}
	}

	if *migrateAll || *migrateSummariesSQLite {
		if err := migrateSummariesSQLiteData(ctx, pool, *sqlitePath); err != nil {
			return fmt.Errorf("migrate summaries sqlite: %w", err)
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
			"screening_total":     float64(s.ScreeningTotal),
			"screening_passed":    float64(s.ScreeningPassed),
			"screening_rate":      s.ScreeningRate,
			"alerts_triggered":    float64(s.AlertsTriggered),
			"alerts_acknowledged": float64(s.AlertsAcknowledged),
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
	var count int
	batchSize := 1000
	seen := make(map[string]bool)

	rootPath := stateDir + "/recommendation_outcomes.jsonl"
	if err := migrateOutcomesFile(ctx, pool, rootPath, seen, &count, batchSize); err != nil {
		log.Printf("Warning: root outcomes migration issue: %v", err)
	}

	sessionsDir := stateDir + "/sessions"
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No sessions directory found, skipping session outcomes")
		} else {
			log.Printf("Warning: failed to read sessions directory: %v", err)
		}
	} else {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			sessionPath := sessionsDir + "/" + entry.Name() + "/recommendation_outcomes.jsonl"
			if err := migrateOutcomesFile(ctx, pool, sessionPath, seen, &count, batchSize); err != nil {
				log.Printf("Warning: session %s outcomes migration issue: %v", entry.Name(), err)
			}
		}
	}

	log.Printf("Migrated %d recommendation outcomes", count)
	return nil
}

func migrateOutcomesFile(ctx context.Context, pool *pgxpool.Pool, filePath string, seen map[string]bool, count *int, batchSize int) error {
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	var batch []domain.RecommendationOutcome

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var outcome domain.RecommendationOutcome
		if err := json.Unmarshal(scanner.Bytes(), &outcome); err != nil {
			continue
		}

		key := outcome.AgentID + "|" + outcome.Symbol + "|" + outcome.Window + "|" + outcome.RecordedAt.Format(time.RFC3339)
		if seen[key] {
			continue
		}
		seen[key] = true

		batch = append(batch, outcome)

		if len(batch) >= batchSize {
			if err := insertOutcomeBatch(ctx, pool, batch); err != nil {
				log.Printf("Warning: failed to insert batch: %v", err)
			} else {
				*count += len(batch)
			}
			batch = batch[:0]
		}
	}

	// Insert remaining
	if len(batch) > 0 {
		if err := insertOutcomeBatch(ctx, pool, batch); err != nil {
			log.Printf("Warning: failed to insert final batch: %v", err)
		} else {
			*count += len(batch)
		}
	}

	return scanner.Err()
}

// insertOutcomeBatch inserts outcomes with session_id = o.Window (JSONL
// semantics — window-as-session key, later normalized by
// -remap-outcome-sessions) and a NOT EXISTS guard keyed on
// (session_id, symbol, agent_id, time) so re-running never duplicates a row.
func insertOutcomeBatch(ctx context.Context, pool *pgxpool.Pool, outcomes []domain.RecommendationOutcome) error {
	for _, o := range outcomes {
		metadata, _ := json.Marshal(o)
		_, err := pool.Exec(ctx, `
			INSERT INTO recommendation_outcomes (time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
			WHERE NOT EXISTS (
				SELECT 1 FROM recommendation_outcomes
				WHERE session_id = $2 AND symbol = $3 AND agent_id = $4 AND time = $1
			)
		`, o.RecordedAt, o.Window, o.Symbol, o.AgentID, string(o.Layer),
			o.Conviction, o.PassedGuards, o.GuardReason, o.Price, metadata)
		if err != nil {
			return err
		}
	}
	return nil
}

// insertOutcomeBatchGlobal inserts SQLite global-aggregate rows (session_id=”
// — mirror of SQLiteOutcomeStore.RecordOutcomes) with the same NOT EXISTS
// guard. Using ” (not o.Window) keeps the migration idempotent across
// re-runs: dated-window global rows no longer land as date-format session_ids
// that -remap-outcome-sessions would otherwise re-key and duplicate.
func insertOutcomeBatchGlobal(ctx context.Context, pool *pgxpool.Pool, outcomes []domain.RecommendationOutcome) (int, error) {
	var inserted int
	for _, o := range outcomes {
		metadata, _ := json.Marshal(o)
		tag, err := pool.Exec(ctx, `
			INSERT INTO recommendation_outcomes (time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
			WHERE NOT EXISTS (
				SELECT 1 FROM recommendation_outcomes
				WHERE session_id = $2 AND symbol = $3 AND agent_id = $4 AND time = $1
			)
		`, o.RecordedAt, "", o.Symbol, o.AgentID, string(o.Layer),
			o.Conviction, o.PassedGuards, o.GuardReason, o.Price, metadata)
		if err != nil {
			return 0, err
		}
		inserted += int(tag.RowsAffected())
	}
	return inserted, nil
}

func migrateScreeningRejectsData(ctx context.Context, pool *pgxpool.Pool, stateDir string) error {
	var count int
	batchSize := 1000
	sessionsDir := stateDir + "/sessions"
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No sessions directory found, skipping screening rejects")
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		filePath := sessionsDir + "/" + entry.Name() + "/screening_rejects.jsonl"
		f, err := os.Open(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			log.Printf("Warning: failed to open %s: %v", filePath, err)
			continue
		}

		var batch []domain.ScreeningReject
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var sr domain.ScreeningReject
			if err := json.Unmarshal(scanner.Bytes(), &sr); err != nil {
				continue
			}
			batch = append(batch, sr)
			if len(batch) >= batchSize {
				if err := insertScreeningRejectBatch(ctx, pool, batch); err != nil {
					log.Printf("Warning: failed to insert screening rejects batch: %v", err)
				} else {
					count += len(batch)
				}
				batch = batch[:0]
			}
		}
		if len(batch) > 0 {
			if err := insertScreeningRejectBatch(ctx, pool, batch); err != nil {
				log.Printf("Warning: failed to insert final screening rejects batch: %v", err)
			} else {
				count += len(batch)
			}
		}
		if err := f.Close(); err != nil {
			log.Printf("Warning: failed to close %s: %v", filePath, err)
		}
	}

	log.Printf("Migrated %d screening rejects", count)
	return nil
}

func insertScreeningRejectBatch(ctx context.Context, pool *pgxpool.Pool, rejects []domain.ScreeningReject) error {
	for _, sr := range rejects {
		factorScores, _ := json.Marshal(sr.FactorScores)
		// NOT EXISTS guard makes the batch idempotent (see insertOutcomeBatch).
		_, err := pool.Exec(ctx, `
			INSERT INTO screening_rejects (time, session_id, symbol, agent_id, skill, criterion, criterion_label, threshold, actual_value, factor_scores)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
			WHERE NOT EXISTS (
				SELECT 1 FROM screening_rejects
				WHERE session_id = $2 AND symbol = $3 AND agent_id = $4 AND time = $1
			)
		`, sr.RecordedAt, sr.SessionID, sr.Symbol, sr.AgentID, sr.Skill,
			sr.Criterion, sr.CriterionLabel, sr.Threshold, sr.ActualValue, factorScores)
		if err != nil {
			return err
		}
	}
	return nil
}

func migrateSessionSummariesData(ctx context.Context, pool *pgxpool.Pool, stateDir string) error {
	var count int
	sessionsDir := stateDir + "/sessions"
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No sessions directory found, skipping session summaries")
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		filePath := sessionsDir + "/" + entry.Name() + "/summary.json"
		data, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			log.Printf("Warning: failed to read %s: %v", filePath, err)
			continue
		}

		var summary domain.SessionSummary
		if err := json.Unmarshal(data, &summary); err != nil {
			log.Printf("Warning: failed to unmarshal %s: %v", filePath, err)
			continue
		}

		brokerRuntime, _ := json.Marshal(summary.BrokerRuntime)
		guardOutcomes, _ := json.Marshal(summary.GuardOutcomes)

		_, err = pool.Exec(ctx, `
			INSERT INTO session_summaries (time, session_id, regime, order_count, position_count, ending_cash, portfolio_value, outcome_count, broker_runtime, next_experiment_agent_id, proposal_id, commit_id, approval_id, guard_outcomes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (session_id) DO UPDATE SET
				time = EXCLUDED.time,
				regime = EXCLUDED.regime,
				order_count = EXCLUDED.order_count,
				position_count = EXCLUDED.position_count,
				ending_cash = EXCLUDED.ending_cash,
				portfolio_value = EXCLUDED.portfolio_value,
				outcome_count = EXCLUDED.outcome_count,
				broker_runtime = EXCLUDED.broker_runtime,
				next_experiment_agent_id = EXCLUDED.next_experiment_agent_id,
				proposal_id = EXCLUDED.proposal_id,
				commit_id = EXCLUDED.commit_id,
				approval_id = EXCLUDED.approval_id,
				guard_outcomes = EXCLUDED.guard_outcomes
		`, summary.RecordedAt, summary.SessionID, string(summary.Regime), summary.OrderCount,
			summary.PositionCount, summary.EndingCash, summary.PortfolioValue, summary.OutcomeCount,
			brokerRuntime, summary.NextExperimentAgentID, summary.ProposalID, summary.CommitID,
			summary.ApprovalID, guardOutcomes)
		if err != nil {
			log.Printf("Warning: failed to insert session summary %s: %v", summary.SessionID, err)
		} else {
			count++
		}
	}

	log.Printf("Migrated %d session summaries", count)
	return nil
}

func migrateHumanInterventionsData(ctx context.Context, pool *pgxpool.Pool, stateDir string) error {
	filePath := stateDir + "/human_interventions.jsonl"
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No human_interventions.jsonl found, skipping")
			return nil
		}
		return err
	}
	defer f.Close()

	var count int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var hi domain.HumanIntervention
		if err := json.Unmarshal(scanner.Bytes(), &hi); err != nil {
			continue
		}

		_, err := pool.Exec(ctx, `
			INSERT INTO human_interventions (time, intervention_id, type, target_agent_id, target_model_id, target_sector, target_symbol, value, reason, operator, session_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (intervention_id) DO NOTHING
		`, hi.RecordedAt, hi.ID, hi.Type, hi.TargetAgentID, hi.TargetModelID,
			hi.TargetSector, hi.TargetSymbol, hi.Value, hi.Reason, hi.Operator, hi.SessionID)
		if err != nil {
			log.Printf("Warning: failed to insert human intervention %s: %v", hi.ID, err)
		} else {
			count++
		}
	}

	log.Printf("Migrated %d human interventions", count)
	return scanner.Err()
}

func migrateCapitalFlowData(ctx context.Context, pool *pgxpool.Pool, stateDir string) error {
	dir := stateDir + "/capital_flow"
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No capital_flow directory found, skipping")
			return nil
		}
		return err
	}

	var count int
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Warning: failed to read %s: %v", entry.Name(), err)
			continue
		}
		var flow marketdata.TWSECapitalFlow
		if err := json.Unmarshal(data, &flow); err != nil {
			log.Printf("Warning: failed to unmarshal %s: %v", entry.Name(), err)
			continue
		}

		// ForeignInvestorNet = net buy, total_buy/total_sell unknown from snapshot
		// Use net as approximation; Store needs channel label
		for _, channel := range []string{"foreign", "domestic", "dealer"} {
			var net float64
			switch channel {
			case "foreign":
				net = flow.ForeignInvestorNet
			case "domestic":
				net = flow.DomesticFundNet
			case "dealer":
				net = flow.DealerNet
			}
			_, err := pool.Exec(ctx, `
				INSERT INTO capital_flow (time, channel, net_buy, total_buy, total_sell)
				VALUES ($1, $2, $3, 0, 0)
			`, flow.Date, channel, net)
			if err != nil {
				log.Printf("Warning: failed to insert capital flow %s/%s: %v", flow.Date, channel, err)
			} else {
				count++
			}
		}
	}

	log.Printf("Migrated %d capital flow records", count)
	return nil
}

func migrateExportStatsData(ctx context.Context, pool *pgxpool.Pool, stateDir string) error {
	dir := stateDir + "/export"
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No export directory found, skipping")
			return nil
		}
		return err
	}

	var count int
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_export.json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Warning: failed to read %s: %v", entry.Name(), err)
			continue
		}
		var exp marketdata.CustomsExportImport
		if err := json.Unmarshal(data, &exp); err != nil {
			log.Printf("Warning: failed to unmarshal %s: %v", entry.Name(), err)
			continue
		}

		gregorianYear := exp.Year + 1911
		ts := time.Date(gregorianYear, time.Month(exp.Month), 1, 0, 0, 0, 0, time.UTC)

		_, err = pool.Exec(ctx, `
			INSERT INTO export_statistics (time, year, month, export_total, import_total, trade_balance)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, ts, exp.Year, exp.Month, exp.ExportTotal, exp.ImportTotal, exp.TradeBalance)
		if err != nil {
			log.Printf("Warning: failed to insert export stats %s: %v", entry.Name(), err)
		} else {
			count++
		}
	}

	log.Printf("Migrated %d export statistics records", count)
	return nil
}

// migrateHistoricalData copies the Stage 4 historical tables
// (regime/stress/geopolitical/period) from the source SQLite atlas.db into
// PostgreSQL. Data volume is small (tens to low hundreds of rows); rows are
// upserted idempotently so re-running is safe. Migration is the one-time
// data move for StoreBackend=postgres; afterwards the live pipeline writes
// straight to postgres.
func migrateHistoricalData(ctx context.Context, pool *pgxpool.Pool, sqlitePath string) error {
	sqliteDB, err := ledger.OpenSQLiteDB(sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", sqlitePath, err)
	}
	defer sqliteDB.Close()

	hist := ledger.NewSQLiteHistoricalStore(sqliteDB)

	// Regime
	regimes, err := hist.LoadRegimeHistoryAll(ctx, 100000)
	if err != nil {
		return fmt.Errorf("load regimes: %w", err)
	}
	pgHist := ledger.NewPostgresHistoricalStore(pool)
	for _, r := range regimes {
		if err := pgHist.UpsertRegime(ctx, r); err != nil {
			return fmt.Errorf("migrate regime %s: %w", r.Date, err)
		}
	}
	log.Printf("Migrated %d regime_history rows", len(regimes))

	// Stress
	stresses, err := hist.LoadStressHistoryAll(ctx, 100000)
	if err != nil {
		return fmt.Errorf("load stresses: %w", err)
	}
	for _, r := range stresses {
		if err := pgHist.UpsertStress(ctx, r); err != nil {
			return fmt.Errorf("migrate stress %s: %w", r.Date, err)
		}
	}
	log.Printf("Migrated %d stress_index_history rows", len(stresses))

	// Geopolitical
	geos, err := hist.LoadGeopoliticalHistoryAll(ctx, 100000)
	if err != nil {
		return fmt.Errorf("load geopolitical: %w", err)
	}
	for _, r := range geos {
		if err := pgHist.UpsertGeopolitical(ctx, r); err != nil {
			return fmt.Errorf("migrate geopolitical %s: %w", r.Date, err)
		}
	}
	log.Printf("Migrated %d geopolitical_history rows", len(geos))

	// Period
	periods, err := hist.LoadPeriodHistoryAll(ctx, 100000)
	if err != nil {
		return fmt.Errorf("load periods: %w", err)
	}
	for _, r := range periods {
		if err := pgHist.UpsertPeriod(ctx, r); err != nil {
			return fmt.Errorf("migrate period %s: %w", r.Date, err)
		}
	}
	log.Printf("Migrated %d period_history rows", len(periods))

	// Event calendar (M2: previously skipped; 0 rows in SQLite but part of
	// the complete historical surface).
	events, err := hist.LoadEventCalendarRangeAll(ctx, "0000-01-01", "9999-12-31", 100000)
	if err != nil {
		return fmt.Errorf("load event calendar: %w", err)
	}
	for _, r := range events {
		if err := pgHist.UpsertEventCalendar(ctx, r); err != nil {
			return fmt.Errorf("migrate event calendar %s/%s: %w", r.Date, r.EventID, err)
		}
	}
	log.Printf("Migrated %d event_calendar_history rows", len(events))

	// Prediction backtest (M2: 2 rows in SQLite).
	sqlitePredictions, err := hist.LoadPredictionBacktestRangeAll(ctx, "0000-01-01", "9999-12-31", 100000)
	if err != nil {
		return fmt.Errorf("load sqlite prediction backtest: %w", err)
	}
	for _, r := range sqlitePredictions {
		if err := pgHist.UpsertPredictionBacktest(ctx, r); err != nil {
			return fmt.Errorf("migrate prediction backtest %s: %w", r.Date, err)
		}
	}
	log.Printf("Migrated %d prediction_backtest rows", len(sqlitePredictions))

	return nil
}

// migrateQuotesData copies the quotes table (symbol/name/date/open/high/low/
// close/volume/source) from the source SQLite atlas.db into PostgreSQL.
// SQLite is the only complete quotes source (66,959 rows vs ~6.5k in the
// JSONL backfill — M4). Rows are upserted on (symbol, date) so re-running
// is idempotent. Batched per 1000 rows.
func migrateQuotesData(ctx context.Context, pool *pgxpool.Pool, sqlitePath string) error {
	sqliteDB, err := ledger.OpenSQLiteDB(sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", sqlitePath, err)
	}
	defer sqliteDB.Close()

	rows, err := sqliteDB.Query(`
		SELECT symbol, name, date, open, high, low, close, volume, source
		FROM quotes ORDER BY id`)
	if err != nil {
		return fmt.Errorf("query sqlite quotes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type quoteRow struct {
		symbol, name, date, source string
		open, high, low, close     float64
		volume                     int64
	}

	var (
		batch []quoteRow
		count int
	)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		for _, q := range batch {
			_, err := pool.Exec(ctx, `
				INSERT INTO quotes (symbol, name, date, open, high, low, close, volume, source)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				ON CONFLICT (symbol, date) DO UPDATE SET
					name = excluded.name,
					open = excluded.open,
					high = excluded.high,
					low = excluded.low,
					close = excluded.close,
					volume = excluded.volume,
					source = excluded.source
			`, q.symbol, q.name, q.date, q.open, q.high, q.low, q.close, q.volume, q.source)
			if err != nil {
				return fmt.Errorf("insert quote %s/%s: %w", q.symbol, q.date, err)
			}
		}
		count += len(batch)
		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		var q quoteRow
		var name, source sql.NullString
		if err := rows.Scan(&q.symbol, &name, &q.date, &q.open, &q.high, &q.low, &q.close, &q.volume, &source); err != nil {
			return fmt.Errorf("scan quote row: %w", err)
		}
		q.name = name.String
		q.source = source.String
		batch = append(batch, q)
		if len(batch) >= 1000 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite quotes iteration: %w", err)
	}
	if err := flush(); err != nil {
		return err
	}

	log.Printf("Migrated %d quotes rows", count)
	return nil
}

// migrateDetectorScansData copies the detector_scan_log table
// (scan_id/scan_batch_id/theme/severity/confidence/detected_at/source/
// metadata_json) from the source SQLite atlas.db into PostgreSQL.
// scan_id is carried explicitly to preserve ORDER BY scan_id DESC semantics;
// ON CONFLICT (scan_id) DO NOTHING makes re-running idempotent.
func migrateDetectorScansData(ctx context.Context, pool *pgxpool.Pool, sqlitePath string) error {
	sqliteDB, err := ledger.OpenSQLiteDB(sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", sqlitePath, err)
	}
	defer sqliteDB.Close()

	rows, err := sqliteDB.Query(`
		SELECT scan_id, scan_batch_id, theme, severity, confidence, detected_at, source, metadata_json
		FROM detector_scan_log ORDER BY scan_id`)
	if err != nil {
		return fmt.Errorf("query sqlite detector_scan_log: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type scanRow struct {
		scanID                                           int64
		scanBatchID, theme, severity, detectedAt, source string
		confidence                                       float64
		metadataJSON                                     sql.NullString
	}

	var (
		batch []scanRow
		count int
	)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		for _, r := range batch {
			_, err := pool.Exec(ctx, `
				INSERT INTO detector_scan_log (scan_id, scan_batch_id, theme, severity, confidence, detected_at, source, metadata_json)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (scan_id) DO NOTHING
			`, r.scanID, r.scanBatchID, r.theme, r.severity, r.confidence,
				r.detectedAt, r.source, r.metadataJSON)
			if err != nil {
				return fmt.Errorf("insert detector scan %d: %w", r.scanID, err)
			}
		}
		count += len(batch)
		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		var r scanRow
		if err := rows.Scan(&r.scanID, &r.scanBatchID, &r.theme, &r.severity,
			&r.confidence, &r.detectedAt, &r.source, &r.metadataJSON); err != nil {
			return fmt.Errorf("scan detector_scan_log row: %w", err)
		}
		batch = append(batch, r)
		if len(batch) >= 1000 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite detector_scan_log iteration: %w", err)
	}
	if err := flush(); err != nil {
		return err
	}

	// Explicit scan_id inserts do not advance the BIGSERIAL sequence; bump it
	// so subsequent AppendScan inserts never collide with migrated scan_ids.
	if _, err := pool.Exec(ctx, `SELECT setval('detector_scan_log_scan_id_seq', (SELECT COALESCE(MAX(scan_id), 1) FROM detector_scan_log))`); err != nil {
		return fmt.Errorf("advance detector_scan_log sequence: %w", err)
	}

	log.Printf("Migrated %d detector_scan_log rows", count)
	return nil
}

// migrateOutcomesSQLiteData copies outcomes from the source SQLite atlas.db
// into PostgreSQL. SQLite is the only complete outcomes source (5,997 rows
// vs 4,164 in per-session JSONL — M5); LoadOutcomes() returns the full
// 21-column domain objects. Rows go through insertOutcomeBatch (NOT EXISTS
// guard) so re-running is idempotent and never duplicates JSONL-migrated rows.
func migrateOutcomesSQLiteData(ctx context.Context, pool *pgxpool.Pool, sqlitePath string) error {
	sqliteDB, err := ledger.OpenSQLiteDB(sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", sqlitePath, err)
	}
	defer sqliteDB.Close()

	store := ledger.NewSQLiteOutcomeStore(sqliteDB)
	outcomes, err := store.LoadOutcomes()
	if err != nil {
		return fmt.Errorf("load sqlite outcomes: %w", err)
	}
	if len(outcomes) == 0 {
		log.Println("No sqlite outcomes to migrate, skipping")
		return nil
	}

	const batchSize = 1000
	var count int
	for start := 0; start < len(outcomes); start += batchSize {
		end := min(start+batchSize, len(outcomes))
		inserted, err := insertOutcomeBatchGlobal(ctx, pool, outcomes[start:end])
		if err != nil {
			return fmt.Errorf("insert outcome batch: %w", err)
		}
		count += inserted
	}
	log.Printf("Migrated %d sqlite global outcomes rows (%d guard-skipped duplicates)", count, len(outcomes)-count)
	return nil
}

// migrateOutcomesSQLiteSessions copies session-scoped outcomes (session_id != ”)
// from the source SQLite atlas.db into PostgreSQL, preserving the original
// session_id (session-YYYYMMDD-daily). This is the A01 gap: the existing
// migrateOutcomesSQLiteData only reads global rows via LoadOutcomes()
// (WHERE session_id = ”), so the 2,965 session-scoped rows were never migrated.
//
// scanOutcomes does not preserve the session_id column, so rows are scanned with
// a dedicated row iterator here. The SQLite outcomes table is a recording log —
// the same logical outcome (session_id, symbol, agent_id, time) is recorded many
// times with evolving state (passed_guards, conviction, window). For each key we
// keep the FINAL recording (MAX id) so passed_guards reflects the last state,
// then insert with the NOT EXISTS guard (idempotent, no ghost rows).
func migrateOutcomesSQLiteSessions(ctx context.Context, pool *pgxpool.Pool, sqlitePath string) error {
	sqliteDB, err := ledger.OpenSQLiteDB(sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", sqlitePath, err)
	}
	defer sqliteDB.Close()

	const query = `
		SELECT o.session_id, o.symbol, o.agent_id, o.action, o.target_price, o.stop_loss,
			o.conviction, o.regime, o.timestamp, o.passed_guards, o.guard_reason,
			o.factor_scores_json, o.conviction_breakdown_json,
			o.layer, o.forward_return, o.window, o.hit, o.benchmark_delta, o.is_synthetic, o.true_regime
		FROM outcomes o
		JOIN (
			SELECT session_id, symbol, agent_id, timestamp, MAX(id) AS max_id
			FROM outcomes
			WHERE session_id != ''
			GROUP BY session_id, symbol, agent_id, timestamp
		) m ON o.id = m.max_id
		ORDER BY o.session_id`

	rows, err := sqliteDB.Query(query)
	if err != nil {
		return fmt.Errorf("query sqlite session outcomes: %w", err)
	}
	defer rows.Close()

	var batch []domain.RecommendationOutcome
	var batchSessions []string
	const batchSize = 500

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		inserted, err := insertSessionOutcomeBatch(ctx, pool, batchSessions, batch)
		if err != nil {
			return err
		}
		log.Printf("Migrated %d sqlite session outcomes rows (%d guard-skipped duplicates)", inserted, len(batch)-inserted)
		batch = batch[:0]
		batchSessions = batchSessions[:0]
		return nil
	}

	for rows.Next() {
		var sessionID, sym, agentID string
		var action, regime, ts, guardReason, factorJSON, convictionJSON sql.NullString
		var passedGuards bool
		var targetPrice, stopLoss, forwardReturn, benchmarkDelta sql.NullFloat64
		var conviction, hit, isSynthetic sql.NullInt64
		var layer, window, trueRegime sql.NullString

		if err := rows.Scan(&sessionID, &sym, &agentID, &action, &targetPrice, &stopLoss,
			&conviction, &regime, &ts, &passedGuards, &guardReason, &factorJSON, &convictionJSON,
			&layer, &forwardReturn, &window, &hit, &benchmarkDelta, &isSynthetic, &trueRegime); err != nil {
			return fmt.Errorf("scan session outcome row: %w", err)
		}

		var fs domain.FactorScores
		if factorJSON.String != "" {
			if err := json.Unmarshal([]byte(factorJSON.String), &fs); err != nil {
				return fmt.Errorf("unmarshal factor_scores: %w", err)
			}
		}
		var cb *domain.ConvictionBreakdown
		if convictionJSON.String != "" {
			var breakdown domain.ConvictionBreakdown
			if err := json.Unmarshal([]byte(convictionJSON.String), &breakdown); err != nil {
				return fmt.Errorf("unmarshal conviction_breakdown: %w", err)
			}
			cb = &breakdown
		}

		effectiveLayer := layer.String
		if effectiveLayer == "" {
			effectiveLayer = regime.String
		}

		batch = append(batch, domain.RecommendationOutcome{
			AgentID:             agentID,
			Symbol:              sym,
			Side:                domain.Side(action.String),
			TargetPrice:         targetPrice.Float64,
			StopLossPrice:       stopLoss.Float64,
			Conviction:          int(conviction.Int64),
			Layer:               domain.AgentLayer(effectiveLayer),
			Regime:              trueRegime.String,
			Window:              window.String,
			ForwardReturn:       forwardReturn.Float64,
			BenchmarkDelta:      benchmarkDelta.Float64,
			Hit:                 hit.Int64 == 1,
			IsSynthetic:         isSynthetic.Int64 == 1,
			RecordedAt:          parseTimestamp(ts.String),
			PassedGuards:        passedGuards,
			GuardReason:         guardReason.String,
			FactorScores:        fs,
			ConvictionBreakdown: cb,
		})
		batchSessions = append(batchSessions, sessionID)

		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session outcomes: %w", err)
	}
	if err := flush(); err != nil {
		return err
	}
	return nil
}

// insertSessionOutcomeBatch inserts session-scoped outcomes under their original
// session_id (session-YYYYMMDD-daily) with the same NOT EXISTS guard as
// insertOutcomeBatch, so re-running the migration never duplicates a row and
// never creates rows for sessions that do not exist in the source. It returns
// the number of rows actually inserted (RowsAffected — 0 for guard-skipped).
func insertSessionOutcomeBatch(ctx context.Context, pool *pgxpool.Pool, sessionIDs []string, outcomes []domain.RecommendationOutcome) (int, error) {
	if len(outcomes) != len(sessionIDs) {
		return 0, fmt.Errorf("insertSessionOutcomeBatch: %d outcomes vs %d session ids", len(outcomes), len(sessionIDs))
	}
	var inserted int
	for i, o := range outcomes {
		metadata, _ := json.Marshal(o)
		tag, err := pool.Exec(ctx, `
			INSERT INTO recommendation_outcomes (time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
			WHERE NOT EXISTS (
				SELECT 1 FROM recommendation_outcomes
				WHERE session_id = $2 AND symbol = $3 AND agent_id = $4 AND time = $1
			)
		`, o.RecordedAt, sessionIDs[i], o.Symbol, o.AgentID, string(o.Layer),
			o.Conviction, o.PassedGuards, o.GuardReason, o.Price, metadata)
		if err != nil {
			return 0, err
		}
		inserted += int(tag.RowsAffected())
	}
	return inserted, nil
}

// remapOutcomeSessions rewrites date-format session_id values (e.g. "2026-07-22")
// — which were written by insertOutcomeBatch/RecordOutcomes using o.Window as
// session_id — to the unified session-YYYYMMDD-daily format matching
// session_summaries.session_id. Q2 audit verified every date maps to a real
// session (24/25 have session dirs; 2026-07-21 has 375 SQLite session-scoped rows
// as provenance), so no ghost sessions are created. metadata.window is untouched
// (it is the original evaluation date — provenance). Idempotent: re-running
// matches zero rows.
func remapOutcomeSessions(ctx context.Context, execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}) error {
	tag, err := execer.Exec(ctx, `
		UPDATE recommendation_outcomes
		SET session_id = 'session-' || replace(session_id, '-', '') || '-daily'
		WHERE session_id ~ '^\d{4}-\d{2}-\d{2}$'`)
	if err != nil {
		return fmt.Errorf("remap outcome sessions: %w", err)
	}
	log.Printf("Remapped %d date-format outcome session_id rows to session-YYYYMMDD-daily", tag.RowsAffected())
	return nil
}

// migrateScreeningSQLiteData copies screening rejects from the source SQLite
// atlas.db into PostgreSQL. SQLite is the only complete source (20,423 rows
// vs 1,155 in JSONL — M6). Old SQLite rows have no agent_id; the PG
// agent_id NOT NULL column receives "legacy". Rows go through
// insertScreeningRejectBatch (NOT EXISTS guard) for idempotency.
func migrateScreeningSQLiteData(ctx context.Context, pool *pgxpool.Pool, sqlitePath string) error {
	sqliteDB, err := ledger.OpenSQLiteDB(sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", sqlitePath, err)
	}
	defer sqliteDB.Close()

	store := ledger.NewSQLiteOutcomeStore(sqliteDB)

	sessionRows, err := sqliteDB.Query(`SELECT DISTINCT session_id FROM screening_rejects`)
	if err != nil {
		return fmt.Errorf("query sqlite screening sessions: %w", err)
	}
	var sessionIDs []string
	for sessionRows.Next() {
		var sid string
		if err := sessionRows.Scan(&sid); err != nil {
			_ = sessionRows.Close()
			return fmt.Errorf("scan screening session: %w", err)
		}
		sessionIDs = append(sessionIDs, sid)
	}
	_ = sessionRows.Close()
	if err := sessionRows.Err(); err != nil {
		return fmt.Errorf("screening sessions iteration: %w", err)
	}

	const batchSize = 1000
	var count int
	var batch []domain.ScreeningReject
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := insertScreeningRejectBatch(ctx, pool, batch); err != nil {
			return err
		}
		count += len(batch)
		batch = batch[:0]
		return nil
	}

	for _, sid := range sessionIDs {
		rejects, err := store.LoadSessionScreeningRejects(sid)
		if err != nil {
			return fmt.Errorf("load screening rejects for %s: %w", sid, err)
		}
		for _, r := range rejects {
			r.AgentID = "legacy"
			batch = append(batch, r)
			if len(batch) >= batchSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}

	log.Printf("Migrated %d sqlite screening rejects rows", count)
	return nil
}

// migrateTradesSQLiteData copies trades from the source SQLite atlas.db into
// PostgreSQL. Evidence E5: all 51 SQLite trades (6 unique trade_ids, 6
// sessions with no trades.jsonl on disk) are absent from JSONL — SQLite is
// the only source. trade_id is not unique in SQLite (repeated daily runs
// re-insert), so the NOT EXISTS guard keys on (trade_id, session_id) and
// keeps the first occurrence per run; re-running is idempotent.
func migrateTradesSQLiteData(ctx context.Context, pool *pgxpool.Pool, sqlitePath string) error {
	sqliteDB, err := ledger.OpenSQLiteDB(sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", sqlitePath, err)
	}
	defer sqliteDB.Close()

	rows, err := sqliteDB.Query(`
		SELECT trade_id, session_id, symbol, side, quantity, price, amount, reason, timestamp
		FROM trades ORDER BY id`)
	if err != nil {
		return fmt.Errorf("query sqlite trades: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type tradeRow struct {
		tradeID, sessionID, symbol, side, reason, timestamp string
		quantity                                            int
		price, amount                                       float64
	}

	var (
		batch []tradeRow
		count int
	)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		for _, t := range batch {
			_, err := pool.Exec(ctx, `
				INSERT INTO trades (trade_id, session_id, symbol, side, quantity, price, amount, reason, timestamp)
				SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9
				WHERE NOT EXISTS (
					SELECT 1 FROM trades WHERE trade_id = $1 AND session_id = $2
				)
			`, t.tradeID, t.sessionID, t.symbol, t.side, t.quantity, t.price, t.amount, t.reason, t.timestamp)
			if err != nil {
				return fmt.Errorf("insert trade %s: %w", t.tradeID, err)
			}
		}
		count += len(batch)
		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		var t tradeRow
		if err := rows.Scan(&t.tradeID, &t.sessionID, &t.symbol, &t.side,
			&t.quantity, &t.price, &t.amount, &t.reason, &t.timestamp); err != nil {
			return fmt.Errorf("scan trade row: %w", err)
		}
		batch = append(batch, t)
		if len(batch) >= 1000 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite trades iteration: %w", err)
	}
	if err := flush(); err != nil {
		return err
	}

	log.Printf("Migrated %d sqlite trades rows", count)
	return nil
}

// migrateExperimentsSQLiteData copies experiments from the source SQLite
// atlas.db into PostgreSQL. Evidence E4/C1: the SQLite experiment_id set
// (984 rows) is disjoint from experiments.jsonl (1,153 rows), so JSONL
// migration cannot cover them. ON CONFLICT (experiment_id) DO NOTHING keeps
// re-runs idempotent.
func migrateExperimentsSQLiteData(ctx context.Context, pool *pgxpool.Pool, sqlitePath string) error {
	full, err := ledger.NewFullStore(config.Config{StoreBackend: "sqlite", SQLitePath: sqlitePath})
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", sqlitePath, err)
	}
	store, ok := full.(ledger.ExperimentStore)
	if !ok {
		return fmt.Errorf("sqlite full store does not expose ExperimentStore")
	}
	records, err := store.LoadExperiments()
	if err != nil {
		return fmt.Errorf("load sqlite experiments: %w", err)
	}
	if len(records) == 0 {
		log.Println("No sqlite experiments to migrate, skipping")
		return nil
	}

	var count int
	for _, rec := range records {
		briefJSON, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("marshal experiment %s: %w", rec.ID, err)
		}
		_, err = pool.Exec(ctx, `
			INSERT INTO experiments (experiment_id, mutation_brief_json, accepted, timestamp)
			SELECT $1, $2, $3, $4
			WHERE NOT EXISTS (SELECT 1 FROM experiments WHERE experiment_id = $1)
		`, rec.ID, string(briefJSON), boolToInt(rec.Status == domain.ExperimentAccepted),
			rec.WindowStart.Format("2006-01-02T15:04:05Z07:00"))
		if err != nil {
			return fmt.Errorf("insert experiment %s: %w", rec.ID, err)
		}
		count++
	}
	log.Printf("Migrated %d sqlite experiments rows", count)
	return nil
}

// migrateSummariesSQLiteData copies session summaries from the source SQLite
// atlas.db into PostgreSQL. Evidence E6/C2: 6 SQLite summaries
// (session-20260721/25/26-daily, 0801/0802/0812-daily) have no session dir
// on disk, so JSONL migration cannot cover them. Reuses the same
// INSERT ... ON CONFLICT (session_id) DO UPDATE SQL as the JSONL path.
func migrateSummariesSQLiteData(ctx context.Context, pool *pgxpool.Pool, sqlitePath string) error {
	sqliteDB, err := ledger.OpenSQLiteDB(sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", sqlitePath, err)
	}
	defer sqliteDB.Close()

	store := ledger.NewSQLiteOutcomeStore(sqliteDB)
	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		return fmt.Errorf("load sqlite session summaries: %w", err)
	}
	if len(summaries) == 0 {
		log.Println("No sqlite session summaries to migrate, skipping")
		return nil
	}

	var count int
	for _, summary := range summaries {
		brokerRuntime, _ := json.Marshal(summary.BrokerRuntime)
		guardOutcomes, _ := json.Marshal(summary.GuardOutcomes)

		_, err := pool.Exec(ctx, `
			INSERT INTO session_summaries (time, session_id, regime, order_count, position_count, ending_cash, portfolio_value, outcome_count, broker_runtime, next_experiment_agent_id, proposal_id, commit_id, approval_id, guard_outcomes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (session_id) DO UPDATE SET
				time = EXCLUDED.time,
				regime = EXCLUDED.regime,
				order_count = EXCLUDED.order_count,
				position_count = EXCLUDED.position_count,
				ending_cash = EXCLUDED.ending_cash,
				portfolio_value = EXCLUDED.portfolio_value,
				outcome_count = EXCLUDED.outcome_count,
				broker_runtime = EXCLUDED.broker_runtime,
				next_experiment_agent_id = EXCLUDED.next_experiment_agent_id,
				proposal_id = EXCLUDED.proposal_id,
				commit_id = EXCLUDED.commit_id,
				approval_id = EXCLUDED.approval_id,
				guard_outcomes = EXCLUDED.guard_outcomes
		`, summary.RecordedAt, summary.SessionID, string(summary.Regime), summary.OrderCount,
			summary.PositionCount, summary.EndingCash, summary.PortfolioValue, summary.OutcomeCount,
			brokerRuntime, summary.NextExperimentAgentID, summary.ProposalID, summary.CommitID,
			summary.ApprovalID, guardOutcomes)
		if err != nil {
			return fmt.Errorf("insert session summary %s: %w", summary.SessionID, err)
		}
		count++
	}
	log.Printf("Migrated %d sqlite session summaries rows", count)
	return nil
}

// boolToInt converts a bool to 0/1 for INTEGER columns.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// parseTimestamp parses ISO8601 timestamps from SQLite (mirror of the
// ledger package helper; unexported there so duplicated locally).
func parseTimestamp(s string) (t time.Time) {
	t, _ = time.Parse("2006-01-02T15:04:05Z07:00", s) //nolint:errcheck
	return t
}
