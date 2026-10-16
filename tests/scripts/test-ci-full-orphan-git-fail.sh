#!/usr/bin/env bash
# ============================================================
# test-ci-full-orphan-git-fail.sh — `make ci-full` 的 orphan artifact check 契約測試：
#   「工具失敗」不得被當成「真的沒有 orphan artifact」
# ------------------------------------------------------------
# 【來源】FU-20261016-25。orphan check 舊版三處以「命令替換 git ls-files」當迴圈清單，
#   git 失敗（不在 work tree／git 缺失／被 shim 擋掉）時命令替換是**空集合**，與
#   「真的沒有 orphan artifact」逐字相同 ⇒ 迴圈 body 一次都沒進去，收尾照印 `✅` 且
#   **rc=0**（閘門說謊；與 FU-20261016-21「git 失敗被當成空集合」同根因）。
#   同一支 recipe 還有**同一形狀的第二個缺口**：`ft=$(file …)` 在沒有 file(1) 的環境是空
#   字串 ⇒ 二進位偵測整段靜默失效、照樣印 ✅（本測試 ⑦ 一併釘住）。
#   修前逐字原文（真檔 recipe ＋ 非 git 目錄；2026-10-16 本車道實跑）：
#     $ cd <非 git 目錄> && make -f <recipe fixture> orphan-check
#       → orphan artifact check
#     fatal: not a git repository (or any of the parent directories): .git   （×3）
#         ✅
#     rc=0
#
# 【為什麼是「抽取真檔 recipe」而不是抄一份】抄一份的話，Makefile 改了、測試還在驗舊行為
#   —— 證明了自己，沒有證明閘門（同 test-ci-slow-zero-counter.sh／test-ci-timeout-tri-state.sh）。
#   抽段錨點＝`@echo "  → orphan artifact check"`（全檔唯一）到其後第一個 `@echo "    ✅"`。
#
# 【覆蓋】
#   ① 真 git、但 CWD 不是 repo ⇒ 必紅（修前 rc=0 ＋ ✅）
#   ② PATH 注入失敗的 git（rc=128 由 shim 決定）⇒ 必紅，且訊息同一行帶 rc=128
#   ③ PATH 內完全沒有 git（rc=127）⇒ 必紅，且訊息同一行帶 rc=127
#   ④ 正常 repo 且沒有 orphan ⇒ 綠 ＋ 照印 ✅（不得誤紅）
#   ⑤ tracked *.pid／*.out ⇒ 紅 ＋ 逐項指名 [PID]／[COVERAGE]（既有行為，未放寬）
#   ⑥ tracked 二進位（/bin/ls）⇒ 紅 ＋ [BINARY]（既有行為，未放寬）
#   ⑦ file(1) 不在 PATH（真 repo、git 正常）⇒ 必紅 ＋「找不到 file(1)」（修前 rc=0 ＋ ✅）
#   ⑧ mutation 自證：(a) 拿掉 git 的 fail-closed 判定 ⇒ ① 的假綠復現，且 ① 的判準在該結果上
#      **變紅**；(b) 拿掉 file(1) 存在守衛 ⇒ ⑦ 的假綠復現，且 ⑦ 的判準在該結果上**變紅**
#      ⇒ 兩個判準都不是「我們的新碼剛好會印這些字」的裝飾品。
#
# 執行： bash tests/scripts/test-ci-full-orphan-git-fail.sh
#   hermetic：只用 mktemp 目錄 ＋ PATH stub；不連網、不改真檔、不碰 production。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MAKEFILE="${ROOT}/Makefile"

# 共用負向證明庫（精確 rc 斷言、同一行雙 needle 斷言、log 縮排輸出）
# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "${ROOT}/scripts/ci/negative-proof-lib.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "  ✅ $*"; }
log_show() { negproof_show_log "${1:-}"; }

TMP="$(mktemp -d "${TMPDIR:-/tmp}/ci-full-orphan.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT

# GIT_* 洩漏會讓 fixture 的 git 呼叫落到呼叫者的 repo（#1927 同款坑）
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX GIT_NAMESPACE

command -v git >/dev/null 2>&1 || fail "本測試需要 git"
command -v file >/dev/null 2>&1 || fail "本測試需要 file(1)"
MK="$(command -v make)"
[ -n "${MK}" ] || fail "本測試需要 make"

# ── 抽取真檔 recipe ──────────────────────────────────────────────────────
TAB="$(printf '\t')"
START="${TAB}@echo \"  → orphan artifact check\""
STOP="${TAB}@echo \"    ✅\""
RECIPE="${TMP}/orphan-recipe.mk"
awk -v s="${START}" -v e="${STOP}" \
  '!inr && $0 == s { inr = 1 } inr { print; if ($0 == e) exit }' "${MAKEFILE}" >"${RECIPE}"

# 抽取器自我斷言（空集合／錨點漂移／守衛被拿掉 ⇒ 立刻紅；否則本測試會「證明空氣」）
[ -s "${RECIPE}" ] || fail "抽不到 Makefile 的 orphan artifact check 段（錨點改名？）—— 空集合不是通過"
grep -Fq -- '|| git_rc=$$?' "${RECIPE}" \
  || fail "orphan check 沒有顯式接 git 的 rc（FU-20261016-25 回歸？）"
grep -Fq -- 'if [ $$git_rc -ne 0 ]; then' "${RECIPE}" \
  || fail "orphan check 沒有『git 失敗 ⇒ fail loud』的判定（FU-20261016-25 回歸？）"
grep -Fq -- 'command -v file' "${RECIPE}" \
  || fail "orphan check 沒有 file(1) 存在性守衛（FU-20261016-25 回歸？）"
grep -Fq -- 'for f in $$TRACKED; do' "${RECIPE}" \
  || fail "orphan check 的二進位迴圈不吃抽取出來的清單（抽取範圍錯？）"
NLINE="$(wc -l <"${RECIPE}" | tr -d ' ')"
ok "抽取到真檔 orphan check recipe（${NLINE} 行）：顯式 git rc ＋ fail-closed ＋ file(1) 守衛"

# ── fixture 工廠 ─────────────────────────────────────────────────────────
# make_fixture <dir> [<recipe 檔>] — 真檔 recipe 包成一個可跑的 target
make_fixture() {
  local d="$1" recipe="${2:-${RECIPE}}"
  mkdir -p "${d}"
  printf 'orphan-check:\n' >"${d}/Makefile"
  cat "${recipe}" >>"${d}/Makefile"
}

# init_repo <dir> — 真 git repo（附 own-repo 斷言：GIT_* 洩漏時立刻失敗，不污染呼叫者）
init_repo() {
  local d="$1"
  (
    cd "${d}" || exit 1
    git init -q -b main . &&
      git config user.email ci@test.invalid &&
      git config user.name ci-test
  ) || fail "fixture repo 建立失敗：${d}"
  [ -d "${d}/.git" ] || fail "fixture repo 沒有 .git（GIT_* 洩漏？）—— 拒絕繼續"
  [ "$(cd "${d}" && git rev-parse --show-toplevel)" = "$(cd "${d}" && pwd -P)" ] \
    || fail "fixture repo 沒有解析到自己（GIT_* 洩漏？）"
}

# commit_all <dir> <訊息> — fixture 內提交（身分已由 init_repo 設好）
commit_all() {
  local d="$1" msg="$2"
  ( cd "${d}" && git add -A && git commit -qm "${msg}" ) || fail "fixture commit 失敗：${d}"
}

# run_recipe <dir> <log> [<PATH 值>] — 在 fixture 執行 target；rc 一律從這裡讀（不經管線）
#   PATH 值省略 ⇒ 用呼叫者的 PATH；給空字串 ⇒ PATH 完全空（模擬 git 不存在）
run_recipe() {
  local d="$1" log="$2" rc=0
  if [ "$#" -ge 3 ]; then
    ( cd "${d}" && env PATH="$3" "${MK}" -f Makefile orphan-check ) >"${log}" 2>&1 || rc=$?
  else
    ( cd "${d}" && "${MK}" -f Makefile orphan-check ) >"${log}" 2>&1 || rc=$?
  fi
  printf '%s' "${rc}"
}

# assert_fail_closed <rc> <log> <needle> — 「工具失敗」必須是**非 0** ＋ 指名訊息 ＋
#   **不得**印通過的 ✅。⑧ 拿這組判準去驗突變後的假綠結果，證明它有鑑別力。
assert_fail_closed() {
  local rc="$1" log="$2" needle="$3"
  [ "${rc}" -ne 0 ] || return 1
  grep -Fq -- "${needle}" "${log}" || return 1
  if grep -Fxq -- '    ✅' "${log}"; then return 1; fi
  return 0
}

echo "── ① 真 git、但 CWD 不是 repo ⇒ 必紅（修前 rc=0 ＋ 印 ✅）────────────"
D1="${TMP}/c1"; make_fixture "${D1}"
L1="${TMP}/c1.log"
RC1="$(run_recipe "${D1}" "${L1}")"
assert_fail_closed "${RC1}" "${L1}" 'git ls-files 失敗' || {
  log_show "${L1}"
  fail "① 不在 git repo 時沒有 fail loud（rc=${RC1}）——『清單取不到』被當成『沒有 orphan』"
}
ok "① 非 repo ⇒ rc=${RC1}（非 0）、訊息指名 git ls-files 失敗、且不印 ✅"

echo "── ② PATH 注入失敗的 git（rc=128）⇒ 必紅 ＋ 同一行帶 rc ────────────"
SHIM128="${TMP}/shim128"; mkdir -p "${SHIM128}"
printf '#!/bin/sh\necho "fatal: simulated git failure" >&2\nexit 128\n' >"${SHIM128}/git"
chmod +x "${SHIM128}/git"
# seam 的鑑別力斷言：這個 shim 真的讓 git ls-files 回 128（不是 no-op）
seam=0; ( cd "${TMP}" && PATH="${SHIM128}:${PATH}" git ls-files ) >/dev/null 2>&1 || seam=$?
[ "${seam}" -eq 128 ] || fail "seam: shim 的 git ls-files 應 exit 128，實得 ${seam}"
D2="${TMP}/c2"; make_fixture "${D2}"
L2="${TMP}/c2.log"
RC2="$(run_recipe "${D2}" "${L2}" "${SHIM128}:${PATH}")"
assert_fail_closed "${RC2}" "${L2}" 'git ls-files 失敗' || {
  log_show "${L2}"
  fail "② git 以 128 失敗時沒有 fail loud（rc=${RC2}）"
}
negproof_expect_output_line 'git ls-files 失敗' 'rc=128' "${L2}" \
  "② 訊息必須同一行帶上 git 的 rc（否則診斷不出是哪一種失敗）" || { log_show "${L2}"; exit 1; }
ok "② 失敗的 git ⇒ rc=${RC2}（非 0），訊息含 rc=128"

echo "── ③ PATH 內完全沒有 git（rc=127）⇒ 必紅 ＋ 同一行帶 rc ────────────"
D3="${TMP}/c3"; make_fixture "${D3}"
L3="${TMP}/c3.log"
RC3="$(run_recipe "${D3}" "${L3}" "")"
assert_fail_closed "${RC3}" "${L3}" 'git ls-files 失敗' || {
  log_show "${L3}"
  fail "③ git 不存在時沒有 fail loud（rc=${RC3}）"
}
negproof_expect_output_line 'git ls-files 失敗' 'rc=127' "${L3}" \
  "③ 訊息必須同一行帶上 127（工具缺失），否則與『沒有 orphan』無從區分" || { log_show "${L3}"; exit 1; }
ok "③ git 不存在 ⇒ rc=${RC3}（非 0），訊息含 rc=127"

echo "── ④ 正常 repo（無 orphan）⇒ 綠 ＋ 照印 ✅ ─────────────────────────"
D4="${TMP}/c4"; make_fixture "${D4}"; init_repo "${D4}"
printf 'plain text\n' >"${D4}/a.txt"
commit_all "${D4}" init
L4="${TMP}/c4.log"
RC4="$(run_recipe "${D4}" "${L4}")"
[ "${RC4}" -eq 0 ] || { log_show "${L4}"; fail "④ 沒有 orphan 的正常 repo 卻紅（rc=${RC4}）—— 不得把正常路徑染紅"; }
negproof_expect_output '    ✅' "${L4}" \
  "④ 正常情況仍必須印通過的 ✅（新增守衛不得讓正常路徑消失）" || { log_show "${L4}"; exit 1; }
ok "④ 正常 repo（無 orphan）⇒ rc=0 ＋ 印 ✅"

echo "── ⑤ tracked *.pid／*.out ⇒ 紅 ＋ 逐項指名 ─────────────────────────"
D5="${TMP}/c5"; make_fixture "${D5}"; init_repo "${D5}"
printf '123\n' >"${D5}/server.pid"
printf 'x\n' >"${D5}/cover.out"
commit_all "${D5}" init
L5="${TMP}/c5.log"
RC5="$(run_recipe "${D5}" "${L5}")"
[ "${RC5}" -ne 0 ] || { log_show "${L5}"; fail "⑤ tracked *.pid／*.out 卻回綠（rc=${RC5}）—— 既有行為被放寬"; }
negproof_expect_output_line 'server.pid' '[PID]' "${L5}" "⑤ .pid 必須被指名為 [PID]" || { log_show "${L5}"; exit 1; }
negproof_expect_output_line 'cover.out' '[COVERAGE]' "${L5}" "⑤ .out 必須被指名為 [COVERAGE]" || { log_show "${L5}"; exit 1; }
ok "⑤ tracked *.pid／*.out ⇒ rc=${RC5}（非 0）＋ [PID]／[COVERAGE] 指名"

echo "── ⑥ tracked 二進位 ⇒ 紅 ＋ [BINARY] ─────────────────────────────"
D6="${TMP}/c6"; make_fixture "${D6}"; init_repo "${D6}"
cp /bin/ls "${D6}/committed-binary"
commit_all "${D6}" init
L6="${TMP}/c6.log"
RC6="$(run_recipe "${D6}" "${L6}")"
[ "${RC6}" -ne 0 ] || { log_show "${L6}"; fail "⑥ tracked 二進位卻回綠（rc=${RC6}）—— 既有行為被放寬"; }
negproof_expect_output_line 'committed-binary' '[BINARY]' "${L6}" "⑥ 二進位必須被指名為 [BINARY]" || { log_show "${L6}"; exit 1; }
ok "⑥ tracked 二進位 ⇒ rc=${RC6}（非 0）＋ [BINARY] 指名"

echo "── ⑦ file(1) 不在 PATH（真 repo、git 正常）⇒ 必紅 ────────────────"
GITONLY="${TMP}/gitonly"; mkdir -p "${GITONLY}"
ln -s "$(command -v git)" "${GITONLY}/git"
# seam 的鑑別力斷言：這個 PATH 內真的有 git、且真的沒有 file(1)
( export PATH="${GITONLY}"; command -v git >/dev/null 2>&1 ) || fail "seam: gitonly PATH 內找不到 git"
if ( export PATH="${GITONLY}"; command -v file >/dev/null 2>&1 ); then
  fail "seam: gitonly PATH 內竟然找得到 file(1) ⇒ ⑦ 沒有鑑別力"
fi
D7="${TMP}/c7"; make_fixture "${D7}"; init_repo "${D7}"
printf 'plain text\n' >"${D7}/a.txt"
commit_all "${D7}" init
L7="${TMP}/c7.log"
RC7="$(run_recipe "${D7}" "${L7}" "${GITONLY}")"
assert_fail_closed "${RC7}" "${L7}" '找不到 file(1)' || {
  log_show "${L7}"
  fail "⑦ 沒有 file(1) 時沒有 fail loud（rc=${RC7}）—— 二進位偵測靜默失效、照印 ✅"
}
ok "⑦ 無 file(1) ⇒ rc=${RC7}（非 0）＋「找不到 file(1)」、且不印 ✅"

echo "── ⑧ mutation 自證：守衛拿掉 ⇒ ①②⑦ 的判準必須失效 ────────────────"
MUT1="${TMP}/mut1.mk"
sed 's/\$\$git_rc -ne 0/0 -ne 0/' "${RECIPE}" >"${MUT1}"
CH1="$(diff "${RECIPE}" "${MUT1}" | grep -c '^[<>]' || true)"
[ "${CH1:-0}" -eq 2 ] || fail "⑧a mutation 沒有真的改到 fail-closed 判定（diff 行數=${CH1:-0}，期望 2）⇒ 突變是裝飾品"
D8="${TMP}/c8"; make_fixture "${D8}" "${MUT1}"
L8="${TMP}/c8.log"
RC8="$(run_recipe "${D8}" "${L8}")"
[ "${RC8}" -eq 0 ] || { log_show "${L8}"; fail "⑧a 拿掉 fail-closed 判定後非 repo 竟仍非 0（rc=${RC8}）⇒ 突變沒改到判準"; }
negproof_expect_output '    ✅' "${L8}" \
  "⑧a 突變後必須重現『清單取不到仍印 ✅』這條假綠路徑" || { log_show "${L8}"; exit 1; }
if assert_fail_closed "${RC8}" "${L8}" 'git ls-files 失敗'; then
  fail "⑧a ①的判準對突變後（假綠）的結果竟然判通過 ⇒ 本測試沒有鑑別力"
fi
ok "⑧a 拿掉 fail-closed 判定 ⇒ rc=0 ＋ ✅（false-green 復現），且 ① 的判準在此變紅"

MUT2="${TMP}/mut2.mk"
sed 's/command -v file >/true >/' "${RECIPE}" >"${MUT2}"
CH2="$(diff "${RECIPE}" "${MUT2}" | grep -c '^[<>]' || true)"
[ "${CH2:-0}" -eq 2 ] || fail "⑧b mutation 沒有真的改到 file(1) 守衛（diff 行數=${CH2:-0}，期望 2）⇒ 突變是裝飾品"
D9="${TMP}/c9"; make_fixture "${D9}" "${MUT2}"; init_repo "${D9}"
printf 'plain text\n' >"${D9}/a.txt"
commit_all "${D9}" init
L9="${TMP}/c9.log"
RC9="$(run_recipe "${D9}" "${L9}" "${GITONLY}")"
[ "${RC9}" -eq 0 ] || { log_show "${L9}"; fail "⑧b 拿掉 file(1) 守衛後無 file 竟仍非 0（rc=${RC9}）⇒ 突變沒改到判準"; }
negproof_expect_output '    ✅' "${L9}" \
  "⑧b 突變後必須重現『沒有 file(1) 仍印 ✅』這條假綠路徑" || { log_show "${L9}"; exit 1; }
if assert_fail_closed "${RC9}" "${L9}" '找不到 file(1)'; then
  fail "⑧b ⑦的判準對突變後（假綠）的結果竟然判通過 ⇒ 本測試沒有鑑別力"
fi
ok "⑧b 拿掉 file(1) 守衛 ⇒ rc=0 ＋ ✅（false-green 復現），且 ⑦ 的判準在此變紅"

echo "ALL PASS: ci-full orphan check 的『工具失敗 ≠ 沒有 orphan』契約（FU-20261016-25）"
