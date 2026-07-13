package eventquality

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// NewFileQualityLog returns a QualityLog that appends one JSON object per
// line to path. The file and its parent directories are created if they
// don't exist.
func NewFileQualityLog(path string) (*QualityLog, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir for quality log %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open quality log %s: %w", path, err)
	}
	return NewQualityLog(f), nil
}

// Close releases the underlying writer if it implements io.Closer (e.g.
// *os.File from NewFileQualityLog). Safe to call multiple times.
func (q *QualityLog) Close() error {
	if c, ok := q.writer.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
