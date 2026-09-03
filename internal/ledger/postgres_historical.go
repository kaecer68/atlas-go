// PostgresHistoricalStore is the PostgreSQL mirror of SQLiteHistoricalStore.
//
// It backs the Stage 4 historical tables (regime / stress / period /
// geopolitical / event_calendar / prediction_backtest) with the shared
// PostgreSQL database. Postgres is multi-process friendly, avoiding the
// SQLite WAL multi-container writer contention that intermittently fails
// with SQLITE_IOERR(522) when atlas-go + cron containers share atlas.db.
//
// Column names mirror the SQLite tables exactly; nullable TEXT columns are
// scanned into *string (nil = NULL) and normalized via deref/ptrToNull.
package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresHistoricalStore implements HistoricalStore on PostgreSQL.
type PostgresHistoricalStore struct {
	pool *pgxpool.Pool
}

// NewPostgresHistoricalStore binds the store to an already-opened pgxpool.
func NewPostgresHistoricalStore(pool *pgxpool.Pool) *PostgresHistoricalStore {
	return &PostgresHistoricalStore{pool: pool}
}

var _ HistoricalStore = (*PostgresHistoricalStore)(nil)

// deref returns the string value or "" for a NULL *string.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ptrToNull converts a *string into sql.NullString for parseTimeColumn.
func ptrToNull(p *string) sql.NullString {
	if p == nil || *p == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

// ------------------------------------------------------------------
// Regime
// ------------------------------------------------------------------

func (s *PostgresHistoricalStore) UpsertRegime(ctx context.Context, row RegimeRow) error {
	if row.Date == "" {
		return fmt.Errorf("regime date is empty")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO regime_history (date, regime, source_session_id, recorded_at, captured_at, is_synthetic, source)
		VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, 'synthetic'))
		ON CONFLICT(date) DO UPDATE SET
			regime = excluded.regime,
			source_session_id = excluded.source_session_id,
			recorded_at = excluded.recorded_at,
			captured_at = excluded.captured_at,
			is_synthetic = excluded.is_synthetic,
			source = excluded.source
	`, row.Date, row.Regime, nullString(row.SourceSessionID),
		nullTime(row.RecordedAt), nullTime(row.CapturedAt), row.IsSynthetic,
		emptyAsNil(row.Source))
	if err != nil {
		return fmt.Errorf("upsert regime %s: %w", row.Date, err)
	}
	return nil
}

func (s *PostgresHistoricalStore) LoadRegimeByDate(ctx context.Context, date string) (RegimeRow, bool, error) {
	return s.loadRegimeByDate(ctx, date, FilterSynthetic)
}

func (s *PostgresHistoricalStore) LoadRegimeByDateAll(ctx context.Context, date string) (RegimeRow, bool, error) {
	return s.loadRegimeByDate(ctx, date, IncludeSynthetic)
}

func (s *PostgresHistoricalStore) loadRegimeByDate(ctx context.Context, date string, filterSynthetic bool) (RegimeRow, bool, error) {
	var r RegimeRow
	var srcSID, sourceVal, recordedAtStr, capturedAtStr *string
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	err := s.pool.QueryRow(ctx, `
		SELECT date, regime, source_session_id, source, recorded_at, captured_at, is_synthetic
		FROM regime_history WHERE date = $1`+filter, date).Scan(&r.Date, &r.Regime, &srcSID, &sourceVal, &recordedAtStr, &capturedAtStr, &r.IsSynthetic)
	if err == pgx.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, fmt.Errorf("load regime %s: %w", date, err)
	}
	r.SourceSessionID = deref(srcSID)
	r.Source = deref(sourceVal)
	r.RecordedAt = parseTimeColumn(ptrToNull(recordedAtStr))
	r.CapturedAt = parseTimeColumn(ptrToNull(capturedAtStr))
	return r, true, nil
}

func (s *PostgresHistoricalStore) LoadRegimeHistory(ctx context.Context, limit int) ([]RegimeRow, error) {
	return s.loadRegimeHistory(ctx, limit, FilterSynthetic)
}

func (s *PostgresHistoricalStore) LoadRegimeHistoryAll(ctx context.Context, limit int) ([]RegimeRow, error) {
	return s.loadRegimeHistory(ctx, limit, IncludeSynthetic)
}

func (s *PostgresHistoricalStore) loadRegimeHistory(ctx context.Context, limit int, filterSynthetic bool) ([]RegimeRow, error) {
	if limit <= 0 {
		limit = 90
	}
	filter := ""
	if filterSynthetic {
		filter = " WHERE is_synthetic = 0"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT date, regime, source_session_id, source, recorded_at, captured_at, is_synthetic
		FROM regime_history`+filter+` ORDER BY date DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("load regime history: %w", err)
	}
	defer rows.Close()
	var out []RegimeRow
	for rows.Next() {
		var r RegimeRow
		var srcSID, sourceVal, recordedAtStr, capturedAtStr *string
		if err := rows.Scan(&r.Date, &r.Regime, &srcSID, &sourceVal, &recordedAtStr, &capturedAtStr, &r.IsSynthetic); err != nil {
			return nil, fmt.Errorf("scan regime row: %w", err)
		}
		r.SourceSessionID = deref(srcSID)
		r.Source = deref(sourceVal)
		r.RecordedAt = parseTimeColumn(ptrToNull(recordedAtStr))
		r.CapturedAt = parseTimeColumn(ptrToNull(capturedAtStr))
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("regime rows: %w", rows.Err())
	}
	return out, nil
}

// ------------------------------------------------------------------
// Period
// ------------------------------------------------------------------

func (s *PostgresHistoricalStore) UpsertPeriod(ctx context.Context, row PeriodRow) error {
	if row.Date == "" {
		return fmt.Errorf("period date is empty")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO period_history (date, period, recorded_at, captured_at, is_synthetic, source, detector_version)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6, 'macro_ingest'), COALESCE($7, 'v1'))
		ON CONFLICT(date) DO UPDATE SET
			period = excluded.period,
			recorded_at = excluded.recorded_at,
			captured_at = excluded.captured_at,
			is_synthetic = excluded.is_synthetic,
			source = excluded.source,
			detector_version = excluded.detector_version
	`, row.Date, row.Period, nullTime(row.RecordedAt), nullTime(row.CapturedAt),
		row.IsSynthetic, emptyAsNil(row.Source), emptyAsNil(row.DetectorVersion))
	if err != nil {
		return fmt.Errorf("upsert period %s: %w", row.Date, err)
	}
	return nil
}

func (s *PostgresHistoricalStore) LoadPeriodByDate(ctx context.Context, date string) (PeriodRow, bool, error) {
	return s.loadPeriodByDate(ctx, date, FilterSynthetic)
}

func (s *PostgresHistoricalStore) LoadPeriodByDateAll(ctx context.Context, date string) (PeriodRow, bool, error) {
	return s.loadPeriodByDate(ctx, date, IncludeSynthetic)
}

func (s *PostgresHistoricalStore) loadPeriodByDate(ctx context.Context, date string, filterSynthetic bool) (PeriodRow, bool, error) {
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	var r PeriodRow
	var recordedAtStr, capturedAtStr *string
	err := s.pool.QueryRow(ctx,
		`SELECT date, period, recorded_at, captured_at, is_synthetic, source, COALESCE(detector_version, 'v1')
		FROM period_history WHERE date = $1`+filter, date).Scan(
		&r.Date, &r.Period, &recordedAtStr, &capturedAtStr, &r.IsSynthetic, &r.Source, &r.DetectorVersion,
	)
	if err == pgx.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, fmt.Errorf("load period by date %s: %w", date, err)
	}
	r.RecordedAt = parseTimeColumn(ptrToNull(recordedAtStr))
	r.CapturedAt = parseTimeColumn(ptrToNull(capturedAtStr))
	return r, true, nil
}

func (s *PostgresHistoricalStore) LoadPeriodHistory(ctx context.Context, limit int) ([]PeriodRow, error) {
	return s.loadPeriodHistory(ctx, limit, FilterSynthetic)
}

func (s *PostgresHistoricalStore) LoadPeriodHistoryAll(ctx context.Context, limit int) ([]PeriodRow, error) {
	return s.loadPeriodHistory(ctx, limit, IncludeSynthetic)
}

func (s *PostgresHistoricalStore) loadPeriodHistory(ctx context.Context, limit int, filterSynthetic bool) ([]PeriodRow, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 365 {
		limit = 365
	}
	filter := ""
	if filterSynthetic {
		filter = " WHERE is_synthetic = 0"
	}
	rows, err := s.pool.Query(ctx,
		`SELECT date, period, recorded_at, captured_at, is_synthetic, source, COALESCE(detector_version, 'v1')
		FROM period_history`+filter+` ORDER BY date DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("load period history: %w", err)
	}
	defer rows.Close()
	var out []PeriodRow
	for rows.Next() {
		var r PeriodRow
		var recordedAtStr, capturedAtStr *string
		if err := rows.Scan(&r.Date, &r.Period, &recordedAtStr, &capturedAtStr, &r.IsSynthetic, &r.Source, &r.DetectorVersion); err != nil {
			return nil, fmt.Errorf("period row scan: %w", err)
		}
		r.RecordedAt = parseTimeColumn(ptrToNull(recordedAtStr))
		r.CapturedAt = parseTimeColumn(ptrToNull(capturedAtStr))
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("period rows: %w", rows.Err())
	}
	return out, nil
}

// ------------------------------------------------------------------
// Stress
// ------------------------------------------------------------------

func (s *PostgresHistoricalStore) UpsertStress(ctx context.Context, row StressRow) error {
	if row.Date == "" {
		return fmt.Errorf("stress date is empty")
	}
	compsJSON := ""
	if row.Components != nil {
		b, err := json.Marshal(row.Components)
		if err != nil {
			return fmt.Errorf("marshal stress components: %w", err)
		}
		compsJSON = string(b)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO stress_index_history (date, score, regime, components_json, source, captured_at, is_synthetic)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT(date) DO UPDATE SET
			score = excluded.score,
			regime = excluded.regime,
			components_json = excluded.components_json,
			source = excluded.source,
			captured_at = excluded.captured_at,
			is_synthetic = excluded.is_synthetic
	`, row.Date, row.Score, nullString(row.Regime), compsJSON,
		nullString(row.Source), nullTime(row.CapturedAt), row.IsSynthetic)
	if err != nil {
		return fmt.Errorf("upsert stress %s: %w", row.Date, err)
	}
	return nil
}

func (s *PostgresHistoricalStore) LoadStressByDate(ctx context.Context, date string) (StressRow, bool, error) {
	return s.loadStressByDate(ctx, date, FilterSynthetic)
}

func (s *PostgresHistoricalStore) LoadStressByDateAll(ctx context.Context, date string) (StressRow, bool, error) {
	return s.loadStressByDate(ctx, date, IncludeSynthetic)
}

func (s *PostgresHistoricalStore) loadStressByDate(ctx context.Context, date string, filterSynthetic bool) (StressRow, bool, error) {
	var r StressRow
	var regime, source, compsJSON, capturedAtStr *string
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	err := s.pool.QueryRow(ctx, `
		SELECT date, score, regime, components_json, source, captured_at, is_synthetic
		FROM stress_index_history WHERE date = $1`+filter, date).Scan(&r.Date, &r.Score, &regime, &compsJSON, &source, &capturedAtStr, &r.IsSynthetic)
	if err == pgx.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, fmt.Errorf("load stress %s: %w", date, err)
	}
	r.Regime = deref(regime)
	r.Source = deref(source)
	r.CapturedAt = parseTimeColumn(ptrToNull(capturedAtStr))
	if compsJSON != nil && *compsJSON != "" {
		_ = json.Unmarshal([]byte(*compsJSON), &r.Components)
	}
	return r, true, nil
}

func (s *PostgresHistoricalStore) LoadStressHistory(ctx context.Context, limit int) ([]StressRow, error) {
	return s.loadStressHistory(ctx, limit, FilterSynthetic)
}

func (s *PostgresHistoricalStore) LoadStressHistoryAll(ctx context.Context, limit int) ([]StressRow, error) {
	return s.loadStressHistory(ctx, limit, IncludeSynthetic)
}

func (s *PostgresHistoricalStore) loadStressHistory(ctx context.Context, limit int, filterSynthetic bool) ([]StressRow, error) {
	if limit <= 0 {
		limit = 90
	}
	filter := ""
	if filterSynthetic {
		filter = " WHERE is_synthetic = 0"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT date, score, regime, components_json, source, captured_at, is_synthetic
		FROM stress_index_history`+filter+` ORDER BY date DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("load stress history: %w", err)
	}
	defer rows.Close()
	var out []StressRow
	for rows.Next() {
		var r StressRow
		var regime, source, compsJSON, capturedAtStr *string
		if err := rows.Scan(&r.Date, &r.Score, &regime, &compsJSON, &source, &capturedAtStr, &r.IsSynthetic); err != nil {
			return nil, fmt.Errorf("scan stress row: %w", err)
		}
		r.Regime = deref(regime)
		r.Source = deref(source)
		r.CapturedAt = parseTimeColumn(ptrToNull(capturedAtStr))
		if compsJSON != nil && *compsJSON != "" {
			_ = json.Unmarshal([]byte(*compsJSON), &r.Components)
		}
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("stress rows: %w", rows.Err())
	}
	return out, nil
}

// ------------------------------------------------------------------
// Geopolitical
// ------------------------------------------------------------------

func (s *PostgresHistoricalStore) UpsertGeopolitical(ctx context.Context, row GeopoliticalRow) error {
	if row.Date == "" {
		return fmt.Errorf("geopolitical date is empty")
	}
	sourcesJSON := ""
	if len(row.Events) > 0 {
		// G5-4: new format {"feeds":[...],"events":[...]} — same as SQLite.
		wrapped, err := json.Marshal(struct {
			Feeds  []string      `json:"feeds"`
			Events []GeoEventRow `json:"events"`
		}{Feeds: row.Sources, Events: row.Events})
		if err != nil {
			return fmt.Errorf("marshal geopolitical sources+events: %w", err)
		}
		sourcesJSON = string(wrapped)
	} else if len(row.Sources) > 0 {
		b, err := json.Marshal(row.Sources)
		if err != nil {
			return fmt.Errorf("marshal geopolitical sources: %w", err)
		}
		sourcesJSON = string(b)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO geopolitical_history (date, intensity, sources_json, source, captured_at, is_synthetic)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT(date) DO UPDATE SET
			intensity = excluded.intensity,
			sources_json = excluded.sources_json,
			source = excluded.source,
			captured_at = excluded.captured_at,
			is_synthetic = excluded.is_synthetic
	`, row.Date, row.Intensity, sourcesJSON,
		nullString(row.Source), nullTime(row.CapturedAt), row.IsSynthetic)
	if err != nil {
		return fmt.Errorf("upsert geopolitical %s: %w", row.Date, err)
	}
	return nil
}

func (s *PostgresHistoricalStore) LoadGeopoliticalByDate(ctx context.Context, date string) (GeopoliticalRow, bool, error) {
	return s.loadGeopoliticalByDate(ctx, date, FilterSynthetic)
}

func (s *PostgresHistoricalStore) LoadGeopoliticalByDateAll(ctx context.Context, date string) (GeopoliticalRow, bool, error) {
	return s.loadGeopoliticalByDate(ctx, date, IncludeSynthetic)
}

func (s *PostgresHistoricalStore) loadGeopoliticalByDate(ctx context.Context, date string, filterSynthetic bool) (GeopoliticalRow, bool, error) {
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	var r GeopoliticalRow
	var source, sourcesJSON, capturedAtStr *string
	err := s.pool.QueryRow(ctx, `
		SELECT date, intensity, sources_json, source, captured_at, is_synthetic
		FROM geopolitical_history WHERE date = $1`+filter, date).Scan(&r.Date, &r.Intensity, &sourcesJSON, &source, &capturedAtStr, &r.IsSynthetic)
	if err == pgx.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, fmt.Errorf("load geopolitical %s: %w", date, err)
	}
	r.Source = deref(source)
	r.CapturedAt = parseTimeColumn(ptrToNull(capturedAtStr))
	if sourcesJSON != nil && *sourcesJSON != "" {
		var wrapped struct {
			Feeds  []string      `json:"feeds"`
			Events []GeoEventRow `json:"events"`
		}
		if err := json.Unmarshal([]byte(*sourcesJSON), &wrapped); err == nil && wrapped.Feeds != nil {
			r.Sources = wrapped.Feeds
			r.Events = wrapped.Events
		} else {
			_ = json.Unmarshal([]byte(*sourcesJSON), &r.Sources)
		}
	}
	return r, true, nil
}

func (s *PostgresHistoricalStore) LoadGeopoliticalHistory(ctx context.Context, limit int) ([]GeopoliticalRow, error) {
	return s.loadGeopoliticalHistory(ctx, limit, FilterSynthetic)
}

func (s *PostgresHistoricalStore) LoadGeopoliticalHistoryAll(ctx context.Context, limit int) ([]GeopoliticalRow, error) {
	return s.loadGeopoliticalHistory(ctx, limit, IncludeSynthetic)
}

func (s *PostgresHistoricalStore) loadGeopoliticalHistory(ctx context.Context, limit int, filterSynthetic bool) ([]GeopoliticalRow, error) {
	if limit <= 0 {
		limit = 90
	}
	filter := ""
	if filterSynthetic {
		filter = " WHERE is_synthetic = 0"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT date, intensity, sources_json, source, captured_at, is_synthetic
		FROM geopolitical_history`+filter+` ORDER BY date DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("load geopolitical history: %w", err)
	}
	defer rows.Close()
	var out []GeopoliticalRow
	for rows.Next() {
		var r GeopoliticalRow
		var source, sourcesJSON, capturedAtStr *string
		if err := rows.Scan(&r.Date, &r.Intensity, &sourcesJSON, &source, &capturedAtStr, &r.IsSynthetic); err != nil {
			return nil, fmt.Errorf("scan geopolitical row: %w", err)
		}
		r.Source = deref(source)
		r.CapturedAt = parseTimeColumn(ptrToNull(capturedAtStr))
		if sourcesJSON != nil && *sourcesJSON != "" {
			var wrapped struct {
				Feeds  []string      `json:"feeds"`
				Events []GeoEventRow `json:"events"`
			}
			if err := json.Unmarshal([]byte(*sourcesJSON), &wrapped); err == nil && wrapped.Feeds != nil {
				r.Sources = wrapped.Feeds
				r.Events = wrapped.Events
			} else {
				_ = json.Unmarshal([]byte(*sourcesJSON), &r.Sources)
			}
		}
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("geopolitical rows: %w", rows.Err())
	}
	return out, nil
}

// ------------------------------------------------------------------
// Event calendar
// ------------------------------------------------------------------

func (s *PostgresHistoricalStore) UpsertEventCalendar(ctx context.Context, row EventCalendarRow) error {
	if row.Date == "" || row.EventID == "" {
		return fmt.Errorf("event date/event_id is empty (date=%q event_id=%q)", row.Date, row.EventID)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO event_calendar_history (date, event_id, active_theme, source, captured_at, is_synthetic)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT(date, event_id) DO UPDATE SET
			active_theme = excluded.active_theme,
			source = excluded.source,
			captured_at = excluded.captured_at,
			is_synthetic = excluded.is_synthetic
	`, row.Date, row.EventID, nullString(row.ActiveTheme), nullString(row.Source),
		nullTime(row.CapturedAt), row.IsSynthetic)
	if err != nil {
		return fmt.Errorf("upsert event %s/%s: %w", row.Date, row.EventID, err)
	}
	return nil
}

func (s *PostgresHistoricalStore) LoadEventCalendarByDate(ctx context.Context, date string) ([]EventCalendarRow, error) {
	return s.loadEventCalendarByDate(ctx, date, FilterSynthetic)
}

func (s *PostgresHistoricalStore) LoadEventCalendarByDateAll(ctx context.Context, date string) ([]EventCalendarRow, error) {
	return s.loadEventCalendarByDate(ctx, date, IncludeSynthetic)
}

func (s *PostgresHistoricalStore) loadEventCalendarByDate(ctx context.Context, date string, filterSynthetic bool) ([]EventCalendarRow, error) {
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT date, event_id, active_theme, source, captured_at, is_synthetic
		FROM event_calendar_history WHERE date = $1`+filter, date)
	if err != nil {
		return nil, fmt.Errorf("load event by date %s: %w", date, err)
	}
	defer rows.Close()
	return scanEventRowsPG(rows)
}

func (s *PostgresHistoricalStore) LoadEventCalendarRange(ctx context.Context, startDate, endDate string, limit int) ([]EventCalendarRow, error) {
	return s.loadEventCalendarRange(ctx, startDate, endDate, limit, FilterSynthetic)
}

func (s *PostgresHistoricalStore) LoadEventCalendarRangeAll(ctx context.Context, startDate, endDate string, limit int) ([]EventCalendarRow, error) {
	return s.loadEventCalendarRange(ctx, startDate, endDate, limit, IncludeSynthetic)
}

func (s *PostgresHistoricalStore) loadEventCalendarRange(ctx context.Context, startDate, endDate string, limit int, filterSynthetic bool) ([]EventCalendarRow, error) {
	if limit <= 0 {
		limit = 500
	}
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT date, event_id, active_theme, source, captured_at, is_synthetic
		FROM event_calendar_history
		WHERE date BETWEEN $1 AND $2`+filter+`
		ORDER BY date ASC LIMIT $3
	`, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("load event range %s..%s: %w", startDate, endDate, err)
	}
	defer rows.Close()
	return scanEventRowsPG(rows)
}

func scanEventRowsPG(rows pgx.Rows) ([]EventCalendarRow, error) {
	var out []EventCalendarRow
	for rows.Next() {
		var r EventCalendarRow
		var theme, source, capturedAtStr *string
		if err := rows.Scan(&r.Date, &r.EventID, &theme, &source, &capturedAtStr, &r.IsSynthetic); err != nil {
			return nil, fmt.Errorf("scan event row: %w", err)
		}
		r.ActiveTheme = deref(theme)
		r.Source = deref(source)
		r.CapturedAt = parseTimeColumn(ptrToNull(capturedAtStr))
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("event rows: %w", rows.Err())
	}
	return out, nil
}

// ------------------------------------------------------------------
// Prediction backtest
// ------------------------------------------------------------------

func (s *PostgresHistoricalStore) UpsertPredictionBacktest(ctx context.Context, row PredictionBacktestRow) error {
	if row.Date == "" {
		return fmt.Errorf("prediction backtest date is empty")
	}
	hitInt := 0
	if row.Hit {
		hitInt = 1
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO prediction_backtest (date, predicted_direction, predicted_confidence, actual_direction, actual_capital_flow_change, hit, model_version, captured_at, is_synthetic)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(date) DO UPDATE SET
			predicted_direction = excluded.predicted_direction,
			predicted_confidence = excluded.predicted_confidence,
			actual_direction = excluded.actual_direction,
			actual_capital_flow_change = excluded.actual_capital_flow_change,
			hit = excluded.hit,
			model_version = excluded.model_version,
			captured_at = excluded.captured_at,
			is_synthetic = excluded.is_synthetic
	`, row.Date, nullString(row.PredictedDirection), row.PredictedConfidence,
		nullString(row.ActualDirection), row.ActualCapitalFlowChan, hitInt,
		nullString(row.ModelVersion), nullTime(row.CapturedAt), row.IsSynthetic)
	if err != nil {
		return fmt.Errorf("upsert prediction %s: %w", row.Date, err)
	}
	return nil
}

func (s *PostgresHistoricalStore) LoadPredictionBacktestRange(ctx context.Context, startDate, endDate string, limit int) ([]PredictionBacktestRow, error) {
	return s.loadPredictionBacktestRange(ctx, startDate, endDate, limit, FilterSynthetic)
}

func (s *PostgresHistoricalStore) LoadPredictionBacktestRangeAll(ctx context.Context, startDate, endDate string, limit int) ([]PredictionBacktestRow, error) {
	return s.loadPredictionBacktestRange(ctx, startDate, endDate, limit, IncludeSynthetic)
}

func (s *PostgresHistoricalStore) loadPredictionBacktestRange(ctx context.Context, startDate, endDate string, limit int, filterSynthetic bool) ([]PredictionBacktestRow, error) {
	if limit <= 0 {
		limit = 500
	}
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT date, predicted_direction, predicted_confidence, actual_direction, actual_capital_flow_change, hit, model_version, captured_at, is_synthetic
		FROM prediction_backtest
		WHERE ($1 = '' OR date >= $1) AND ($2 = '' OR date <= $2)`+filter+`
		ORDER BY date ASC LIMIT $3
	`, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("load prediction range %s..%s: %w", startDate, endDate, err)
	}
	defer rows.Close()
	var out []PredictionBacktestRow
	for rows.Next() {
		var r PredictionBacktestRow
		var pDir, aDir, mv, capturedAtStr *string
		var hitInt int
		if err := rows.Scan(&r.Date, &pDir, &r.PredictedConfidence, &aDir,
			&r.ActualCapitalFlowChan, &hitInt, &mv, &capturedAtStr, &r.IsSynthetic); err != nil {
			return nil, fmt.Errorf("scan prediction row: %w", err)
		}
		r.PredictedDirection = deref(pDir)
		r.ActualDirection = deref(aDir)
		r.ModelVersion = deref(mv)
		r.Hit = hitInt == 1
		r.CapturedAt = parseTimeColumn(ptrToNull(capturedAtStr))
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("prediction rows: %w", rows.Err())
	}
	return out, nil
}

// ------------------------------------------------------------------
// Introspection
// ------------------------------------------------------------------

// HasTables reports whether each Stage 4 history table exists in postgres.
func (s *PostgresHistoricalStore) HasTables(ctx context.Context) (map[string]bool, error) {
	want := []string{
		"regime_history",
		"stress_index_history",
		"period_history",
		"geopolitical_history",
		"event_calendar_history",
		"prediction_backtest",
	}
	out := map[string]bool{}
	for _, name := range want {
		var n int
		err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1`, name).Scan(&n)
		if err != nil {
			return nil, fmt.Errorf("check table %s: %w", name, err)
		}
		out[name] = n > 0
	}
	return out, nil
}

// CountSynthetic returns the number of rows with is_synthetic=1 per table.
func (s *PostgresHistoricalStore) CountSynthetic(ctx context.Context) (map[string]int64, error) {
	tables := []string{
		"regime_history",
		"stress_index_history",
		"geopolitical_history",
		"period_history",
		"event_calendar_history",
		"prediction_backtest",
	}
	out := make(map[string]int64, len(tables))
	for _, name := range tables {
		var n int64
		err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM `+name+` WHERE is_synthetic = 1`).Scan(&n)
		if err != nil {
			return nil, fmt.Errorf("count synthetic %s: %w", name, err)
		}
		out[name] = n
	}
	return out, nil
}
