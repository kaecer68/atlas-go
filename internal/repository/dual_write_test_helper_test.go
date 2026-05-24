//go:build integration

package repository

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/db"
)

// testPGURL is the default PostgreSQL connection string for integration tests.
// Override via DATABASE_URL environment variable.
const testPGURL = "postgres://atlas:atlas@localhost:5432/atlas?sslmode=disable"

// testMigrationsPath is the relative path from internal/repository/ to sql/migrations/.
const testMigrationsPath = "../../sql/migrations"

// connectTestDB connects to PostgreSQL, runs migrations, and returns the pool.
// Skips the test if PostgreSQL is not available.
func connectTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testPGURL
	}

	pool, err := db.Init(context.Background(), dsn, testMigrationsPath)
	if err != nil {
		t.Skipf("Skipping integration test: PostgreSQL not available (%v). Start with: docker compose up -d postgres", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newTestDualWrite creates a DualWriteRepository backed by a real PostgreSQL
// connection and in-memory JSONL stores. It registers cleanup to drop all test
// data when the test completes.
func newTestDualWrite(t *testing.T) *DualWriteRepository {
	t.Helper()
	pool := connectTestDB(t)

	// Use the well-known mock JSONL stores from repository_test.go
	repo := NewDualWriteRepository(
		pool,
		&mockAlertStore{},
		&mockMetricsStore{},
		&mockOutcomeStore{},
		&mockScreeningRejectStore{},
		&mockSessionSummaryStore{},
		&mockHumanInterventionStore{},
	)

	// Clean up all test data after the test completes.
	t.Cleanup(func() {
		ctx := context.Background()
		for _, table := range []string{
			"metrics",
			"alerts",
			"recommendation_outcomes",
			"capital_flow",
			"export_statistics",
			"screening_rejects",
			"session_summaries",
			"human_interventions",
		} {
			if _, err := pool.Exec(ctx, "DELETE FROM "+table); err != nil {
				t.Errorf("cleanup delete from %s: %v", table, err)
			}
		}
	})

	return repo
}
