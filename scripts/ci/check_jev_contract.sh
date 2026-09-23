#!/usr/bin/env bash
#
# check_jev_contract.sh — Jev 用量契約靜態檢查（CI 閘門）
#
# 規範：docs/jev/JEV-USAGE-CONTRACT.md
# 實作：scripts/jev-contract-check.py（AST 靜態分析，不連網）
#
# 本 repo 目前沒有任何 Jev（TypeSafe System One）用法 → 本檢查應 PASS（0 違規）。
# 目的是「守門」：未來任何人新增呼叫 Jev 的程式碼，違反 C1–C5 就會在這裡被擋下。
#
# 呼叫來源：`make ci`（glob scripts/ci/check_*.sh）、`make ci-quick`、
#           以及 GitHub Actions .github/workflows/quality.yml 的 jev-contract job。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

if ! command -v python3 >/dev/null 2>&1; then
    echo "❌ 需要 python3 才能執行 Jev 契約檢查"
    exit 1
fi

exec python3 scripts/jev-contract-check.py --root "$REPO_ROOT"
