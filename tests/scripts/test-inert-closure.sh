#!/usr/bin/env bash
# tests/scripts/test-inert-closure.sh — inert 閉環檢查的 hermetic 回歸測試（#1944 建議 2）
#
# 為什麼需要：這個檢查的價值全在「能抓到新增的 inert」且「不對既有正確寫法誤報」。
# 只跑 repo 掃描無法證明前者（main 上沒有故意違規）。因此本測試用**自帶 fixture**（不碰 git、
# 不碰 production、不需要 Go toolchain）證明四個行為：
#   1. fixture（正確寫法 + baseline 覆蓋）→ PASS，且不誤報 wired writer / annotated writer / 由證據推導的 claim
#   2. negative fixture（故意違規、無 baseline）→ FAIL，且四類違規都被抓到（含缺理由的豁免註解）
#   3. 在既有 repo（main）上跑 → PASS（baseline allowlist 生效，歷史遺留不讓檢查紅燈）
#   4. baseline 的 reason 留 TODO → FAIL（理由必填強制生效）
#
# 本測試不建立 git repo、不呼叫 git，因此沒有 #1927 的 fixture 污染風險。
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
CHECKER="$ROOT/scripts/ci/check_inert_closure.py"
FIXTURE="$ROOT/tests/fixtures/inert-closure"
NEGATIVE="$ROOT/tests/fixtures/inert-closure-negative"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/inert-closure-test.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {  # file pattern
  grep -Fq -- "$2" "$1" || fail "$1 應該包含: $2"
}

assert_not_contains() {  # file pattern
  if grep -Fq -- "$2" "$1"; then
    fail "$1 不應該包含: $2"
  fi
}

command -v python3 >/dev/null 2>&1 || fail "需要 python3"

echo "== case 1: fixture（正確寫法 + baseline）應 PASS 且不誤報"
set +e
python3 "$CHECKER" --root "$FIXTURE" >"$TMP/pass.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 0 ] || { cat "$TMP/pass.out"; fail "fixture 應 exit 0，實際 $rc"; }
assert_not_contains "$TMP/pass.out" "SetWiredWriter"
assert_not_contains "$TMP/pass.out" "SetAnnotatedWriter"
assert_not_contains "$TMP/pass.out" "demo.read_flag"
assert_not_contains "$TMP/pass.out" "BuildEvidenceDerived"

echo "== case 2: negative fixture（故意違規、無 baseline）應抓到四類違規"
set +e
python3 "$CHECKER" --root "$NEGATIVE" --baseline "$TMP/no-such-baseline.json" \
  >"$TMP/neg.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 1 ] || { cat "$TMP/neg.out"; fail "negative fixture 應 exit 1，實際 $rc"; }
assert_contains "$TMP/neg.out" "neg.unread_flag"          # config 旗標沒有 reader
assert_contains "$TMP/neg.out" "SetDeadWriter"            # writer 沒有 consumer
assert_contains "$TMP/neg.out" "applied@Applied"          # claim 由字面值硬寫
assert_contains "$TMP/neg.out" "inert-ok-missing-reason"  # 豁免註解缺理由

echo "== case 3: 既有 repo（main）應 PASS（baseline allowlist 生效）"
set +e
python3 "$CHECKER" --root "$ROOT" >"$TMP/repo.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 0 ] || { cat "$TMP/repo.out"; fail "既有 repo 掃描應 exit 0（baseline 失效？），實際 $rc"; }
assert_contains "$TMP/repo.out" "inert 閉環檢查通過"

echo "== case 4: baseline reason 留 TODO 應 FAIL（理由必填）"
python3 - "$ROOT/scripts/ci/inert-baseline.json" "$TMP/baseline-todo.json" <<'PY'
import json, sys
src, dst = sys.argv[1], sys.argv[2]
data = json.load(open(src, encoding="utf-8"))
entries = data.setdefault("entries", {})
entries.setdefault("config-inert", {})["demo.inert_flag"] = {
    "reason": "TODO: 未填理由",
    "evidence": "fixture",
}
json.dump(data, open(dst, "w", encoding="utf-8"), ensure_ascii=False, indent=2)
PY
set +e
python3 "$CHECKER" --root "$ROOT" --baseline "$TMP/baseline-todo.json" >"$TMP/todo.out" 2>&1
rc=$?
set -e
[ "$rc" -eq 1 ] || { cat "$TMP/todo.out"; fail "reason=TODO 應 exit 1，實際 $rc"; }
assert_contains "$TMP/todo.out" "沒有填理由"

echo "✅ inert 閉環檢查回歸測試全數通過（4/4）"
