#!/usr/bin/env bash
# jev-contract-c5-selftest.sh — C5 warn-only 行為回歸測試（2026-09-24 新增）
#
# 背景：jevkit --selftest 含實際 API 呼叫（依賴網路），與本檢查宣稱的
#       「AST 靜態分析，不需網路」矛盾；API 限流會讓 pre-push gate 偽失敗。
#       修正後：預設 warn-only；JEV_CONTRACT_STRICT_SELFTEST=1 時仍 blocking。
set -uo pipefail
cd "$(dirname "$0")/../.."
PASS=0; FAIL=0
ok()  { PASS=$((PASS+1)); echo "ok $1"; }
bad() { FAIL=$((FAIL+1)); echo "FAIL $1"; }

CHECK="scripts/jev-contract-check.py"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# ① 壞 key + 預設 → exit 0（warn-only）
TYPESAFE_API_KEY=invalid-for-selftest python3 "$CHECK" --root . > "$TMP/a.log" 2>&1
rc=$?
if [ "${rc}" = "0" ] && grep -q 'warn-only' "$TMP/a.log"; then ok "① 壞 key + 預設 → exit 0 + warn"; else bad "① 壞 key + 預設（rc=${rc}）"; sed -n '1,15p' "$TMP/a.log"; fi

# ② 壞 key + STRICT → exit 1（blocking）
JEV_CONTRACT_STRICT_SELFTEST=1 TYPESAFE_API_KEY=invalid-for-selftest python3 "$CHECK" --root . > "$TMP/b.log" 2>&1
rc=$?
if [ "${rc}" = "1" ] && grep -q 'C5 自測失敗' "$TMP/b.log"; then ok "② 壞 key + STRICT → exit 1 + 違規"; else bad "② 壞 key + STRICT（rc=${rc}）"; sed -n '1,15p' "$TMP/b.log"; fi

# ③ jevkit.py 不存在 → 仍 blocking（靜態要求）
TMPREPO="$(mktemp -d)"; mkdir -p "$TMPREPO/scripts"
cp "$CHECK" "$TMPREPO/scripts/" 2>/dev/null || true
python3 "$CHECK" --root "$TMPREPO" > "$TMP/c.log" 2>&1
rc=$?
if [ "${rc}" = "1" ] && grep -q 'jevkit.py 不存在' "$TMP/c.log"; then ok "③ jevkit.py 不存在 → exit 1（靜態仍 blocking）"; else bad "③ jevkit.py 不存在（rc=${rc}）"; sed -n '1,15p' "$TMP/c.log"; fi
rm -rf "$TMPREPO"

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[[ "$FAIL" == 0 ]]
