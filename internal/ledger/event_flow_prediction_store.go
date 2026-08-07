package ledger

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventFlowPredictionRecord is a single event-driven capital-flow prediction
// captured at evaluation time. DirectionSign encodes the predicted capital flow
// direction as a signed magnitude:
//
//	"inflow"  → +Confidence
//	"outflow" → -Confidence
//	"neutral" → 0
//
// Downstream alert rules consume []float64 of DirectionSign values to detect
// model-confidence degradation and prediction drift.
type EventFlowPredictionRecord struct {
	PredictedAt   time.Time `json:"predicted_at"`
	DirectionSign float64   `json:"direction_sign"`
	Confidence    float64   `json:"confidence"`
	Direction     string    `json:"direction"`

	// ActualSign is the realized capital-flow direction as a signed magnitude
	// (same encoding as DirectionSign: inflow → +, outflow → −, neutral → 0).
	// Filled on T+1 by the prev-day reconcile task. Zero until then.
	ActualSign float64 `json:"actual_sign,omitempty"`
	// ActualSource records where the realized value came from (e.g. "twse_t86").
	ActualSource string `json:"actual_source,omitempty"`
	// ActualCapturedAt records when the actual was captured. nil = not yet
	// reconciled (distinct from a zero value, which would be ambiguous with
	// "not filled"). Pointer keeps the JSON field omitted until T+1.
	ActualCapturedAt *time.Time `json:"actual_captured_at,omitempty"`
}

// EventFlowPredictionStore persists event-driven capital-flow predictions so
// the Stage 3 alert evaluator can compare current predictions against recent
// history without re-deriving them on every tick. Len and Size exist for
// warmup logic and Prometheus gauges.
type EventFlowPredictionStore interface {
	AppendPrediction(rec EventFlowPredictionRecord) error
	LoadRecentPredictions(limit int) ([]EventFlowPredictionRecord, error)
	Len() int
	Size() int64
	// UpdateActual fills in the realized T+1 outcome for the prediction made
	// at predictedAt. No-op with error when no matching prediction exists.
	UpdateActual(predictedAt time.Time, actualSign float64, source string) error
	// LoadByDate returns the prediction captured at date (T0 midnight UTC).
	// Returns ErrPredictionNotFound when no record matches that timestamp.
	LoadByDate(date time.Time) (EventFlowPredictionRecord, error)
}

// defaultEventFlowPredictionCap bounds the JSONL file size. At ~1
// prediction/day this is roughly three years of history.
const defaultEventFlowPredictionCap = 1000

// JSONLEventFlowPredictionStore writes predictions as JSONL into baseDir.
// It keeps the last maxRecords entries (FIFO eviction).
type JSONLEventFlowPredictionStore struct {
	baseDir    string
	maxRecords int
	mu         sync.Mutex
}

func NewJSONLEventFlowPredictionStore(baseDir string) *JSONLEventFlowPredictionStore {
	return &JSONLEventFlowPredictionStore{
		baseDir:    baseDir,
		maxRecords: defaultEventFlowPredictionCap,
	}
}

func (s *JSONLEventFlowPredictionStore) AppendPrediction(rec EventFlowPredictionRecord) error {
	if rec.DirectionSign == 0 {
		rec.DirectionSign = directionSign(rec.Direction, rec.Confidence)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return fmt.Errorf("mkdir all: %w", err)
	}
	path := filepath.Join(s.baseDir, "event_flow_predictions.jsonl")
	existing, err := readPredictionJSONL(path)
	if err != nil {
		return fmt.Errorf("read existing: %w", err)
	}
	existing = append(existing, rec)
	if s.maxRecords > 0 && len(existing) > s.maxRecords {
		existing = existing[len(existing)-s.maxRecords:]
	}
	return writePredictionJSONL(path, existing)
}

func (s *JSONLEventFlowPredictionStore) LoadRecentPredictions(limit int) ([]EventFlowPredictionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, "event_flow_predictions.jsonl")
	records, err := readPredictionJSONL(path)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(records) > limit {
		records = records[len(records)-limit:]
	}
	return records, nil
}

// Len returns the number of records currently on disk.
func (s *JSONLEventFlowPredictionStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, "event_flow_predictions.jsonl")
	records, err := readPredictionJSONL(path)
	if err != nil {
		return 0
	}
	return len(records)
}

// Size returns the on-disk file size in bytes. Returns 0 if the file does
// not exist (an empty store is a valid state, never a failure).
func (s *JSONLEventFlowPredictionStore) Size() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, "event_flow_predictions.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func readPredictionJSONL(path string) ([]EventFlowPredictionRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var result []EventFlowPredictionRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec EventFlowPredictionRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("decode prediction: %w", err)
		}
		result = append(result, rec)
	}
	return result, scanner.Err()
}

func writePredictionJSONL(path string, records []EventFlowPredictionRecord) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	enc := json.NewEncoder(f)
	for _, rec := range records {
		if err := enc.Encode(rec); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("encode prediction: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close file: %w", err)
	}
	return os.Rename(tmp, path)
}

func directionSign(direction string, confidence float64) float64 {
	return DirectionSign(direction, confidence)
}

// ErrPredictionNotFound is returned by LoadByDate/UpdateActual when no
// prediction record matches the requested date.
var ErrPredictionNotFound = errors.New("ledger: prediction not found")

// loadByDateMatches reports whether the record's PredictedAt corresponds to
// the given calendar date. Predictions are captured around market close
// (13:45 Taipei) so comparing on the date component in Asia/Taipei is the
// stable key; timestamps stored in UTC are converted back for comparison.
func samePredictionDate(rec EventFlowPredictionRecord, date time.Time) bool {
	taipei := time.FixedZone("Asia/Taipei", 8*3600)
	a := rec.PredictedAt.In(taipei)
	b := date.In(taipei)
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// UpdateActual fills in the realized T+1 outcome for the prediction made at
// predictedAt (matched by Taipei date). read-modify-write under the store
// mutex, consistent with AppendPrediction. Returns ErrPredictionNotFound when
// no matching prediction exists.
func (s *JSONLEventFlowPredictionStore) UpdateActual(predictedAt time.Time, actualSign float64, source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, "event_flow_predictions.jsonl")
	records, err := readPredictionJSONL(path)
	if err != nil {
		return fmt.Errorf("update actual read: %w", err)
	}
	updated := false
	for i := range records {
		if !samePredictionDate(records[i], predictedAt) {
			continue
		}
		records[i].ActualSign = actualSign
		records[i].ActualSource = source
		now := time.Now().UTC()
		records[i].ActualCapturedAt = &now
		updated = true
		break
	}
	if !updated {
		return ErrPredictionNotFound
	}
	return writePredictionJSONL(path, records)
}

// LoadByDate returns the prediction captured at date (matched by Taipei date
// component). Returns ErrPredictionNotFound when no record matches.
func (s *JSONLEventFlowPredictionStore) LoadByDate(date time.Time) (EventFlowPredictionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, "event_flow_predictions.jsonl")
	records, err := readPredictionJSONL(path)
	if err != nil {
		return EventFlowPredictionRecord{}, fmt.Errorf("load by date read: %w", err)
	}
	for _, rec := range records {
		if samePredictionDate(rec, date) {
			return rec, nil
		}
	}
	return EventFlowPredictionRecord{}, ErrPredictionNotFound
}

// DirectionSign encodes a predicted capital-flow direction as a signed magnitude:
// "inflow" → +confidence, "outflow" → -confidence, anything else → 0.
func DirectionSign(direction string, confidence float64) float64 {
	switch direction {
	case "inflow":
		return confidence
	case "outflow":
		return -confidence
	default:
		return 0
	}
}

var _ EventFlowPredictionStore = (*JSONLEventFlowPredictionStore)(nil)
