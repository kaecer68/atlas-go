package subscription

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store manages user and subscription data in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates or opens a user store at the given data directory.
func NewStore(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("store mkdir: %w", err)
	}
	dbPath := filepath.Join(dataDir, "users.db")
	db, err := sql.Open("sqlite", dbPath+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("store open: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			tier TEXT NOT NULL DEFAULT 'free',
			trial_end INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		);
		CREATE TABLE IF NOT EXISTS subscription_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			event TEXT NOT NULL,
			timestamp INTEGER NOT NULL DEFAULT (strftime('%s','now')),
			FOREIGN KEY (user_id) REFERENCES users(id)
		);
	`)
	return err
}

// Register creates a new user with 7-day premium trial.
func (s *Store) Register(email, passwordHash string) (*User, error) {
	trialEnd := time.Now().Add(7 * 24 * time.Hour)
	result, err := s.db.Exec(
		`INSERT INTO users (email, password_hash, tier, trial_end) VALUES (?, ?, ?, ?)`,
		email, passwordHash, string(TierRegistered), trialEnd.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	id, _ := result.LastInsertId()
	u := &User{ID: id, Email: email, Tier: TierRegistered, TrialEnd: trialEnd, CreatedAt: time.Now()}
	s.recordEvent(id, "registered")
	s.recordEvent(id, "trial_start")
	return u, nil
}

// GetByEmail retrieves a user by email.
func (s *Store) GetByEmail(email string) (*User, error) {
	var u User
	var trialUnix, createdUnix int64
	err := s.db.QueryRow(
		`SELECT id, email, tier, trial_end, created_at FROM users WHERE email = ?`, email,
	).Scan(&u.ID, &u.Email, &u.Tier, &trialUnix, &createdUnix)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	u.TrialEnd = time.Unix(trialUnix, 0)
	u.CreatedAt = time.Unix(createdUnix, 0)
	return &u, nil
}

// Authenticate verifies email/password against the store.
func (s *Store) Authenticate(email, passwordHash string) (*User, error) {
	var storedHash string
	u, err := s.GetByEmail(email)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, fmt.Errorf("user not found")
	}
	err = s.db.QueryRow(
		`SELECT password_hash FROM users WHERE email = ?`, email,
	).Scan(&storedHash)
	if err != nil {
		return nil, fmt.Errorf("auth query: %w", err)
	}
	if storedHash != passwordHash {
		return nil, fmt.Errorf("invalid password")
	}
	s.recordEvent(u.ID, "login")
	return u, nil
}

// SetTier updates a user's tier and records the change.
func (s *Store) SetTier(userID int64, tier Tier) error {
	_, err := s.db.Exec(`UPDATE users SET tier = ? WHERE id = ?`, string(tier), userID)
	if err != nil {
		return err
	}
	s.recordEvent(userID, string(tier))
	return nil
}

func (s *Store) recordEvent(userID int64, event string) {
	_, _ = s.db.Exec(
		`INSERT INTO subscription_events (user_id, event) VALUES (?, ?)`,
		userID, event,
	)
}
