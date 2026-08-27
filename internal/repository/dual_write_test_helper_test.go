//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/testdb"
)

// testMigrationsPath is the relative path from internal/repository/ to sql/migrations/.
const testMigrationsPath = "../../sql/migrations"

// connectTestDB connects to PostgreSQL (DATABASE_URL only, no hardcoded DSN)
// and returns the pool. See testdb.Pool for the CI/local skip policy.
func connectTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t, testMigrationsPath)
}

// newTestDualWrite creates a DualWriteRepository backed by a real PostgreSQL
// connection and in-memory JSONL stores.
//
// It TRUNCATEs the dual-write tables at the start of every test (M8). The
// dual-write assertions expect a clean table per test (e.g. QueryTopSymbols
// asserts an exact count), so the previous full-table DELETE at cleanup only
// was both insufficient and destructive to other packages sharing the CI
// postgres service. CI runs integration packages serially (-p 1), so a
// per-test TRUNCATE is a deterministic clean slate.
func newTestDualWrite(t *testing.T) *DualWriteRepository {
	t.Helper()
	pool := connectTestDB(t)

	tables := []string{
		"metrics",
		"alerts",
		"recommendation_outcomes",
		"capital_flow",
		"export_statistics",
		"screening_rejects",
		"session_summaries",
		"human_interventions",
	}
	truncate := func() {
		ctx := context.Background()
		if _, err := pool.Exec(ctx, "TRUNCATE TABLE "+strings.Join(tables, ", ")); err != nil {
			t.Errorf("truncate dual-write tables: %v", err)
		}
	}
	truncate()
	t.Cleanup(truncate)

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

	return repo
}
