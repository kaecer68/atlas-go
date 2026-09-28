package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/futures"
)

// PostgresFuturesShadowStore 是影子列的 PostgreSQL 後端（生產）。
type PostgresFuturesShadowStore struct {
	pool *pgxpool.Pool
}

// NewPostgresFuturesShadowStore 綁定已開啟的 pgxpool。
func NewPostgresFuturesShadowStore(pool *pgxpool.Pool) *PostgresFuturesShadowStore {
	return &PostgresFuturesShadowStore{pool: pool}
}

var _ FuturesShadowStore = (*PostgresFuturesShadowStore)(nil)

// RecordFuturesShadow upsert（ON CONFLICT），四欄冪等鍵。
func (s *PostgresFuturesShadowStore) RecordFuturesShadow(ctx context.Context, rows []FuturesShadowRow) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	batch := &pgx.Batch{}
	for _, row := range rows {
		if row.ModelVersion == "" {
			return 0, fmt.Errorf("futures shadow row %s: empty model version", row.Sample.TradeDate.Format("2006-01-02"))
		}
		batch.Queue(`
			INSERT INTO futures_shadow_predictions (
				contract, trade_date, model_version, near_month, term_structure,
				spread_points, spread_bp, oi_near_trend, oi_near_change_pct, near_return_pct,
				foreign_oi_change, pcr_oi_ratio, pcr_oi_ratio_change,
				score, terms_used, predicted_direction, confidence,
				label_return_pct, actual_direction, hit, created_at
			) VALUES ($1, $2::date, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, now())
			ON CONFLICT (contract, trade_date, model_version) DO UPDATE SET
				near_month = excluded.near_month,
				term_structure = excluded.term_structure,
				spread_points = excluded.spread_points,
				spread_bp = excluded.spread_bp,
				oi_near_trend = excluded.oi_near_trend,
				oi_near_change_pct = excluded.oi_near_change_pct,
				near_return_pct = excluded.near_return_pct,
				foreign_oi_change = excluded.foreign_oi_change,
				pcr_oi_ratio = excluded.pcr_oi_ratio,
				pcr_oi_ratio_change = excluded.pcr_oi_ratio_change,
				score = excluded.score,
				terms_used = excluded.terms_used,
				predicted_direction = excluded.predicted_direction,
				confidence = excluded.confidence,
				label_return_pct = excluded.label_return_pct,
				actual_direction = excluded.actual_direction,
				hit = excluded.hit,
				created_at = excluded.created_at
		`, row.Sample.Contract, row.Sample.TradeDate.Format("2006-01-02"), row.ModelVersion, row.Sample.NearMonth, row.Sample.TermStructure,
			row.Sample.SpreadPoints, row.Sample.SpreadBP, row.Sample.OINearTrend, row.Sample.OINearChangePct, row.Sample.NearReturnPct,
			futuresIntPtr(row.Sample.ForeignOIChange), futuresFloatPtr(row.Sample.PCROIRatio), futuresFloatPtr(row.Sample.PCRDelta),
			row.Sample.Score, row.Sample.TermsUsed, string(row.Sample.Predicted), row.Sample.Confidence,
			futuresFloatPtr(row.Sample.LabelReturnPct), nullableDirection(row.Sample.ActualDirection), nullableBoolInt(row.Sample.Hit))
	}
	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	if _, err := br.Exec(); err != nil {
		return 0, fmt.Errorf("insert futures shadow rows: %w", err)
	}
	return len(rows), nil
}

// LoadFuturesShadow 取回影子列（pgx 原生可空型別）。
func (s *PostgresFuturesShadowStore) LoadFuturesShadow(ctx context.Context, contract, modelVersion string, start, end time.Time) ([]FuturesShadowRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT contract, to_char(trade_date, 'YYYY-MM-DD'), model_version, near_month, term_structure,
		       spread_points, spread_bp, oi_near_trend, oi_near_change_pct, near_return_pct,
		       foreign_oi_change, pcr_oi_ratio, pcr_oi_ratio_change,
		       score, terms_used, predicted_direction, confidence,
		       label_return_pct, actual_direction, hit
		FROM futures_shadow_predictions
		WHERE contract = $1
		  AND trade_date >= $2::date
		  AND trade_date <= $3::date
		  AND ($4 = '' OR model_version = $4)
		ORDER BY trade_date ASC`,
		contract, start.Format("2006-01-02"), end.Format("2006-01-02"), modelVersion)
	if err != nil {
		return nil, fmt.Errorf("query futures shadow rows: %w", err)
	}
	defer rows.Close()
	return scanFuturesShadowPG(rows)
}

// pgFuturesShadowRows 是 pgx.Rows 的最小介面。
type pgFuturesShadowRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanFuturesShadowPG(rows pgFuturesShadowRows) ([]FuturesShadowRow, error) {
	var out []FuturesShadowRow
	for rows.Next() {
		var (
			row             FuturesShadowRow
			dateStr         string
			foreignOIChange *int64
			pcrOIRatio      *float64
			pcrDelta        *float64
			labelReturn     *float64
			actualDirection *string
			hit             *bool
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
		row.Sample.ForeignOIChange = foreignOIChange
		row.Sample.PCROIRatio = pcrOIRatio
		row.Sample.PCRDelta = pcrDelta
		row.Sample.LabelReturnPct = labelReturn
		if actualDirection != nil {
			row.Sample.ActualDirection = futures.Direction(*actualDirection)
		}
		row.Sample.Hit = hit
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate futures shadow rows: %w", err)
	}
	return out, nil
}
