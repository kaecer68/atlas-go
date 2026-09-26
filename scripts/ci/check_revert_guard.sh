#!/usr/bin/env bash
# check_revert_guard.sh — 薄殼；實作在 .py（只用標準庫，離線可跑）。
#
# 為什麼要有 .sh：`make ci` 以 `bash scripts/ci/check_*.sh` 逐支跑、`make ci-gate` 也走同一支；
# GitHub Actions 的 quality.yml（job: revert-guard）同樣呼叫本檔。維持與其他 check_*.sh 一致。
#
# 背景與規則：scripts/ci/check_revert_guard.py 檔頭；規格：docs/specs/branch-revert-guard-spec.md
# 用法：
#   bash scripts/ci/check_revert_guard.sh                       # base=origin/main, head=HEAD
#   bash scripts/ci/check_revert_guard.sh --base <ref> --head <ref>
#   bash scripts/ci/check_revert_guard.sh --json
# exit code: 0 = PASS（可能含 WARN）；1 = FAIL；2 = 用法／環境錯誤（含 base ref 解不到）
set -uo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT" || { echo "❌ 無法進入 repo 根目錄：$REPO_ROOT" >&2; exit 2; }

if ! command -v python3 >/dev/null 2>&1; then
    echo "❌ 需要 python3 才能執行 revert-guard 檢查"
    exit 2
fi

exec python3 "$REPO_ROOT/scripts/ci/check_revert_guard.py" "$@"
