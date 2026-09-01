package monitoring

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

// AlertStore provides thread-safe JSONL persistence for alert records.
type AlertStore struct {
	filePath string
	mu       sync.RWMutex
}

// NewAlertStore creates an AlertStore backed by a JSONL file.
// The directory is created if it does not exist.
func NewAlertStore(dir string) (*AlertStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create alert store dir: %w", err)
	}
	return &AlertStore{
		filePath: filepath.Join(dir, "alerts.jsonl"),
	}, nil
}

// Save appends an alert record to the JSONL file.
func (s *AlertStore) Save(alert domain.AlertRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open alert file: %w", err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(alert); err != nil {
		return fmt.Errorf("encode alert record: %w", err)
	}
	return nil
}

// LoadAll reads all alert records from the JSONL file.
// Returns nil slice when the file does not exist.
func (s *AlertStore) LoadAll() ([]domain.AlertRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.loadFromFile()
}

// LoadUnacknowledged reads only unacknowledged alert records.
// Returns nil slice when the file does not exist.
func (s *AlertStore) LoadUnacknowledged() ([]domain.AlertRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all, err := s.loadFromFile()
	if err != nil {
		return nil, fmt.Errorf("load alerts: %w", err)
	}

	// #1787: a RESOLVED alert needs no human decision — auto-resolve hooks
	// (task success / coverage restored / TTL) close the condition but do
	// not touch the Acknowledged bool. The queue must report only live,
	// undecided conditions; resolved records previously lingered here even
	// after the underlying condition had cleared.
	var unacked []domain.AlertRecord
	for _, a := range all {
		if !a.Acknowledged && a.Status != domain.AlertStatusResolved && a.Status != domain.AlertStatusSilenced {
			unacked = append(unacked, a)
		}
	}
	return unacked, nil
}

// Acknowledge marks an alert as acknowledged by the given user.
// Returns an error if the alert is not found.
func (s *AlertStore) Acknowledge(alertID string, user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.loadFromFile()
	if err != nil {
		return fmt.Errorf("load alerts: %w", err)
	}

	now := time.Now()
	found := false
	for i := range all {
		if all[i].ID == alertID {
			// Decision 9 (alert-redesign-v2.md Part 3.7): record SLA latency
			// (seconds from emit to ack) for the per-severity SLA threshold.
			latencySec := int(now.Sub(all[i].Timestamp).Seconds())
			all[i].AcknowledgedWithinSec = &latencySec
			all[i].Acknowledged = true
			all[i].AcknowledgedAt = &now
			all[i].AcknowledgedBy = user
			// #1787: acknowledgement is a lifecycle transition, not a side
			// flag. The "需要決策" queue reads status==triggered; without this
			// the same record stayed listed forever after "已知悉" — the UI
			// literally could not clear anything. The bool fields are kept in
			// sync for the unacknowledged endpoint and backward compat.
			all[i].Status = domain.AlertStatusAcknowledged
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("alert %q not found", alertID)
	}

	return s.rewriteAll(all)
}

// Resolve marks an alert as resolved by the given user.
// Returns an error if the alert is not found.
func (s *AlertStore) Resolve(alertID string, user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.loadFromFile()
	if err != nil {
		return fmt.Errorf("load alerts: %w", err)
	}

	now := time.Now()
	found := false
	for i := range all {
		if all[i].ID == alertID {
			all[i].Status = domain.AlertStatusResolved
			all[i].ResolvedAt = &now
			all[i].ResolvedBy = user
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("alert %q not found", alertID)
	}

	return s.rewriteAll(all)
}

// FindByDedupKey searches for an alert record by dedup_key.
// Returns nil when no match is found.
func (s *AlertStore) FindByDedupKey(dedupKey string) (*domain.AlertRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all, err := s.loadFromFile()
	if err != nil {
		return nil, fmt.Errorf("load alerts: %w", err)
	}

	for i := range all {
		if all[i].DedupKey == dedupKey {
			return &all[i], nil
		}
	}
	return nil, nil
}

// Update loads all records, applies fn to the matching record, and rewrites.
// Returns an error if the alert ID is not found.
func (s *AlertStore) Update(id string, fn func(*domain.AlertRecord)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.loadFromFile()
	if err != nil {
		return fmt.Errorf("load alerts: %w", err)
	}

	found := false
	for i := range all {
		if all[i].ID == id {
			fn(&all[i])
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("alert %q not found", id)
	}

	return s.rewriteAll(all)
}

// DeleteWhere removes all alerts matching the predicate and returns the count removed.
func (s *AlertStore) DeleteWhere(predicate func(*domain.AlertRecord) bool) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.loadFromFile()
	if err != nil {
		return 0, fmt.Errorf("load alerts: %w", err)
	}

	var remaining []domain.AlertRecord
	for i := range all {
		if !predicate(&all[i]) {
			remaining = append(remaining, all[i])
		}
	}

	deleted := len(all) - len(remaining)
	if deleted == 0 {
		return 0, nil
	}

	return deleted, s.rewriteAll(remaining)
}

// AcknowledgeWhere acknowledges all alerts matching the predicate and returns the count acknowledged.
func (s *AlertStore) AcknowledgeWhere(predicate func(*domain.AlertRecord) bool, user string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.loadFromFile()
	if err != nil {
		return 0, fmt.Errorf("load alerts: %w", err)
	}

	now := time.Now()
	acknowledged := 0
	for i := range all {
		if predicate(&all[i]) {
			all[i].Acknowledged = true
			all[i].AcknowledgedAt = &now
			all[i].AcknowledgedBy = user
			all[i].Status = domain.AlertStatusAcknowledged // #1787 lifecycle transition
			acknowledged++
		}
	}
	if acknowledged == 0 {
		return 0, nil
	}

	return acknowledged, s.rewriteAll(all)
}

// ResolveWhere resolves all alerts matching the predicate and returns the count resolved.
func (s *AlertStore) ResolveWhere(predicate func(*domain.AlertRecord) bool, user string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.loadFromFile()
	if err != nil {
		return 0, fmt.Errorf("load alerts: %w", err)
	}

	now := time.Now()
	resolved := 0
	for i := range all {
		if predicate(&all[i]) {
			all[i].Status = domain.AlertStatusResolved
			all[i].ResolvedAt = &now
			all[i].ResolvedBy = user
			resolved++
		}
	}
	if resolved == 0 {
		return 0, nil
	}

	return resolved, s.rewriteAll(all)
}

// loadFromFile reads all records from the JSONL file.
// Caller must hold at least a read lock.
func (s *AlertStore) loadFromFile() ([]domain.AlertRecord, error) {
	f, err := os.Open(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open alert file: %w", err)
	}
	defer f.Close()

	var records []domain.AlertRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec domain.AlertRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("decode alert record: %w", err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan alert file: %w", err)
	}
	// #1787 one-time self-heal: records acknowledged before the lifecycle
	// transition fix carry acknowledged=true but status=triggered, which kept
	// them in the "需要決策" queue forever. Normalize on load (idempotent).
	for i := range records {
		if records[i].Acknowledged && records[i].Status == domain.AlertStatusTriggered {
			records[i].Status = domain.AlertStatusAcknowledged
		}
	}
	return records, nil
}

// rewriteAll overwrites the JSONL file with the given records.
// Caller must hold the write lock.
func (s *AlertStore) rewriteAll(records []domain.AlertRecord) error {
	f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open alert file for rewrite: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, rec := range records {
		if err := enc.Encode(rec); err != nil {
			return fmt.Errorf("encode alert record: %w", err)
		}
	}
	return nil
}
