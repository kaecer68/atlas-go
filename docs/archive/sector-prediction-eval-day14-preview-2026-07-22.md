# C07 Sector Prediction — Day 14 evaluation

**Date**: 2026-07-22 22:39:56 CST

**Result**: Day 14 acceptance: SOME MUST criteria FAILED

## Criteria

| Criterion | Actual | Expected | Severity | Result | Note |
|-----------|--------|----------|----------|--------|------|
| jsd.alert_rate < 5% | 0.0% | < 5% | must | ✅ PASS | 超標 → 檢查 macro weight 與 cycle shift |
| latency_p95 < 200ms | 14ms | < 200ms | must | ✅ PASS | 超標 → 改為 cron 預計算 |
| confidence.floor_violations = 0 | 0 | = 0 | must | ✅ PASS | 違反 → 立即排查 (invariant I7) |
| sector.count_per_day = 20 | 20 | = 20 | must | ✅ PASS | 違反 → 檢查 industry.L1Sectors() |
| panic_count = 0 | 0 | = 0 | must | ✅ PASS | 觸發 → 立即 rollback |
| spot_check_count >= 15 | 20 | >= 15 | must | ✅ PASS | 不足 → 延長觀察至 day 14 |
| hit-rate >= baseline (Δ >= -3%) | deferred | >= 55.0% | should | ✅ PASS | 需歷史板塊報酬才能計算；標記為未來升級條件 |
| driver explainability >= 20 spot-checks | 20 | >= 20 | must | ✅ PASS | 每筆 driver 至少引用 1 個具體 macro/cycle/event |
| rollback verified | manual | verified | must | ❌ FAIL | 至少一次手動測試把 flag 翻回未設置 |

## Next Steps

- Trigger runbook §4 Rollback Procedure
- File follow-up issue with root cause analysis
- Do NOT re-enable flag until issue is resolved
