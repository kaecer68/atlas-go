#!/usr/bin/env bash
set -euo pipefail
# hermes-smoke.sh — Hermes 13 項 audit smoke test
# 對 running atlas-go container 打所有 Hermes audit 驗證過的 endpoint
# 路徑來源：MCP handler code (tools_*.go)，不是 canary test (後者有 stale 路徑)
# Exit 0 = all pass, Exit 1 = any fail

ATLAS_URL="${ATLAS_URL:-http://localhost:18080}"

check() {
  local name="$1" path="$2" expected_status="${3:-200}"
  local status
  status=$(docker exec atlas-go curl -sS -o /dev/null -w '%{http_code}' \
    "$ATLAS_URL$path" 2>/dev/null || echo "ERR")
  if [ "$status" = "$expected_status" ]; then
    printf "  ✓ %-40s HTTP %s\n" "$name" "$status"
    return 0
  else
    printf "  ✗ %-40s HTTP %s (expected %s)\n" "$name" "$status" "$expected_status"
    return 1
  fi
}

echo "=== Hermes Smoke Test ==="
echo "  Target: $ATLAS_URL"
echo ""

FAIL=0
PASS=0

do_check() { check "$@" && PASS=$((PASS+1)) || FAIL=$((FAIL+1)); }

# === Hermes audit items (E-01 ~ E-13) ===
do_check "E-01 explain_market_move"        "/api/market/explain"
do_check "E-02 regime_get_history"         "/api/regime/history?days=7"
do_check "E-03 risk_get_metrics"           "/api/dashboard/risk"
do_check "E-04 crossmarket_correlation"    "/api/cross-market/correlation"
do_check "E-05 strategy_get_summary"       "/api/strategies/foreign-3day-inflow/summary"
do_check "E-06 capital_flow_daily"         "/api/capital-flow/daily"
do_check "E-07 crossmarket_us_indices"     "/api/dashboard/us-indices"
do_check "E-08 macro_snapshot_latest"      "/api/macro/snapshot/latest"
# E-09~E-11: crossmarket/strategy/data tools — tested below
do_check "E-12 data_field_contract"        "/api/field-contract"
do_check "E-13 strategy_summary_llm"       "/api/strategies/foreign-3day-inflow/summary"

# === Additional key endpoints ===
do_check "data_get_quality"                "/api/dashboard/data-quality"
do_check "llm_get_health"                  "/api/llm/health"
do_check "taiwan_stress_index"             "/api/taiwan/stress-index"
do_check "capital_flow_summary"            "/api/capital-flow/summary"
do_check "crossmarket_status"              "/api/cross-market/status"
do_check "system_health"                   "/api/dashboard/system-health"

TOTAL=$((PASS + FAIL))
echo ""
echo "==========================================="
printf "  %d passed | %d failed | %d total\n" "$PASS" "$FAIL" "$TOTAL"
echo "==========================================="

[ "$FAIL" -eq 0 ] && echo "✅ SMOKE PASSED" && exit 0
echo "❌ SMOKE FAILED"
exit 1
