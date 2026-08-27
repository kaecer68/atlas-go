#!/usr/bin/env bash
#
# check-migration-sql.sh
#
# Static gate for sql/migrations/*.up.sql. Catches two classes of DDL that
# pass on SQLite but break (or silently lose precision) on PostgreSQL:
#
#   1. Unquoted PostgreSQL reserved words used as column names
#      (window / user / order / select / ...). E.g. a `window TEXT` column
#      fails on PG with SQLSTATE 42601 while SQLite accepts it.
#   2. Bare REAL column types. SQLite REAL is an 8-byte double; PostgreSQL
#      REAL is 4-byte float4 with precision loss. Float columns must use
#      DOUBLE PRECISION.
#
# Exit 0 = clean; Exit 1 = at least one violation.
#
# The check needs no database service and must never be skipped in CI.
# It parses CREATE TABLE column definitions and ALTER TABLE ... ADD COLUMN
# statements; quoted identifiers ("window") are treated as intentional and
# pass.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

cd "$REPO_ROOT"

python3 - <<'PYEOF'
import re
import sys
from pathlib import Path

# PostgreSQL reserved keywords that are commonly (and incorrectly) used as
# column names. Detection is position-specific (column definitions only), so
# legitimate keyword usage such as `SELECT ...`, `WHERE ...`, or `ORDER BY`
# is not flagged.
RESERVED = frozenset({
    "all", "analyse", "analyze", "and", "any", "array", "as", "asc",
    "asymmetric", "authorization", "between", "binary", "both", "case",
    "cast", "check", "collate", "column", "concurrently", "constraint",
    "create", "cross", "current_catalog", "current_date", "current_role",
    "current_schema", "current_time", "current_timestamp", "current_user",
    "default", "deferrable", "desc", "distinct", "do", "else", "end",
    "except", "false", "fetch", "for", "foreign", "freeze", "from", "full",
    "grant", "group", "having", "ilike", "in", "initially", "inner",
    "intersect", "into", "is", "isnull", "join", "lateral", "leading",
    "left", "like", "limit", "localtime", "localtimestamp", "natural",
    "not", "notnull", "null", "offset", "on", "only", "or", "order",
    "outer", "overlaps", "placing", "primary", "references", "returning",
    "right", "select", "session_user", "similar", "some", "symmetric",
    "table", "tablesample", "then", "trailing", "true", "union", "unique",
    "user", "using", "variadic", "verbose", "when", "where", "window",
    "with",
})

# Column-definition fragments that are table-level constraints, not columns.
# Their first token would otherwise look like a column name.
CONSTRAINT_STARTS = frozenset({
    "primary", "unique", "foreign", "check", "constraint", "exclude",
})

# Historical migrations shipped before the REAL gate. They are immutable
# (never edited after shipping), so their pre-existing bare REAL columns are
# grandfathered. Any NEW migration must use DOUBLE PRECISION for float data.
REAL_GRANDFATHERED = frozenset({
    "000002_create_timescale_tables.up.sql",
})

MIGRATION_DIR = Path("sql/migrations")


def find_matching_paren(text, open_idx):
    depth = 0
    for i in range(open_idx, len(text)):
        if text[i] == "(":
            depth += 1
        elif text[i] == ")":
            depth -= 1
            if depth == 0:
                return i
    return -1


def split_top_level(body):
    """Split a CREATE TABLE column list on top-level commas."""
    parts = []
    depth = 0
    cur = []
    for ch in body:
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
        if ch == "," and depth == 0:
            parts.append("".join(cur))
            cur = []
        else:
            cur.append(ch)
    parts.append("".join(cur))
    return parts


def create_table_columns(sql):
    """Yield column-definition fragments from CREATE TABLE statements."""
    pattern = re.compile(
        r"\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?[A-Za-z0-9_.\"]+\s*\(",
        re.IGNORECASE,
    )
    for m in pattern.finditer(sql):
        open_idx = m.end() - 1  # index of the '(' after the table name
        close_idx = find_matching_paren(sql, open_idx)
        if close_idx < 0:
            continue
        body = sql[m.end():close_idx]
        yield from split_top_level(body)


def add_columns(sql):
    """Yield (name, type_token) from ALTER TABLE ... ADD COLUMN statements."""
    pattern = re.compile(
        r"\bALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?[A-Za-z0-9_.\"]+\s+"
        r"ADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\s+"
        r"([A-Za-z0-9_\"]+)\s+([A-Za-z_][A-Za-z0-9_]*)",
        re.IGNORECASE,
    )
    for m in pattern.finditer(sql):
        yield m.group(1), m.group(2).upper()


def leading_identifier(seg):
    seg = seg.strip()
    if not seg or seg[0] == '"':
        return None  # quoted identifier: intentional and safe
    m = re.match(r"([A-Za-z_][A-Za-z0-9_]*)", seg)
    return m.group(1) if m else None


def column_type(seg, name):
    """Return the first type token after the column name, or None."""
    rest = seg.strip()[len(name):]
    m = re.match(r"\s*([A-Za-z_][A-Za-z0-9_]*)", rest)
    return m.group(1).upper() if m else None


def scan_file(path):
    violations = []
    sql = path.read_text()
    rel = path.name

    for seg in create_table_columns(sql):
        name = leading_identifier(seg)
        if not name:
            continue
        low = name.lower()
        if low in CONSTRAINT_STARTS:
            continue
        if low in RESERVED:
            violations.append(f"{rel}: unquoted reserved word used as column name: `{name}`")
        if rel not in REAL_GRANDFATHERED and column_type(seg, name) == "REAL":
            violations.append(
                f"{rel}: column `{name}` uses bare REAL; use DOUBLE PRECISION for float data"
            )

    for name, typ in add_columns(sql):
        if name.startswith('"'):
            continue
        if name.lower() in RESERVED:
            violations.append(
                f"{rel}: unquoted reserved word in ADD COLUMN: `{name}`"
            )
        if rel not in REAL_GRANDFATHERED and typ == "REAL":
            violations.append(
                f"{rel}: ADD COLUMN `{name}` uses bare REAL; use DOUBLE PRECISION for float data"
            )

    return violations


def main():
    files = sorted(MIGRATION_DIR.glob("*.up.sql"))
    if not files:
        print(f"error: no migration files found under {MIGRATION_DIR}", file=sys.stderr)
        return 1

    all_violations = []
    for path in files:
        all_violations.extend(scan_file(path))

    if all_violations:
        print("Migration SQL gate violations:", file=sys.stderr)
        for v in all_violations:
            print(f"  - {v}", file=sys.stderr)
        return 1

    print(f"Migration SQL gate passed ({len(files)} files)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
PYEOF
