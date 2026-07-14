package eventquality

import (
	"encoding/json"
	"io"
	"sync"
)

// QualityLog is the destination for every ValidationResult emitted by an
// EventValidator. The log is intentionally append-only JSONL so the same
// stream can be tailed, grep'd, and ingested into a future warehouse
// without further parsing.
//
// The Stage 2 spec requires rejected events to be "記錄到資料品質日誌" —
// QualityLog is that log. Callers wire one QualityLog per ingestion
// pipeline (HTTP handler, background task) and pass it to the validator
// alongside the rule set.
type QualityLog struct {
	mu      sync.Mutex
	writer  io.Writer
	encoder *json.Encoder
}

// NewQualityLog returns a QualityLog that writes one JSON object per line to
// w. The QualityLog takes ownership of w for concurrent writes — callers
// must not write to w from other goroutines.
func NewQualityLog(w io.Writer) *QualityLog {
	enc := json.NewEncoder(w)
	return &QualityLog{writer: w, encoder: enc}
}

// Record writes result as a single JSONL line and returns any encoding error.
// Record is safe for concurrent use.
func (l *QualityLog) Record(result ValidationResult) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.encoder.Encode(result)
}

// Writer returns the underlying io.Writer. Callers that need to compose
// QualityLog with their own writers (e.g. multi-writer fan-out) use this
// to wrap the target writer.
func (l *QualityLog) Writer() io.Writer { return l.writer }
