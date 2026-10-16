#!/usr/bin/env bash
# monitoring-single-source-negative-proof.sh — CI 負向證明：在**真的 repo 內容**上，
# 「監控設定只有一棵權威樹」檢查會擋下「掛載指回舊樹」的 tracked compose 檔。
#
# 【為什麼要有它】`tests/scripts/test-monitoring-single-source.sh` 證明的是 tempdir fixture 上的邏輯；
#   本腳本證明「**用 repo 的 tracked 檔清單、走 CI 的路徑**」時仍然會擋（2026-09-25 實測過 CI 平台差異）。
#   2026-10-16（FU-20260927-05）新增第二個 fixture：R4（runbook_url 目標存在性）作用在**規則檔**上，
#   與 R2/R3 的 compose 檔是不同類別 ⇒ 它需要自己的負向證明與自己的規則 id 斷言。
#
# 【為什麼一定要斷言「恰好 exit 1」】（issue #2011）
#   本步原本寫在 `.github/workflows/quality.yml` 內：
#       if bash scripts/ci/check_monitoring_single_source.sh; then echo "❌ 沒擋下"; exit 1; fi
#       echo "✅ 擋下了"
#   `if <cmd>` 把「**非 0**」都當成「擋下了」；檢查腳本被改名（127）、參數寫錯（2）時照樣印 ✅（假綠）。
#   改用 negproof_expect 精確比對 rc==1。註：本檢查的 `2` 是**用法錯誤**（`--root` 不存在），
#   不是「有違規」，所以 2 絕不可算通過。
#
# 【為什麼還要斷言「是哪幾條規則擋下」】
#   精確 rc 只證明「**有東西**擋下」。這個 fixture 的設計是同時踩到 R2（掛載源落點）與 R3（舊樹引用）；
#   若其中一條被停用／改壞，仍可能由另一條擋下 ⇒ 只看 rc 會誤以為「兩條都還有效」。
#   故逐條斷言「fixture 的那一行同時含規則 id `[R2]`／`[R3]`」：證明的是「**正確的規則**擋下」。
#
# 【為什麼不用 #2003 的 plumbing】見 secret-scan-negative-proof.sh 的同名段落：
#   受測檢查同樣刻意過濾 `GIT_*`（fixture 必須進真 index 才看得到），故改用
#   「暫時 `git add -f` + trap 無條件還原 + 跑前驗證 fixture 確實在 index 中」。
#
# 用法：bash scripts/ci/monitoring-single-source-negative-proof.sh
# exit：0 = 負向證明成立（檢查以 exit 1 擋下 fixture）；1 = 證明失敗/不成立
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${HERE}/../.." && pwd)"
# shellcheck source=./negative-proof-lib.sh
. "${HERE}/negative-proof-lib.sh"
cd "${REPO_ROOT}" || { negproof_err "無法進入 ${REPO_ROOT}"; exit 1; }

CHECK="scripts/ci/check_monitoring_single_source.sh"
FIXTURE="scripts/docker-compose.__negtest__.yml"
# 第二個 fixture（2026-10-16，FU-20260927-05）：R4 是**另一個檔案類別**（規則檔，不是 compose），
# 所以它需要自己的負向證明 —— 只證明 R2/R3 有牙齒，不代表 R4 有。
R4_FIXTURE="monitoring/rules/__negtest__.yml"
R4_DIR="$(dirname "${R4_FIXTURE}")"
# 這個 fixture 是為了同時踩到**哪幾條**規則（規則 id 字面值，見 check_monitoring_single_source.py
# 的 R2 / R3）。負向證明不只要證明「有東西擋下」，還要證明「**這幾條規則**擋下」——否則其中一條
# 被停用／改壞、由另一條代擋時，只看 rc 會誤以為「兩條都還有效」。
RULE_IDS="R2 R3"
LOG="$(mktemp)"
cleanup() {
  git -C "${REPO_ROOT}" rm --cached -q --ignore-unmatch -- "${FIXTURE}" "${R4_FIXTURE}" >/dev/null 2>&1 || :
  rm -f "${REPO_ROOT}/${FIXTURE}" "${REPO_ROOT}/${R4_FIXTURE}"
  rmdir "${REPO_ROOT}/${R4_DIR}" 2>/dev/null || :
  rm -f "${LOG}"
}
trap cleanup EXIT INT TERM

if [ -n "${NEGPROOF_CHECK_CMD:-}" ]; then
  read -r -a CHECK_CMD <<< "${NEGPROOF_CHECK_CMD}"
else
  negproof_require_file "${REPO_ROOT}/${CHECK}" "受測檢查（薄殼）" || exit 1
  CHECK_CMD=(bash "${CHECK}")
fi

# 舊樹字串用「相鄰字串相接」組出 ⇒ 本檔自身不含可被 R3 命中的字面值。
# 檔名用 `docker-compose.*` 開頭 ⇒ 同時觸發 R2（掛載源落點，dst=/etc/prometheus）與 R3（舊樹路徑引用）。
printf '%s\n' \
  'services:' \
  '  prometheus:' \
  '    volumes:' \
  '      - ~/workspace/'"atlas-monitoring"'/prometheus.yml:/etc/prometheus/prometheus.yml:ro' \
  > "${REPO_ROOT}/${FIXTURE}"
# R4 fixture：一條 alert rule，`runbook_url` 指到**本 repo 內不存在**的檔案
# ⇒ R4 必須以 `[R4]` 擋下（R2/R3 不看這種檔案，所以它是 R4 的**專屬**證據）。
mkdir -p "${REPO_ROOT}/${R4_DIR}"
printf '%s\n' \
  'groups:' \
  '  - name: negtest' \
  '    rules:' \
  '      - alert: NegTestRule' \
  '        annotations:' \
  '          runbook_url: "https://github.com/kaecer68/atlas-go/blob/main/docs/operations/__negtest_missing__.md"' \
  > "${REPO_ROOT}/${R4_FIXTURE}"
git -C "${REPO_ROOT}" add -f -- "${FIXTURE}" "${R4_FIXTURE}" || { negproof_err "git add -f fixture 失敗"; exit 1; }
for f in "${FIXTURE}" "${R4_FIXTURE}"; do
  if ! git -C "${REPO_ROOT}" ls-files --error-unmatch -- "${f}" >/dev/null 2>&1; then
    negproof_err "fixture ${f} 沒進 index（tracked 清單）⇒ 檢查看不到它，負向證明失去意義（fail-closed）"
    exit 1
  fi
done

echo "負向證明（monitoring 單一設定樹）：${FIXTURE} 已 staged（掛載指回舊樹）、${R4_FIXTURE} 已 staged（runbook_url 指向不存在的檔）→ 檢查必須以 exit 1 擋下"
if ! negproof_expect 1 "檢查擋下指向舊樹的 tracked compose" "${LOG}" -- "${CHECK_CMD[@]}"; then
  negproof_show_log "${LOG}"
  exit 1
fi
if ! negproof_expect_output "$(basename "${FIXTURE}")" "${LOG}" "擋下的就是那個 fixture（不是別的原因）"; then
  negproof_show_log "${LOG}"
  exit 1
fi
# 規則層斷言：擋下 fixture 的那一行必須**同時**出現每一條預期規則的 id（`[R2]`、`[R3]`）。
# 只證明「有東西擋下」不足：停用其中一條、由另一條代擋時，rc 仍是 1，只看 rc 會假安心。
for rule in ${RULE_IDS}; do
  if ! negproof_expect_output_line "$(basename "${FIXTURE}")" "[${rule}]" "${LOG}" \
       "擋下 fixture 的是 ${rule} 這條規則（不是別的規則代擋）"; then
    negproof_show_log "${LOG}"
    exit 1
  fi
done
# R4 的規則層斷言：擋下規則檔 fixture 的那一行必須帶 `[R4]`（同一個「不是別的規則代擋」的要求）。
if ! negproof_expect_output_line "$(basename "${R4_FIXTURE}")" "[R4]" "${LOG}" \
     "擋下 ${R4_FIXTURE} 的是 R4 這條規則（不是別的規則代擋）"; then
  negproof_show_log "${LOG}"
  exit 1
fi
negproof_show_log "${LOG}"
echo "✅ 負向證明通過：檢查以 exit 1 擋下 ${FIXTURE} 與 ${R4_FIXTURE}，且擋下它們的是規則 [$(echo "${RULE_IDS}" | tr ' ' '/')]/R4（精確 rc ＋ 規則 id 斷言；127/2 不再算「擋下了」）"
