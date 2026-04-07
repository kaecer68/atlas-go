#!/bin/bash
#
# Run one full validated OpenClaw experiment round:
# propose -> execute -> judge -> metric sanity check
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# 资源监控检查
check_resources() {
  if [[ -f "${PROJECT_ROOT}/scripts/monitor/resource-guard.sh" ]]; then
    echo -e "${CYAN}[Resource Check]${NC} Checking system resources..."
    if ! bash "${PROJECT_ROOT}/scripts/monitor/resource-guard.sh" check; then
      echo -e "${YELLOW}Warning: System resources are constrained.${NC}"
      read -r -p "Continue anyway? [y/N]: " response
      if [[ ! "$response" =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 1
      fi
    fi
  fi
}

# 轮次停止条件检查
check_round_limits() {
  if [[ -f "${PROJECT_ROOT}/scripts/monitor/round-tracker.sh" ]]; then
    echo -e "${CYAN}[Round Check]${NC} Checking round limits..."
    local stop_check=$(bash "${PROJECT_ROOT}/scripts/monitor/round-tracker.sh" check 2>&1)
    if echo "$stop_check" | grep -q '"should_stop":true'; then
      echo -e "${RED}Stop condition met:${NC}"
      echo "$stop_check"
      exit 1
    fi
    # 获取当前轮次
    local current_round=$(echo "$stop_check" | grep -o '"current_round":[0-9]*' | cut -d: -f2)
    echo -e "${CYAN}[Round Tracker]${NC} Current round: ${current_round:-unknown}"
  fi
}

cd "$PROJECT_ROOT"

# 前置检查
check_resources
check_round_limits

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

PROPOSE_ARGS=("--auto")
if [ "$#" -gt 0 ]; then
  PROPOSE_ARGS+=("$@")
fi

echo -e "${CYAN}[1/4] Propose mutation brief...${NC}"
./scripts/openclaw/propose-mutation.sh "${PROPOSE_ARGS[@]}" >/tmp/openclaw_propose.out 2>&1 || {
  cat /tmp/openclaw_propose.out
  exit 1
}
cat /tmp/openclaw_propose.out

brief_path=$(ls -t data/state/mutation-briefs/*.json 2>/dev/null | head -1)
if [ -z "${brief_path:-}" ]; then
  echo -e "${RED}No mutation brief found after propose.${NC}"
  exit 1
fi

echo -e "${CYAN}[2/4] Execute experiment with brief...${NC}"
exec_out=$(go run ./cmd/execute-experiment --brief "$brief_path" 2>&1) || {
  echo "$exec_out"
  exit 1
}
echo "$exec_out"

exp_id=$(echo "$exec_out" | awk -F': ' '/^experiment:/{print $2}' | tail -1)
if [ -z "${exp_id:-}" ]; then
  echo -e "${RED}Cannot parse experiment id from execute output.${NC}"
  exit 1
fi

result_file="data/state/experiments/${exp_id}.json"
if [ ! -f "$result_file" ]; then
  echo -e "${RED}Result file not found: $result_file${NC}"
  exit 1
fi

echo -e "${CYAN}[3/4] Run replay judge...${NC}"
judge_out=$(go run ./cmd/judge-experiment -result "$result_file" 2>&1) || {
  echo "$judge_out"
  exit 1
}
echo "$judge_out"

baseline=$(echo "$judge_out" | awk -F': ' '/^baseline:/{print $2}' | tail -1)
candidate=$(echo "$judge_out" | awk -F': ' '/^candidate:/{print $2}' | tail -1)
status=$(echo "$judge_out" | awk -F': ' '/^status:/{print $2}' | tail -1)

echo -e "${CYAN}[4/4] Sanity check...${NC}"
echo "experiment: $exp_id"
echo "status: ${status:-unknown}"
echo "baseline: ${baseline:-N/A}"
echo "candidate: ${candidate:-N/A}"

if [ "${baseline:-}" = "0.000000" ] && [ "${candidate:-}" = "0.000000" ]; then
  echo -e "${YELLOW}Warning: baseline and candidate are both zero. This round is valid but not discriminative.${NC}"
  echo "Try:"
  echo "  1) choose a different --agent"
  echo "  2) run backtest-window on another date range"
  echo "  3) use --type risk_rule_change for stronger candidate differences"
else
  echo -e "${GREEN}Validated round completed with non-trivial baseline/candidate metrics.${NC}"
fi

# 记录本轮结果到轮次追踪器
if [[ -f "${PROJECT_ROOT}/scripts/monitor/round-tracker.sh" ]]; then
  round=$(bash "${PROJECT_ROOT}/scripts/monitor/round-tracker.sh" check 2>&1 | grep -o '"current_round":[0-9]*' | cut -d: -f2)
  round=${round:-0}
  result="${status:-rejected}"
  
  # 提取 agent 和 mutation type
  agent=$(echo "$brief_path" | grep -oP 'brief-\K[^-]+' || echo "unknown")
  mutation_type=$(cat "$brief_path" 2>/dev/null | jq -r '.mutation_type // "unknown"' || echo "unknown")
  
  # 计算改进值
  if [[ -n "$baseline" && -n "$candidate" ]]; then
    improvement=$(echo "$candidate - $baseline" | bc 2>/dev/null || echo "0")
  else
    improvement="0"
  fi
  
  bash "${PROJECT_ROOT}/scripts/monitor/round-tracker.sh" record "$round" "$result" "$exp_id" "$agent" "$mutation_type" "$improvement" 2>/dev/null || true
  echo -e "${CYAN}[Round Recorded]${NC} Round $round: $result (improvement: $improvement)"
fi

echo ""
echo "Next: Run ./scripts/openclaw/run-validated-round.sh for another round"
echo "      or check stats: ./scripts/monitor/round-tracker.sh stats"
