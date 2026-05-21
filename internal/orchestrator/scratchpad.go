package orchestrator

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Scratchpad persists reasoning traces as JSONL files.
type Scratchpad struct {
	sessionID string
	baseDir   string
	traces    []ReasoningTrace
	mu        sync.RWMutex
}

// NewScratchpad creates a new Scratchpad and auto-creates the traces directory.
func NewScratchpad(sessionID, baseDir string) *Scratchpad {
	tracesDir := filepath.Join(baseDir, "traces")
	os.MkdirAll(tracesDir, 0755) //nolint:errcheck
	return &Scratchpad{
		sessionID: sessionID,
		baseDir:   baseDir,
		traces:    make([]ReasoningTrace, 0),
	}
}

// Record appends a reasoning trace in a thread-safe manner.
func (s *Scratchpad) Record(trace ReasoningTrace) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.traces = append(s.traces, trace)
}

// MarkAllAsFallback sets IsFallback=true on every recorded trace.
func (s *Scratchpad) MarkAllAsFallback() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.traces {
		s.traces[i].IsFallback = true
	}
}

// Traces returns a copy of all recorded traces.
func (s *Scratchpad) Traces() []ReasoningTrace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ReasoningTrace, len(s.traces))
	copy(result, s.traces)
	return result
}

// ExportJSONL writes all traces to {baseDir}/traces/{sessionID}.jsonl.
func (s *Scratchpad) ExportJSONL() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tracesDir := filepath.Join(s.baseDir, "traces")
	if err := os.MkdirAll(tracesDir, 0755); err != nil {
		return "", fmt.Errorf("create traces directory: %w", err)
	}

	filePath := filepath.Join(tracesDir, s.sessionID+".jsonl")
	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	for _, trace := range s.traces {
		if err := enc.Encode(trace); err != nil {
			return "", fmt.Errorf("encode trace: %w", err)
		}
	}

	return filePath, nil
}

// LoadScratchpad reads a JSONL file and returns a Scratchpad instance.
// Silently skips malformed lines for corruption resilience.
func LoadScratchpad(sessionID, baseDir string) (*Scratchpad, error) {
	filePath := filepath.Join(baseDir, "traces", sessionID+".jsonl")
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	s := &Scratchpad{
		sessionID: sessionID,
		baseDir:   baseDir,
		traces:    make([]ReasoningTrace, 0),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var trace ReasoningTrace
		if err := json.Unmarshal(line, &trace); err != nil {
			continue // silently skip malformed lines
		}
		s.traces = append(s.traces, trace)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan file: %w", err)
	}

	return s, nil
}
