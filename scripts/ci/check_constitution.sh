#!/usr/bin/env bash
# =============================================================================
# check_constitution.sh — Atlas 數據源憲法合規檢查
#
# 檢查四條憲法規則：
#   1. os.Getenv 白名單 (Constitution 1.1, 1.2)
#   2. 數據通道 Gateway 註冊 (Constitution 1.1)
#   3. Rate Limiter 設定 (Constitution 2.1)
#   4. 背景任務註冊 (Constitution 4.1)
#
# 用法:
#   bash scripts/ci/check_constitution.sh          # 完整檢查
#   bash scripts/ci/check_constitution.sh --json   # JSON 輸出 (CI 整合用)
#
# 退出碼: 0 = 通過, 1 = 違規發現
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

OUTPUT_MODE="${1:-text}"
VIOLATIONS=0
JSON_VIOLATIONS="[]"

if [ -t 1 ] && [ "$OUTPUT_MODE" != "json" ]; then
  RED='\033[0;31m' GREEN='\033[0;32m' YELLOW='\033[1;33m' NC='\033[0m'
else
  RED='' GREEN='' YELLOW='' NC=''
fi

log_pass()  { printf "${GREEN}[PASS]${NC} %s\n" "$1"; }
log_fail()  { printf "${RED}[FAIL]${NC} %s\n" "$1"; }
log_warn()  { printf "${YELLOW}[WARN]${NC} %s\n" "$1"; }
log_info()  { printf "       %s\n" "$1"; }

# =============================================================================
# JSON violation accumulator — pure bash array, no processes per violation
# =============================================================================
VIOLATION_ENTRIES=()

add_json_violation() {
  local check="$1" file="$2" line="$3" detail="$4"
  VIOLATIONS=$((VIOLATIONS + 1))
  VIOLATION_ENTRIES+=("$check|$file|$line|$detail")
}

build_json_output() {
  if [ "${#VIOLATION_ENTRIES[@]}" -eq 0 ]; then
    JSON_VIOLATIONS='[]'
    return
  fi
  local first=1
  local json='['
  for entry in "${VIOLATION_ENTRIES[@]}"; do
    IFS='|' read -r check file line detail <<< "$entry"
    if [ "$first" -eq 1 ]; then
      first=0
    else
      json+=','
    fi
    json+='{"check":"'"$check"'","file":"'"$file"'","line":"'"$line"'","detail":"'"$detail"'"}'
  done
  json+=']'
  JSON_VIOLATIONS="$json"
}

# macOS-compatible: extract variable name from os.Getenv("VAR") line
extract_env_var() {
  echo "$1" | sed -n 's/.*os\.Getenv("\([^"]*\)").*/\1/p'
}

# =============================================================================
# 檢查 1: os.Getenv 白名單 (Constitution 1.1, 1.2)
# =============================================================================
check_env_vars() {
  printf "\n═══ 檢查 1/4: os.Getenv 白名單 ═══\n"

  local whitelist_file="configs/allowed_env_vars.md"
  if [ ! -f "$whitelist_file" ]; then
    log_fail "白名單文件不存在: $whitelist_file"
    add_json_violation "env_vars" "$whitelist_file" "0" "whitelist file missing"
    return 1
  fi

  # Extract allowed vars from markdown table (backtick-wrapped ATLAS_ vars)
  local allowed_vars
  allowed_vars=$(grep -Eo '`ATLAS_[A-Z_]+`' "$whitelist_file" | tr -d '`' | sort -u)
  allowed_vars="${allowed_vars}
FUGLE_API_KEY
FUBON_API_KEY
FUBON_PERSONAL_ID
FUBON_PROXY_PYTHON
FINMIND_API_KEY
FINMIND_TOKEN
FINMIND_RATE_LIMIT_PER_HOUR
TEJ_API_KEY
TEJ_ENABLED
ATLAS_FUGLE_API_KEY
ATLAS_FUBON_API_KEY
DATABASE_URL
HOME
TMPDIR
CI"

  local found_any=0

  while IFS=: read -r file line content; do
    [[ "$file" == *_test.go ]] && continue
    [[ "$file" == *"internal/config/config.go" ]] && continue
    [[ "$file" == *"check_constitution.sh" ]] && continue

    local var_name=""
    local re='os\.Getenv\("([^"]+)"\)'
    if [[ "$content" =~ $re ]]; then
      var_name="${BASH_REMATCH[1]}"
    fi

    if [ -z "$var_name" ]; then
      continue
    fi

    local is_allowed=0
    while IFS= read -r allowed; do
      [ "$var_name" = "$allowed" ] && { is_allowed=1; break; }
    done <<< "$allowed_vars"

    if [ "$is_allowed" -eq 0 ]; then
      log_fail "$file:$line — 未授權的 os.Getenv(\"$var_name\")"
      add_json_violation "env_vars" "$file" "$line" "unauthorized os.Getenv(\"$var_name\")"
      found_any=1
    fi
  done < <(grep -rn 'os\.Getenv' --include="*.go" . | grep -v 'allowed_env_vars\.md')

  if [ "$found_any" -eq 0 ]; then
    log_pass "所有 os.Getenv 調用均在白名單內 (或位於 config.go)"
  fi
  return $found_any
}

# =============================================================================
# 檢查 2: 數據通道 Gateway 註冊 (Constitution 1.1)
# =============================================================================
check_gateway_registration() {
  printf "\n═══ 檢查 2/4: Gateway 通道註冊 ═══\n"

  local gateway_file="internal/apigateway/gateway.go"
  if [ ! -f "$gateway_file" ]; then
    log_warn "Gateway 檔案不存在，跳過檢查"
    return 0
  fi

  local found_any=0
  local provider_name=""
  local re='(New[A-Z][a-zA-Z]*(Provider|Client))[(]'

  # Scan for New*Provider or New*Client patterns outside apigateway/
  while IFS=: read -r file line content; do
    [[ "$file" == *"internal/apigateway/"* ]] && continue
    [[ "$file" == *_test.go ]] && continue

    # Extract constructor name with bash ERE
    provider_name=""
    if [[ "$content" =~ $re ]]; then
      provider_name="${BASH_REMATCH[1]-}"
    fi

    if [ -n "${provider_name-}" ]; then
      log_warn "$file:$line — 直接建立 ${provider_name-}，確認是否已透過 Gateway 註冊"
      add_json_violation "gateway_registration" "$file" "$line" "direct provider instantiation: ${provider_name-}"
      found_any=1
    fi
  done < <(grep -rn 'New[A-Z][a-zA-Z]*\(Provider\|Client\)(' --include="*.go" . | grep -v '_test.go' | grep -v 'internal/apigateway/')

  if [ "$found_any" -eq 0 ]; then
    log_pass "未發現繞過 Gateway 的 Provider 直接建立"
  else
    log_warn "以上為潛在違規，請確認是否應改用 gateway.Fetch(channelID)"
  fi
  return 0
}

# =============================================================================
# 檢查 3: Rate Limiter 設定 (Constitution 2.1)
# =============================================================================
check_rate_limits() {
  printf "\n═══ 檢查 3/4: Rate Limiter 設定 ═══\n"

  local limits_file="internal/apigateway/limits.go"
  if [ ! -f "$limits_file" ]; then
    log_fail "Rate Limiter 設定檔不存在: $limits_file"
    add_json_violation "rate_limits" "$limits_file" "0" "limits file missing"
    return 1
  fi

	local known_channels=("us_yahoo" "frankfurter_fx" "twse_replay" "twse_capital_flow" "fugle" "fubon" "finmind" "geopolitical" "geopolitical_taiwan" "twse_margin" "export_statistics" "tsmc_revenue" "janus_regime" "tej" "exchange_rate" "sox_index" "sector_data" "dram_spot_price" "twse_sector_index" "day_trading" "bdi" "taifex_daily" "taifex_institutional" "twse_oddlot" "government_flow" "twse_etf" "us_spx" "us_ndx" "us_dji" "taiex_index" "tw_vol" "us_nvda" "us_aapl" "us_msft" "tsm_adr" "twse_sbl" "tdcc_equity_dispersion")

  # Single-pass: extract all channel identifiers from limits.go, then check known list
  local configured_channels
  configured_channels=$(grep -oE '"[a-z_]+"' "$limits_file" | tr -d '"' | sort -u)

  local missing=0
  for ch in "${known_channels[@]}"; do
    if ! grep -qF "$ch" <<< "$configured_channels"; then
      log_fail "通道 '$ch' 在 limits.go 中無限流設定"
      add_json_violation "rate_limits" "$limits_file" "0" "channel '$ch' missing rate limiter"
      missing=1
    fi
  done

  if [ "$missing" -eq 0 ]; then
    log_pass "所有 ${#known_channels[@]} 個通道均有限流設定"
  fi
  return $missing
}

# =============================================================================
# 檢查 4: 背景任務註冊 (Constitution 4.1)
# =============================================================================
check_background_tasks() {
  printf "\n═══ 檢查 4/4: 背景任務註冊 ═══\n"

  local found_any=0

  # Known-legitimate goroutine patterns (task-internal, event-driven, one-shot).
  # Many of these are documented Constitution §4.5.2 exceptions.
  local exclude_patterns=(
    "_test.go"
    "internal/apigateway/background.go"
    "cmd/atlas/main.go"
    "internal/taskexec/"
    "internal/eventbus/"
    "internal/orchestrator/phase3_controller.go"  # WaitGroup-coordinated parallel optimization
    "internal/live/orchestrator.go"               # event listener, not scheduled task
    "internal/live/fubon_dma.go"                  # process-wait cleanup
    "internal/live/scheduler.go"                  # live trading real-time scheduler
    "internal/monitoring/monitor.go"              # per-alert one-shot handlers
    "internal/monitoring/service/"                # Wave9 detector subsystems (lifecycle-bound)
    "internal/monitoring/dashboard_api.go"        # monitoring internals
    "internal/monitoring/api/system/handlers.go"  # monitoring API handlers (event-driven)
    "internal/monitoring/wave9_runtime.go"        # Wave9 detector startup goroutines
    "internal/marketdata/realtime/"               # WebSocket/streaming connection lifecycle
    "internal/marketdata/fubon_client.go"         # fubon proxy health probe
    "internal/marketdata/streaming.go"            # streaming adapter
    "internal/narrative/detector.go"              # narrative detector internals
    "internal/autobacktest/loop.go"               # autobacktest dedicated loop
    "internal/prism/prism_manager.go"             # PRISM training worker (dedicated scheduler)
    "internal/realtime/regime_adapter.go"         # real-time regime detection (sub-second ticker)
    "internal/metalearning/metalearner.go"        # strategy evolution (config-driven scheduler)
    "internal/spawning/spawning_manager.go"       # spawning manager (lifecycle-bound)
    "internal/mcp/anomaly/emitter.go"             # MCP anomaly emitter (monitoring subsystem)
    "internal/config/filelock.go"                 # file lock spinlock, not periodic
    "internal/fubonproxy/manager.go"              # F1-F9 supervisor invariants
  )

  # Build single-pass exclusion alternation for grep -vE
  local alt_pat=""
  local first=1
  for pat in "${exclude_patterns[@]}"; do
    # Escape '.' to '\\.' for literal regex matching
    local escaped="${pat//./\\.}"
    if [ "$first" -eq 1 ]; then
      alt_pat="$escaped"
      first=0
    else
      alt_pat="$alt_pat|$escaped"
    fi
  done

  while IFS= read -r match; do
    local file line
    file="${match%%:*}"
    line="${match#*:}"; line="${line%%:*}"

    log_warn "$file:$line — 可能繞過 BackgroundTaskManager 的獨立 goroutine"
    add_json_violation "background_tasks" "$file" "$line" "potential unregistered background goroutine"
    found_any=1
  done < <(grep -rn 'go func()' --include="*.go" internal/ | grep -vE "$alt_pat")

  if [ "$found_any" -eq 0 ]; then
    log_pass "未發現繞過 BackgroundTaskManager 的背景 goroutine"
  else
    log_info "以上為潛在違規，請確認是否應改用 taskMgr.Register()"
  fi
  return $found_any
}

# =============================================================================
# 主流程
# =============================================================================
main() {
  printf "Atlas 數據源憲法合規檢查\n==========================\n\n"

  local env_result=0 gw_result=0 rate_result=0 bg_result=0

  set +e
  check_env_vars; env_result=$?
  check_gateway_registration; gw_result=$?
  check_rate_limits; rate_result=$?
  check_background_tasks; bg_result=$?
  set -e

  printf "\n═══════════════════════════════════════\n"

  local checks_passed=0
  [ "$env_result" -eq 0 ] && checks_passed=$((checks_passed + 1))
  [ "$gw_result" -eq 0 ] && checks_passed=$((checks_passed + 1))
  [ "$rate_result" -eq 0 ] && checks_passed=$((checks_passed + 1))
  [ "$bg_result" -eq 0 ] && checks_passed=$((checks_passed + 1))

  if [ "$OUTPUT_MODE" = "json" ]; then
    build_json_output
    if command -v jq >/dev/null 2>&1; then
      jq -n \
        --argjson violations "${JSON_VIOLATIONS:-[]}" \
        --argjson total "$VIOLATIONS" \
        --argjson passed "$checks_passed" \
        --argjson env_ok "$([ "$env_result" -eq 0 ] && echo true || echo false)" \
        --argjson gw_ok "$([ "$gw_result" -eq 0 ] && echo true || echo false)" \
        --argjson rate_ok "$([ "$rate_result" -eq 0 ] && echo true || echo false)" \
        --argjson bg_ok "$([ "$bg_result" -eq 0 ] && echo true || echo false)" \
        '{status:(if $total > 0 then "violations_found" else "passed" end),total_violations:$total,checks_passed:$passed,checks_total:4,checks:{env_vars:$env_ok,gateway_registration:$gw_ok,rate_limits:$rate_ok,background_tasks:$bg_ok},violations:$violations}'
    else
      printf '{"status":"ok","note":"jq not available for detailed output"}\n'
    fi
  else
    printf "檢查結果: %d/4 通過 (env + gateway + rate_limit + background)\n" "$checks_passed"
    if [ "$VIOLATIONS" -gt 0 ]; then
      printf "${RED}發現 %d 處違規${NC}\n\n" "$VIOLATIONS"
      printf "修復建議:\n"
      printf "  1. os.Getenv 違規 → 改用 config.GetSecret() 或移至 config.go\n"
      printf "  2. Provider 直接建立 → 改用 gateway.Fetch(channelID)\n"
      printf "  3. Rate limiter 缺失 → 在 limits.go 補上限流設定\n"
      printf "  4. 背景 goroutine → 改用 taskMgr.Register()\n"
    else
      printf "${GREEN}所有檢查通過${NC}\n"
    fi
  fi

  if [ "$env_result" -ne 0 ] || [ "$gw_result" -ne 0 ] || [ "$rate_result" -ne 0 ] || [ "$bg_result" -ne 0 ]; then
    exit 1
  fi
  exit 0
}

main "$@"
