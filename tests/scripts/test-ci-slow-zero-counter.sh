#!/usr/bin/env bash
# ============================================================
# test-ci-slow-zero-counter.sh — `make ci-slow` 的「0 支慢檢查被執行 ⇒ 紅」計數守衛契約測試
# ------------------------------------------------------------
# 【來源】FU-20261016-24：`make ci-slow` 逐字明列 4 支慢檢查，卻只以 `[ -f "$$script" ]`
#   決定跑不跑，且**無任何計數守衛** ⇒ 清單內每一支都不存在時（腳本被改名／被搬到別的路徑／
#   在錯的工作目錄跑 make）迴圈 body 一次都沒進去，收尾仍印 `✅ ci-slow complete` 且 rc=0
#   ⇒「什麼都沒跑」被當成通過（閘門說謊；同族 FU-20260926-12／FU-20261016-18／-22）。
#   修前逐字原文（真檔 Makefile，2026-10-16 本車道實跑）：
#     $ cd "$(mktemp -d)" && make -f <repo>/Makefile ci-slow
#     🛡️  Running slow CI checks (data_naming/layer3/markdown_links)...
#     ✅ ci-slow complete
#     rc=0
#
# 【為什麼是「抽取真檔 recipe」而不是抄一份】抄一份的話，Makefile 改了、測試還在驗舊行為
#   —— 證明了自己，沒有證明閘門（同 tests/scripts/test-ci-timeout-tri-state.sh 的理由）。
#   本測試逐字抽出 Makefile 的 `ci-slow:` recipe（tab 縮排起、到第一個非 tab 行為止），放進
#   fixture 執行；受測的檔名清單由 recipe 內文推導（不寫死在測試裡 ⇒ recipe 改名時測試跟著
#   變，不會偷偷驗一個不存在的清單）。
#
# 【覆蓋】
#   ① 0 支候選腳本 ⇒ 必紅，且逐字印出「沒有任何慢檢查被執行」與 0/N 計數、且**不再**印 complete
#   ② 正常（recipe 宣告的每一支都在、全部 exit 0）⇒ 綠 ＋ 計數行逐字相符（不得誤紅）
#   ③ 部分缺失（N-1 支）⇒ **仍綠**（部分缺失不等於「什麼都沒跑」）＋ 逐支印出缺哪一支
#   ④ 清單內某支 rc≠0 ⇒ 紅 ＋ 同一行指名該腳本與它的 rc（fail-fast 未放寬）
#   ⑤ 真檔一致性：recipe 宣告的檔名在 repo 內真的存在（清單漂移 ⇒ 紅）
#   ⑥ mutation 自證（判準要有鑑別力）：(a) 拿掉 fail-closed 判定 ⇒ 0 支再度變回綠
#      （false-green 復現），且 ① 的判準在該結果上**變紅**；(b) 拿掉 missing 計數 ⇒
#      ① 的計數斷言在該結果上**變紅** ⇒ 兩個斷言都不是裝飾品
#
# 執行： bash tests/scripts/test-ci-slow-zero-counter.sh
#   hermetic：只用 mktemp 目錄 ＋ 腳本 stub；不連網、不跑真的 check_*.sh、不改真檔。
set -uo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
MAKEFILE="$ROOT/Makefile"

# 共用負向證明庫（精確 rc 斷言、同一行雙 needle 斷言、log 縮排輸出）
# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "$ROOT/scripts/ci/negative-proof-lib.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "  ✅ $*"; }

TMP=$(mktemp -d "${TMPDIR:-/tmp}/ci-slow-zero.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

log_show() { negproof_show_log "${1:-}"; }

# ── 抽取真檔 recipe ───────────────────────────────────────────────────────
RECIPE="$TMP/ci-slow-recipe.mk"
awk '
  /^ci-slow:$/ { inrecipe = 1; next }
  inrecipe && substr($0, 1, 1) == "\t" { print; next }
  inrecipe { exit }
' "$MAKEFILE" >"$RECIPE"

# 抽取器自我斷言：空集合／錨點漂移 ⇒ 立刻紅（否則本測試會「證明空氣」）
[ -s "$RECIPE" ] || fail "抽不到 $MAKEFILE 的 ci-slow recipe（target 改名或縮排改變？）—— 空集合不是通過"
grep -Fq -- 'executed=$$((executed+1))' "$RECIPE" \
  || fail "ci-slow recipe 沒有「實際執行」計數 executed（FU-20261016-24 回歸？）"
grep -Fq -- 'missing=$$((missing+1))' "$RECIPE" \
  || fail "ci-slow recipe 沒有「清單內不存在」計數 missing ⇒『少跑幾支』看不出來（FU-20261016-24 回歸？）"
grep -Fq -- 'if [ $$executed -eq 0 ]; then' "$RECIPE" \
  || fail "ci-slow recipe 收尾沒有 executed=0 的 fail-closed 判定（FU-20261016-24 回歸？）"

# 受測清單由 recipe 內文推導（逐字：scripts/ci/*.sh）
DECLARED=$(grep -oE 'scripts/ci/[A-Za-z0-9_.-]+\.sh' "$RECIPE" | sort -u)
NDECL=$(printf '%s\n' "$DECLARED" | grep -c . || true)
[ "${NDECL:-0}" -ge 1 ] || fail "recipe 內找不到任何 scripts/ci/*.sh 宣告（抽取範圍錯？）"
ok "抽取到真檔 ci-slow recipe（$(wc -l <"$RECIPE" | tr -d ' ') 行）：executed/missing 計數 ＋ fail-closed 收尾；清單 ${NDECL} 支"

# ── fixture 工廠 ──────────────────────────────────────────────────────────
# make_fixture <dir> [<recipe 檔>] — 真檔 recipe 放進 fixture；fixture 只提供自己的 scripts/ci/
make_fixture() {
  local d=$1 recipe=${2:-$RECIPE}
  mkdir -p "$d/scripts/ci"
  printf 'ci-slow:\n' >"$d/Makefile"
  cat "$recipe" >>"$d/Makefile"
}

# add_script <dir> <name> <行…> — 在 fixture 的 scripts/ci/ 放一支（stub）慢檢查
add_script() {
  local d=$1 n=$2
  shift 2
  { printf '#!/usr/bin/env bash\n'; printf '%s\n' "$@"; } >"$d/scripts/ci/$n"
  chmod +x "$d/scripts/ci/$n"
}

# add_all_declared <dir> [<不建立的那一支>] — 依 recipe 清單把 stub 補齊（全部 exit 0）；
# 有給 omit 時那一支**不建立**（＝模擬清單漂移／腳本被搬走）
add_all_declared() {
  local d=$1 omit=${2:-} s base
  while IFS= read -r s; do
    [ -n "$s" ] || continue
    base=$(basename "$s")
    if [ -n "$omit" ] && [ "$base" = "$omit" ]; then continue; fi
    add_script "$d" "$base" 'exit 0'
  done <<<"$DECLARED"
}

# set_script_body <dir> <name> <行…> — 覆寫 fixture 內某支的內容（用來造「清單內某支失敗」）
set_script_body() {
  local d=$1 n=$2
  shift 2
  add_script "$d" "$n" "$@"
}

# run_fixture <dir> <log> — 在 fixture 目錄跑 `make ci-slow`，印出 rc（rc 一律從這裡讀，不經管線）
run_fixture() {
  local d=$1 log=$2 rc=0
  ( cd "$d" && make ci-slow ) >"$log" 2>&1 || rc=$?
  printf '%s' "$rc"
}

SUM_HDR='✅ ci-slow: '

# summary_is <log> <逐字期望的計數行> — 閘門的自述必須與事實一致（不用它 fail，供 mutation 自證反用）
summary_is() { grep -Fxq -- "$2" "$1"; }

assert_summary() {
  local log=$1 want=$2
  summary_is "$log" "$want" || {
    log_show "$log"
    fail "計數行與事實不符。期望逐字：'$want'"
  }
}

# assert_zero_red <rc> <log> — ①／⑥ 共用的核心判準：「0 支被執行」必須表現為**非 0**、
# 帶正確的紅燈訊息、且**不再**印 complete。⑥ 拿這個判準去驗突變後的假綠結果，證明它有鑑別力
# （而不是只證明「我們的新碼剛好會印這些字」）。
assert_zero_red() {
  local rc=$1 log=$2
  [ "$rc" -ne 0 ] || return 1
  grep -Fq -- '沒有任何慢檢查被執行' "$log" || return 1
  if grep -Fxq -- '✅ ci-slow complete' "$log"; then return 1; fi
  return 0
}

echo "── ① 0 支候選腳本 ⇒ 必紅（修前是 rc=0 ＋ 印 complete）────────────────"
D1="$TMP/c1"
make_fixture "$D1" # 一支候選腳本都不放 ＝ 清單內全部不存在
L1="$TMP/c1.log"
RC1=$(run_fixture "$D1" "$L1")
assert_zero_red "$RC1" "$L1" || {
  log_show "$L1"
  fail "① 0 支候選腳本卻沒有紅（rc=${RC1}）——『什麼都沒跑』不是通過（FU-20261016-24 未修）"
}
assert_summary "$L1" "${SUM_HDR}0/${NDECL} 支慢檢查被執行、${NDECL} 支不存在"
negproof_expect_output_line 'scripts/ci/' 'MISSING' "$L1" \
  "① 每一個缺失的候選腳本都要逐支印出（否則『少跑幾支』在 log 裡看不出來）" || { log_show "$L1"; exit 1; }
ok "① 0/${NDECL} ⇒ rc=${RC1}（非 0），訊息逐字說明『沒有任何慢檢查被執行』且不再印 complete"

echo "── ② 正常（清單每一支都在且 exit 0）⇒ 綠 ＋ 計數 ────────────────"
D2="$TMP/c2"
make_fixture "$D2"
add_all_declared "$D2"
L2="$TMP/c2.log"
RC2=$(run_fixture "$D2" "$L2")
[ "$RC2" -eq 0 ] || { log_show "$L2"; fail "② 正常情況閘門誤紅（rc=${RC2}）——不得把有腳本的情形一律染紅"; }
assert_summary "$L2" "${SUM_HDR}${NDECL}/${NDECL} 支慢檢查被執行、0 支不存在"
negproof_expect_output '✅ ci-slow complete' "$L2" \
  "② 正常情況仍必須印收尾的 complete（新增守衛不得讓正常路徑消失）" || { log_show "$L2"; exit 1; }
ok "② 正常情況 ⇒ rc=0，計數行逐字相符且照印 complete"

echo "── ③ 部分缺失（N-1 支）⇒ 仍綠，但要指名缺哪一支 ───────────────"
MISSING_NAME=$(basename "$(printf '%s\n' "$DECLARED" | tail -n 1)")
NDECL_MINUS=$((NDECL - 1))
D3="$TMP/c3"
make_fixture "$D3"
add_all_declared "$D3" "$MISSING_NAME" # 那一支不建立 ＝ 清單內檔案不存在（不是「建了但失敗」）
L3="$TMP/c3.log"
RC3=$(run_fixture "$D3" "$L3")
[ "$RC3" -eq 0 ] || { log_show "$L3"; fail "③ 部分缺失讓閘門誤紅（rc=${RC3}）——正常 checkout 不得因單支缺失而紅"; }
assert_summary "$L3" "${SUM_HDR}${NDECL_MINUS}/${NDECL} 支慢檢查被執行、1 支不存在"
negproof_expect_output_line "$MISSING_NAME" 'MISSING' "$L3" \
  "③ 缺失的那一支必須被指名（清單漂移要可診斷）" || { log_show "$L3"; exit 1; }
ok "③ ${NDECL_MINUS}/${NDECL} ⇒ rc=0（不誤紅），且指名缺失的 $MISSING_NAME"

echo "── ④ 清單內某支 rc≠0 ⇒ 紅 ＋ 同一行指名它與 rc ─────────────────"
FAIL_NAME=$(basename "$(printf '%s\n' "$DECLARED" | head -n 1)")
D4="$TMP/c4"
make_fixture "$D4"
add_all_declared "$D4"
set_script_body "$D4" "$FAIL_NAME" 'exit 3'
L4="$TMP/c4.log"
RC4=$(run_fixture "$D4" "$L4")
[ "$RC4" -ne 0 ] || { log_show "$L4"; fail "④ 慢檢查失敗卻回綠（rc=${RC4}）——fail-fast 被放寬了"; }
negproof_expect_output_line "$FAIL_NAME" 'FAILED (rc=3' "$L4" \
  "④ 失敗必須同一行指名腳本與它的 rc（不得只說『有一支失敗』）" || { log_show "$L4"; exit 1; }
ok "④ rc=3 的慢檢查 ⇒ rc=${RC4}（非 0），訊息指名 $FAIL_NAME"

echo "── ⑤ 真檔一致性：recipe 宣告的檔名在 repo 內真的存在 ────────────"
while IFS= read -r s; do
  [ -n "$s" ] || continue
  [ -f "$ROOT/$s" ] || fail "⑤ ci-slow 清單宣告 '$s'，但 repo 內不存在（清單漂移 ⇒ 該支永遠不會跑）"
done <<<"$DECLARED"
ok "⑤ 清單 ${NDECL} 支在 repo 內都存在（宣告與檔案一致）"

echo "── ⑥ mutation 自證：判定拿掉 ⇒ ① 的判準必須失效 ────────────────"
MUT1="$TMP/mut1.mk"
sed 's/if \[ \$\$executed -eq 0 \]; then/if false; then/' "$RECIPE" >"$MUT1"
CH1=$(diff "$RECIPE" "$MUT1" | grep -c '^[<>]' || true)
[ "${CH1:-0}" -eq 2 ] || fail "⑥a mutation 沒有真的改到 fail-closed 判定（diff 行數=${CH1:-0}，期望 2）⇒ 突變是裝飾品"
D6="$TMP/c6"
make_fixture "$D6" "$MUT1"
L6="$TMP/c6.log"
RC6=$(run_fixture "$D6" "$L6")
# 突變後重現的是**修前行為**：0 支被執行仍回 0（false-green）
[ "$RC6" -eq 0 ] || { log_show "$L6"; fail "⑥a 拿掉 fail-closed 判定後 0 支竟仍非 0（rc=${RC6}）⇒ 突變沒改到判準"; }
negproof_expect_output '✅ ci-slow complete' "$L6" \
  "⑥a 突變後必須重現『0 支仍印 complete』這條假綠路徑" || { log_show "$L6"; exit 1; }
# 而 ① 的同一個判準必須在突變結果上**變紅** ⇒ 這個守衛抓得到回歸，不是裝飾品
if assert_zero_red "$RC6" "$L6"; then
  fail "⑥a ①的判準對突變後（假綠）的結果竟然判「通過」⇒ 本測試沒有鑑別力"
fi
ok "⑥a 拿掉 fail-closed 判定 ⇒ rc=0（false-green 復現），且 ① 的判準在此**變紅**"

MUT2="$TMP/mut2.mk"
sed 's/missing=\$\$((missing+1))/:/' "$RECIPE" >"$MUT2"
CH2=$(diff "$RECIPE" "$MUT2" | grep -c '^[<>]' || true)
[ "${CH2:-0}" -eq 2 ] || fail "⑥b mutation 沒有真的改到 missing 計數（diff 行數=${CH2:-0}，期望 2）⇒ 突變是裝飾品"
D7="$TMP/c7"
make_fixture "$D7" "$MUT2"
L7="$TMP/c7.log"
RC7=$(run_fixture "$D7" "$L7")
[ "$RC7" -ne 0 ] || { log_show "$L7"; fail "⑥b 拿掉 missing 計數後閘門竟回 0（rc=${RC7}）⇒ 突變沒改到計數"; }
# ① 的計數斷言必須在此**變紅**（log 會宣稱『0 支不存在』，與事實的 ${NDECL} 支不符）
if summary_is "$L7" "${SUM_HDR}0/${NDECL} 支慢檢查被執行、${NDECL} 支不存在"; then
  fail "⑥b ① 的計數斷言對『missing 沒被計數』的結果竟然判通過 ⇒ 該斷言沒有鑑別力"
fi
ok "⑥b 拿掉 missing 計數 ⇒ 計數行說謊（宣稱 0 支不存在），① 的計數斷言在此**變紅**"

echo "ALL PASS: ci-slow 計數守衛契約（FU-20261016-24）"
