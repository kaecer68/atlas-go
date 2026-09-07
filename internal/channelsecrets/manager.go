package channelsecrets

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// Provider whitelist (issue #1776): the two data channels that actually
// consume API keys at runtime. TEJ is shelved (decision on #1758), fubon is
// a broker not a data channel, yahoo is keyless — none of them belong here.
var AllowedProviders = map[string]bool{
	"finmind": true,
	"fugle":   true,
}

// MinAPIKeyLen / MaxAPIKeyLen bound the accepted key length (same bounds as
// the pre-#1777 panel: 8..512) to reject accidental pastes of unrelated text.
const (
	MinAPIKeyLen = 8
	MaxAPIKeyLen = 512
)

// Applier applies a plaintext key to a live consumer (e.g. the shared
// FinMind/Fugle client). Must be safe to call at any time.
type Applier func(plaintextKey string)

// KeyStatus is the admin-facing view of one channel key (never plaintext).
type KeyStatus struct {
	Provider  string `json:"provider"`
	MaskedKey string `json:"masked_key"` // "****" + last 4; "" when unset
	Source    string `json:"source"`     // "db" (persisted override) | "env" (startup only) | "unset"
	UpdatedAt string `json:"updated_at,omitempty"`
	UpdatedBy string `json:"updated_by,omitempty"`
}

// Manager owns the encrypted store and the runtime apply fan-out.
type Manager struct {
	mu       sync.Mutex
	store    Store
	master   []byte // nil = encryption unavailable (writes fail closed)
	appliers map[string]Applier
}

// NewManager wires the manager to a store. The master key is loaded once
// from MasterKeyEnv; when unset the manager still serves reads of unset
// status but refuses writes (fail-closed — see package doc).
func NewManager(store Store) (*Manager, error) {
	master, err := LoadMasterKey()
	if err != nil && !errors.Is(err, ErrNoMasterKey) {
		return nil, err
	}
	return &Manager{
		store:    store,
		master:   master,
		appliers: make(map[string]Applier),
	}, nil
}

// RegisterApplier wires provider → live consumer. Later Set() calls fan out.
func (m *Manager) RegisterApplier(provider string, a Applier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appliers[provider] = a
}

// EncryptionEnabled reports whether writes are possible (master key set).
func (m *Manager) EncryptionEnabled() bool {
	return m.master != nil
}

// LoadOverrides decrypts all stored keys for the startup merge (DB override
// over .env). Undecryptable rows are skipped with a log line — a wrong or
// lost master key must degrade to "ignore override", never crash startup
// and never leak the row.
func (m *Manager) LoadOverrides(ctx context.Context) map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.master == nil {
		return nil
	}
	recs, err := m.store.Load(ctx)
	if err != nil {
		log.Printf("[channelsecrets] load overrides failed (non-fatal): %v", err)
		return nil
	}
	out := make(map[string]string, len(recs))
	for provider, rec := range recs {
		pt, err := Decrypt(m.master, rec.EncryptedKey)
		if err != nil {
			log.Printf("[channelsecrets] skip undecryptable override for %s", provider)
			continue
		}
		out[provider] = string(pt)
	}
	return out
}

// Set validates, encrypts, persists, then applies the key to live consumers.
// Apply runs even if the caller's request later fails to serialize — the
// key is already durably stored, and the shared clients must not diverge
// from the store.
func (m *Manager) Set(ctx context.Context, provider, apiKey, updatedBy string) (KeyStatus, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !AllowedProviders[provider] {
		return KeyStatus{}, fmt.Errorf("%w: %q", ErrUnknownProvider, provider)
	}
	apiKey = strings.TrimSpace(apiKey)
	if len(apiKey) < MinAPIKeyLen || len(apiKey) > MaxAPIKeyLen {
		return KeyStatus{}, ErrInvalidKey
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.master == nil {
		return KeyStatus{}, ErrNoMasterKey
	}
	enc, err := Encrypt(m.master, []byte(apiKey))
	if err != nil {
		return KeyStatus{}, err
	}
	now := time.Now().UTC()
	if err := m.store.Upsert(ctx, provider, SecretRecord{EncryptedKey: enc, UpdatedBy: updatedBy, UpdatedAt: now}); err != nil {
		return KeyStatus{}, err
	}
	if a, ok := m.appliers[provider]; ok {
		a(apiKey)
	}
	return KeyStatus{
		Provider:  provider,
		MaskedKey: MaskKey(apiKey),
		Source:    "db",
		UpdatedAt: now.Format(time.RFC3339),
		UpdatedBy: updatedBy,
	}, nil
}

// Status lists the admin-facing view for all known providers. env-source
// rows are masked values observed at startup (SetEnvObserved), so operators
// can see "key came from .env" without a plaintext read path.
func (m *Manager) Status(ctx context.Context, envObserved map[string]string) ([]KeyStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, err := m.store.Load(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]KeyStatus, 0, len(AllowedProviders))
	for provider := range AllowedProviders {
		st := KeyStatus{Provider: provider, Source: "unset"}
		if rec, ok := recs[provider]; ok {
			st.Source = "db"
			st.UpdatedAt = rec.UpdatedAt.UTC().Format(time.RFC3339)
			st.UpdatedBy = rec.UpdatedBy
			if m.master != nil {
				if pt, err := Decrypt(m.master, rec.EncryptedKey); err == nil {
					st.MaskedKey = MaskKey(string(pt))
				}
			}
		} else if envKey := envObserved[provider]; envKey != "" {
			st.Source = "env"
			st.MaskedKey = MaskKey(envKey)
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out, nil
}

// ErrUnknownProvider / ErrInvalidKey are sentinel errors for handler mapping.
var (
	ErrUnknownProvider = errors.New("channelsecrets: unknown provider")
	ErrInvalidKey      = errors.New("channelsecrets: api key length invalid")
)

// MaskKey renders a key safe for display: last 4 characters only, per the
// issue requirement (write-only panel; at most 尾 4 碼 visible).
func MaskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}
