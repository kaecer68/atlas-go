package channelsecrets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// SecretRecord is one stored channel key row (ciphertext only — plaintext
// never leaves the Manager).
type SecretRecord struct {
	EncryptedKey []byte
	UpdatedBy    string
	UpdatedAt    time.Time
}

// Store persists encrypted channel keys. Postgres in production
// (backend=postgres + injected pool), job-local SQLite as dev fallback —
// the same backend split as the ledger store factories.
type Store interface {
	// Load returns all stored records keyed by provider.
	Load(ctx context.Context) (map[string]SecretRecord, error)
	// Upsert stores (or replaces) the encrypted key for a provider.
	Upsert(ctx context.Context, provider string, rec SecretRecord) error
}

// NewStore returns the backend-appropriate Store. backend "postgres" with a
// non-nil pool uses Postgres (production SSoT); anything else falls back to
// the job-local SQLite artifact (dev/CLI).
func NewStore(ctx context.Context, backend string, pool *pgxpool.Pool, workDir string) (Store, error) {
	if backend == "postgres" && pool != nil {
		return &postgresStore{pool: pool}, nil
	}
	if workDir == "" {
		return nil, errors.New("channelsecrets: workDir required for sqlite store")
	}
	dbPath := filepath.Join(workDir, "data", "state", "atlas.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("channelsecrets: mkdir %s: %w", filepath.Dir(dbPath), err)
	}
	db, err := ledger.OpenSQLiteDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("channelsecrets: open sqlite %s: %w", dbPath, err)
	}
	s := &sqliteStore{db: db}
	if err := s.ensureTable(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// --- Postgres (production) ---

type postgresStore struct {
	pool *pgxpool.Pool
}

func (s *postgresStore) Load(ctx context.Context) (map[string]SecretRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT provider, api_key_enc, updated_by, updated_at FROM channel_secrets`)
	if err != nil {
		return nil, fmt.Errorf("channelsecrets: load: %w", err)
	}
	defer rows.Close()
	out := make(map[string]SecretRecord)
	for rows.Next() {
		var provider, updatedBy string
		var enc []byte
		var updatedAt time.Time
		if err := rows.Scan(&provider, &enc, &updatedBy, &updatedAt); err != nil {
			return nil, fmt.Errorf("channelsecrets: scan: %w", err)
		}
		out[provider] = SecretRecord{EncryptedKey: enc, UpdatedBy: updatedBy, UpdatedAt: updatedAt}
	}
	return out, rows.Err()
}

func (s *postgresStore) Upsert(ctx context.Context, provider string, rec SecretRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO channel_secrets (provider, api_key_enc, updated_by, updated_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (provider) DO UPDATE
		 SET api_key_enc = EXCLUDED.api_key_enc,
		     updated_by = EXCLUDED.updated_by,
		     updated_at = EXCLUDED.updated_at`,
		provider, rec.EncryptedKey, rec.UpdatedBy, rec.UpdatedAt)
	if err != nil {
		return fmt.Errorf("channelsecrets: upsert %s: %w", provider, err)
	}
	return nil
}

// --- SQLite (dev/CLI fallback; job-local atlas.db) ---

type sqliteStore struct {
	db *sql.DB
}

func (s *sqliteStore) ensureTable(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS channel_secrets (
			provider     TEXT PRIMARY KEY,
			api_key_enc  BLOB NOT NULL,
			updated_by   TEXT NOT NULL DEFAULT '',
			updated_at   TEXT NOT NULL
		)`)
	if err != nil {
		return fmt.Errorf("channelsecrets: ensure table: %w", err)
	}
	return nil
}

func (s *sqliteStore) Load(ctx context.Context) (map[string]SecretRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT provider, api_key_enc, updated_by, updated_at FROM channel_secrets`)
	if err != nil {
		return nil, fmt.Errorf("channelsecrets: load: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]SecretRecord)
	for rows.Next() {
		var provider, updatedBy, updatedAt string
		var enc []byte
		if err := rows.Scan(&provider, &enc, &updatedBy, &updatedAt); err != nil {
			return nil, fmt.Errorf("channelsecrets: scan: %w", err)
		}
		ts, _ := time.Parse(time.RFC3339, updatedAt)
		out[provider] = SecretRecord{EncryptedKey: enc, UpdatedBy: updatedBy, UpdatedAt: ts}
	}
	return out, rows.Err()
}

func (s *sqliteStore) Upsert(ctx context.Context, provider string, rec SecretRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO channel_secrets (provider, api_key_enc, updated_by, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (provider) DO UPDATE
		 SET api_key_enc = excluded.api_key_enc,
		     updated_by = excluded.updated_by,
		     updated_at = excluded.updated_at`,
		provider, rec.EncryptedKey, rec.UpdatedBy,
		rec.UpdatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("channelsecrets: upsert %s: %w", provider, err)
	}
	return nil
}
