#!/usr/bin/env bash
# ============================================================
# test-local-ci-completes-all-checks.sh — `scripts/ci/local-ci.sh` 的「跑完全部檢查才總結」契約測試
# ------------------------------------------------------------
# 【來源】FU-20261016-28（缺陷 ①）：`scripts/ci/local-ci.sh:41,44` 的計數用 `((PASS++))` /
#   `((FAIL++))`。在 `set -euo pipefail` 下，**算術命令的結束碼是「運算式是否為 0」** ⇒ `PASS=0`
#   時的 `((PASS++))` 回傳 **1**（post-increment 的運算式值是舊值 0）⇒ shell 立刻結束。
#   ⇒ 第一支**通過**的檢查之後整個腳本中止：後續檢查不跑、總結（`🎉 全部 N 項檢查通過` /
#   `💥 M 項檢查失敗`）永遠不印、rc=1。這是**假紅**（通過的檢查被報成失敗），與本批已修的假綠
#   同屬「判定機制」病灶：假紅會誘導「無條件重跑」。
#   修前逐字原文（真檔，2026-10-16 本車道實跑；log 見本票驗證段）：
#     $ cd <lane> && bash scripts/ci/local-ci.sh --quick
#     ▶ gofmt formatting
#     ✅ gofmt formatting passed
#     rc=1        ← 第 2 支（go build）之後的 5 支檢查連跑都沒跑
#
# 【為什麼是「逐字抽取真檔」而不是抄一份】抄一份的話，真檔改回 `((PASS++))` 時測試還在驗舊行為
#   —— 證明了自己，沒有證明閘門（同 tests/scripts/test-ci-slow-zero-counter.sh 的理由）。
#   本測試逐字抽出真檔的 `run_check()` 函式與 `# ── Summary ──` 之後的總結區塊，放進 hermetic
#   driver 執行（driver 只提供 `PASS=0`／`FAIL=0`／空的顏色變數與 4 個受控檢查）。
#
# 【覆蓋】
#   ① 抽不到函式／計數不是賦值形式／總結的失敗分支不見了 ⇒ 立刻紅（抽取器自我斷言，不驗空氣）
#   ② 4 支檢查（1 支失敗）⇒ **4 支都跑**才總結；rc=1；總結逐字 `💥 1 項檢查失敗，3 項通過`
#   ③ 4 支檢查全通過 ⇒ rc=0；總結逐字 `🎉 全部 4 項檢查通過`
#   ④ 真檔靜態契約：`scripts/ci/local-ci.sh` 內不得再出現裸 `((VAR++))` 形狀（返祖守衛）
#   ⑤ mutation 自證（判準要有鑑別力）：把兩處計數改回修前的 `((PASS++))` ⇒ **假紅復現**
#      （只跑到第 1 支、無總結、rc=1），且 ② 的判準在該結果上**變紅** ⇒ ② 不是裝飾品
#
# 執行： bash tests/scripts/test-local-ci-completes-all-checks.sh
#   hermetic：只用 mktemp ＋ 受控的 `true`／`false` 檢查；不跑真的 gofmt/go build、不連網、不改真檔。
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC="$REPO/scripts/ci/local-ci.sh"

# 共用負向證明庫（精確 rc 斷言；同其他 tests/scripts/*.sh）
# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "$REPO/scripts/ci/negative-proof-lib.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "  ✅ $*"; }
log_show() { negproof_show_log "${1:-}"; }

TMP="$(mktemp -d "${TMPDIR:-/tmp}/fu28-localci.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

NCHECKS=4

negproof_require_file "$SRC" "受測檔案 scripts/ci/local-ci.sh" || exit 1

# ── 逐字抽取真檔的兩個區塊 ───────────────────────────────────────────────
extract_run_check() { awk '/^run_check\(\) \{/ {go=1} go {print} go && /^\}/ {exit}' "$1"; }
extract_summary() { awk '/^# .*Summary/ {go=1} go {print}' "$1"; }

RUN_CHECK="$TMP/run_check.sh"
SUMMARY="$TMP/summary.sh"
extract_run_check "$SRC" >"$RUN_CHECK"
extract_summary "$SRC" >"$SUMMARY"

# 抽取器自我斷言（空集合／錨點漂移 ⇒ 立刻紅，否則本測試會「證明空氣」）
[ -s "$RUN_CHECK" ] || fail "抽不到 $SRC 的 run_check() 函式（函式改名或縮排改變？）—— 空集合不是通過"
[ -s "$SUMMARY" ] || fail "抽不到 $SRC 的 Summary 區塊（註解錨點漂移？）—— 空集合不是通過"
grep -Fq 'PASS=$((PASS + 1))' "$RUN_CHECK" \
  || fail '受測計數不是賦值形式（PASS=$((PASS + 1))）⇒ 修前形狀回來了（FU-20261016-28 回歸？）'
grep -Fq 'FAIL=$((FAIL + 1))' "$RUN_CHECK" \
  || fail '受測計數不是賦值形式（FAIL=$((FAIL + 1))）⇒ 修前形狀回來了（FU-20261016-28 回歸？）'
grep -Fq 'if [ "$FAIL" -eq 0 ]; then' "$SUMMARY" \
  || fail '總結區塊抽不到 if [ "$FAIL" -eq 0 ] 判定（抽取範圍錯？）'
grep -Fq 'exit 1' "$SUMMARY" || fail "總結區塊的失敗分支（exit 1）不見了 ⇒ 閘門被放寬了"
ok "逐字抽取真檔 run_check()（$(wc -l <"$RUN_CHECK" | tr -d ' ') 行）＋ Summary 區塊（$(wc -l <"$SUMMARY" | tr -d ' ') 行）"

# ── driver 工廠 ─────────────────────────────────────────────────────────
# make_driver <輸出檔> <run_check 檔> <summary 檔> <逐支檢查的 rc 清單（1=通過 0=失敗）>
#   driver 只提供真檔依賴的四個全域（PASS／FAIL／顏色），其餘（run_check／總結）皆為真檔原文。
make_driver() {
  local out="$1" rc_file="$2" sm_file="$3"
  shift 3
  {
    printf '#!/usr/bin/env bash\n'
    printf 'set -euo pipefail\n'
    printf 'RED=""; GREEN=""; YELLOW=""; BLUE=""; NC=""\n'
    printf 'PASS=0\nFAIL=0\n'
    cat "$rc_file"
    local i=0 verdict
    for verdict in "$@"; do
      i=$((i + 1))
      if [ "$verdict" = "1" ]; then
        printf 'run_check "check%s" true\n' "$i"
      else
        printf 'run_check "check%s" false\n' "$i"
      fi
    done
    cat "$sm_file"
  } >"$out"
}

# run_driver <driver> <log> — 跑 driver 並把 rc 印出來（rc 一律從這裡讀，不經管線）
run_driver() {
  local drv="$1" log="$2" rc=0
  bash "$drv" >"$log" 2>&1 || rc=$?
  printf '%s' "$rc"
}

# count_started <log> — 真的**開始跑**的檢查數（`▶ <name>` 行）
count_started() { grep -c '▶' "$1" || true; }

# assert_full_run <rc> <log> — 核心判準：4 支都跑完 **且** 總結說出實情（1 失敗 / 3 通過）
#   ⑥ 拿這個判準去驗突變後的假紅結果，證明它有鑑別力（不是「我們的新碼剛好會印這些字」）。
assert_full_run() {
  local rc="$1" log="$2"
  [ "$rc" -eq 1 ] || return 1
  [ "$(count_started "$log")" -eq "$NCHECKS" ] || return 1
  grep -Fq '❌ check3 FAILED' "$log" || return 1
  grep -Fq '1 項檢查失敗，3 項通過' "$log" || return 1
  return 0
}

echo "── ① 抽取器自我斷言（見上）─────────────────────────────────────"

echo "── ② 4 支檢查（第 3 支失敗）⇒ 4 支都跑完才總結，且總結說實情 ─────"
D1="$TMP/d1.sh"
make_driver "$D1" "$RUN_CHECK" "$SUMMARY" 1 1 0 1
L1="$TMP/d1.log"
RC1="$(run_driver "$D1" "$L1")"
assert_full_run "$RC1" "$L1" || {
  log_show "$L1"
  fail "② 沒有跑完全部 ${NCHECKS} 支並給出正確總結（rc=${RC1}，實際開始 ${NCHECKS} 支中的 $(count_started "$L1") 支）——『第一支通過後就中止』的假紅（FU-20261016-28 未修？）"
}
ok "② rc=${RC1}（有 1 支失敗 ⇒ 非 0），${NCHECKS} 支全部跑完，總結逐字『1 項檢查失敗，3 項通過』"

echo "── ③ 全部通過 ⇒ rc=0 ＋ 總結『全部 N 項檢查通過』──────────────"
D2="$TMP/d2.sh"
make_driver "$D2" "$RUN_CHECK" "$SUMMARY" 1 1 1 1
L2="$TMP/d2.log"
RC2="$(run_driver "$D2" "$L2")"
[ "$RC2" -eq 0 ] || { log_show "$L2"; fail "③ 全部通過卻非 0（rc=${RC2}）——正常路徑不得被守衛染紅"; }
[ "$(count_started "$L2")" -eq "$NCHECKS" ] || { log_show "$L2"; fail "③ 只跑了 $(count_started "$L2")/${NCHECKS} 支"; }
negproof_expect_output "🎉 全部 ${NCHECKS} 項檢查通過" "$L2" \
  "③ 全數通過時必須印總結（修前這一行永遠不印）" || { log_show "$L2"; exit 1; }
ok "③ rc=0，${NCHECKS} 支全部跑完，總結逐字相符"

echo "── ④ 真檔靜態契約：不得再有裸 ((VAR++)) 形狀 ───────────────────"
BARE="$(grep -nE '^[[:space:]]*\(\([[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]*(\+\+|--)[[:space:]]*\)\)' "$SRC" || true)"
if [ -n "$BARE" ]; then
  printf '%s\n' "$BARE" >&2
  fail "④ scripts/ci/local-ci.sh 內又出現裸算術自增（set -e 下會回非 0 ⇒ 假紅）：\n${BARE}"
fi
ok "④ scripts/ci/local-ci.sh 內 0 處裸算術自增"

echo "── ⑤ mutation 自證：改回 ((PASS++)) ⇒ ② 的判準必須變紅 ─────────"
MUT_RC="$TMP/mut_run_check.sh"
sed -e 's/PASS=\$((PASS + 1))/((PASS++))/' -e 's/FAIL=\$((FAIL + 1))/((FAIL++))/' \
  "$RUN_CHECK" >"$MUT_RC"
CH=$(diff "$RUN_CHECK" "$MUT_RC" | grep -c '^[<>]' || true)
[ "${CH:-0}" -eq 4 ] || fail "⑤ mutation 沒有真的改到兩處計數（diff 行數=${CH:-0}，期望 4）⇒ 突變是裝飾品"
D3="$TMP/d3.sh"
make_driver "$D3" "$MUT_RC" "$SUMMARY" 1 1 0 1
L3="$TMP/d3.log"
RC3="$(run_driver "$D3" "$L3")"
# 突變後必須重現**修前行為**：第 1 支通過後整體中止、無總結、rc=1（假紅）
[ "$(count_started "$L3")" -eq 1 ] || { log_show "$L3"; fail "⑤ 突變後竟然跑了 $(count_started "$L3") 支（期望 1）⇒ 突變沒打在行為行"; }
if grep -Fq '項檢查' "$L3"; then log_show "$L3"; fail "⑤ 突變後竟然印出總結 ⇒ 突變沒打到行為行"; fi
[ "$RC3" -ne 0 ] || { log_show "$L3"; fail "⑤ 突變後 rc=0（期望非 0 的假紅）"; }
if assert_full_run "$RC3" "$L3"; then
  fail "⑤ ②的判準對突變後（假紅）的結果竟然判「通過」⇒ 本測試沒有鑑別力"
fi
ok "⑤ 改回 ((PASS++)) ⇒ 只跑 1 支、無總結、rc=${RC3}（假紅復現），且 ② 的判準在此**變紅**"

echo "ALL PASS: local-ci.sh 判定機制契約（FU-20261016-28 缺陷①：只有第一支檢查會跑）"
