#!/usr/bin/env bash
#
# check_ledger_seam.sh — ledger store seam 守門（禁新增裸 ledger.NewStore，CI 閘門）
#
# 背景：internal/ledger 的 backend-aware factory（NewFullStore / NewOutcomeStore /
# NewReportOutcomeStore / PGFirstOutcomeStore）是 opt-in。直建 ledger.NewStore(dir)（JSONL-only）的
# 站點在生產（ATLAS_STORE_BACKEND=postgres）會安靜地讀錯後端：JSONL store 對缺檔回 (nil, nil)，
# 於是 autobacktest signal engine 讀空目錄 ⇒ GET /api/backtest/signals 回 200 全 0、err 為 nil
# ⇒ CIRCUIT_BREAKER 的 ForceOpenChannel 永不觸發（2026-10 生產缺陷 T1）。
#
# 本檢查在 PR 上擋下**新增**的裸呼叫；既有站點由 allowlist 明確登記（每筆 reason 必填）。
#
# 實作：scripts/ci/check_ledger_seam.py（純 Python3 詞法掃描，不需 Go toolchain、不連網）
# allowlist：scripts/ci/ledger-seam-allowlist.json
# 回歸測試：tests/scripts/test-ledger-seam.sh（hermetic fixtures）
#
# 呼叫來源：`make ledger-seam-check`（含回歸測試）、`make ci-gate`（pre-push），
#           以及 GitHub Actions .github/workflows/quality.yml 的 ledger-seam job。
#
# 用法：
#   bash scripts/ci/check_ledger_seam.sh              # 掃描 cmd/ 與 internal/
#   bash scripts/ci/check_ledger_seam.sh --json       # 機器可讀輸出
#   bash scripts/ci/check_ledger_seam.sh --root tests/fixtures/ledger-seam
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

if ! command -v python3 >/dev/null 2>&1; then
    echo "❌ 需要 python3 才能執行 ledger seam 檢查"
    exit 1
fi

exec python3 scripts/ci/check_ledger_seam.py --root "$REPO_ROOT" "$@"
