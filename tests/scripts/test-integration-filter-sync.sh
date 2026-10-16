#!/usr/bin/env bash
# tests/scripts/test-integration-filter-sync.sh — integration 過濾器同步守門的 hermetic 契約測試
#
# 為什麼需要：這個守門的價值全在「能抓到白名單漏列的目錄」＋「不對相依套件式的白名單條目誤報」。
# 只跑 repo 掃描證明不了前者（main 上修好之後就是綠的），因此本測試用**自帶 fixture**
# （純檔案樹：不建 git repo、不連網、不執行 Go、不動 repo 任何檔）證明下列行為。
#
# 契約（失敗即 exit 1）：
#   1. fixture（白名單完整）→ PASS，且輸出「帶 integration tag 的目錄：4」= 量具真的看到東西
#      （4 = 根 integration_test.go ＋ internal/alpha ＋ internal/beta ＋ cmd/tool；
#        `internal/beta/reader_pg_test.go` 檔名不含 integration 但帶 tag ⇒ 必須算進來）
#      同時證明 `.worktrees/**.go`（巢狀 worktree）**不**被計入。
#   2. 白名單漏一個目錄（internal/beta）→ FAIL(1)，訊息含該目錄與建議補的 glob。
#   3. 同一個目錄以 `# integration-filter-exempt:` 登錄理由 → PASS(0)，輸出含該理由。
#   4. 豁免沒寫理由 → FAIL(1)，訊息含「理由必填」。
#   5. 豁免 stale（目錄已無 integration 測試）→ FAIL(1)，訊息含「stale」。
#   6. 豁免與白名單同時涵蓋同一目錄 → FAIL(1)，訊息含「矛盾」。
#   7. 只有 `internal/**` 這種樣式時，**根目錄**的 integration_test.go 未被涵蓋 → FAIL(1)，
#      且建議是 `- 'integration_test.go'`（picomatch 無 matchBase ⇒ 裸樣式只匹配根那一支）。
#   8. 白名單有條目但該目錄沒有任何 integration 測試 → 只報告、**不**紅燈（PASS(0)）。
#   9. 解析不到 `integration:` 白名單 → 執行錯誤(2)（fail-closed，不給假綠）。
#  10. 白名單含本守門看不懂的 glob（`{a,b}`）→ 執行錯誤(2)（寧可紅也不要假綠）。
#  11. `--json` 輸出可被 JSON 解析，且 uncovered 條目帶目錄名。
#  12. `--selftest-glob`（glob 轉譯器自我對拍，預期值以 node+picomatch 實測為準）→ PASS(0)。
#  13. 白名單檔不存在／root 不存在 → 執行錯誤(2)。
#  14. 真 repo 掃描（git 模式；只看已追蹤檔）→ PASS(0)：白名單與實際目錄同步。
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
CHECKER="$ROOT/scripts/ci/check_integration_filter_sync.py"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/integration-filter-test.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

[ -f "$CHECKER" ] || { echo "FAIL: 找不到 $CHECKER" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "FAIL: 需要 python3" >&2; exit 1; }

fail() {
  echo "FAIL: $*" >&2
  if [ -f "$TMP/out.txt" ]; then
    echo "--- 最後一次檢查輸出 ---" >&2
    cat "$TMP/out.txt" >&2
  fi
  exit 1
}

RC=0
run_checker() {  # <args...>
  RC=0
  python3 "$CHECKER" "$@" >"$TMP/out.txt" 2>&1 || RC=$?
}

assert_rc() {  # <expected>
  [ "$RC" -eq "$1" ] || fail "rc=${RC}，預期 $1"
}

assert_contains() {  # <pattern>
  grep -Fq -- "$1" "$TMP/out.txt" || fail "輸出不含: $1"
}

assert_not_contains() {  # <pattern>
  grep -Fq -- "$1" "$TMP/out.txt" && fail "輸出不應含: $1"
  return 0
}

# ---------------------------------------------------------------------------
# fixture 檔案樹（純檔案；沒有 .git ⇒ 走 walk 模式，且 walk 必須排除 .worktrees/）
# ---------------------------------------------------------------------------

FIX="$TMP/fixture"
mkdir -p "$FIX/internal/alpha" "$FIX/internal/beta" "$FIX/internal/gamma" "$FIX/cmd/tool"
mkdir -p "$FIX/.worktrees/other/internal/zzz"
mkdir -p "$FIX/.github/workflows"

printf '%s\n' '//go:build integration' '' 'package atlas' > "$FIX/integration_test.go"
printf '%s\n' '//go:build integration' '' 'package alpha' > "$FIX/internal/alpha/alpha_integration_test.go"
printf '%s\n' '//go:build integration' '' 'package beta' > "$FIX/internal/beta/reader_pg_test.go"
printf '%s\n' '//go:build integration' '' 'package main' > "$FIX/cmd/tool/main_integration_test.go"
printf '%s\n' 'package gamma' '' 'import "testing"' > "$FIX/internal/gamma/plain_test.go"
# 巢狀 worktree（未追蹤）：不得被計入，否則本機永遠紅燈
printf '%s\n' '//go:build integration' '' 'package zzz' > "$FIX/.worktrees/other/internal/zzz/zzz_integration_test.go"
# fixture 檔必須被解析到：白名單區塊以 2 空格縮排的 `  integration:` job 收尾（與真 workflow 同形）
printf '%s\n' 'name: ci-fixture' 'on: [pull_request]' 'jobs:' \
  '  changes:' '    steps:' '    - uses: dorny/paths-filter@v3' '      with:' '        filters: |' \
  '          code:' "            - '**/*.go'" '          integration:' > "$FIX/.github/workflows/ci-cd.yml"
printf '%s\n' '  integration:' '    runs-on: ubuntu-latest' '    steps:' > "$FIX/tail.yml"

FIXWF="$FIX/.github/workflows/ci-cd.yml"

# 以「白名單 body」重建 fixture workflow（body 由 stdin 餵入）
write_wf() {  # <file>  (body on stdin)
  local out="$1"
  {
    printf '%s\n' 'name: ci-fixture' 'on: [pull_request]' 'jobs:' \
      '  changes:' '    steps:' '    - uses: dorny/paths-filter@v3' '      with:' '        filters: |' \
      '          code:' "            - '**/*.go'" '          integration:'
    cat
    printf '%s\n' '' '  integration:' '    runs-on: ubuntu-latest' '    steps:'
  } > "$out"
}

baseline_body() {  # 完整覆蓋 fixture 的 4 個目錄
  cat <<'EOF'
            - 'internal/alpha/**'
            - 'internal/beta/**'
            - 'cmd/tool/**'
            - 'integration_test.go'
EOF
}

echo "① fixture（白名單完整）→ PASS，且確實掃到 4 個目錄、排除 .worktrees/"
baseline_body | write_wf "$FIXWF"
run_checker --root "$FIX" --workflow "$FIXWF"
assert_rc 0
assert_contains "帶 integration tag 的目錄：4"
assert_contains "✅ ."
assert_contains "✅ internal/beta"
assert_not_contains ".worktrees"

echo "② 白名單漏一個目錄（internal/beta）→ FAIL(1) 並點名"
{
  grep -v "internal/beta" <<'EOF'
            - 'internal/alpha/**'
            - 'internal/beta/**'
            - 'cmd/tool/**'
            - 'integration_test.go'
EOF
} | write_wf "$FIXWF"
run_checker --root "$FIX" --workflow "$FIXWF"
assert_rc 1
assert_contains "internal/beta"
assert_contains "- 'internal/beta/**'"
assert_contains "skipping"

echo "③ 該目錄以豁免登錄理由 → PASS(0)，輸出含理由"
{
  grep -v "internal/beta" <<'EOF'
            - 'internal/alpha/**'
            - 'internal/beta/**'
            - 'cmd/tool/**'
            - 'integration_test.go'
EOF
  echo "            # integration-filter-exempt: internal/beta — 由專屬 job 逐 PR 執行（fixture 理由）"
} | write_wf "$FIXWF"
run_checker --root "$FIX" --workflow "$FIXWF"
assert_rc 0
assert_contains "fixture 理由"

echo "④ 豁免沒寫理由 → FAIL(1)"
{
  grep -v "internal/beta" <<'EOF'
            - 'internal/alpha/**'
            - 'internal/beta/**'
            - 'cmd/tool/**'
            - 'integration_test.go'
EOF
  echo "            # integration-filter-exempt: internal/beta —"
} | write_wf "$FIXWF"
run_checker --root "$FIX" --workflow "$FIXWF"
assert_rc 1
assert_contains "理由必填"

echo "⑤ 豁免 stale（internal/gamma 無 integration 測試）→ FAIL(1)"
{
  baseline_body
  echo "            # integration-filter-exempt: internal/gamma — 已無測試（fixture stale）"
} | write_wf "$FIXWF"
run_checker --root "$FIX" --workflow "$FIXWF"
assert_rc 1
assert_contains "stale"

echo "⑥ 豁免與白名單同時涵蓋同一目錄 → FAIL(1)"
{
  baseline_body
  echo "            # integration-filter-exempt: internal/alpha — 同時被 glob 涵蓋（fixture 矛盾）"
} | write_wf "$FIXWF"
run_checker --root "$FIX" --workflow "$FIXWF"
assert_rc 1
assert_contains "矛盾"

echo "⑦ 只有 internal/** 時，根目錄 integration_test.go 未被涵蓋 → FAIL(1)，建議為字面路徑"
cat <<'EOF' | write_wf "$FIXWF"
            - 'internal/**'
            - 'cmd/tool/**'
EOF
run_checker --root "$FIX" --workflow "$FIXWF"
assert_rc 1
assert_contains "（integration_test.go）→ 建議補：- 'integration_test.go'"

echo "⑧ 白名單有條目但該目錄沒有任何 integration 測試 → 只報告、不紅燈"
{
  baseline_body
  echo "            - 'internal/empty/**'"
} | write_wf "$FIXWF"
run_checker --root "$FIX" --workflow "$FIXWF"
assert_rc 0
assert_contains "internal/empty/**"
assert_contains "報告用，不紅燈"

echo "⑨ 解析不到 integration: 白名單 → 執行錯誤(2)"
printf '%s\n' 'name: x' 'on: [push]' 'jobs:' '  build:' '    runs-on: ubuntu-latest' > "$FIX/.github/workflows/no-filter.yml"
run_checker --root "$FIX" --workflow "$FIX/.github/workflows/no-filter.yml"
assert_rc 2
assert_contains "找不到"

echo "⑩ 看不懂的 glob（{a,b}）→ 執行錯誤(2)，不給假綠"
cat <<'EOF' | write_wf "$FIXWF"
            - 'internal/{alpha,beta}/**'
            - 'cmd/tool/**'
            - 'integration_test.go'
EOF
run_checker --root "$FIX" --workflow "$FIXWF"
assert_rc 2
assert_contains "看不懂的 glob"

echo "⑪ --json 可解析，uncovered 帶目錄名"
{
  grep -v "internal/beta" <<'EOF'
            - 'internal/alpha/**'
            - 'internal/beta/**'
            - 'cmd/tool/**'
            - 'integration_test.go'
EOF
} | write_wf "$FIXWF"
run_checker --root "$FIX" --workflow "$FIXWF" --json
assert_rc 1
python3 - "$TMP/out.txt" <<'PY' || fail "--json 內容不符預期"
import json, sys
data = json.load(open(sys.argv[1]))
assert data["ok"] is False, data["ok"]
dirs = [u["dir"] for u in data["uncovered"]]
assert dirs == ["internal/beta"], dirs
PY

echo "⑫ --selftest-glob（glob 轉譯器對拍）→ PASS(0)"
run_checker --selftest-glob
assert_rc 0
assert_contains "10/10 通過"

echo "⑬ workflow 檔不存在 → 執行錯誤(2)"
run_checker --root "$FIX" --workflow "$TMP/does-not-exist.yml"
assert_rc 2
run_checker --root "$TMP/no-such-root"
assert_rc 2

echo "⑭ 真 repo 掃描（git 模式）→ PASS(0)"
run_checker --root "$ROOT"
assert_rc 0
assert_contains "同步"

echo "✅ integration 過濾器同步守門契約測試全部通過（14 組）"
