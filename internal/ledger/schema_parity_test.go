package ledger

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Schema parity guard (2026-10-12).
//
// Why: the same logical table lives in TWO schemas — PostgreSQL via
// sql/migrations/*.up.sql, SQLite via the CREATE TABLE IF NOT EXISTS block plus
// the additiveMigrations list in sqlite_core.go. Adding a column to only one of
// them is silent until production reads it: #2213 added break_even_cost_rate to
// the PG migration and to the Go struct, the SQLite file never got the column,
// and /api/stock/win_rate answered HTTP 503 ("no such column") for every request
// until #2218. CREATE TABLE IF NOT EXISTS cannot heal that — only ALTER can.
//
// This test derives the SQLite side EMPIRICALLY (InitSchema on an in-memory DB
// + PRAGMA table_info) instead of parsing Go, and the PG side from the
// migration text, then requires the shared tables to have identical column
// sets. Documented, deliberate differences live in schemaParityAllowlist with a
// reason each; anything else fails.
var (
	reCreateTable = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)\s*\((.*?)\n\s*\);`)
	reAddColumn   = regexp.MustCompile(`(?is)ALTER\s+TABLE\s+([a-z_][a-z0-9_]*)\s+ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)`)
	reSQLiteTable = regexp.MustCompile(`^([a-z_][a-z0-9_]*)$`)
)

// parityAllowlist records the differences that exist TODAY and are not being
// fixed in this change. Each entry needs a reason; the guard still fails for
// anything not listed, and it LOGS every allowlisted divergence so they stay
// visible instead of being silently blessed.
//
// Reconciliation target: the three divergent shared tables must converge when
// the Q2 "single PostgreSQL" migration runs (see FU-20261012-09).
var parityAllowlist = map[string]string{
	// PostgreSQL-only tables. They belong to other subsystems' stores (auth,
	// monitoring metrics, task liveness, the recommendation/advice chain); the
	// SQLite ledger schema never implemented them, so there is no dual-schema
	// risk to guard here — a column change reaches only one backend by design.
	"alerts":                  pgOnlyReason,
	"atlas_mcp_tokens":        pgOnlyReason,
	"baseline_history":        pgOnlyReason,
	"capital_flow":            pgOnlyReason,
	"channel_health":          pgOnlyReason,
	"channel_secrets":         pgOnlyReason,
	"experiment_lineage":      pgOnlyReason,
	"export_statistics":       pgOnlyReason,
	"metric_trends":           pgOnlyReason,
	"metrics":                 pgOnlyReason,
	"otel_traces":             pgOnlyReason,
	"recommendation_outcomes": pgOnlyReason,
	"symbol_industry":         pgOnlyReason,
	"task_execution_events":   pgOnlyReason,
	"task_executions":         pgOnlyReason,
	"task_liveness":           pgOnlyReason,
	"users":                   pgOnlyReason,
	"workspaces":              pgOnlyReason,

	// Shared table names whose column sets genuinely differ (measured
	// 2026-10-12: human_interventions 10 PG-only / 4 SQLite-only,
	// screening_rejects 8 / 3, session_summaries 14 / 3 ⇒ 44 columns total).
	// The PG shape is the newer one (session_summaries carries approval_id,
	// tax_snapshots, guard_outcomes…), while the SQLite shape still carries the
	// legacy names (timestamp/agent_id/symbol/action). Nothing reads the SQLite
	// shape through a PG-shaped query yet — that is exactly what would 503 next.
	"human_interventions": divergentLegacyReason,
	"screening_rejects":   divergentLegacyReason,
	"session_summaries":   divergentLegacyReason,
}

const (
	pgOnlyReason          = "PostgreSQL-only table managed by another subsystem's store; the SQLite ledger schema never had it"
	divergentLegacyReason = "known legacy schema divergence for the same table name (measured 2026-10-12, tracked as FU-20261012-09)"
)

// pgExpectedColumns parses the migration set into table -> column names.
func pgExpectedColumns(t *testing.T) map[string]map[string]bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "sql", "migrations", "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("glob migrations: %v (found %d)", err, len(files))
	}
	sort.Strings(files)
	out := map[string]map[string]bool{}
	for _, f := range files {
		blob, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		text := string(blob)
		for _, m := range reCreateTable.FindAllStringSubmatch(text, -1) {
			table, body := strings.ToLower(m[1]), m[2]
			if out[table] == nil {
				out[table] = map[string]bool{}
			}
			for _, line := range strings.Split(body, "\n") {
				s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ","))
				if s == "" || strings.HasPrefix(s, "--") {
					continue
				}
				up := strings.ToUpper(s)
				if strings.HasPrefix(up, "CONSTRAINT") || strings.HasPrefix(up, "PRIMARY KEY") ||
					strings.HasPrefix(up, "UNIQUE") || strings.HasPrefix(up, "CHECK") ||
					strings.HasPrefix(up, "FOREIGN KEY") {
					continue
				}
				field := strings.Fields(s)
				if len(field) == 0 {
					continue
				}
				out[table][strings.ToLower(strings.Trim(field[0], `"`))] = true
			}
		}
		for _, m := range reAddColumn.FindAllStringSubmatch(text, -1) {
			table, col := strings.ToLower(m[1]), strings.ToLower(m[2])
			if out[table] == nil {
				out[table] = map[string]bool{}
			}
			out[table][col] = true
		}
	}
	return out
}

// sqliteEffectiveColumns returns the column set each SQLite table ACTUALLY has
// after InitSchema (including the additive migrations).
func sqliteEffectiveColumns(t *testing.T) map[string]map[string]bool {
	t.Helper()
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatalf("list sqlite tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		if reSQLiteTable.MatchString(n) {
			tables = append(tables, n)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}
	_ = rows.Close()

	out := map[string]map[string]bool{}
	for _, tbl := range tables {
		info, err := db.Query("PRAGMA table_info(" + tbl + ")")
		if err != nil {
			t.Fatalf("pragma %s: %v", tbl, err)
		}
		cols := map[string]bool{}
		for info.Next() {
			var cid, notnull, pk int
			var name, ctype string
			var dflt any
			if err := info.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				t.Fatalf("scan pragma %s: %v", tbl, err)
			}
			cols[strings.ToLower(name)] = true
		}
		if err := info.Err(); err != nil {
			t.Fatalf("iterate pragma %s: %v", tbl, err)
		}
		_ = info.Close()
		out[tbl] = cols
	}
	return out
}

// TestSchemaParity_PGMigrationsVsSQLite is the guard for the whole P0 class.
func TestSchemaParity_PGMigrationsVsSQLite(t *testing.T) {
	pg := pgExpectedColumns(t)
	lite := sqliteEffectiveColumns(t)

	allowedTable := func(table string) (string, bool) { r, ok := parityAllowlist[table]; return r, ok }

	var problems, noted []string
	for table, pgCols := range pg {
		liteCols, shared := lite[table]
		if !shared {
			if reason, ok := allowedTable(table); ok {
				noted = append(noted, "allowlisted PG-only table "+table+" ("+reason+")")
				continue
			}
			problems = append(problems,
				"table "+table+" exists in PG migrations but NOT in SQLite (add a CREATE TABLE to sqlite_core.go or allowlist it with a reason)")
			continue
		}

		var pgOnly, liteOnly []string
		for col := range pgCols {
			if !liteCols[col] {
				pgOnly = append(pgOnly, col)
			}
		}
		for col := range liteCols {
			if !pgCols[col] {
				liteOnly = append(liteOnly, col)
			}
		}
		if len(pgOnly) == 0 && len(liteOnly) == 0 {
			continue
		}
		sort.Strings(pgOnly)
		sort.Strings(liteOnly)
		if reason, ok := allowedTable(table); ok {
			noted = append(noted, fmt.Sprintf(
				"allowlisted divergence %s: %d PG-only %v / %d SQLite-only %v (%s)",
				table, len(pgOnly), pgOnly, len(liteOnly), liteOnly, reason))
			continue
		}
		for _, col := range pgOnly {
			problems = append(problems, "column "+table+"."+col+" is in PG migrations but MISSING in SQLite (add it to the CREATE TABLE + the additiveMigrations list, or allowlist it)")
		}
		for _, col := range liteOnly {
			problems = append(problems, "column "+table+"."+col+" exists in SQLite but NOT in PG migrations (add a migration, or allowlist it)")
		}
	}

	for _, n := range noted {
		t.Log(n)
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("schema drift between PG migrations and SQLite (%d finding(s)):\n  - %s\n\nThis is the 2026-10-12 production 503 class: a column added to one backend only. Fix the schema (both sides) or record a deliberate difference in schemaParityAllowlist with a reason.",
			len(problems), strings.Join(problems, "\n  - "))
	}
}
