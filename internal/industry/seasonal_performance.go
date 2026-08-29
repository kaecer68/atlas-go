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

type SeasonalPerformance struct {
	PatternID         string    `json:"pattern_id"`
	Year              int       `json:"year"`
	ActualReturn      float64   `json:"actual_return"`
	PredictedReturn   float64   `json:"predicted_return"`
	Accuracy          float64   `json:"accuracy"`
	FavoredIndustries []string  `json:"favored_industries"`
	RecordedAt        time.Time `json:"recorded_at"`
}

type SeasonalPerformanceStore struct {
	filePath string
	mu       sync.RWMutex
}

func NewSeasonalPerformanceStore(dir string) (*SeasonalPerformanceStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create seasonal performance store dir: %w", err)
	}
	return &SeasonalPerformanceStore{
		filePath: filepath.Join(dir, "seasonal-performance.jsonl"),
	}, nil
}

func (s *SeasonalPerformanceStore) Record(perf SeasonalPerformance) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open seasonal performance file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := json.NewEncoder(f).Encode(perf); err != nil {
		return fmt.Errorf("encode seasonal performance: %w", err)
	}
	return nil
}

func (s *SeasonalPerformanceStore) GetRollingAccuracy(patternID string, years int) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read seasonal performance file: %w", err)
	}

	var totalAccuracy float64
	var count int
	currentYear := time.Now().Year()

	lines := strings.Split(string(data), "\n")
	for _, line := range slices.Backward(lines) {
		line := strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var perf SeasonalPerformance
		if err := json.Unmarshal([]byte(line), &perf); err != nil {
			continue
		}

		if perf.PatternID == patternID && perf.Year >= currentYear-years {
			totalAccuracy += perf.Accuracy
			count++
		}
	}

	if count == 0 {
		return 0, nil
	}
	return totalAccuracy / float64(count), nil
}

func (s *SeasonalPerformanceStore) GetPatternHistory(patternID string) ([]SeasonalPerformance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read seasonal performance file: %w", err)
	}

	var history []SeasonalPerformance
	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var perf SeasonalPerformance
		if err := json.Unmarshal([]byte(line), &perf); err != nil {
			continue
		}

		if perf.PatternID == patternID {
			history = append(history, perf)
		}
	}

	return history, nil
}
