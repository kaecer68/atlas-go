package ledger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type JSONLQuoteStore struct {
	baseDir string
	mu      sync.Mutex
}

func NewJSONLQuoteStore(baseDir string) *JSONLQuoteStore {
	return &JSONLQuoteStore{baseDir: baseDir}
}

func (s *JSONLQuoteStore) RecordQuotes(quotes []domain.DailyBar) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return fmt.Errorf("mkdir all: %w", err)
	}

	path := filepath.Join(s.baseDir, "quotes.jsonl")

	// First-wins dedup on (symbol, date), matching SQLiteQuoteStore's INSERT OR REPLACE.
	quoteKey := func(q domain.DailyBar) string {
		return q.Symbol + "|" + q.Date.UTC().Format("2006-01-02")
	}

	seen := make(map[string]struct{})
	var ordered []domain.DailyBar
	if f, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var quote domain.DailyBar
			if err := json.Unmarshal(scanner.Bytes(), &quote); err == nil {
				k := quoteKey(quote)
				if _, dup := seen[k]; dup {
					continue
				}
				seen[k] = struct{}{}
				ordered = append(ordered, quote)
			}
		}
		_ = f.Close()
	}

	for _, q := range quotes {
		k := quoteKey(q)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		ordered = append(ordered, q)
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	enc := json.NewEncoder(f)
	for _, quote := range ordered {
		if err := enc.Encode(quote); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("encode quote: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close file: %w", err)
	}
	return os.Rename(tmp, path)
}

func (s *JSONLQuoteStore) LoadQuotes(symbol string, start, end time.Time) ([]domain.DailyBar, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, "quotes.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	var result []domain.DailyBar
	for scanner.Scan() {
		var quote domain.DailyBar
		if err := json.Unmarshal(scanner.Bytes(), &quote); err != nil {
			return nil, fmt.Errorf("decode quote: %w", err)
		}
		if quote.Symbol != symbol {
			continue
		}
		if quote.Date.Before(start) || quote.Date.After(end) {
			continue
		}
		result = append(result, quote)
	}
	return result, scanner.Err()
}

func (s *JSONLQuoteStore) LoadLatestQuotes(symbols []string) (map[string]domain.DailyBar, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, "quotes.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	latest := make(map[string]domain.DailyBar)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var quote domain.DailyBar
		if err := json.Unmarshal(scanner.Bytes(), &quote); err != nil {
			return nil, fmt.Errorf("decode quote: %w", err)
		}
		for _, sym := range symbols {
			if quote.Symbol != sym {
				continue
			}
			existing, ok := latest[sym]
			if !ok || quote.Date.After(existing.Date) {
				latest[sym] = quote
			}
		}
	}
	return latest, scanner.Err()
}

var _ QuoteStore = (*JSONLQuoteStore)(nil)
