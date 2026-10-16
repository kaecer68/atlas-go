#!/usr/bin/env bash
# ============================================================
# test-ci-timeout-tri-state.sh — `make ci` 三態閘門（pass／skip／fail）的契約測試
# ------------------------------------------------------------
# 【來源】FU-20260926-12：「`make ci` 把『掛住（timeout 124）』算成 skipped ⇒ 閘門回 0」。
#   根因在 Makefile 的 ci recipe：rc=124 只做 `skipped=$$((skipped+1))`，收尾只檢查
#   `failed -gt 0` ⇒ 一支永遠跑不完的檢查（等網路／等鎖／等 docker）在閘門眼裡等於通過
#   （false-green，#2011 家族）。
#
# 【為什麼是「抽取真檔」而不是抄一份】本測試**逐字抽出** Makefile 的
#   `ci: check-production-host` recipe（tab 縮排起、到第一個非 tab 行為止），把那張真檔
#   放進 fixture 目錄執行（fixture 只提供自己的 scripts/ci/ 內容）。抄一份的話，Makefile
#   改了、測試還在驗舊行為 —— 證明了自己，沒有證明閘門。
#
# 【覆蓋】
#   ① 掛住（超出 CI_SCRIPT_TIMEOUT）⇒ 必須紅，且**同一行**指名是哪一支、附 elapsed
#   ② 被訊號帶走（kill -TERM 自己）⇒ 必須紅，且不得被誤標成 TIMEOUT
#   ③ 宣告式 skip（SKIP (declared)）⇒ 必須綠，且那支**真的沒被執行**（marker 不存在）
#   ④ 正常（全部 pass）⇒ 必須綠，且計數行逐字等於預期（閘門不得自行美化）
#   ⑤ 非 0 失敗 ⇒ 必須紅，訊息帶 rc
#   ⑥ glob 空集合（只剩 declared-skip 的檔案）⇒ 必須紅（「什麼都沒跑」不是通過）
#   ⑧ 慢檢查的預算覆寫（CI_SCRIPT_TIMEOUT_SLOW）：清單內放寬 ⇒ 綠；超過放寬後預算 ⇒ 仍紅
#   ⑦ mutation 自證：把 recipe 改回舊行為（timeout 計入 skipped、收尾不看 timeout）
#      ⇒ 掛住又變回綠（rc=0），且 ① 的同一個判準在該結果上**變紅** ⇒ 本測試對「舊行為」
#      有鑑別力（抓到回歸），不是裝飾品
#
# 執行： bash tests/scripts/test-ci-timeout-tri-state.sh
#   hermetic：只用 mktemp 目錄 ＋ 腳本 stub；不連網、不碰 git、不跑真的 check_*.sh。
set -uo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
MAKEFILE="$ROOT/Makefile"

# 共用負向證明庫（精確 rc 斷言、同一行雙 needle 斷言、log 縮排輸出）
# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "$ROOT/scripts/ci/negative-proof-lib.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "  ✅ $*"; }

TMP=$(mktemp -d "${TMPDIR:-/tmp}/ci-tri-state.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

log_show() { negproof_show_log "${1:-}"; }

# ── 抽取真檔 recipe ───────────────────────────────────────────────────────
RECIPE="$TMP/ci-recipe.mk"
awk '
  /^ci: check-production-host$/ { inrecipe = 1; next }
  inrecipe && substr($0, 1, 1) == "\t" { print; next }
  inrecipe { exit }
' "$MAKEFILE" >"$RECIPE"

# 抽取器自我斷言：空集合／錨點漂移 ⇒ 立刻紅（否則本測試會「證明空氣」）
[ -s "$RECIPE" ] || fail "抽不到 $MAKEFILE 的 ci recipe（target 改名或縮排改變？）—— 空集合不是通過"
grep -Fq -- 'for script in scripts/ci/check_*.sh; do' "$RECIPE" \
  || fail "抽取到的 ci recipe 內沒有 scripts/ci/check_*.sh glob（抽取範圍錯？）"
grep -Fq -- 'timedout=$$((timedout+1))' "$RECIPE" \
  || fail "ci recipe 沒有「timeout 獨立計數」（FU-20260926-12 回歸？）"
grep -Fq -- 'if [ $$timedout -gt 0 ]; then' "$RECIPE" \
  || fail "ci recipe 收尾沒有 timedout 的 fail-closed 判定（FU-20260926-12 回歸？）"
grep -Fq -- 'script_timeout=$(CI_SCRIPT_TIMEOUT_SLOW)' "$RECIPE" \
  || fail "ci recipe 沒有 per-script 預算覆寫 ⇒ 已量測的慢檢查會在高負載下假紅（2026-10-16 pre-push 實證）"
ok "抽取到真檔 ci recipe（$(wc -l <"$RECIPE" | tr -d ' ') 行）：含 timeout 獨立計數 + fail-closed 收尾"

# ── fixture 工廠 ──────────────────────────────────────────────────────────
# make_fixture <dir> [<recipe 檔>] — 真檔 recipe ＋ stub 的 check_frontend_dist.sh。
# recipe 的第一條是 `bash scripts/ci/check_frontend_dist.sh --diff origin/main...HEAD`
# （ci target 開頭明列跑的那一支）；fixture 不是 git repo，故以 stub 隔離 —— 本測試
# 驗的是**迴圈的三態判定**，不是那支腳本本身。
make_fixture() {
  local d=$1 recipe=${2:-$RECIPE}
  mkdir -p "$d/scripts/ci"
  printf 'check-production-host:\n\t@true\n\n.PHONY: ci\nci: check-production-host\n' >"$d/Makefile"
  cat "$recipe" >>"$d/Makefile"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$d/scripts/ci/check_frontend_dist.sh"
  chmod +x "$d/scripts/ci/check_frontend_dist.sh"
}

# add_script <dir> <name> <行…> — 在 fixture 的 scripts/ci/ 放一支檢查腳本
add_script() {
  local d=$1 n=$2
  shift 2
  { printf '#!/usr/bin/env bash\n'; printf '%s\n' "$@"; } >"$d/scripts/ci/$n"
  chmod +x "$d/scripts/ci/$n"
}

# run_fixture <dir> <log> [額外 make 變數…] — 跑 fixture 的 `make ci`（預設預算縮到 1s），印出 rc
run_fixture() {
  local d=$1 log=$2 rc=0
  shift 2
  ( cd "$d" && make ci CI_SCRIPT_TIMEOUT=1 "$@" ) >"$log" 2>&1 || rc=$?
  printf '%s' "$rc"
}

# assert_gate_red_on_hang <rc> — ① 的核心判準：掛住時閘門必須回非 0。
# ⑦（mutation 自證）拿**同一個**判準去驗突變後的結果，證明這個判準有鑑別力
# （而不是只證明「我們的新碼剛好會印這些字」）。
assert_gate_red_on_hang() { [ "$1" -ne 0 ]; }

# assert_summary <log> <逐字期望的計數行> — 閘門的自述必須與事實一致
assert_summary() {
  local log=$1 want=$2
  grep -Fxq -- "$want" "$log" || {
    log_show "$log"
    fail "計數行與事實不符。期望逐字：'$want'"
  }
}

# assert_no_line_with <needleA> <needleB> <log> <說明> — 不得有任何一行**同時**含 A 與 B
# （用來證明「這支掛住／被訊號帶走的腳本，沒有被歸到另一個分類」）
assert_no_line_with() {
  local a=$1 b=$2 log=$3 desc=$4
  if awk -v a="$a" -v b="$b" \
       'index($0, a) && index($0, b) { found = 1; exit } END { exit(found ? 0 : 1) }' "$log"; then
    log_show "$log"
    fail "${desc}（同一行同時含 '$a' 與 '$b'）"
  fi
  return 0
}

SUM_HDR='✅ CI: '
# 註：每個 fixture 都放了 check_frontend_dist.sh 的 stub（recipe 第一條明列呼叫它），
# 而它在本 target 內是 declared-skip ⇒ 每個 fixture 的計數行都固定有 1 個 skipped。
echo "── ① 掛住（rc=124）必須紅 ──────────────────────────────────────"
D1="$TMP/c1"
make_fixture "$D1"
add_script "$D1" check_aaa_ok.sh 'exit 0'
add_script "$D1" check_zzz_hang.sh 'sleep 5'
L1="$TMP/c1.log"
RC1=$(run_fixture "$D1" "$L1")
assert_gate_red_on_hang "$RC1" || {
  log_show "$L1"
  fail "① 掛住的檢查讓 make ci 回 0（FU-20260926-12 未修：false-green）"
}
negproof_expect_output_line 'check_zzz_hang.sh' 'TIMEOUT' "$L1" \
  "① 掛住必須紅，且訊息指名掛住的是哪一支" || { log_show "$L1"; exit 1; }
negproof_expect_output_line 'check_zzz_hang.sh' 'elapsed=' "$L1" \
  "① 掛住的訊息必須附耗時（elapsed），否則無法分辨「掛住」與「跳過」" || { log_show "$L1"; exit 1; }
assert_summary "$L1" "${SUM_HDR}1 passed, ⏭️  1 skipped (declared), ❌ 0 failed, ⏱️  1 timed out"
ok "① 掛住 ⇒ rc=${RC1}（非 0），且訊息指名腳本 + elapsed，且**不**計入 skipped"

echo "── ② 訊號（rc=143）必須紅，且不得被當成 TIMEOUT/skip ────────────"
D2="$TMP/c2"
make_fixture "$D2"
add_script "$D2" check_aaa_ok.sh 'exit 0'
add_script "$D2" check_yyy_kill.sh 'kill -TERM $$'
L2="$TMP/c2.log"
RC2=$(run_fixture "$D2" "$L2")
[ "$RC2" -ne 0 ] || { log_show "$L2"; fail "② 被訊號帶走的檢查沒有讓閘門變紅（rc=${RC2}）"; }
negproof_expect_output_line 'check_yyy_kill.sh' 'FAILED (rc=' "$L2" \
  "② 訊號必須落在 FAILED（帶 rc），不是 TIMEOUT、也不是 skip" || { log_show "$L2"; exit 1; }
assert_no_line_with 'check_yyy_kill.sh' 'TIMEOUT' "$L2" "② 訊號被誤標成 TIMEOUT"
assert_summary "$L2" "${SUM_HDR}1 passed, ⏭️  1 skipped (declared), ❌ 1 failed, ⏱️  0 timed out"
ok "② 訊號 ⇒ rc=${RC2}（非 0，計入 failed 而非 timed out/skipped）"

echo "── ③ 宣告式 skip：必須綠，且那支真的沒被執行 ────────────────────"
D3="$TMP/c3"
make_fixture "$D3"
add_script "$D3" check_aaa_ok.sh 'exit 0'
# 被跳過的腳本若真的被執行，就會留下 marker（=> 「skip 是假的」可被否證）。
# 用絕對路徑寫死，不靠環境變數傳遞 —— 否則「MARKER 沒設 ⇒ 沒寫檔」會與「真的沒跑」無法分辨。
MARKER="$D3/ran.marker"
add_script "$D3" check_data_naming.sh ": >'$MARKER'; exit 7"
L3="$TMP/c3.log"
RC3=$(run_fixture "$D3" "$L3")
[ "$RC3" -eq 0 ] || { log_show "$L3"; fail "③ 宣告式 skip 讓閘門誤紅（rc=${RC3}）——正常跳過不得被放寬成一律紅"; }
[ ! -e "$MARKER" ] || { log_show "$L3"; fail "③ 被判 SKIP 的腳本竟然被執行了（skip 是假的）"; }
negproof_expect_output_line 'check_data_naming.sh' 'SKIP (declared)' "$L3" \
  "③ 跳過必須是「宣告式」且印出被跳過的檔名與理由" || { log_show "$L3"; exit 1; }
assert_summary "$L3" "${SUM_HDR}1 passed, ⏭️  2 skipped (declared), ❌ 0 failed, ⏱️  0 timed out"
ok "③ 宣告式 skip ⇒ rc=0，且被跳過的腳本確實沒有被執行（marker 不存在）"

echo "── ④ 正常（無掛住／無失敗）必須綠 ──────────────────────────────"
D4="$TMP/c4"
make_fixture "$D4"
add_script "$D4" check_aaa_ok.sh 'exit 0'
add_script "$D4" check_bbb_ok.sh 'exit 0'
L4="$TMP/c4.log"
RC4=$(run_fixture "$D4" "$L4")
[ "$RC4" -eq 0 ] || { log_show "$L4"; fail "④ 正常情況閘門誤紅（rc=${RC4}）"; }
assert_summary "$L4" "${SUM_HDR}2 passed, ⏭️  1 skipped (declared), ❌ 0 failed, ⏱️  0 timed out"
ok "④ 正常情況 ⇒ rc=0，計數行逐字相符"

echo "── ⑤ 非 0 失敗必須紅且訊息帶 rc ─────────────────────────────────"
D5="$TMP/c5"
make_fixture "$D5"
add_script "$D5" check_aaa_ok.sh 'exit 0'
add_script "$D5" check_zzz_fail.sh 'exit 3'
L5="$TMP/c5.log"
RC5=$(run_fixture "$D5" "$L5")
[ "$RC5" -ne 0 ] || { log_show "$L5"; fail "⑤ 失敗的檢查沒有讓閘門變紅（rc=${RC5}）"; }
negproof_expect_output_line 'check_zzz_fail.sh' 'FAILED (rc=3' "$L5" \
  "⑤ 失敗必須指名腳本與它的 rc" || { log_show "$L5"; exit 1; }
ok "⑤ 非 0 失敗 ⇒ rc=${RC5}（非 0），訊息帶 rc=3"

echo "── ⑥ glob 空集合必須紅 ─────────────────────────────────────────"
D6="$TMP/c6"
make_fixture "$D6"   # 只有 frontend_dist stub，而它是 declared-skip ⇒ 迴圈內 0 支可跑
add_script "$D6" check_data_naming.sh 'exit 0'
L6="$TMP/c6.log"
RC6=$(run_fixture "$D6" "$L6")
[ "$RC6" -ne 0 ] || { log_show "$L6"; fail "⑥ 0 支檢查被執行卻回綠（rc=${RC6}）——『什麼都沒跑』不是通過"; }
negproof_expect_output '0 支檢查被執行' "$L6" "⑥ 空集合必須以「什麼都沒跑不是通過」紅燈" || { log_show "$L6"; exit 1; }
ok "⑥ 全域 declared-skip（0 支可跑）⇒ rc=${RC6}（非 0）"

echo "── ⑦ mutation 自證：改回舊行為 ⇒ ①的斷言必須失效 ────────────────"
MUT="$TMP/ci-recipe-old.mk"
sed -e 's/timedout=\$\$((timedout+1))/skipped=$$((skipped+1))/' \
    -e 's/if \[ \$\$timedout -gt 0 \]; then/if [ 0 -gt 0 ]; then/' \
    "$RECIPE" >"$MUT"
CHANGED=$(diff "$RECIPE" "$MUT" | grep -c '^[<>]' || true)
[ "${CHANGED:-0}" -eq 4 ] || fail "⑦ mutation 沒有真的改到舊行為的兩處（diff 行數=${CHANGED:-0}，期望 4）⇒ 突變是裝飾品"
D7="$TMP/c7"
make_fixture "$D7" "$MUT"
add_script "$D7" check_aaa_ok.sh 'exit 0'
add_script "$D7" check_zzz_hang.sh 'sleep 5'
L7="$TMP/c7.log"
RC7=$(run_fixture "$D7" "$L7")
# 突變後重現的是**舊行為**：掛住仍讓閘門回 0（false-green）
[ "$RC7" -eq 0 ] || { log_show "$L7"; fail "⑦ 突變回舊行為後竟非 0（rc=${RC7}）⇒ 突變沒改到判準"; }
# 而 ① 的同一個判準必須在突變結果上**變紅** ⇒ 這個護衛抓得到回歸，不是裝飾品
if assert_gate_red_on_hang "$RC7"; then
  fail "⑦ ①的判準對突變後（假綠）的結果竟然判「通過」⇒ 本測試沒有鑑別力"
fi
ok "⑦ mutation（timeout 計入 skipped ＋ 收尾不看 timedout）⇒ rc=0，且 ① 的判準在此**變紅**（護衛有牙齒）"

echo "── ⑧ 慢檢查的預算覆寫：清單內放寬、清單外照舊 ────────────────────"
D8="$TMP/c8"
make_fixture "$D8"
add_script "$D8" check_aaa_ok.sh 'exit 0'
add_script "$D8" check_secrets.sh 'sleep 3'      # 覆寫清單內（空機 13.6s ⇒ 已放寬到 CI_SCRIPT_TIMEOUT_SLOW）
L8A="$TMP/c8a.log"
RC8A=$(run_fixture "$D8" "$L8A" CI_SCRIPT_TIMEOUT_SLOW=10)
[ "$RC8A" -eq 0 ] || { log_show "$L8A"; fail "⑧a 覆寫清單內的慢檢查在放寬預算下仍誤紅（rc=${RC8A}）"; }
assert_no_line_with 'check_secrets.sh' 'TIMEOUT' "$L8A" "⑧a 覆寫清單內的檢查在放寬預算下被誤標 TIMEOUT"
assert_summary "$L8A" "${SUM_HDR}2 passed, ⏭️  1 skipped (declared), ❌ 0 failed, ⏱️  0 timed out"
ok "⑧a 覆寫清單內的慢檢查用 CI_SCRIPT_TIMEOUT_SLOW ⇒ 綠（不會因負載假紅）"

L8B="$TMP/c8b.log"
RC8B=$(run_fixture "$D8" "$L8B" CI_SCRIPT_TIMEOUT_SLOW=1)
[ "$RC8B" -ne 0 ] || { log_show "$L8B"; fail "⑧b 覆寫不是「免死金牌」：超過預算必須紅（rc=${RC8B}）"; }
negproof_expect_output_line 'check_secrets.sh' 'TIMEOUT' "$L8B" \
  "⑧b 覆寫只加大預算，超時仍必須落在 TIMEOUT ⇒ 紅" || { log_show "$L8B"; exit 1; }
ok "⑧b 覆寫清單內但超過預算 ⇒ 仍紅（rc=${RC8B}；覆寫沒有開新的假綠路徑）"

echo "ALL PASS: ci timeout 三態契約（FU-20260926-12）"
