package narrative

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// auditEntry is a lightweight JSONL record for narrative mutation audit.
// Format mirrors the pattern used by the MCP audit_v2 writer.
type auditEntry struct {
	TS       string `json:"ts"`
	Tool     string `json:"tool"`
	ArgsHash string `json:"args_hash"`
	Status   string `json:"status"`
}

// FileAuditLogger writes narrative mutation audit entries to a JSONL file.
// Thread-safe; one writer per file.
type FileAuditLogger struct {
	mu     sync.Mutex
	file   *os.File
	enc    *json.Encoder
}

// NewFileAuditLogger opens (or creates) path for append and returns a logger.
func NewFileAuditLogger(path string) (*FileAuditLogger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	return &FileAuditLogger{
		file: f,
		enc:  json.NewEncoder(f),
	}, nil
}

// record writes one audit entry.
func (l *FileAuditLogger) record(tool, argsHash string) {
	_ = l.enc.Encode(auditEntry{
		TS:       time.Now().UTC().Format(time.RFC3339Nano),
		Tool:     tool,
		ArgsHash: argsHash,
		Status:   "ok",
	})
}

// Close flushes and closes the underlying file.
func (l *FileAuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// TemplateAuditHookFromLogger returns a TemplateAuditHook that writes to l.
func TemplateAuditHookFromLogger(l *FileAuditLogger) TemplateAuditHook {
	return func(templateID, contentHash string) {
		// Derive args hash from the full content hash.
		h := sha256.Sum256([]byte(contentHash))
		argsHash := hex.EncodeToString(h[:])
		l.mu.Lock()
		_ = l.enc.Encode(auditEntry{
			TS:       time.Now().UTC().Format(time.RFC3339Nano),
			Tool:     "narrative_template_modified",
			ArgsHash: argsHash,
			Status:   "ok",
		})
		l.mu.Unlock()
	}
}

// WeightAuditHookFromLogger returns a WeightAuditHook that writes to l.
func WeightAuditHookFromLogger(l *FileAuditLogger) WeightAuditHook {
	return func(modelID, weightHash string) {
		h := sha256.Sum256([]byte(weightHash))
		argsHash := hex.EncodeToString(h[:])
		l.mu.Lock()
		_ = l.enc.Encode(auditEntry{
			TS:       time.Now().UTC().Format(time.RFC3339Nano),
			Tool:     "narrative_model_weight_modified",
			ArgsHash: argsHash,
			Status:   "ok",
		})
		l.mu.Unlock()
	}
}
