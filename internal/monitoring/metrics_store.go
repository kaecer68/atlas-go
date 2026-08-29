package monitoring

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type MetricsStore struct {
	filePath string
	mu       sync.RWMutex
}

func NewMetricsStore(dir string) (*MetricsStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create metrics store dir: %w", err)
	}
	return &MetricsStore{
		filePath: filepath.Join(dir, "metrics.jsonl"),
	}, nil
}

func (s *MetricsStore) SaveSnapshot(snapshot MetricsSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open metrics file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := json.NewEncoder(f).Encode(snapshot); err != nil {
		return fmt.Errorf("encode metrics snapshot: %w", err)
	}
	return nil
}

func (s *MetricsStore) LoadToday() (*MetricsSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read metrics file: %w", err)
	}

	today := time.Now().Format("2006-01-02")
	var lastToday *MetricsSnapshot

	lines := strings.Split(string(data), "\n")
	for _, line := range slices.Backward(lines) {
		line := strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var snapshot MetricsSnapshot
		if err := json.Unmarshal([]byte(line), &snapshot); err != nil {
			continue
		}

		if snapshot.Timestamp.Format("2006-01-02") == today {
			lastToday = &snapshot
			break
		}
	}

	return lastToday, nil
}

func (s *MetricsStore) LoadRecent(n int) ([]MetricsSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read metrics file: %w", err)
	}

	var snapshots []MetricsSnapshot
	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var snapshot MetricsSnapshot
		if err := json.Unmarshal([]byte(line), &snapshot); err != nil {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}

	if len(snapshots) > n {
		return snapshots[len(snapshots)-n:], nil
	}
	return snapshots, nil
}
