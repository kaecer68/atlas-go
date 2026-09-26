#!/usr/bin/env bash
# monitoring-single-source-negative-proof.sh — CI 負向證明：在**真的 repo 內容**上，
# 「監控設定只有一棵權威樹」檢查會擋下「掛載指回舊樹」的 tracked compose 檔。
#
# 【為什麼要有它】`tests/scripts/test-monitoring-single-source.sh` 證明的是 tempdir fixture 上的邏輯；
#   本腳本證明「**用 repo 的 tracked 檔清單、走 CI 的路徑**」時仍然會擋（2026-09-25 實測過 CI 平台差異）。
#
# 【為什麼一定要斷言「恰好 exit 1」】（issue #2011）
#   本步原本寫在 `.github/workflows/quality.yml` 內：
#       if bash scripts/ci/check_monitoring_single_source.sh; then echo "❌ 沒擋下"; exit 1; fi
#       echo "✅ 擋下了"
#   `if <cmd>` 把「**非 0**」都當成「擋下了」；檢查腳本被改名（127）、參數寫錯（2）時照樣印 ✅（假綠）。
#   改用 negproof_expect 精確比對 rc==1。註：本檢查的 `2` 是**用法錯誤**（`--root` 不存在），
#   不是「有違規」，所以 2 絕不可算通過。
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
LOG="$(mktemp)"
cleanup() {
  git -C "${REPO_ROOT}" rm --cached -q --ignore-unmatch -- "${FIXTURE}" >/dev/null 2>&1 || :
  rm -f "${REPO_ROOT}/${FIXTURE}"
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
git -C "${REPO_ROOT}" add -f -- "${FIXTURE}" || { negproof_err "git add -f ${FIXTURE} 失敗"; exit 1; }
if ! git -C "${REPO_ROOT}" ls-files --error-unmatch -- "${FIXTURE}" >/dev/null 2>&1; then
  negproof_err "fixture 沒進 index（tracked 清單）⇒ 檢查看不到它，負向證明失去意義（fail-closed）"
  exit 1
fi

echo "負向證明（monitoring 單一設定樹）：${FIXTURE} 已 staged（掛載指回舊樹）→ 檢查必須以 exit 1 擋下"
if ! negproof_expect 1 "檢查擋下指向舊樹的 tracked compose" "${LOG}" -- "${CHECK_CMD[@]}"; then
  negproof_show_log "${LOG}"
  exit 1
fi
if ! negproof_expect_output "$(basename "${FIXTURE}")" "${LOG}" "擋下的就是那個 fixture（不是別的原因）"; then
  negproof_show_log "${LOG}"
  exit 1
fi
negproof_show_log "${LOG}"
echo "✅ 負向證明通過：檢查以 exit 1 擋下 ${FIXTURE}（精確 rc 斷言；127/2 不再算「擋下了」）"
