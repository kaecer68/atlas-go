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
#   bash scripts/ci/check_finmind_quota.sh --state-file <path> --limit 12000 --warn-pct 90
#   環境變數: FINMIND_DAILY_LIMIT / FINMIND_WARN_PCT 可覆寫預設
#
# cron 建議（iMac, 週一開市前 08:00 UTC+8）:
#   0 0 * * 1 cd ~/workspace/atlas && bash scripts/ci/check_finmind_quota.sh --strict \
#     || logger -t finmind-quota "⚠ FinMind quota ≥90% — 開市前需確認 key/quota"
#
# 退出碼: 0 = ok / state file 不存在（非 prod 或 tracker 未初始化）
#         1 = --strict 且 quota ≥ warn-pct，或 state file 解析失敗，
#             或 state file 被標記 quota-unknown（#2014：用量不可知 ⇒ fail-closed）
# =============================================================================
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_FILE="${STATE_FILE:-$REPO_ROOT/data/state/finmind_daily_quota.json}"
# LIMIT must track internal/marketdata/finmind_client.go finmindDailyLimit — it is
# the number this script divides by to decide "how close are we to the wall".
# 14400 → 12000 (fix/finmind-quota-honor-402-r, 2026-09-26): the upstream began
# refusing at ~12,500 calls, so the old default under-reported pressure — at
# 12,500 calls it printed "12500/14400 (86%)", i.e. below the 90% warn
# threshold, while FinMind was already answering 402. A Go test
# (TestFinMindQuotaOpsScript_DefaultMatchesCeiling) fails if this drifts from
# the constant again.
LIMIT="${FINMIND_DAILY_LIMIT:-12000}"
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

# #2014: a state file carrying `quota_unknown: true` is a deliberate fail-closed
# marker — the previous counter file was corrupt (or unreadable), so TODAY'S
# USAGE IS UNKNOWN and DailyQuotaTracker is refusing calls on purpose. Printing
# "0/12000 (0%)" here would be the same silent pass the marker exists to
# prevent, so report it as a problem instead.
if grep -q '"quota_unknown"[[:space:]]*:[[:space:]]*true' "$STATE_FILE"; then
  echo "❌ finmind quota: $STATE_FILE 被標記為 quota-unknown — 今日用量不可知，DailyQuotaTracker 正 fail-closed（不放行呼叫）"
  REASON="$(grep -o '"quota_unknown_reason"[[:space:]]*:[[:space:]]*"[^"]*"' "$STATE_FILE" | head -1 | sed 's/.*"[[:space:]]*:[[:space:]]*"//; s/"$//')"
  [ -n "$REASON" ] && echo "   原因: $REASON"
  echo "   處置: 修復目標就是這個 marker 檔（內含 quota_unknown_reason）。**不要刪除它** —— 刪掉等於把當日已用量與上游 latch 一起歸零，之後會重複噴到牆。"
  echo "         正常復原：等配額日跨日（00:00Z = 台北 08:00）自動解除；期間請以快取資料為準。同目錄若有 *.corrupt-* 是損壞前的原檔副本，僅供診斷。"
  if [ "$STRICT" -eq 1 ]; then exit 1; else exit 0; fi
fi

# 解析 {"calls_today":N,"last_reset":"..."} — 純 grep/sed，零依賴
CALLS="$(grep -o '"calls_today"[[:space:]]*:[[:space:]]*[0-9]*' "$STATE_FILE" | head -1 | sed 's/.*:[[:space:]]*//')"
LAST_RESET="$(grep -o '"last_reset"[[:space:]]*:[[:space:]]*"[^"]*"' "$STATE_FILE" | head -1 | sed 's/.*"[[:space:]]*:[[:space:]]*"//; s/"$//')"

if ! [[ "$CALLS" =~ ^[0-9]+$ ]]; then
  echo "❌ finmind quota: 無法解析 ${STATE_FILE}（calls_today 缺失）"
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
