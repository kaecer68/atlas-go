> ⚠️ **DEPRECATED 2026-08-12** — `/client/capital_predictions` 頁面已移除（PR #1523）。本 spec 描述的「板塊方向預測」前端區段（§6 「板塊方向預測」與 §3.2 「熱度圖」）不再有對應 UI。Sector predictor 邏輯本身（`internal/eventdriven/sector_predictor.go` + `/api/events/prediction`）仍於 home 頁的「未來 5 日錢潮預測」section 服務。如需恢復完整板塊方向預測 UI，請參考 git log 復活此 spec。

# C07 — 板塊維度預測（Per-Sector Direction Prediction）

> **狀態**：v1.1 — 因 Phase 0 發現歷史板塊日報酬無法回填，改採 rule-based / heuristic 實作，保留統計模型升級空間
> **作者**：atlas-dev（agent-driven, Sisyphus pattern）
> **建立日期**：2026-07-16
> **最後更新**：2026-07-16
> **優先級**：P2
> **對應 manifest 條目**：`docs/archive/2026-07-16-sector-dimension-prediction-invariant-manifest.md`
> **相依**：C06（`/api/events/prediction` 已穩定 expose `etf_estimates` / `revenue_surprises`）
> **Phase 0 決策**：原始「3 年歷史板塊回報 backfill」不可行；改以 `MacroDataSnapshot` + `CycleTracker` + 事件 `affected_industries` 作為輸入，實作可解釋的 rule-based 預測器。詳見 §9 與 manifest §6。

---

## 1. 背景與目標

### 1.1 現狀

`/api/events/prediction` 目前給出：

- 5 日整體錢潮方向（inflow / outflow / neutral）+ confidence + distribution
- `active_events[]` — 未來事件，含 `affected_industries[]`（哪些板塊受事件影響）
- `driving_events[]` per prediction（哪個事件觸發）

前端 `capital_predictions.js` 用 `affected_industries` 做「板塊 × 日期」熱度圖：每個 cell 只顯示「該日該板塊是否受事件影響 + 該整體日方向」，不是「板塊自己的方向預測」。

### 1.2 為何需要 C07

`affected_industries` 是 **事件影響標籤**，不是板塊自己的方向預測。風險：

- 整體 inflow 日若事件標註「半導體」，cell 顯示偏多；但當日資金可能從半導體流出、流入金融 — 投資人會被誤導。
- 投資人真正需要：「**未來 5 日各板塊自己會往哪個方向流**」，用於調整部位與風險。

### 1.3 目標

- 為每個 L1 板塊（20 個 canonical ID）給出 5 日方向（inflow / outflow / neutral）+ confidence + distribution + drivers。
- 與整體預測 **一致性約束**：板塊加權方向不能與整體方向嚴重衝突。
- 前端以投資人分層視角呈現：先看「必須看」板塊，進階投資人再展開全部 20 個。

### 1.4 非目標

- 不做 stock-level 預測（recommender / LLM-agent 已有）。
- 不做 daily factor weight Darwinian 調整（`internal/darwinian` 已有）。
- 不做板塊 regime 模型（`internal/industry/cycle.go` 已有 cycle 資訊，我們拿來當 feature）。
- 不追求 60+ L2 sub-industries；第一版只解決 L1 20 個板塊的可解釋性與準確度。

---

## 2. 專家決策（Expert Decisions）

### 2.1 訓練回溯範圍：3 年

**決策**：使用 2023-07-01 ~ 2026-07-01 共 3 年滾動回溯。

**理由**：
- 3 年涵蓋一個完整库存 / 利率 / 地緣週期，足以學到事件在不同宏觀環境下的板塊反應。
- 台股 L1 板塊日報酬 3 年約 750 個交易日，可支撐 20 板塊 × 5 方向的基礎統計。
- 再往前（>3 年）會進入 2020-2022 疫情極端環境，事件反應結構不同，反而引入 drift。

### 2.2 Sector 範圍：L1 20 個板塊，但 UI 採投資人分層

**決策**：第一版只做 L1（20 個 canonical sector ID）。UI 預設顯示 **5 個必須看板塊**，提供「顯示全部 20 板塊」切換。

**預設 5 個必須看板塊**：
1. `semiconductor`（半導體）— 台股權重最大，外資錨點
2. `electronics`（電子零組件）— 出口鏈風向球
3. `financials`（金融保險）— 利率 / 本國資金風向
4. `shipping`（航運）— 景氣週期高 Beta
5. `ai_supply_chain`（AI 供應鏈）— 新增 narrative 主題，投資人關注度高

**理由**：
- 20 板塊 × 5 日 = 100 個 cell，預設全展開會讓首頁資訊過載。
- 投資人不是都想看全部板塊；先給「最影響大盤、最具週期意義」的 5 個，其餘 15 個放在「進階」切換，兼顧簡潔與完整。
- L1 是 canonical ID，與 `internal/industry/sector.go` 對齊，不會像 L2 那樣發生 ID 漂移。
- 未來若要擴展到 L2，可以在同一 UI 切換的「advanced」層級加入，不會破壞現有資料模型。

### 2.3 驅動因子（Drivers）清單

**決策**：從本系統已存在的 `MacroDataSnapshot` 與 `internal/industry/cycle.go` 選出 7 個對板塊流向有明確經濟意義的訊號。

| Driver | 來源 | 主要影響板塊 | 方向邏輯 |
|--------|------|-------------|----------|
| `dxy` | `MacroDataSnapshot.DXY` | electronics, semiconductor, shipping | DXY 走強 → 出口商計價壓力 → 偏弱 |
| `us10y` | `MacroDataSnapshot.US10Y` | financials, construction | 利率上升 → 金融淨息差擴大 / 營建成本承壓 |
| `tsm_adr` | `MacroDataSnapshot.TSMADR` | semiconductor, ai_supply_chain | 前一晚 ADR 走勢 → 隔天台股半導體風向 |
| `nvda` | `MacroDataSnapshot.NVDA` | semiconductor, ai_supply_chain | AI 資本支出風向球 |
| `foreign_investor_net` | `MacroDataSnapshot.ForeignInvestorNet` | broad leader | 外資買賣超 → 權值板塊（半導體、金融）帶頭 |
| `bdi` | `MacroDataSnapshot.Bdi` | shipping, steel | 散裝運價 → 航運、鋼鐵景氣 |
| `cycle_position` | `internal/industry/cycle.go` | 全板塊 | 復甦 / 擴張 / 收縮 / 衰退對各板塊的結構性偏移 |

**理由**：
- 這些訊號都已經在系統中每日更新，不需要新建 data channel。
- 每個 driver 都有跨國文獻與台股實證支持，不是拍腦袋。
- 數量控制在 7 個，避免 driver list 過長導致前端「每一格都塞滿原因」的雜訊。
- 前端 drivers 顯示：只列出對該板塊當日方向貢獻度最高的 2 個（貢獻度 = absolute weight in model），避免清單爆炸。

### 2.4 一致性 Alert Threshold：Jensen-Shannon Divergence ≤ 0.25

**決策**：以 sector-weighted 預測分布與整體預測分布的 **Jensen-Shannon divergence（JSD）** 衡量一致性。若 JSD > 0.25，觸發「內部不一致」標記，並將該日 confidence 乘以 0.85 衰減。

**理由**：
- 單純用 confidence 差絕對值會被分布形狀誤導；JSD 比較整個 inflow/neutral/outflow 分布，更穩健。
- 0.25 是 JSD 的「中等差異」門檻，約對應 0.65-0.70 的 Brier score 可接受區間。
- 0.3（user 建議）會放過太多「表面方向一致、但分布質量差」的衝突；0.25 更能在 UI 上給投資人可信的 warning。
- 發生不一致時不是隱藏結果，而是降權 confidence 並顯示「板塊加總與整體信心存在分歧」標籤 — 保持透明。

### 2.5 歷史 Baseline Floor N：分層門檻

**決策**：按事件頻率使用不同 floor，而非單一 N=30。

| 事件頻率 | 3 年內近似樣本數 | Floor N | 低於 floor 的處理 |
|---------|-----------------|---------|------------------|
| 月度（MSCI、月營收、FOMC）| ~36 | 24 | 用 Bayesian shrinkage 退向 prior |
| 季度（季報、ETF rebalance）| ~12 | 12 | 較強 shrinkage |
| 年度（除權息高峰、股東會）| ~3 | 3 | 幾乎完全使用 prior |

**理由**：
- 用 N=30 一刀切，會把季度、年度事件全部打回 uniform，模型變成無法學習季節性。
- 24 / 12 / 3 對應統計上可接受的最小樣本：N=24 可支撐 t-test power 0.8（effect size 0.6），N=12 勉強可算方向多數，N=3 只能當 seasonality hint。
- Bayesian shrinkage 強度與 N 成反比：N 越大越相信數據，N 越小越相信 uniform prior。

### 2.6 前端預設展開：預設摺疊，記憶用戶偏好

**決策**：新板塊預測區塊 **預設摺疊**。在 desktop 與 mobile 都使用一個醒目的「板塊方向預測」toggle button，點擊後展開。用戶一旦展開，localStorage 記住偏好，下次登入自動展開。

**理由**：
- 本質上這是進階資訊；預設展開會干擾「整體 5 日錢潮」這個主要任務。
- 摺疊後保留一個 summary badge（例如「5 板塊中 2 個偏多」）讓投資人快速感知，不點開也不會錯失全局。
- localStorage 記憶偏好是尊重 power user 的標準 UX 模式。
- mobile 上 100 個 cell 全展開會把後續內容推到底部以下，必須摺疊。

---

## 3. 訓練資料與缺口

### 3.1 資料需求

| 資料 | 用途 | 來源 | 現狀 |
|------|------|------|------|
| L1 板塊日報酬序列 | 訓練/驗證 per-sector 模型 | TWSE 指數編輯（上市/上櫃各類股指數）| 需要 backfill |
| 事件 → 板塊影響對應 | 建立 empirical baseline | `internal/eventdriven` 的 event calendar + `affected_industries` | 已有，需 canonicalize |
| 7 個 macro driver 時序 | macro exposure adjustment | `internal/marketdata/macro_provider.go` | 已有 |
| 板塊 cycle position | structural tilt | `internal/industry/cycle.go` | 已有 |
| ETF rebalance 影響 | rotation impact | C06 已 expose `etf_estimates` | 已可用 |
| Stock → sector mapping | 將事件影響標籤從 stock symbol 轉為 sector ID | 內部 mapping table | 需要建立 |

### 3.2 缺口與補齊方案

1. **板塊日報酬 backfill**：Phase 0 核心任務。優先從 TWSE OpenAPI 拉取 2023-07-01 起各類股指數日收盤；若無法取得完整序列，fallback 到 FinMind `TaiwanStockIndex`。產出 `data/sector_data/sector_returns.jsonl`（每日一行，key 為 canonical sector_id）。
2. **Stock → Sector canonical mapping**：建立 `internal/industry/stock_sector_map.go`（或 YAML），將常見 stock symbol（如 2330）與事件中的中文 alias（如「半導體」）統一 mapping 到 L1 canonical ID。這是資料淨化層，不進模型。
3. **Event win-rate 歷史表**：跑 `internal/eventdriven` 的歷史 event calendar，對每個 event 計算其後 +1 / +3 / +5 日各 L1 板塊實際方向，產生 `(event_type, sector) → {inflow_count, outflow_count, neutral_count}` lookup table。這是 **baseline_probability** 的來源。

### 3.3 訓練流程

```text
Phase 0: Backfill
  1. 拉取/整理 2023-07-01 ~ 2026-07-01 的 L1 板塊日報酬
  2. 建立 stock/symbol → canonical sector_id 的 mapping
  3. 對歷史 event calendar 跑後向標記，產生 event_type + sector 的 hit table
  4. 套用分層 N floor 與 Bayesian shrinkage

Phase 1: Feature engineering
  5. 對每個板塊計算 macro beta（對 dxy / us10y / tsm_adr / nvda / foreign_investor_net / bdi）
  6. 從 CycleTracker 取得該板塊當前 cycle position

Phase 2: Model
  7. 組合 hybrid score：baseline_probability 60% + macro_exposure 25% + cycle_shift 15%
  8. 將 score 轉為 inflow/neutral/outflow 概率分布（softmax 或 constrained linear）
  9. 跑一致性檢查：JSD vs 整體預測；>0.25 則降權 confidence

Phase 3: Validation
  10. Hold-out: 2026-01-01 ~ 2026-07-01 作 out-of-sample，計算 hit-rate 與 Brier score
```

---

## 4. 功能設計

### 4.1 資料模型

```go
// internal/eventdriven/types.go 新增

// SectorPrediction represents per-sector direction for a single day.
type SectorPrediction struct {
    SectorID     string                  `json:"sector_id"`     // canonical L1 sector ID
    SectorName   string                  `json:"sector_name"` // 中文 display name
    Direction    string                  `json:"direction"`   // "inflow" | "outflow" | "neutral"
    Confidence   float64                 `json:"confidence"`  // 0..1
    Distribution PredictionDistribution `json:"distribution"`
    Drivers      []string                `json:"drivers"`     // 對該板塊貢獻最大的 2 個 driver
}

// SectorDayPrediction groups all L1 sectors for a single forecast date.
type SectorDayPrediction struct {
    Date    string             `json:"date"`
    Sectors []SectorPrediction `json:"sectors"`
}

// PredictionReport 新增 sector_predictions（C07）
type PredictionReport struct {
    // ... 既有欄位 ...
    SectorPredictions []SectorDayPrediction `json:"sector_predictions"`
}
```

**規定**：
- `SectorPredictions` 必須始終存在（不帶 `omitempty`），無資料時為 `[]`。
- 每個 `SectorDayPrediction.Sectors` 必須包含全部 20 個 L1 sectors（排序固定）。
- `sector_id` 必須是 canonical L1 ID（由 `industry.NormalizeSectorID` 檢查）。

### 4.2 演算法（Rule-Based / Heuristic Model）

因 Phase 0 確認歷史板塊日報酬無法回填，C07 v1.1 改採可解釋的 rule-based 模型。所有權重與門檻由參數系統（`ParametersConfig`）管理，預設值採用下表，未來有歷史資料時可平滑替換為統計模型。

對每個板塊 S（canonical L1 sector ID）與預測日 D：

```text
score_inflow, score_outflow, score_neutral = 0, 0, 0

# 1. 整體方向基線（overall baseline）
if P_overall.direction == "inflow":
    score_inflow  += sector_weight(S) * overall_confidence
elif P_overall.direction == "outflow":
    score_outflow += sector_weight(S) * overall_confidence
else:
    score_neutral   += sector_weight(S) * overall_confidence

# 2. 事件板塊標籤調整（event affected_industries）
for e in active_events where S in e.affected_industries:
    w = e.base_weight * (1.0 if not e.backfilled else 0.7)
    if e.direction == "bullish":
        score_inflow  += w
    elif e.direction == "bearish":
        score_outflow += w
    elif e.direction == "mixed":
        score_inflow  += w * 0.3
        score_outflow += w * 0.3

# 3. 宏觀驅動調整（macro drivers，使用 MacroDataSnapshot.ChangePct）
for (driver, relevance) in sector_macro_map(S):
    if driver is missing or ChangePct == 0: continue
    sign = driver_direction_logic(driver, ChangePct)  # +1 bullish, -1 bearish
    magnitude = clamp(|ChangePct| / typical_move, 0, 1)
    if sign > 0:
        score_inflow  += relevance * magnitude
    elif sign < 0:
        score_outflow += relevance * magnitude

# 4. 週期位置調整（CycleTracker）
cycle_score = GetContinuousPhaseScore(S)  # -1..1
sector_cycle_sensitivity = cycle_sensitivity(S)  # e.g. cyclical 1.0, defensive 0.5
if cycle_score > 0:
    score_inflow  += cycle_score * sector_cycle_sensitivity
elif cycle_score < 0:
    score_outflow += -cycle_score * sector_cycle_sensitivity
else:
    score_neutral += 0.2

# 5. 轉為概率分布
P_raw = softmax(score_inflow, score_neutral, score_outflow)

# 6. 方向與信心
direction = argmax(P_raw)
confidence = 1 - normalized_entropy(P_raw)
confidence = max(confidence, 0.40)

# 7. 一致性檢查（JSD vs 整體預測）
if JSD(P_raw, P_overall) > 0.25:
    confidence *= 0.85
    drivers.append("板塊加總與整體信心存在分歧")

# 8. drivers（貢獻最大的 2 個因子，不含一致性 warning）
drivers = top_k_contributors(S, P_raw, top=2)
```

#### 4.2.1 板塊權重（sector_weight）

以板塊對台灣加權指數的近似影響力為基礎：

| 板塊 | 預設權重 | 理由 |
|---|---|---|
| semiconductor | 0.30 | 台股權重最大 |
| electronics | 0.15 | 電子零組件出口鏈 |
| financials | 0.12 | 金融保險 |
| ai_supply_chain | 0.10 | 新增 narrative 主題 |
| shipping | 0.08 | 高 Beta 週期 |
| 其他 15 個 L1 | 0.25 均分 | 合計 0.25 |

權重總和為 1.0。未來可用實際市值權重替換。

#### 4.2.2 宏觀驅動映射（sector_macro_map）

| Driver | 主要影響板塊 | 方向邏輯 |
|---|---|---|
| `dxy` | electronics, semiconductor, shipping, steel | ChangePct > 0 → 出口計價壓力 → 偏空 |
| `us10y` | financials, construction | ChangePct > 0 → 淨息差擴大 / 營建成本承壓 → 金融偏多、營建偏空 |
| `tsm_adr` | semiconductor, ai_supply_chain | ChangePct > 0 → 偏多 |
| `nvda` | semiconductor, ai_supply_chain | ChangePct > 0 → 偏多 |
| `foreign_investor_net` | semiconductor, financials | ChangePct > 0 → 外資買超 → 權值板塊偏多 |
| `bdi` | shipping, steel | ChangePct > 0 → 景氣偏多 |
| `sox_index` | semiconductor | ChangePct > 0 → 偏多 |
| `taiex` | 全部 | ChangePct > 0 → 偏多 |

每個 driver 的 magnitude 以該 driver 的「典型單日變動」進行標準化。典型值由參數管理，預設：
- DXY: 0.5%, US10Y: 5bps, TSM ADR: 2%, NVDA: 3%, BDI: 2%, SOX: 2%, ForeignInvestorNet: 1%, TAIEX: 1%

#### 4.2.3 週期敏感度（cycle_sensitivity）

| 板塊類型 | 敏感度 | 例子 |
|---|---|---|
| 高週期 | 1.0 | semiconductor, shipping, steel, electronics, auto, construction, machinery, chemicals, plastics, cement |
| 中週期 | 0.7 | optoelectronics, other_electronics, telecom, biotech, energy, retail, food, textiles, tourism |
| 低週期/防禦 | 0.4 | financials |

#### 4.2.4 一致性與信心衰減

- `confidence` 計算後強制 ≥ 0.40（I7）。
- JSD 與整體預測超過 0.25 時乘以 0.85 並顯示 warning（I5）。
- 若 `MacroDataSnapshot.DataStatus` 為 `stale` 或 `degraded`，所有板塊 confidence 再乘以 0.90。

#### 4.2.5 與原始統計模型的關係

本 rule-based 模型保留與原始統計模型相同的資料模型（`SectorPrediction`、`SectorDayPrediction`）與 invariants（I1-I11）。原始「3 年回溯 + 事件命中表 + Bayesian shrinkage」路徑標記為 **deferred**，在歷史板塊資料可取得後可無縫替換 `SectorPredictor` 內部實作，而不改 API contract 與前端。

### 4.3 API 契約

延續 C06 的方向：`/api/events/prediction` JSON 內新增 `sector_predictions` 欄位（必出現，empty → `[]`）。

範例 fragment:

```json
{
  "sector_predictions": [
    {
      "date": "2026-07-17",
      "sectors": [
        {
          "sector_id": "semiconductor",
          "sector_name": "半導體",
          "direction": "inflow",
          "confidence": 0.62,
          "distribution": {"inflow": 0.55, "neutral": 0.30, "outflow": 0.15},
          "drivers": ["tsm_adr: +2.8%", "foreign_investor_net: 連 3 日買超"]
        },
        {
          "sector_id": "financials",
          "sector_name": "金融保險",
          "direction": "neutral",
          "confidence": 0.45,
          "distribution": {"inflow": 0.30, "neutral": 0.50, "outflow": 0.20},
          "drivers": ["us10y: +6bps"]
        }
      ]
    }
  ]
}
```

### 4.4 前端整合

`capital_predictions.js` 新增區段「**板塊方向預測**」：

- 預設摺疊；summary badge 顯示「5 個必須看板塊中 X 個偏多 / Y 個偏空 / Z 個觀望」。
- 點擊 toggle 展開 5 日 × 必須看板塊表格；再點「顯示全部 20 板塊」展開其餘 15 個。
- 每個 cell：方向箭頭（▲/—/▼）+ confidence 百分比 + 顏色（profit/loss/neutral）。
- Hover 顯示 drivers 列表（最多 2 條 + 1 條一致性 warning）。
- 點擊 cell 展開 detail（同現有機制）。
- localStorage key: `cp_sector_predictions_expanded` 記住用戶偏好。
- 若 `sector_predictions` 為 `[]` 或 missing，顯示 empty state（「尚無板塊預測資料」），不 crash。

---

## 5. 測試策略

| Level | 工具 | 內容 | Invariant |
|-------|------|------|-----------|
| Unit | `go test ./internal/eventdriven/...` | `SectorPredictor` 各 component（baseline / macro / cycle / consistency） | 每個板塊輸出 distribution 總和為 1.0 |
| Integration | `go test ./internal/eventdriven/...` | 接 API 後 response 必含 `sector_predictions`，且 20 sectors × 5 days | 欄位不出現 null / 必為陣列 |
| Backtest | replay-based | 跑 2026-Q2 replay windows，hit-rate ≥ baseline + 3% | hit-rate ≥ 0.55 |
| Experiment | `cmd/run-experiment` | `sector-prediction-v0` vs `sector-prediction-disabled` | Brier ≤ 0.20 |
| Frontend | `__tests__/sector_predictions.test.mjs` | 摺疊/展開、必須看板塊、全部 20 板塊、driver tooltip、escape | 無 `<script>` 注入 |
| Consistency | `go test` | JSD vs 整體預測 | JSD ≤ 0.25 |
| Calibration | `internal/calibration/` | weekly Brier score | drift ≤ 0.10 |

---

## 6. Guardrails 與風險

| 風險 | 緩解 |
|------|------|
| 板塊日報酬資料缺 | Phase 0 先驗證；缺資料則 `sector_predictions` 回傳 `[]` 並標記 `data_status=degraded` |
| Sector ID 不 canonical | 全部走 `industry.NormalizeSectorID()`；API response 用 canonical ID；前端用 label map |
| 模型過度自信 | Bayesian shrinkage + JSD 一致性衰減 + calibration floor 0.40 |
| 整體與板塊方向衝突 | JSD 檢查；衝突時降權並顯示 warning，但不隱藏結果 |
| 前端 100 cell 效能 | 摺疊預設 + 只在展開時渲染；可選 virtual scrolling（未來） |
| 新模型破壞既有預測 | Feature flag `SECTOR_PREDICTION_ENABLED` 控制；預設 false，觀察窗口後才開啟 |
| L2 sub-industry 需求 | 不在第一版；資料模型預留 `sector_id` 可為 L2 ID，但 UI 第一版只處理 L1 |

---

## 7. Rollout 計畫（L2.4 五條件硬門 + 觀察窗口）

1. **Phase 0**（1 週）：Backfill 板塊日報酬 + 事件命中表；產出資料可用性報告。
2. **Phase 1**（1 週）：實作 `SectorPredictor` 純函式與 unit test；offline 跑 2026-Q2 驗證 hit-rate / Brier。
3. **Phase 2**（1 週）：接入 `PredictionReport`；新增 `sector_predictions` 欄位；API contract test；feature flag 預設 false。
4. **Phase 3**（1 週）：前端區段實作；dark launch（server 回傳但前端不顯示，只 log）。
5. **Phase 4**（2 週）：觀察窗口 — 對照 baseline，整體預測 hit-rate 不退步、JSD alert rate ≤ 5%。
6. **Phase 5**：開啟 `SECTOR_PREDICTION_ENABLED=true`，正式上線。

### 7.1 KPI

- Hold-out hit-rate ≥ 0.55（baseline +3%）
- Brier score ≤ 0.20
- Sector-weighted vs overall JSD alert rate ≤ 5%
- `/api/events/prediction` p95 latency < 200ms（with sector predictions）
- Frontend unit tests ≥ 15 cases
- Backend unit tests ≥ 10 cases

### 7.2 退出條件

- 任何 Phase 0-2 發現資料缺口無法補齊 → 停止，改為純 frontend 顯示 `affected_industries` 標籤（不聲稱預測）。
- Phase 4 hit-rate 比 baseline 退步 → 維持 feature flag false，回頭調校 macro beta / baseline shrinkage。
- JSD alert rate > 10% → 檢查 baseline 與 macro 權重，必要時降低 macro weight。

---

## 8. 完成定義（DoD）

- [ ] L1 板塊日報酬 backfill 可用，覆蓋率 ≥ 95%（3 年）
- [ ] `SectorPredictor` 實作 + unit test ≥ 10 cases PASS
- [ ] `PredictionReport` 新增 `sector_predictions` 欄位，contract test PASS
- [ ] Frontend 新區段實作：摺疊/展開、必須看板塊、全部 20 板塊、driver tooltip、localStorage 記憶
- [ ] Frontend unit test ≥ 15 cases PASS
- [ ] Offline hit-rate ≥ 0.55（2026-Q2 hold-out）
- [ ] Brier score ≤ 0.20
- [ ] Feature flag `SECTOR_PREDICTION_ENABLED` 文件化於 `docs/reference/parameter-system.md`
- [ ] Runbook 寫進 `docs/operations/sector-prediction-runbook.md`
- [ ] L2.4-style observation window 進入 standby，退出條件明確

---

**v1.0 決策摘要**：3 年回溯、L1 20 板塊但 UI 分層、7 個 macro/cycle driver、JSD ≤ 0.25 一致性、分層 N floor（24/12/3）、前端預設摺疊。下一步：進入 Phase 0 backfill 與 invariant tracker manifest 執行。
