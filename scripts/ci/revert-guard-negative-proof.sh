#!/usr/bin/env bash
# revert-guard-negative-proof.sh — CI 專用的「負向證明」：本檢查**真的會擋**。
#
# 為什麼需要它：護欄若只會印 PASS 就等於沒有。hermetic fixture（tests/scripts/test-revert-guard.sh）
# 證明的是程式邏輯；這支證明的是「**用真 repo 歷史、走 CI 的 ref 解析**，本檢查的 FAIL 相會擋」。
#
# 2026-09-26 重新定性後的 FAIL 相 = **evil merge**（不是落後分支的假刪除；後者已改為 WARN 不擋）：
#   合併結果會改掉 main 上「本 PR 沒以普通 commit 引進」的內容。唯一現實成因是落後分支在本地
#   `git merge origin/main` 解衝突時解錯（把 main 的修正丟掉）。
#
# 做法（刻意**不切換工作樹、不動任何 ref**，只用 plumbing 造一個合成 merge commit）：
#   1. 從 base 的樹裡挑一個真實存在的共用資產檔 FILE（`git ls-tree`）。
#   2. 用**暫存 index** 讀入 base 的 tree、移除 FILE → `git write-tree` 得 TREE。
#   3. `git commit-tree TREE -p <base^> -p <base>` 造合成 commit NEG：
#      * 它是 **merge commit**（兩個 parent）——evil merge 的形狀（改動只可能來自「衝突解法」）；
#      * 其中一個 parent 就是 base ⇒ 它「已對齊」main（正是舊閘門會放行的形狀）；
#        第一個 parent 刻意**不是** base：parents[0] == base tip 的 head 會被檢查當成
#        GitHub 的**新鮮合成 merge ref** 直接拒絕（exit 2，假綠防線），那是另一條路徑。
#      * `base..NEG` 之間沒有任何**普通 commit**（--no-merges）動過 FILE
#        ⇒ 本檢查的 owned 判準必須把這個刪除歸給 merge commit 的解法 ⇒ FAIL。
#   4. 跑檢查，並要求 **exit code 恰好等於 1**，且輸出指出 evil merge 與該檔。
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
# 共用資產清單與 check_revert_guard.py 的 SHARED_* 常數一致（只影響「挑哪個檔」，不影響 severity）。
SPEC=(.github monitoring configs docs/reference Makefile 'docker-compose*.yml' 'docker-compose*.yaml' scripts/ci)

test -f "${CHECK}" || { echo "::error::找不到 ${CHECK}（檢查不存在就不可能證明它會擋）"; exit 1; }
git rev-parse --verify --quiet "${BASE}^{commit}" >/dev/null || {
    echo "::error::base ref 解不到：${BASE}"; exit 1; }

# 挑一個 base 上真實存在的共用資產檔（被刪掉才有意義）。
FILE="$(git ls-tree -r --name-only "${BASE}" -- "${SPEC[@]}" | head -1)"
test -n "${FILE}" || { echo "::error::${BASE} 的樹裡找不到共用資產檔（無法造負向案例）"; exit 1; }

PARENT2="$(git rev-parse "${BASE}^")"
BASE_SHORT="$(git rev-parse --short "${BASE}")"
echo "負向證明：以 ${BASE_SHORT} 的樹刪掉 ${FILE}，造一個「已對齊的 evil merge」合成 commit"

IDX="$(mktemp)"
trap 'rm -f "${IDX}"' EXIT
GIT_INDEX_FILE="${IDX}" git read-tree "${BASE}^{tree}"
GIT_INDEX_FILE="${IDX}" git update-index --force-remove -- "${FILE}"
TREE="$(GIT_INDEX_FILE="${IDX}" git write-tree)"
NEG="$(git -c user.name=ci -c user.email=ci@example.invalid \
        commit-tree "${TREE}" -p "${PARENT2}" -p "${BASE}" \
        -m "negtest: evil merge（解衝突解錯，丟掉 ${FILE}）")"
echo "合成 head=${NEG:0:8}（merge commit：parents=${PARENT2:0:8} + ${BASE_SHORT}；tree 已移除 ${FILE}）"

# 額外證據：merge-tree 的結果樹就是這個合成樹（即「合併結果真的少了 ${FILE}」）。
MT="$(git merge-tree --write-tree "${BASE}" "${NEG}" | head -1)"
echo "merge-tree --write-tree ${BASE_SHORT} ${NEG:0:8} = ${MT:0:12}（合成樹 ${TREE:0:12}）"
if [ "${MT}" != "${TREE}" ]; then
    echo "::error::merge-tree 的結果樹與合成樹不同（證據不成立）"; exit 1
fi

set +e
OUT="$(bash "${CHECK}" --base "${BASE}" --head "${NEG}" 2>&1)"
RC=$?
set -e
printf '%s\n' "${OUT}" | sed 's/^/    /'

if [ "${RC}" -ne 1 ]; then
    echo "::error title=負向證明失敗::期望檢查以 exit 1 擋下 evil merge，實際 exit ${RC}"
    exit 1
fi
printf '%s\n' "${OUT}" | grep -q "evil merge" || {
    echo "::error title=負向證明失敗::exit 1 但輸出沒有指出 evil merge（擋錯東西＝沒證明到）"; exit 1; }
printf '%s\n' "${OUT}" | grep -qF "${FILE}" || {
    echo "::error title=負向證明失敗::輸出沒有指出 ${FILE}"; exit 1; }
echo "✅ 負向證明通過：檢查以 exit 1 擋下 evil merge（合成 commit ${NEG:0:8} 少掉 ${FILE}）"
