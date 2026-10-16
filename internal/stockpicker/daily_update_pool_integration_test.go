//go:build integration

package stockpicker

// FU-20260930-12, real-server half.
//
// The hermetic tests in daily_update_pool_lifecycle_test.go prove that the run
// hands its pool back (they install an opener that never dials). This test
// proves the RESOURCE claim against a real PostgreSQL: the connections the run
// opened are gone once it returns, so an hourly tick can no longer accumulate
// connections.
//
// How the measurement stays exact: the run reads DATABASE_URL, so the test
// points that variable at the same database with a marker
// application_name. Every connection the run's pools (and its migration
// connection) opens carries that marker, and counting
// pg_stat_activity WHERE application_name = marker counts THIS run only — a
// backend count over the whole database would be flaky, because `go test`
// runs packages in parallel against one shared database (CI).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// poolProbeAppName marks the connections this test's runs open.
const poolProbeAppName = "atlas-fu20260930-12-probe"

// probeDSN returns the integration DSN with the marker application_name added.
func probeDSN(t *testing.T) string {
	t.Helper()
	dsn := testdb.URL(t)
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "application_name=" + poolProbeAppName
}

// probeBackends counts the live backends carrying the marker.
func probeBackends(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_stat_activity WHERE application_name = $1`,
		poolProbeAppName).Scan(&n); err != nil {
		t.Fatalf("count probe backends: %v", err)
	}
	return n
}

// waitForProbeBackends polls until the marker count reaches want (or the
// deadline passes) and returns the last observation. A pool that was never
// closed keeps its idle connections for the pool's MaxConnIdleTime — 30 minutes
// by default — so a leaked pool can never settle within this window, while a
// closed one settles immediately.
func waitForProbeBackends(t *testing.T, pool *pgxpool.Pool, want int, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := probeBackends(t, pool)
	for last > want && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		last = probeBackends(t, pool)
	}
	return last
}

// realRecordingOpener replaces the opener with the real one and records the
// pools it hands out, so the test can check them after the run returns.
func realRecordingOpener(t *testing.T) *[]*pgxpool.Pool {
	t.Helper()
	opened := &[]*pgxpool.Pool{}
	previous := openPostgresPool
	openPostgresPool = func(ctx context.Context) (*pgxpool.Pool, error) {
		pool, err := openPostgres(ctx)
		if err != nil {
			return nil, err
		}
		*opened = append(*opened, pool)
		return pool, nil
	}
	t.Cleanup(func() { openPostgresPool = previous })
	return opened
}

// serverWorkDir builds a work directory the run accepts (parameters.json) and
// that absorbs any state file a non-dry run writes, so nothing lands in the
// checked-out tree.
func serverWorkDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs repo root: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "configs", "parameters.json"))
	if err != nil {
		t.Fatalf("read parameters.json: %v", err)
	}
	dst := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dst, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "configs", "parameters.json"), raw, 0o644); err != nil {
		t.Fatalf("write parameters.json: %v", err)
	}
	return dst
}

// TestRunDailyUpdateReleasesItsPoolAgainstPostgres drives the real pipeline
// against a live database and asserts that neither open site leaks.
func TestRunDailyUpdateReleasesItsPoolAgainstPostgres(t *testing.T) {
	dsn := testdb.URL(t)
	observer := testdb.Connect(t, dsn)
	defer observer.Close()

	var dbName string
	if err := observer.QueryRow(context.Background(), `SELECT current_database()`).Scan(&dbName); err != nil {
		t.Fatalf("current_database: %v", err)
	}

	// The run resolves its own DSN from the environment, so point it at the
	// marked one for the duration of the test.
	t.Setenv("DATABASE_URL", probeDSN(t))
	// migrations are addressed relative to the repository root.
	t.Chdir(filepath.Join("..", ".."))

	baseline := probeBackends(t, observer)
	if baseline != 0 {
		t.Fatalf("probe baseline = %d, want 0 (the marker must only be used by this test's runs)", baseline)
	}

	cases := []struct {
		name    string
		dryRun  bool
		dual    bool
		comment string
	}{
		{"derived destination pool", false, true, "resolvePGDest opens the pool for the PG destination"},
		{"panel pool of a dry run", true, true, "the panel build is the only open site on a dry run"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sentinel := testdb.UnconnectedPool(t, unreachableDSN)
			ledger.SetPostgresPool(sentinel)
			t.Cleanup(func() { ledger.SetPostgresPool(nil) })

			opened := realRecordingOpener(t)

			// A non-dry run writes data/state/stock_win_rate.json into WorkDir,
			// hence the disposable work directory. An error is tolerated: what
			// this test pins is the lifecycle on EVERY return path, and the
			// empty database can legitimately stop the pipeline early.
			res, err := RunDailyUpdate(context.Background(), RunDailyOptions{
				WorkDir:     serverWorkDir(t),
				Backend:     "postgres",
				ExpectDB:    dbName,
				DualWrite:   tc.dual,
				DryRun:      tc.dryRun,
				Idempotency: IdempotencyDay,
				AsOf:        DateOnlyUTC(time.Now().UTC()),
			})
			t.Logf("run result: skipped=%v outcomes=%d primary=%s err=%v (%s)", res.Skipped, res.Outcomes, res.Primary, err, tc.comment)

			if len(*opened) == 0 {
				t.Fatal("the run opened no pool; it no longer exercises the path under test")
			}
			for i, pool := range *opened {
				if !poolIsClosed(t, pool) {
					t.Errorf("pool %d is still open after the run returned (FU-20260930-12)", i)
				}
			}

			// The connections really are gone on the server side, not just
			// released back into a live pool.
			if got := waitForProbeBackends(t, observer, baseline, 5*time.Second); got != baseline {
				t.Errorf("live probe backends after the run = %d, want %d: the run's connections outlived it (pool not closed)", got, baseline)
			}

			if got := ledger.PostgresPool(); got != sentinel {
				t.Errorf("ledger injection after the run = %p, want the pre-run pool %p (a closed pool must not stay installed)", got, sentinel)
			}
		})
	}
}
