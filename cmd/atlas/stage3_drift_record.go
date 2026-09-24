package main

// Stage-3 capital-flow prediction-vs-actual observation record (issue #1941).
//
// The Stage-3 market-close alert task (13:45 Taipei) compares the event-driven
// capital-flow prediction against the realized capital-flow actual
// (monitoring.Stage3AlertDeps.LatestCapitalFlowPrediction /
// LatestCapitalFlowActual). Before #1941 that comparison only ever surfaced as
// an alert on a direction mismatch, so there was no durable
// "predicted vs actual" record to audit and the actual side came from a
// throwaway capitalflow service with an empty rolling window (every Z = 0).
//
// This file persists one JSONL record per comparison, hit or miss, so the
// Stage-3 task demonstrably produces prediction-vs-actual history. Records are
// appended under the ledger dir (same place as the other Stage-3 ledger
// artifacts) and capped so the file cannot grow without bound.

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// stage3DriftRecordFile is the JSONL artifact name under the ledger dir.
const stage3DriftRecordFile = "capital_flow_stage3_drift.jsonl"

// stage3DriftRecordCap bounds the artifact; at one comparison per trading day
// this is roughly four years of history.
const stage3DriftRecordCap = 1000

// stage3DriftRecord is one prediction-vs-actual observation. Both sides are
// the normalized CapitalFlowSignal the drift rule compares (direction label +
// raw value for diagnostics).
type stage3DriftRecord struct {
	RecordedAt         time.Time `json:"recorded_at"`
	TradingDate        string    `json:"trading_date"`
	PredictedDirection string    `json:"predicted_direction"`
	PredictedValue     float64   `json:"predicted_value"`
	ActualDirection    string    `json:"actual_direction"`
	ActualValue        float64   `json:"actual_value"`
	Hit                bool      `json:"hit"`
	ActualSource       string    `json:"actual_source"`
}

// stage3DriftRecorder appends predicted-vs-actual records to a JSONL file
// under the ledger dir. Safe for concurrent use; a nil recorder is a no-op.
type stage3DriftRecorder struct {
	path string
	mu   sync.Mutex
}

// newStage3DriftRecorder returns a recorder writing to
// <ledgerDir>/capital_flow_stage3_drift.jsonl. An empty ledgerDir yields a
// no-op recorder so a misconfigured process never writes to the repo root.
func newStage3DriftRecorder(ledgerDir string) *stage3DriftRecorder {
	if ledgerDir == "" {
		return nil
	}
	return &stage3DriftRecorder{path: filepath.Join(ledgerDir, stage3DriftRecordFile)}
}

// record appends one comparison. The actual side is tagged with the
// capital-flow service as its source so a reader can tell where the realized
// value came from.
func (r *stage3DriftRecorder) record(now time.Time, predicted, actual monitoring.CapitalFlowSignal) error {
	if r == nil {
		return nil
	}
	rec := stage3DriftRecord{
		RecordedAt:         now.UTC(),
		TradingDate:        now.In(taipeiLocation()).Format("2006-01-02"),
		PredictedDirection: predicted.Direction,
		PredictedValue:     predicted.Value,
		ActualDirection:    actual.Direction,
		ActualValue:        actual.Value,
		Hit:                predicted.Direction == actual.Direction,
		ActualSource:       "capitalflow.Service.LatestDaily",
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	lines, err := readJSONLLines(r.path)
	if err != nil {
		return err
	}
	lines = append(lines, string(line))
	if len(lines) > stage3DriftRecordCap {
		lines = lines[len(lines)-stage3DriftRecordCap:]
	}
	return writeJSONLLines(r.path, lines)
}

// readJSONLLines loads the existing records; a missing file is an empty slice.
func readJSONLLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			lines = append(lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

// writeJSONLLines rewrites the artifact atomically (temp file + rename) so a
// crash mid-write cannot leave a truncated record.
func writeJSONLLines(path string, lines []string) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	w := bufio.NewWriter(f)
	var writeErr error
	for _, line := range lines {
		if _, err := w.WriteString(line + "\n"); err != nil {
			writeErr = err
			break
		}
	}
	if writeErr == nil {
		writeErr = w.Flush()
	}
	if writeErr == nil {
		writeErr = f.Sync()
	}
	if closeErr := f.Close(); closeErr != nil && writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		_ = os.Remove(tmp)
		return writeErr
	}
	return os.Rename(tmp, path)
}
