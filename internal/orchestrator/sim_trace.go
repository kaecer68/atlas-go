package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SimTraceRecord is a single simulation pipeline layer status record.
type SimTraceRecord struct {
	Step      int            `json:"step"`
	Layer     string         `json:"layer"`
	Status    string         `json:"status"`
	TS        time.Time      `json:"ts"`
	SessionID string         `json:"session_id"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// SimTraceWriter persists simulation pipeline trace records to JSONL.
// Thread-safe via sync.Mutex. Optionally prints color-coded output to terminal.
type SimTraceWriter struct {
	mu      sync.Mutex
	records []SimTraceRecord
	baseDir string
	date    string
	verbose bool
}

// NewSimTraceWriter creates a new SimTraceWriter.
// baseDir is the project root directory. date must be in "20060102" format.
// When verbose is true, each Record() call prints color-coded output to terminal.
func NewSimTraceWriter(baseDir string, date string, verbose bool) *SimTraceWriter {
	return &SimTraceWriter{
		baseDir: baseDir,
		date:    date,
		verbose: verbose,
		records: make([]SimTraceRecord, 0),
	}
}

// Record appends a trace record in a thread-safe manner.
// When verbose is true, prints color-coded output:
//
//	OK = green, WARN = yellow, FAIL = red, START = blue
//
// Format: "[step] layer: STATUS (metadata...)"
func (w *SimTraceWriter) Record(step int, layer, status string, meta map[string]any) {
	if meta == nil {
		meta = make(map[string]any)
	}

	record := SimTraceRecord{
		Step:      step,
		Layer:     layer,
		Status:    status,
		TS:        time.Now().UTC(),
		SessionID: w.date,
		Metadata:  meta,
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.records = append(w.records, record)

	if w.verbose {
		w.printColored(record)
	}
}

// ExportJSONL flushes all records to {baseDir}/.omo/traces/sim-{date}.jsonl.
// Returns the absolute file path on success.
func (w *SimTraceWriter) ExportJSONL() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	tracesDir := filepath.Join(w.baseDir, ".omo", "traces")
	if err := os.MkdirAll(tracesDir, 0o755); err != nil {
		return "", fmt.Errorf("create traces directory: %w", err)
	}

	filePath := filepath.Join(tracesDir, "sim-"+w.date+".jsonl")
	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer func() { _ = file.Close() }()

	enc := json.NewEncoder(file)
	for _, record := range w.records {
		if err := enc.Encode(record); err != nil {
			return "", fmt.Errorf("encode trace: %w", err)
		}
	}

	return filePath, nil
}

// printColored prints a color-coded trace record to stdout.
func (w *SimTraceWriter) printColored(record SimTraceRecord) {
	var colorCode string
	switch record.Status {
	case "OK":
		colorCode = "\033[32m" // green
	case "WARN":
		colorCode = "\033[33m" // yellow
	case "FAIL":
		colorCode = "\033[31m" // red
	case "START":
		colorCode = "\033[34m" // blue
	default:
		colorCode = "\033[0m" // reset
	}

	metaStr := ""
	if len(record.Metadata) > 0 {
		metaBytes, err := json.Marshal(record.Metadata)
		if err == nil {
			metaStr = " " + string(metaBytes)
		}
	}

	fmt.Printf("%s[%d] %s: %s%s\033[0m\n", colorCode, record.Step, record.Layer, record.Status, metaStr)
}
