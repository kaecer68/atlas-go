package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/futures"
)

// SQLiteFuturesShadowStore 是影子列的 SQLite 後端（本機開發用）。
type SQLiteFuturesShadowStore struct {
	db *sql.DB
}

// NewSQLiteFuturesShadowStore 以既有 *sql.DB 建立 store。
func NewSQLiteFuturesShadowStore(db *sql.DB) *SQLiteFuturesShadowStore {
	return &SQLiteFuturesShadowStore{db: db}
}

// NewSQLiteFuturesShadowStoreFromPath 開檔並確保 schema 後建立 store。
func NewSQLiteFuturesShadowStoreFromPath(path string) (*SQLiteFuturesShadowStore, error) {
	db, err := OpenSQLiteDB(path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	if err := InitSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return NewSQLiteFuturesShadowStore(db), nil
}

var _ FuturesShadowStore = (*SQLiteFuturesShadowStore)(nil)

// RecordFuturesShadow upsert（INSERT OR REPLACE），四欄冪等鍵。
func (s *SQLiteFuturesShadowStore) RecordFuturesShadow(ctx context.Context, rows []FuturesShadowRow) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback() //nolint:errcheck
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO futures_shadow_predictions (
			contract, trade_date, model_version, near_month, term_structure,
			spread_points, spread_bp, oi_near_trend, oi_near_change_pct, near_return_pct,
			foreign_oi_change, pcr_oi_ratio, pcr_oi_ratio_change,
			score, terms_used, predicted_direction, confidence,
			label_return_pct, actual_direction, hit, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare shadow statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().UTC().Format(time.RFC3339)
	n := 0
	for _, row := range rows {
		if row.ModelVersion == "" {
			return 0, fmt.Errorf("futures shadow row %s: empty model version", row.Sample.TradeDate.Format("2006-01-02"))
		}
		if _, err := stmt.ExecContext(ctx,
			row.Sample.Contract,
			row.Sample.TradeDate.Format("2006-01-02"),
			row.ModelVersion,
			row.Sample.NearMonth,
			row.Sample.TermStructure,
			row.Sample.SpreadPoints,
			row.Sample.SpreadBP,
			row.Sample.OINearTrend,
			row.Sample.OINearChangePct,
			row.Sample.NearReturnPct,
			futuresIntPtr(row.Sample.ForeignOIChange),
			futuresFloatPtr(row.Sample.PCROIRatio),
			futuresFloatPtr(row.Sample.PCRDelta),
			row.Sample.Score,
			row.Sample.TermsUsed,
			string(row.Sample.Predicted),
			row.Sample.Confidence,
			futuresFloatPtr(row.Sample.LabelReturnPct),
			nullableDirection(row.Sample.ActualDirection),
			nullableBoolInt(row.Sample.Hit),
			now,
		); err != nil {
			return 0, fmt.Errorf("insert futures shadow row: %w", err)
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}
	committed = true
	return n, nil
}

// LoadFuturesShadow 取回影子列（SQLite 走 database/sql 的 sql.Null* 掃描）。
func (s *SQLiteFuturesShadowStore) LoadFuturesShadow(ctx context.Context, contract, modelVersion string, start, end time.Time) ([]FuturesShadowRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT contract, trade_date, model_version, near_month, term_structure,
		       spread_points, spread_bp, oi_near_trend, oi_near_change_pct, near_return_pct,
		       foreign_oi_change, pcr_oi_ratio, pcr_oi_ratio_change,
		       score, terms_used, predicted_direction, confidence,
		       label_return_pct, actual_direction, hit
		FROM futures_shadow_predictions
		WHERE contract = ?
		  AND trade_date >= ?
		  AND trade_date <= ?
		  AND (? = '' OR model_version = ?)
		ORDER BY trade_date ASC`,
		contract, start.Format("2006-01-02"), end.Format("2006-01-02"), modelVersion, modelVersion)
	if err != nil {
		return nil, fmt.Errorf("query futures shadow rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanFuturesShadowSQL(rows)
}

func scanFuturesShadowSQL(rows *sql.Rows) ([]FuturesShadowRow, error) {
	var out []FuturesShadowRow
	for rows.Next() {
		var (
			row             FuturesShadowRow
			dateStr         string
			foreignOIChange sql.NullInt64
			pcrOIRatio      sql.NullFloat64
			pcrDelta        sql.NullFloat64
			labelReturn     sql.NullFloat64
			actualDirection sql.NullString
			hit             sql.NullInt64
		)
		if err := rows.Scan(
			&row.Sample.Contract, &dateStr, &row.ModelVersion, &row.Sample.NearMonth, &row.Sample.TermStructure,
			&row.Sample.SpreadPoints, &row.Sample.SpreadBP, &row.Sample.OINearTrend, &row.Sample.OINearChangePct, &row.Sample.NearReturnPct,
			&foreignOIChange, &pcrOIRatio, &pcrDelta,
			&row.Sample.Score, &row.Sample.TermsUsed, &row.Sample.Predicted, &row.Sample.Confidence,
			&labelReturn, &actualDirection, &hit,
		); err != nil {
			return nil, fmt.Errorf("scan futures shadow row: %w", err)
		}
		date, err := time.ParseInLocation("2006-01-02", dateStr, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse trade_date %q: %w", dateStr, err)
		}
		row.Sample.TradeDate = date
		row.Sample.ForeignOIChange = futuresNullIntPtr(foreignOIChange)
		row.Sample.PCROIRatio = futuresNullFloatPtr(pcrOIRatio)
		row.Sample.PCRDelta = futuresNullFloatPtr(pcrDelta)
		row.Sample.LabelReturnPct = futuresNullFloatPtr(labelReturn)
		if actualDirection.Valid {
			row.Sample.ActualDirection = futures.Direction(actualDirection.String)
		}
		if hit.Valid {
			v := hit.Int64 != 0
			row.Sample.Hit = &v
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate futures shadow rows: %w", err)
	}
	return out, nil
}

// nullableDirection 把空方向寫成 SQL NULL（不得以空字串混入）。
func nullableDirection(d futures.Direction) any {
	if d == "" {
		return nil
	}
	return string(d)
}

// nullableBoolInt 把 *bool 寫成 0/1 或 SQL NULL。
func nullableBoolInt(v *bool) any {
	if v == nil {
		return nil
	}
	if *v {
		return 1
	}
	return 0
}
