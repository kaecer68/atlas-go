#!/usr/bin/env bash
# test-negative-proofs.sh — 「負向證明」自我測試：證明**它不會把 127/2 當成「擋下了」**（issue #2011）
#
# 背景：舊的負向證明寫成 `if <受測命令>; then echo "❌ 沒擋下"; exit 1; fi; echo "✅ 擋下了"`，
# 把「非 0」等同於「擋下了」⇒ 受測腳本被改名（127）或參數寫錯（2）時**照樣印 ✅**。
# 本測試把「測試本身有沒有鑑別力」變成可執行斷言：
#
#   1. 靜態接線：quality.yml 必須呼叫兩支負向證明腳本，且不得回到 inline 的假綠寫法。
#   2. 斷言庫單元：negproof_expect 對 rc=0/2/124/127 一律判「測試失敗」，只有恰好等於期望值才通過；
#      空值（期望 rc 空字串／比對字串空字串）不得被當成通過。
#   3. 端到端（hermetic throwaway repo，在 $TMPDIR，**不在 repo 內**）：
#      對兩支負向證明各餵四種情境 →「真命令(1)」必須綠、「命令不存在(127)／用法錯誤(2)／根本沒擋(0)」必須紅；
#      另驗「受測腳本不存在」時的前置檢查 fail-closed，以及跑完後 index 必須還原。
#
# ⚠️ git hook 會 export GIT_DIR/GIT_WORK_TREE（SOP ★37 / #1927）：fixture 的 git 命令會打到
#    呼叫端 repo（2026-09-23 曾把呼叫端分支改寫）。本檔先 unset 全部 GIT_*，並在建立 fixture 後
#    斷言它真的是自己的 repo root。
set -uo pipefail

unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LIB="$ROOT/scripts/ci/negative-proof-lib.sh"
SS_PROOF="scripts/ci/secret-scan-negative-proof.sh"
MS_PROOF="scripts/ci/monitoring-single-source-negative-proof.sh"
TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/atlas-negproof-selftest.XXXXXX")"
trap 'rm -rf "${TMP_ROOT}"' EXIT
pass=0; fail=0
ok()  { echo "  ✅ $1"; pass=$((pass+1)); }
bad() { echo "  ❌ $1"; fail=$((fail+1)); }

echo "── 負向證明自我測試（精確 exit code）──"

# ── 1. 靜態接線 ───────────────────────────────────────────────────────────
WF="$ROOT/.github/workflows/quality.yml"
if grep -qF "bash scripts/ci/secret-scan-negative-proof.sh" "$WF" \
   && grep -qF "bash scripts/ci/monitoring-single-source-negative-proof.sh" "$WF"; then
  ok "W1 quality.yml 以腳本呼叫兩支負向證明（可被本測試覆蓋）"
else
  bad "W1 quality.yml 沒有呼叫負向證明腳本（inline 假綠可能又回來了）"
fi
if grep -qE 'if bash scripts/(ci/check_monitoring_single_source\.sh|secret-scan\.sh); then' "$WF"; then
  bad "W2 quality.yml 仍有 inline『非 0 即算擋下』的負向證明"
else
  ok "W2 quality.yml 已無 inline『非 0 即算擋下』寫法"
fi
if grep -qF "negative-proof-lib.sh" "$ROOT/$SS_PROOF" && grep -qF "negative-proof-lib.sh" "$ROOT/$MS_PROOF"; then
  ok "W3 兩支負向證明都載入共用斷言庫"
else
  bad "W3 有負向證明沒載入共用斷言庫（兩套語意會漂移）"
fi

# ── 2. 斷言庫單元案例（直接 source，餵各種 rc）───────────────────────────
unit() {   # unit <期望 lib rc> <說明> <lib 呼叫…>
  local want="$1" desc="$2"; shift 2
  bash -c '. "$1"; shift; "$@"' _ "$LIB" "$@" >"${TMP_ROOT}/unit.log" 2>&1
  local rc=$?
  if [ "$rc" -eq "$want" ]; then ok "U ${desc}（lib rc=${rc}）"
  else bad "U ${desc}（lib rc=${rc}，期望 ${want}）"; sed 's/^/     /' "${TMP_ROOT}/unit.log" | head -3; fi
}
unit 0 "rc 恰為 1（＝擋下）→ 通過"          negproof_expect 1 d "${TMP_ROOT}/u.log" -- bash -c 'exit 1'
unit 1 "rc 恰為 2 → 測試失敗"                negproof_expect 1 d "${TMP_ROOT}/u.log" -- bash -c 'exit 2'
unit 1 "rc 恰為 127 → 測試失敗"              negproof_expect 1 d "${TMP_ROOT}/u.log" -- bash -c 'exit 127'
unit 1 "rc 恰為 126 → 測試失敗"              negproof_expect 1 d "${TMP_ROOT}/u.log" -- bash -c 'exit 126'
unit 1 "rc 恰為 124 → 測試失敗"              negproof_expect 1 d "${TMP_ROOT}/u.log" -- bash -c 'exit 124'
unit 1 "rc 為 0（根本沒擋）→ 測試失敗"        negproof_expect 1 d "${TMP_ROOT}/u.log" -- bash -c 'exit 0'
unit 1 "期望 rc 是空字串 → 測試失敗（空值不通過）" negproof_expect "" d "${TMP_ROOT}/u.log" -- bash -c 'exit 1'
unit 1 "完全沒給受測命令 → 測試失敗"          negproof_expect 1 d "${TMP_ROOT}/u.log"
unit 1 "受測檔案不存在 → fail-closed"        negproof_require_file "${TMP_ROOT}/not-there.sh" "假檔案"
unit 0 "受測檔案存在 → 通過"                 negproof_require_file "${LIB}" "斷言庫"
unit 1 "比對字串為空 → 測試失敗（空字串會永遠命中）" negproof_expect_output "" "${LIB}" d
unit 1 "比對字串沒出現 → 測試失敗"            negproof_expect_output "__no_such_marker__" "${LIB}" d

# ── 3. 端到端：hermetic throwaway repo（$TMPDIR，不在 repo 內）───────────
mk_repo() {
  local tmp tmp_real fixture_root
  tmp="$(mktemp -d "${TMP_ROOT}/repo.XXXXXX")"
  tmp_real="$(cd "$tmp" && pwd -P)"
  git init -q "$tmp"
  fixture_root="$(git -C "$tmp" rev-parse --show-toplevel 2>/dev/null || echo "")"
  [ "$fixture_root" = "$tmp_real" ] || {
    echo "FAIL: fixture 逃出自己的目錄（root='$fixture_root' tmp_real='$tmp_real'）" >&2
    exit 1
  }
  mkdir -p "$tmp/scripts/ci" "$tmp/tests/scripts"
  cp "$ROOT/scripts/secret-scan.sh"                       "$tmp/scripts/"
  [ -f "$ROOT/scripts/secret-scan-allowlist.txt" ] && cp "$ROOT/scripts/secret-scan-allowlist.txt" "$tmp/scripts/"
  cp "$ROOT/scripts/ci/check_monitoring_single_source.sh"  "$tmp/scripts/ci/"
  cp "$ROOT/scripts/ci/check_monitoring_single_source.py"  "$tmp/scripts/ci/"
  cp "$ROOT/scripts/ci/negative-proof-lib.sh"             "$tmp/scripts/ci/"
  cp "$ROOT/$SS_PROOF" "$tmp/$SS_PROOF"
  cp "$ROOT/$MS_PROOF" "$tmp/$MS_PROOF"
  git -C "$tmp" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false \
      add -A >/dev/null
  git -C "$tmp" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false \
      commit -qm base >/dev/null
  printf '%s\n' "$tmp"
}

# 用法錯誤 stub（exit 2）：模擬「參數寫錯」
printf '#!/usr/bin/env bash\necho "usage: stub" >&2\nexit 2\n' > "${TMP_ROOT}/stub-exit2.sh"

case_proof() {   # case_proof <repo> <proof 相對路徑> <期望 rc> <說明> <override（可空）> <訊息應含（可空）>
  local repo="$1" proof="$2" want="$3" desc="$4" override="${5:-}" needle="${6:-}"
  local out="${TMP_ROOT}/case.log" rc=0
  if [ -n "${override}" ]; then
    ( cd "$repo" && NEGPROOF_CHECK_CMD="${override}" bash "$repo/$proof" ) >"${out}" 2>&1 || rc=$?
  else
    ( cd "$repo" && bash "$repo/$proof" ) >"${out}" 2>&1 || rc=$?
  fi
  if [ "$rc" -ne "$want" ]; then
    bad "${desc}（proof rc=${rc}，期望 ${want}）"; sed 's/^/     /' "${out}" | head -8; return
  fi
  if [ -n "${needle}" ] && ! grep -qF -- "${needle}" "${out}"; then
    bad "${desc}（rc 正確但訊息未含 '${needle}'）"; sed 's/^/     /' "${out}" | head -8; return
  fi
  # index 必須還原：fixture 不得殘留（否則 CI 會被自己的負向證明污染）
  if git -C "$repo" ls-files --error-unmatch -- 'scripts/__negtest_secret__.py' \
       'scripts/docker-compose.__negtest__.yml' >/dev/null 2>&1; then
    bad "${desc}（跑完 fixture 仍留在 index）"; return
  fi
  ok "${desc}（proof rc=${rc}）"
}

for spec in "secret-scan|${SS_PROOF}|secret-scan 檢查" "monitoring|${MS_PROOF}|monitoring 檢查"; do
  label="${spec%%|*}"; rest="${spec#*|}"; proof="${rest%%|*}"; human="${rest#*|}"
  repo="$(mk_repo)"
  case_proof "$repo" "$proof" 0   "E1 ${human}＋真命令 → 負向證明成立"                  ""        ""
  case_proof "$repo" "$proof" 1   "E2 ${human}＋命令不存在(127) → 必須紅燈"            "/nonexistent/bin/no-such-cmd" "exit=127"
  case_proof "$repo" "$proof" 1   "E3 ${human}＋用法錯誤(2) → 必須紅燈"                "bash ${TMP_ROOT}/stub-exit2.sh" "exit=2"
  case_proof "$repo" "$proof" 1   "E4 ${human}＋根本沒擋(0) → 必須紅燈"                "true"    ""
  # 前置檢查 fail-closed：受測腳本被刪掉（不 override）
  if [ "$label" = "secret-scan" ]; then rm -f "$repo/scripts/secret-scan.sh"; else rm -f "$repo/scripts/ci/check_monitoring_single_source.sh"; fi
  case_proof "$repo" "$proof" 1   "E5 ${human}＋受測腳本被刪 → 前置檢查 fail-closed"    ""        "找不到"
  rm -rf "$repo"
done

echo ""
if [ "${fail}" -eq 0 ]; then
  echo "✅ test-negative-proofs PASS（${pass} 項）"
  exit 0
fi
echo "❌ test-negative-proofs FAIL（pass=${pass} fail=${fail}）"
exit 1
