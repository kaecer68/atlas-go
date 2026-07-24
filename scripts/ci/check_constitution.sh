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

add_json_violation() {
  local check="$1" file="$2" line="$3" detail="$4"
  VIOLATIONS=$((VIOLATIONS + 1))
  if command -v jq >/dev/null 2>&1; then
    JSON_VIOLATIONS=$(echo "$JSON_VIOLATIONS" | jq \
      --arg check "$check" --arg file "$file" --arg line "$line" --arg detail "$detail" \
      '. + [{"check":$check,"file":$file,"line":$line,"detail":$detail}]')
  fi
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
TEJ_API_KEY
ATLAS_FUGLE_API_KEY
ATLAS_FUBON_API_KEY
DATABASE_URL
HOME
TMPDIR"

  local found_any=0

  while IFS=: read -r file line content; do
    [[ "$file" == *_test.go ]] && continue
    [[ "$file" == *"internal/config/config.go" ]] && continue
    [[ "$file" == *"check_constitution.sh" ]] && continue

    local var_name
    var_name=$(extract_env_var "$content")

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

  # Scan for New*Provider or New*Client patterns outside apigateway/
  while IFS=: read -r file line content; do
    [[ "$file" == *"internal/apigateway/"* ]] && continue
    [[ "$file" == *_test.go ]] && continue

    # Extract constructor name with sed (not grep -P)
    local provider_name
    provider_name=$(echo "$content" | sed -n 's/.*\(New[A-Z][a-zA-Z]*\(Provider\|Client\)\)(.*/\1/p' | head -1)

    if [ -n "$provider_name" ]; then
      log_warn "$file:$line — 直接建立 $provider_name，確認是否已透過 Gateway 註冊"
      add_json_violation "gateway_registration" "$file" "$line" "direct provider instantiation: $provider_name"
      found_any=1
    fi
  done < <(grep -rn 'New[A-Z][a-zA-Z]*\(Provider\|Client\)(' --include="*.go" . | grep -v '_test.go' | grep -v 'internal/apigateway/')

  if [ "$found_any" -eq 0 ]; then
    log_pass "未發現繞過 Gateway 的 Provider 直接建立"
  else
    log_info "以上為潛在違規，請手動確認是否應透過 gateway.Fetch() 調用"
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

  local missing=0
  for ch in "${known_channels[@]}"; do
    if ! grep -q "\"$ch\"" "$limits_file"; then
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

  # Known-legitimate goroutine patterns (task-internal, event-driven, one-shot)
  local exclude_patterns=(
    "_test.go"
    "internal/apigateway/background.go"
    "cmd/atlas/main.go"
    "internal/taskexec/"
    "internal/eventbus/"
    "internal/orchestrator/phase3_controller.go"  # WaitGroup-coordinated parallel optimization
    "internal/live/orchestrator.go"               # event listener, not scheduled task
    "internal/live/fubon_dma.go"                  # process-wait cleanup
    "internal/monitoring/monitor.go"              # per-alert one-shot handlers
    "internal/monitoring/service/backtest.go"     # timeout-based one-shots
    "internal/marketdata/realtime/redis_subscriber.go"  # connection lifecycle
    "internal/fubonproxy/manager.go"              # F1-F9 supervisor invariants govern Start() health check + supervise() main loop
  )

  while IFS= read -r match; do
    local file line
    file=$(echo "$match" | cut -d: -f1)
    line=$(echo "$match" | cut -d: -f2)

    local skip=0
    for pat in "${exclude_patterns[@]}"; do
      [[ "$file" == *"$pat"* ]] && { skip=1; break; }
    done
    [ "$skip" -eq 1 ] && continue

    log_warn "$file:$line — 可能繞過 BackgroundTaskManager 的獨立 goroutine"
    add_json_violation "background_tasks" "$file" "$line" "potential unregistered background goroutine"
    found_any=1
  done < <(grep -rn 'go func()' --include="*.go" internal/)

  if [ "$found_any" -eq 0 ]; then
    log_pass "未發現繞過 BackgroundTaskManager 的背景 goroutine"
  else
    log_info "以上為潛在違規，請確認是否應改用 taskMgr.Register()"
  fi
  return 0
}

# =============================================================================
# 主流程
# =============================================================================
main() {
  printf "Atlas 數據源憲法合規檢查\n==========================\n\n"

  local env_result=0 rate_result=0

  check_env_vars || env_result=$?
  check_gateway_registration || true
  check_rate_limits || rate_result=$?
  check_background_tasks || true

  printf "\n═══════════════════════════════════════\n"

  if [ "$OUTPUT_MODE" = "json" ]; then
    if command -v jq >/dev/null 2>&1; then
      jq -n \
        --argjson violations "${JSON_VIOLATIONS:-[]}" \
        --argjson total "$VIOLATIONS" \
        --argjson passed "$((4 - (env_result>0?1:0) - (rate_result>0?1:0)))" \
        '{status:(if $total > 0 then "violations_found" else "passed" end),total_violations:$total,checks_passed:$passed,checks_total:4}'
    else
      printf '{"status":"ok","note":"jq not available for detailed output"}\n'
    fi
  else
    local checks_passed=0
    [ "$env_result" -eq 0 ] && checks_passed=$((checks_passed + 1))
    [ "$rate_result" -eq 0 ] && checks_passed=$((checks_passed + 1))

    printf "檢查結果: %d/4 通過 (強制: env + rate_limit)\n" "$checks_passed"
    if [ "$VIOLATIONS" -gt 0 ]; then
      printf "${RED}發現 %d 處違規${NC}\n\n" "$VIOLATIONS"
      printf "修復建議:\n"
      printf "  1. os.Getenv 違規 → 改用 config.GetSecret() 或移至 config.go\n"
      printf "  2. Provider 直接建立 → 改用 gateway.Fetch(channelID)\n"
      printf "  3. 背景 goroutine → 改用 taskMgr.Register()\n"
    else
      printf "${GREEN}所有檢查通過${NC}\n"
    fi
  fi

  if [ "$env_result" -ne 0 ] || [ "$rate_result" -ne 0 ]; then
    exit 1
  fi
  exit 0
}

main "$@"
