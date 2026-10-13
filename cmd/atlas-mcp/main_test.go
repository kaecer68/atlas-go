package main

import (
	"context"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/cmd/atlas-mcp/server"
)

// TestPackageCompiles is a build-time smoke that the package imports cleanly
// and the public surface (server.Config, server.Run) is reachable.
func TestPackageCompiles(t *testing.T) {
	// Bare type and function references keep the test stable across implementations.
	_ = server.Config{}
	_ = server.Run
	_, cancel := context.WithCancel(context.Background())
	cancel()
}

// TestWireStockpickerWinRatePG pins the C6 wiring contract: the win-rate tools
// get a PostgreSQL handle only when DATABASE_URL is set AND the declared store
// backend normalizes to postgres. Every other combination must leave
// cfg.WinRatePGDB nil, which is what makes the SQLite ledger the read path —
// and therefore must never happen silently: wireStockpickerWinRatePG logs the
// reason on each branch.
func TestWireStockpickerWinRatePG(t *testing.T) {
	const dsn = "postgres://atlas:atlas@127.0.0.1:1/atlas_c6_wiring?sslmode=disable"
	cases := []struct {
		name       string
		pgURL      string
		declared   string
		wantHandle bool
	}{
		{name: "no DATABASE_URL", pgURL: "", declared: "postgres"},
		{name: "declared postgres", pgURL: dsn, declared: "postgres", wantHandle: true},
		{name: "declared sqlite", pgURL: dsn, declared: "sqlite"},
		{name: "declared jsonl", pgURL: dsn, declared: "jsonl"},
		{name: "backend unset (legacy jsonl default)", pgURL: dsn, declared: ""},
		{name: "unknown backend", pgURL: dsn, declared: "mysql"},
		{name: "unparsable DSN", pgURL: "not-a-dsn", declared: "postgres"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := server.Config{}
			wireStockpickerWinRatePG(&cfg, tc.pgURL, tc.declared)
			if tc.wantHandle {
				if cfg.WinRatePGDB == nil {
					t.Fatalf("WinRatePGDB = nil, want a postgres handle for pgURL=%q backend=%q", tc.pgURL, tc.declared)
				}
				t.Cleanup(func() { _ = cfg.WinRatePGDB.Close() })
				// Wiring must not dial: an unreachable PostgreSQL is handled by
				// the per-call probe + SQLite fallback, not by refusing to
				// start the server.
				if err := cfg.WinRatePGDB.PingContext(context.Background()); err == nil {
					t.Fatal("wired handle answered a ping for an unreachable DSN; the test DSN is not actually unreachable")
				}
				return
			}
			if cfg.WinRatePGDB != nil {
				t.Fatalf("WinRatePGDB wired for pgURL=%q backend=%q, want the SQLite read path", tc.pgURL, tc.declared)
			}
		})
	}
}

// TestLoggableBackendValue: the startup log prints ATLAS_STORE_BACKEND, so the
// value is reduced to an enum-shaped token first — an environment variable must
// not be able to write arbitrary text into the log.
func TestLoggableBackendValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{in: "", want: "(unset)"},
		{in: "postgres", want: "postgres"},
		{in: "sqlite", want: "sqlite"},
		{in: "some_backend-2.0", want: "some_backend-2.0"},
		{in: "postgres\nlevel=ERROR forged", want: "(contains characters outside [A-Za-z0-9_.-])"},
		{in: "postgres%0a", want: "(contains characters outside [A-Za-z0-9_.-])"},
		{in: strings.Repeat("a", 33), want: "(too long)"},
	}
	for _, tc := range cases {
		if got := loggableBackendValue(tc.in); got != tc.want {
			t.Errorf("loggableBackendValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
