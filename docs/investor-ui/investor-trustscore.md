# Atlas 投資人 TrustScore — 信任分數系統

**版本**: 1.0  
**日期**: 2026-06-02  
**成熟度**: X（experimental）  
**父技能**: `atlas-investor-ui`

---

## 一、描述

本技能定義 TrustScore 信任分數系統的設計——將分散在多個模組的信任相關指標，彙總為一個投資人可理解的 0-100 分數。

投資人核心問題：「這個系統的建議，我該多相信？」

---

## 二、現狀：數據分散在五處

| 數據 | 位置 | 格式 |
|------|------|------|
| 模型校準品質 | `risk/self_calibrate.go` → CalibrationReport（F1 + precision/recall） | Go struct |
| 推薦歷史命中率 | `ledger/` outcomes vs ForwardReturn | JSONL |
| Sharpe 穩定性 | `internal/experiment/` SharpestabilityCheck | 內部計算 |
| 數據品質 | `portfolio/factor_engine.go` fallback 比例 | 追蹤中（無匯總） |
| 回撤保護 | `risk/macro_aware_drawdown.go` 觸發歷史 | 事件記錄 |

這些數據從未被匯總成一個投資人可理解的指標。

---

## 三、設計

### 新模組：`internal/trustscore/`

**成熟度標記**：X（experimental）— 初始版本

```
internal/trustscore/
├── doc.go           # Maturity: experimental
├── calculator.go    # TrustScoreCalculator
└── types.go         # TrustScore, DimensionScore
```

### 五維度加權

| 維度 | 權重 | 資料來源 | 說明 |
|------|------|----------|------|
| 模型校準品質 | 25% | `self_calibrate.go` → F1 score | Bayesian optimizer 的校準精度 |
| 推薦命中率 | 30% | `ledger/` ForwardReturn 對比 | 推薦方向正確的比例 |
| Sharpe 穩定性 | 20% | 30 日滾動 Sharpe 方差 | Sharpe 近期是否退化 |
| 數據品質 | 15% | 各因子 `IsFallback` 比例 | fallback 越多，分數越低 |
| 回撤保護 | 10% | `macro_aware_drawdown.go` 觸發有效性 | 觸發後是否成功降低損失 |

### API

```
GET /api/client/trust-score

Response:
{
  "overall": 78,
  "trend": "up",
  "dimensions": {
    "calibration":    {"score": 85, "weight": 25},
    "hit_rate":       {"score": 71, "weight": 30},
    "sharpe_stability":{"score": 82, "weight": 20},
    "data_quality":   {"score": 90, "weight": 15},
    "drawdown_guard": {"score": 65, "weight": 10}
  }
}
```

---

## 四、前端呈現（推薦詳情頁內嵌）

```
信任分數：78/100 ↑

模型校準：85/100 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 85%
推薦命中：71/100 ━━━━━━━━━━━━━━━━━━━━━━━━━ 71%
Sharpe：  82/100 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 82%
數據品質：90/100 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 90%
回撤保護：65/100 ━━━━━━━━━━━━━━━━━━━ 65%
```

趨勢箭頭：↑ 上升 / → 持平 / ↓ 下降（最近 7 天 vs 前 7 天對比）

---

## 五、計算時機

- 每日盤後（BackgroundTaskManager 排程）自動重新計算
- 支援手動觸發（API `POST /api/client/trust-score/refresh`）
- 歷史 TrustScore 保留（供趨勢圖使用）

---

## 六、信任分數的實務限制

信任分數不是精確科學。它是一個**溝通工具**，不是交易信號。設計時必須考慮：

- **信心度不確定性**：5 個維度的權重本身是主觀設定，未來可用 backtest 校準
- **樣本偏差**：命中率只反映已有推薦，不代表未來預測能力
- **時間滯後**：校準品質是過去數據，無法預測 regime change 後的表現
- **不要讓 TrustScore 成為唯一的信任來源**：結合圖表、歷史記錄、每日摘要，形成完整的信任證據鏈

---

## 七、與其他技能關聯

| 技能 | 關聯 |
|------|------|
| `atlas-investor-ui` | 信任金字塔第 2-4 層的量化指標 |
| `atlas-investor-pages` | 儀表板、風險報告中使用 TrustScore 顯示 |
| `atlas-risk-management` | 回撤保護維度直接取自 risk 模組 |
| `atlas-core-architecture` | 理解模組後設計 TrustScore 整合架構 |
| `atlas-investor-roadmap` | Phase B P1 項目 |
