#!/usr/bin/env bash
# jev-contract-c5-selftest.sh — C5 行為回歸測試（2026-09-24 新增；2026-09-30 改 opt-in）
#
# 背景：jevkit --selftest 含實際 API 呼叫（依賴網路），與本檢查宣稱的
#       「AST 靜態分析，不需網路」矛盾；API 限流會讓 pre-push gate 偽失敗。
#       2026-09-24：預設降為 warn-only。
#       2026-09-30：改為 **opt-in** —— 預設**完全不呼叫**（不連外）；顯式
#       `--with-selftest` / `JEV_CONTRACT_SELFTEST=1` 才跑 live，且失敗即 blocking。
set -uo pipefail
cd "$(dirname "$0")/../.."
PASS=0; FAIL=0
ok()  { PASS=$((PASS+1)); echo "ok $1"; }
bad() { FAIL=$((FAIL+1)); echo "FAIL $1"; }

CHECK="scripts/jev-contract-check.py"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# ① 預設 → exit 0 **且完全不呼叫**（純靜態、不連外）
TYPESAFE_API_KEY=invalid-for-selftest python3 "$CHECK" --root . > "$TMP/a.log" 2>&1
rc=$?
if [ "${rc}" = "0" ] && ! grep -q '實際呼叫' "$TMP/a.log" && grep -q 'live 自測未執行' "$TMP/a.log"; then
  ok "① 預設 → exit 0 且未呼叫 API（純靜態）"
else bad "① 預設（rc=${rc}）"; sed -n '1,15p' "$TMP/a.log"; fi

# ② opt-in ＋ 壞 key → exit 1（顯式要求 ⇒ live 失敗即 blocking）
JEV_CONTRACT_STRICT_SELFTEST=1 TYPESAFE_API_KEY=invalid-for-selftest python3 "$CHECK" --root . > "$TMP/b.log" 2>&1
rc=$?
if [ "${rc}" = "1" ] && grep -q 'C5 live 自測失敗' "$TMP/b.log"; then ok "② opt-in（STRICT 別名）＋ 壞 key → exit 1 + blocking"; else bad "② opt-in ＋ 壞 key（rc=${rc}）"; sed -n '1,15p' "$TMP/b.log"; fi

# ②b opt-in 旗標（--with-selftest）亦可觸發 live 路徑（同樣 blocking）
TYPESAFE_API_KEY=invalid-for-selftest python3 "$CHECK" --root . --with-selftest > "$TMP/b2.log" 2>&1
rc=$?
if [ "${rc}" = "1" ] && grep -q 'C5 live 自測失敗' "$TMP/b2.log"; then ok "②b --with-selftest → 走 live 且失敗 blocking"; else bad "②b --with-selftest（rc=${rc}）"; sed -n '1,15p' "$TMP/b2.log"; fi

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
