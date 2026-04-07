# 執行實驗循環測試報告

## 測試執行記錄

### 1. 資料準備 ✅
```bash
# 生成 1,280 行資料（90天，20檔股票）
python3 generate_extended_data.py
# 結果: 1,280 行 ✅
```

### 2. Backtest 測試 ✅
```bash
# 新資料產生 527 outcomes
go run ./cmd/backtest-window -start 2026-02-01 -end 2026-02-28
# 結果: 527 outcomes ✅
```

### 3. Experiment 執行 ✅
```bash
# 自動生成 experiment
go run ./cmd/execute-experiment
# 結果: exec-etf-rotation-01-1774965708 (prompt_tightening)
```

### 4. Judge 結果 ❌
```bash
./scripts/openclaw/judge-latest.sh --auto
# 結果: SKIP (無改善)
# Baseline: 0, Candidate: 0
```

## 關鍵發現

### 資料問題 ✅ 已解決
- 從 50 行 → 1,280 行 (+2,460%)
- Backtest outcomes: 527 (足夠統計顯著)

### Mutation 問題 ❌ 仍然存在
- 自動生成的都是 `prompt_tightening`
- 即使有足夠資料，還是無法產生改善
- Baseline = Candidate (無差異)

## 結論

| 組件 | 狀態 | 說明 |
|------|------|------|
| 資料 | ✅ 足夠 | 1,280 行，90天 |
| Backtest | ✅ 正常 | 527 outcomes |
| Judge | ✅ 正常 | 可判斷 reject/accept |
| **Mutation** | **❌ 無效** | **全是 prompt_tightening，無改善** |

**根本原因**: 系統無法自動生成 `risk_rule_change` 類型的 mutation。

## 給 OpenClaw 的提示詞

見 `OPENCLAW_FINAL_PROMPT.md`
