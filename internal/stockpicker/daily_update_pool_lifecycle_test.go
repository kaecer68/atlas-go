package stockpicker

// FU-20260930-12 — the daily update opened a pgxpool per run and never closed
// it.
//
// The defect is a lifecycle bug, so these tests drive a WHOLE RunDailyUpdate
// call with a pool that never dials (internal/testdb.UnconnectedPool) and check
// what happened to the pool afterwards. Without that, the only way to see the
// leak would be an integration test against a live server — a lifecycle bug
// that only a live server can see regrows the moment the integration job is
// skipped.
//
// Why the pool survives the old code: pgx v5 documents that closing the *sql.DB
// returned by stdlib.OpenDBFromPool does NOT close the pool it wraps, and the
// panel path never closed its pool at all.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
	"github.com/kaecer68/atlas-go/internal/testutil/reporoot"
)

// unreachableDSN is never dialed: the pool under test is created with
// UnconnectedPool, so the address only has to parse.
const unreachableDSN = "postgres://atlas:atlas@127.0.0.1:1/atlas_none"

// poolIsClosed reports whether a pool was closed, by asking it for a
// connection: a closed pool answers with pgxpool/puddle's ErrClosedPool
// ("closed pool") BEFORE any dial is attempted, while an open but unreachable
// pool fails with a connection error instead.
func poolIsClosed(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	conn, err := pool.Acquire(context.Background())
	if err == nil {
		conn.Release()
		return false
	}
	return strings.Contains(err.Error(), "closed pool")
}

// recordingOpener installs a pool opener that hands out unconnected pools and
// remembers them, so a test can inspect the lifecycle after RunDailyUpdate
// returns. It mirrors openPostgres' side effect (installing the pool in the
// ledger factory) because that side effect is what makes the restore-on-release
// logic necessary.
func recordingOpener(t *testing.T) *[]*pgxpool.Pool {
	t.Helper()
	opened := &[]*pgxpool.Pool{}
	previous := openPostgresPool
	openPostgresPool = func(context.Context) (*pgxpool.Pool, error) {
		pool := testdb.UnconnectedPool(t, unreachableDSN)
		*opened = append(*opened, pool)
		ledger.SetPostgresPool(pool)
		return pool, nil
	}
	t.Cleanup(func() { openPostgresPool = previous })
	return opened
}

// stubTargetGuard removes the M12 target assertion for the duration of a test.
// The guard is what production uses to refuse a stray DATABASE_URL; these tests
// are about what the run does with the pools it opens afterwards, and the guard
// would otherwise dial for real before either open site is reached.
func stubTargetGuard(t *testing.T) {
	t.Helper()
	previous := guardPostgresTarget
	guardPostgresTarget = func(context.Context, string, string) error { return nil }
	t.Cleanup(func() { guardPostgresTarget = previous })
}

// workDirForRun is a work directory the run's parameter loading accepts. It is
// read-only for these tests: the postgres backend never opens the SQLite
// artifact, so no data/state/atlas.db is created.
func workDirForRun(t *testing.T) string {
	t.Helper()
	abs, err := reporoot.FromCwd()
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	return abs
}

// TestRunDailyUpdateClosesTheDerivedPool covers the dual-write site
// (resolvePGDest): the run opens a pool for its PostgreSQL destination and the
// ledger injection must be exactly as it was before the call.
func TestRunDailyUpdateClosesTheDerivedPool(t *testing.T) {
	injected := testdb.UnconnectedPool(t, unreachableDSN)
	ledger.SetPostgresPool(injected)
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })

	opened := recordingOpener(t)
	stubTargetGuard(t)

	// The run cannot get past the idempotency COUNT without a server: the error
	// is expected and is exactly the return path the cleanup must survive.
	res, err := RunDailyUpdate(context.Background(), RunDailyOptions{
		WorkDir:     workDirForRun(t),
		Backend:     "postgres",
		ExpectDB:    "atlas_none",
		DualWrite:   true,
		Idempotency: IdempotencyDay,
		AsOf:        DateOnlyUTC(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)),
	})
	if err == nil {
		t.Fatalf("RunDailyUpdate against an unreachable database = nil error, want a failure (res=%+v)", res)
	}
	if len(*opened) == 0 {
		t.Fatal("no pool was opened; the test no longer exercises the derived-pool path")
	}
	for i, pool := range *opened {
		if !poolIsClosed(t, pool) {
			t.Errorf("pool %d opened by the run is still open after it returned (FU-20260930-12)", i)
		}
	}
	if got := ledger.PostgresPool(); got != injected {
		t.Errorf("ledger injection after the run = %p, want the pre-run pool %p", got, injected)
	}
}

// TestRunDailyUpdateClosesThePanelPool covers the second open site: a dry run
// opens no destination pool, so the panel build is the only place a pool can
// come from — and it used to leak unconditionally.
func TestRunDailyUpdateClosesThePanelPool(t *testing.T) {
	injected := testdb.UnconnectedPool(t, unreachableDSN)
	ledger.SetPostgresPool(injected)
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })

	opened := recordingOpener(t)
	stubTargetGuard(t)

	res, err := RunDailyUpdate(context.Background(), RunDailyOptions{
		WorkDir:   workDirForRun(t),
		Backend:   "postgres",
		ExpectDB:  "atlas_none",
		DualWrite: true,
		DryRun:    true,
		AsOf:      DateOnlyUTC(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)),
	})
	if err == nil {
		t.Fatalf("dry run against an unreachable database = nil error, want a failure from the panel build (res=%+v)", res)
	}
	if len(*opened) == 0 {
		t.Fatal("no pool was opened; the test no longer exercises the panel-pool path")
	}
	for i, pool := range *opened {
		if !poolIsClosed(t, pool) {
			t.Errorf("pool %d opened by the dry run is still open after it returned (FU-20260930-12)", i)
		}
	}
	if got := ledger.PostgresPool(); got != injected {
		t.Errorf("ledger injection after the dry run = %p, want the pre-run pool %p", got, injected)
	}
}

// TestPoolOwnershipReleasesWhatItAdopted pins the unit itself: every adopted
// pool is closed, and a run that opened nothing still restores the injection.
func TestPoolOwnershipReleasesWhatItAdopted(t *testing.T) {
	injected := testdb.UnconnectedPool(t, unreachableDSN)
	ledger.SetPostgresPool(injected)
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })

	first := testdb.UnconnectedPool(t, unreachableDSN)
	second := testdb.UnconnectedPool(t, unreachableDSN)
	owner := newPoolOwnership()
	owner.adopt(nil) // never panics, never adopts
	owner.adopt(first)
	owner.adopt(second)

	for name, pool := range map[string]*pgxpool.Pool{"first": first, "second": second} {
		if poolIsClosed(t, pool) {
			t.Fatalf("%s pool was closed before release()", name)
		}
	}

	owner.release()
	for name, pool := range map[string]*pgxpool.Pool{"first": first, "second": second} {
		if !poolIsClosed(t, pool) {
			t.Errorf("%s adopted pool is still open after release()", name)
		}
	}
	if got := ledger.PostgresPool(); got != injected {
		t.Errorf("injection after release() = %p, want %p", got, injected)
	}

	// A second release must stay safe (Close is idempotent, the slice is
	// cleared) — nothing here may panic or re-close the wrong pool.
	owner.release()
	if got := ledger.PostgresPool(); got != injected {
		t.Errorf("injection after a second release() = %p, want %p", got, injected)
	}
}

// TestPoolOwnershipRestoresNilInjection: a caller that had nothing injected
// (the CLI) must end up with nothing injected, not with a dead pool.
func TestPoolOwnershipRestoresNilInjection(t *testing.T) {
	ledger.SetPostgresPool(nil)

	pool := testdb.UnconnectedPool(t, unreachableDSN)
	owner := newPoolOwnership()
	owner.adopt(pool)
	pool.Close() // pgxpool.Close is idempotent; release must stay safe after it
	owner.release()
	if got := ledger.PostgresPool(); got != nil {
		t.Errorf("injection after release() = %p, want nil", got)
	}
}
