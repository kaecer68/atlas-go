#!/usr/bin/env bash
# revert-guard-negative-proof.sh — CI 專用的「負向證明」：本檢查**真的會擋**。
#
# 為什麼需要它：護欄若只會印 PASS 就等於沒有。hermetic fixture（tests/scripts/test-revert-guard.sh）
# 證明的是程式邏輯；這支證明的是「**用真 repo 歷史、走 CI 的 ref 解析**，落後分支刪掉共用資產時
# 檢查會擋」。
#
# 做法（刻意**不切換工作樹、不動任何 ref**，只用 plumbing 造一個合成 commit）：
#   1. 取 base 上最後一次「動過共用資產」的 commit（LAST_SHARED）與它的 parent。
#   2. 用**暫存 index** 讀入 parent 的 tree、移除 LAST_SHARED 動過的共用資產檔 → `git write-tree`。
#   3. `git commit-tree` 造出合成 commit（parent = parent，內容 = parent 減掉那個檔）。
#      ⇒ 這就是「落後分支把共用資產顯示成刪除」的等價物（該檔在 base 上有本分支沒有的改動）。
#   4. 跑檢查，並要求 **exit code 恰好等於 1**。
#
# ⚠️ 為什麼一定要斷言「恰好 1」而不是「非 0」：`if ! cmd` 會把 **127（腳本不存在）**、**2（設定錯誤）**
# 全部當成「擋下了」。2026-09-26 首版就是這樣假綠一次：合成分支 checkout 到舊 commit 後
# `scripts/ci/check_revert_guard.sh` 在該 commit 不存在 ⇒ 127 ⇒ 印出 ✅。本檔改用 plumbing 後
# 工作樹不動（腳本一直在），加上 exit code 精確斷言，兩道都補起來。
#
# 用法：bash scripts/ci/revert-guard-negative-proof.sh <base-ref>     # 例 refs/remotes/origin/main
# exit：0 = 負向證明成立（檢查以 exit 1 擋下）；非 0 = 證明失敗或不成立
set -euo pipefail

BASE="${1:-}"
if [ -z "${BASE}" ]; then
    echo "用法: $0 <base-ref>（例 refs/remotes/origin/main）" >&2
    exit 2
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${REPO_ROOT}" || { echo "❌ 無法進入 repo 根目錄：${REPO_ROOT}" >&2; exit 2; }

CHECK="scripts/ci/check_revert_guard.sh"
# 共用資產清單必須與 check_revert_guard.py 的 SHARED_* 常數一致（負向證明要落在 FAIL 路徑上）。
SPEC=(.github monitoring configs docs/reference Makefile 'docker-compose*.yml' 'docker-compose*.yaml' scripts/ci)

test -f "${CHECK}" || { echo "::error::找不到 ${CHECK}（檢查不存在就不可能證明它會擋）"; exit 1; }
git rev-parse --verify --quiet "${BASE}^{commit}" >/dev/null || {
    echo "::error::base ref 解不到：${BASE}"; exit 1; }

LAST_SHARED="$(git log --no-merges -n1 --format=%H "${BASE}" -- "${SPEC[@]}")"
test -n "${LAST_SHARED}" || { echo "::error::找不到動過共用資產的 ${BASE} commit"; exit 1; }

FILE="$(git diff --name-only --diff-filter=ACM "${LAST_SHARED}^" "${LAST_SHARED}" -- "${SPEC[@]}" | head -1)"
test -n "${FILE}" || { echo "::error::找不到候選共用資產檔（${LAST_SHARED}）"; exit 1; }

PARENT="$(git rev-parse "${LAST_SHARED}^")"
echo "負向證明：${LAST_SHARED:0:8} 動過 ${FILE}；以 ${PARENT:0:8} 為 parent 造『刪掉該檔』的合成 commit"

IDX="$(mktemp)"
trap 'rm -f "${IDX}"' EXIT
GIT_INDEX_FILE="${IDX}" git read-tree "${PARENT}^{tree}"
GIT_INDEX_FILE="${IDX}" git update-index --force-remove -- "${FILE}"
TREE="$(GIT_INDEX_FILE="${IDX}" git write-tree)"
NEG="$(git -c user.name=ci -c user.email=ci@example.invalid \
        commit-tree "${TREE}" -p "${PARENT}" -m "negtest: 落後分支刪除共用資產（${FILE}）")"
echo "合成 head=${NEG:0:8}（tree 已移除 ${FILE}）"

set +e
OUT="$(bash "${CHECK}" --base "${BASE}" --head "${NEG}" 2>&1)"
RC=$?
set -e
printf '%s\n' "${OUT}" | sed 's/^/    /'

if [ "${RC}" -ne 1 ]; then
    echo "::error title=負向證明失敗::期望檢查以 exit 1 擋下落後分支的假刪除，實際 exit ${RC}"
    exit 1
fi
echo "✅ 負向證明通過：檢查以 exit 1 擋下（合成 commit ${NEG:0:8} 刪除 ${FILE}）"
