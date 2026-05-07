package data

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ChannelAlert struct {
	ChannelID string `json:"channel_id"`
	Status    string `json:"status"`
	Error     string `json:"error"`
	FetchAt   string `json:"fetch_at"`
}

type ChannelHealthRecord struct {
	Status        string `json:"status"`
	LastFetchAt   string `json:"last_fetch_at"`
	LastError     string `json:"last_error,omitempty"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
}

type ChannelHealthRecorder interface {
	Record(channelID, status, errMsg string) error
	Get(channelID string) *ChannelHealthRecord
	Alerts() []ChannelAlert
	SyncAllToDB() error
}

type channelHealthStore struct {
	path   string
	mu     sync.RWMutex
	data   map[string]*ChannelHealthRecord
	pool   *pgxpool.Pool
	loaded bool
}

func NewChannelHealthStoreWithPool(dir string, pool *pgxpool.Pool) ChannelHealthRecorder {
	return &channelHealthStore{
		path: filepath.Join(dir, "channel_health.json"),
		data: make(map[string]*ChannelHealthRecord),
		pool: pool,
	}
}

func (s *channelHealthStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return nil
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.loaded = true
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
	s.loaded = true
	return nil
}

func (s *channelHealthStore) save() error {
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

func (s *channelHealthStore) Record(channelID, status, errMsg string) error {
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
	}
	s.mu.Unlock()

	if s.pool != nil {
		dbErr := s.recordToDB(channelID, status, errMsg)
		if dbErr != nil {
			log.Printf("[ChannelHealth] DB write failed for %s: %v", channelID, dbErr)
		}
	}
	return s.save()
}

func (s *channelHealthStore) recordToDB(channelID, status, errMsg string) error {
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

func (s *channelHealthStore) Get(channelID string) *ChannelHealthRecord {
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

func (s *channelHealthStore) getFromDB(channelID string) *ChannelHealthRecord {
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

func (s *channelHealthStore) Alerts() []ChannelAlert {
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

func (s *channelHealthStore) SyncAllToDB() error {
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

func (s *channelHealthStore) alertsFromDB() []ChannelAlert {
	if s.pool == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := s.pool.Query(ctx, `
		SELECT channel_id, status, COALESCE(last_error,''), last_fetch_at
		FROM channel_health
		WHERE status NOT IN ('ok','inactive')
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var alerts []ChannelAlert
	for rows.Next() {
		var a ChannelAlert
		var fetchAt *time.Time
		if err := rows.Scan(&a.ChannelID, &a.Status, &a.Error, &fetchAt); err != nil {
			continue
		}
		if fetchAt != nil {
			a.FetchAt = fetchAt.Format(time.RFC3339)
		}
		alerts = append(alerts, a)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return alerts
}
