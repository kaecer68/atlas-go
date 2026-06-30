package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mapTokenStore is an in-memory TokenStore for testing.
type mapTokenStore struct {
	mu          sync.RWMutex
	tokens      map[uuid.UUID]tokenRow // tokenID -> row
	byHash      map[string]uuid.UUID   // sha256(raw) -> tokenID
	unavailable bool                   // simulate DB down
}

type tokenRow struct {
	info    TokenInfo
	rawHash string // never expose raw token
}

func newMapTokenStore() *mapTokenStore {
	return &mapTokenStore{
		tokens: make(map[uuid.UUID]tokenRow),
		byHash: make(map[string]uuid.UUID),
	}
}

func (m *mapTokenStore) Lookup(ctx context.Context, rawToken string) (*TokenInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.unavailable {
		return nil, ErrDBUnavailable
	}

	h := hashToken(rawToken)
	id, ok := m.byHash[h]
	if !ok {
		return nil, ErrTokenNotFound
	}
	row := m.tokens[id]
	if row.info.RevokedAt != nil {
		return nil, ErrRevoked
	}
	if row.info.ExpiresAt != nil && time.Now().After(*row.info.ExpiresAt) {
		return nil, ErrExpired
	}
	now := time.Now()
	row.info.LastUsedAt = &now
	m.tokens[id] = row
	cp := row.info
	return &cp, nil
}

func (m *mapTokenStore) Register(ctx context.Context, reg TokenRegistration) (*TokenInfo, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	raw := "mcp-" + uuid.New().String()
	h := hashToken(raw)
	id := uuid.New()
	now := time.Now()

	info := TokenInfo{
		TokenID:         id,
		TokenHash:       h,
		TenantID:        reg.TenantID,
		AgentID:         reg.AgentID,
		Scopes:          reg.Scopes,
		RateLimitPerMin: reg.RateLimitPerMin,
		CreatedAt:       now,
		ExpiresAt:       reg.ExpiresAt,
	}
	if info.Scopes == nil {
		info.Scopes = []string{}
	}

	m.tokens[id] = tokenRow{info: info, rawHash: h}
	m.byHash[h] = id
	return &info, raw, nil
}

func (m *mapTokenStore) Revoke(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	row, ok := m.tokens[id]
	if !ok {
		return ErrTokenNotFound
	}
	now := time.Now()
	row.info.RevokedAt = &now
	m.tokens[id] = row
	// Keep the hash mapping so Lookup can detect the revoked status.
	return nil
}

func (m *mapTokenStore) Rotate(ctx context.Context, id uuid.UUID) (*TokenInfo, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	row, ok := m.tokens[id]
	if !ok {
		return nil, "", ErrTokenNotFound
	}
	if row.info.RevokedAt != nil {
		return nil, "", ErrRevoked
	}

	delete(m.byHash, row.rawHash)

	raw := "mcp-" + uuid.New().String()
	h := hashToken(raw)
	row.rawHash = h
	row.info.TokenHash = h
	m.byHash[h] = id

	cp := row.info
	return &cp, raw, nil
}

func (m *mapTokenStore) List(ctx context.Context) ([]TokenInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []TokenInfo
	for _, row := range m.tokens {
		if row.info.RevokedAt != nil {
			continue
		}
		cp := row.info
		out = append(out, cp)
	}
	return out, nil
}

func hashToken(raw string) string {
	s := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(s[:])
}

// ─── TokenStore unit tests ──────────────────────────────────────────

func TestTokenStore_RevokeImmediateReject(t *testing.T) {
	store := newMapTokenStore()
	_, raw, err := store.Register(context.Background(), TokenRegistration{
		TenantID: "t1",
		AgentID:  "a1",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	info, err := store.Lookup(context.Background(), raw)
	if err != nil {
		t.Fatalf("lookup before revoke: %v", err)
	}

	if err := store.Revoke(context.Background(), info.TokenID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	_, err = store.Lookup(context.Background(), raw)
	if !errors.Is(err, ErrRevoked) {
		t.Fatalf("expected ErrRevoked after revoke, got %v", err)
	}
}

func TestTokenStore_Rotate(t *testing.T) {
	store := newMapTokenStore()
	info, oldRaw, err := store.Register(context.Background(), TokenRegistration{
		TenantID: "t1",
		AgentID:  "a1",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	newInfo, newRaw, err := store.Rotate(context.Background(), info.TokenID)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if newInfo.TokenID != info.TokenID {
		t.Fatalf("rotate changed TokenID: %v != %v", newInfo.TokenID, info.TokenID)
	}

	_, err = store.Lookup(context.Background(), oldRaw)
	if !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("old token after rotate: expected ErrTokenNotFound, got %v", err)
	}

	got, err := store.Lookup(context.Background(), newRaw)
	if err != nil {
		t.Fatalf("new token after rotate: %v", err)
	}
	if got.TokenID != info.TokenID {
		t.Fatalf("wrong TokenID: %v", got.TokenID)
	}

	if oldRaw == newRaw {
		t.Fatal("old and new raw tokens must differ")
	}
}

func TestTokenStore_HashNeverRawToken(t *testing.T) {
	store := newMapTokenStore()
	_, raw, err := store.Register(context.Background(), TokenRegistration{
		TenantID: "t1",
		AgentID:  "a1",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	for _, row := range store.tokens {
		if row.rawHash == raw {
			t.Fatal("store contains raw token in rawHash field")
		}
	}
	if _, ok := store.byHash[raw]; ok {
		t.Fatal("byHash map keyed by raw token")
	}
}

func TestTokenStore_DBUnavailableFailsClosed(t *testing.T) {
	store := newMapTokenStore()
	store.unavailable = true

	auth := NewTokenAuth("")
	auth.SetStore(store)

	err := auth.Check("anything")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized on DB unavailable, got %v", err)
	}
}

func TestTokenStore_LookupUnknownFallsBackToEnv(t *testing.T) {
	os.Setenv("ATLAS_MCP_TOKEN", "fallback-token")
	defer os.Unsetenv("ATLAS_MCP_TOKEN")

	store := newMapTokenStore()
	auth := NewTokenAuth("fallback-token")
	auth.SetStore(store)

	if err := auth.Check("fallback-token"); err != nil {
		t.Fatalf("env fallback failed: %v", err)
	}

	err := auth.Check("wrong")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for wrong token, got %v", err)
	}
}

func TestRateLimit_PerTenantKey(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{PerMinute: 60, Burst: 1})

	if r := rl.Allow("toolX", "tenant-a"); !r.Allowed {
		t.Fatal("tenant-a first call should allow")
	}
	if r := rl.Allow("toolX", "tenant-a"); r.Allowed {
		t.Fatal("tenant-a second call should deny")
	}
	if r := rl.Allow("toolX", "tenant-b"); !r.Allowed {
		t.Fatal("tenant-b first call should allow (separate bucket)")
	}
	if r := rl.Allow("toolY", "anonymous"); !r.Allowed {
		t.Fatal("anonymous first call should allow")
	}
}

func TestMigration_UpDown(t *testing.T) {
	upPath := "../../../sql/migrations/000010_atlas_mcp_tokens.up.sql"
	downPath := "../../../sql/migrations/000010_atlas_mcp_tokens.down.sql"

	up, err := os.ReadFile(upPath)
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	down, err := os.ReadFile(downPath)
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}

	ups := string(up)
	downs := string(down)

	if !strContains(ups, "CREATE TABLE atlas_mcp_tokens") {
		t.Fatal("up migration missing CREATE TABLE")
	}
	if !strContains(ups, "idx_atlas_mcp_tokens_hash") {
		t.Fatal("up migration missing hash index")
	}
	if !strContains(ups, "idx_atlas_mcp_tokens_tenant") {
		t.Fatal("up migration missing tenant index")
	}
	if !strContains(ups, "token_hash VARCHAR(64) NOT NULL UNIQUE") {
		t.Fatal("up migration missing UNIQUE on token_hash")
	}
	if !strContains(ups, "rate_limit_per_min INT") {
		t.Fatal("up migration missing rate_limit_per_min")
	}
	if !strContains(downs, "DROP TABLE IF EXISTS atlas_mcp_tokens") {
		t.Fatal("down migration missing DROP TABLE")
	}
}

func TestTokenStore_ExpireCheck(t *testing.T) {
	store := newMapTokenStore()
	past := time.Now().Add(-time.Hour)
	_, raw, err := store.Register(context.Background(), TokenRegistration{
		TenantID:  "t1",
		AgentID:   "a1",
		ExpiresAt: &past,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err = store.Lookup(context.Background(), raw)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}

func TestTokenStore_LastUsedAtUpdated(t *testing.T) {
	store := newMapTokenStore()
	_, raw, err := store.Register(context.Background(), TokenRegistration{
		TenantID: "t1",
		AgentID:  "a1",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	info, err := store.Lookup(context.Background(), raw)
	if err != nil {
		t.Fatalf("first lookup: %v", err)
	}
	if info.LastUsedAt == nil {
		t.Fatal("expected LastUsedAt set after first lookup")
	}
	first := *info.LastUsedAt

	time.Sleep(10 * time.Millisecond)
	info2, err := store.Lookup(context.Background(), raw)
	if err != nil {
		t.Fatalf("second lookup: %v", err)
	}
	if info2.LastUsedAt == nil || !info2.LastUsedAt.After(first) {
		t.Fatalf("expected LastUsedAt updated after second lookup: %v -> %v", first, info2.LastUsedAt)
	}
}

func strContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestPGTokenStore_DBClosedFailsAuth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect to a non-existent port so Lookup triggers a connection failure.
	poolCfg, err := pgxpool.ParseConfig("postgres://127.0.0.1:1/postgres?connect_timeout=1")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	store := NewPGTokenStore(pool)
	_, err = store.Lookup(ctx, "any-token")
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
	if !errors.Is(err, ErrDBUnavailable) {
		t.Fatalf("expected ErrDBUnavailable, got %v (type=%T)", err, err)
	}
}
