#!/usr/bin/env bash
#
# check_integration_filter_sync.sh — integration 路徑過濾器同步守門（CI 閘門）
#
# 背景：.github/workflows/ci-cd.yml 的 `integration` job 只在 `dorny/paths-filter` 的
# `integration:` 白名單命中時執行（job 內跑整個 `go test -tags=integration -p 1 ./...`）。
# 白名單一旦漏列某個「有 //go:build integration 測試檔」的目錄，只改該目錄的 PR 就會讓
# 這個 job 變成 **skipping（不是 pass）** ⇒ 那些 integration 測試在 CI 從未被觸發
# （本機跑得到 ⇒ 假安全）。FU-20261015-01。
#
# 判準（唯一權威）：來源是「repo 內帶 integration tag 的檔所在目錄」（可由檔案推導），
# 不是人工清單、也不是檔名慣例；例外必須逐條登錄且附理由（見 .py 的 docstring）。
#
# 實作：scripts/ci/check_integration_filter_sync.py（純 Python3；不執行 Go、不連網、不改檔）
# 回歸測試：tests/scripts/test-integration-filter-sync.sh（hermetic fixtures + mutation 自證）
#
# 呼叫來源：`make ci-gate`（pre-push 必跑，見 Makefile）、
#           GitHub Actions .github/workflows/ci-cd.yml 的 `governance` job（無 paths filter）。
#
# 用法：
#   bash scripts/ci/check_integration_filter_sync.sh
#   bash scripts/ci/check_integration_filter_sync.sh --json
#
# 退出碼：0 = 同步；1 = 不同步（有目錄未被涵蓋／豁免沒理由／豁免 stale／豁免矛盾）；
#         2 = 執行錯誤（解析不到白名單／root 或 workflow 不存在／glob 樣式不支援）
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

if ! command -v python3 >/dev/null 2>&1; then
    echo "❌ 需要 python3 才能執行 integration 過濾器同步檢查"
    exit 1
fi

exec python3 scripts/ci/check_integration_filter_sync.py --root "$REPO_ROOT" "$@"
