package monitoring

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ChannelHealthRecord stores the last fetch result for a single channel.
type ChannelHealthRecord struct {
	Status        string `json:"status"`        // ok | warn | error | inactive
	LastFetchAt   string `json:"last_fetch_at"` // RFC3339
	LastError     string `json:"last_error,omitempty"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
}

// ChannelHealthStore persists channel fetch outcomes.
type ChannelHealthStore struct {
	path string
	mu   sync.RWMutex
	data map[string]*ChannelHealthRecord
}

// NewChannelHealthStore creates or loads a health store at the given directory.
func NewChannelHealthStore(dir string) *ChannelHealthStore {
	return &ChannelHealthStore{
		path: filepath.Join(dir, "channel_health.json"),
		data: make(map[string]*ChannelHealthRecord),
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
		return err
	}
	var wrapper struct {
		Channels map[string]*ChannelHealthRecord `json:"channels"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return err
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
		return err
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0755)
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Record updates the health record for a channel.
func (s *ChannelHealthStore) Record(channelID, status, errMsg string) error {
	if err := s.load(); err != nil {
		// non-fatal: start with empty map
	}
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
	return s.save()
}

// Get retrieves the health record for a channel (nil if missing).
func (s *ChannelHealthStore) Get(channelID string) *ChannelHealthRecord {
	_ = s.load()
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec := s.data[channelID]
	if rec == nil {
		return nil
	}
	// return a copy
	cp := *rec
	return &cp
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

// ChannelAlert represents a single unhealthy channel.
type ChannelAlert struct {
	ChannelID string `json:"channel_id"`
	Status    string `json:"status"`
	Error     string `json:"error"`
	FetchAt   string `json:"fetch_at"`
}

// RecordChannelFetch is a convenience helper for CLI tools.
func RecordChannelFetch(stateDir, channelID, status, errMsg string) {
	store := NewChannelHealthStore(stateDir)
	if err := store.Record(channelID, status, errMsg); err != nil {
		fmt.Fprintf(os.Stderr, "[ChannelHealth] failed to record %s: %v\n", channelID, err)
	}
}
