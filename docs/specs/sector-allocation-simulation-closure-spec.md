# 產業配置模擬閉環規格

> **文件角色**：atlas-go 產業分類、產業權重、題材配置、資產覆蓋與模擬策略套用的唯一權威契約。
> **狀態**：accepted design；implementation pending。
> **版本**：1.0
> **日期**：2026-07-18
> **產品邊界**：simulation-first；live broker 與實盤下單不在本規格範圍。
> **治理文件**：[（`.omo/manifests/`，內部）
> **上游正本**：[`../reference/product-positioning.md`](../reference/product-positioning.md)、[`capital-flow-seven-dimension-spec.md`](capital-flow-seven-dimension-spec.md)、[`../guides/fu-7-sector-norm.md`](../guides/fu-7-sector-norm.md)

---

## 1. 目標與完成契約

本修復必須完成原始散戶定位審計 `P1-4` 所要求的「預測進策略層」，並優於原始 F05 的單純 wiring 目標。完成後，系統必須形成可驗證的模擬閉環：

```text
canonical 產業分類與 strategic prior
  → WeightEngine 動態 target
  → 經校準的 macro / capital-flow / theme tilt
  → canonical L1 sector target
  + simulation 真實 current exposure
  → SectorRotationPlan(target/current/delta)
  → simulation allocator 真實狀態變更
  → outcome / benchmark / shadow strategy ranking
  → REST / Web / MCP 同一份可解釋輸出
```

只有同時具備下列證據，整體工作才可標記 `done`：

1. 型別與資料契約證明 final equity target 只含 20 個 canonical L1 sector。
2. 單元、整合、API、前端與 MCP contract tests 全數通過。
3. simulation state 產生可重現的 before/after mutation receipt；不能只寫 `applied` log。
4. 所有 fallback 具有 machine-readable reason，且不會改變 simulation state。
5. dark launch／A-B observation 通過 promotion gate。
6. production static search 證明 legacy sector map、nil current exposure、假 `applied` 路徑均已退出。
7. F06 只使用 non-synthetic outcome 與真實 benchmark；不足時為 `warming_up`。

---

## 2. 為何舊 F05 計畫停止

### 2.1 現行來源矩陣

| 來源 | Universe | 現行用途 | 問題 |
|------|----------|----------|------|
| `internal/industry/sector.go` | 20 L1 + 18 L2 canonical IDs | taxonomy、C07、MCP lookup | 尚未約束 sectorallocation／portfolio |
| `sector_allocation.base_weights` | 12 個 GICS-like/L2/asset 混合 key | WeightEngine；production `NewSectorRotator()` static base | 只 6 個 key 可直接解析為 FU-7 canonical；缺 14 個 L1 |
| `engine.sector_rotation.base_allocations` | 12 個 L1/L2/strategy/asset 混合 key | legacy config；custom constructor | production 主路徑已不使用，但仍存活於 config/tests |
| `eventdriven._sectorWeights` | 20 L1，sum=1 | C07 rule-based predictor baseline | hardcoded duplicate；未進參數 provenance |
| `portfolio_allocation.json` | top-level asset class + stock bucket | OpenClaw monitoring target | 與 equity sector allocation 分母不同 |

兩張舊 map 各自 `sum=1`，但只有五個共同 key；直接 union 會得到 19 keys、總和 1.57，並同時出現 `cash/_cash_reserve` 與 `industrial/industrials`。因此「WeightEngine 勝出、legacy 缺項補入」不是模型融合，而是跨 universe 相加。

### 2.2 根因

1. 2026-05 legacy rotator 將 L1、L2、策略桶與現金放在同一 map。
2. 2026-06 WeightEngine 遷移改用另一套 base weights，但未移除舊 config 與 theme-based overlay。
3. 2026-07 FU-7 建立 canonical taxonomy，採 additive rollout，未回填 sectorallocation／portfolio。
4. C07 使用 canonical 20 L1，卻另建 hardcoded prior。
5. 2026-07-17 audit 正確找到 WeightEngine wiring 缺口，但未盤查跨模組 taxonomy、current exposure 與 allocator no-op。
6. 後續 F05 plan 將「兩套融合」誤實作為 string-key union。

---

## 3. 權威層級與 namespace

### 3.1 權威順序

```text
internal/industry/sector.go taxonomy
  → 本規格的 allocation 語意
  → sector-allocation simulation-closure manifest
  → implementation / config
  → runtime output / observation evidence
  → audit / historical plan
```

舊程式或舊 config 不能反向改寫 taxonomy；audit 與 `.omo/plans` 只保留歷史證據，不是現行規範。

### 3.2 四個不可混用的 namespace

| Namespace | 合法內容 | Sum contract | 對外用途 |
|-----------|----------|--------------|----------|
| `equity_sector_l1` | `industry.L1Sectors()` 的 20 個 ID | final target 必須 sum=1 | 投資人產業配置、simulation allocator |
| `research_theme_l2` | canonical L2、narrative theme | 不獨立宣稱投組占比；經 exposure matrix 轉成 L1 tilt | 研究與策略訊號 |
| `strategy_bucket` | defensive、high_dividend、small_cap 等 | 必須宣告 budget 與 funding source | 策略 sleeve |
| `asset_class` | cash、gold、bond、JPY exposure 等 | 由 top-level asset allocation 管理 | 風險與資產配置 |

`cash`、`gold`、`defensive`、`short_term_bonds`、`jpy` 不得出現在 `equity_sector_l1` target。中文名稱只能是 display label，不能作 machine key。

---

## 4. 核心領域模型

### 4.1 Strategic prior

`StrategicSectorPrior` 是唯一的 L1 base distribution：

```go
type StrategicSectorPrior struct {
    Weights           map[industry.SectorID]float64
    Source            string
    ModelVersion      string
    CalibrationStatus string
    AsOfDate          string
}
```

第一版將 C07 現有 20 L1 heuristic seed 遷入 ParametersConfig，標示 `source=heuristic`、`calibration_status=calibrating`。在 SA11 promotion 前只能用於 observation/dark launch，不得自動修改 simulation allocation。原 `sector_allocation.base_weights` 與 C07 hardcoded map 必須在 sunset 後刪除。

### 4.2 Theme exposure

L2/theme 不直接占用 L1 target key；使用顯式 exposure matrix：

```go
type ResearchThemeID string

type ThemeExposure struct {
    Theme              ResearchThemeID
    CanonicalSubsector *industry.SectorID
    ToL1               map[industry.SectorID]float64
    Source             string
    Version            string
}
```

`ResearchThemeID` 與 `industry.SectorID` 是不同型別；只有確實對應 FU-7 L2 時才填 `CanonicalSubsector`。narrative theme 不得假裝是 canonical L2。每一列 `ToL1` 必須只含 L1 且 sum=1。禁止 fuzzy matching；`industrial` 不得自動改成 `industrials`。

### 4.3 Tactical tilt

所有動態 driver 先轉為 tilt，而不是第二張 allocation：

```go
type SectorTilt struct {
    Driver       string
    Values       map[industry.SectorID]float64
    FundingSource string
    ModelVersion string
    Eligible     bool
}
```

若 `FundingSource=equity_sector_l1`，projection 前 tilt 必須 zero-sum。若資金來自 cash 或其他 asset class，必須由 top-level asset allocator 明確提供 budget，不能在 sector map 內憑空新增。

### 4.4 Final snapshot

```go
type SectorAllocationSnapshot struct {
    AsOfTradingDate   string
    Target            map[industry.SectorID]float64
    Current           map[industry.SectorID]float64
    Delta             map[industry.SectorID]float64
    WeightSource      string
    ModelVersion      string
    CalibrationStatus string
    FallbackReason    string
    Applied           bool
    MutationReceipt   *SimulationMutationReceipt
}
```

`Applied=true` 必須附 receipt；receipt 至少包含 before hash、after hash、changed sector count 與 simulation session ID。

---

## 5. 權重融合與 constraint projection

本系統採 strategic core + constrained tilt，不採兩張 allocation union：

```text
raw_target = strategic_prior
           + cycle_tilt
           + seasonal_tilt
           + linkage_tilt
           + narrative_tilt
           + calibrated_macro_tilt
           + calibrated_capital_flow_tilt
           + mapped_theme_tilt

final_target = Project(raw_target, constraints)
```

`Project` 必須一次完成：

1. 拒絕非 L1 key。
2. 套用 min/max sector exposure。
3. 保持非負。
4. 重新分配被 clamp 的差額。
5. final sum=1，容許誤差不超過 `1e-9`。
6. 產生每個 clamp／redistribution 的 adjustment log。

不得先由 WeightEngine normalize、再由 SectorRotator 用另一套語意 normalize。整條路徑只能有一個 final projection owner。

---

## 6. Macro、capital flow 與 C07 邊界

### 6.1 Macro action vocabulary

現行 `MacroRiskAssessment` 可產生：

```text
mixed / risk_off / carry_trade_unwind / sector_rotation
```

此 vocabulary 需升級為 typed enum，並由 macro adapter 轉成 L1 tilt。舊 rotator map 中不存在的正 delta 不得新增 key，負 delta 也不得被歸零後假裝成功。

### 6.2 Capital-flow assessment

E07 `CapitalFlowAssessment` 在校準完成前只提供：

- eligibility gate；
- institutional／behavioral／foreign-position／cross-market 分層證據；
- data quality 與 as-of date。

它目前不得直接產生 rotator action。只有經獨立 walk-forward 驗證、具 model version 的 mapper，才可把 assessment 轉成 `SectorTilt`。未建立 mapper時，capital-flow tilt 必須為零並回 `capital_flow_action_unavailable`，不得猜測空 `PrimaryFlow`。

### 6.3 C07

C07 在 observation gate 通過前是 investor-facing prediction，不是 allocation input。未來若晉升，必須經 adapter 將 per-sector distribution 轉成 bounded zero-sum tilt，不能直接把 confidence 當 weight。

---

## 7. Production composition 與執行路徑

WeightEngine 必須由 composition root 建立一次，並注入 Dashboard 與 simulation System。禁止：

- 在 `IndustryService` 內建 nil-provider partial engine；
- 從 Dashboard getter 反向借 WeightEngine 給 orchestrator；
- 每個 main caller 各建一套不同 provider 的 engine。

六個 System constructor 必須全部列入 wiring matrix，但只有下列四條可進入 sector observation/application gate：

1. admin manual simulation；
2. auto daily simulation；
3. stress-test 前置 simulation；
4. CLI simulation。

四條路徑不代表都能猜測下一交易日。只有取得 authoritative `TradingSessionResolver` 的路徑才可建立 applied policy：replay 優先使用 `Dataset.NextDate()`；其他路徑若沒有交易所日曆或可驗證 dataset，必須回 `effective_session_unavailable`，只保存 observation snapshot，不得用 `AddDate(1)` 或 weekday 規則猜測。

`auto_experiment` 與 live trading 不得因本修復新增 sector mutation；需有 negative integration tests 鎖定。現行 CLI simulation 會將 simulation positions 同步到 live store，這會讓 sector policy 間接污染 live state；SA08 必須先移除該 sync 或置於獨立、預設 false 且不由本閉環啟用的明確 opt-in gate。未完成隔離前，SA08 不得進入 observing。

---

## 8. Current exposure 與 simulation application

### 8.1 Current exposure

current exposure 以交易日 T 的 simulation closing positions、T 日價格與 symbol→L1 mapping計算：

```text
sector market value / total Taiwan-equity market value
```

unknown symbol 不得靜默歸類；若未映射 equity weight > 0，整次 application fallback 為 `current_exposure_incomplete`，並揭露 unmapped symbols/weight。

### 8.2 No-look-ahead effective date

T 日收盤後產生的 sector snapshot 只能建立 `effective_from > as_of_trading_date` 的 next-session policy。它不得回頭改變 T 日已產生的 orders、positions 或 outcome。交易日必須由 strict session ID 與 authoritative `TradingSessionResolver` 推導，不可用 wall-clock、weekday 或固定加一天猜測；resolver 不可用時必須 fail closed。

### 8.3 Application truthfulness

`ApplySectorRotation` 必須符合二選一：

- 持久化一份可被下一有效 simulation session 的 allocator 消費的 `SectorAllocationPolicy`，回傳 mutation receipt；或
- 完全不修改並回 `Applied=false` + fallback reason。

一份只供展示、未被 allocation/order-sizing path 消費的 snapshot 不算 applied。下一有效 session 必須在產生 orders 前載入 policy，並由端到端測試證明 sector cap、position sizing 或候選配置確實因 policy 改變；receipt 同時記錄 source session、effective session、before/after policy hash 與 changed sector count。

只做 gate 判斷、只寫 log、只保存無 consumer 的狀態，或在同一 session 事後回寫結果都屬違規。sector policy、simulation application 與 CLI simulation 均不得寫入 live state；broker adapter 與真實訂單不在本規格範圍。

---

## 9. Fallback 與錯誤語意

Machine-readable fallback 至少包含：

```text
feature_flag_off
strategic_prior_unavailable
weight_engine_unavailable
weight_engine_error
noncanonical_target
constraint_projection_failed
assessment_unavailable
assessment_error
assessment_calibrating
assessment_degraded
capital_flow_action_unavailable
current_exposure_incomplete
effective_session_unavailable
allocator_unavailable
no_effective_change
observation_gate_closed
```

Fallback 必須：

1. 不 mutation simulation state；
2. 保留完整原因、來源與 as-of date；
3. API/UI/MCP 顯示一致；
4. 進入 metrics／audit log；
5. 不將缺資料解讀成 neutral。

---

## 10. Manifest 狀態機與階段閘門

每個修復 ID 使用：

```text
pending → in_progress → implemented → observing → done
```

另有 `blocked`、`superseded`、`rolled_back`。禁止從 `in_progress` 直接跳 `done`。

### Gate 0 — Baseline freeze

- source matrix、call graph、舊計畫錯誤與負面現況有可重現證據；
- 舊 F05 plan 停止執行。

### Gate 1 — Contract

- namespace、typed model、mapping、projection 與 fallback contract 測試先 RED；
- canonical spec 與 manifest 無衝突。

### Gate 2 — Implementation

- SA01–SA10 focused tests、full tests、build、lint、generate 全綠；
- feature flags 預設關閉；
- live negative tests 全綠。

### Gate 3 — Dark launch

- 只產生 snapshot，不 mutation；
- 舊／新 target A-B 差異、fallback rate、noncanonical count、projection error 持久化。

### Gate 4 — Simulation application

- 僅 simulation flag 開啟；
- mutation receipt、current exposure 與 outcome attribution 可對帳；
- rollback 可恢復至 application 前狀態。

### Gate 5 — Observation promotion

- observation window 達 manifest 所定最少有效 session 數；
- 無 invariant violation、無 live mutation、無 synthetic ranking；
- 未通過則 rollback，不得延長後直接宣稱完成。

### Gate 6 — Sunset／close-out

- legacy reads=0；
- dead config、duplicate weights、temporary adapters 移除；
- proof bundle 與 verification report 完整；
- manifest 才可標 `done`。

---

## 11. 防止再次迷失或反覆推翻

1. **任何修改前先更新 source matrix**：新來源必須標 namespace、unit、sum contract、owner、consumer。
2. **測試名稱引用 invariant ID**：沒有對應 invariant 的新行為不得合併。
3. **狀態與實作分離**：code merged 只能到 `implemented`；runtime 證據完成才到 `done`。
4. **正向與負向證據並列**：除證明新路徑可用，也要證明 legacy/live/synthetic/unknown-key 路徑沒有被誤用。
5. **單一 ID 單一責任**：每次只處理 manifest 一列；scope 擴大先新增 ID，不在原 ID 偷塞。
6. **每個 ID 兩段證據**：implementation commit 後跑完整 gate；manifest evidence commit 只在 gate 通過後標狀態。
7. **三次失敗停線**：同一 gate 三次修復仍失敗，回到 architecture review，禁止疊第四個 workaround。
8. **文件先於語意變更**：若研究推翻本規格，先提出 source matrix、反例、影響與驗證方法；核准後同步更新 spec／manifest，再改程式。
9. **禁止 `.omo` 成為真相源**：短期 plan merge 後刪除；未來 Agent 只依本 spec 與 manifest。
10. **close-out 責任不可轉嫁**：遇到 blocked 必須記錄 blocker owner、解除條件與下一個可執行動作，不能以「待後續」標 done。

---

## 12. 工作分解與依賴

```text
SA00 baseline freeze [done]
  → SA01 namespace contract
  → SA02 strategic prior
  → SA03 legacy migration
  → SA04 WeightEngine canonical output
  → SA05 capital-flow anti-corruption
  → SA06 shared production wiring
  → SA07 current exposure
  → SA08 simulation application
  → SA09 REST/Web/MCP parity
  → SA10 F06 shadow ranking
  → SA11 observation + promotion + legacy sunset
  → SA12 closure proof bundle
```

SA10 可在 SA06 後平行開發 store 與 ranking，但不得在 SA08 outcome attribution 完成前進入 observation。其餘依賴不得跨越。

---

## 13. 文件同步規則

- 本文件是 allocation 語意唯一正本。
- `capital-flow-seven-dimension-spec.md` 只保留 capital-flow gate 摘要並連回本文件。
- `fu-7-sector-norm.md` 維持 taxonomy 定義，不複製 allocation 規則。
- retail positioning audit 保留原文；fix manifest 的 F05 連回本文件與專屬 manifest。
- SA11 實作時必須建立對應的 operations runbook 與 verification report，並在建立後加入文件地圖；觀察中的逐日原始資料放 `data/state/`，不把 runtime log 寫進 canonical spec。
