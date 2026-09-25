#!/usr/bin/env bash
#
# check_inert_closure.sh — inert（宣告生效、實際未生效）閉環靜態檢查（CI 閘門）
#
# 背景：issue #1944 建議 2。atlas-go 反覆出現「有寫入、無消費」的靜默失效（config 有宣告但沒有
# reader、writer 有 setter 但沒有 production consumer、狀態欄位硬寫 applied=true 而無消費證據）。
# 本檢查在 PR 上擋下**新增**的 inert 閉環；既有歷史項由 baseline allowlist 明確登記（每項必填理由）。
#
# 實作：scripts/ci/check_inert_closure.py（純 Python3、AST-free 詞法掃描，不需 Go toolchain、不連網）
# 規格：docs/specs/inert-static-check-spec.md
# baseline：scripts/ci/inert-baseline.json（`make inert-check-update` 更新，reason 必填）
#
# 呼叫來源：`make inert-check`（含自測）、`make ci-gate`（含在 pre-push）、`make ci`（glob 到本檔）、
#           以及 GitHub Actions .github/workflows/quality.yml 的 inert-closure job。
#
# 用法：
#   bash scripts/ci/check_inert_closure.sh              # 掃描整個 repo
#   bash scripts/ci/check_inert_closure.sh --json       # 機器可讀輸出
#   bash scripts/ci/check_inert_closure.sh --root tests/fixtures/inert-closure
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

if ! command -v python3 >/dev/null 2>&1; then
    echo "❌ 需要 python3 才能執行 inert 閉環檢查"
    exit 1
fi

exec python3 scripts/ci/check_inert_closure.py --root "$REPO_ROOT" "$@"
