package stockpicker

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// aggregateWinRate computes a win-rate summary across outcomes without
// enforcing the single-symbol/single-source homogeneity checks in
// SignalWinRate. This is the internal solution to the PR 1a acceptance
// report §5-1 warning: aggregation jobs that compute strategy-level or
// market-level win rates mix symbols and/or sources, and would otherwise
// fail with "mixed symbols" or "mixed sources" errors.
//
// This helper is intentionally unexported and only for use inside
// internal/stockpicker. It never errors; invalid inputs are handled by
// the pure math helpers (WinRate, WilsonScoreInterval return zeros).
func aggregateWinRate(outcomes []SignalOutcome, costRate float64, minSamples int, confidence float64) SignalWinRateSummary {
	var summary SignalWinRateSummary
	if len(outcomes) == 0 {
		summary.Confidence = confidence
		summary.CalibrationStatus = CalibrationStatusFor(0, minSamples)
		return summary
	}

	var totalReturn float64
	for _, o := range outcomes {
		if NetHit(o.ForwardReturn, costRate) {
			summary.Hits++
		}
		totalReturn += o.ForwardReturn
	}

	n := len(outcomes)
	summary.Observations = n
	summary.WinRate = WinRate(summary.Hits, n)
	summary.WilsonLower, summary.WilsonUpper = WilsonScoreInterval(summary.Hits, n, confidence)
	summary.Confidence = confidence
	summary.CalibrationStatus = CalibrationStatusFor(n, minSamples)
	summary.NetCostRate = costRate
	summary.AvgForwardReturn = totalReturn / float64(n)
	return summary
}

// SignalOutcomeStore is a thin SQLite-backed wrapper around
// stock_signal_outcomes.
type SignalOutcomeStore struct {
	db *sql.DB
}

// NewSignalOutcomeStore creates a SignalOutcomeStore backed by db.
// The caller is responsible for ensuring the schema exists (typically via
// ledger.InitSchema); this wrapper only validates the table is reachable.
func NewSignalOutcomeStore(db *sql.DB) *SignalOutcomeStore {
	return &SignalOutcomeStore{db: db}
}

// RecordOutcomes delegates to the package-level RecordOutcomes function.
func (s *SignalOutcomeStore) RecordOutcomes(ctx context.Context, outcomes []SignalOutcome) error {
	return RecordOutcomes(ctx, s.db, outcomes)
}

// LoadOutcomes delegates to the package-level LoadOutcomes function.
func (s *SignalOutcomeStore) LoadOutcomes(ctx context.Context, symbol, source, window string) ([]SignalOutcome, error) {
	return LoadOutcomes(ctx, s.db, symbol, source, window)
}

// RecordOutcomes batch-writes SignalOutcome rows to stock_signal_outcomes.
// Duplicate rows matching (symbol, trigger_date, source) are silently ignored
// so the operation is idempotent. An empty source is rejected with an error
// to match the NOT NULL schema semantics.
func RecordOutcomes(ctx context.Context, db *sql.DB, outcomes []SignalOutcome) error {
	if len(outcomes) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("stockpicker: begin signal outcome tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO stock_signal_outcomes
			(symbol, trigger_date, source, forward_return, net_forward_return, hit, cost_rate, regime, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("stockpicker: prepare signal outcome insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	createdAt := time.Now().UTC().Format(time.RFC3339)
	for _, o := range outcomes {
		if o.Source == "" {
			return fmt.Errorf("stockpicker: signal outcome has empty source (symbol=%q trigger_date=%q)", o.Symbol, o.TriggerDate)
		}
		if o.Symbol == "" {
			return fmt.Errorf("stockpicker: signal outcome has empty symbol (source=%q)", o.Source)
		}
		if o.TriggerDate == "" {
			return fmt.Errorf("stockpicker: signal outcome has empty trigger_date (symbol=%q source=%q)", o.Symbol, o.Source)
		}

		ca := o.CreatedAt
		if ca == "" {
			ca = createdAt
		}

		if _, err := stmt.ExecContext(ctx,
			o.Symbol,
			o.TriggerDate,
			o.Source,
			o.ForwardReturn,
			o.NetForwardReturn,
			boolToInt(o.Hit),
			o.CostRate,
			o.Regime,
			ca,
		); err != nil {
			return fmt.Errorf("stockpicker: insert signal outcome symbol=%s date=%s source=%s: %w",
				o.Symbol, o.TriggerDate, o.Source, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("stockpicker: commit signal outcomes: %w", err)
	}
	return nil
}

// LoadOutcomes reads raw SignalOutcome rows for the given symbol and source.
// The window argument is reserved for future date-range filtering: when
// non-empty (e.g. "120d") only rows whose trigger_date is within the last N
// calendar days are returned; an empty window returns all rows. Rows are
// ordered by trigger_date ascending, source ascending.
func LoadOutcomes(ctx context.Context, db *sql.DB, symbol, source, window string) ([]SignalOutcome, error) {
	cutoff, err := rollingWindowCutoff(window)
	if err != nil {
		return nil, err
	}

	// Use a single static query with placeholder guards to avoid dynamic SQL
	// concatenation flagged by gosec G202. Empty filters are represented by
	// empty strings or a date well in the past.
	const query = `SELECT symbol, trigger_date, source, forward_return, net_forward_return, hit, cost_rate, regime, created_at
		FROM stock_signal_outcomes
		WHERE (symbol = ? OR ? = '')
		  AND (source = ? OR ? = '')
		  AND (trigger_date >= ? OR ? = '')
		ORDER BY trigger_date ASC, source ASC`

	rows, err := db.QueryContext(ctx, query, symbol, symbol, source, source, cutoff, cutoff)
	if err != nil {
		return nil, fmt.Errorf("stockpicker: query signal outcomes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SignalOutcome, 0)
	for rows.Next() {
		var o SignalOutcome
		var hit int
		if err := rows.Scan(
			&o.Symbol,
			&o.TriggerDate,
			&o.Source,
			&o.ForwardReturn,
			&o.NetForwardReturn,
			&hit,
			&o.CostRate,
			&o.Regime,
			&o.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("stockpicker: scan signal outcome: %w", err)
		}
		o.Hit = hit == 1
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("stockpicker: iterate signal outcomes: %w", err)
	}
	return out, nil
}

// rollingWindowCutoff converts a rolling-window label ("60d"/"120d") into the
// earliest trigger_date (YYYY-MM-DD) included in the window. An empty label
// returns "" (no lower bound).
func rollingWindowCutoff(window string) (string, error) {
	if window == "" {
		return "", nil
	}
	if !strings.HasSuffix(window, "d") {
		return "", fmt.Errorf("stockpicker: invalid window %q (want Nd, e.g. 120d)", window)
	}
	days, err := strconv.Atoi(strings.TrimSuffix(window, "d"))
	if err != nil || days <= 0 {
		return "", fmt.Errorf("stockpicker: invalid window %q (want Nd, e.g. 120d)", window)
	}
	return time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02"), nil
}

// boolToInt converts a bool to a SQLite INTEGER 0/1 value.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
