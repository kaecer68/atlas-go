package ledger

import (
	"bufio"
	"encoding/json"
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
