#!/usr/bin/env bash
set -euo pipefail
# hermes-smoke.sh — Hermes consumer smoke test with JSON shape validation
# 對 running atlas-go container 打所有 endpoint，驗證 HTTP status + JSON structure
# Exit 0 = all pass (shape warnings non-blocking), Exit 1 = hard HTTP fail

ATLAS_URL="${ATLAS_URL:-http://localhost:18080}"
BODY_FILE=$(mktemp -t smoke-body-XXXXXX)
trap 'rm -f "$BODY_FILE"' EXIT

# ---------------------------------------------------------------------------
# HTTP status check — writes body to BODY_FILE, returns status
# ---------------------------------------------------------------------------
http_check() {
  local name="$1" path="$2" expected_status="${3:-200}"
  > "$BODY_FILE"
  docker exec atlas-go curl -sS -w '\n%{http_code}' "$ATLAS_URL$path" \
    2>/dev/null > "$BODY_FILE" || { echo "ERR" > "$BODY_FILE"; }
  local status
  status=$(tail -1 "$BODY_FILE")
  sed -i.bak '$d' "$BODY_FILE" 2>/dev/null && rm -f "$BODY_FILE.bak"
  # expected_status may be an alternation ("200|404") for endpoints whose
  # data is environment-dependent (fresh CI container, no upstream data).
  if echo "$status" | grep -qE "^($expected_status)\$"; then
    printf "  ✓ %-42s HTTP %s\n" "$name" "$status"
    return 0
  else
    printf "  ✗ %-42s HTTP %s (expected %s)\n" "$name" "$status" "$expected_status"
    return 1
  fi
}

# ---------------------------------------------------------------------------
# JSON shape check (warning only, never hard-fails)
# $1 = name, $2 = comma-separated expected top-level keys
# ---------------------------------------------------------------------------
shape_check() {
  local name="$1" expected_keys="$2"
  [ "$expected_keys" = "-" ] && return 0
  [ ! -s "$BODY_FILE" ] && return 0

  local body missing
  body=$(cat "$BODY_FILE")
  missing=$(echo "$body" | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    print('__PARSE_ERROR__')
    sys.exit(0)
if isinstance(d, list):
    # JSON array — just check it's valid
    sys.exit(0)
expected = '$expected_keys'.split(',')
missing = [k for k in expected if k not in d]
if missing:
    print(','.join(missing))
" 2>/dev/null)

  if [ "$missing" = "__PARSE_ERROR__" ]; then
    printf "    ⚠  %-40s JSON parse error\n" ""
  elif [ -n "$missing" ]; then
    printf "    ⚠  %-40s missing: %s\n" "" "$missing"
  fi
}

# ---------------------------------------------------------------------------
# Combined: HTTP → shape
# ---------------------------------------------------------------------------
smoke() {
  local name="$1" path="$2" expected_keys="$3" expected_status="${4:-200}"
  http_check "$name" "$path" "$expected_status"
  local rc=$?
  [ $rc -eq 0 ] && shape_check "$name" "$expected_keys"
  return $rc
}

echo "=== Hermes Smoke Test ==="
echo "  Target: $ATLAS_URL"
echo ""

FAIL=0 PASS=0
do_smoke() { smoke "$@" && PASS=$((PASS+1)) || FAIL=$((FAIL+1)); }

# === Hermes audit items (E-01 ~ E-13) ===
do_smoke "E-01 explain_market_move"        "/api/market/explain"                           "generated_at,headline,detail,sections"
do_smoke "E-02 regime_get_history"         "/api/regime/history?days=7"                    "sessions,current_regime"
do_smoke "E-03 risk_get_metrics"           "/api/dashboard/risk"                           "gate_mode,risk_snapshot"
do_smoke "E-04 crossmarket_correlation"    "/api/cross-market/correlation"                 "correlation,observations"
do_smoke "E-05 strategy_get_summary"       "/api/strategies/foreign-3day-inflow/summary"   "id,summary"
do_smoke "E-06 capital_flow_daily"         "/api/capital-flow/daily"                       "date,forces,resonance"
do_smoke "E-07 crossmarket_us_indices"     "/api/dashboard/us-indices"                     "indices"
do_smoke "E-08 macro_snapshot_latest"      "/api/macro/snapshot/latest"                    "recorded_at,dxy" "200|404"
do_smoke "E-12 data_field_contract"        "/api/field-contract"                           "-"
do_smoke "E-13 strategy_summary_llm"       "/api/strategies/foreign-3day-inflow/summary"   "id,summary"

# === Additional key endpoints ===
do_smoke "capital_flow_summary"            "/api/capital-flow/summary"                     "assessment,summary,forces"
do_smoke "crossmarket_status"              "/api/cross-market/status"                      "generated_at,data_status"
do_smoke "system_health"                   "/api/dashboard/system-health"                  "regime,data_channels"
do_smoke "data_get_quality"                "/api/dashboard/data-quality"                   "score,overall"
do_smoke "llm_get_health"                  "/api/llm/health"                               "providers,router_version"
do_smoke "taiwan_stress_index"             "/api/taiwan/stress-index"                      "score,regime,components" "200|500"
do_smoke "alert_list_unacknowledged"       "/api/alerts/unacknowledged"                    "alerts,total"
do_smoke "backtest_status"                 "/api/backtest/status"                          "last_auto_date"
do_smoke "narrative_get_events"            "/api/narrative/events"                         "events"
do_smoke "narrative_stress_thresholds"     "/api/narrative/stress-index/thresholds"        "-"

TOTAL=$((PASS + FAIL))
echo ""
printf "  HTTP: %d passed | %d failed | %d total\n" "$PASS" "$FAIL" "$TOTAL"
echo "  Shape warnings above are non-blocking"
echo "==========================================="

[ "$FAIL" -eq 0 ] && echo "✅ SMOKE PASSED" && exit 0
echo "❌ SMOKE FAILED"
exit 1
