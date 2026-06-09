package repository

import (
	"context"
	"fmt"
	"time"
)

// ============================================
// Capital Flow Repository Implementation
// ============================================

func (r *PostgresRepository) RecordCapitalFlow(ctx context.Context, channel string, netBuy, totalBuy, totalSell float64) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO capital_flow (time, channel, net_buy, total_buy, total_sell)
		VALUES (NOW(), $1, $2, $3, $4)
	`, channel, netBuy, totalBuy, totalSell)
	if err != nil {
		return fmt.Errorf("insert capital flow: %w", err)
	}
	return nil
}

func (r *PostgresRepository) QueryLatestCapitalFlow(ctx context.Context, channel string) (*CapitalFlowRecord, error) {
	var rec CapitalFlowRecord
	err := r.pool.QueryRow(ctx, `
		SELECT time, channel, net_buy, total_buy, total_sell
		FROM capital_flow
		WHERE channel = $1
		ORDER BY time DESC
		LIMIT 1
	`, channel).Scan(&rec.Time, &rec.Channel, &rec.NetBuy, &rec.TotalBuy, &rec.TotalSell)
	if err != nil {
		return nil, err
	}

	return &rec, nil
}

func (r *PostgresRepository) QueryCapitalFlowRange(ctx context.Context, channel string, start, end time.Time) ([]CapitalFlowRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT time, channel, net_buy, total_buy, total_sell
		FROM capital_flow
		WHERE channel = $1 AND time >= $2 AND time <= $3
		ORDER BY time DESC
	`, channel, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CapitalFlowRecord
	for rows.Next() {
		var rec CapitalFlowRecord
		if err := rows.Scan(&rec.Time, &rec.Channel, &rec.NetBuy, &rec.TotalBuy, &rec.TotalSell); err != nil {
			continue
		}
		records = append(records, rec)
	}

	return records, rows.Err()
}

// ============================================
// Export Stats Repository Implementation
// ============================================

func (r *PostgresRepository) SaveExportStats(ctx context.Context, year, month int, exportTotal, importTotal, tradeBalance float64) error {
	// Convert ROC year to Gregorian for timestamp
	gregorianYear := year + 1911
	ts := time.Date(gregorianYear, time.Month(month), 1, 0, 0, 0, 0, time.UTC)

	// Use explicit transaction rather than a single CTE.
	//
	// A CTE that mixes DELETE and INSERT against a TimescaleDB hypertable
	// can fail to expose the newly inserted row to a subsequent
	// `WHERE year=$ AND month=$` SELECT in TimescaleDB 2.14.x (CI image
	// timescale/timescaledb:2.14.2-pg15). The DELETE/INSERT are planned
	// independently on the access node and the just-inserted row is not
	// visible to a same-transaction read that scans by non-partition
	// columns. Splitting into a real transaction with separate DELETE
	// and INSERT statements forces a single execution context, which
	// is what subsequent same-tx reads expect.
	//
	// A unique index on (year, month) is not an option here because
	// unique indexes on a hypertable must include the partition key
	// `time`, and (time, year, month) is not naturally unique (multiple
	// writes to the same month are upserts).
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save export stats: %w", err)
	}
	// Rollback is a no-op after a successful Commit; error is not
	// actionable at this point.
	//nolint:errcheck
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`DELETE FROM export_statistics WHERE year = $1 AND month = $2`,
		year, month,
	); err != nil {
		return fmt.Errorf("delete export stats: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO export_statistics (time, year, month, export_total, import_total, trade_balance)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		ts, year, month, exportTotal, importTotal, tradeBalance,
	); err != nil {
		return fmt.Errorf("insert export stats: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit save export stats: %w", err)
	}
	return nil
}

func (r *PostgresRepository) QueryLatestExportStats(ctx context.Context) (*ExportStatsRecord, error) {
	var rec ExportStatsRecord
	err := r.pool.QueryRow(ctx, `
		SELECT time, year, month, export_total, import_total, trade_balance
		FROM export_statistics
		ORDER BY time DESC
		LIMIT 1
	`).Scan(&rec.Time, &rec.Year, &rec.Month, &rec.ExportTotal, &rec.ImportTotal, &rec.TradeBalance)
	if err != nil {
		return nil, err
	}

	return &rec, nil
}

func (r *PostgresRepository) QueryExportStatsByYearMonth(ctx context.Context, year, month int) (*ExportStatsRecord, error) {
	var rec ExportStatsRecord
	err := r.pool.QueryRow(ctx, `
		SELECT time, year, month, export_total, import_total, trade_balance
		FROM export_statistics
		WHERE year = $1 AND month = $2
	`, year, month).Scan(&rec.Time, &rec.Year, &rec.Month, &rec.ExportTotal, &rec.ImportTotal, &rec.TradeBalance)
	if err != nil {
		return nil, err
	}

	return &rec, nil
}
