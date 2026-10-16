#!/usr/bin/env bash
# ============================================================
# test-decide-completes-all-checks.sh — `scripts/openclaw/decide.sh` 的
#   「跑完全部安全檢查才產生總結」契約測試（FU-20261016-30）
# ------------------------------------------------------------
# 【來源】FU-20261016-30：`scripts/openclaw/decide.sh` 的計數用裸算術自增
#   （修前 :184,194,203 `((checks_total++))`；:188,197,206 `((checks_passed++))`）。
#   本檔 :7 是 `set -e`，而在 `set -e` 下**算術命令的結束碼是「運算式是否為 0」**
#   ⇒ `checks_total=0` 時的 `((checks_total++))` 回傳 **1**（post-increment 的運算式值是舊值 0）
#   ⇒ shell 立刻結束。
#   ⇒ **第一項安全檢查跑之前**整個腳本中止：三項檢查都不跑、總結
#   （`Safety: N/M checks passed`）不印、`--dry-run` 預覽（`=== DRY RUN ===`）不印、rc=1
#   —— 這是**假紅**（判定機制病灶）。與 FU-20261016-28 的 `scripts/ci/local-ci.sh` 同根因。
#
#   修前逐字原文（真檔，2026-10-16 本車道實跑；節錄）：
#     $ bash scripts/openclaw/decide.sh --promote exp-lane30 --reason "lane30 repro" --dry-run
#     Analyzing experiment result...
#     Running safety checks...
#
#     rc=1        ← 三項檢查、`Safety: N/M checks passed`、`=== DRY RUN ===` 全都沒印
#
# 【為什麼是「真檔副本」而不是抄一份】抄一份的話，真檔改回 `((checks_total++))` 時測試還在驗
#   舊行為 —— 證明了自己，沒有證明閘門（同 tests/scripts/test-local-ci-completes-all-checks.sh）。
#   本測試在臨時目錄造一個**假 repo 根**（`$TMP/root/scripts/openclaw/decide.sh` ＝ 真檔逐位元組
#   副本，sha256 相等才繼續；`decide.sh` 以 `$SCRIPT_DIR/../..` 推導 `PROJECT_ROOT`，故可搬移），
#   裡面的 `data/state/experiments/*.json` 全是受控 fixture。**不碰真 repo、不跑 go、不連網。**
#
# 【覆蓋】
#   ① 抽取器自我斷言（副本 sha256＝真檔／`set -e` 仍在／總結錨點仍在）＋真檔靜態契約
#      （`check_promotion_safety()` 內不得再有裸 `((VAR++))` 形狀；計數必須是賦值形式）
#   ② 正常路徑（--dry-run）⇒ 三項檢查全跑 ＋ 總結 `Safety: 3/3 checks passed` ＋
#      `=== DRY RUN ===` 預覽 ＋ rc=0
#   ③ 一項檢查失敗（status=pending）⇒ **三項檢查照跑** ＋ 總結 `Safety: 2/3 checks passed` ＋
#      `Warning: Not all safety checks passed` ＋ rc=1（**真錯誤不得被忽略**：fail-closed 不變）
#   ④ `--help` rc=0 且印 `Usage:`；未知選項 rc=1（本次改動不動 CLI 介面）
#   ⑤ mutation 自證（鑑別力）：六處計數全改回裸 `((VAR++))` ⇒ **修前行為復現**
#      （一項檢查都沒跑、無總結、無預覽、rc=1），且 ② 的判準在該結果上**變紅** ⇒ ② 不是裝飾品
#   ⑥ mutation（第二形狀）：只把 `checks_passed` 三處改回裸形式 ⇒ 只跑到「Check 1」之後就中止
#      （3 項中只跑 1 項）⇒ 兩種形狀各別都是致命點，不是只修 `checks_total` 就好
#
# 執行： bash tests/scripts/test-decide-completes-all-checks.sh
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC="$REPO/scripts/openclaw/decide.sh"

# 共用負向證明庫（精確 rc 斷言；同其他 tests/scripts/*.sh）
# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "$REPO/scripts/ci/negative-proof-lib.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "  ✅ $*"; }
log_show() { negproof_show_log "${1:-}"; }

TMP="$(mktemp -d "${TMPDIR:-/tmp}/fu30-decide.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

negproof_require_file "$SRC" "受測檔案 scripts/openclaw/decide.sh" || exit 1

BARE_RE='^[[:space:]]*\(\([[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]*(\+\+|--)[[:space:]]*\)\)'

# ── hermetic 假 repo 根（真檔副本 ＋ 同層相對深度，PROJECT_ROOT 才推導得出來）──
FAKE="$TMP/root"
mkdir -p "$FAKE/scripts/openclaw" "$FAKE/data/state/experiments"
cp "$SRC" "$FAKE/scripts/openclaw/decide.sh"

# strip_ansi <in> <out> — 去掉 SGR 顏色碼（受測腳本硬編碼顏色，無 NO_COLOR 開關）
#   用**字面 ESC 位元組**（不靠 `\033` escape，GNU/BSD sed 都吃）＋ `LC_ALL=C`（byte 導向，
#   避開 BSD sed 在 UTF-8 locale 對本檔輸出的「illegal byte sequence」）。sed 失敗或行數不合
#   ⇒ 直接留原檔（本測試的斷言都是 `grep -F`，原檔帶顏色碼也命中，不會變假綠）。
strip_ansi() {
  local in="$1" out="$2" esc=$'\033' rc=0
  LC_ALL=C sed "s/${esc}\[[0-9;]*m//g" "$in" >"$out" 2>/dev/null || rc=$?
  if [ "$rc" -ne 0 ] || [ "$(wc -l <"$out")" != "$(wc -l <"$in")" ]; then
    cp "$in" "$out"
  fi
}

# run_decide <script> <log> [args…] — 回傳 rc（rc 只從這裡讀，不掛在管線後）
run_decide() {
  local script="$1" log="$2" rc=0
  shift 2
  bash "$script" "$@" >"$log.raw" 2>&1 || rc=$?
  strip_ansi "$log.raw" "$log"
  printf '%s' "$rc"
}

# count_checks <log> — 真的印出結論的安全檢查數（三項的結論行）
count_checks() { grep -cE '(Experiment status:|Has candidate metrics|Reason provided)' "$1" || true; }
count_started() { grep -c 'Running safety checks' "$1" || true; }

write_fixture() { # <repo 根> <id> <status> <有 candidate metrics=1/0>
  local root="$1" id="$2" status="$3" has_cand="$4"
  if [ "$has_cand" = "1" ]; then
    printf '{"id":"%s","status":"%s","baseline_value":1.0,"candidate_value":1.5,"target_agent_id":"fixture"}\n' \
      "$id" "$status" >"$root/data/state/experiments/$id.json"
  else
    printf '{"id":"%s","status":"%s","target_agent_id":"fixture"}\n' \
      "$id" "$status" >"$root/data/state/experiments/$id.json"
  fi
}

write_fixture "$FAKE" exp-ok accepted 1
write_fixture "$FAKE" exp-pending pending 1

# assert_happy <rc> <log> — ② 的核心判準：三項全跑 ＋ 正確總結 ＋ dry-run 預覽 ＋ rc=0
assert_happy() {
  local rc="$1" log="$2"
  [ "$rc" -eq 0 ] || return 1
  [ "$(count_checks "$log")" -eq 3 ] || return 1
  grep -Fq 'Safety: 3/3 checks passed' "$log" || return 1
  grep -Fq '=== DRY RUN ===' "$log" || return 1
  grep -Fq 'Would execute:' "$log" || return 1
  if grep -Fq 'Warning: Not all safety checks passed' "$log"; then return 1; fi
  return 0
}

extract_fn() { awk '/^check_promotion_safety\(\) \{/ {go=1} go {print} go && /^\}/ {exit}' "$1"; }

echo "── ① 抽取器自我斷言 ＋ 真檔靜態契約 ────────────────────────────"
REAL_SUM="$(shasum -a 256 "$SRC" | awk '{print $1}')"
COPY_SUM="$(shasum -a 256 "$FAKE/scripts/openclaw/decide.sh" | awk '{print $1}')"
[ -n "$REAL_SUM" ] || fail "① shasum 沒輸出（工具缺失？）"
[ "$REAL_SUM" = "$COPY_SUM" ] \
  || fail "① 副本與真檔不是逐位元組相同（$REAL_SUM vs ${COPY_SUM}）⇒ 受測的不是真檔"
grep -qx 'set -e' "$SRC" \
  || fail '① 受測腳本不再有 `set -e` ⇒ 本契約的前提（算術命令 rc≠0 會中止腳本）不成立，請先確認前提再談修法'
grep -Fq 'Safety: ${checks_passed}/${checks_total} checks passed' "$SRC" \
  || fail '① 抽不到總結行 `Safety: ${checks_passed}/${checks_total} checks passed`（錨點漂移？）'

FN="$TMP/check_promotion_safety.sh"
extract_fn "$SRC" >"$FN"
[ -s "$FN" ] || fail "① 抽不到 $SRC 的 check_promotion_safety()（函式改名或縮排改變？）—— 空集合不是通過"
grep -Fq 'checks_total=$((checks_total + 1))' "$FN" \
  || fail '① 受測計數不是賦值形式（checks_total=$((checks_total + 1))）⇒ 修前形狀回來了（FU-20261016-30 回歸？）'
grep -Fq 'checks_passed=$((checks_passed + 1))' "$FN" \
  || fail '① 受測計數不是賦值形式（checks_passed=$((checks_passed + 1))）⇒ 修前形狀回來了（FU-20261016-30 回歸？）'
BARE="$(grep -nE "$BARE_RE" "$SRC" || true)"
if [ -n "$BARE" ]; then
  printf '%s\n' "$BARE" >&2
  fail "① scripts/openclaw/decide.sh 內又出現裸算術自增（set -e 下會回非 0 ⇒ 假紅）：$(printf '%s' "$BARE" | tr '\n' ' ')"
fi
ok "① 副本 sha256 相同；真檔無裸算術自增；check_promotion_safety() 抽到 $(wc -l <"$FN" | tr -d ' ') 行且計數為賦值形式"

echo "── ② 正常路徑（--dry-run）⇒ 三項檢查全跑 ＋ 總結 ＋ 預覽 ────────"
L2="$TMP/happy.log"
RC2="$(run_decide "$FAKE/scripts/openclaw/decide.sh" "$L2" --promote exp-ok --reason "happy path" --dry-run)"
assert_happy "$RC2" "$L2" || {
  log_show "$L2"
  fail "② 沒有跑完三項檢查並給出正確總結／預覽（rc=${RC2}，實際印出結論 $(count_checks "$L2")/3 項）——『第一項檢查之前就中止』的假紅（FU-20261016-30 未修？）"
}
ok "② rc=${RC2}，三項檢查全跑，總結逐字『Safety: 3/3 checks passed』，dry-run 預覽已印"

echo "── ③ 一項檢查失敗 ⇒ 三項照跑 ＋ 總結 ＋ rc=1（真錯誤不得被忽略）───"
L3="$TMP/pending.log"
RC3="$(run_decide "$FAKE/scripts/openclaw/decide.sh" "$L3" --promote exp-pending --reason "pending path" --dry-run)"
[ "$RC3" -eq 1 ] || { log_show "$L3"; fail "③ 一項檢查未過卻不是 rc=1（rc=${RC3}）——安全閘門被放寬了"; }
[ "$(count_checks "$L3")" -eq 3 ] || { log_show "$L3"; fail "③ 有檢查沒跑完（$(count_checks "$L3")/3）——仍然是『跑不完全部檢查』"; }
negproof_expect_output 'Safety: 2/3 checks passed' "$L3" "③ 總結必須說出實情（2/3）" || { log_show "$L3"; exit 1; }
negproof_expect_output 'Warning: Not all safety checks passed' "$L3" "③ 未過的檢查必須出聲" || { log_show "$L3"; exit 1; }
ok "③ rc=${RC3}（fail-closed 不變），三項全跑，總結逐字『Safety: 2/3 checks passed』"

echo "── ④ CLI 介面未受影響：--help rc=0／未知選項 rc=1 ─────────────"
L4="$TMP/help.log"
RC4="$(run_decide "$FAKE/scripts/openclaw/decide.sh" "$L4" --help)"
[ "$RC4" -eq 0 ] || { log_show "$L4"; fail "④ --help 不是 rc=0（rc=${RC4}）"; }
negproof_expect_output 'Usage: ' "$L4" "④ --help 必須印用法" || { log_show "$L4"; exit 1; }
L5="$TMP/bogus.log"
negproof_expect 1 "④ 未知選項必須 rc=1" "$L5" -- bash "$FAKE/scripts/openclaw/decide.sh" --bogus || { log_show "$L5"; exit 1; }
ok "④ --help rc=0（含用法）；--bogus rc=1"

echo "── ⑤ mutation 自證：六處計數全改回裸形式 ⇒ 假紅復現 ＋ ② 變紅 ──"
MUT1="$TMP/mut1"
mkdir -p "$MUT1/scripts/openclaw" "$MUT1/data/state/experiments"
sed -e 's/checks_total=\$((checks_total + 1))/((checks_total++))/g' \
    -e 's/checks_passed=\$((checks_passed + 1))/((checks_passed++))/g' \
    "$FAKE/scripts/openclaw/decide.sh" >"$MUT1/scripts/openclaw/decide.sh"
write_fixture "$MUT1" exp-ok accepted 1
CH1="$(diff "$FAKE/scripts/openclaw/decide.sh" "$MUT1/scripts/openclaw/decide.sh" | grep -c '^[<>]' || true)"
[ "${CH1:-0}" -eq 12 ] \
  || fail "⑤ mutation 沒有真的改到六處計數（diff 行數=${CH1:-0}，期望 12）⇒ 突變是裝飾品"
L6="$TMP/mut1.log"
RC6="$(run_decide "$MUT1/scripts/openclaw/decide.sh" "$L6" --promote exp-ok --reason "mutation" --dry-run)"
[ "$(count_checks "$L6")" -eq 0 ] || { log_show "$L6"; fail "⑤ 突變後竟然印出檢查結論（$(count_checks "$L6") 項）⇒ 突變沒打在行為行"; }
if grep -Fq 'Safety:' "$L6"; then log_show "$L6"; fail "⑤ 突變後竟然印出總結 ⇒ 突變沒打到行為行"; fi
if grep -Fq '=== DRY RUN ===' "$L6"; then log_show "$L6"; fail "⑤ 突變後竟然印出 dry-run 預覽"; fi
[ "$RC6" -ne 0 ] || { log_show "$L6"; fail "⑤ 突變後 rc=0（期望非 0 的假紅）"; }
if assert_happy "$RC6" "$L6"; then
  fail "⑤ ②的判準對突變後（假紅）的結果竟然判「通過」⇒ 本測試沒有鑑別力"
fi
ok "⑤ 改回裸形式 ⇒ 一項檢查都沒跑、無總結、無預覽、rc=${RC6}（假紅復現），且 ② 的判準在此**變紅**"

echo "── ⑥ mutation（第二形狀）：只改回 checks_passed ⇒ 只跑 1/3 項 ──"
MUT2="$TMP/mut2"
mkdir -p "$MUT2/scripts/openclaw" "$MUT2/data/state/experiments"
sed -e 's/checks_passed=\$((checks_passed + 1))/((checks_passed++))/g' \
    "$FAKE/scripts/openclaw/decide.sh" >"$MUT2/scripts/openclaw/decide.sh"
write_fixture "$MUT2" exp-ok accepted 1
CH2="$(diff "$FAKE/scripts/openclaw/decide.sh" "$MUT2/scripts/openclaw/decide.sh" | grep -c '^[<>]' || true)"
[ "${CH2:-0}" -eq 6 ] \
  || fail "⑥ mutation 沒有真的改到三處 checks_passed（diff 行數=${CH2:-0}，期望 6）⇒ 突變是裝飾品"
L7="$TMP/mut2.log"
RC7="$(run_decide "$MUT2/scripts/openclaw/decide.sh" "$L7" --promote exp-ok --reason "mutation2" --dry-run)"
[ "$(count_checks "$L7")" -eq 1 ] \
  || { log_show "$L7"; fail "⑥ 只改回 checks_passed 卻跑了 $(count_checks "$L7")/3 項（期望 1：Check 1 印完即中止）⇒ 突變沒打在行為行"; }
[ "$RC7" -ne 0 ] || { log_show "$L7"; fail "⑥ rc=0（期望非 0）"; }
ok "⑥ 只改回 checks_passed ⇒ 只跑 1/3 項、rc=${RC7}（每一處自增都是致命點，不是只修 checks_total 就好）"

echo "ALL PASS: decide.sh 判定機制契約（FU-20261016-30：三項安全檢查跑不完、總結與 dry-run 預覽不印、rc=1 的假紅）"
