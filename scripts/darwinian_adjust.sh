#!/bin/bash
#
# DEPRECATED — D1 決策退役 2026-08-17 / E17 改為明示 no-op 2026-09-26
#
# 真實 Darwinian 演化在 Go 側，本檔不承擔任何計算:
#   - 實作: internal/portfolio/darwinian_weights.go (DarwinianWeightManager)
#   - BTM 任務: auto_daily_simulation（「每日模擬場次執行」, 任務名稱表見
#     internal/apigateway/background.go）
#   - 產出: data/state/darwinian_history.jsonl
#
# 為何本檔還存在: docker-compose.yml 的 atlas-cron-darwinian 服務仍以
#   CRON_COMMAND=/app/scripts/darwinian_adjust.sh --apply（CRON_SCHEDULE=0 9 * * *）
# 呼叫本檔。移除該容器屬生產拓撲變更，故保留服務、只把腳本改成明示 no-op。
#
# 舊行為（E17 修掉的就是這個）: 參數解析器只認 --dry-run / --reset，收到 --apply 即
#   "Unknown option: --apply" 並 exit 1 ⇒ cron 每日在 /var/log/cron/cron.log 留下
#   一次靜默失敗，並回報 liveness exit_code=1。
#
# 新行為: 任何參數一律接受、一律 no-op、一律 exit 0，並印出可 grep 的訊息。
#   看到下面這行代表排程正常運作，不是故障。
#
# Usage: ./scripts/darwinian_adjust.sh [--apply] [--dry-run] [--reset]
#   （所有參數都被接受並忽略；本檔不讀寫任何檔案、不產生 report）
#
set -euo pipefail

# 參數一律接受並忽略（--apply / --dry-run / --reset / 未知參數皆同）:
# 退役 stub 的退出碼不應隨呼叫端參數而變（E17 要求 4）。
: "$@"

printf '[%s] darwinian_adjust: DEPRECATED — Go auto_daily_simulation handles this; no-op exit 0\n' \
    "$(date '+%Y-%m-%d %H:%M:%S')"
printf '[%s] darwinian_adjust: real implementation = internal/portfolio/darwinian_weights.go (BTM task auto_daily_simulation); no-op here is expected, not a failure\n' \
    "$(date '+%Y-%m-%d %H:%M:%S')"

exit 0
