package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGTokenStore implements TokenStore backed by PostgreSQL.
type PGTokenStore struct {
	pool *pgxpool.Pool
}

// NewPGTokenStore creates a PGTokenStore. pool must not be nil.
func NewPGTokenStore(pool *pgxpool.Pool) *PGTokenStore {
	return &PGTokenStore{pool: pool}
}

func (s *PGTokenStore) Lookup(ctx context.Context, rawToken string) (*TokenInfo, error) {
	h := hashTokenRaw(rawToken)
	row := s.pool.QueryRow(ctx,
		`SELECT token_id, token_hash, tenant_id, agent_id, scopes,
		        rate_limit_per_min, created_at, expires_at, revoked_at, last_used_at
		 FROM atlas_mcp_tokens WHERE token_hash = $1`, h)
	var info TokenInfo
	var scopesRaw []byte
	err := row.Scan(
		&info.TokenID, &info.TokenHash, &info.TenantID, &info.AgentID, &scopesRaw,
		&info.RateLimitPerMin, &info.CreatedAt, &info.ExpiresAt, &info.RevokedAt, &info.LastUsedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("%w: lookup token: %v", ErrDBUnavailable, err)
	}
	if err := json.Unmarshal(scopesRaw, &info.Scopes); err != nil {
		return nil, fmt.Errorf("lookup token: decode scopes: %w", err)
	}
	if info.RevokedAt != nil {
		return nil, ErrRevoked
	}
	if info.ExpiresAt != nil && time.Now().After(*info.ExpiresAt) {
		return nil, ErrExpired
	}
	now := time.Now()
	_, _ = s.pool.Exec(ctx,
		`UPDATE atlas_mcp_tokens SET last_used_at = $1 WHERE token_id = $2`,
		now, info.TokenID,
	)
	info.LastUsedAt = &now
	return &info, nil
}

func (s *PGTokenStore) Register(ctx context.Context, reg TokenRegistration) (*TokenInfo, string, error) {
	raw, err := generateRawToken()
	if err != nil {
		return nil, "", fmt.Errorf("register: generate token: %w", err)
	}
	h := hashTokenRaw(raw)
	id := uuid.New()
	now := time.Now()

	scopes := reg.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return nil, "", fmt.Errorf("register: encode scopes: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO atlas_mcp_tokens
		 (token_id, token_hash, tenant_id, agent_id, scopes, rate_limit_per_min, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, h, reg.TenantID, reg.AgentID, scopesJSON, reg.RateLimitPerMin, now, reg.ExpiresAt,
	)
	if err != nil {
		return nil, "", fmt.Errorf("register: insert: %w", err)
	}

	info := &TokenInfo{
		TokenID:         id,
		TokenHash:       h,
		TenantID:        reg.TenantID,
		AgentID:         reg.AgentID,
		Scopes:          scopes,
		RateLimitPerMin: reg.RateLimitPerMin,
		CreatedAt:       now,
		ExpiresAt:       reg.ExpiresAt,
	}
	return info, raw, nil
}

func (s *PGTokenStore) Revoke(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE atlas_mcp_tokens SET revoked_at = now() WHERE token_id = $1 AND revoked_at IS NULL`,
		id,
	)
	if err != nil {
		return fmt.Errorf("revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (s *PGTokenStore) Rotate(ctx context.Context, id uuid.UUID) (*TokenInfo, string, error) {
	raw, err := generateRawToken()
	if err != nil {
		return nil, "", fmt.Errorf("rotate: generate token: %w", err)
	}
	h := hashTokenRaw(raw)

	tag, err := s.pool.Exec(ctx,
		`UPDATE atlas_mcp_tokens
		 SET token_hash = $1 WHERE token_id = $2 AND revoked_at IS NULL`,
		h, id,
	)
	if err != nil {
		return nil, "", fmt.Errorf("rotate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var revoked bool
		err2 := s.pool.QueryRow(ctx,
			`SELECT revoked_at IS NOT NULL FROM atlas_mcp_tokens WHERE token_id = $1`, id,
		).Scan(&revoked)
		if err2 != nil {
			if errors.Is(err2, pgx.ErrNoRows) {
				return nil, "", ErrTokenNotFound
			}
			return nil, "", fmt.Errorf("rotate: check revoked: %w", err2)
		}
		return nil, "", ErrRevoked
	}

	var info TokenInfo
	var scopesRaw []byte
	err = s.pool.QueryRow(ctx,
		`SELECT token_id, token_hash, tenant_id, agent_id, scopes,
		        rate_limit_per_min, created_at, expires_at, revoked_at, last_used_at
		 FROM atlas_mcp_tokens WHERE token_id = $1`, id,
	).Scan(
		&info.TokenID, &info.TokenHash, &info.TenantID, &info.AgentID, &scopesRaw,
		&info.RateLimitPerMin, &info.CreatedAt, &info.ExpiresAt, &info.RevokedAt, &info.LastUsedAt,
	)
	if err != nil {
		return nil, "", fmt.Errorf("rotate: re-read: %w", err)
	}
	if err := json.Unmarshal(scopesRaw, &info.Scopes); err != nil {
		return nil, "", fmt.Errorf("rotate: decode scopes: %w", err)
	}
	return &info, raw, nil
}

func (s *PGTokenStore) List(ctx context.Context) ([]TokenInfo, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT token_id, token_hash, tenant_id, agent_id, scopes,
		        rate_limit_per_min, created_at, expires_at, revoked_at, last_used_at
		 FROM atlas_mcp_tokens WHERE revoked_at IS NULL ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()

	var out []TokenInfo
	for rows.Next() {
		var info TokenInfo
		var scopesRaw []byte
		if err := rows.Scan(
			&info.TokenID, &info.TokenHash, &info.TenantID, &info.AgentID, &scopesRaw,
			&info.RateLimitPerMin, &info.CreatedAt, &info.ExpiresAt, &info.RevokedAt, &info.LastUsedAt,
		); err != nil {
			return nil, fmt.Errorf("list: scan: %w", err)
		}
		if err := json.Unmarshal(scopesRaw, &info.Scopes); err != nil {
			return nil, fmt.Errorf("list: decode scopes: %w", err)
		}
		out = append(out, info)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list: rows: %w", err)
	}
	return out, nil
}

func hashTokenRaw(raw string) string {
	s := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(s[:])
}

func generateRawToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "mcp-" + hex.EncodeToString(b), nil
}
