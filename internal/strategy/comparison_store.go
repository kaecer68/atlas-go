package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
)

// FileComparisonStore persists ComparisonDay entries to a JSON file.
type FileComparisonStore struct {
	path    string
	maxDays int
	mu      sync.Mutex
}

// NewFileComparisonStore creates a file-backed store.
// maxDays caps the number of persisted days (0 = unlimited).
func NewFileComparisonStore(path string, maxDays int) *FileComparisonStore {
	return &FileComparisonStore{path: path, maxDays: maxDays}
}

// Load reads all comparison days from disk. Returns nil for non-existent file.
func (s *FileComparisonStore) Load(ctx context.Context) ([]ComparisonDay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var days []ComparisonDay
	if err := json.Unmarshal(raw, &days); err != nil {
		return nil, fmt.Errorf("comparison store: corrupt file %s: %w", s.path, err)
	}
	return days, nil
}

// Upsert inserts or replaces a day entry and persists to disk.
func (s *FileComparisonStore) Upsert(ctx context.Context, day ComparisonDay) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	days, _ := s.loadUnlocked()
	replaced := false
	for i, d := range days {
		if d.TradingDate == day.TradingDate {
			days[i] = day
			replaced = true
			break
		}
	}
	if !replaced {
		days = append(days, day)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].TradingDate < days[j].TradingDate })

	if s.maxDays > 0 && len(days) > s.maxDays {
		days = days[len(days)-s.maxDays:]
	}

	raw, err := json.MarshalIndent(days, "", "  ")
	if err != nil {
		return fmt.Errorf("comparison store: marshal: %w", err)
	}
	if err := os.WriteFile(s.path, raw, 0o644); err != nil {
		return fmt.Errorf("comparison store: write: %w", err)
	}
	return nil
}

// loadUnlocked reads without locking (caller must hold mu).
func (s *FileComparisonStore) loadUnlocked() ([]ComparisonDay, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var days []ComparisonDay
	if err := json.Unmarshal(raw, &days); err != nil {
		return nil, fmt.Errorf("comparison store: corrupt file: %w", err)
	}
	return days, nil
}
