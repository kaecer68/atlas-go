#!/usr/bin/env bash
#
# check_task_liveness.sh — 每日維護可觀測性閘門（真實來源：daemon task-liveness）
#
# 實作：scripts/ci/check_task_liveness.py（純 Python3 stdlib，無外部依賴）
#
# 背景（E21）：.github/workflows/daily-maintenance.yml 原本每天呼叫 7 個**不存在**的
# atlas CLI 子命令（weights adjust / prism status / reflexivity report …）。CLI 當時
# 靜默丟棄未知 positional args 並落到預設 one-shot simulation ⇒ job 永遠 exit 0、
# artifact 永遠是空的。這些報告的真實生產者其實是 daemon 的背景任務
# （internal/apigateway/background.go、cmd/atlas/main.go 的 BTM 註冊）。
#
# 本腳本把 CI 的「真實來源」換成 daemon 對外公開的活性端點
# （GET /api/dashboard/task-liveness；/api/dashboard/ 在 public-read 白名單內，免 API key），
# 並維持 fail-closed：抓不到快照＝無法驗證＝紅燈（exit 2），不是綠燈。
#
# 呼叫來源：.github/workflows/daily-maintenance.yml 的三個 job
#           （darwinian-adjust / prism-maintenance / reflexivity-analysis）。
#
# 用法：
#   bash scripts/ci/check_task_liveness.sh --label darwinian \
#       --tasks auto_daily_simulation,evolution_health \
#       --artifact reports/darwinian_liveness.json
#
# Exit: 0 = 健康、1 = 已驗證不健康、2 = 無法驗證（含用法錯誤）。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if ! command -v python3 >/dev/null 2>&1; then
    echo "❌ 需要 python3 才能執行 task-liveness 檢查" >&2
    exit 2
fi

exec python3 "${SCRIPT_DIR}/check_task_liveness.py" "$@"
