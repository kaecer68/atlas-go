#!/usr/bin/env bash
# =============================================================================
# check_methodology_constitution.sh — ATLAS 方法論憲章合規檢查
#
# 檢查:
#   1. methodology_rules.yaml 存在且七時期完整
#   2. 每個時期有 strategies 映射
#   3. 每個時期有 cash_reserve_pct
#   4. YAML risk_level 與 PeriodToRiskLevel() 一致
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

RED='\033[0;31m' GREEN='\033[0;32m' YELLOW='\033[1;33m' NC='\033[0m'
VIOLATIONS=0

log_pass()  { printf "${GREEN}[PASS]${NC} %s\n" "$1"; }
log_fail()  { printf "${RED}[FAIL]${NC} %s\n" "$1"; VIOLATIONS=$((VIOLATIONS + 1)); }
log_warn()  { printf "${YELLOW}[WARN]${NC} %s\n" "$1"; }

YAML_FILE="configs/methodology_rules.yaml"

echo "─── 方法論憲章檢查 ───"
echo ""

if [ ! -f "$YAML_FILE" ]; then
  log_fail "YAML 配置不存在: $YAML_FILE"
  exit 1
fi
log_pass "YAML 配置存在: $YAML_FILE"

# Only check the 7 known period IDs (not indicator IDs)
PERIODS="downturn turnaround_up bull plateau consolidation turnaround_down black_swan"

# ─── Check: All seven periods present ───
echo ""
echo "─── 七時期完整性 ───"
for p in $PERIODS; do
  if grep -q "^  - id: ${p}$" "$YAML_FILE"; then
    log_pass "$p"
  else
    log_fail "$p: NOT in YAML regimes"
  fi
done

# ─── Strategy mapping ───
echo ""
echo "─── 策略映射 ───"
for p in $PERIODS; do
  # Extract from "  - id: $p" to next "  - id:" at same indentation
  BLOCK=$(sed -n "/^  - id: ${p}$/,/^  - id:/p" "$YAML_FILE" | sed '$d')
  
  if echo "$BLOCK" | grep -q 'primary:'; then
    STRATEGIES=$(echo "$BLOCK" | grep -A5 'primary:' | grep -E '^\s+- ' | sed 's/.*- //' | tr -d ' ' | tr '\n' ',' | sed 's/,$//')
    if [ -n "$STRATEGIES" ]; then
      log_pass "${p}: [${STRATEGIES}]"
    else
      log_warn "${p}: strategies.primary is empty"
    fi
  else
    log_fail "${p}: no strategies.primary"
  fi
done

# ─── Cash reserve ───
echo ""
echo "─── 現金部位建議 ───"
for p in $PERIODS; do
  CASH=$(grep -A80 "^  - id: ${p}$" "$YAML_FILE" | grep 'cash_reserve_pct:' | head -1 | sed 's/.*: //' | tr -d ' ')
  if [ -n "$CASH" ]; then
    log_pass "${p}: ${CASH}%"
  else
    log_fail "${p}: cash_reserve_pct not found"
  fi
done

# ─── Risk level mapping ───
echo ""
echo "─── Period→RiskLevel 映射 ───"
declare -A EXPECTED_RISK=(
  [downturn]=orange
  [turnaround_up]=yellow
  [bull]=yellow
  [plateau]=yellow
  [consolidation]=yellow
  [turnaround_down]=orange
  [black_swan]=red
)

for p in $PERIODS; do
  ACTUAL=$(grep -A80 "^  - id: ${p}$" "$YAML_FILE" | grep 'risk_level:' | head -1 | sed 's/.*: //' | tr -d ' ')
  EXPECTED="${EXPECTED_RISK[$p]}"
  if [ "$ACTUAL" = "$EXPECTED" ]; then
    log_pass "${p}: ${ACTUAL} (matches PeriodToRiskLevel)"
  elif [ -n "$ACTUAL" ]; then
    log_fail "${p}: YAML=${ACTUAL}, code=${EXPECTED}"
  else
    log_warn "${p}: risk_level not in YAML (code=${EXPECTED})"
  fi
done

echo ""
if [ "$VIOLATIONS" -eq 0 ]; then
  echo "✅ 方法論憲章檢查通過"
  exit 0
else
  echo "❌ 方法論憲章檢查發現 ${VIOLATIONS} 項違規"
  exit 1
fi
