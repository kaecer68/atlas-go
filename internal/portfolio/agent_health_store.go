package portfolio

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// AgentHealthStore provides thread-safe JSONL persistence for agent health records.
// Each line in the JSONL file is a single AgentHealth JSON object.
// LoadAll() deduplicates by AgentID, keeping the most recent record per agent.
type AgentHealthStore struct {
	filePath string
	mu       sync.RWMutex
}

// NewAgentHealthStore creates an AgentHealthStore backed by a JSONL file.
// The directory is created if it does not exist.
func NewAgentHealthStore(dir string) (*AgentHealthStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create agent health store dir: %w", err)
	}
	return &AgentHealthStore{
		filePath: filepath.Join(dir, "agent_health.jsonl"),
	}, nil
}

// Save appends an agent health record to the JSONL file.
func (s *AgentHealthStore) Save(h *AgentHealth) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open agent health file: %w", err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(h); err != nil {
		return fmt.Errorf("encode agent health record: %w", err)
	}
	return nil
}

// LoadAll reads all agent health records from the JSONL file and returns
// a map deduplicated by AgentID, keeping the most recent record for each agent.
// Returns nil map when the file does not exist.
func (s *AgentHealthStore) LoadAll() (map[string]*AgentHealth, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, err := os.Open(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open agent health file: %w", err)
	}
	defer f.Close()

	result := make(map[string]*AgentHealth)
	scanner := bufio.NewScanner(f)
	const maxScanTokenSize = 1024 * 1024
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)

	for scanner.Scan() {
		var h AgentHealth
		if err := json.Unmarshal(scanner.Bytes(), &h); err != nil {
			return nil, fmt.Errorf("decode agent health record: %w", err)
		}
		// Deduplicate: keep the latest record for each agent.
		// Copy to avoid loop variable capture bug.
		copied := h
		result[h.AgentID] = &copied
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan agent health file: %w", err)
	}
	return result, nil
}

// rewriteAll overwrites the JSONL file with the given health records.
// Caller must hold the write lock.
func (s *AgentHealthStore) rewriteAll(records []*AgentHealth) error {
	f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open agent health file for rewrite: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, rec := range records {
		if err := enc.Encode(rec); err != nil {
			return fmt.Errorf("encode agent health record: %w", err)
		}
	}
	return nil
}
