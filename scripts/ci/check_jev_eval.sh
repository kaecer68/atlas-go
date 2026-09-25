#!/usr/bin/env bash
#
# check_jev_eval.sh — Jev 評估框架的 CI 閘門（2026-09-25, issue #1966）
#
# 規範：docs/jev/JEV-EVAL-FRAMEWORK.md
# 檢查項：
#   1. 框架自檢（離線、不連網、不需 TYPESAFE_API_KEY）：PIT 分離、決定性、
#      fail-open 計分、指標數學、門檻紀律、分級規則。
#   2. Go 端 panel 匯出器的單元測試（canonical 口徑 + PIT 性質）。
#
# 為什麼要進 CI：這個框架的失敗模式都是靜默的——GT 洩漏進 state、把「沒答」
# 當成「答 0」、或用評估集自己校準門檻，都仍會產出一份看起來合理的報告。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

if ! command -v python3 >/dev/null 2>&1; then
    echo "❌ 需要 python3 才能執行 Jev 評估框架自檢"
    exit 1
fi

echo "jev-eval: framework self-check (offline)"
python3 scripts/jev_eval/selfcheck.py >/dev/null 2>&1 || {
    echo "❌ 框架自檢失敗 —— 重跑以看細節： python3 scripts/jev_eval/selfcheck.py"
    exit 1
}

if command -v go >/dev/null 2>&1; then
    echo "jev-eval: panel exporter unit tests"
    go test ./cmd/experimental/jev-eval-panel/ >/dev/null || {
        echo "❌ jev-eval-panel 測試失敗"
        exit 1
    }
else
    echo "⚠ 找不到 go，略過 panel exporter 測試"
fi

echo "✅ JEV-EVAL OK"
