#!/usr/bin/env bash
# tests/scripts/test-check-schema-parity.sh — schema parity 守門的 hermetic 契約測試
#
# 為什麼需要：這個守門的價值全在「同一個表在兩側多／少一個欄位就紅」，而 main 上修好
# 之後 repo 掃描永遠是綠的 ⇒ 只用真 repo 證明不了它有牙齒。本測試以**自帶 fixture**
# （純檔案：migrations/*.up.sql ＋ 一份 SQLite DDL ＋ 一份 allowlist JSON；不建 git
# repo、不連網、不改 repo 任何檔）逐條證明下列契約。
#
# 契約（失敗即 exit 1）：
#   1. 兩側一致 → rc=0，輸出含「shared table」量測行（量具真的看到表）。
#   2. SQLite 少一個「由 multi-ADD-COLUMN 第二個 ADD 宣告」的欄位（b）→ rc=1，訊息指名
#      shared.b。★ regression probe：2026-10-12 版的解析器每個 ALTER TABLE 只抓第一個
#      ADD COLUMN，這條在舊版是**假綠**（真 repo 因此漏掉 session_summaries 的
#      after_tax_pnl／total_tax_paid／parameters_version）。
#   3. 型別不一致（PG INTEGER vs SQLite TEXT）→ rc=1，訊息含型別差異。
#   4. SQLite 獨有的表 → rc=1，訊息含「exists in SQLite」。
#   5. allowlist 有條目但差異已不存在（stale）→ rc=1，訊息含 STALE（allowlist 不許腐爛）。
#   6. PG 獨有的表未登錄 allowlist → rc=1，訊息含「exists in PostgreSQL」。
#   7. migrations 目錄不存在／allowlist 不存在 → rc=2（執行錯誤，不得當成通過）。
#   8. 真 repo（現況）→ rc=0。
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
CHECK="$ROOT/scripts/ci/check_schema_parity.sh"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/schema-parity-contract.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

[ -f "$CHECK" ] || { echo "FAIL: 找不到 $CHECK" >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "FAIL: 需要 go" >&2; exit 1; }

fail() {
    echo "FAIL: $*" >&2
    if [ -f "$TMP/out.txt" ]; then
        echo "--- 最後一次檢查輸出 ---" >&2
        cat "$TMP/out.txt" >&2
    fi
    exit 1
}

RC=0
run_check() {  # <migrations-dir> <sqlite-ddl> <allowlist>
    RC=0
    ATLAS_SCHEMA_PARITY_MIGRATIONS_DIR="$1" \
    ATLAS_SCHEMA_PARITY_SQLITE_DDL="$2" \
    ATLAS_SCHEMA_PARITY_ALLOWLIST="$3" \
        bash "$CHECK" >"$TMP/out.txt" 2>&1 || RC=$?
}

assert_rc() { [ "$RC" -eq "$1" ] || fail "rc=${RC}，預期 $1"; }
assert_contains() { grep -Fq -- "$1" "$TMP/out.txt" || fail "輸出不含: $1"; }

# ---------------------------------------------------------------------------
# fixture：一份 migration（含 multi-ADD-COLUMN）＋ 對應的 SQLite DDL ＋ allowlist
# ---------------------------------------------------------------------------
MIG="$TMP/migrations"
mkdir -p "$MIG"
printf '%s\n' \
    'CREATE TABLE shared (' \
    '    id uuid PRIMARY KEY,' \
    '    name text NOT NULL' \
    ');' \
    '' \
    '-- 一個 ALTER 宣告兩個欄位：舊版解析器只會抓到 a，b 會被靜默漏掉。' \
    'ALTER TABLE shared' \
    '    ADD COLUMN a TEXT,' \
    '    ADD COLUMN b INTEGER;' \
    '' \
    'CREATE TABLE pgonly (' \
    '    id uuid PRIMARY KEY' \
    ');' > "$MIG/000001_base.up.sql"

printf '%s\n' \
    'CREATE TABLE shared (' \
    '    id INTEGER PRIMARY KEY AUTOINCREMENT,' \
    '    name TEXT NOT NULL,' \
    '    a TEXT,' \
    '    b INTEGER' \
    ');' > "$TMP/sqlite.sql"

printf '%s\n' \
    '{' \
    '  "note": "fixture",' \
    '  "pg_only_tables": [{"table": "pgonly", "reason": "fixture：PG 專用表"}],' \
    '  "sqlite_only_tables": [],' \
    '  "column_differences": [],' \
    '  "type_differences": [],' \
    '  "nullability_differences": []' \
    '}' > "$TMP/allowlist.json"

# 1. 兩側一致 → 綠，且量測行真的出現。
run_check "$MIG" "$TMP/sqlite.sql" "$TMP/allowlist.json"
assert_rc 0
assert_contains "shared table shared"

# 2. base 的 mutation：SQLite 少了 multi-ADD 的第二個欄位 b → 紅且指名。
printf '%s\n' \
    'CREATE TABLE shared (' \
    '    id INTEGER PRIMARY KEY AUTOINCREMENT,' \
    '    name TEXT NOT NULL,' \
    '    a TEXT' \
    ');' > "$TMP/sqlite-missing-b.sql"
if cmp -s "$TMP/sqlite.sql" "$TMP/sqlite-missing-b.sql"; then
    fail "mutation 沒有生效（sqlite-missing-b.sql 與原檔相同）"
fi
run_check "$MIG" "$TMP/sqlite-missing-b.sql" "$TMP/allowlist.json"
assert_rc 1
assert_contains "shared.b"

# 3. 型別不一致 → 紅且說明兩側型別。
sed 's/^    b INTEGER$/    b TEXT/' "$TMP/sqlite.sql" > "$TMP/sqlite-type.sql"
run_check "$MIG" "$TMP/sqlite-type.sql" "$TMP/allowlist.json"
assert_rc 1
assert_contains "type shared.b"

# 4. SQLite 獨有的表 → 紅。
printf '%s\n' 'CREATE TABLE sqliteonly (' '    id INTEGER PRIMARY KEY' ');' >> "$TMP/sqlite-with-extra-table.sql.tmp"
cat "$TMP/sqlite.sql" "$TMP/sqlite-with-extra-table.sql.tmp" > "$TMP/sqlite-extra-table.sql"
run_check "$MIG" "$TMP/sqlite-extra-table.sql" "$TMP/allowlist.json"
assert_rc 1
assert_contains "exists in SQLite"

# 5. allowlist 腐爛（登錄了不存在的欄位差異）→ 紅。
printf '%s\n' \
    '{' \
    '  "note": "stale fixture",' \
    '  "pg_only_tables": [{"table": "pgonly", "reason": "fixture：PG 專用表"}],' \
    '  "sqlite_only_tables": [],' \
    '  "column_differences": [{"table": "shared", "column": "gone", "backend": "pg", "reason": "已修好但沒刪"}],' \
    '  "type_differences": [],' \
    '  "nullability_differences": []' \
    '}' > "$TMP/allowlist-stale.json"
run_check "$MIG" "$TMP/sqlite.sql" "$TMP/allowlist-stale.json"
assert_rc 1
assert_contains "STALE"

# 6. PG 獨有的表沒登錄 → 紅（allowlist 空）。
printf '%s\n' '{"note": "empty", "pg_only_tables": [], "sqlite_only_tables": [], "column_differences": [], "type_differences": [], "nullability_differences": []}' > "$TMP/allowlist-empty.json"
run_check "$MIG" "$TMP/sqlite.sql" "$TMP/allowlist-empty.json"
assert_rc 1
assert_contains "exists in PostgreSQL"

# 7. 執行錯誤：migrations 目錄不存在 / allowlist 不存在 → rc=2（不是 0，也不是 1）。
run_check "$TMP/no-such-dir" "$TMP/sqlite.sql" "$TMP/allowlist.json"
assert_rc 2
run_check "$MIG" "$TMP/sqlite.sql" "$TMP/no-such-allowlist.json"
assert_rc 2

# 8. 真 repo → 綠（守門對現況不誤報）。
RC=0
bash "$CHECK" >"$TMP/out.txt" 2>&1 || RC=$?
assert_rc 0

echo "✅ schema parity contract tests passed"
