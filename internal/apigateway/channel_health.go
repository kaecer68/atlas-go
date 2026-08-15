package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ChannelHealthRecord stores the last fetch result for a single channel.
//
// Originally declared in internal/monitoring/channel_health.go; relocated
// here in Wave 12 Phase 2 (Issue #731) to break the 4-layer transitive
// import cycle `llm_annotator → apigateway → monitoring → llm/capabilities →
// llm_annotator`. Monitoring keeps type aliases for backward compatibility;
// new code should depend on apigateway directly.
type ChannelHealthRecord struct {
	Status             string   `json:"status"`        // ok | warn | error | inactive
	LastFetchAt        string   `json:"last_fetch_at"` // RFC3339
	LastDataAt         string   `json:"last_data_at,omitempty"`
	LastError          string   `json:"last_error,omitempty"`
	LastSuccessAt      string   `json:"last_success_at,omitempty"`
	RateLimitRemaining int      `json:"rate_limit_remaining,omitempty"`
	LatencyMs          int64    `json:"latency_ms,omitempty"`
	RecordsFetched     int      `json:"records_fetched,omitempty"`
	SymbolsProcessed   int      `json:"symbols_processed,omitempty"`
	Errors             []string `json:"errors,omitempty"`
}

// ChannelFetchLogEntry captures a single channel fetch event for the recent-fetches ring buffer.
type ChannelFetchLogEntry struct {
	Channel   string `json:"channel"`
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	Timestamp string `json:"timestamp"` // RFC3339
	Error     string `json:"error,omitempty"`
}

// fetchLogCap bounds the persistent recent-fetches ring buffer per ChannelHealthStore.
const fetchLogCap = 50

// DefaultStaleThreshold is the default maximum age of a non-ok
// ChannelHealthRecord before Alerts() filters it out. Channels refresh
// every 5min-1hr; records older than this window are no longer
// actionable signals (e.g., a transient DNS failure that resolved hours
// ago should not surface as a current alert in the dashboard).
//
// Set to 0 to disable stale filtering (return all records regardless of age).
const DefaultStaleThreshold = 1 * time.Hour

// ChannelHealthStore persists channel fetch outcomes.
//
// Originally declared in internal/monitoring/channel_health.go; relocated
// here in Wave 12 Phase 2 (Issue #731) — see package doc on
// ChannelHealthRecord for the cycle-breaking rationale.
type ChannelHealthStore struct {
	path         string
	fetchLogPath string
	mu           sync.RWMutex
	data         map[string]*ChannelHealthRecord
	fetchLog     []ChannelFetchLogEntry
	pool         *pgxpool.Pool

	// staleThreshold filters non-ok records from Alerts() when the record's
	// LastFetchAt is older than this duration from nowFunc(). Set to 0 to
	// disable filtering (return all records regardless of age). Default:
	// DefaultStaleThreshold (1 hour). Configurable via WithStaleThreshold.
	staleThreshold time.Duration

	// nowFunc returns the current time. Defaults to time.Now; injectable
	// for tests via WithNowFunc. Matches the CircuitBreaker.WithNowFunc
	// pattern (Wave 12 Phase 2, Issue #731) for deterministic clock in
	// stale-threshold boundary tests.
	nowFunc func() time.Time
}

// NewChannelHealthStore creates or loads a health store at the given directory.
func NewChannelHealthStore(dir string) *ChannelHealthStore {
	return NewChannelHealthStoreWithPool(dir, nil)
}

// NewChannelHealthStoreWithPool creates a health store with an optional DB pool.
func NewChannelHealthStoreWithPool(dir string, pool *pgxpool.Pool) *ChannelHealthStore {
	return &ChannelHealthStore{
		path:           filepath.Join(dir, "channel_health.json"),
		fetchLogPath:   filepath.Join(dir, "channel_fetch_log.json"),
		data:           make(map[string]*ChannelHealthRecord),
		pool:           pool,
		staleThreshold: DefaultStaleThreshold,
		nowFunc:        time.Now,
	}
}

// WithStaleThreshold configures the maximum age of a non-ok record before
// Alerts() filters it out. Pass 0 to disable filtering. Returns the store
// for chaining. Safe to call concurrently with Alerts() — both reads and
// writes are guarded by the store's RWMutex.
func (s *ChannelHealthStore) WithStaleThreshold(d time.Duration) *ChannelHealthStore {
	s.mu.Lock()
	s.staleThreshold = d
	s.mu.Unlock()
	return s
}

// WithNowFunc replaces the clock used by Alerts() to determine record age.
// Defaults to time.Now. Used by tests to inject deterministic time for
// boundary assertions (mirrors CircuitBreaker.WithNowFunc, Wave 12 Phase 2).
// Returns the store for chaining.
func (s *ChannelHealthStore) WithNowFunc(now func() time.Time) *ChannelHealthStore {
	s.mu.Lock()
	s.nowFunc = now
	s.mu.Unlock()
	return s
}

func (s *ChannelHealthStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.fetchLog = s.fetchLog[:0]
			return nil
		}
		return fmt.Errorf("read channel health file: %w", err)
	}
	var wrapper struct {
		Channels map[string]*ChannelHealthRecord `json:"channels"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return fmt.Errorf("unmarshal channel health: %w", err)
	}
	s.data = wrapper.Channels
	if s.data == nil {
		s.data = make(map[string]*ChannelHealthRecord)
	}
	return s.loadFetchLogLocked()
}

func (s *ChannelHealthStore) loadFetchLogLocked() error {
	b, err := os.ReadFile(s.fetchLogPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.fetchLog = s.fetchLog[:0]
			return nil
		}
		return fmt.Errorf("read channel fetch log: %w", err)
	}
	if len(b) == 0 {
		s.fetchLog = s.fetchLog[:0]
		return nil
	}
	if err := json.Unmarshal(b, &s.fetchLog); err != nil {
		return fmt.Errorf("unmarshal channel fetch log: %w", err)
	}
	if len(s.fetchLog) > fetchLogCap {
		s.fetchLog = s.fetchLog[len(s.fetchLog)-fetchLogCap:]
	}
	return nil
}

func (s *ChannelHealthStore) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wrapper := struct {
		Channels map[string]*ChannelHealthRecord `json:"channels"`
	}{Channels: s.data}
	b, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal channel health: %w", err)
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename channel health: %w", err)
	}
	return s.saveFetchLogLocked()
}

func (s *ChannelHealthStore) saveFetchLogLocked() error {
	b, err := json.MarshalIndent(s.fetchLog, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal channel fetch log: %w", err)
	}
	_ = os.MkdirAll(filepath.Dir(s.fetchLogPath), 0o755)
	tmp := s.fetchLogPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write fetch log temp: %w", err)
	}
	return os.Rename(tmp, s.fetchLogPath)
}

func (s *ChannelHealthStore) appendFetchLogLocked(entry ChannelFetchLogEntry) {
	s.fetchLog = append(s.fetchLog, entry)
	if len(s.fetchLog) > fetchLogCap {
		s.fetchLog = s.fetchLog[len(s.fetchLog)-fetchLogCap:]
	}
}

// RecentFetches returns the most recent fetch log entries, newest first, up to limit.
// If limit exceeds fetchLogCap it is clamped to fetchLogCap. A non-positive limit returns nil.
func (s *ChannelHealthStore) RecentFetches(limit int) []ChannelFetchLogEntry {
	if limit <= 0 {
		return nil
	}
	_ = s.load()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit > fetchLogCap {
		limit = fetchLogCap
	}
	n := len(s.fetchLog)
	if n == 0 {
		return []ChannelFetchLogEntry{}
	}
	if limit > n {
		limit = n
	}
	out := make([]ChannelFetchLogEntry, 0, limit)
	for i := n - 1; i >= n-limit; i-- {
		out = append(out, s.fetchLog[i])
	}
	return out
}

// Record updates the health record for a channel.
func (s *ChannelHealthStore) Record(channelID, status, errMsg string, opts ...RecordOption) error {
	_ = s.load()
	s.mu.Lock()
	rec := s.data[channelID]
	if rec == nil {
		rec = &ChannelHealthRecord{}
		s.data[channelID] = rec
	}
	rec.Status = status
	rec.LastFetchAt = time.Now().Format(time.RFC3339)
	if status == "ok" {
		rec.LastError = ""
		rec.Errors = nil // P2: clear stale error text so a healthy channel no longer shows old errors
		rec.LastSuccessAt = rec.LastFetchAt
	} else {
		rec.LastError = errMsg
		if errMsg != "" {
			rec.Errors = []string{errMsg}
		}
	}
	for _, opt := range opts {
		opt(rec)
	}
	s.appendFetchLogLocked(ChannelFetchLogEntry{
		Channel:   channelID,
		Status:    status,
		LatencyMs: rec.LatencyMs,
		Timestamp: rec.LastFetchAt,
		Error:     errMsg,
	})
	s.mu.Unlock()

	if s.pool != nil {
		dbErr := s.recordToDB(channelID, status, errMsg)
		if dbErr == nil {
			return s.save()
		}
		fmt.Fprintf(os.Stderr, "[ChannelHealth] DB write failed for %s, fallback to JSON: %v\n", channelID, dbErr)
	}
	return s.save()
}

func (s *ChannelHealthStore) recordToDB(channelID, status, errMsg string) error {
	if s.pool == nil {
		return fmt.Errorf("database pool not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	var lastSuccessAt *time.Time
	if status == "ok" {
		ts := now
		lastSuccessAt = &ts
	}

	var lastErrorPtr *string
	if errMsg != "" {
		lastErrorPtr = &errMsg
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO channel_health (channel_id, status, last_fetch_at, last_error, last_success_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (channel_id)
		DO UPDATE SET status = EXCLUDED.status,
					  last_fetch_at = EXCLUDED.last_fetch_at,
					  last_error = EXCLUDED.last_error,
					  last_success_at = EXCLUDED.last_success_at,
					  updated_at = EXCLUDED.updated_at
	`, channelID, status, now, lastErrorPtr, lastSuccessAt, now)
	if err != nil {
		return fmt.Errorf("exec channel health query: %w", err)
	}
	return nil
}

// All returns a shallow copy of every recorded channel health record. The
// returned map keys are channel IDs; values are copies of the stored records.
func (s *ChannelHealthStore) All() map[string]ChannelHealthRecord {
	_ = s.load()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]ChannelHealthRecord, len(s.data))
	for k, v := range s.data {
		if v == nil {
			continue
		}
		out[k] = *v
	}
	return out
}

// Get retrieves the health record for a channel (nil if missing).
func (s *ChannelHealthStore) Get(channelID string) *ChannelHealthRecord {
	_ = s.load()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if rec, ok := s.data[channelID]; ok {
		cp := *rec
		return &cp
	}
	if s.pool != nil {
		if rec := s.getFromDB(channelID); rec != nil {
			return rec
		}
	}
	return nil
}

func (s *ChannelHealthStore) getFromDB(channelID string) *ChannelHealthRecord {
	if s.pool == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var rec ChannelHealthRecord
	var lastFetchAt, lastSuccessAt *time.Time
	var lastError string
	err := s.pool.QueryRow(ctx, `
		SELECT status, last_fetch_at, COALESCE(last_error,''), last_success_at
		FROM channel_health
		WHERE channel_id = $1
	`, channelID).Scan(&rec.Status, &lastFetchAt, &lastError, &lastSuccessAt)
	if err != nil {
		return nil
	}
	if lastFetchAt != nil {
		rec.LastFetchAt = lastFetchAt.Format(time.RFC3339)
	}
	if lastSuccessAt != nil {
		rec.LastSuccessAt = lastSuccessAt.Format(time.RFC3339)
	}
	rec.LastError = lastError
	return &rec
}

// Alerts returns all non-ok channels whose LastFetchAt is within the
// configured staleThreshold (default: DefaultStaleThreshold = 1 hour).
// Records older than the threshold are filtered out — they represent
// historical errors that are no longer actionable signals (the channel
// may have recovered, or the error condition may have changed). Set
// staleThreshold to 0 via WithStaleThreshold to disable filtering.
func (s *ChannelHealthStore) Alerts() []ChannelAlert {
	_ = s.load()
	s.mu.RLock()
	threshold := s.staleThreshold
	now := s.nowFunc()
	s.mu.RUnlock()

	var alerts []ChannelAlert
	for id, rec := range s.data {
		if rec.Status != "ok" && rec.Status != "inactive" {
			// Skip stale records when threshold > 0 and LastFetchAt is
			// parseable. Unparseable timestamps are kept (defensive:
			// prefer showing over hiding when in doubt).
			if threshold > 0 && rec.LastFetchAt != "" {
				if ts, err := time.Parse(time.RFC3339, rec.LastFetchAt); err == nil {
					if now.Sub(ts) > threshold {
						continue
					}
				}
			}
			alerts = append(alerts, ChannelAlert{
				ChannelID: id,
				Status:    rec.Status,
				Error:     rec.LastError,
				FetchAt:   rec.LastFetchAt,
			})
		}
	}
	return alerts
}

// SyncAllToDB writes all in-memory health records to the database.
func (s *ChannelHealthStore) SyncAllToDB() error {
	if s.pool == nil {
		return fmt.Errorf("database pool not initialized")
	}
	_ = s.load()
	s.mu.RLock()
	defer s.mu.RUnlock()

	var failed []string
	for id, rec := range s.data {
		errMsg := ""
		if rec.Status != "ok" {
			errMsg = rec.LastError
		}
		if err := s.recordToDB(id, rec.Status, errMsg); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", id, err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("sync partial failure: %v", failed)
	}
	return nil
}

// ChannelAlert represents a single unhealthy channel.
type ChannelAlert struct {
	ChannelID string `json:"channel_id"`
	Status    string `json:"status"`
	Error     string `json:"error"`
	FetchAt   string `json:"fetch_at"`
}

// RecordOption configures optional fields on a ChannelHealthRecord.
type RecordOption func(*ChannelHealthRecord)

// WithRateLimitRemaining sets the rate limit remaining count.
func WithRateLimitRemaining(remaining int) RecordOption {
	return func(r *ChannelHealthRecord) { r.RateLimitRemaining = remaining }
}

// WithLatencyMs sets the latency in milliseconds.
func WithLatencyMs(ms int64) RecordOption {
	return func(r *ChannelHealthRecord) { r.LatencyMs = ms }
}

// WithLastDataAt sets the last data timestamp.
func WithLastDataAt(t time.Time) RecordOption {
	return func(r *ChannelHealthRecord) {
		r.LastDataAt = t.Format(time.RFC3339)
	}
}

// WithRecordsFetched sets the number of records fetched.
func WithRecordsFetched(n int) RecordOption {
	return func(r *ChannelHealthRecord) { r.RecordsFetched = n }
}

// WithSymbolsProcessed sets the number of symbols processed.
func WithSymbolsProcessed(n int) RecordOption {
	return func(r *ChannelHealthRecord) { r.SymbolsProcessed = n }
}

// RecordChannelFetch is a convenience helper for CLI tools.
func RecordChannelFetch(stateDir, channelID, status, errMsg string, rateRemaining int, latencyMs int64) {
	RecordChannelFetchWithPool(
		stateDir, channelID, status, errMsg, nil,
		WithRateLimitRemaining(rateRemaining),
		WithLatencyMs(latencyMs),
	)
}

// RecordChannelFetchWithPool is a convenience helper that accepts an optional DB pool.
func RecordChannelFetchWithPool(stateDir, channelID, status, errMsg string, pool *pgxpool.Pool, opts ...RecordOption) {
	store := NewChannelHealthStoreWithPool(stateDir, pool)
	if err := store.Record(channelID, status, errMsg, opts...); err != nil {
		fmt.Fprintf(os.Stderr, "[ChannelHealth] failed to record %s: %v\n", channelID, err)
	}
}
