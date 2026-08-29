// File: historical_store.go
// Package: internal/ledger
//
// Stage 4 PR#2 — Historical query layer for the Stage 4 backfilled tables.
// Each row carries is_synthetic (always 1 for these tables). The store
// exposes typed read paths for the three read-only MCP tools added in this PR
// (history_regime, history_stress, history_event_calendar) and a write path
// for the Stage 4 loader CLI.
//
// Red lines respected:
//   - InitSchema is the single source of truth for table DDL (sqlite_core.go).
//   - Writes go through SQLite UPSERT (ON CONFLICT DO UPDATE) so re-running
//     the loader is idempotent and never duplicates rows.
//   - Read APIs use bounds on LIMIT to defend against memory spikes.
package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const (
	FilterSynthetic  = true
	IncludeSynthetic = false
)

// ------------------------------------------------------------------
// Read DTOs (mirror the staging JSONL shapes from cmd/atlas-stage4-backfill).
// ------------------------------------------------------------------

// RegimeRow is one row from regime_history (one regime per date).
type RegimeRow struct {
	Date            string    `json:"date"`
	Regime          string    `json:"regime"`
	SourceSessionID string    `json:"source_session_id"`
	RecordedAt      time.Time `json:"recorded_at"`
	CapturedAt      time.Time `json:"captured_at"`
	IsSynthetic     uint8     `json:"is_synthetic"`
	Source          string    `json:"source"`
}

// StressRow is one row from stress_index_history.
type StressRow struct {
	Date        string         `json:"date"`
	Score       float64        `json:"score"`
	Regime      string         `json:"regime"`
	Components  map[string]any `json:"components,omitempty"`
	Source      string         `json:"source"`
	CapturedAt  time.Time      `json:"captured_at"`
	IsSynthetic uint8          `json:"is_synthetic"`
}

// GeoEventRow is one keyword-matched geopolitical news item persisted for
// event-layer tracing (G5-4). Stored inside geopolitical_history.sources_json
// alongside the feed URLs (new format: {"feeds":[...],"events":[...]}).
type GeoEventRow struct {
	Title   string `json:"title"`
	Keyword string `json:"keyword"`
	Source  string `json:"source"`
}

// GeopoliticalRow is one row from geopolitical_history.
type GeopoliticalRow struct {
	Date        string        `json:"date"`
	Intensity   float64       `json:"intensity"`
	Sources     []string      `json:"sources,omitempty"`
	Events      []GeoEventRow `json:"events,omitempty"` // G5-4 event-layer trace
	Source      string        `json:"source"`
	CapturedAt  time.Time     `json:"captured_at"`
	IsSynthetic uint8         `json:"is_synthetic"`
}

// EventCalendarRow is one row from event_calendar_history.
// (date, event_id) is the composite primary key, so the same date can
// have many EventCalendarRow entries — one per event.
type EventCalendarRow struct {
	Date        string    `json:"date"`
	EventID     string    `json:"event_id"`
	ActiveTheme string    `json:"active_theme"`
	Source      string    `json:"source"`
	CapturedAt  time.Time `json:"captured_at"`
	IsSynthetic uint8     `json:"is_synthetic"`
}

// PredictionBacktestRow is one row from prediction_backtest. Empty rows
// (no prediction yet) are skipped by readers — this struct maps the
// fully-populated shape.
type PredictionBacktestRow struct {
	Date                  string    `json:"date"`
	PredictedDirection    string    `json:"predicted_direction"`
	PredictedConfidence   float64   `json:"predicted_confidence"`
	ActualDirection       string    `json:"actual_direction"`
	ActualCapitalFlowChan float64   `json:"actual_capital_flow_change"`
	Hit                   bool      `json:"hit"`
	ModelVersion          string    `json:"model_version"`
	CapturedAt            time.Time `json:"captured_at"`
	IsSynthetic           uint8     `json:"is_synthetic"`
}

// PeriodRow is one row from period_history (one period per date).
// Populated by the live ingest pipeline (applyMacroUpdate).
type PeriodRow struct {
	Date       string    `json:"date"`
	Period     string    `json:"period"`
	RecordedAt time.Time `json:"recorded_at,omitempty"`
	CapturedAt time.Time `json:"captured_at"`
	// IsSynthetic is 1 when backfilled, 0 when written by live ingest.
	IsSynthetic uint8  `json:"is_synthetic"`
	Source      string `json:"source"`
}

// ------------------------------------------------------------------
// Store interface + SQLite implementation.
// ------------------------------------------------------------------

// HistoricalStore is the type-safe query/load API for the Stage 4 backfill
// tables. Implementations must be safe for concurrent use.
type HistoricalStore interface {
	// Regime
	UpsertRegime(ctx context.Context, row RegimeRow) error
	LoadRegimeByDate(ctx context.Context, date string) (RegimeRow, bool, error)
	LoadRegimeByDateAll(ctx context.Context, date string) (RegimeRow, bool, error)
	LoadRegimeHistory(ctx context.Context, limit int) ([]RegimeRow, error)
	LoadRegimeHistoryAll(ctx context.Context, limit int) ([]RegimeRow, error)

	// Stress
	UpsertStress(ctx context.Context, row StressRow) error
	LoadStressByDate(ctx context.Context, date string) (StressRow, bool, error)
	LoadStressByDateAll(ctx context.Context, date string) (StressRow, bool, error)
	LoadStressHistory(ctx context.Context, limit int) ([]StressRow, error)
	LoadStressHistoryAll(ctx context.Context, limit int) ([]StressRow, error)

	// Geopolitical

	// Period (seven-period market cycle classification)
	UpsertPeriod(ctx context.Context, row PeriodRow) error
	LoadPeriodByDate(ctx context.Context, date string) (PeriodRow, bool, error)
	LoadPeriodByDateAll(ctx context.Context, date string) (PeriodRow, bool, error)
	LoadPeriodHistory(ctx context.Context, limit int) ([]PeriodRow, error)
	LoadPeriodHistoryAll(ctx context.Context, limit int) ([]PeriodRow, error)
	UpsertGeopolitical(ctx context.Context, row GeopoliticalRow) error
	LoadGeopoliticalByDate(ctx context.Context, date string) (GeopoliticalRow, bool, error)
	LoadGeopoliticalByDateAll(ctx context.Context, date string) (GeopoliticalRow, bool, error)
	LoadGeopoliticalHistory(ctx context.Context, limit int) ([]GeopoliticalRow, error)
	LoadGeopoliticalHistoryAll(ctx context.Context, limit int) ([]GeopoliticalRow, error)

	// Event calendar
	UpsertEventCalendar(ctx context.Context, row EventCalendarRow) error
	LoadEventCalendarByDate(ctx context.Context, date string) ([]EventCalendarRow, error)
	LoadEventCalendarByDateAll(ctx context.Context, date string) ([]EventCalendarRow, error)
	LoadEventCalendarRange(ctx context.Context, startDate, endDate string, limit int) ([]EventCalendarRow, error)
	LoadEventCalendarRangeAll(ctx context.Context, startDate, endDate string, limit int) ([]EventCalendarRow, error)

	// Prediction backtest (PR#3 will populate; readers exist now for completeness)
	UpsertPredictionBacktest(ctx context.Context, row PredictionBacktestRow) error
	LoadPredictionBacktestRange(ctx context.Context, startDate, endDate string, limit int) ([]PredictionBacktestRow, error)
	LoadPredictionBacktestRangeAll(ctx context.Context, startDate, endDate string, limit int) ([]PredictionBacktestRow, error)

	// CountSynthetic returns the number of rows with is_synthetic=1 per table.
	CountSynthetic(ctx context.Context) (map[string]int64, error)
}

// SQLiteHistoricalStore is the canonical implementation backed by the
// stage-4 tables in data/state/atlas.db. The *sql.DB is reused across all
// ledger stores in the project (OpenSQLiteDB initializes WAL + FK).
type SQLiteHistoricalStore struct {
	db *sql.DB
}

// NewSQLiteHistoricalStore binds the store to an already-opened *sql.DB.
// The caller is responsible for calling OpenSQLiteDB and InitSchema first.
func NewSQLiteHistoricalStore(db *sql.DB) *SQLiteHistoricalStore {
	return &SQLiteHistoricalStore{db: db}
}

// Compile-time interface satisfaction guard.
var _ HistoricalStore = (*SQLiteHistoricalStore)(nil)

// ------------------------------------------------------------------
// Regime
// ------------------------------------------------------------------

func (s *SQLiteHistoricalStore) UpsertRegime(ctx context.Context, row RegimeRow) error {
	if row.Date == "" {
		return fmt.Errorf("regime date is empty")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO regime_history (date, regime, source_session_id, recorded_at, captured_at, is_synthetic, source)
		VALUES (?, ?, ?, ?, ?, ?, COALESCE(?, 'synthetic'))
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

// emptyAsNil converts "" to nil so SQLite's COALESCE picks up the schema
// DEFAULT instead of failing the NOT NULL constraint with an empty string.
func emptyAsNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *SQLiteHistoricalStore) LoadRegimeByDate(ctx context.Context, date string) (RegimeRow, bool, error) {
	return s.loadRegimeByDate(ctx, date, FilterSynthetic)
}

func (s *SQLiteHistoricalStore) LoadRegimeByDateAll(ctx context.Context, date string) (RegimeRow, bool, error) {
	return s.loadRegimeByDate(ctx, date, IncludeSynthetic)
}

func (s *SQLiteHistoricalStore) loadRegimeByDate(ctx context.Context, date string, filterSynthetic bool) (RegimeRow, bool, error) {
	var r RegimeRow
	var srcSID, sourceVal, recordedAtStr, capturedAtStr sql.NullString
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT date, regime, source_session_id, source, recorded_at, captured_at, is_synthetic
		FROM regime_history WHERE date = ?`+filter, date).Scan(&r.Date, &r.Regime, &srcSID, &sourceVal, &recordedAtStr, &capturedAtStr, &r.IsSynthetic)
	if err == sql.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, fmt.Errorf("load regime %s: %w", date, err)
	}
	r.SourceSessionID = srcSID.String
	r.Source = sourceVal.String
	r.RecordedAt = parseTimeColumn(recordedAtStr)
	r.CapturedAt = parseTimeColumn(capturedAtStr)
	return r, true, nil
}

func (s *SQLiteHistoricalStore) LoadRegimeHistory(ctx context.Context, limit int) ([]RegimeRow, error) {
	return s.loadRegimeHistory(ctx, limit, FilterSynthetic)
}

func (s *SQLiteHistoricalStore) LoadRegimeHistoryAll(ctx context.Context, limit int) ([]RegimeRow, error) {
	return s.loadRegimeHistory(ctx, limit, IncludeSynthetic)
}

func (s *SQLiteHistoricalStore) loadRegimeHistory(ctx context.Context, limit int, filterSynthetic bool) ([]RegimeRow, error) {
	if limit <= 0 {
		limit = 90
	}
	filter := ""
	if filterSynthetic {
		filter = " WHERE is_synthetic = 0"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT date, regime, source_session_id, source, recorded_at, captured_at, is_synthetic
		FROM regime_history`+filter+` ORDER BY date DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("load regime history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []RegimeRow
	for rows.Next() {
		var r RegimeRow
		var srcSID, sourceVal, recordedAtStr, capturedAtStr sql.NullString
		if err := rows.Scan(&r.Date, &r.Regime, &srcSID, &sourceVal, &recordedAtStr, &capturedAtStr, &r.IsSynthetic); err != nil {
			return nil, fmt.Errorf("scan regime row: %w", err)
		}
		r.SourceSessionID = srcSID.String
		r.Source = sourceVal.String
		r.RecordedAt = parseTimeColumn(recordedAtStr)
		r.CapturedAt = parseTimeColumn(capturedAtStr)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("regime rows: %w", err)
	}
	return out, nil
}

// ------------------------------------------------------------------
// Period (seven-period market cycle)
// ------------------------------------------------------------------

// UpsertPeriod inserts or updates a period_history row. Date is the
// trading day (YYYY-MM-DD). The ON CONFLICT(date) DO UPDATE makes
// repeated writes from the live ingest pipeline idempotent.
func (s *SQLiteHistoricalStore) UpsertPeriod(ctx context.Context, row PeriodRow) error {
	if row.Date == "" {
		return fmt.Errorf("period date is empty")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO period_history (date, period, recorded_at, captured_at, is_synthetic, source)
		VALUES (?, ?, ?, ?, ?, COALESCE(?, 'macro_ingest'))
		ON CONFLICT(date) DO UPDATE SET
			period = excluded.period,
			recorded_at = excluded.recorded_at,
			captured_at = excluded.captured_at,
			is_synthetic = excluded.is_synthetic,
			source = excluded.source
	`, row.Date, row.Period, nullTime(row.RecordedAt), nullTime(row.CapturedAt),
		row.IsSynthetic, emptyAsNil(row.Source))
	if err != nil {
		return fmt.Errorf("upsert period %s: %w", row.Date, err)
	}
	return nil
}

// LoadPeriodByDate returns the period row for a date, excluding
// synthetic rows.
func (s *SQLiteHistoricalStore) LoadPeriodByDate(ctx context.Context, date string) (PeriodRow, bool, error) {
	return s.loadPeriodByDate(ctx, date, FilterSynthetic)
}

// LoadPeriodByDateAll returns the period row for a date including
// synthetic rows.
func (s *SQLiteHistoricalStore) LoadPeriodByDateAll(ctx context.Context, date string) (PeriodRow, bool, error) {
	return s.loadPeriodByDate(ctx, date, IncludeSynthetic)
}

func (s *SQLiteHistoricalStore) loadPeriodByDate(ctx context.Context, date string, filterSynthetic bool) (PeriodRow, bool, error) {
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	var r PeriodRow
	var recordedAtStr, capturedAtStr sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT date, period, recorded_at, captured_at, is_synthetic, source
		FROM period_history WHERE date = ?`+filter, date).Scan(
		&r.Date, &r.Period, &recordedAtStr, &capturedAtStr, &r.IsSynthetic, &r.Source,
	)
	if err == sql.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, fmt.Errorf("load period by date %s: %w", date, err)
	}
	r.RecordedAt = parseTimeColumn(recordedAtStr)
	r.CapturedAt = parseTimeColumn(capturedAtStr)
	return r, true, nil
}

// LoadPeriodHistory returns period rows ordered by date DESC (newest
// first), excluding synthetic rows.
func (s *SQLiteHistoricalStore) LoadPeriodHistory(ctx context.Context, limit int) ([]PeriodRow, error) {
	return s.loadPeriodHistory(ctx, limit, FilterSynthetic)
}

// LoadPeriodHistoryAll returns period rows including synthetic rows.
func (s *SQLiteHistoricalStore) LoadPeriodHistoryAll(ctx context.Context, limit int) ([]PeriodRow, error) {
	return s.loadPeriodHistory(ctx, limit, IncludeSynthetic)
}

func (s *SQLiteHistoricalStore) loadPeriodHistory(ctx context.Context, limit int, filterSynthetic bool) ([]PeriodRow, error) {
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
	rows, err := s.db.QueryContext(ctx,
		`SELECT date, period, recorded_at, captured_at, is_synthetic, source
		FROM period_history`+filter+` ORDER BY date DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("load period history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PeriodRow
	for rows.Next() {
		var r PeriodRow
		var recordedAtStr, capturedAtStr sql.NullString
		if err := rows.Scan(&r.Date, &r.Period, &recordedAtStr, &capturedAtStr, &r.IsSynthetic, &r.Source); err != nil {
			return nil, fmt.Errorf("period row scan: %w", err)
		}
		r.RecordedAt = parseTimeColumn(recordedAtStr)
		r.CapturedAt = parseTimeColumn(capturedAtStr)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("period rows: %w", err)
	}
	return out, nil
}

// ------------------------------------------------------------------
// Stress
// ------------------------------------------------------------------

func (s *SQLiteHistoricalStore) UpsertStress(ctx context.Context, row StressRow) error {
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
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO stress_index_history (date, score, regime, components_json, source, captured_at, is_synthetic)
		VALUES (?, ?, ?, ?, ?, ?, ?)
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

func (s *SQLiteHistoricalStore) LoadStressByDate(ctx context.Context, date string) (StressRow, bool, error) {
	return s.loadStressByDate(ctx, date, FilterSynthetic)
}

func (s *SQLiteHistoricalStore) LoadStressByDateAll(ctx context.Context, date string) (StressRow, bool, error) {
	return s.loadStressByDate(ctx, date, IncludeSynthetic)
}

func (s *SQLiteHistoricalStore) loadStressByDate(ctx context.Context, date string, filterSynthetic bool) (StressRow, bool, error) {
	var r StressRow
	var regime, source, compsJSON, capturedAtStr sql.NullString
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT date, score, regime, components_json, source, captured_at, is_synthetic
		FROM stress_index_history WHERE date = ?`+filter, date).Scan(&r.Date, &r.Score, &regime, &compsJSON, &source, &capturedAtStr, &r.IsSynthetic)
	if err == sql.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, fmt.Errorf("load stress %s: %w", date, err)
	}
	r.Regime = regime.String
	r.Source = source.String
	r.CapturedAt = parseTimeColumn(capturedAtStr)
	if compsJSON.Valid && compsJSON.String != "" {
		_ = json.Unmarshal([]byte(compsJSON.String), &r.Components)
	}
	return r, true, nil
}

func (s *SQLiteHistoricalStore) LoadStressHistory(ctx context.Context, limit int) ([]StressRow, error) {
	return s.loadStressHistory(ctx, limit, FilterSynthetic)
}

func (s *SQLiteHistoricalStore) LoadStressHistoryAll(ctx context.Context, limit int) ([]StressRow, error) {
	return s.loadStressHistory(ctx, limit, IncludeSynthetic)
}

func (s *SQLiteHistoricalStore) loadStressHistory(ctx context.Context, limit int, filterSynthetic bool) ([]StressRow, error) {
	if limit <= 0 {
		limit = 90
	}
	filter := ""
	if filterSynthetic {
		filter = " WHERE is_synthetic = 0"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT date, score, regime, components_json, source, captured_at, is_synthetic
		FROM stress_index_history`+filter+` ORDER BY date DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("load stress history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StressRow
	for rows.Next() {
		var r StressRow
		var regime, source, compsJSON, capturedAtStr sql.NullString
		if err := rows.Scan(&r.Date, &r.Score, &regime, &compsJSON, &source, &capturedAtStr, &r.IsSynthetic); err != nil {
			return nil, fmt.Errorf("scan stress row: %w", err)
		}
		r.Regime = regime.String
		r.Source = source.String
		r.CapturedAt = parseTimeColumn(capturedAtStr)
		if compsJSON.Valid && compsJSON.String != "" {
			_ = json.Unmarshal([]byte(compsJSON.String), &r.Components)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("stress rows: %w", err)
	}
	return out, nil
}

// ------------------------------------------------------------------
// Geopolitical
// ------------------------------------------------------------------

func (s *SQLiteHistoricalStore) UpsertGeopolitical(ctx context.Context, row GeopoliticalRow) error {
	if row.Date == "" {
		return fmt.Errorf("geopolitical date is empty")
	}
	sourcesJSON := ""
	if len(row.Events) > 0 {
		// G5-4: new format {"feeds":[...],"events":[...]} — keeps feed URLs
		// for backward compat and adds the event-layer trace items.
		wrapped, err := json.Marshal(struct {
			Feeds  []string      `json:"feeds"`
			Events []GeoEventRow `json:"events"`
		}{Feeds: row.Sources, Events: row.Events})
		if err != nil {
			return fmt.Errorf("marshal geopolitical sources+events: %w", err)
		}
		sourcesJSON = string(wrapped)
	} else if row.Sources != nil {
		b, err := json.Marshal(row.Sources)
		if err != nil {
			return fmt.Errorf("marshal geopolitical sources: %w", err)
		}
		sourcesJSON = string(b)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO geopolitical_history (date, intensity, sources_json, source, captured_at, is_synthetic)
		VALUES (?, ?, ?, ?, ?, ?)
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

func (s *SQLiteHistoricalStore) LoadGeopoliticalByDate(ctx context.Context, date string) (GeopoliticalRow, bool, error) {
	return s.loadGeopoliticalByDate(ctx, date, FilterSynthetic)
}

func (s *SQLiteHistoricalStore) LoadGeopoliticalByDateAll(ctx context.Context, date string) (GeopoliticalRow, bool, error) {
	return s.loadGeopoliticalByDate(ctx, date, IncludeSynthetic)
}

func (s *SQLiteHistoricalStore) loadGeopoliticalByDate(ctx context.Context, date string, filterSynthetic bool) (GeopoliticalRow, bool, error) {
	var r GeopoliticalRow
	var source, sourcesJSON, capturedAtStr sql.NullString
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT date, intensity, sources_json, source, captured_at, is_synthetic
		FROM geopolitical_history WHERE date = ?`+filter, date).Scan(&r.Date, &r.Intensity, &sourcesJSON, &source, &capturedAtStr, &r.IsSynthetic)
	if err == sql.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, fmt.Errorf("load geopolitical %s: %w", date, err)
	}
	r.Source = source.String
	r.CapturedAt = parseTimeColumn(capturedAtStr)
	if sourcesJSON.Valid && sourcesJSON.String != "" {
		unmarshalGeopoliticalSources(sourcesJSON.String, &r)
	}
	return r, true, nil
}

// unmarshalGeopoliticalSources tolerantly reads geopolitical_history.sources_json:
// legacy rows store a plain []string of feed URLs; G5-4 rows store
// {"feeds":[...],"events":[...]}. Both round-trip into GeopoliticalRow.
func unmarshalGeopoliticalSources(raw string, r *GeopoliticalRow) {
	var wrapped struct {
		Feeds  []string      `json:"feeds"`
		Events []GeoEventRow `json:"events"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapped); err == nil && wrapped.Feeds != nil {
		r.Sources = wrapped.Feeds
		r.Events = wrapped.Events
		return
	}
	// legacy: plain feed-URL array
	_ = json.Unmarshal([]byte(raw), &r.Sources)
}

func (s *SQLiteHistoricalStore) LoadGeopoliticalHistory(ctx context.Context, limit int) ([]GeopoliticalRow, error) {
	return s.loadGeopoliticalHistory(ctx, limit, FilterSynthetic)
}

func (s *SQLiteHistoricalStore) LoadGeopoliticalHistoryAll(ctx context.Context, limit int) ([]GeopoliticalRow, error) {
	return s.loadGeopoliticalHistory(ctx, limit, IncludeSynthetic)
}

func (s *SQLiteHistoricalStore) loadGeopoliticalHistory(ctx context.Context, limit int, filterSynthetic bool) ([]GeopoliticalRow, error) {
	if limit <= 0 {
		limit = 90
	}
	filter := ""
	if filterSynthetic {
		filter = " WHERE is_synthetic = 0"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT date, intensity, sources_json, source, captured_at, is_synthetic
		FROM geopolitical_history`+filter+` ORDER BY date DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("load geopolitical history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []GeopoliticalRow
	for rows.Next() {
		var r GeopoliticalRow
		var source, sourcesJSON, capturedAtStr sql.NullString
		if err := rows.Scan(&r.Date, &r.Intensity, &sourcesJSON, &source, &capturedAtStr, &r.IsSynthetic); err != nil {
			return nil, fmt.Errorf("scan geopolitical row: %w", err)
		}
		r.Source = source.String
		r.CapturedAt = parseTimeColumn(capturedAtStr)
		if sourcesJSON.Valid && sourcesJSON.String != "" {
			unmarshalGeopoliticalSources(sourcesJSON.String, &r)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("geopolitical rows: %w", err)
	}
	return out, nil
}

// ------------------------------------------------------------------
// Event calendar
// ------------------------------------------------------------------

func (s *SQLiteHistoricalStore) UpsertEventCalendar(ctx context.Context, row EventCalendarRow) error {
	if row.Date == "" || row.EventID == "" {
		return fmt.Errorf("event date/event_id is empty (date=%q event_id=%q)", row.Date, row.EventID)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO event_calendar_history (date, event_id, active_theme, source, captured_at, is_synthetic)
		VALUES (?, ?, ?, ?, ?, ?)
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

func (s *SQLiteHistoricalStore) LoadEventCalendarByDate(ctx context.Context, date string) ([]EventCalendarRow, error) {
	return s.loadEventCalendarByDate(ctx, date, FilterSynthetic)
}

func (s *SQLiteHistoricalStore) LoadEventCalendarByDateAll(ctx context.Context, date string) ([]EventCalendarRow, error) {
	return s.loadEventCalendarByDate(ctx, date, IncludeSynthetic)
}

func (s *SQLiteHistoricalStore) loadEventCalendarByDate(ctx context.Context, date string, filterSynthetic bool) ([]EventCalendarRow, error) {
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT date, event_id, active_theme, source, captured_at, is_synthetic
		FROM event_calendar_history WHERE date = ?`+filter, date)
	if err != nil {
		return nil, fmt.Errorf("load event by date %s: %w", date, err)
	}
	defer func() { _ = rows.Close() }()
	return scanEventRows(rows)
}

func (s *SQLiteHistoricalStore) LoadEventCalendarRange(ctx context.Context, startDate, endDate string, limit int) ([]EventCalendarRow, error) {
	return s.loadEventCalendarRange(ctx, startDate, endDate, limit, FilterSynthetic)
}

func (s *SQLiteHistoricalStore) LoadEventCalendarRangeAll(ctx context.Context, startDate, endDate string, limit int) ([]EventCalendarRow, error) {
	return s.loadEventCalendarRange(ctx, startDate, endDate, limit, IncludeSynthetic)
}

func (s *SQLiteHistoricalStore) loadEventCalendarRange(ctx context.Context, startDate, endDate string, limit int, filterSynthetic bool) ([]EventCalendarRow, error) {
	if limit <= 0 {
		limit = 500
	}
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT date, event_id, active_theme, source, captured_at, is_synthetic
		FROM event_calendar_history
		WHERE date BETWEEN ? AND ?`+filter+`
		ORDER BY date ASC LIMIT ?
	`, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("load event range %s..%s: %w", startDate, endDate, err)
	}
	defer func() { _ = rows.Close() }()
	return scanEventRows(rows)
}

func scanEventRows(rows *sql.Rows) ([]EventCalendarRow, error) {
	var out []EventCalendarRow
	for rows.Next() {
		var r EventCalendarRow
		var theme, source, capturedAtStr sql.NullString
		if err := rows.Scan(&r.Date, &r.EventID, &theme, &source, &capturedAtStr, &r.IsSynthetic); err != nil {
			return nil, fmt.Errorf("scan event row: %w", err)
		}
		r.ActiveTheme = theme.String
		r.Source = source.String
		r.CapturedAt = parseTimeColumn(capturedAtStr)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("event rows: %w", err)
	}
	return out, nil
}

// ------------------------------------------------------------------
// Prediction backtest (PR#3 will populate; readers are defined now)
// ------------------------------------------------------------------

func (s *SQLiteHistoricalStore) UpsertPredictionBacktest(ctx context.Context, row PredictionBacktestRow) error {
	if row.Date == "" {
		return fmt.Errorf("prediction backtest date is empty")
	}
	hitInt := 0
	if row.Hit {
		hitInt = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO prediction_backtest (date, predicted_direction, predicted_confidence, actual_direction, actual_capital_flow_change, hit, model_version, captured_at, is_synthetic)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
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

func (s *SQLiteHistoricalStore) LoadPredictionBacktestRange(ctx context.Context, startDate, endDate string, limit int) ([]PredictionBacktestRow, error) {
	return s.loadPredictionBacktestRange(ctx, startDate, endDate, limit, FilterSynthetic)
}

func (s *SQLiteHistoricalStore) LoadPredictionBacktestRangeAll(ctx context.Context, startDate, endDate string, limit int) ([]PredictionBacktestRow, error) {
	return s.loadPredictionBacktestRange(ctx, startDate, endDate, limit, IncludeSynthetic)
}

func (s *SQLiteHistoricalStore) loadPredictionBacktestRange(ctx context.Context, startDate, endDate string, limit int, filterSynthetic bool) ([]PredictionBacktestRow, error) {
	if limit <= 0 {
		limit = 500
	}
	filter := ""
	if filterSynthetic {
		filter = " AND is_synthetic = 0"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT date, predicted_direction, predicted_confidence, actual_direction, actual_capital_flow_change, hit, model_version, captured_at, is_synthetic
		FROM prediction_backtest
		WHERE (? = '' OR date >= ?) AND (? = '' OR date <= ?)`+filter+`
		ORDER BY date ASC LIMIT ?
	`, startDate, startDate, endDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("load prediction range %s..%s: %w", startDate, endDate, err)
	}
	defer func() { _ = rows.Close() }()
	var out []PredictionBacktestRow
	for rows.Next() {
		var r PredictionBacktestRow
		var pDir, aDir, mv, capturedAtStr sql.NullString
		var hitInt int
		if err := rows.Scan(&r.Date, &pDir, &r.PredictedConfidence, &aDir,
			&r.ActualCapitalFlowChan, &hitInt, &mv, &capturedAtStr, &r.IsSynthetic); err != nil {
			return nil, fmt.Errorf("scan prediction row: %w", err)
		}
		r.PredictedDirection = pDir.String
		r.ActualDirection = aDir.String
		r.ModelVersion = mv.String
		r.Hit = hitInt == 1
		r.CapturedAt = parseTimeColumn(capturedAtStr)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("prediction rows: %w", err)
	}
	return out, nil
}

// ------------------------------------------------------------------
// null helpers — concise Nullable for store fields that allow empty.
// ------------------------------------------------------------------

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}

// parseTimeColumn converts a TEXT column into time.Time.
// modernc.org/sqlite does not support scanning a TEXT column into a
// sql.NullTime target, so we route through sql.NullString + manual parse.
// Accepts both RFC3339 family strings and the Go-native time.Time.String()
// format (e.g. "2026-06-29 06:00:00 +0000 UTC" or
// "2026-07-20 16:14:01.734248004 +0000 UTC"), the latter being what the
// modernc sqlite driver writes when serializing sql.NullTime values whose
// underlying time.Time has its monotonic clock stripped (i.e. UTC times
// produced by `t.UTC()`). Returns the zero time when the column is NULL
// or empty, or when no layout matches the stored format.
func parseTimeColumn(ns sql.NullString) time.Time {
	if !ns.Valid || ns.String == "" {
		return time.Time{}
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, ns.String); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000Z",
	"2006-01-02T15:04:05Z",
	// Go-native time.Time.String() format — handles both zero-fractional
	// and nanosecond-precision representations via the .999... token.
	"2006-01-02 15:04:05.999999999 -0700 MST",
}

// HasTables reports whether each of the Stage 4 history tables exists
// in the open SQLite database. Used by the loader CLI to fail fast when
// InitSchema has not been run.
func (s *SQLiteHistoricalStore) HasTables(ctx context.Context) (map[string]bool, error) {
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
		err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, name).Scan(&n)
		if err != nil {
			return nil, fmt.Errorf("check table %s: %w", name, err)
		}
		out[name] = n > 0
	}
	return out, nil
}

// CountSynthetic returns the number of rows with is_synthetic=1 in each of the
// Stage 4 history tables. Useful for ops/debugging and for validating the
// effect of the --drop-synthetic loader flag.
func (s *SQLiteHistoricalStore) CountSynthetic(ctx context.Context) (map[string]int64, error) {
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
		err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM `+name+` WHERE is_synthetic = 1`).Scan(&n)
		if err != nil {
			return nil, fmt.Errorf("count synthetic %s: %w", name, err)
		}
		out[name] = n
	}
	return out, nil
}
