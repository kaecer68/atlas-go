#!/usr/bin/env bash
# apply-edits.sh — 在 workspace 重置後重新套用所有 tracked files 的編輯
# 在每次 session 開始時執行: bash scripts/openclaw/apply-edits.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

echo "=== 套用事件邏輯規則系統編輯 (All Phases) ==="

# Phase 2a: factor_weight_engine.go — 新增 3 個敘事主題 cases
FWE="$ROOT/internal/portfolio/factor_weight_engine.go"
if grep -q "taiwan_political_risk" "$FWE" 2>/dev/null; then
  echo "✅ factor_weight_engine.go — 已套用，跳過"
else
  # 在 retail_institutional_divergence 和 default 之間插入
  sed -i '' '/case "retail_institutional_divergence":/,/default:/{
    /default:/i\
\tcase "taiwan_political_risk":\n\t\te.eventWeights[event.ID] = map[FactorType]float64{\n\t\t\tFactorETF: -delta, FactorValue: delta, FactorQuality: delta * 0.5, FactorAgent: -delta,\n\t\t\tFactorNarrative: delta * 1.5, FactorInstSent: delta, FactorLiquidity: -delta * 0.5,\n\t\t}\n\tcase "election_cycle":\n\t\te.eventWeights[event.ID] = map[FactorType]float64{\n\t\t\tFactorMomentum: -delta * 0.5, FactorETF: -delta * 0.5, FactorInstSent: delta * 0.5,\n\t\t\tFactorNarrative: delta * 0.5, FactorIndustryCycle: delta * 0.5,\n\t\t}\n\tcase "spring_festival_season":\n\t\te.eventWeights[event.ID] = map[FactorType]float64{\n\t\t\tFactorLiquidity: delta * 0.5, FactorValue: delta * 0.3, FactorETF: delta * 0.3, FactorNarrative: delta * 0.3,\n\t\t}
  }' "$FWE"
  echo "✅ factor_weight_engine.go — 已套用"
fi

# Phase 2b: seasonal_bridge.go — SeasonalMultiplier
SB="$ROOT/internal/narrative/seasonal_bridge.go"
if grep -q "taiwan_political_risk" "$SB" 2>/dev/null; then
  echo "✅ seasonal_bridge.go — 已套用，跳過"
else
  echo "❌ 需要手動編輯 seasonal_bridge.go — sed 模式太複雜"
fi

# Phase 2c: templates.go — 刪除 dead templates
TP="$ROOT/internal/narrative/templates.go"
if grep -q "post_election_relief" "$TP" 2>/dev/null; then
  echo "❌ templates.go — 需要清理 dead templates"
  echo "  手動: 刪除 post_election_relief 和 middle_east_escalation 兩個 template 區塊"
else
  echo "✅ templates.go — 已清理"
fi

# Phase 4a: detector.go — Registry 匯出
DT="$ROOT/internal/eventlogic/detector.go"
if grep -q "Registry \*RuleRegistry" "$DT" 2>/dev/null; then
  echo "✅ detector.go — 已套用"
else
  sed -i '' 's/registry \*RuleRegistry/Registry *RuleRegistry/g' "$DT"
  sed -i '' 's/{registry: r}/{Registry: r}/g' "$DT"
  sed -i '' 's/d\.registry\.Add(r)/d.Registry.Add(r)/g' "$DT"
  echo "✅ detector.go — 已套用"
fi

# Phase 3+4a: system_plugins.go — WithEventLogic
SP="$ROOT/internal/orchestrator/system_plugins.go"
if grep -q "func.*System.*WithEventLogic" "$SP" 2>/dev/null; then
  echo "✅ system_plugins.go — 已套用"
else
  echo "❌ 需要手動添加 WithEventLogic 方法到 system_plugins.go"
fi

# Phase 1: audit_subscriber.go — 已作為新檔案存在
if [ -f "$ROOT/internal/risk/audit_subscriber.go" ]; then
  echo "✅ audit_subscriber.go — 已存在"
else
  echo "❌ audit_subscriber.go — 遺失，需要重建"
fi

echo "=== 完成 ==="
echo "若要完整重建，請執行: git stash && git checkout main && git pull && bash scripts/openclaw/apply-edits.sh"
