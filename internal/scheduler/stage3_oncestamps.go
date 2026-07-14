package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// OncestampStore prevents a scheduled task wrapper from running more than
// once per natural period (day / week / month) even across process restarts.
//
// TryClaim returns (run, ok):
//   - run=true + ok=true: claim succeeded, caller should run the task.
//   - run=false + ok=true: same period already claimed, caller should skip.
//   - ok=false: store unavailable (read-only fallback, disk error). Caller
//     MUST run as if the claim succeeded — failing closed on a stale cache
//     can cause double-runs; failing closed on a missing store just gives
//     at-most-once semantics instead of exactly-once.
type OncestampStore interface {
	TryClaim(key string, now time.Time, samePeriod func(a, b time.Time) bool) (run bool, ok bool)
}

// InMemoryOncestampStore is the default when no persistence is configured.
// It is goroutine-safe but loses its claims on process restart.
type InMemoryOncestampStore struct {
	mu     sync.Mutex
	claims map[string]time.Time
}

func NewInMemoryOncestampStore() *InMemoryOncestampStore {
	return &InMemoryOncestampStore{claims: make(map[string]time.Time)}
}

func (s *InMemoryOncestampStore) TryClaim(key string, now time.Time, samePeriod func(a, b time.Time) bool) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prev, ok := s.claims[key]; ok && samePeriod(prev, now) {
		return false, true
	}
	s.claims[key] = now
	return true, true
}

// FileOncestampStore persists oncestamps to a JSON file under baseDir
// (default filename: stage3_oncestamps.json). Writes use an atomic
// .tmp + rename so a process crash mid-write cannot corrupt the file.
//
// The store is goroutine-safe; multiple BackgroundTaskManager goroutines can
// claim different keys concurrently.
type FileOncestampStore struct {
	path string
	mu   sync.Mutex
}

func NewFileOncestampStore(baseDir string) (*FileOncestampStore, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("oncestamp mkdir: %w", err)
	}
	return &FileOncestampStore{path: filepath.Join(baseDir, "stage3_oncestamps.json")}, nil
}

func (s *FileOncestampStore) TryClaim(key string, now time.Time, samePeriod func(a, b time.Time) bool) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	claims, err := readOncestampsFile(s.path)
	if err != nil {
		return true, false
	}
	if prev, ok := claims[key]; ok && samePeriod(prev, now) {
		return false, true
	}
	claims[key] = now
	if err := writeOncestampsFile(s.path, claims); err != nil {
		return true, false
	}
	return true, true
}

type oncestampsFile struct {
	Claims map[string]time.Time `json:"claims"`
}

func readOncestampsFile(path string) (map[string]time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]time.Time), nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var file oncestampsFile
	if err := json.NewDecoder(f).Decode(&file); err != nil {
		return nil, err
	}
	if file.Claims == nil {
		file.Claims = make(map[string]time.Time)
	}
	return file.Claims, nil
}

func writeOncestampsFile(path string, claims map[string]time.Time) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	encodeErr := enc.Encode(oncestampsFile{Claims: claims})
	closeErr := f.Close()
	if encodeErr != nil {
		_ = os.Remove(tmp)
		return encodeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, path)
}

// sameDay returns true if a and b fall on the same local calendar day in tz.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// sameWeek returns true if a and b fall in the same Mon-anchored week in tz.
// Used by weeklyOnceGuard.
func sameWeek(tz *time.Location, a, b time.Time) bool {
	if tz == nil {
		tz = time.UTC
	}
	la := a.In(tz)
	lb := b.In(tz)
	weekA := la.Add(-time.Duration((la.Weekday()+6)%7) * 24 * time.Hour)
	weekB := lb.Add(-time.Duration((lb.Weekday()+6)%7) * 24 * time.Hour)
	_, wA := weekA.ISOWeek()
	_, wB := weekB.ISOWeek()
	return weekA.Year() == weekB.Year() && wA == wB
}

// sameMonth returns true if a and b fall in the same year+month in tz.
func sameMonth(tz *time.Location, a, b time.Time) bool {
	if tz == nil {
		tz = time.UTC
	}
	la := a.In(tz)
	lb := b.In(tz)
	return la.Year() == lb.Year() && la.Month() == lb.Month()
}
