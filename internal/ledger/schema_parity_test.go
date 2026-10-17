package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Schema parity guard (2026-10-12, hardened 2026-10-16).
//
// Why: the same logical table lives in TWO schemas — PostgreSQL via
// sql/migrations/*.up.sql, SQLite via the CREATE TABLE IF NOT EXISTS block plus
// the additiveMigrations list in sqlite_core.go. Adding a column to only one of
// them is silent until production reads it: #2213 added break_even_cost_rate to
// the PG migration and to the Go struct, the SQLite file never got the column,
// and /api/stock/win_rate answered HTTP 503 ("no such column") for every request
// until #2218. CREATE TABLE IF NOT EXISTS cannot heal that — only ALTER can.
//
// What the guard compares (per table, both directions):
//
//  1. column SET — a column on one side only;
//  2. column TYPE — normalised to a storage family, with the cross-backend
//     equivalences SQLite cannot express (uuid↔INTEGER, jsonb↔TEXT,
//     timestamptz↔TEXT, …) declared in typeFamiliesCompatible;
//  3. column nullability — PG NOT NULL vs SQLite nullable (a backend that
//     accepts rows the other rejects).
//
// The SQLite side is derived EMPIRICALLY from InitSchema on an in-memory
// database (that also runs every additive migration, including the ones built
// from loop variables) plus the `ALTER TABLE … ADD COLUMN` statements that the
// stores execute lazily outside InitSchema. The PG side is parsed from the
// migration text.
//
// Deliberate differences live in scripts/ci/schema-parity-allowlist.json, one
// reviewed entry each with a reason; the guard also fails when an allowlist
// entry is STALE (no longer observed), so the file cannot rot into permanent
// silence. FU-20261012-07 (dual definition) / FU-20261012-09 (divergence).
//
// Env overrides exist so tests/scripts/test-check-schema-parity.sh can prove
// the guard is red on broken input without touching the repository:
//
//	ATLAS_SCHEMA_PARITY_MIGRATIONS_DIR  directory of *.up.sql (default ../../sql/migrations)
//	ATLAS_SCHEMA_PARITY_SQLITE_DDL      file with SQLite DDL to execute instead of InitSchema
//	ATLAS_SCHEMA_PARITY_ALLOWLIST       allowlist JSON (default ../../scripts/ci/schema-parity-allowlist.json)
const (
	envMigrationsDir = "ATLAS_SCHEMA_PARITY_MIGRATIONS_DIR"
	envSQLiteDDL     = "ATLAS_SCHEMA_PARITY_SQLITE_DDL"
	envAllowlistPath = "ATLAS_SCHEMA_PARITY_ALLOWLIST"

	defaultMigrationsDir = "../../sql/migrations"
	defaultAllowlistPath = "../../scripts/ci/schema-parity-allowlist.json"
)

// columnDef is one column of one table on one backend.
type columnDef struct {
	Type    string // storage type, normalised by typeFamily for comparison
	NotNull bool
}

// schema is table -> column -> definition.
type schema map[string]map[string]columnDef

// schemaParityAllowlist mirrors scripts/ci/schema-parity-allowlist.json.
type schemaParityAllowlist struct {
	Note              string        `json:"note"`
	PGOnlyTables      []tableReason `json:"pg_only_tables"`
	SQLiteOnlyTables  []tableReason `json:"sqlite_only_tables"`
	ColumnDifferences []columnDiff  `json:"column_differences"`
	TypeDifferences   []columnDiff  `json:"type_differences"`
	NullDifferences   []columnDiff  `json:"nullability_differences"`
}

type tableReason struct {
	Table  string `json:"table"`
	Reason string `json:"reason"`
}

// columnDiff identifies one allowlisted difference. Backend is "pg" or "sqlite"
// and says which side carries the column (column_differences) or which side
// declares the wider constraint (type/nullability differences).
type columnDiff struct {
	Table   string `json:"table"`
	Column  string `json:"column"`
	Backend string `json:"backend"`
	PGType  string `json:"pg_type,omitempty"`
	SQLite  string `json:"sqlite_type,omitempty"`
	Reason  string `json:"reason"`
}

func (c columnDiff) key() string {
	return c.Table + "." + c.Column + "#" + c.Backend
}

var (
	reCreateTable = regexp.MustCompile(`(?is)^CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)\s*\((.*)\)\s*$`)
	reAlterTable  = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:ONLY\s+)?([a-z_][a-z0-9_]*)\s+(.*)$`)
	reAddColumn   = regexp.MustCompile(`(?is)^ADD\s+(?:COLUMN\s+)?(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)\s+(.*)$`)
	reGoAlter     = regexp.MustCompile(`ALTER\s+TABLE\s+([a-z_][a-z0-9_]*)\s+ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_ ()]*)`)
	reSQLiteName  = regexp.MustCompile(`^([a-z_][a-z0-9_]*)$`)
)

// splitSQLStatements splits a migration file into statements on ';' outside
// single quotes, parens and comments. It is deliberately small: the migration
// set uses no dollar quoting and no procedural blocks (asserted by
// TestSchemaParity_ParserHandlesRepoMigrations).
func splitSQLStatements(text string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	depth := 0
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if inQuote {
			cur.WriteByte(ch)
			if ch == '\'' {
				inQuote = false
			}
			continue
		}
		switch ch {
		case '-':
			if i+1 < len(text) && text[i+1] == '-' { // line comment
				for i < len(text) && text[i] != '\n' {
					i++
				}
				cur.WriteByte('\n')
				continue
			}
			cur.WriteByte(ch)
		case '\'':
			inQuote = true
			cur.WriteByte(ch)
		case '(':
			depth++
			cur.WriteByte(ch)
		case ')':
			depth--
			cur.WriteByte(ch)
		case ';':
			if depth <= 0 {
				if s := strings.TrimSpace(cur.String()); s != "" {
					out = append(out, s)
				}
				cur.Reset()
				depth = 0
				continue
			}
			cur.WriteByte(ch)
		default:
			cur.WriteByte(ch)
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// splitTopLevel splits a CREATE TABLE body on commas that are not nested in
// parens or quotes.
func splitTopLevel(body string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	depth := 0
	for i := 0; i < len(body); i++ {
		ch := body[i]
		if inQuote {
			cur.WriteByte(ch)
			if ch == '\'' {
				inQuote = false
			}
			continue
		}
		switch ch {
		case '\'':
			inQuote = true
			cur.WriteByte(ch)
		case '(':
			depth++
			cur.WriteByte(ch)
		case ')':
			depth--
			cur.WriteByte(ch)
		case ',':
			if depth == 0 {
				out = append(out, cur.String())
				cur.Reset()
				continue
			}
			cur.WriteByte(ch)
		default:
			cur.WriteByte(ch)
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// constraintKeywords terminate the type part of a column definition.
var constraintKeywords = []string{
	"NOT NULL", "NULL", "DEFAULT", "UNIQUE", "PRIMARY KEY", "REFERENCES",
	"CHECK", "GENERATED", "COLLATE", "CONSTRAINT", "DEFERRABLE",
}

// parseColumnDef parses "<name> <type> [constraints]" into a columnDef.
// It returns ok=false for table-level constraints.
func parseColumnDef(def string) (name string, col columnDef, ok bool) {
	s := strings.TrimSpace(def)
	if s == "" {
		return "", columnDef{}, false
	}
	lower := strings.ToLower(s)
	for _, prefix := range []string{"constraint", "primary key", "unique", "check", "foreign key", "exclude", "like"} {
		if strings.HasPrefix(lower, prefix) {
			return "", columnDef{}, false
		}
	}
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return "", columnDef{}, false
	}
	name = strings.ToLower(strings.Trim(fields[0], `"`))
	rest := strings.TrimSpace(strings.TrimPrefix(s, fields[0]))
	typePart := rest
	notNull := false
	upper := strings.ToUpper(rest)
	for _, kw := range constraintKeywords {
		if idx := strings.Index(upper, kw); idx >= 0 {
			if idx < len(typePart) {
				typePart = rest[:idx]
			}
			if kw == "NOT NULL" || kw == "PRIMARY KEY" {
				notNull = true
			}
			// keep scanning: DEFAULT … might precede NOT NULL
		}
	}
	typePart = strings.Join(strings.Fields(typePart), " ")
	return name, columnDef{Type: typePart, NotNull: notNull}, true
}

// pgSchemaFromMigrations parses table -> column definitions out of the
// PostgreSQL migration set. It handles CREATE TABLE and every ADD COLUMN of an
// ALTER TABLE statement — including the multi-column form
// (`ALTER TABLE t ADD COLUMN a … , ADD COLUMN b …`), whose trailing columns a
// naive one-match-per-statement regex silently drops (that bug hid
// session_summaries.after_tax_pnl / total_tax_paid / parameters_version from
// the 2026-10-12 version of this guard; see FU-20261012-09).
func pgSchemaFromMigrations(t *testing.T, dir string) schema {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("glob %s/*.up.sql: %v (found %d)", dir, err, len(files))
	}
	sort.Strings(files)
	out := schema{}
	for _, f := range files {
		blob, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, stmt := range splitSQLStatements(string(blob)) {
			if m := reCreateTable.FindStringSubmatch(stmt); m != nil {
				table := strings.ToLower(m[1])
				if out[table] == nil {
					out[table] = map[string]columnDef{}
				}
				for _, part := range splitTopLevel(m[2]) {
					name, col, ok := parseColumnDef(part)
					if !ok {
						continue
					}
					out[table][name] = col
				}
				continue
			}
			m := reAlterTable.FindStringSubmatch(stmt)
			if m == nil {
				continue
			}
			table := strings.ToLower(m[1])
			for _, part := range splitTopLevel(m[2]) {
				am := reAddColumn.FindStringSubmatch(strings.TrimSpace(part))
				if am == nil {
					continue
				}
				name := strings.ToLower(am[1])
				colText := strings.TrimSpace(am[2])
				// Re-use parseColumnDef by feeding "<name> <rest>".
				_, col, ok := parseColumnDef(am[1] + " " + colText)
				if !ok {
					continue
				}
				if out[table] == nil {
					out[table] = map[string]columnDef{}
				}
				out[table][name] = col
			}
		}
	}
	return out
}

// sqliteSchemaFromDDL builds the SQLite schema by executing DDL text.
func sqliteSchemaFromDDL(t *testing.T, db *sql.DB, ddl string) {
	t.Helper()
	for _, stmt := range splitSQLStatements(ddl) {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec sqlite DDL %q: %v", truncate(stmt, 120), err)
		}
	}
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// goLazyAlters returns the columns the stores ADD outside InitSchema, e.g.
// `ALTER TABLE session_summaries ADD COLUMN summary_json TEXT` in
// session_store_sqlite.go. Without them the guard would model a schema no
// deployed database has.
func goLazyAlters(t *testing.T, dir string) schema {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s/*.go: %v", dir, err)
	}
	out := schema{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		blob, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, m := range reGoAlter.FindAllStringSubmatch(string(blob), -1) {
			table := strings.ToLower(m[1])
			if out[table] == nil {
				out[table] = map[string]columnDef{}
			}
			if _, exists := out[table][strings.ToLower(m[2])]; exists {
				continue
			}
			out[table][strings.ToLower(m[2])] = columnDef{Type: strings.TrimSpace(m[3])}
		}
	}
	return out
}

// sqliteEffectiveSchema returns the column set each SQLite table ACTUALLY has.
func sqliteEffectiveSchema(t *testing.T) schema {
	t.Helper()
	ddlFile := os.Getenv(envSQLiteDDL)
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	if ddlFile != "" {
		blob, err := os.ReadFile(ddlFile)
		if err != nil {
			t.Fatalf("read %s: %v", envSQLiteDDL, err)
		}
		sqliteSchemaFromDDL(t, db, string(blob))
	} else if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	out := schema{}
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
		// sqlite_sequence (and any other sqlite_* internal table) is created by
		// SQLite itself for AUTOINCREMENT; it is not part of the ledger schema.
		if reSQLiteName.MatchString(n) && !strings.HasPrefix(n, "sqlite_") {
			tables = append(tables, n)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}
	_ = rows.Close()

	for _, tbl := range tables {
		info, err := db.Query("PRAGMA table_info(" + tbl + ")")
		if err != nil {
			t.Fatalf("pragma %s: %v", tbl, err)
		}
		cols := map[string]columnDef{}
		for info.Next() {
			var cid, notnull, pk int
			var name, ctype string
			var dflt any
			if err := info.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				t.Fatalf("scan pragma %s: %v", tbl, err)
			}
			cols[strings.ToLower(name)] = columnDef{Type: ctype, NotNull: notnull == 1 || pk == 1}
		}
		if err := info.Err(); err != nil {
			t.Fatalf("iterate pragma %s: %v", tbl, err)
		}
		_ = info.Close()
		out[tbl] = cols
	}

	// Real mode only: fold in the lazily executed ALTERs.
	if ddlFile == "" {
		for table, cols := range goLazyAlters(t, ".") {
			if out[table] == nil {
				continue // table not modelled by InitSchema — nothing to merge
			}
			for name, col := range cols {
				if _, ok := out[table][name]; !ok {
					out[table][name] = col
				}
			}
		}
	}
	return out
}

// typeFamiliesCompatible lists the cross-backend storage equivalences SQLite
// cannot express exactly. Anything not listed (or identical) is a mismatch.
var typeFamiliesCompatible = map[string][]string{
	"json":      {"text"}, // SQLite has no JSON type; JSON is stored as TEXT
	"timestamp": {"text"}, // SQLite has no date/time type
	"date":      {"text"}, // idem
	"uuid":      {"int"},  // SQLite rows use INTEGER PRIMARY KEY AUTOINCREMENT
	"bool":      {"int"},  // SQLite has no BOOLEAN type
}

// typeFamily normalises a backend type name to a storage family.
func typeFamily(raw string) string {
	s := strings.ToLower(strings.Join(strings.Fields(raw), " "))
	if s == "" {
		return "none"
	}
	switch s {
	case "uuid":
		return "uuid"
	case "int", "int2", "int4", "int8", "integer", "bigint", "smallint", "tinyint", "mediumint",
		"serial", "bigserial", "smallserial", "unsigned big int":
		return "int"
	case "text", "varchar", "character varying", "char", "character", "nvarchar", "nchar",
		"clob", "citext", "varying character", "native character":
		return "text"
	case "json", "jsonb":
		return "json"
	case "timestamp", "timestamptz", "timestamp with time zone", "timestamp without time zone",
		"timetz", "time", "time with time zone", "time without time zone", "datetime":
		return "timestamp"
	case "date":
		return "date"
	case "real", "double", "double precision", "float", "float4", "float8", "numeric", "decimal", "money":
		return "float"
	case "boolean", "bool":
		return "bool"
	case "bytea", "blob":
		return "blob"
	}
	return s
}

// typesCompatible reports whether the PG and SQLite types are the same storage
// family, allowing only the documented cross-backend equivalences.
func typesCompatible(pgType, sqliteType string) bool {
	p, s := typeFamily(pgType), typeFamily(sqliteType)
	if p == s {
		return true
	}
	for _, alt := range typeFamiliesCompatible[p] {
		if alt == s {
			return true
		}
	}
	return false
}

func loadAllowlist(t *testing.T) (*schemaParityAllowlist, string) {
	t.Helper()
	path := os.Getenv(envAllowlistPath)
	if path == "" {
		path = defaultAllowlistPath
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read allowlist %s: %v", path, err)
	}
	var al schemaParityAllowlist
	if err := json.Unmarshal(blob, &al); err != nil {
		t.Fatalf("parse allowlist %s: %v", path, err)
	}
	return &al, path
}

// finding is one observed divergence.
type finding struct {
	class   string // "pg_only_table" | "sqlite_only_table" | "column" | "type" | "nullability"
	table   string
	column  string
	backend string // "pg" | "sqlite"
	detail  string
}

func (f finding) String() string {
	switch f.class {
	case "pg_only_table":
		return "table " + f.table + ": exists in PostgreSQL migrations but NOT in SQLite"
	case "sqlite_only_table":
		return "table " + f.table + ": exists in SQLite but NOT in the PostgreSQL migrations"
	}
	if f.class == "column" {
		side := "PostgreSQL"
		if f.backend == "sqlite" {
			side = "SQLite"
		}
		return "column " + f.table + "." + f.column + ": exists only in " + side
	}
	return f.class + " " + f.table + "." + f.column + ": " + f.detail
}

// TestSchemaParity_PGMigrationsVsSQLite is the guard for the whole P0 class.
func TestSchemaParity_PGMigrationsVsSQLite(t *testing.T) {
	migDir := os.Getenv(envMigrationsDir)
	if migDir == "" {
		migDir = defaultMigrationsDir
	}
	pg := pgSchemaFromMigrations(t, migDir)
	lite := sqliteEffectiveSchema(t)
	al, alPath := loadAllowlist(t)

	pgOnlyTable := map[string]string{}
	for _, e := range al.PGOnlyTables {
		if strings.TrimSpace(e.Reason) == "" {
			t.Fatalf("allowlist %s: pg_only_tables entry %q has no reason", alPath, e.Table)
		}
		pgOnlyTable[e.Table] = e.Reason
	}
	liteOnlyTable := map[string]string{}
	for _, e := range al.SQLiteOnlyTables {
		if strings.TrimSpace(e.Reason) == "" {
			t.Fatalf("allowlist %s: sqlite_only_tables entry %q has no reason", alPath, e.Table)
		}
		liteOnlyTable[e.Table] = e.Reason
	}
	cols := map[string]columnDiff{}
	for _, e := range al.ColumnDifferences {
		if strings.TrimSpace(e.Reason) == "" || (e.Backend != "pg" && e.Backend != "sqlite") {
			t.Fatalf("allowlist %s: column_differences entry %q needs a reason and backend pg|sqlite", alPath, e.key())
		}
		cols[e.key()] = e
	}
	types := map[string]columnDiff{}
	for _, e := range al.TypeDifferences {
		if strings.TrimSpace(e.Reason) == "" {
			t.Fatalf("allowlist %s: type_differences entry %q has no reason", alPath, e.key())
		}
		types[e.key()] = e
	}
	nulls := map[string]columnDiff{}
	for _, e := range al.NullDifferences {
		if strings.TrimSpace(e.Reason) == "" {
			t.Fatalf("allowlist %s: nullability_differences entry %q has no reason", alPath, e.key())
		}
		nulls[e.key()] = e
	}

	var findings []finding
	usedPGTable := map[string]bool{}
	usedLiteTable := map[string]bool{}
	used := map[string]bool{}

	tableNames := map[string]bool{}
	for _, tbl := range sortedKeys(pg) {
		tableNames[tbl] = true
	}
	for _, tbl := range sortedKeys(lite) {
		tableNames[tbl] = true
	}

	for _, table := range sortedKeys(tableNames) {
		pgCols, inPG := pg[table]
		liteCols, inLite := lite[table]
		switch {
		case inPG && !inLite:
			if _, ok := pgOnlyTable[table]; ok {
				usedPGTable[table] = true
				continue
			}
			findings = append(findings, finding{class: "pg_only_table", table: table})
			continue
		case !inPG && inLite:
			if _, ok := liteOnlyTable[table]; ok {
				usedLiteTable[table] = true
				continue
			}
			findings = append(findings, finding{class: "sqlite_only_table", table: table})
			continue
		}

		for _, column := range sortedKeys(pgCols) {
			pc := pgCols[column]
			lc, ok := liteCols[column]
			if !ok {
				key := table + "." + column + "#pg"
				if e, allowed := cols[key]; allowed && e.Backend == "pg" {
					used[key] = true
					continue
				}
				findings = append(findings, finding{class: "column", table: table, column: column, backend: "pg"})
				continue
			}
			if !typesCompatible(pc.Type, lc.Type) {
				key := table + "." + column + "#pg"
				if _, allowed := types[key]; allowed {
					used[key] = true
					continue
				}
				findings = append(findings, finding{
					class: "type", table: table, column: column,
					detail: fmt.Sprintf("PostgreSQL declares %q (family %s) but SQLite has %q (family %s)",
						strings.TrimSpace(pc.Type), typeFamily(pc.Type), strings.TrimSpace(lc.Type), typeFamily(lc.Type)),
				})
				continue
			}
			if pc.NotNull != lc.NotNull {
				// The backend field says which side carries the stricter
				// constraint, so the allowlist entry is direction-specific.
				key := table + "." + column + "#pg"
				where := "SQLite accepts NULL where PostgreSQL declares NOT NULL"
				if lc.NotNull {
					key = table + "." + column + "#sqlite"
					where = "SQLite declares NOT NULL where PostgreSQL accepts NULL"
				}
				if _, allowed := nulls[key]; allowed {
					used[key] = true
					continue
				}
				findings = append(findings, finding{class: "nullability", table: table, column: column, detail: where})
			}
		}
		for _, column := range sortedKeys(liteCols) {
			if _, ok := pgCols[column]; ok {
				continue
			}
			key := table + "." + column + "#sqlite"
			if e, allowed := cols[key]; allowed && e.Backend == "sqlite" {
				used[key] = true
				continue
			}
			findings = append(findings, finding{class: "column", table: table, column: column, backend: "sqlite"})
		}
	}

	// Stale allowlist entries (the difference no longer exists) are failures:
	// an allowlist that outlives its reason turns real drift back into silence.
	var stale []string
	for table := range pgOnlyTable {
		if !usedPGTable[table] {
			stale = append(stale, "pg_only_tables "+table)
		}
	}
	for table := range liteOnlyTable {
		if !usedLiteTable[table] {
			stale = append(stale, "sqlite_only_tables "+table)
		}
	}
	for key := range cols {
		if !used[key] {
			stale = append(stale, "column_differences "+key)
		}
	}
	for key := range types {
		if !used[key] {
			stale = append(stale, "type_differences "+key)
		}
	}
	for key := range nulls {
		if !used[key] {
			stale = append(stale, "nullability_differences "+key)
		}
	}

	// Report every shared table so the authoritative list can be regenerated.
	common := 0
	for _, table := range sortedKeys(tableNames) {
		_, inPG := pg[table]
		_, inLite := lite[table]
		if !inPG || !inLite {
			continue
		}
		common++
		t.Logf("schema-parity: shared table %s — %d PostgreSQL columns, %d SQLite columns (allowlisted: pg-only table %v, sqlite-only table %v)",
			table, len(pg[table]), len(lite[table]), pgOnlyTable[table] != "" || usedPGTable[table], liteOnlyTable[table] != "" || usedLiteTable[table])
	}
	pgOnlyCount := 0
	for table := range pg {
		if _, ok := lite[table]; !ok {
			pgOnlyCount++
		}
	}
	liteOnlyCount := 0
	for table := range lite {
		if _, ok := pg[table]; !ok {
			liteOnlyCount++
		}
	}
	t.Logf("schema-parity: %d shared tables, %d PostgreSQL-only tables (allowlisted %d), %d SQLite-only tables (allowlisted %d); migrations %s, allowlist %s",
		common, pgOnlyCount, len(pgOnlyTable), liteOnlyCount, len(liteOnlyTable), migDir, alPath)

	if len(stale) > 0 {
		sort.Strings(stale)
		t.Fatalf("schema-parity allowlist is STALE (%d entry/entries no longer observed in %s):\n  - %s\n\nDelete or fix the entry: an allowlist entry that outlives its divergence hides the next real drift.",
			len(stale), alPath, strings.Join(stale, "\n  - "))
	}

	if len(findings) > 0 {
		lines := make([]string, 0, len(findings))
		for _, f := range findings {
			lines = append(lines, f.String())
		}
		sort.Strings(lines)
		t.Fatalf("schema drift between PostgreSQL migrations and SQLite (%d finding(s)):\n  - %s\n\nThis is the 2026-10-12 production 503 class: a column added to one backend only. Fix both sides (migration + sqlite_core.go CREATE TABLE + addSchemaParityColumns) or record a deliberate difference in %s with a reason.",
			len(findings), strings.Join(lines, "\n  - "), alPath)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
