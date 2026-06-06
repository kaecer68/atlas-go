package monitoring

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

// ChannelHealthStore persists channel fetch outcomes.
type ChannelHealthStore struct {
	path string
	mu   sync.RWMutex
	data map[string]*ChannelHealthRecord
	pool *pgxpool.Pool
}

// NewChannelHealthStore creates or loads a health store at the given directory.
func NewChannelHealthStore(dir string) *ChannelHealthStore {
	return NewChannelHealthStoreWithPool(dir, nil)
}

// NewChannelHealthStoreWithPool creates a health store with an optional DB pool.
func NewChannelHealthStoreWithPool(dir string, pool *pgxpool.Pool) *ChannelHealthStore {
	return &ChannelHealthStore{
		path: filepath.Join(dir, "channel_health.json"),
		data: make(map[string]*ChannelHealthRecord),
		pool: pool,
	}
}

func (s *ChannelHealthStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
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
	return os.Rename(tmp, s.path)
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

// Alerts returns all channels with non-ok status.
func (s *ChannelHealthStore) Alerts() []ChannelAlert {
	_ = s.load()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var alerts []ChannelAlert
	for id, rec := range s.data {
		if rec.Status != "ok" && rec.Status != "inactive" {
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
