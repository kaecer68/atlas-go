#!/usr/bin/env bash
# =============================================================================
# check_finmind_quota.sh — FinMind 每日 quota 檢查（#1742 P1）
#
# 背景（2026-08-29 prod 驗證, #1742）：
#   channel_health_finmind task_failed: finmind daily quota exhausted (402)
#   → Token illegal (400) → circuit breaker open。QuotaRegistry 只有
#   Snapshot()/IsProviderExhausted()，沒有告警消費者；唯一告警來源是
#   1h 一次的 channel_health_finmind probe（失敗才打 task_failed）。
#   本腳本補上「開市前 quota 檢查」：直接讀 DailyQuotaTracker 持久化
#   state file（data/state/finmind_daily_quota.json）。
#
# 用法:
#   bash scripts/ci/check_finmind_quota.sh                 # 只輸出,exit 0
#   bash scripts/ci/check_finmind_quota.sh --strict        # ≥90% → exit 1（cron 用）
#   bash scripts/ci/check_finmind_quota.sh --state-file <path> --limit 14400 --warn-pct 90
#   環境變數: FINMIND_DAILY_LIMIT / FINMIND_WARN_PCT 可覆寫預設
#
# cron 建議（iMac, 週一開市前 08:00 UTC+8）:
#   0 0 * * 1 cd ~/workspace/atlas && bash scripts/ci/check_finmind_quota.sh --strict \
#     || logger -t finmind-quota "⚠ FinMind quota ≥90% — 開市前需確認 key/quota"
#
# 退出碼: 0 = ok / state file 不存在（非 prod 或 tracker 未初始化）
#         1 = --strict 且 quota ≥ warn-pct，或 state file 解析失敗
# =============================================================================
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_FILE="${STATE_FILE:-$REPO_ROOT/data/state/finmind_daily_quota.json}"
LIMIT="${FINMIND_DAILY_LIMIT:-14400}"   # internal/marketdata/finmind_client.go finmindDailyLimit
WARN_PCT="${FINMIND_WARN_PCT:-90}"
STRICT=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --strict) STRICT=1; shift ;;
    --state-file=*) STATE_FILE="${1#*=}"; shift ;;
    --state-file) STATE_FILE="$2"; shift 2 ;;
    --limit=*) LIMIT="${1#*=}"; shift ;;
    --limit) LIMIT="$2"; shift 2 ;;
    --warn-pct=*) WARN_PCT="${1#*=}"; shift ;;
    --warn-pct) WARN_PCT="$2"; shift 2 ;;
    *) echo "❌ check_finmind_quota.sh: 未知參數 '$1'" >&2; exit 2 ;;
  esac
done

if [ ! -f "$STATE_FILE" ]; then
  echo "finmind quota: state file not found ($STATE_FILE) — 跳過（非 prod 或 tracker 未初始化）"
  exit 0
fi

# 解析 {"calls_today":N,"last_reset":"..."} — 純 grep/sed，零依賴
CALLS="$(grep -o '"calls_today"[[:space:]]*:[[:space:]]*[0-9]*' "$STATE_FILE" | head -1 | sed 's/.*:[[:space:]]*//')"
LAST_RESET="$(grep -o '"last_reset"[[:space:]]*:[[:space:]]*"[^"]*"' "$STATE_FILE" | head -1 | sed 's/.*"[[:space:]]*:[[:space:]]*"//; s/"$//')"

if ! [[ "$CALLS" =~ ^[0-9]+$ ]]; then
  echo "❌ finmind quota: 無法解析 $STATE_FILE（calls_today 缺失）"
  [ "$STRICT" -eq 1 ] && exit 1 || exit 0
fi

PCT=$(( CALLS * 100 / LIMIT ))
echo "finmind quota: ${CALLS}/${LIMIT} (${PCT}%)  last_reset=${LAST_RESET:-unknown}"

if [ "$PCT" -ge 100 ]; then
  echo "🔴 finmind quota EXHAUSTED (${PCT}%) — FinMind 上游將回 402，channel_health_finmind 會 task_failed"
  [ "$STRICT" -eq 1 ] && exit 1
elif [ "$PCT" -ge "$WARN_PCT" ]; then
  echo "⚠️  finmind quota 高於 ${WARN_PCT}% (${PCT}%) — 開市前需確認剩餘額度 / 換 key"
  [ "$STRICT" -eq 1 ] && exit 1
fi

exit 0
