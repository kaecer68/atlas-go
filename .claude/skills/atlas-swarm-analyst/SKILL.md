---
name: atlas-swarm-analyst
description: "Use when reading MiroFish Swarm simulation results, reporting market consensus, or detecting anomalies from swarm data. Triggers: swarm analysis, market consensus, fish performance, anomaly detection."
---

**用途**: 讓 OpenClaw/Hermes/claude 等 AI Agent 能讀取 MiroFish Swarm 的模擬結果，以自然語言向投資人報告市場共識、異常偵測與情境分析。

---

## 背景

MiroFish Swarm 是 Atlas 系統的平行市場模擬引擎。同時執行 100 條「魚」在 5 種市場情境（牛市、熊市、高波動、低波動、轉換期）中模擬價格路徑。每條魚使用不同的預測規則（窗口長度、門檻、情緒權重、反向偏好），並在每次模擬後透過演化機制（crossover + mutation）優化。

**輸出資料**：
- **共識方向**: 所有魚對各標的的看多/看空/中立投票
- **異常偵測**: 魚群間意見高度分歧時產生告警
- **情境參數**: 各情境的波動率與趨勢（受 Reflexivity Engine 動態調整）
- **魚群績效**: 最佳魚的準確率、演化世代數

---

## API 端點

所有端點回傳 JSON，無需認證（僅限內部儀表板使用）。

### `GET /api/dashboard/swarm-status`

回傳最新一次模擬的摘要。

```json
{
  "recorded_at": "2026-05-27T21:00:00+08:00",
  "total_fish": 100,
  "consensus_symbols": 5,
  "consensus_confidence": 0.72,
  "top_accuracy": 0.83,
  "anomaly_count": 2,
  "scenario_count": 5,
  "generations_evolved": 3
}
```

| 欄位 | 意義 |
|------|------|
| `consensus_confidence` | 整體共識信心度 [0,1] |
| `top_accuracy` | 最佳魚的準確率 [0,1]；基於模擬價格路徑的實際漲跌方向計算（非隨機） |
| `anomaly_count` | 異常偵測數量 |
| `generations_evolved` | 已演化世代數 |

### `GET /api/dashboard/swarm-consensus`

回傳逐標的共識細項。

```json
[
  {
    "symbol": "2330.TW",
    "bullish_count": 12,
    "bearish_count": 5,
    "neutral_count": 3,
    "consensus_direction": "bullish",
    "average_confidence": 0.72
  }
]
```

### `GET /api/dashboard/swarm-anomalies`

回傳異常清單。

```json
[
  {
    "type": "high_disagreement",
    "description": "Significant disagreement on 2330.TW: 45 vs 35",
    "severity": 0.78,
    "symbols": ["2330.TW"]
  }
]
```

### `GET /api/dashboard/swarm-scenarios`

回傳各情境的當前參數。

```json
[
  {
    "id": "bull_trend",
    "name": "Bull Market Trend",
    "regime": "risk_on",
    "volatility": 0.15,
    "trend": 0.001
  }
]
```

### `GET /api/dashboard/swarm-strategies`

回傳 MetaLearner 推薦的學習策略清單（最多 5 條）。

```json
[
  {
    "id": "strategy_momentum_0",
    "name": "Conservative Momentum",
    "type": "momentum",
    "score": 0.75
  }
]
```

| 欄位 | 意義 |
|------|------|
| `id` | 策略唯一識別碼 |
| `name` | 策略名稱 |
| `type` | 策略類型（momentum / adaptive / curriculum / ensemble / evolutionary） |
| `score` | 策略成功率（SuccessCount / TotalApplications） |

---

## Agent 使用指南

### 對話原則

1. **永遠先報告共識方向，再討論分歧。**
   - 「目前 Swarm 共識偏多看（信心度 72%），但 2330.TW 有顯著分歧。」
2. **提及機率時必須附上信心度來源。**
   - ❌ 「市場會漲」
   - ✅ 「模擬中共有 60% 的 fish 看多，信心度約 72%」
3. **異常偵測結果優先於樂觀敘事。**
   - 若有 anomaly，第一個回答就該提到
4. **情境參數變化要與前次對比。**
   - 「相比上一次模擬，高波動情境的波動率從 0.40 上升到 0.45」
5. **永遠不說「預測」，說「模擬共識」。**
   - ❌ 「Swarm 預測 2330.TW 會漲」
   - ✅ 「Swarm 模擬中，多數 fish 對 2330.TW 持看多共識」

### 常見對話場景

**Q: 現在市場氣氛如何？**
```
1. 呼叫 GET /api/dashboard/swarm-status
2. 從 consensus_confidence 判斷整體信心
3. 從 anomaly_count 判斷是否有異常
4. 從 top_accuracy 判斷模擬品質
5. 用自然語言組織回答
```

**Q: 哪檔標的最有爭議？**
```
1. 呼叫 GET /api/dashboard/swarm-consensus
2. 計算 bullish 與 bearish 最接近的標的
3. 交叉比對 GET /api/dashboard/swarm-anomalies
4. 回報分歧最大的標的
```

**Q: 和上次模擬有什麼不同？**
```
1. 記錄前次查詢的 snapshot 資料
2. 呼叫 GET /api/dashboard/swarm-status 取得最新
3. 對比 consensus_confidence 與 generations_evolved 變化
4. 對比 GET /api/dashboard/swarm-scenarios 的 volatility/trend 變化
```

---

## 校準框架

Swarm 模組包含參數校準框架（`internal/swarm/calibration.go`），用於比對模擬統計量與真實市場數據：

- `ComputeSimulationStats(results)` — 從模擬結果計算均值、波動率、偏度、峰度、最大回撤、Sharpe ratio、相關矩陣
- `CalibrateParameters(current, simStats, targetStats)` — 產生參數調整建議（GARCH omega/alpha/beta、跳躍過程 lambda/mu/sigma、趨勢 drift）
- `MiroFishSwarm.CalibrateAgainstTarget(target)` — 對當前魚群歷史路徑執行完整校準

校準誤差以均方根（RMS）計算，調整量採比例修正：`adjustment = error * learningRate`（預設 learningRate = 0.1）。

## 相關指令

- `GET /api/dashboard/phase3-status` — Phase 3 整體指標（含 Swarm, PRISM, Spawning, Reflexivity 的統一狀態）
