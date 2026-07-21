// Package main implements the Stage 4 historical backfill CLI.
//
// Purpose:
//
//	For each day in the lookback window, scan the existing session
//	directory + global recommendation_outcomes.jsonl + macro directory and
//	emit 4 staging JSONL files. These JSONLs are the source of truth for
//	the Stage 4 PR#2 SQLite loader; PR#3 prediction backtest engine
//	consumes them too.
//
// Why 4 JSONLs and not 5 (per ../../docs/archive/2026-07-14-atlas-stage4-backfill-plan.md):
//
//	The plan listed prediction_input_snapshot_90d.jsonl as the 5th file.
//	After inspecting the predictor's input shape (EventCalendar + CapitalFlow
//	+ NarrativeModels), we determined that the predictor's input can be
//	re-derived on-the-fly by PR#3 from event_calendar_90d + regime_history_90d
//	+ the existing narrative cache. Storing it separately would double-write
//	data without adding audit value.
//
// Output contract:
//
//	Each line is one JSON object terminated by '\n'. Every record carries:
//	  - captured_at:   UTC RFC3339 timestamp
//	  - is_synthetic:  always 1 (backfilled, not produced live)
//
// Files emitted:
//
//	<stagingDir>/regime_history_90d.jsonl
//	<stagingDir>/event_calendar_90d.jsonl
//	<stagingDir>/stress_index_history_90d.jsonl
//	<stagingDir>/prediction_actual_90d.jsonl
package main

import (
	"fmt"
	"path/filepath"
	"time"
)

// ------------------------------------------------------------------
// StagingRecord — root contract shared by every output file.
// ------------------------------------------------------------------

// StagingRecord is the interface every JSONL line in every staging file
// implements. It exists so the writer can stamp captured_at + is_synthetic
// in a single place without each extractor hand-rolling the boilerplate.
type StagingRecord interface {
	SetCapturedAt(t time.Time)
	SetSyntheticFlag(v uint8)
}

// stampDefaults mutates any StagingRecord to fill captured_at + is_synthetic.
// We use a marker struct scan rather than reflection to keep dependencies
// flat; each Record type must implement StagingRecord explicitly.
func stampDefaults(r StagingRecord, now time.Time) {
	r.SetCapturedAt(now)
	r.SetSyntheticFlag(1)
}

// ------------------------------------------------------------------
// RegimeRecord — one row per session that has a summary.json.
// ------------------------------------------------------------------

// RegimeRecord is one row in regime_history_90d.jsonl.
//
// Source: data/state/sessions/session-YYYYMMDD-daily/summary.json:regime
// (via OutcomeStore.LoadSessionSummaries — same path the dashboard uses).
type RegimeRecord struct {
	Date            string    `json:"date"`   // YYYY-MM-DD (Asia/Taipei trading day)
	Regime          string    `json:"regime"` // RISK_ON / RISK_OFF / NEUTRAL / TRANSITIONAL
	SourceSessionID string    `json:"source_session_id"`
	RecordedAt      time.Time `json:"recorded_at"`  // raw UTC from summary.json
	CapturedAt      time.Time `json:"captured_at"`  // when this row was generated
	IsSynthetic     uint8     `json:"is_synthetic"` // always 1
}

func (r *RegimeRecord) SetCapturedAt(t time.Time) { r.CapturedAt = t.UTC() }
func (r *RegimeRecord) SetSyntheticFlag(v uint8)  { r.IsSynthetic = v }

// ------------------------------------------------------------------
// EventCalendarRecord — one row per day with at least one event id.
// ------------------------------------------------------------------

// EventCalendarRecord is one row in event_calendar_90d.jsonl.
//
// Source: aggregated from recommendation_outcomes.jsonl:supporting_events.
// Each day's session summarizes the events the agents "saw" via the
// supporting_events field. We persist those IDs verbatim; PR#3 will resolve
// them against internal/industry.EventCalendar for richer context.
type EventCalendarRecord struct {
	Date         string    `json:"date"`          // YYYY-MM-DD
	EventIDs     []string  `json:"event_ids"`     // evt-tech-peak-..., evt-... from supporting_events
	Source       string    `json:"source"`        // "session-derive"
	ActiveThemes []string  `json:"active_themes"` // deduped themes from event id prefixes (best-effort)
	CapturedAt   time.Time `json:"captured_at"`
	IsSynthetic  uint8     `json:"is_synthetic"`
}

func (r *EventCalendarRecord) SetCapturedAt(t time.Time) { r.CapturedAt = t.UTC() }
func (r *EventCalendarRecord) SetSyntheticFlag(v uint8)  { r.IsSynthetic = v }

// ------------------------------------------------------------------
// StressIndexRecord — one row per day with a macro snapshot.
// ------------------------------------------------------------------

// StressIndexRecord is one row in stress_index_history_90d.jsonl.
//
// Source: data/state/macro/YYYY-MM-DD.json plus optional per-day stress
// computation. We attempt to read a `stress_index` block; when the file
// has no such block we write the row with score=0 and note the absence
// in `source` (raw).
type StressIndexRecord struct {
	Date        string         `json:"date"`
	Score       float64        `json:"score"`                // 0..1 normalized stress
	Regime      string         `json:"regime"`               // "low" / "medium" / "high" / "raw"
	Components  map[string]any `json:"components,omitempty"` // passthrough components block if present
	Source      string         `json:"source"`               // "macro-file" | "raw" | "missing"
	CapturedAt  time.Time      `json:"captured_at"`
	IsSynthetic uint8          `json:"is_synthetic"`
}

func (r *StressIndexRecord) SetCapturedAt(t time.Time) { r.CapturedAt = t.UTC() }
func (r *StressIndexRecord) SetSyntheticFlag(v uint8)  { r.IsSynthetic = v }

// ------------------------------------------------------------------
// PredictionActualRecord — one row per session that has outcomes.
// ------------------------------------------------------------------

// PredictionActualRecord is one row in prediction_actual_90d.jsonl.
//
// Source: aggregated from data/state/sessions/session-*/recommendation_outcomes.jsonl.
// Fields are derived (NOT raw) — they summarize what the market "actually"
// did on that day, which PR#3 will compare against the predictor's output.
//
// Definitions:
//   - TotalOutcomes    : number of RecommendationOutcome rows
//   - HitOutcomesCount : hit==true count
//   - WinRate          : HitOutcomesCount / TotalOutcomes (0 if no outcomes)
//   - MeanForwardReturn: mean(forward_return) across all rows; missing values
//     (forward_return == 0 and window != date) excluded
//   - StdDevForwardReturn: sample std-dev of forward_return (sqrt of biased var
//     would be biased; we use PopVar for simplicity since
//     the metric feeds qualitative comparisons in PR#3)
//   - CapitalFlowChangeProxy: weighted mean of forward_return * conviction/100;
//     a positive number means bullish consensus was correct.
//   - BullishOutcomes / BearishOutcomes: hit counts split by side.
//   - PredominantRegime: regime from the session's summary.json (best-effort
//     cross-reference; left empty when no summary.json).
type PredictionActualRecord struct {
	Date                   string    `json:"date"`
	TotalOutcomes          int       `json:"total_outcomes"`
	HitOutcomesCount       int       `json:"hit_outcomes_count"`
	WinRate                float64   `json:"win_rate"`
	MeanForwardReturn      float64   `json:"mean_forward_return"`
	StdDevForwardReturn    float64   `json:"stddev_forward_return"`
	CapitalFlowChangeProxy float64   `json:"capital_flow_change_proxy"`
	BullishOutcomes        int       `json:"bullish_outcomes_hit"`
	BearishOutcomes        int       `json:"bearish_outcomes_hit"`
	PredominantRegime      string    `json:"predominant_regime,omitempty"`
	SourceSessionID        string    `json:"source_session_id"`
	CapturedAt             time.Time `json:"captured_at"`
	IsSynthetic            uint8     `json:"is_synthetic"`
}

func (r *PredictionActualRecord) SetCapturedAt(t time.Time) { r.CapturedAt = t.UTC() }
func (r *PredictionActualRecord) SetSyntheticFlag(v uint8)  { r.IsSynthetic = v }

// ------------------------------------------------------------------
// StagingFiles — physical file paths in a given staging directory.
// ------------------------------------------------------------------

// StagingFiles bundles the 4 output paths. Resolve via ResolveStagingFiles.
type StagingFiles struct {
	RegimeHistory        string
	EventCalendarHistory string
	StressIndexHistory   string
	PredictionActual     string
}

const (
	RegimeHistoryFile        = "regime_history_90d.jsonl"
	EventCalendarHistoryFile = "event_calendar_90d.jsonl"
	StressIndexHistoryFile   = "stress_index_history_90d.jsonl"
	PredictionActualFile     = "prediction_actual_90d.jsonl"
)

// ResolveStagingFiles returns the 4 expected output paths. Errors only when
// stagingDir is the empty string (a programming mistake we'd rather catch).
func ResolveStagingFiles(stagingDir string) (StagingFiles, error) {
	if stagingDir == "" {
		return StagingFiles{}, fmt.Errorf("staging dir is empty")
	}
	return StagingFiles{
		RegimeHistory:        filepath.Join(stagingDir, RegimeHistoryFile),
		EventCalendarHistory: filepath.Join(stagingDir, EventCalendarHistoryFile),
		StressIndexHistory:   filepath.Join(stagingDir, StressIndexHistoryFile),
		PredictionActual:     filepath.Join(stagingDir, PredictionActualFile),
	}, nil
}

// EncodeLine is a small helper used by all extractors when serializing a
// record through the staging writer. Errors are returned (rather than
// panicked) so the CLI can exit with status code instead of crashing.
func EncodeLine(w *atomicJSONLWriter, r StagingRecord) error {
	if err := w.Encode(r); err != nil {
		return fmt.Errorf("encode %T: %w", r, err)
	}
	return nil
}
