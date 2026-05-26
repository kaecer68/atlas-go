#!/usr/bin/env bash
# =============================================================================
# check_agent_prompts.sh — 驗證 agents.json 中每個 enabled agent 都有 prompt 檔案
#
# 檢查項目:
#   1. configs/agents.json 存在且為合法 JSON
#   2. 每個 enabled: true 的 agent 的 promptFile 路徑存在
#   3. promptFile 路徑必須在 prompts/agents/ 下（防止路徑遍歷）
#
# 用法:
#   bash scripts/ci/check_agent_prompts.sh          # 完整檢查
#   bash scripts/ci/check_agent_prompts.sh --json   # JSON 輸出 (CI 整合用)
#
# 退出碼: 0 = 通過, 1 = 違規發現
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

OUTPUT_MODE="${1:-text}"
VIOLATIONS=0

if [ -t 1 ] && [ "$OUTPUT_MODE" != "json" ]; then
  RED='\033[0;31m' GREEN='\033[0;32m' YELLOW='\033[1;33m' NC='\033[0m'
else
  RED='' GREEN='' YELLOW='' NC=''
fi

log_pass()  { printf "${GREEN}[PASS]${NC} %s\n" "$1"; }
log_fail()  { printf "${RED}[FAIL]${NC} %s\n" "$1"; }
log_info()  { printf "       %s\n" "$1"; }

AGENTS_JSON="configs/agents.json"

# =============================================================================
# 檢查 1: agents.json 存在且可解析
# =============================================================================
check_config_exists() {
  printf "\n═══ 檢查 1/3: agents.json 存在性 ═══\n"

  if [ ! -f "$AGENTS_JSON" ]; then
    log_fail "$AGENTS_JSON 不存在"
    return 1
  fi

  if ! command -v python3 >/dev/null 2>&1 && ! command -v jq >/dev/null 2>&1; then
    log_fail "需要 python3 或 jq 來解析 JSON"
    return 1
  fi

  log_pass "$AGENTS_JSON 存在"
  return 0
}

# =============================================================================
# 檢查 2: 每個 enabled agent 的 promptFile 存在
# =============================================================================
check_prompt_files() {
  printf "\n═══ 檢查 2/3: enabled agent → prompt 檔案 ═══\n"

  local found_any=0
  local total=0
  local ok=0
  local missing=0

  # Use python3 for robust JSON parsing
  while IFS='|' read -r agent_id prompt_file enabled; do
    total=$((total + 1))
    if [ "$enabled" != "true" ]; then
      continue
    fi

    if [ -z "$prompt_file" ] || [ "$prompt_file" = "null" ]; then
      log_fail "$agent_id — enabled=true 但缺少 promptFile 欄位"
      found_any=1
      missing=$((missing + 1))
      continue
    fi

    if [ ! -f "$prompt_file" ]; then
      log_fail "$agent_id — promptFile '$prompt_file' 不存在"
      found_any=1
      missing=$((missing + 1))
    else
      ok=$((ok + 1))
    fi
  done < <(python3 -c "
import json, sys
with open('$AGENTS_JSON') as f:
    data = json.load(f)
for a in data.get('agents', []):
    pf = a.get('promptFile', '')
    enabled = str(a.get('enabled', False)).lower()
    print(f\"{a['id']}|{pf}|{enabled}\")
" 2>/dev/null)

  if [ "$found_any" -eq 0 ]; then
    local enabled_count
    enabled_count=$(python3 -c "
import json
with open('$AGENTS_JSON') as f:
    data = json.load(f)
print(sum(1 for a in data.get('agents', []) if a.get('enabled')))
")
    log_pass "所有 ${enabled_count} 個 enabled agent 的 prompt 檔案都存在 ($total total)"
  else
    log_info "通過: $ok, 缺失: $missing"
  fi

  return $found_any
}

# =============================================================================
# 檢查 3: promptFile 路徑安全性（必須在 prompts/agents/ 下）
# =============================================================================
check_path_safety() {
  printf "\n═══ 檢查 3/3: promptFile 路徑安全性 ═══\n"

  local found_any=0

  while IFS='|' read -r agent_id prompt_file enabled; do
    if [ "$enabled" != "true" ]; then
      continue
    fi
    if [ -z "$prompt_file" ] || [ "$prompt_file" = "null" ]; then
      continue
    fi

    # Must start with prompts/agents/
    case "$prompt_file" in
      prompts/agents/*) ;;
      *)
        log_fail "$agent_id — promptFile '$prompt_file' 不在 prompts/agents/ 下"
        found_any=1
        ;;
    esac

    # Must not contain path traversal
    case "$prompt_file" in
      *..*)
        log_fail "$agent_id — promptFile '$prompt_file' 包含路徑遍歷 (..)"
        found_any=1
        ;;
    esac
  done < <(python3 -c "
import json
with open('$AGENTS_JSON') as f:
    data = json.load(f)
for a in data.get('agents', []):
    pf = a.get('promptFile', '')
    enabled = str(a.get('enabled', False)).lower()
    print(f\"{a['id']}|{pf}|{enabled}\")
" 2>/dev/null)

  if [ "$found_any" -eq 0 ]; then
    log_pass "所有 promptFile 路徑均安全 (在 prompts/agents/ 下，無路徑遍歷)"
  fi

  return $found_any
}

# =============================================================================
# 主流程
# =============================================================================
main() {
  printf "Agent Prompt 檔案完整性檢查\n===========================\n\n"

  local r1=0 r2=0 r3=0

  check_config_exists || r1=$?
  if [ "$r1" -ne 0 ]; then
    printf "\n${RED}無法讀取 agents.json，終止檢查${NC}\n"
    exit 1
  fi

  check_prompt_files || r2=$?
  check_path_safety || r3=$?

  printf "\n═══════════════════════════════════════\n"

  local checks_passed=0
  [ "$r1" -eq 0 ] && checks_passed=$((checks_passed + 1))
  [ "$r2" -eq 0 ] && checks_passed=$((checks_passed + 1))
  [ "$r3" -eq 0 ] && checks_passed=$((checks_passed + 1))

  local total_violations=0
  [ "$r2" -ne 0 ] && total_violations=$((total_violations + 1))
  [ "$r3" -ne 0 ] && total_violations=$((total_violations + 1))

  if [ "$OUTPUT_MODE" = "json" ]; then
    if command -v jq >/dev/null 2>&1; then
      jq -n \
        --argjson checks_passed "$checks_passed" \
        --argjson total_violations "$total_violations" \
        '{status:(if $total_violations > 0 then "violations_found" else "passed" end),checks_passed:$checks_passed,checks_total:3,total_violations:$total_violations}'
    else
      printf '{"status":"ok"}\n'
    fi
  else
    printf "檢查結果: %d/3 通過\n" "$checks_passed"
    if [ "$total_violations" -gt 0 ]; then
      printf "${RED}發現 %d 類違規${NC}\n\n" "$total_violations"
      printf "修復建議:\n"
      printf "  1. 缺少 promptFile → 在 agents.json 中為 enabled agent 加上 promptFile 欄位\n"
      printf "  2. prompt 檔案不存在 → 建立對應的 prompts/agents/<name>.md\n"
      printf "  3. 路徑不安全 → promptFile 必須在 prompts/agents/ 下，禁止 .. 路徑遍歷\n"
    else
      printf "${GREEN}所有檢查通過${NC}\n"
    fi
  fi

  if [ "$r2" -ne 0 ] || [ "$r3" -ne 0 ]; then
    exit 1
  fi
  exit 0
}

main "$@"
