# Sector Allocation Closure — 操作手冊

> **SA11.B dark launch operation runbook**

## Preflight Checklist

1. **Simulation sessions**: 確認 ≥20 valid simulation sessions 完成
2. **Benchmark data**: TAIEX daily returns 已載入 `FileTAIEXBenchmarkProvider`
3. **Legacy reads**: `sac.legacy.read` counter == 0
4. **Negative evidence**: `sa12-negative-evidence.sh` all 0 hits
5. **Mutation count**: Live mutation count == 0

## Promotion Gate

符合 L2.4 5-condition hard gate 模式（`docs/operations/l2-4-runbook.md` §3）：
- ✅ Feature flag `sector_allocation_closure_enabled` 預設 off
- ✅ SACMetrics 11 events 全部觀察中
- ✅ No live mutations during observation
- ✅ Rollback drill 通過
- ✅ Operator sign-off

## Rollback

```bash
# 關閉 feature flag
export SECTOR_ALLOCATION_CLOSURE_ENABLED=false
# 確認 legacy reader 正常運作
curl http://localhost:18080/api/dashboard/sector-allocation-plan
```
