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
#   2b. 規則層斷言單元（negproof_expect_output_line）：要求「fixture 檔名與**規則 id 同一行**」。
#       「有東西擋下」不等於「正確的規則擋下」——A 規則壞了、B 規則代擋時，只比對 fixture 檔名會誤綠。
#   3. 端到端（hermetic throwaway repo，在 $TMPDIR，**不在 repo 內**）：
#      對兩支負向證明各餵四種情境 →「真命令(1)」必須綠、「命令不存在(127)／用法錯誤(2)／根本沒擋(0)」必須紅；
#      另驗「受測腳本不存在」時的前置檢查 fail-closed，以及跑完後 index 必須還原。
#   3b. 規則層 end-to-end（每支各三情境）：①擋錯規則（rc=1、輸出含 fixture 但缺該規則 id）②正確的規則
#      被停用（永遠通過）③規則照跑但 rc 被強制 0 ⇒ 三者都必須紅燈。①是**舊寫法會誤綠**的那一類。
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

# 規則層斷言的 log fixtures（模擬真實掃描器輸出形狀）
printf '%s\n' 'denied: scripts/__negtest_secret__.py:1  [telegram_bot_token]  1234…2345' > "${TMP_ROOT}/line-both.log"
printf '%s\n' 'denied: scripts/__negtest_secret__.py:1' '[telegram_bot_token] listed elsewhere' > "${TMP_ROOT}/line-split.log"
printf '%s\n' 'denied: scripts/__negtest_secret__.py:1  [some_other_rule]' > "${TMP_ROOT}/line-wrong.log"
printf '%s\n' 'denied: scripts/__negtest_secret__.py:1' > "${TMP_ROOT}/line-fixture-only.log"
SS_FIX='scripts/__negtest_secret__.py'
unit 0 "同一行含 fixture 與規則 id → 通過"        negproof_expect_output_line "${SS_FIX}" '[telegram_bot_token]' "${TMP_ROOT}/line-both.log" d
unit 1 "fixture 與規則 id 在不同行 → 失敗（不得拼湊兩行）" negproof_expect_output_line "${SS_FIX}" '[telegram_bot_token]' "${TMP_ROOT}/line-split.log" d
unit 1 "含 fixture 但規則 id 不對 → 失敗（＝擋錯規則）" negproof_expect_output_line "${SS_FIX}" '[telegram_bot_token]' "${TMP_ROOT}/line-wrong.log" d
unit 1 "只有 fixture、缺規則 id → 失敗（規則被停用時的形狀）" negproof_expect_output_line "${SS_FIX}" '[telegram_bot_token]' "${TMP_ROOT}/line-fixture-only.log" d
unit 1 "needleA 空字串 → 失敗（空值即通過）"      negproof_expect_output_line "" '[telegram_bot_token]' "${TMP_ROOT}/line-both.log" d
unit 1 "needleB 空字串 → 失敗（空值即通過）"      negproof_expect_output_line "${SS_FIX}" "" "${TMP_ROOT}/line-both.log" d
unit 1 "log 檔不存在 → 失敗"                      negproof_expect_output_line "${SS_FIX}" '[telegram_bot_token]' "${TMP_ROOT}/no-such.log" d

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

# 「擋錯規則」stub（rc=1、輸出**提到 fixture**，但沒有該閘門專屬的規則 id）：
# 模擬「A 規則壞了、B 規則代擋同一行」——這正是只比對 fixture 檔名會誤綠的形狀。
cat > "${TMP_ROOT}/stub-wrong-rule-ss.sh" <<'STUB'
#!/usr/bin/env bash
echo "❌ [some-other-rule] scripts/__negtest_secret__.py:1 — 別的規則擋下"
exit 1
STUB
cat > "${TMP_ROOT}/stub-wrong-rule-ms.sh" <<'STUB'
#!/usr/bin/env bash
echo "❌ [R9] scripts/docker-compose.__negtest__.yml:4 — 別的規則擋下"
exit 1
STUB

# ── 規則層 mutation（改的是**規則程式碼**，不是換掉受測命令）───────────────────
# ① 「正確的規則」被改成永遠通過：把該規則的樣式／記錄改成永不命中這個 fixture。
mutate_disable_rule() {   # mutate_disable_rule <label> <repo>
  case "$1" in
    secret-scan)
      python3 - "$2/scripts/secret-scan.sh" <<'PY'
import re, sys
p = sys.argv[1]
s = open(p, encoding="utf-8").read()
s2 = re.sub(r'\("telegram_bot_token",\s*re\.compile\(r"[^"]*"\)\)',
            '("telegram_bot_token", re.compile(r"(?!)"))', s)
if s2 == s:
    sys.exit("mutation 目標（telegram_bot_token 的 pattern）找不到 —— 掃描器被重構過，請更新本測試")
open(p, "w", encoding="utf-8").write(s2)
PY
      ;;
    monitoring)
      python3 - "$2/scripts/ci/check_monitoring_single_source.py" <<'PY'
import sys
p = sys.argv[1]
s = open(p, encoding="utf-8").read()
s2 = s.replace('violations.append(("R2"', 'None and violations.append(("R2"')
s2 = s2.replace('violations.append(("R3"', 'None and violations.append(("R3"')
if s2 == s:
    sys.exit("mutation 目標（R2/R3 的 violations.append）找不到 —— 檢查被重構過，請更新本測試")
open(p, "w", encoding="utf-8").write(s2)
PY
      ;;
  esac
}

# ② 規則照跑（輸出仍有 ❌ 命中），但閘門的 exit code 被改成永遠 0。
mutate_force_rc0() {      # mutate_force_rc0 <label> <repo>
  case "$1" in
    secret-scan)
      mv "$2/scripts/secret-scan.sh" "$2/scripts/.orig-check.sh"
      printf '#!/usr/bin/env bash\n# mutation：真規則照跑，rc 被強制 0\nbash "$(dirname "$0")/.orig-check.sh" "$@"\necho "[mutation] 命中照印，但閘門回 0"\nexit 0\n' \
        > "$2/scripts/secret-scan.sh"
      ;;
    monitoring)
      mv "$2/scripts/ci/check_monitoring_single_source.py" "$2/scripts/ci/.orig-check.py"
      printf '#!/usr/bin/env bash\n# mutation：真規則照跑，rc 被強制 0\npython3 "$(dirname "$0")/.orig-check.py" "$@"\necho "[mutation] 命中照印，但閘門回 0"\nexit 0\n' \
        > "$2/scripts/ci/check_monitoring_single_source.sh"
      ;;
  esac
}

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

# 每個 case 的 spec 欄位：label|proof|human|fixture|rule|擋錯規則 stub
for spec in \
  "secret-scan|${SS_PROOF}|secret-scan 檢查|scripts/__negtest_secret__.py|telegram_bot_token|${TMP_ROOT}/stub-wrong-rule-ss.sh" \
  "monitoring|${MS_PROOF}|monitoring 檢查|scripts/docker-compose.__negtest__.yml|R2|${TMP_ROOT}/stub-wrong-rule-ms.sh"; do
  label="$(printf '%s' "${spec}" | cut -d'|' -f1)"
  proof="$(printf '%s' "${spec}" | cut -d'|' -f2)"
  human="$(printf '%s' "${spec}" | cut -d'|' -f3)"
  fixture="$(printf '%s' "${spec}" | cut -d'|' -f4)"
  rule="$(printf '%s' "${spec}" | cut -d'|' -f5)"
  wrong_stub="$(printf '%s' "${spec}" | cut -d'|' -f6)"
  repo="$(mk_repo)"
  case_proof "$repo" "$proof" 0   "E1 ${human}＋真命令 → 負向證明成立"                  ""        ""
  case_proof "$repo" "$proof" 1   "E2 ${human}＋命令不存在(127) → 必須紅燈"            "/nonexistent/bin/no-such-cmd" "exit=127"
  case_proof "$repo" "$proof" 1   "E3 ${human}＋用法錯誤(2) → 必須紅燈"                "bash ${TMP_ROOT}/stub-exit2.sh" "exit=2"
  case_proof "$repo" "$proof" 1   "E4 ${human}＋根本沒擋(0) → 必須紅燈"                "true"    ""
  # E6：rc=1、輸出**含 fixture**，但缺該規則的 id ⇒ 擋錯規則 ⇒ 必須紅燈
  #     （★ 只比對 fixture 檔名的舊寫法在這一格會誤綠：這正是本升級要釘住的形狀）
  case_proof "$repo" "$proof" 1   "E6 ${human}＋擋錯規則（含 ${fixture}、缺 [${rule}]）→ 必須紅燈" \
                                  "bash ${wrong_stub}" "沒有任一行同時含"
  # 前置檢查 fail-closed：受測腳本被刪掉（不 override）。放最後，因為它會把受測檔刪掉。
  if [ "$label" = "secret-scan" ]; then rm -f "$repo/scripts/secret-scan.sh"; else rm -f "$repo/scripts/ci/check_monitoring_single_source.sh"; fi
  case_proof "$repo" "$proof" 1   "E5 ${human}＋受測腳本被刪 → 前置檢查 fail-closed"    ""        "找不到"
  rm -rf "$repo"

  # E7：把「正確的規則」改成永遠通過（**改規則程式碼**，不是換掉受測命令）⇒ 閘門不再擋 ⇒ 必須紅燈
  repo7="$(mk_repo)"
  mutate_disable_rule "$label" "$repo7" || bad "E7 ${human}：規則層 mutation 執行失敗（規則被重構？請更新本測試）"
  case_proof "$repo7" "$proof" 1  "E7 ${human}＋正確規則被停用 → 必須紅燈"              ""        "exit=0"
  rm -rf "$repo7"

  # E8：規則照跑（輸出仍有 ❌ 命中），但閘門 rc 被強制 0 ⇒ 必須紅燈
  repo8="$(mk_repo)"
  mutate_force_rc0 "$label" "$repo8"
  case_proof "$repo8" "$proof" 1  "E8 ${human}＋規則照跑但 rc 強制 0 → 必須紅燈"        ""        "exit=0"
  rm -rf "$repo8"
done

echo ""
if [ "${fail}" -eq 0 ]; then
  echo "✅ test-negative-proofs PASS（${pass} 項）"
  exit 0
fi
echo "❌ test-negative-proofs FAIL（pass=${pass} fail=${fail}）"
exit 1
