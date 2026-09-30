package stockpicker

// resolvePGDest unit tests: destination resolution must never attempt a PG
// connection except on the derived path (kill switch off / non-postgres
// backend / dry-run all yield SQLite-only), and an injected handle wins.

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestResolvePGDest_NonPostgresNeverTouchesPG(t *testing.T) {
	cases := []struct {
		name    string
		backend string
		opts    RunDailyOptions
	}{
		{"sqlite backend ignores DualWrite", "sqlite", RunDailyOptions{DualWrite: true}},
		{"empty backend resolves sqlite", "", RunDailyOptions{DualWrite: true}},
		{"kill switch off on postgres", "postgres", RunDailyOptions{DualWrite: false}},
		{"dry-run derives nothing", "postgres", RunDailyOptions{DualWrite: true, DryRun: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pgDB, pool, err := resolvePGDest(context.Background(), tc.opts, tc.backend, false)
			if err != nil {
				t.Fatalf("must not error (no PG should be contacted): %v", err)
			}
			if pgDB != nil || pool != nil {
				t.Fatalf("want nil destination (SQLite-only), got pgDB=%v pool=%v", pgDB, pool)
			}
		})
	}
}

func TestResolvePGDest_InjectedHandleWins(t *testing.T) {
	injected := &sql.DB{}
	pgDB, pool, err := resolvePGDest(context.Background(),
		RunDailyOptions{PGOutcomesDB: injected}, "sqlite", true)
	if err != nil {
		t.Fatalf("injected handle must resolve without error: %v", err)
	}
	if pgDB != injected {
		t.Fatalf("injected handle must win, got %v", pgDB)
	}
	if pool != nil {
		t.Fatalf("injected path must not open a pool, got %v", pool)
	}
}

func TestResolvePGDest_DerivedPathRejectsInjectedPanel(t *testing.T) {
	_, _, err := resolvePGDest(context.Background(),
		RunDailyOptions{DualWrite: true}, "postgres", true /* panelInjected */)
	if err == nil {
		t.Fatal("derived path with an injected panel must fail loudly")
	}
	if !strings.Contains(err.Error(), "PGOutcomesDB") {
		t.Fatalf("error must name PGOutcomesDB, got: %v", err)
	}
}
