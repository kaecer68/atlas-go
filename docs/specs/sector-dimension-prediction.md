# C07 — 板塊維度預測（Per-Sector Direction Prediction）

> **狀態**：Draft v0.1，待 owner 過目
> **作者**：atlas-dev（agent-driven, Sisyphus pattern）
> **建立日期**：2026-07-16
> **優先級**：P2（風險面顯示誤導已在 v0.0.0.32 透過 `affected_industries` 標籤緩解；本次要的是真正的 per-sector 方向預測）
> **對應 manifest 條目**：docs/archive/2026-07-16-ui-data-regime-fix-manifest.md C07

---

## 1. 背景與目標

### 1.1 現狀（v0.0.0.32+）

`/api/events/prediction` 目前給出：

- 5 日整體錢潮方向（inflow / outflow / neutral）+ confidence + distribution
- `active_events[]` — 未來事件，含 `affected_industries[]`（哪些板塊受事件影響）
- `driving_events[]` per prediction（哪個事件觸發）

前端 `capital_predictions.js` 用 `affected_industries` 做「板塊 × 日期」熱度圖：
每個 cell 只顯示「該日該板塊是否受事件影響 + 該整體日方向」，不是「板塊自己的方向預測」。

### 1.2 為何需要 C07

風險：
- `affected_industries` 是 **事件影響標籤**，不是「板塊自己的方向預測」。當一個 inflow 日搭配半導體 event，heatmap 把半導體 cell 標成偏多 — 但當日其實可能資金從半導體流出（流入金融、PCB），cell 反而誤導。
- 投資人真正想看的是：「**5 日後各板塊自己會往哪個方向流**」（含 confidence），用於部位調整。

目標：
- 為每個 L1 板塊（20 個）給出 5 日方向（inflow / outflow / neutral）+ confidence + distribution
- 與既有整體預測 **整合**：板塊加權應該對得上整體信心（例如整體 high inflow 不可能所有板塊都 outflow）

### 1.3 非目標

- 不做 daily factor weight（Darwinian 那套已有）
- 不做 stock-level 預測（recommender/llm-agent 已有）
- 不做 sector regime detection（`internal/industry/cycle.go` 已有 cycle_calibration）

---

## 2. 訓練資料需求

| 資料 | 用途 | 來源 | 現狀 |
|------|------|------|------|
| 板塊日報酬序列 | 訓練/驗證 per-sector 模型 | TWSE 指數編輯（上市/上櫃各類股指數）+ Fugle 指數 | `data/sector_data/` 有 partial，需要驗證涵蓋率 |
| 事件 → 板塊影響對應 | 推導 event-driven sector shift | `internal/eventdriven/predictor.go` 的 `affected_industries` 標籤 | 已有，但是 sector_id 字串不一致，需要 normalize 到 canonical L1 sector |
| 板塊 macro exposure | 推導 structural tilt | `internal/industry` 既有 sector data bridge | 已有 |
| ETF 持倉板塊歸屬 | 推導 ETF rotation impact | `internal/eventdriven/predictor.go:etfRebalanceEstimates` 已有 stock-level，但缺 stock → sector mapping | **缺** |

### 2.1 缺口

1. **Sector → Event win-rate 歷史**：需要拉 2-3 年事件當日 N 日後（+1 / +3 / +5 / +10）板塊報酬，計算「事件類型 + 板塊 → 後續方向」的歷史命中率。**完全沒有**，需要寫新的 backfill 流程。
2. **Stock → Sector canonical mapping**：現在的 `affected_industries` 混雜 stock symbol（2330）與 sector name（半導體）。需要先 normalize。
3. **板塊日報酬完整歷史**：目前 `data/sector_data/` 可能只有 snapshot，缺時序。需驗證。

### 2.2 訓練/驗證流程（建議）

```text
1. Backfill: 從 TWSE / Fugle 拉 2022-01 ~ 2026-07 各 L1 板塊日報酬 → parquet/jsonl
2. Backfill: 從現有 event calendar 用 normalized event_type + sector 計算每個事件的後續 1/3/5/10 日板塊實際方向
3. Train: per (event_type, sector) cell → historical baseline direction prob
4. Calibrate: Bayesian shrinkage — 樣本 < 30 個事件的 cell 退回 uniform prior
5. Hold-out: 2026-01 ~ 2026-07 作為 out-of-sample，計算 hit-rate 與 Brier score
```

---

## 3. 功能設計

### 3.1 資料模型

```go
// 新增 SectorDirection 內部型別於 internal/eventdriven/types.go
type SectorDirection struct {
    SectorID    string                       `json:"sector_id"`    // 20 個 L1 sector 之一
    SectorName  string                       `json:"sector_name"`  // 中文 label
    Direction   string                       `json:"direction"`    // "inflow"|"outflow"|"neutral"
    Confidence  float64                      `json:"confidence"`   // 0-1
    Distribution PredictionDistribution      `json:"distribution"`
    Drivers     []string                     `json:"drivers"`      // 觸發原因（event 名稱、macro signal）
}

// PredictionReport 新增 sector_predictions 欄位（C07）
type PredictionReport struct {
    // ... 既有欄位 ...
    SectorPredictions []SectorDayPrediction   `json:"sector_predictions"`
}

type SectorDayPrediction struct {
    Date    string           `json:"date"`
    Sectors []SectorDirection `json:"sectors"` // 該日 20 個板塊的方向預測
}
```

### 3.2 演算法（hybrid）

對每個板塊 S 與預測日 D：

```
direction_raw = baseline_probability(S, similar_events)
             + macro_exposure_adjustment(S, regime)
             + sector_cycle_shift(S, CycleTracker.CurrentPosition(S))
direction_final = sigmoid(direction_raw + static_calibration)
```

Components:
1. **Historical baseline**：`P(direction | event_type, sector)` from step 4 of training flow
2. **Macro exposure**：用 `internal/macro/MacroDataSnapshot` 與板塊 macro beta 推導 structural tilt
3. **Sector cycle shift**：`internal/industry/cycle.go` 的 cycle position（復甦 / 擴張 / 收縮 / 衰退）

加權：
- baseline 60%
- macro 25%
- cycle 15%

Confidence 加權整合：
```
confidence = 1 - entropy(distribution)
```
Entropy 越低 → confidence 越高。

### 3.3 API 契約

延續 C06 的方向：`/api/events/prediction` JSON 內新增 `sector_predictions` 欄位（必出現，empty → `[]`）。

範例 response fragment:
```json
{
  "sector_predictions": [
    {
      "date": "2026-07-17",
      "sectors": [
        { "sector_id": "semiconductor", "sector_name": "半導體",
          "direction": "inflow", "confidence": 0.62,
          "distribution": {"inflow": 0.55, "neutral": 0.30, "outflow": 0.15},
          "drivers": ["TSMC 月營收優於預期", "外資金流連 3 日買超"] },
        { "sector_id": "financials", "sector_name": "金融",
          "direction": "neutral", "confidence": 0.45,
          "distribution": {"inflow": 0.30, "neutral": 0.50, "outflow": 0.20},
          "drivers": [] }
      ]
    }
  ]
}
```

### 3.4 前端整合

`capital_predictions.js` 新增區段「**板塊 × 日期 方向預測**」（與既有 heatmap 並列）。

- 每個 cell 顯示方向（▲ / — / ▼）+ confidence (% + heat color)
- Hover 顯示 `drivers[]` 列表
- 點擊開 detail（同現有機制）
- 板塊加總列可對應驗證整體信心（一致性檢查）

---

## 4. 測試策略

| Level | 工具 | 內容 |
|-------|------|------|
| Unit | `go test ./internal/eventdriven/... -run Test_SectorPrediction` | 各 component 獨立測試（baseline / macro / cycle integration） |
| Integration | replay-based | 跑 2026-Q2 replay windows，比對 hit-rate ≥ baseline + 5% |
| Backtest | `cmd/run-experiment` with `sector-prediction-v0` config | 對照 `sector-prediction-disabled` baseline |
| Frontend | `__tests__/sector_predictions.test.mjs`（new） | 表格渲染、配色、drivers 列表、escape |
| LLM judge | （實驗性） | 用 LLM 評估預測 rationale 是否 human-readable |

### 4.1 風險與 Guardrails

- **Calibration drift**：holistic 信心 / Brier score 監控（沿用 `internal/calibration/`）。Drift > 0.1 自動降權 baseline。
- **Sector ID canonicalization**：`SEED_SECTORS` 不允許下游硬編碼 sector_id string。所有 mapping 必須經過 `industry.NormalizeSectorID(id)` wrapper。
- **API rate limit**：sector_predictions 每個板塊每天 1 次計算，cron 排程避免 hot-path。

---

## 5. 風險評估

| Risk | Mitigation |
|------|------------|
| 訓練資料缺（板塊日報酬時序） | Phase 0 backfill，先驗證可用性再投入模型設計 |
| Sector ID 字串不一致 | 全部走 canonical L1 ID，pipeline 入口加 normalization layer |
| 模型自信過頭（overconfident） | Bayesian shrinkage + 經驗校正 |
| 前端渲染大量 cell（20 sectors × 5 days = 100）可能慢 | 用 virtual scrolling 或 limit 顯示前 5 sectors by confidence |
| 整體信心 vs 板塊加權不一致 | 計算後跑 consistency check，severity > threshold 寫 alert |
| Sora-LLM induced over-confidence | 顯卡信心分位加 floor（例：minimum 0.4 一致於人類謙遜） |

---

## 6. Rollout 計畫

仿 L2.4 五條件硬門 + observation window pattern：

1. **Phase 0**（1 週）: Backfill 板塊日報酬 + 事件 → 板塊命中表
2. **Phase 1**（2 週）: 演算法 PoC 在 offline notebook 完成，hit-rate 報告
3. **Phase 2**（1 週）: 包裝成 `SectorPredictionEngine`，feature flag `SECTOR_PREDICTION_ENABLED=false`
4. **Phase 3**（1 週）: 接上 API，dark launch（前端不顯示，僅 server 端 log）
5. **Phase 4**（2 週）: 觀察窗口 — 對照 baseline，整體 hit-rate 不退步才升 user-visible
6. **Phase 5**: 升 `SECTOR_PREDICTION_ENABLED=true`，前端正式顯示

KPI:
- Hit-rate vs baseline ≥ +5%
- Brier score ≤ 0.20
- Sector 加權與整體信心 Pearson correlation ≥ 0.6
- API p95 latency /prediction < 200ms（多了板塊計算）

退出條件：
- Hit-rate 比 baseline 差，退回 disabled
- Consistency check failure rate > 5%，暫停升 phase

---

## 7. Open Questions（給 owner 確認）

1. **訓練資料回溯範圍**：用 2 年（2024-07 ~ 2026-07）還是 3 年？要兼顧新樣本 vs 事件類型覆蓋。
2. **Sector L1 vs L2**：L2 大概 60+ sectors，畫面會擠。是否只先做 L1（20）就好？後續再展開 L2？
3. **驅動因子（drivers）來源**：用哪些 macro/cycle 訊號列進 drivers？list 越長越有意義但越擾人。
4. **Consistency alert threshold**：整體信心與板塊加權差異超過多少要 alert？建議 ≥ 0.3。
5. **歷史 baseline window**：每個 (event_type, sector) cell 至少要有 N=30 個歷史樣本才用 empirical probs，不然退回 uniform prior —— N=30 合理嗎？
6. **Front-end 是否預設展開**：現有 heatmap 已存在，新 sector_predictions 區塊要不要預設展開？預設摺疊（用「查看板塊預測」按鈕）保持首屏簡潔？

---

## 8. 完成定義（DoD）

- [ ] 板塊日報酬 backfill 資料可用且覆蓋率 ≥ 95%
- [ ] Backend API 新增 `sector_predictions` 欄位，arbiter 通過 contract test
- [ ] Frontend 渲染 20 × 5 表格，cell 顯示方向 + 信心 + drivers
- [ ] Offline hit-rate ≥ baseline +5%（2026-Q2 hold-out）
- [ ] Brier score ≤ 0.20
- [ ] Frontend unit test ≥ 12 case（涵蓋空 / partial / hit cases / escape）
- [ ] Feature flag 文件化於 `docs/reference/parameters.md`
- [ ] Runbook 寫進 `docs/operations/sector-prediction-runbook.md`
- [ ] L2.4-style observation window 進入 standby

---

**請 owner 過目此 spec，告訴我哪些段落需要擴充、哪些要砍掉、Open Questions 怎麼回答。確認後再開始 Phase 0 backfill。**
