#!/usr/bin/env bash
# secret-scan-negative-proof.sh — CI 負向證明：在**真的 repo 內容**上，secret-scan 會擋下含憑證的 tracked 檔。
#
# 【為什麼要有它】護欄若只會印 PASS 就等於沒有。`tests/scripts/test-secret-scan.sh` 證明的是
#   scanner 在 tempdir fixture 上的邏輯；本腳本證明的是「**用 repo 的 tracked 檔清單、走 CI 的路徑**」
#   時仍然會擋（2026-09-25 實測過一次 CI 平台差異造成的假失敗／假通過）。
#
# 【為什麼一定要斷言「恰好 exit 1」】（issue #2011）
#   本步原本寫在 `.github/workflows/quality.yml` 內：
#       if bash scripts/secret-scan.sh; then echo "❌ 沒擋下"; exit 1; fi
#       echo "✅ 擋下了"
#   `if <cmd>` 把「**非 0**」都當成「擋下了」，於是受測掃描器被改名／路徑寫錯時 `bash` 回
#   **127**、參數寫錯回 **2** ⇒ 照樣印 ✅（假綠）。改用 negproof_expect 精確比對 rc==1，
#   任何其他值都判為「測試本身失敗」。
#
# 【為什麼還要斷言「是哪一條規則擋下」】
#   精確 rc 只證明「**有東西**擋下」。若 telegram 規則被停用／改壞，卻由另一條規則順手擋下同一個
#   fixture，這支證明照樣綠燈。故再斷言「fixture 的那一行**同時**含該規則的 id `[telegram_bot_token]`」：
#   證明的是「**正確的規則**擋下」（強度升級；原本「輸出含 fixture 檔名」的斷言保留，未放寬）。
#
# 【為什麼不用 #2003 的 plumbing（暫存 index + write-tree/commit-tree）】
#   revert-guard 是「用 ref 指定 base/head」的檢查，所以能造合成 commit 而不動工作樹。
#   本檢查要靠 `git ls-files` 看到 fixture，而受測的 `secret-scan.sh` **刻意過濾所有 GIT_\* 環境變數**
#   （防 git hook 注入 GIT_DIR 後掃到呼叫端 repo，SOP ★37）⇒ `GIT_INDEX_FILE` 注入無效。
#   因此本腳本改為：暫時 `git add -f` 一個 fixture，離開時**無條件還原**（EXIT/INT/TERM trap），
#   並在跑之前用 `git ls-files --error-unmatch` 證明 fixture 真的進了 index（否則「掃不到 ⇒ rc=0」
#   會被誤讀成「掃描器放水」）。
#
# 用法：bash scripts/ci/secret-scan-negative-proof.sh
# exit：0 = 負向證明成立（掃描器以 exit 1 擋下 fixture）；1 = 證明失敗/不成立
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${HERE}/../.." && pwd)"
# shellcheck source=./negative-proof-lib.sh
. "${HERE}/negative-proof-lib.sh"
cd "${REPO_ROOT}" || { negproof_err "無法進入 ${REPO_ROOT}"; exit 1; }

CHECK="scripts/secret-scan.sh"
FIXTURE="scripts/__negtest_secret__.py"
# 這個 fixture 是為了觸發掃描器的**哪一條**規則（規則 id 字面值，見 secret-scan.sh 的 pattern 清單）。
# 負向證明不只要證明「有東西擋下」，還要證明「**這條規則**擋下」——否則 A 規則壞了、由 B 規則代擋，
# 斷言照樣綠燈（假安心）。
RULE_ID="telegram_bot_token"
LOG="$(mktemp)"
cleanup() {
  # 還原 index + 移除 fixture（任何結束路徑都跑；cleanup 失敗不影響判定結果，故吞掉錯誤）
  git -C "${REPO_ROOT}" rm --cached -q --ignore-unmatch -- "${FIXTURE}" >/dev/null 2>&1 || :
  rm -f "${REPO_ROOT}/${FIXTURE}"
  rm -f "${LOG}"
}
trap cleanup EXIT INT TERM

# 測試鉤子：只有 `tests/scripts/test-negative-proofs.sh` 會設定（餵「命令不存在」與「用法錯誤」兩種情境）
if [ -n "${NEGPROOF_CHECK_CMD:-}" ]; then
  read -r -a CHECK_CMD <<< "${NEGPROOF_CHECK_CMD}"
else
  negproof_require_file "${REPO_ROOT}/${CHECK}" "受測掃描器" || exit 1
  CHECK_CMD=(bash "${CHECK}")
fi

# 合成（無效）憑證：用「相鄰字串相接」組出，讓本檔**自身**不含可被掃描器命中的字面值。
printf '%s\n' 'BOT_TOKEN=1234567890:'"FAKEfakeFAKEfakeFAKEfakeFAKEfake12345" > "${REPO_ROOT}/${FIXTURE}"
git -C "${REPO_ROOT}" add -f -- "${FIXTURE}" || { negproof_err "git add -f ${FIXTURE} 失敗"; exit 1; }
if ! git -C "${REPO_ROOT}" ls-files --error-unmatch -- "${FIXTURE}" >/dev/null 2>&1; then
  negproof_err "fixture 沒進 index（tracked 清單）⇒ 掃描器看不到它，負向證明失去意義（fail-closed）"
  exit 1
fi

echo "負向證明（secret-scan）：${FIXTURE} 已 staged（合成、無效憑證）→ 掃描器必須以 exit 1 擋下"
if ! negproof_expect 1 "secret-scan 擋下含合成憑證的 tracked 檔" "${LOG}" -- "${CHECK_CMD[@]}"; then
  negproof_show_log "${LOG}"
  exit 1
fi
if ! negproof_expect_output "$(basename "${FIXTURE}")" "${LOG}" "擋下的就是那個 fixture（不是別的原因）"; then
  negproof_show_log "${LOG}"
  exit 1
fi
# 規則層斷言：擋下 fixture 的那一行必須**同時**出現該規則的 id（`[telegram_bot_token]`）。
# 只證明「有東西擋下」不足：目標規則被停用/改壞、卻由別的規則代擋同一行時，仍會假綠。
if ! negproof_expect_output_line "$(basename "${FIXTURE}")" "[${RULE_ID}]" "${LOG}" \
     "擋下 fixture 的是 ${RULE_ID} 這條規則（不是別的規則代擋）"; then
  negproof_show_log "${LOG}"
  exit 1
fi
negproof_show_log "${LOG}"
echo "✅ 負向證明通過：secret-scan 以 exit 1 擋下 ${FIXTURE}，且擋下它的是規則 [${RULE_ID}]（精確 rc ＋ 規則 id 斷言；127/2 不再算「擋下了」）"
