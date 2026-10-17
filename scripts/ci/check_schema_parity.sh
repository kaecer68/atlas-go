#!/usr/bin/env bash
#
# check_schema_parity.sh — PostgreSQL migrations 與 SQLite schema 的逐欄比對守門
#
# 背景（FU-20261012-07 / FU-20261012-09）：同一張表有**兩份** schema 定義 ——
# PG 在 sql/migrations/*.up.sql，SQLite 在 internal/ledger/sqlite_core.go 的
# `CREATE TABLE IF NOT EXISTS` ＋ additiveMigrations（＋ 各 store 首次使用時的
# 延遲 `ALTER TABLE … ADD COLUMN`）。#2213 只補了 PG 的 break_even_cost_rate，
# SQLite 檔案沒有那個欄，2026-10-12 生產 `/api/stock/win_rate` 直接 503
# （"no such column"）。CREATE TABLE IF NOT EXISTS **不會**替既有資料庫補欄。
#
# 這支腳本是 make ci-gate 的入口，真正的比對引擎在
# internal/ledger/schema_parity_test.go::TestSchemaParity_PGMigrationsVsSQLite
# （SQLite 側以 InitSchema 跑在 in-memory DB 上實測 PRAGMA table_info，PG 側解析
# migration 文字；逐表比對欄位集合、型別家族、可空性；刻意差異登錄在
# scripts/ci/schema-parity-allowlist.json，且 allowlist 過期（stale）同樣紅燈）。
#
# 退出碼：0 = 兩側一致；1 = 有差異（含 allowlist stale）；2 = 執行錯誤
#         （go 不存在／migrations 目錄或 allowlist 檔案不存在 ⇒ fail loud，不給假綠）。
#
# 用法：
#   bash scripts/ci/check_schema_parity.sh
#
# 契約測試：tests/scripts/test-check-schema-parity.sh（hermetic fixtures；含
# multi-ADD-COLUMN 的 regression probe —— 舊版解析器會漏掉該寫法的第二個以後的欄位）。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

MIGRATIONS_DIR="${ATLAS_SCHEMA_PARITY_MIGRATIONS_DIR:-sql/migrations}"
ALLOWLIST="${ATLAS_SCHEMA_PARITY_ALLOWLIST:-scripts/ci/schema-parity-allowlist.json}"

if ! command -v go >/dev/null 2>&1; then
    echo "❌ 需要 go 才能執行 schema parity 檢查" >&2
    exit 2
fi
if [ ! -d "$MIGRATIONS_DIR" ]; then
    echo "❌ 找不到 migrations 目錄：${MIGRATIONS_DIR}（不得因此當成通過）" >&2
    exit 2
fi
if ! compgen -G "$MIGRATIONS_DIR/*.up.sql" >/dev/null; then
    echo "❌ ${MIGRATIONS_DIR} 內沒有任何 *.up.sql（0 份 migration 不是「一致」）" >&2
    exit 2
fi
if [ ! -f "$ALLOWLIST" ]; then
    echo "❌ 找不到 allowlist：${ALLOWLIST}（刻意差異必須逐條登錄並附理由，不得靜默）" >&2
    exit 2
fi

OUT="$(mktemp "${TMPDIR:-/tmp}/schema-parity-check.XXXXXX")"
trap 'rm -f "$OUT"' EXIT

RC=0
# -count=1：不吃快取，否則「改了 schema 卻拿到上次的綠」正是本守門要防的假綠。
go test ./internal/ledger/ -run '^TestSchemaParity_PGMigrationsVsSQLite$' -count=1 -v >"$OUT" 2>&1 || RC=$?

case "$RC" in
0)
    grep -E 'schema-parity:' "$OUT" | sed -E 's/^[[:space:]]*schema_parity_test\.go:[0-9]+: //' || true
    echo "✅ schema parity：PG migrations 與 SQLite 逐表一致（刻意差異見 ${ALLOWLIST}）"
    ;;
1)
    {
        echo "❌ schema parity FAIL：PG 與 SQLite 的 schema 不一致"
        grep -E 'schema-parity:|drift|STALE|^[[:space:]]+- |allowlist' "$OUT" || cat "$OUT"
        echo ""
        echo "修法（兩邊都要）：PG 補 sql/migrations/*.up.sql；SQLite 同時補"
        echo "  internal/ledger/sqlite_core.go 的 CREATE TABLE 定義 **與** addSchemaParityColumns"
        echo "  （既有資料庫只有 ALTER 補得到欄位）。刻意差異則登錄 ${ALLOWLIST} 並寫理由。"
    } >&2
    exit 1
    ;;
*)
    echo "❌ schema parity 執行錯誤（go test rc=${RC}）；輸出如下" >&2
    cat "$OUT" >&2
    exit 2
    ;;
esac
