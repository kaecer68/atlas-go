# Macro First-Principles Causal Chain Gap — Spec

> **狀態**: v0.2（kimi-k3 審查 GO-WITH-FIXES，裁決已併入；實作依此版）
> **日期**: 2026-09-16
> **審查紀錄**: opencode-go/kimi-k3 session `ses_f55c3fce8ffeR62GRcip1BC9iW`（2026-09-16），
> verdict **GO-WITH-FIXES**；所有 P0/P1 修正與裁決（dollar_softening 採納 24→29、
> us_earnings_boom 三道護欄、banking_nim 改事件型觸發改名 inflation_moderate）已併入本版
> **分支**: `feat/20260916-macro-first-principles-chains`
> **來源**: 用戶提出的九條第一性基礎因果關係 + 系統宏觀因果鏈盤查（2026-09-16 session）
> **關聯檔案**: `internal/narrative/templates.go`、`internal/narrative/taiwan_stress_index.go`、`internal/orchestrator/regime_inference.go`、`internal/orchestrator/executor_regime.go`
> **審計方法**: ACI 工具（codegraph / gitnexus / atlas-mcp）+ 圖譜影響面分析

---

## 1. 背景與目的

用戶提出九條「大概率成立」的第一性基礎因果關係（戰略常識層），要求盤查本系統宏觀因果鏈是否具備，並把缺的補齊、把「具備但下游吃不到」的接通：

| # | 第一性因果 | 語意 |
|---|---|---|
| L1 | 美加息，股市弱 | 利率↑ → 折現率↑ → 估值壓縮 + 外資流出 |
| L2 | 美降息，股市強 | 利率↓ → 資金回流新興市場 → 估值擴張 |
| L3 | 美元強，黃金弱 | 美元定價反向 + 避險互斥 |
| L4 | 地緣亂，資源漲 | 風險溢酬↑ → 避險/商品受惠 |
| L5 | 通膨降，科技漲 | good disinflation → 折現率預期下降 → 成長股估值擴張 |
| L6 | 通膨溫，銀行漲 | 利率路徑可預測 → NIM 回穩 |
| L7 | 盈利增，美股強 | 盈利驅動的上行可承受較高利率 |
| L8 | 美元貶，科技強 | 美元↓ → 外資回流 → 高Beta 估值擴張 |
| L9 | 戰事停，能源跌 | 去升級 → 戰爭溢價回吐 |

> 九條皆為「大概率」先驗，非機械定律。與既有 `CausalTemplate.HistoricalHitRate`（0.58–0.81）的量化表達同構。

## 2. 現況盤查（2026-09-16）

### 2.1 因果邏輯的承載位置

```
數據層   apigateway 10 個宏觀通道（DXY/US10Y/VIX/USD-TWD/油/金/日圓/GPR/三大法人）
   ↓
事件層   narrative：24 個 Detector（KB pipeline=權威、snapshot pipeline=降級代理）
         → KnowledgeBase 24 條 CausalTemplate（Steps×Impact + HistoricalHitRate）
   ↓
推論層   ① regime 四層傳導鏈（layer_0 宏觀 → layer_4 台股量能 → layer_7 敘事 → layer_root LLM）
         ② TaiwanStressIndex 八維加權
         ③ MacroRiskAssessmentEngine（風險分級 + 外資流出概率 + 板塊輪動）
   ↓
預測/決策層   ForeignForecast scorecard / macroflow / SectorRotator /
              MacroAwareDrawdown / NarrativeConvictionModulator / Darwinian ×21
```

### 2.2 九條邏輯覆蓋矩陣

| # | 偵測器 | 因果鏈模板 | regime theme score | 判定 |
|---|---|---|---|---|
| L1 | ✅ `detectUSRatesEvent` | ✅ `US_rates_up` | ✅ −0.5 | 具備 |
| L2 | ✅ `detectUSRatesDownEvent` | ✅ `US_rates_down` | ✅ +0.5 | 具備（下游吃不到，見 §2.3） |
| L3 | ✅ `detectDollarSurgeKBEvent` | ✅ `dollar_surge`（明文「美元強=黃金弱」） | −0.3 | 具備；`dollar_surge`/`gold_rally` 無跨資產仲裁（backlog） |
| L4 | ✅ geo 偵測 | ⚠️ 部分 | −0.5 | 部分（無獨立「資源普遍漲」鏈） |
| L5 | ❌ 僅 `inflation_spike` 單向 | ❌ | ❌ | **缺口** |
| L6 | ❌ | ❌（銀行受惠掛在升息→NIM） | ❌ | **缺口** |
| L7 | ❌（僅台股版 `earnings_surprise`） | ❌ | ❌ | **缺口** |
| L8 | ❌ | ⚠️ 隱含於 `US_rates_down` | ❌ | 部分 |
| L9 | ❌ | ❌（無 de-escalation 反向鏈） | ❌ | **缺口** |

結論：**3 條完整、4 條部分/間接、2 條（L6、L9）完全缺失**。

### 2.3 結構性缺口（模板有、下游吃不到）

1. **layer_0 `MacroEvidenceSource` 只讀 VIX**（`regime_inference.go::Evidence`）。宏觀層無任何利率/美元方向性知識，全靠 VIX 閾值與下游事件觸發。
2. **`TaiwanStressCalculator` 方向盲目**：`dxy`/`gold`/`jpy`（與 hybrid 模式同名成分）用 `math.Abs(changePct)`——美元走弱、日圓走弱同樣被計成正壓力，違反 L2/L8 方向性。`foreign_flow` 已帶符號、`us10y` 用水位（高=壓力，方向正確）。
3. **`MacroRiskAssessmentEngine` 全部單向**：只有「高於閾值=風險」因子，無正向（tailwind）因子，無法表達 L2/L5/L7。
4. **`ForeignForecast.Scorecard` 無利率特徵**：七特徵（期貨OI/現貨斜率/台積電ADR/SPX/NDX/匯率/VIX）缺利率週期，L1/L2 只能靠 SPX 動能間接代理。
5. **`macroflow` 只有 6 條風險分級規則**，結構上無法表達 risk-on 輪動。

## 3. 範圍（本 spec 的三個 PR）

| PR | 內容 | 行為改變 |
|---|---|---|
| **PR1** | 新增 4 條 CausalTemplate + 對應 Detector + hard gate 同步 | 僅事件觸發時改變（增量） |
| **PR2** | `TaiwanStressCalculator` 方向性修正（含 relief cap） | 行為改變最大，需閾值重校 |
| **PR3** | layer_0 `MacroEvidenceSource` 方向性證據 | 小改動，regime 加厚 |

Backlog（不在本 spec 範圍）：ForeignForecast 利率特徵、MacroRiskAssessment 正向因子、macroflow 事件感知、`dollar_surge`×`gold_rally` 跨資產仲裁、retail sentiment decay 路徑。

## 4. PR1 — 四條新 CausalTemplate

### 4.1 `inflation_cool` 通膨回落（L5）

```go
{
    ID: "通膨回落", TriggerTheme: "inflation_cool", RequiredRegion: "US",
    Steps: []CausalStep{
        {Description: "CPI/PCE 連續低於預期 → 通膨預期回落", Affected: []string{"通膨預期", "實質利率"}, Impact: -0.6},
        {Description: "市場定價聯準會鴿派路徑，降息預期升溫", Affected: []string{"美國利率"}, Impact: -0.6},
        {Description: "折現率下降 → 高估值成長股估值擴張", Affected: []string{"AI供應鏈", "半導體", "中小型股"}, Impact: +0.7},
        {Description: "外資回流新興亞洲，台股資金面寬鬆", Affected: []string{"外資流向_台股", "台股大盤"}, Impact: +0.5},
    },
    HistoricalHitRate: 0.62,
    SourceReferences:  []string{"FRED CPI/PCE Series", "Federal Reserve SEP"},
    Rationale: "inflation_spike 的鏡像鏈。必須區分 good disinflation（需求正常化，股市受惠）與 bad deflation（需求崩潰，盈利下修）——後者由 semiconductor_downturn / china_slowdown 鏈承接，本鏈僅覆蓋前者。",
}
```

### 4.2 `conflict_deescalation` 戰事降溫（L9 + L3 反向）

```go
{
    ID: "戰事降溫", TriggerTheme: "conflict_deescalation", RequiredRegion: "Global",
    Steps: []CausalStep{
        {Description: "停火/談判降溫 → 地緣風險溢酬快速回落", Affected: []string{"地緣政治風險指數"}, Impact: -0.8},
        {Description: "原油回吐戰爭溢價，能源板塊承壓", Affected: []string{"原油", "能源"}, Impact: -0.7},
        {Description: "黃金避險溢價消退，回歸美元定價", Affected: []string{"黃金"}, Impact: -0.5},
        {Description: "風險偏好回升，外資回流，高Beta科技估值修復", Affected: []string{"台股大盤", "AI供應鏈", "外資流向_台股"}, Impact: +0.5},
    },
    HistoricalHitRate: 0.55, // 刻意保守：停火易反覆破裂；de-escalation 的命中率上限天然低於 escalation
    SourceReferences:  []string{"Caldara-Iacoviello GPR", "EIA Oil Market Report"},
    Rationale: "geopolitical_risk_spike 的反向鏈。油價下跌需區分「供給中斷解除」（利多）與「需求崩潰」（利空），僅前者屬本鏈；後者由 oil_price_shock 模板的供給/需求判別承接。",
}
```

### 4.3 `us_earnings_boom` 美股盈利擴張（L7）

```go
{
    ID: "美股盈利擴張", TriggerTheme: "us_earnings_boom", RequiredRegion: "US",
    Steps: []CausalStep{
        {Description: "SPX 財報季超預期比重上升 / EPS 上修廣度擴大", Affected: []string{"美股盈利"}, Impact: +0.7},
        {Description: "美股風險偏好回升，SPX/NDX 走強", Affected: []string{"SPX", "NDX"}, Impact: +0.6},
        {Description: "外資風險預算擴大，加碼台股科技權值", Affected: []string{"外資流向_台股", "台股大盤"}, Impact: +0.5},
        {Description: "終端需求與估值外溢帶動台灣 AI 供應鏈", Affected: []string{"AI供應鏈", "半導體"}, Impact: +0.6},
    },
    HistoricalHitRate: 0.60,
    SourceReferences:  []string{"FactSet Earnings Breadth", "Ball & Brown (1968) JAR"},
    Rationale: "盈利驅動的上行與估值驅動（AI_capex_surge）互補：盈利驅動的強勢對利率上行容忍度較高（L7 的核心含義）。與 earnings_surprise（台股版，台積電財報→台股）觸發來源與傳導起點不同。",
}
```

### 4.4 `inflation_moderate` 溫和通膨（CPI 落入目標帶）（L6）

> k3 審查裁決：原 `banking_nim`（US10Y 平穩 + CPI 溫和共現）為**常駐條件，駁回**——「利率不動 + CPI 在目標區」是世界多數時間的預設狀態，會造成常駐事件與永久 risk-on 偏置。改為**事件型觸發**並改用事件語義命名。

```go
{
    ID: "溫和通膨（CPI 落入目標帶）", TriggerTheme: "inflation_moderate", RequiredRegion: "US",
    Steps: []CausalStep{
        {Description: "CPI 公布落於央行目標區間 → 利率路徑可預測性上升", Affected: []string{"通膨預期"}, Impact: -0.4},
        {Description: "殖利率曲線正常化 → 銀行淨息差（NIM）回穩", Affected: []string{"金融", "銀行"}, Impact: +0.6},
        {Description: "存款流失壓力緩解、信用成本下降", Affected: []string{"金融"}, Impact: +0.4},
        {Description: "台股金融板塊受惠於確定性溢價", Affected: []string{"金融"}, Impact: +0.5},
    },
    HistoricalHitRate: 0.58,
    SourceReferences:  []string{"FDIC Quarterly Banking Profile", "IMF Global Financial Stability Report"},
    Rationale: "銀行受惠的關鍵不是通膨水位，而是「利率路徑可預測性提升」的轉換事件。採 CPI 公布日單次事件觸發（lifecycle duration 7 天 + cooldown），禁止常駐條件（k3 裁決）。與 US_rates_up（急升息 NIM 擴大但信用風險同時升）刻意區分為兩個 trigger。殖利率曲線（2s10s）或 NIM 代理資料源到位前，不做常駐水位觸發（backlog）。",
}
```

事件生命週期：`DefaultThemeDurations()` 設 7 天（單次事件；CPI 月度公布頻率天然節流）。

### 4.5 `dollar_softening` 美元轉弱（L8）— k3 裁決：採納（24→29）

> 裁決理由：(1) 鏈內容有真增量——step3「美元弱→黃金 +0.5」是 L3 反向腿，US_rates_down 模板未覆蓋；(2) 上行側本就有 US_rates_up + dollar_surge 雙主題共燃，下行側補此鏈是恢復對稱。**條件**：觸發對齊 `dollar_surge` 鏡像（`DXYChangePct < -1.5` 單點，不寫 5 日下行——無資料源）；`narrativeThemeScore` 給 +0.5（+0.3 會在共燃時稀釋 US_rates_down 的 +0.5）。

```go
{
    ID: "美元轉弱", TriggerTheme: "dollar_softening", RequiredRegion: "US",
    Steps: []CausalStep{
        {Description: "美元指數回落跌破區間下緣", Affected: []string{"DXY"}, Impact: -0.6},
        {Description: "新興市場貨幣回升，外資回流亞洲", Affected: []string{"外資流向_台股"}, Impact: +0.6},
        {Description: "貴金屬受惠於美元定價反向", Affected: []string{"黃金", "貴金屬"}, Impact: +0.5},
        {Description: "台幣升值匯率壓力被資金面利多抵消，AI供應鏈估值擴張", Affected: []string{"AI供應鏈", "出口股"}, Impact: +0.4},
    },
    HistoricalHitRate: 0.60,
    SourceReferences:  []string{"Federal Reserve DXY Index", "BIS Triennial Survey"},
    Rationale: "dollar_surge 的反向鏈，補齊 L8 獨立主題（現僅隱含於 US_rates_down 步驟）。",
}
```

> 事件生命週期：`DefaultThemeDurations()` 設 7 天。

### 4.6 偵測器設計（k3 裁決後的可實作版本）

> k3 指出：KB pipeline 偵測器是**無狀態單點函式**（`MarketNarrativeData` 無歷史欄位），「連續 N 日趨勢」「5 日下行」在現有資料結構下不可表達（tariff_shock 當年同樣缺口走 snapshot pipeline 解決）。GPR 在 snapshot 路徑恒為 0（`snapshot_converter.go:56`）。故所有新偵測器只用單點或 curr/prev 雙點可表達的條件。

| Theme | Pipeline | 觸發條件（可實作） |
|---|---|---|
| `inflation_cool` | KB（單點帶狀） | `CPIYoY` 落入目標帶下半（如 ≤ 2.4% 帶內）→ 單點狀態事件；靠 lifecycle duration 7 天 + cooldown 防 spam；不做「連續 N 日」趨勢（無歷史欄位，backlog：DetectorInput 加歷史） |
| `conflict_deescalation` | **snapshot（curr/prev 雙點）**，比照 `tariff_shock` | 複合條件：`Oil.ChangePct < -2%` **且** `Gold.ChangePct < -1%` **且** VIX 下降——停火的可觀察市場印記（能源跌+金跌+波動回落），不依賴 GPR 歷史；`SourceIngestor` |
| `us_earnings_boom` | KB（複合條件代理，三道護欄） | ① SPX 與 NDX 同向上漲（`SPXChangePct > +1%` 且 NDX 同向）**且** VIX 下降（排除單純降息反彈）② `Confidence` cap 0.4 ③ `Metadata["proxy"]=true`，HistoricalHitRate 標 proxy 層級 0.50；正式盈利廣度資料源列 backlog |
| `inflation_moderate` | KB（單點帶狀，事件型） | `CPIYoY` 落入央行目標帶（如 ≤ 2.5% 且 ≥ 下緣）單次觸發；CPI 月度公布頻率天然節流；duration 7 天 |
| `dollar_softening` | KB（單點，鏡像） | `DXYChangePct < -1.5`（與 `dollar_surge` 的 `> +1.5` 鏡像；溫和下行只觸發 US_rates_down，強下行才雙燃） |

> 雙軌歸屬依 `internal/narrative/AGENTS.md` 陷阱 1：`inflation_cool` / `us_earnings_boom` / `inflation_moderate` / `dollar_softening` 走 KB pipeline；`conflict_deescalation` 走 snapshot pipeline（Ingestor 降級代理模式，即 `detectTariffShockEventFromSnapshot` 同款）。

## 5. PR2 — TaiwanStressCalculator 方向性修正

### 5.1 現況問題

`internal/narrative/taiwan_stress_index.go::Calculate`：

- `foreign_flow`：帶符號 ✅（外資淨賣超=壓力、買超=負壓力）
- `us10y`：水位制 ✅（殖利率高=壓力，低=自然減壓）
- **`dxy` / `gold` / `jpy`：`math.Abs(changePct)` ❌ 方向盲目**——美元/黃金/日圓下跌（L2/L8 的風險偏好方向）同樣被計成正壓力

### 5.2 修正設計

1. 新增 `SignalDirectional` 策略（與 `SignalHybrid` 並列，baselines 檔存在時可啟用）：
   - `dxy`：利空方向（DXY↑）計正壓力；利多方向（DXY↓）計**負值舒緩項**
   - `gold`：同上（金↑=避險壓力；金↓=舒緩）
   - `jpy`：日圓↑（套利平倉方向）=壓力；日圓↓=舒緩
2. **Relief cap 單位定義（k3 P1 修正 + 實作期數學更正）**：cap 定義在 **pre-weight 0–100 成分制**，每成分舒緩下限 **−50**（`clampComponentDirectional`），再乘 scale 與權重。post-weight 每成分下限 = −50 × scale × weight（以預設值算：dxy −50×5×0.13 = −32.5、jpy −50×10×0.08 = −40、gold −50×2×0.06 = −6）。**修正 k3 估算**：k3 的「post-weight 合計 −13.5 分」把 scale 位置算錯（其兩種讀法都不含 scale 因子）；正確總下限 = 50×(5×0.13+10×0.08+2×0.06) = **−78.5 分**，但僅在極端尾端（DXY −10%、JPY −5%、Gold −25% 同時）才會觸底——**典型 −2% 級利多移動的總舒緩 ≈ −3.1 分**。Alert=30 / High=50 / Crisis=70 閾值的重校幅度以 calibration 對照報告實測為準（常態移動區間位移小、尾部有大下限）。
3. **`clampComponent` 必須方向化（k3 P1）**：現行 `clampComponent`（`taiwan_stress_index.go:140-148`）把 `v < 0` 夾到 0——directional 路徑必須改用允許負值的 clamp（`[-50, 100]`），否則舒緩項永遠出不了來。Score 總和最後仍 clamp 到 `[0, 100]`。
4. `oil` 維持 `Abs`：油價下跌有供給解除/需求崩潰雙義（`oil_price_shock` 模板 rationale 已記載判別原則）；方向性交由 `conflict_deescalation` 事件鏈做 cross-factor 調變（phase 2 backlog）。
5. **閾值重校幅度如實評估（k3 P1）**：方向化後，強 risk-on 情境的分數最多下移 ~13.5 分（0–100 制），Alert=30 邊界可能大量穿越。上線前必跑 calibration 對照（新舊分數分布差異報告），據報告決定閾值是否重定義——**不預設「小幅重校」**。
6. **calibration package 同步清單（k3 P2）**：
   - `calibration/weight_calibration.go`：`SignalDirectional` enum；**注意 `calibration_baseline_test.go:345` 有 `SignalHybrid == 2` 的值斷言，新增值放枚舉尾**（Directional = 3）
   - `calibration_facade.go:20-32` alias
   - `factorSignalWithStrategy` switch（`weight_calibration.go:309`）
   - `useHybridSignal()` 泛化為 `useSignalStrategy(SignalDirectional)`
   - `NewTaiwanStressCalculator` 的自動啟用優先序（`taiwan_stress_index.go:62-67` 現為「baselines 存在即 SignalHybrid」→ 改為 baselines 存在即 SignalDirectional，並保留 hybrid 常數相容）
7. 部署順序：方向化上線 → calibration 對照報告 → 更新閾值 → 驗證 regime 一致性（`monitoring/service/regime_consistency.go`）。

### 5.3 驗收測試

- DXY −2% 單獨變動 → score 下降（現行為：上升）
- DXY +2% 單獨變動 → score 上升（與現行一致）
- US10Y 不變、DXY −2% → pre-weight `components["dxy"]` 為負且 ≥ −50（pre-weight 0–100 制）
- 舒緩總量測試：全利多情境的 score 下移量 ≤ 0.5 × (wDXY+wJPY+wGold)
- hybrid/directional 兩策略的向後相容測試 + `calibration_baseline_test.go` 值斷言不破壞

## 6. PR3 — layer_0 MacroEvidenceSource 方向性證據

### 6.1 現況與資料可得性（k3 P0 修正後的如實描述）

`regime_inference.go::MacroEvidenceSource.Evidence` 只讀 `quotes["VIX"]`（或 `^VIX`）：
- VIX > 1.5×volThreshold → −0.8 / conf 0.7；> volThreshold → −0.4 / conf 0.5；**< 0.7×volThreshold → +0.4 / conf 0.5**；無 VIX → conf 0（#1785 不出幻影票）。
- `RegimeEvidenceSource` 介面簽名**不需變更**（quote map 是通用型別）。DXY/US10Y 是否存在於 `inferRegime` 收到的 map **取決於 session quote universe**（`ctx.Quotes`，executor_pipeline.go:73-75）；#1785 註解記載 replay/小樣本情境連 VIX 都常缺席。`system.go:1340-1343` 只證明轉換器會解析這些 symbol，不保證該 map 有鍵。
- **降級設計**：DXY/US10Y 缺席時子證據 confidence=0，行為與現行完全相同。實作時以 production quote universe 實測確認鍵覆蓋。

### 6.2 修正設計（k3 P1 修正後的合成公式）

```text
VIX 證據（現行保留，為主訊號）之外，疊加兩個帶符號子證據（每個 bounded ±0.4）：
1) ratesEvidence：US10Y ChangePct 超過 +rateThreshold → −0.4（L1：加息=折現率壓縮）
                 低於 −rateThreshold → +0.4（L2：降息=資金回流）
                 conf_rates = 0.5
2) dollarEvidence：DXY ChangePct > +dxyThreshold → −0.3（L8 反向：美元強=新興市場壓力）
                  < −dxyThreshold → +0.3（L8：美元弱=回流受惠）
                  conf_dollar = 0.5
單位（k3 P2）：threshold 為盤中開到最新的變動率（%）。US10Y 1% ≈ 4.5bps，
             預設 rateThreshold=0.5%（≈2.2bps 盤中變動）、dxyThreshold=0.8%。

合成公式（避免稀釋主訊號，k3 P1 裁決）：
  同向（所有非零子證據同號）→ score = max(|score_i|) 帶符號，
                              confidence = min(1, max(conf_i) + 0.1×(其餘同向子證據數))
  反向（子證據符號衝突）    → score = Σ(score_i × conf_i) / Σ(conf_i)（加權平均），
                              confidence = min(conf_i)
缺資料處理：子證據無對應 quote → 該子項 confidence = 0（沿用 #1785 模式）；
           全部缺席 → 沿用現行（VIX 缺席即 conf 0）。
參數：閾值進 ParametersConfig（Realtime.RateMoveThresholdPct / DXYMoveThresholdPct），
     不硬編碼——沿用 NarrativeConvictionModulator「策略 IP 放 config」先例。
```

> 設計理由：加權平均在「VIX 危機 + rates 同向」時會把 −0.8 的主訊號稀釋到 < 0.8，確認性證據反而削弱主訊號（k3 P1）。同向取 max、反向才平均，保證「確認訊號至少不弱於單一最強子訊號」。

### 6.3 影響面

`MacroEvidenceSource` 僅 `regime_inference.go` + `executor_regime.go::inferRegime` 單一實例化鏈（k3 人工驗證成立；GitNexus index 落後 237 commits、interface dispatch 無法自動解析，以人工驗證為準），無跨模組風險。

### 6.4 生效範圍（k3 P1 如實揭露）

**Production 的 regime 判定由 authority 主導**：`executor_regime.go:112-133` 中，`RegimeAuthorityFunc`（regime_history，由 macro_ingest 依 stress index 持續寫入）存在時**直接回傳 authority 值**，四層推論僅為 advisory（寫入 scratchpad trace）。因此：
- PR3 在 production **不改變任何 regime 判定**，只影響 cold-start / sim fallback 與 reasoning trace。
- **Production 的方向性改善由 PR2 承載**（stress index → regime_history → authority），PR2 才是關鍵路徑；PR3 價值在 sim/回測/小樣本情境與 trace 可觀測性。

### 6.5 驗收測試

- US10Y 大幅上行 + VIX 低 → macro 層給負分（現行為 +0.4；k3 修正後的正確現況描述）
- US10Y 大幅下行 + VIX 低 → macro 層給正分
- quotes 無 US10Y/DXY → 該子項 conf=0、不出幻影票
- VIX 危機 + rates 同向 → 合成 magnitude ≥ 0.8（不稀釋）
- VIX 中性 + rates 反向 → 走加權平均分支
- 與 layer_7 敘事分數的矛盾情境：上層 macro RISK_OFF + layer_7 RISK_ON → confidence 減半邏輯不受影響

## 7. Hard Gate 同步清單（k3 審查後擴充版；N=5：inflation_cool / conflict_deescalation / us_earnings_boom / inflation_moderate / dollar_softening）

**程式碼**
1. `internal/narrative/templates.go` — `DefaultTemplates()` ＋5 條
2. `internal/narrative/narrative_detectors.go` — ＋5 個 KB 偵測函式（§4.6 表）；`getThemeDuration` switch 補 5 個 case（此為第二份 duration 地圖，`narrative_detectors.go:1121`）
3. `internal/narrative/detector_impls.go` — ＋5 個 detector struct + `NewDefaultDetectorRegistry()` 內 `MustRegister`（`detector_impls.go:401-431`）+ compile-time interface check（比照 L436-456）
4. `internal/narrative/lifecycle.go` — `DefaultThemeDurations()` 補 5 個主題（EventLifecycleManager canonical 來源，`lifecycle.go:23-52`）
5. `internal/orchestrator/regime_inference.go` — `narrativeThemeScore()` 補映射（收斂既有 tier 風格 + 對稱性依據）：
   `conflict_deescalation` +0.5（`geopolitical_risk_spike` −0.5 的鏡像）/ `us_earnings_boom` +0.5 / `dollar_softening` +0.5（k3 裁決，共燃時不可稀釋 US_rates_down）/ `inflation_cool` +0.3 / `inflation_moderate` +0.3（`inflation_spike` −0.3 的鏡像 → tier 集合擴為 −0.5/−0.3/+0.3/+0.5/+0.1，鏡像對稱性即分層依據）

**測試 golden（k3 P0：遺漏即 CI 紅）**
6. `internal/narrative/detector_e2e_test.go` — `expectedCount = 24 → 29`
7. `internal/narrative/detector_impls_test.go` — `allExpectedThemes` ＋5
8. `internal/narrative/testdata/default_templates.golden.json` — 以測試 snapshot 機制重新生成（`knowledge_base_api_test.go:30-41` `TestDefaultTemplates_Golden`）
9. `internal/narrative/testdata/default_theme_durations.golden.json` — 重新生成（`knowledge_base_api_test.go:47-58`）

**前端標籤（兩處，k3 P1）**
10. `shared_web/static/js/shared/theme-labels.js` — `THEME_LABELS` 補 5 鍵
11. `shared_web/static/js/shared/constants.js`（:23,:38）— 第二處主題標籤 map 補鍵；既有重複標籤不一致（'台灣出口暢旺' vs '台灣出口強勁'）一併統一

**PR3 專屬**
12. ParametersConfig — `Realtime.RateMoveThresholdPct` / `DXYMoveThresholdPct` 新鍵：`defaults_narrative.go` defaults + validate + **`internal/config/testdata/default_parameters_config.golden.json` 重新生成**

**明確不做的決定（k3 建議明寫）**
13. 不為 5 個新主題新增 `NarrativeConviction.ThemeHitRates` config 鍵（現狀僅 5/24 主題有鍵、19 個既有主題本就無鍵；事件命中率走 `hitRateForTheme` 自動查表 `ingestor.go:360-373`，與此 config 無關）
14. 不新增 InvestmentModel（`knowledge_base.go:358` ActiveThemes 模式）與 eventdriven `type_theme_mapping.go` 映射——新主題先以模板/偵測器/主題分數層級運作，演化模型列 backlog
15. `internal/narrative/AGENTS.md` — 模組快照 24 → 29；記錄 duration 雙地圖既有技術債（US_rates_up：14d vs 7d，本次不合併，僅補 case）

**PR2 專屬**：§5.2 第 6 點的 calibration package 同步清單（enum/alias/switch/clamp/自動啟用優先序 + `calibration_baseline_test.go:345` 值斷言相容）

## 8. 驗收標準（整體）

- `gofmt -l internal/narrative/ internal/orchestrator/` → 0
- `go test -count=1 ./internal/narrative/... ./internal/orchestrator/... ./internal/calibration/...` 全綠
- golden 檔案（templates/durations/parameters config）全部重新生成且測試綠
- PR2 附 calibration 新舊分數分布對照報告
- CI：`make ci-full` 綠燈；PR 流程走 `gh pr create`（Summary/Root Cause/Verification 三段）
- 禁止 `git push --no-verify`；未經用戶同意不 merge

## 9. 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v0.1 | 2026-09-16 | 初稿：九條第一性因果盤查 + 三 PR 設計（待 k3 審查） |
| v0.2 | 2026-09-16 | k3 審查（GO-WITH-FIXES）裁決併入：golden 檔同步、relief cap 單位定義（pre-weight −50）、layer_0 合成公式改同向-max/反向-平均、生效範圍如實揭露（production advisory-only）、banking_nim 改事件型 inflation_moderate、dollar_softening 採納（24→29）、偵測器條件改可實作版本 |
