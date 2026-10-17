#!/usr/bin/env bash
#
# check_test_repo_root.sh — 測試不得用固定相對層數定位 repo 根（CI 閘門；FU-20261016-20）
#
# 背景：`filepath.Join(wd, "../..")` / `os.Chdir("../..")` / `const repoRoot = "../.."`
# 這一類寫法把「cwd 永遠是套件目錄」「套件永遠在 module root 下兩層」寫死。實際造成
# 一次「本機綠、CI 紅」：lane worktree 位於 `<主 clone>/.worktrees/<lane>`，從 lane 根往上
# 兩層剛好是主 clone（有 configs/parameters.json）⇒ 錯誤答案在本機看起來是對的。
#
# 實作：scripts/ci/check_test_repo_root.py（純 Python3；git ls-files 蒐集、不執行 Go、不改檔）
# Tier A＝硬禁止（把某個值當成 repo 根算出來）；Tier B＝既有固定層數字面值的額度（ratchet，
# 額度見 scripts/ci/test-repo-root-baseline.json）。
#
# 回歸測試：tests/scripts/test-test-repo-root-guard.sh（hermetic fixtures ＋ mutation 自證）
# 呼叫來源：`make ci-gate`（pre-push 必跑）
#
# 用法：
#   bash scripts/ci/check_test_repo_root.sh
#   bash scripts/ci/check_test_repo_root.sh --json
#   bash scripts/ci/check_test_repo_root.sh --update-baseline
#
# 退出碼：0 = 合規；1 = 違規（Tier A 命中／Tier B 超額）；2 = 執行或用法錯誤
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

if ! command -v python3 >/dev/null 2>&1; then
    echo "❌ 需要 python3 才能執行「測試不得用固定層數定位 repo 根」檢查（工具缺失不算通過）"
    exit 2
fi

exec python3 scripts/ci/check_test_repo_root.py --root "$REPO_ROOT" "$@"
