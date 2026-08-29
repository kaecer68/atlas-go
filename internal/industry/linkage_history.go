package industry

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

type LinkageHistoryRecord struct {
	Date                  string    `json:"date"`
	IndustryID            string    `json:"industry_id"`
	SystemicImportance    float64   `json:"systemic_importance"`
	ShockPropagationSpeed float64   `json:"shock_propagation_speed"`
	AvgCorrelation        float64   `json:"avg_correlation"`
	UpstreamCount         int       `json:"upstream_count"`
	DownstreamCount       int       `json:"downstream_count"`
	RecordedAt            time.Time `json:"recorded_at"`
}

type LinkageHistoryStore struct {
	filePath string
	mu       sync.RWMutex
}

func NewLinkageHistoryStore(dir string) (*LinkageHistoryStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create linkage history store dir: %w", err)
	}
	return &LinkageHistoryStore{
		filePath: filepath.Join(dir, "linkage-history.jsonl"),
	}, nil
}

func (s *LinkageHistoryStore) Record(record LinkageHistoryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open linkage history file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := json.NewEncoder(f).Encode(record); err != nil {
		return fmt.Errorf("encode linkage history: %w", err)
	}
	return nil
}

func (s *LinkageHistoryStore) GetHistory(industryID string, days int) ([]LinkageHistoryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read linkage history file: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	var history []LinkageHistoryRecord

	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var record LinkageHistoryRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}

		if record.IndustryID == industryID && record.RecordedAt.After(cutoff) {
			history = append(history, record)
		}
	}

	return history, nil
}

func (s *LinkageHistoryStore) GetLatest(industryID string) (*LinkageHistoryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read linkage history file: %w", err)
	}

	var latest *LinkageHistoryRecord
	lines := strings.Split(string(data), "\n")
	for _, line := range slices.Backward(lines) {
		line := strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var record LinkageHistoryRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}

		if record.IndustryID == industryID {
			latest = &record
			break
		}
	}

	return latest, nil
}
