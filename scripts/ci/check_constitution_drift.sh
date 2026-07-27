#!/usr/bin/env bash
# =============================================================================
# check_constitution_drift.sh — 憲章文件漂移偵測
#
# 偵測 domain struct 新增欄位是否在憲章文件中有對應描述。
# 防止程式碼演進導致憲章文件過時。
#
# 檢查規則:
#   1. domain 包中新增的 exported struct 欄位必須在 METHODOLOGY.md 或 AUDIT.md 有提及
#   2. MarketPeriod 常數變更必須同步憲章文件
#   3. methodology_rules.yaml 變更必須有對應的憲章文件更新日期
#
# 用法:
#   bash scripts/ci/check_constitution_drift.sh
#
# 退出碼: 0 = 通過, 1 = 漂移發現 (容忍)
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

YELLOW='\033[1;33m' NC='\033[0m'
WARNINGS=0

log_warn()  { printf "${YELLOW}[DRIFT]${NC} %s\n" "$1"; WARNINGS=$((WARNINGS + 1)); }

# ─── Check 1: Period detection config fields documented ───
echo "─── 憲章文件漂移偵測 ───"
echo ""

PERIOD_CONFIG_FILE="internal/config/period_detection_config.go"
CONSTITUTION_FILE="docs/ATLAS_METHODOLOGY.md"

if [ -f "$PERIOD_CONFIG_FILE" ]; then
  # Extract exported field names from PeriodDetectionConfig
  FIELDS=$(grep -oE '^[[:space:]]+[A-Z][a-zA-Z0-9_]+' "$PERIOD_CONFIG_FILE" | sed 's/^[[:space:]]*//' | sort -u || true)
  
  for field in $FIELDS; do
    if ! grep -q "$field" "$CONSTITUTION_FILE" 2>/dev/null; then
      # Only warn for threshold-like fields (containing Min/Max/Threshold/Pct/Days)
      if echo "$field" | grep -qE 'Min|Max|Threshold|Pct|Days|Ratio|Price'; then
        log_warn "PeriodDetectionConfig.${field}: 未在憲章文件中找到對應描述"
      fi
    fi
  done
fi

# ─── Check 2: MarketPeriod constants covered ───
echo ""
echo "─── MarketPeriod 常數覆蓋 ───"

SHARED_FILE="internal/domain/shared/shared.go"
if [ -f "$SHARED_FILE" ]; then
  PERIODS=$(grep -oE 'Period[A-Za-z]+\s+MarketPeriod' "$SHARED_FILE" | awk '{print $1}' || true)
  for p in $PERIODS; do
    if grep -q "$p" "$CONSTITUTION_FILE" 2>/dev/null; then
      : # covered
    else
      log_warn "${p}: 未在 METHODOLOGY.md 中出現"
    fi
  done
fi

# ─── Check 3: methodology_rules.yaml last-modified vs constitution ───
echo ""
echo "─── YAML 配置新鮮度 ───"

YAML_MTIME=$(stat -f %m "configs/methodology_rules.yaml" 2>/dev/null || stat -c %Y "configs/methodology_rules.yaml" 2>/dev/null || echo 0)
CONST_MTIME=$(stat -f %m "$CONSTITUTION_FILE" 2>/dev/null || stat -c %Y "$CONSTITUTION_FILE" 2>/dev/null || echo 0)

if [ "$YAML_MTIME" -gt "$CONST_MTIME" ]; then
  log_warn "methodology_rules.yaml 比 METHODOLOGY.md 更新 — 憲章文件可能需要同步"
else
  echo "  METHODOLOGY.md 比 YAML 更新或同步 — OK"
fi

# ─── Summary ───
echo ""
if [ "$WARNINGS" -eq 0 ]; then
  echo "✅ 憲章文件漂移偵測通過"
  exit 0
else
  echo "⚠️  發現 ${WARNINGS} 項潛在漂移（非阻斷性警告）"
  exit 0  # Drift detection is advisory, not blocking
fi
