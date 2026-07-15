#!/usr/bin/env bash
# staging-soak-check.sh — daily 5-check verification for the
# 2026-07-15 capital-flow audit follow-up staging soak test.
# Uses the 5 endpoints that actually return 200 in the running
# atlas-go container (verified 2026-07-15):
#   /health, /api/llm/health, /api/capital-flow/summary,
#   /api/events/prediction, /api/scheduler/status
#
# The other 2 originally planned (stress_index, alert_list) are
# not exposed as HTTP routes — only as MCP tool wrappers.

set -euo pipefail

STAGING_URL="${STAGING_URL:-http://localhost:18080}"
LOG_DIR="${LOG_DIR:-$HOME/logs/atlas-soak}"
STAGING_ENV_FILE="${STAGING_ENV_FILE:-$HOME/.config/atlas-go/.env}"
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
    if [[ "$status" == "fail" ]]; then overall="fail"; fi
}

# 1. /health
res=$(curl -sf --max-time 5 "$STAGING_URL/health" 2>/dev/null || echo "")
if [[ -z "$res" ]]; then
    record "health" "fail" "endpoint unreachable"
else
    state=$(echo "$res" | jq -r '.status // ""' 2>/dev/null)
    if [[ "$state" == "ok" ]]; then
        record "health" "pass" "atlas-go responsive"
    else
        record "health" "fail" "status field=$state (expected ok)"
    fi
fi

# 2. /api/llm/health — LLM routing
res=$(curl -sf --max-time 5 "$STAGING_URL/api/llm/health" 2>/dev/null || echo "")
if [[ -z "$res" ]]; then
    record "llm_router" "fail" "endpoint unreachable"
else
    count=$(echo "$res" | jq -r '[.providers // {} | to_entries[] | select(.value.healthy==true)] | length' 2>/dev/null || echo "0")
    if [[ "$count" -ge 1 ]]; then
        record "llm_router" "pass" "$count healthy providers"
    else
        record "llm_router" "fail" "0 healthy providers"
    fi
fi

# 3. /api/capital-flow/summary — verifies G-12 ChangePct fix
res=$(curl -sf --max-time 5 "$STAGING_URL/api/capital-flow/summary" 2>/dev/null || echo "")
if [[ -z "$res" ]]; then
    record "capital_flow" "fail" "endpoint unreachable"
else
    has_resonance=$(echo "$res" | jq -r '.resonance_dir // ""' 2>/dev/null)
    if [[ -n "$has_resonance" ]]; then
        record "capital_flow" "pass" "resonance_dir=$has_resonance (G-12 ChangePct wired)"
    else
        record "capital_flow" "fail" "no resonance_dir in response"
    fi
fi

# 4. /api/events/prediction — 5-day forecast exists
res=$(curl -sf --max-time 5 "$STAGING_URL/api/events/prediction" 2>/dev/null || echo "")
if [[ -z "$res" ]]; then
    record "event_prediction" "fail" "endpoint unreachable"
else
    pred_count=$(echo "$res" | jq -r '.predictions | length' 2>/dev/null || echo "0")
    if [[ "$pred_count" -ge 5 ]]; then
        record "event_prediction" "pass" "$pred_count predictions returned"
    else
        record "event_prediction" "fail" "only $pred_count predictions (expected ≥5)"
    fi
fi

# 5. /api/scheduler/status — scheduler has the right tasks
res=$(curl -sf --max-time 5 "$STAGING_URL/api/scheduler/status" 2>/dev/null || echo "")
if [[ -z "$res" ]]; then
    record "scheduler" "fail" "endpoint unreachable"
else
    count=$(echo "$res" | jq -r 'length' 2>/dev/null || echo "0")
    has_macro_ingest=$(echo "$res" | jq -r '[.[] | select(.name=="macro_ingest")] | length' 2>/dev/null || echo "0")
    has_auto_capital=$(echo "$res" | jq -r '[.[] | select(.name=="auto_capital_flow")] | length' 2>/dev/null || echo "0")
    if [[ "$count" -ge 30 && "$has_macro_ingest" -ge 1 && "$has_auto_capital" -ge 1 ]]; then
        record "scheduler" "pass" "$count tasks, macro_ingest + auto_capital_flow present"
    else
        record "scheduler" "fail" "count=$count macro_ingest=$has_macro_ingest auto_capital=$has_auto_capital"
    fi
fi

API_KEY=""
if [[ -f "$STAGING_ENV_FILE" ]]; then
    API_KEY=$(grep "ATLAS_API_KEY" "$STAGING_ENV_FILE" | cut -d= -f2 | tr -d '"' | tr -d "'")
fi
if [[ -z "$API_KEY" ]]; then
    record "detector_scan" "warn" "ATLAS_API_KEY not found in $STAGING_ENV_FILE (cannot test auth endpoint)"
else
    http_code=$(curl -s -o /tmp/detector_scan_body -w "%{http_code}" --max-time 5 -H "Authorization: Bearer $API_KEY" "$STAGING_URL/api/detector/scan/status?limit=1" 2>/dev/null || echo "000")
    body=$(cat /tmp/detector_scan_body 2>/dev/null)
    if [[ "$http_code" == "200" ]] && echo "$body" | jq -e '.scans' > /dev/null 2>&1; then
        scan_count=$(echo "$body" | jq '.scans | length' 2>/dev/null || echo "0")
        record "detector_scan" "pass" "200 + valid JSON, scans=$scan_count (G-08 detector route verified)"
    elif [[ "$http_code" == "401" ]]; then
        record "detector_scan" "fail" "401 unauthorized (auth header rejected — check API key)"
    elif [[ "$http_code" == "404" ]]; then
        record "detector_scan" "fail" "404 not found (route not registered on the mux that serves auth requests — known G-08 follow-up)"
    else
        record "detector_scan" "fail" "http=$http_code body=${body:0:100}"
    fi
    rm -f /tmp/detector_scan_body
fi

(IFS=,; printf '%s' "{\"date\":\"$DATE\",\"ts\":\"$TS\",\"overall\":\"$overall\",\"checks\":[${results[*]}]}" > "$REPORT")
cat "$REPORT"
echo

case "$overall" in pass|warn) exit 0 ;; fail) exit 1 ;; esac
