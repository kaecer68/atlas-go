#!/usr/bin/env bash
# staging-soak-check.sh — Daily 6-check verification for the 2026-07-15
# capital-flow audit follow-up staging soak test (see
# docs/operations/2026-07-15-staging-soak-test.md).
#
# Exit codes:
#   0  all checks pass or warn (data missing but system healthy)
#   1  any check is a hard fail (data integrity violation)
#   2  script cannot reach the staging endpoints
#
# Output: JSON line per check to stdout AND to $LOG_DIR/$DATE.json
#
# Crontab: see scripts/staging-soak-check.cron

set -euo pipefail

STAGING_URL="${STAGING_URL:-http://localhost:18080}"
DATA_DB="${DATA_DB:-/var/lib/atlas/data/state/atlas.db}"
LOG_DIR="${LOG_DIR:-/var/log/atlas-soak}"
DATE=$(date -u +%Y-%m-%d)
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)

mkdir -p "$LOG_DIR"
REPORT="$LOG_DIR/$DATE.json"

results=()
overall="pass"

record() {
    local check="$1" status="$2" reason="$3" extra="${4:-}"
    local entry
    if [[ -n "$extra" ]]; then
        entry="{\"check\":\"$check\",\"status\":\"$status\",\"reason\":\"$reason\",\"$extra\"}"
    else
        entry="{\"check\":\"$check\",\"status\":\"$status\",\"reason\":\"$reason\"}"
    fi
    results+=("$entry")
    if [[ "$status" == "fail" ]]; then
        overall="fail"
    fi
}

# Check 1: stress_index geopolitical > 0 (G-11)
res=$(curl -sf --max-time 10 "$STAGING_URL/api/llm/stress_index/current" 2>/dev/null || echo "")
if [[ -z "$res" ]]; then
    record "stress_index_geopolitical" "fail" "endpoint unreachable"
else
    geo=$(echo "$res" | jq -r '.components.geopolitical // 0' 2>/dev/null || echo "0")
    if [[ "$geo" == "0" || "$geo" == "null" || -z "$geo" ]]; then
        record "stress_index_geopolitical" "warn" "geopolitical=0 (RSS/GDELT may be unreachable)" "geopolitical":0
    else
        record "stress_index_geopolitical" "pass" "geopolitical non-zero" "geopolitical":$geo
    fi
fi

# Check 2: event_flow_prediction non-neutral
res=$(curl -sf --max-time 10 "$STAGING_URL/api/events/prediction" 2>/dev/null || echo "")
if [[ -z "$res" ]]; then
    record "event_flow_prediction" "fail" "endpoint unreachable"
else
    unique=$(echo "$res" | jq -r '[.predictions[].direction] | unique | length' 2>/dev/null || echo "0")
    if [[ "$unique" -lt 2 ]]; then
        record "event_flow_prediction" "fail" "only $unique direction value(s); predictor not generating variety" "directions":$unique
    else
        record "event_flow_prediction" "pass" "$unique direction values" "directions":$unique
    fi
fi

# Check 3: regime_history no test-data pollution
if [[ ! -f "$DATA_DB" ]]; then
    record "regime_history_no_pollution" "warn" "db not found at $DATA_DB"
else
    dupes=$(sqlite3 "$DATA_DB" "SELECT COUNT(*) FROM (SELECT recorded_at, COUNT(*) c FROM regime_history GROUP BY recorded_at HAVING c > 1)" 2>/dev/null || echo "-1")
    if [[ "$dupes" == "-1" ]]; then
        record "regime_history_no_pollution" "fail" "sqlite3 query failed"
    elif [[ "$dupes" -gt 0 ]]; then
        record "regime_history_no_pollution" "fail" "$dupes duplicate recorded_at groups (test pollution)" "duplicates":$dupes
    else
        record "regime_history_no_pollution" "pass" "no duplicates"
    fi
fi

# Check 4: prediction_backtest has data
if [[ ! -f "$DATA_DB" ]]; then
    record "prediction_backtest_data" "warn" "db not found at $DATA_DB"
else
    rows=$(sqlite3 "$DATA_DB" "SELECT COUNT(*) FROM prediction_backtest WHERE is_synthetic = 0" 2>/dev/null || echo "-1")
    if [[ "$rows" == "-1" ]]; then
        record "prediction_backtest_data" "fail" "sqlite3 query failed"
    elif [[ "$rows" -lt 1 ]]; then
        record "prediction_backtest_data" "warn" "no real backtest rows yet" "rows":0
    else
        record "prediction_backtest_data" "pass" "$rows real backtest rows" "rows":$rows
    fi
fi

# Check 5: all 6 stage3 scheduler tasks registered
res=$(curl -sf --max-time 10 "$STAGING_URL/api/scheduler/status" 2>/dev/null || echo "")
if [[ -z "$res" ]]; then
    record "scheduler_tasks_registered" "fail" "endpoint unreachable"
else
    expected="recalibrate-templates-monthly sync-capital-daily sync-events-daily sync-macro-daily sync-regime-weekly template-detector-scan"
    missing=""
    for t in $expected; do
        if ! echo "$res" | grep -q "\"name\":\"$t\""; then
            missing="$missing $t"
        fi
    done
    if [[ -n "$missing" ]]; then
        record "scheduler_tasks_registered" "fail" "missing tasks:$missing"
    else
        record "scheduler_tasks_registered" "pass" "all 6 tasks registered"
    fi
fi

# Check 6: alert volume reasonable (< 10 in 24h, no critical)
res=$(curl -sf --max-time 10 "$STAGING_URL/api/alert/list?since=24h" 2>/dev/null || echo "")
if [[ -z "$res" ]]; then
    record "alert_volume_24h" "fail" "endpoint unreachable"
else
    count=$(echo "$res" | jq -r '.alerts | length' 2>/dev/null || echo "-1")
    critical=$(echo "$res" | jq -r '[.alerts[] | select(.severity == "critical")] | length' 2>/dev/null || echo "0")
    if [[ "$count" == "-1" ]]; then
        record "alert_volume_24h" "fail" "jq parse failed"
    elif [[ "$critical" -gt 0 ]]; then
        record "alert_volume_24h" "fail" "$critical critical alerts in 24h" "alerts":$count,"critical":$critical
    elif [[ "$count" -gt 10 ]]; then
        record "alert_volume_24h" "warn" "$count alerts in 24h (above 10 threshold)" "alerts":$count
    else
        record "alert_volume_24h" "pass" "$count alerts in 24h" "alerts":$count
    fi
fi

# Emit JSON report
(IFS=,; printf '%s' "{\"date\":\"$DATE\",\"ts\":\"$TS\",\"overall\":\"$overall\",\"checks\":[${results[*]}]}" > "$REPORT")
cat "$REPORT"
echo

# Exit code
case "$overall" in
    pass) exit 0 ;;
    warn) exit 0 ;;
    fail) exit 1 ;;
esac
