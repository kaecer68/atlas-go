#!/usr/bin/env bash
# check_revert_guard.sh — 薄殼；實作在 .py（只用標準庫，離線可跑）。
#
# 為什麼要有 .sh：`make ci` 以 `bash scripts/ci/check_*.sh` 逐支跑、`make ci-gate` 也走同一支；
# GitHub Actions 的 quality.yml（job: revert-guard）同樣呼叫本檔。維持與其他 check_*.sh 一致。
#
# 兩個相（2026-09-26 重新定性）：
#   FAIL（evil merge）：`git merge-tree --write-tree` 算出的合併結果，改掉 main 上「本 PR 沒以普通
#                       commit 引進」的內容 ⇒ 真的回退（唯一現實成因：本地解衝突解錯）。
#   WARN（diff 衛生）：分支落後 main 時 diff 把別人已合併的改動顯示成刪除——**不是回退**（三方合併
#                       會保留 main 的版本），但會誤導 review／agent。不擋 CI；--strict 才失敗。
#
# 背景與規則：scripts/ci/check_revert_guard.py 檔頭；規格：docs/specs/branch-revert-guard-spec.md
# 用法：
#   bash scripts/ci/check_revert_guard.sh                       # base=origin/main, head=HEAD
#   bash scripts/ci/check_revert_guard.sh --base <ref> --head <ref>
#   bash scripts/ci/check_revert_guard.sh --json
#   bash scripts/ci/check_revert_guard.sh --strict              # WARN 也視為失敗（本機用）
# exit code: 0 = PASS（可能含 WARN）；1 = FAIL（evil merge 或 allowlist 理由缺失）；2 = 用法／環境錯誤
set -uo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT" || { echo "❌ 無法進入 repo 根目錄：$REPO_ROOT" >&2; exit 2; }

if ! command -v python3 >/dev/null 2>&1; then
    echo "❌ 需要 python3 才能執行 revert-guard 檢查"
    exit 2
fi

exec python3 "$REPO_ROOT/scripts/ci/check_revert_guard.py" "$@"
