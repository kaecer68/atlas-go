# Risk Rule Change 執行報告

## 執行摘要

**策略**: B - 嘗試 risk_rule_change mutation ✅ **成功**

## 約束變更詳情

| 約束項 | 變更前 | 變更後 | 變化 |
|--------|--------|--------|------|
| MinRecommendationConviction | 0 | 60 | +60 |
| MaxPositionWeight | 0.18 | 0.15 | -17% |
| MinTradableVolume | 1,000,000 | 1,500,000 | +50% |
| ConvictionFloor | 50 | 60 | +10 |

## Backtest 結果比較

| 指標 | 變更前 | 變更後 | 差異 |
|------|--------|--------|------|
| Outcomes | 367 | 383 | +16 (+4.4%) |
| Worst Agent Sharpe | 6.775633 | 6.908733 | +0.133 (+2.0%) |

## 關鍵發現

✅ **約束變更產生了實質差異**
- Outcomes 增加 16 個（過濾減少了）
- Worst agent Sharpe 提升 2%
- 說明更嚴格的約束確實改變了行為

⚠️ **但需要驗證是否改善**
- 需要比較完整的 scorecard
- 需要看 portfolio 層級的指標
- 需要看是否有更少但更好的交易

## 當前 Baseline

```json
{
  "Version": 3,
  "Constraints": {
    "MinRecommendationConviction": 60,
    "MaxPositionWeight": 0.15,
    "MinTradableVolume": 1500000
  },
  "ExecutionPolicy": {
    "ConvictionFloor": 60
  }
}
```

## 後續建議

### 立即執行
```bash
# 1. 執行完整 backtest 比較
./scripts/openclaw/status.sh

# 2. 如果有改善，記錄 promotion
./scripts/openclaw/decide.sh --reason "Applied risk_rule_change: conviction 0→60, position weight 0.18→0.15"
```

### 監控指標
- Sharpe-like score 是否改善
- Turnover 是否降低（交易次數減少）
- Drawdown 是否減少
- Hit rate 是否提升

## 結論

✅ **Risk Rule Change 策略成功執行**
- 約束已更新至 Version 3
- Backtest 顯示行為確實改變
- 需要進一步驗證績效是否改善

**這證明了**：約束變更（risk_rule_change）比 prompt_tightening 更能產生實質差異。
