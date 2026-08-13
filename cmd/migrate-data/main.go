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
		migrateMetrics       = flag.Bool("metrics", false, "Migrate metrics data")
		migrateAlerts        = flag.Bool("alerts", false, "Migrate alerts data")
		migrateOutcomes      = flag.Bool("outcomes", false, "Migrate recommendation outcomes")
		migrateScreening     = flag.Bool("screening", false, "Migrate screening rejects")
		migrateSummaries     = flag.Bool("summaries", false, "Migrate session summaries")
		migrateInterventions = flag.Bool("interventions", false, "Migrate human interventions")
		migrateCapitalFlow   = flag.Bool("capitalflow", false, "Migrate capital flow data")
		migrateExportStats   = flag.Bool("exportstats", false, "Migrate export statistics")
		migrateHistorical    = flag.Bool("historical", false, "Migrate historical tables (regime/stress/geo/period) from SQLite atlas.db")
		migrateQuotes        = flag.Bool("quotes", false, "Migrate quotes from SQLite atlas.db")
		sqlitePath           = flag.String("sqlite-path", "data/state/atlas.db", "source SQLite atlas.db path for -historical")
		migrateAll           = flag.Bool("all", false, "Migrate all data")
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
		_, err := pool.Exec(ctx, `
			INSERT INTO screening_rejects (time, session_id, symbol, agent_id, skill, criterion, criterion_label, threshold, actual_value, factor_scores)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
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
