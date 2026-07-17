package strategies

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FeedbackStore persists validate-API results so hit_rate accumulates over
// backtest runs (manifest #F07). Files are JSONL-per-day under <root>/<id>.json
// with the latest record representing the current snapshot (last-write-wins).
// Production path: data/state/strategy_feedback/.
type FeedbackStore struct {
	root string
	mu   sync.RWMutex
}

// NewFeedbackStore creates a store rooted at the given directory. The
// directory is created lazily on the first write.
func NewFeedbackStore(root string) *FeedbackStore {
	return &FeedbackStore{root: root}
}

// Record is the persisted shape of a validation event.
type Record struct {
	StrategyID  string    `json:"strategy_id"`
	TotalTests  int       `json:"total_tests"`
	TotalHits   int       `json:"total_hits"`
	HitRate     float64   `json:"hit_rate"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Load returns the latest feedback for a strategy, or (Record{}, false, nil)
// if none has been recorded yet. Missing/malformed files degrade gracefully
// to "no feedback yet" rather than an error so reads survive partial state.
func (s *FeedbackStore) Load(strategyID string) (Record, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.path(strategyID))
	if err != nil {
		if os.IsNotExist(err) {
			return Record{}, false, nil
		}
		return Record{}, false, err
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return Record{}, false, fmt.Errorf("strategy_feedback decode %s: %w", strategyID, err)
	}
	return r, true, nil
}

// Write persists the latest validation result. If a previous record exists
// for the same strategy, the new record's totals are CUMULATIVE — we sum
// prior total_tests + delta, and total_hits likewise — so backtest batches
// accumulate rather than overwrite.
func (s *FeedbackStore) Write(r Record) error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return fmt.Errorf("strategy_feedback mkdir: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, hadPrev, _ := s.readLocked(r.StrategyID)
	if hadPrev {
		r.TotalTests += prev.TotalTests
		r.TotalHits += prev.TotalHits
	}
	if r.TotalTests > 0 {
		r.HitRate = float64(r.TotalHits) / float64(r.TotalTests)
	}
	r.UpdatedAt = time.Now().UTC()

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("strategy_feedback marshal: %w", err)
	}
	return os.WriteFile(s.path(r.StrategyID), data, 0o644)
}

func (s *FeedbackStore) readLocked(strategyID string) (Record, bool, error) {
	data, err := os.ReadFile(s.path(strategyID))
	if err != nil {
		if os.IsNotExist(err) {
			return Record{}, false, nil
		}
		return Record{}, false, err
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return Record{}, false, err
	}
	return r, true, nil
}

func (s *FeedbackStore) path(strategyID string) string {
	// Defence-in-depth: the caller already validated the path component via
	// shared.ValidatePathComponent, but a stray "../" must never escape the
	// store root.
	safe := strings.ReplaceAll(strategyID, "/", "_")
	safe = strings.ReplaceAll(safe, "..", "_")
	return filepath.Join(s.root, safe+".json")
}