#!/usr/bin/env bash
# =============================================================================
# check_strategy_path.sh — 策略推薦路徑稽核
#
# 確保所有策略推薦路徑經過 MethodologyAdvisor.FilterStrategies()，
# 避免 RISK_OFF 時期仍推薦 growth/momentum。
#
# 檢查規則:
#   1. RankedStrategies() / GetRecommendations() 呼叫點必須經過 FilterStrategies
#   2. buildPremiumStrategy() 必須使用 MethodologyAdvisor
#   3. Tier gating 不應 hardcode 策略 ID
#
# 用法:
#   bash scripts/ci/check_strategy_path.sh
#
# 退出碼: 0 = 通過, 1 = 違規發現
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

RED='\033[0;31m' GREEN='\033[0;32m' YELLOW='\033[1;33m' NC='\033[0m'
VIOLATIONS=0

log_pass()  { printf "${GREEN}[PASS]${NC} %s\n" "$1"; }
log_fail()  { printf "${RED}[FAIL]${NC} %s\n" "$1"; VIOLATIONS=$((VIOLATIONS + 1)); }
log_warn()  { printf "${YELLOW}[WARN]${NC} %s\n" "$1"; }

# ─── Check 1: Recommender uses MethodologyAdvisor ───
echo "─── 策略路徑稽核 ───"
echo ""

RECOMMENDER_FILE="internal/recommender/handler.go"
if grep -q "MethodologyAdvisor\|FilterStrategies\|AllowedStrategies" "$RECOMMENDER_FILE"; then
  log_pass "recommender: MethodologyAdvisor 已接線"
else
  log_fail "recommender: 未找到 MethodologyAdvisor 引用"
fi

# ─── Check 2: Hardcoded strategy lists ───
echo ""
echo "─── Hardcoded 策略清單 ───"

# Check for hardcoded strategy gating without period awareness
HARDCODED=$(grep -rn '"all_weather"\|"defensive"\|"growth"\|"momentum"\|"value"' internal/ --include="*.go" \
  | grep -v "_test.go" \
  | grep -v "methodology/" \
  | grep -v "config/" \
  | grep -v "domain/shared" \
  | grep -v "portfolio/regime.go" \
  | grep -v "orchestrator/executor_pipeline.go" \
  | grep -v "orchestrator/executor_types.go" \
  || true)

if [ -n "$HARDCODED" ]; then
  echo "$HARDCODED" | while IFS= read -r line; do
    log_warn "hardcoded strategy: $line"
  done
  echo ""
  log_warn "上述 hardcoded 策略 ID 可能未經時期過濾（若已由 MethodologyAdvisor 包裝則安全）"
else
  log_pass "無可疑 hardcoded 策略 ID（已排除已知安全位置）"
fi

# ─── Check 3: Tier gating uses period-aware filtering ───
echo ""
echo "─── Tier Gating 時期感知 ───"

TIER_FILE="internal/recommender/handler.go"
if grep -q "FilterStrategies\|AllowedStrategies" "$TIER_FILE"; then
  log_pass "recommender: tier gating 使用時期過濾"
else
  # Check if tier gating exists but without period filtering
  if grep -q "TierRegistered\|TierPremium\|tier" "$TIER_FILE"; then
    log_warn "recommender: tier gating 存在但未確認時期過濾"
  fi
fi

# ─── Check 4: buildPremiumStrategy path ───
echo ""
echo "─── buildPremiumStrategy 路徑 ───"

BUILD_FILES=$(grep -rn "buildPremiumStrategy\|buildStrategy\|GetRecommendations" internal/ --include="*.go" -l | grep -v "_test.go" || true)
for f in $BUILD_FILES; do
  if grep -q "FilterStrategies\|Advisor\|advisor" "$f"; then
    log_pass "$f: 包含時期過濾"
  else
    log_warn "$f: 建議確認是否經過 MethodologyAdvisor"
  fi
done

# ─── Summary ───
echo ""
if [ "$VIOLATIONS" -eq 0 ]; then
  echo "✅ 策略路徑稽核通過"
  exit 0
else
  echo "❌ 策略路徑稽核發現 ${VIOLATIONS} 項違規"
  exit 1
fi
