# ATLAS 方法論憲章

> **版本**: v1.0
> **狀態**: 正式（生效中）
> **定位**: 全專案唯一真理源頭 — 所有策略邏輯、推薦輸出、前端展示、AI prompt 皆須與本文一致。
> **關聯實作**: `internal/domain/shared/shared.go`（Regime）、`internal/macroflow/`（MacroFlow）、`internal/portfolio/regime.go`（RegimeAllocator）、`internal/narrative/`（Narrative Engine）、`internal/capitalflow/`（七大資金勢力）
> **會員鎖住三原則**: 見 [`docs/guides/membership-gating.md`](guides/membership-gating.md)（2026-08-19 業主定案；第二層公開數據開放即其落地）

---

## 現狀報告（程式碼對應）

以下為本憲章各層級在 atlas-go 程式碼中的既有實作位置，供開發時交叉參照：

| 方法論層級 | 程式碼對應 | 關鍵類型／函數 |
|-----------|-----------|---------------|
| 七時期判斷 | `internal/portfolio/period_detector.go` | `PeriodDetector.DetectPeriod()` → `MarketPeriod` |
| 時期→三態映射 | `internal/portfolio/period_detector.go` | `PeriodToRegime(MarketPeriod)` → `Regime` |
| 時期→風險層級 | `internal/portfolio/period_detector.go` | `PeriodToRiskLevel(MarketPeriod)` → `macroflow.RiskLevel` |
| 策略時期過濾 | `internal/methodology/advisor.go` | `Advisor.AllowedStrategies(MarketPeriod)` / `Advisor.FilterStrategies(MarketPeriod, []string)` |
| 市場狀態判定（向下相容） | `internal/domain/shared/shared.go` | `Regime`（`RISK_ON` / `RISK_OFF` / `NEUTRAL`）|
| 狀態配置映射 | `internal/portfolio/regime.go` | `RegimeAllocator`、`DefaultRegimeConfigs()`、`RegimeDetector.Detect()` |
| 即時微觀狀態 | `internal/realtime/regime_adapter.go` | `RegimeDetector`（7 種微觀狀態：calm/volatile/trending_up/trending_down/reversing/breakout/breakdown） |
| 模擬動態閾值 | `internal/sim/dynamic_threshold.go` | `DynamicThresholdEngine`（bull/bear/neutral/highvol）+ `RegimeFromDomain()` |
| 宏觀風險層級 | `internal/macroflow/` | `RiskLevel`（yellow/orange/red）、`Engine.Compute()`、`AdjustmentResult` |
| 台灣壓力指數 | `internal/portfolio/stress_index.go`、`internal/narrative/taiwan_stress_index.go` | `TaiwanStressCalculator`（VIX/DXY/US10Y 三元件，0-100 分） |
| 資金流向 | `internal/marketdata/twse_capital_flow_provider.go` | `TWSECapitalFlow`（外資/投信/自營商） |
| 七大資金勢力 | `internal/capitalflow/` | Z-score + 共振係數 + 品質分數 |
| 事件驅動 | `internal/eventdriven/` | 5 日 forward 預測 + ETF 規模×權重 |
| 敘事引擎 | `internal/narrative/detector_impls.go` | 24 個 template trigger detectors（US_rates_up/down、JPY_carry_unwind、tariff_shock 等） |
| 策略分層 | `internal/domain/shared/shared.go` | `AgentLayer`（context/macro/sector/style/superinvestor/control） |
| 策略執行管線 | `internal/orchestrator/executor_pipeline.go` | `ExecuteWithContext()`（RegimeInference → Collection → MomentumCrashProtection → WeightApplication → MacroFlow → ControlLayer） |

**關鍵觀察**：
- `PeriodDetector.DetectPeriod()` 已實作七時期判斷（低迷／轉折開高／上升／高原／盤整／轉折下壓／黑天鵝），並透過 `PeriodToRegime()` 向下相容映射到三態 `Regime`。
- `macroflow.RiskLevel` 與七時期存在自然映射關係（見第五節），已由 `PeriodToRiskLevel()` 提供自動推導。
- `Advisor.AllowedStrategies()` / `Advisor.FilterStrategies()` 已按當前時期過濾策略，確保 RISK_OFF 時期不推薦 growth/momentum。
- 壓力指數已有 VIX/DXY/US10Y 三元件，符合本憲章「美台資金開關」觀測框架。

---

## 一、投資哲學

> **全球宏觀流動性決定資金方向，美股科技估值決定台股基本面動能，外資資金流決定台股量能水位，內資勢力角力創造結構性機會，散戶應跟隨聰明錢並利用事件套利。**

### 核心原則

1. **由上而下，由外而內**：從全球利率與匯率出發，逐步收斂至台灣個股，不可反向推導。
2. **資金比消息重要**：資金流的量與方向比新聞標題更有預測力；消息是用來解釋資金流的，不是用來預測資金流的。
3. **跟隨聰明錢**：散戶不創造趨勢，只跟隨趨勢。外資、投信、公司派是「聰明錢」，散戶情緒是反向指標。
4. **事件創造錯價**：ETF 調整、MSCI 權重變更、營收公告、除權息等可預測事件，會在短期內創造可捕捉的資金流動與錯價。
5. **保守者存活**：任何時期都保留足夠現金部位；不在不確定性高時重押；不在看不懂的時候進場。
6. **AI 不是預測股價，而是捕捉資金流動的必然性與慣性**：AI 的價值不在預測明天漲跌，而在辨識「資金流向的品質」— 當聰明錢（外資、投信）方向一致且剛啟動時跟進；當聰明錢與散戶對做時站在聰明錢那一側；當事件創造出短期必然的資金流（ETF 換股、除息回補、MSCI 調整）時進行套利。短期必然性與中期慣性才是可回測、可信任的信號來源。

---

## 二、因果傳導鏈（完整路徑）

```text
┌─────────────────────────────────────────────────────────────────────────┐
│ 第〇層：全球資金總開關                                                    │
│                                                                         │
│  美債實質利率 (US10Y TIPS) ──→ 全球風險偏好                              │
│  美元指數 (DXY)            ──→ 新興市場資金流入／流出                     │
│  日圓 (USD/JPY)           ──→ 利差交易 (carry trade) 動能                │
│  Fed 政策利率預期          ──→ 資金成本預期                              │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ 第一層：美股科技估值                                                      │
│                                                                         │
│  費城半導體指數 (SOX)       ──→ 全球半導體需求信號                        │
│  台積電 ADR (TSM)          ──→ 台股權值龍頭估值錨                        │
│  NVIDIA (NVDA)             ──→ AI 資本支出領先指標                        │
│  NASDAQ-100                ──→ 科技成長股風險偏好                         │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ 第二層：台灣出口與半導體景氣                                               │
│                                                                         │
│  台灣出口訂單 (MOEA)        ──→ 基本面方向                                │
│  半導體設備進口             ──→ 產能擴張預期                              │
│  台積電月度營收             ──→ 產業鏈拉貨信號                            │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ 第三層：外資資金流與匯率                                                   │
│                                                                         │
│  外資期貨未平倉             ──→ 外資對台股方向性押注                       │
│  外資現貨買賣超             ──→ 外資實際資金進出                          │
│  新台幣匯率 (USD/TWD)       ──→ 熱錢流入／流出即時信號                    │

> **外資雙重動機模型**：流入台股的外資並非單一行為者，而是由兩種不同性質的資金組成：
> - **結構性配置資金**（主權基金、退休基金、全球指數追蹤基金）：進出緩慢、量體穩定，因台灣是全球高階晶片唯一供應源而**必須持有**台積電等權值股。這類資金決定台股的**底部**。
> - **投機性資金**（對沖基金、熱錢、ETF 短線交易）：進出快速、量大，追逐短期報酬與匯率價差。這類資金決定台股的**漲幅**。
>
> 兩者行為模式截然不同：結構性資金在低迷期不離場（甚至逢低加碼），投機性資金在風險偏好轉變時秒撤。判斷外資流向時，若能區分兩者，對時期的判斷將更精確。
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ 第四層：台股大盤量能                                                      │
│                                                                         │
│  加權指數                   ──→ 價格方向                                  │
│  集中市場成交量              ──→ 市場參與熱度                             │
│  融資餘額                   ──→ 散戶槓桿水位                              │
│  當沖交易佔比               ──→ 市場投機熱度                              │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ 第五層：內資勢力反應                                                      │
│                                                                         │
│  投信買賣超                 ──→ 被動資金配置（ETF、共同基金）              │
│  公股券商買賣超             ──→ 政策護盤信號                              │
│  壽險／銀行資金             ──→ 長線配置資金                              │
│  公司派／內部人             ──→ 內部人對公司價值的判斷                     │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ 第六層：散戶情緒與籌碼                                                    │
│                                                                         │
│  散戶買賣超                 ──→ 散戶實際行為                              │
│  融資維持率                 ──→ 散戶壓力水位                              │
│  Google Trends              ──→ 散戶關注熱度（反向指標）                  │
│  券商分行買賣統計           ──→ 散戶進出結構                              │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ 第七層：可捕捉的資金事件與錯價                                             │
│                                                                         │
│  ETF 成份股調整             ──→ 被動資金強制買入／賣出                    │
│  MSCI 季度權重變更          ──→ 外資被動調整                              │
│  營收公告密集期             ──→ 基本面驚喜可能性                          │
│  除權息季節                 ──→ 股息再投資與稅務考量                      │
│  股東會季節                 ──→ 公司派維穩                                │
│  年底作帳行情               ──→ 投信／公司派美化帳面                      │
└─────────────────────────────────────────────────────────────────────────┘
```

**使用方式**：任何策略信號必須能在這條傳導鏈上標註自己的位置。不能標註的信號，其因果基礎存疑。

---

## 三、七個市場時期定義與判別規則

七個時期形成一個完整的市場循環。時期不是靜態分類，而是動態轉換 — 每個時期都有進場條件和出場條件。

### 時期總覽

```text
         低迷 ──→ 轉折開高 ──→ 上升(多頭) ──→ 高原
          ↑                                      │
          │                                      ▼
       黑天鵝  ←── 轉折下壓 ←──────────────  盤整
          │                         ↗
          └─────────────────────── (跳躍，非線性)
```

### 時期定義

#### 1. 低迷（Downturn）

> **市場特徵**：恐慌已釋放，但信心未恢復。價格不再創新低，但上漲意願薄弱。

| 指標 | 條件 |
|------|------|
| 外資賣超 | 賣超幅度顯著縮小（連續 5 日賣超均值 < 前波賣超峰值的 30%） |
| 融資餘額 | 較高點減少 > 15%，且近 5 日不再大減 |
| 公股券商 | 連續買超 > 5 日 |
| VIX | > 25，但不再創高（近 5 日高點下降） |
| 新台幣 | 貶勢趨緩（近 5 日變動 < 0.3%） |
| 加權指數 | 站回 5 日線，但仍低於 20 日線 |

**程式碼映射**：`domain.RegimeRiskOff` + `macroflow.RiskOrange`（若 VIX stress → orangeStressRules）

#### 2. 轉折開高（Turnaround Up）

> **市場特徵**：聰明錢率先進場。外資突然轉買，匯率急升，美股科技突破關鍵均線。

| 指標 | 條件 |
|------|------|
| 外資現貨 | 單日買超 > 100 億 或 連續 3 日買超 |
| 新台幣 | 單日升值 > 0.3% 或 3 日累積升值 > 0.5% |
| 費半 (SOX) | 突破 50 日線（或 20 日線向上穿越 50 日線） |
| 台積電 ADR | 單日漲幅 > 2% 且收盤高於前 5 日高點 |
| 外資期貨 | 未平倉多單增加 > 3,000 口 |

**程式碼映射**：`domain.RegimeNeutral` → `domain.RegimeRiskOn` 的過渡期；`macroflow.RiskYellow`

#### 3. 上升／多頭（Bull）

> **市場特徵**：趨勢確立，資金持續流入，散戶開始跟進。最適合跟隨聰明錢的策略。

| 指標 | 條件 |
|------|------|
| 外資現貨 | 連續買超（過去 10 日 > 7 日買超） |
| 外資期貨 | 未平倉多單持續增加或維持高水位（> 30,000 口） |
| 融資餘額 | 溫和增加（日均增幅 < 1%） |
| 加權指數 | 站穩 20 日線之上，20 日線斜率向上 |
| 新台幣 | 維持強勢（月線以下或區間偏升） |

**程式碼映射**：`domain.RegimeRiskOn`；`macroflow.RiskYellow`

#### 4. 高原（Plateau）

> **市場特徵**：漲勢停滯但未反轉。外資買盤減弱，當沖佔比升高，類股輪動加快。

| 指標 | 條件 |
|------|------|
| 外資買超 | 幅度顯著縮小（3 日均值 < 前 10 日均值的 50%） |
| 外資期貨 | 未平倉多單開始減少（連續 3 日減少） |
| 當沖佔比 | > 35%（集中市場） |
| 加權指數 | 在 20 日線上下 2% 內狹幅整理 |
| 類股輪動 | 近 5 日漲幅前 3 名類股與前 5 日不同（輪動指標） |

**程式碼映射**：`domain.RegimeNeutral`；`macroflow.RiskYellow`

#### 5. 盤整（Consolidation）

> **市場特徵**：方向不明，外資忽買忽賣，匯率區間震盪。不適合趨勢跟隨策略。

| 指標 | 條件 |
|------|------|
| 外資現貨 | 買賣交錯（近 10 日中，買超日與賣超日各 > 3 日） |
| 新台幣 | 在月線上下 0.5% 內震盪 |
| 類股輪動 | 快速輪動，無持續領漲類股（單一類股連續領漲 < 3 日） |
| 成交量 | 萎縮至 20 日均量的 70%-100% |
| 加權指數 | 在一個明確區間內波動（區間振幅 < 5%） |

**程式碼映射**：`domain.RegimeNeutral`；`macroflow.RiskYellow`

#### 6. 轉折下壓（Turnaround Down）

> **市場特徵**：聰明錢率先離場。外資連續大賣，新台幣貶破月線，融資開始斷頭。

| 指標 | 條件 |
|------|------|
| 外資現貨 | 連續 3 日賣超，且單日賣超 > 150 億至少一次 |
| 新台幣 | 貶破月線（20 日 SMA），且貶值加速 |
| 融資維持率 | 降至 150% 以下 |
| 費半 (SOX) | 跌破 50 日線 |
| 外資期貨 | 未平倉轉為淨空單或淨多單大幅減少（> 10,000 口） |
| 地緣政治（台海緊張升溫） | 地緣強度 GeoIntensity ≥ 40 且 5 日趨勢非下降（GeoIntensityChange5D ≥ 0；無歷史資料時僅以當日強度判定） |

**程式碼映射**：`domain.RegimeRiskOff`；`macroflow.RiskOrange`

#### 7. 黑天鵝（Black Swan）

> **市場特徵**：無差別拋售，流動性枯竭，政府宣布干預。不適合任何策略，應全面轉為現金或防禦。

| 指標 | 條件 |
|------|------|
| 外資現貨 | 單日賣超 > 500 億 或 連續巨量賣超 |
| VIX | > 35（`macroflow.Engine.isStressful()` 觸發條件） |
| 加權指數 | 單日跌幅 > 5% 或連續下跌累積 > 10% |
| 國安基金 | 宣布進場護盤 |
| 新台幣 | 單日重貶 > 0.5%（熱錢恐慌撤離） |
| 地緣政治（台海危機） | 地緣強度 GeoIntensity ≥ 60（4 級制 ≥ 高張(3)） |

**程式碼映射**：`domain.RegimeRiskOff` + `macroflow.RiskRed`（stress flag = true → redStressRules: 防禦 +25%，攻擊 -30%，現金 +15%）

### 時期轉換矩陣

| 從 → 到 | 低迷 | 轉折開高 | 上升 | 高原 | 盤整 | 轉折下壓 | 黑天鵝 |
|---------|------|---------|------|------|------|---------|--------|
| **低迷** | — | ✅ | — | — | — | — | — |
| **轉折開高** | ✅ (假突破回歸) | — | ✅ | — | — | ✅ (假突破後反殺) | — |
| **上升** | — | — | — | ✅ | — | — | — |
| **高原** | — | — | ✅ (突破續漲) | — | ✅ | ✅ (頭部確立) | — |
| **盤整** | — | ✅ (整理後突破) | — | — | — | ✅ (整理後破底) | — |
| **轉折下壓** | ✅ (賣壓衰竭) | — | — | — | — | — | ✅ |
| **黑天鵝** | ✅ (恐慌消退) | — | — | — | — | — | — |

### 判別聚合規則（v1.1，2026-08-25 補）

各時期判定為「條件命中數 ≥ 閾值」；零值輸入視為資料不可用（unavailable），該條件自動跳過，不計入分母影響（分母固定為該時期條件總數）：

| 時期 | 條件總數 | 命中門檻 |
|------|---------|---------|
| 黑天鵝 | 6 | ≥ 1（任一條件成立即觸發） |
| 轉折下壓 | 6 | ≥ 3 |
| 低迷 | 5 | ≥ 3 |
| 轉折開高 | 5 | ≥ 2 |
| 上升 | 5 | ≥ 3 |
| 高原 | 5 | ≥ 3 |
| 盤整 | 4 | ≥ 3 |

判別順序依優先鏈：黑天鵝 → 轉折下壓 → 低迷 → 轉折開高 → 上升 → 高原 → 盤整（最後兜底）；
全鏈皆不命中時回傳盤整（consolidation），`is_fallback=true` 表示無任何非零輸入。

**程式碼映射**：`internal/portfolio/period_detector.go` `DetectAssessment()`；閾值於
`internal/config/period_detection_config.go` `DefaultPeriodDetectionConfig()` 參數化。

### 參數欄位對照表（Go field → 憲章閾值，drift-check 用，2026-08-29 新增）

此表為 `internal/config/period_detection_config.go` `PeriodDetectionConfig` 的欄位名與本憲章 §3 閾值的對照，供 `scripts/ci/check_constitution_drift.sh` drift 偵測使用。欄位名本身即為機讀標記，數值與上節各時期表格一致。

| Go 欄位 | 預設值 | 對應憲章描述 |
|---|---|---|
| BlackSwanForeignSellBillion | 500 | 外資單日賣超 > 500 億（黑天鵝） |
| BlackSwanVIX | 35 | VIX > 35（黑天鵝） |
| BlackSwanTAIEXDeclinePct | 5 | 加權指數單日跌幅 > 5%（黑天鵝） |
| BlackSwanTWDDepreciationPct | 0.5 | 新台幣單日重貶 > 0.5%（黑天鵝） |
| BlackSwanGeoIntensity | 60 | 地緣強度 ≥ 60（黑天鵝，4 級制 ≥ 高張） |
| TurnDownConsecSellDays | 3 | 外資連續賣超 3 日（轉折下壓） |
| TurnDownSingleSellBillion | 150 | 單日賣超 > 150 億（轉折下壓） |
| TurnDownMarginMaintRatio | 150 | 融資維持率 < 150%（轉折下壓） |
| TurnDownFuturesOIDecrease | 10000 | 期貨未平倉減少 > 10,000 口（轉折下壓） |
| TurnDownGeoIntensity | 40 | 地緣強度 ≥ 40（轉折下壓，4 級制 ≥ 升溫） |
| DownturnSellRatioToPeak | 0.30 | 5 日均值 / 峰值 < 0.30（低迷） |
| DownturnMarginReductionPct | 0.15 | 融資較高點減少 > 15%（低迷） |
| DownturnPublicBankBuyDays | 5 | 公股連續買超 > 5 日（低迷） |
| DownturnVIXMin | 25 | VIX > 25（低迷） |
| TurnUpSingleBuyBillion | 100 | 外資單日買超 > 100 億（轉折開高） |
| TurnUpConsecBuyDays | 3 | 外資連續買超 3 日（轉折開高） |
| TurnUpTWDApprec1DPct | -0.3 | 新台幣單日升值 > 0.3%（轉折開高，負值表示升值） |
| TurnUpTWDApprec3DPct | -0.5 | 新台幣 3 日累積升值 > 0.5%（轉折開高） |
| TurnUpFuturesOIIncrease | 3000 | 期貨多單增加 > 3,000 口（轉折開高） |
| TurnUpTSMADRPct | 2.0 | TSM ADR 單日漲幅 > 2%（轉折開高） |
| BullForeignBuyRatio10 | 0.7 | 近 10 日買超佔比 > 0.7（多頭） |
| BullFuturesOIMin | 30000 | 期貨多單 > 30,000 口（多頭） |
| BullMarginDailyMaxPct | 1.0 | 融資日均增幅 < 1%（多頭） |
| PlateauBuyRatio3to10 | 0.50 | 3 日均值 / 10 日均值 < 0.50（高原） |
| PlateauDayTradeMinPct | 35 | 當沖佔比 > 35%（高原） |
| PlateauTAIEXDeviationPct | 2.0 | 指數偏離 20 日線 < ±2%（高原） |
| ConsolidationBuyDaysMin | 3 | 近 10 日買超天數 > 3（盤整） |
| ConsolidationSellDaysMin | 3 | 近 10 日賣超天數 > 3（盤整） |
| ConsolidationTWDBandPct | 0.5 | 新台幣偏離月線 < ±0.5%（盤整） |
| ConsolidationVolRatioMin | 0.7 | 成交量 / 20 日均量 > 0.7（盤整） |
| ConsolidationVolRatioMax | 1.0 | 成交量 / 20 日均量 < 1.0（盤整） |

### MarketPeriod 常數對照（Go constant → 時期，drift-check 用，2026-08-29 新增）

`internal/domain/shared/shared.go` `MarketPeriod` 七階段常數與本憲章時期名稱的對照。

| Go 常數 | 值 | 中文 | 憲章小節 |
|---|---|---|---|
| PeriodDownturn | downturn | 低迷 | §3.1 |
| PeriodTurnaroundUp | turnaround_up | 轉折開高 | §3.2 |
| PeriodBull | bull | 上升／多頭 | §3.3 |
| PeriodPlateau | plateau | 高原 | §3.4 |
| PeriodConsolidation | consolidation | 盤整 | §3.5 |
| PeriodTurnaroundDown | turnaround_down | 轉折下壓 | §3.6 |
| PeriodBlackSwan | black_swan | 黑天鵝 | §3.7 |

### 台海緊張 4 級制與地緣強度來源（v1.1，2026-08-25 新增）

`GeoIntensity`（0-100）由 `TaiwanRSSGeopoliticalProvider`（4 財經 RSS 關鍵字計數）產出，
並同時作為台灣壓力指數的 geopolitical 元件（scale=1.0、weight=0.13）輸入。

| 級別 | 名稱 | GeoIntensity | 對應判別 |
|------|------|--------------|---------|
| 1 | 平靜 | 0-25 | 無 |
| 2 | 升溫 | 26-50 | 轉折下壓候選（≥ 40） |
| 3 | 高張 | 51-75 | 黑天鵝候選（≥ 60） |
| 4 | 危機 | 76-100 | 黑天鵝（≥ 60 已涵蓋；≥ 76 視為危機級） |

> 註（2026-08-25 更新）：5 日趨勢已實作——轉折下壓地緣條件需 GeoIntensity ≥ 40 且
> GeoIntensityChange5D ≥ 0（5 日變動；0 = 無歷史資料 → 僅以當日強度判定）。
> **元件換算**：壓力指數 geopolitical 元件 = GeoIntensity × 1.0 × 0.13（scale × weight）；
> 反向換算 GeoIntensity = 元件值 ÷ 0.13（consumers 勿把元件值當獨立「台海緊張」刻度）。

### §3 與 §5 的角色對位（v1.1，2026-08-25）

- **§3 七時期判別條件 = 權威判定**：machine-executable，`PeriodDetector.DetectAssessment()`
  實作，agent／外部消費者引用判別一律以 §3 為準。
- **§5 六個核心觀測指標 = 寬鬆觀測框架**：投資人跟隨座標與敘事輔助，**非判別條件**。
  例：§5 #1「美台資金開關（US10Y、DXY、USD/JPY、台灣壓力指數）」是資金總開關的觀測，
  不直接改變 current_period；§5 #6「事件觸發（ETF/MSCI/除權息）」同理。
- 延伸指標（atlas-wiki／hermes 擴充）不得覆寫 §3 判別；若需新增判別條件，先修憲章 §3。

---

## 四、七大資金勢力在各時期的典型行為

| 時期 | 外資 | 投信 | 自營商 | 公股／政府 | 壽險／銀行 | 公司派／主力 | 散戶 |
|------|------|------|--------|-----------|-----------|------------|------|
| **低迷** | 賣超趨緩，被動資金開始回補 | 被動買超（ETF 申購），緩慢累積 | 觀望，短線少量進出 | **積極買超護盤**，連買數量增加 | 逢低分批布局長線部位 | 低調回補庫藏股 | 恐慌已過但不敢進場，融資餘額低 |
| **轉折開高** | **突然轉買**，領先信號；期貨多單快速增加 | 跟進買超，但幅度小於外資 | 開始積極短多操作，當沖增加 | 減少護盤力道（讓市場自主） | 跟進加碼但節奏較慢 | 開始釋放利多消息 | 觀望中，少數跟進 |
| **上升** | **連續買超**，主力推力 | 持續買超，ETF 規模擴張加速買盤 | 短多積極，當沖活躍 | 退場觀望，逢高調節 | 穩定加碼 | 順勢拉抬，發布樂觀展望 | 開始追價，融資溫和增加 |
| **高原** | 買盤減弱，部分獲利了結 | 買盤持續但幅度減小 | 轉為短空或觀望，當沖佔比升高 | 逢高積極調節 | 停止加碼，鎖定利潤 | 部分主力出貨，產業消息開始分歧 | 追價意願仍強，融資高水位 |
| **盤整** | 忽買忽賣，無方向性 | 持續被動配置（ETF），少主動操作 | 短線快速進出，類股輪動 | 低調觀望 | 維持現有部位 | 個股操作為主，無一致方向 | 短進短出，當沖增加 |
| **轉折下壓** | **連續大賣**，領先信號；期貨轉空 | 被動贖回壓力，開始減碼 | 轉空積極，放空或避險 | 開始進場護盤，但力道有限 | 減碼高風險部位，轉向防禦 | 沉默或發布悲觀展望，停止買回 | 融資斷頭潮，恐慌性拋售 |
| **黑天鵝** | 無差別拋售，流動性枯竭 | 被迫贖回賣壓，無法挑選標的 | 全面撤退，自營部位大幅減持 | **國安基金進場**，全力護盤 | 全面減持，轉向現金與公債 | 停止所有操作 | 全面恐慌，融資斷頭 → 散戶退場 |
- **自營商**需區分大小：大型自營商（元大、凱基、群益等）有研究團隊，操作具邏輯性，可納入宏觀預測框架；小型自營商以投機炒作為主，行為難以從宏觀角度預測，應透過 AI 分點數據捕捉其足跡，不納入時期判斷的宏觀指標。
- **投信**需區分主動與被動：ETF 被動買盤（如 0050、0056、00878 等）規模龐大且具強制性（申購即買、贖回即賣），是結構性支撐力量；主動基金的操作則反映經理人對市場的判斷，是行為信號。兩者對市場的影響機制不同，不可混為一談。

**行為規律**：
- **外資**是方向制定者，行動最領先；散戶是最落後的跟隨者。
- **公股**是逆周期調節者：低迷／黑天鵝時買，上升時賣。
- **投信**是被動資金代表：ETF 規模變化比投信買賣超數字更有預測力。
- **公司派**是資訊優勢方：內部人申報轉讓是重要信號。
- **散戶情緒**是反向指標：融資高點 ≈ 市場頭部；融資斷頭潮 ≈ 市場底部。

---

## 五、散戶跟隨座標與策略映射

### 六個核心觀測指標（優先級由高到低）

| # | 觀測指標 | 對應數據 | 判讀邏輯 |
|---|---------|---------|---------|
| 1 | **美台資金開關** | US10Y、DXY、USD/JPY、台灣壓力指數 | 資金總開關決定能否進場；壓力指數 > 60 不進場 |
| 2 | **美股科技動能** | SOX、TSM ADR、NVDA、NASDAQ-100 | 科技股方向決定台股方向；SOX 在 50 日線下不做多 |
| 3 | **外資期現貨** | 外資買賣超、期貨未平倉、新台幣匯率 | 外資是方向制定者；連續買超+期貨多單=做多信號 |
| 4 | **內資抗衡** | 投信買賣超、公股買賣超、公司派動向 | 內資共識可對抗外資賣壓；投信+公股同步買超=底部信號 |
| 5 | **散戶情緒** | 融資餘額、融資維持率、當沖佔比 | 反向指標；融資暴增+當沖過熱=頭部警訊 |
| 6 | **事件觸發** | ETF 調整日、MSCI 生效日、營收公告、除權息 | 預測性事件創造可捕捉的短期錯價 |

### 策略矩陣：不同時期下的散戶優先策略

| 時期 | 策略優先級 1 | 策略優先級 2 | 策略優先級 3 | 現金部位建議 |
|------|------------|------------|------------|------------|
| **低迷** | 防禦（all_weather） | 價值（value） | — | 40-50% |
| **轉折開高** | 成長（growth） | 動能（momentum） | 事件套利 | 10-20% |
| **上升** | 動能（momentum） | 成長（growth） | 事件套利 | 5-10% |
| **高原** | 事件套利 | 價值（value） | 防禦（all_weather） | 20-30% |
| **盤整** | 事件套利 | 防禦（all_weather） | — | 30-40% |
| **轉折下壓** | 防禦（all_weather） | — | — | 50-70% |
| **黑天鵝** | 防禦（all_weather） | 現金 | — | 80-100% |

### 三類核心策略說明

#### A. 跟隨聰明錢啟動（Follow Smart Money）
- **適用時期**：轉折開高、上升
- **邏輯**：外資連續買超 + 新台幣升值 + SOX 突破關鍵均線 → 跟隨外資流入的類股和個股
- **程式碼映射**：`domain.RegimeRiskOn` → `RegimeAllocator` 配置 Growth 40% + Momentum 30%
- **風控**：外資轉賣即減碼；新台幣貶破月線即出場

#### B. 事件套利（Event Arbitrage）
- **適用時期**：高原、盤整、轉折開高（輔助）
- **邏輯**：利用 ETF 調整、MSCI 變更、營收公告等可預測事件，在事件前布局，事件後獲利了結
- **程式碼映射**：`internal/eventdriven` 5 日 forward 預測 + `internal/capitalflow` 品質分數
- **風控**：持倉 < 5 日；單一事件曝險 < 10%

#### C. 資金對抗後爆發（Contrarian after Capitulation）
- **適用時期**：低迷（布局）、轉折開高（收割）
- **邏輯**：在低迷期末端，當外資賣壓衰竭 + 公股持續買超 + 融資大減時，逐步布局；等待轉折信號出現後加碼
- **程式碼映射**：`domain.RegimeRiskOff` → `RegimeAllocator` 配置 Value 40% + Quality 35%
- **風控**：分批進場；轉折信號未出現前倉位 < 30%

### 策略與 macroflow 風險層級的動態調整

`internal/macroflow/` 的 `RiskLevel` → `AdjustmentResult` 提供自動化的因子權重調整：

| 時期 | 對應 RiskLevel | VIX Stress | 防禦調整 | 攻擊調整 | 現金調整 |
|------|---------------|-----------|---------|---------|---------|
| 上升、轉折開高、高原、盤整 | `RiskYellow` | No | +5% | 0% | 0% |
| 上升、高原（高 VIX） | `RiskYellow` | Yes | +20% | -15% | 0% |
| 低迷、轉折下壓 | `RiskOrange` | No | +15% | -10% | 0% |
| 轉折下壓（高 VIX） | `RiskOrange` | Yes | +20% | -20% | +5% |
| 黑天鵝 | `RiskRed` | No | +20% | -25% | +10% |
| 黑天鵝（極端 VIX） | `RiskRed` | Yes | +25% | -30% | +15% |

---

## 六、與既有程式碼的整合路徑

本憲章為方法論層，不要求一次性重構所有程式碼。以下是建議的漸進整合路徑：

### Phase 1：文檔先行（本 PR）
- [x] 建立 `docs/ATLAS_METHODOLOGY.md`（本文件）
- [x] 建立 `configs/methodology_rules.yaml`（時期→策略映射配置）

### Phase 2：時期判斷器（後續 PR）
- 在 `internal/portfolio/regime.go` 中新增 `RegimeDetector.DetectDetailed()` 方法
- 從現有的三態（RISK_ON/OFF/NEUTRAL）擴展為七時期 + 自動映射到三態（向下相容）
- 現有程式碼不受影響：`RegimeInferenceStrategy.InferRegime()` 仍回傳三態

### Phase 3：策略自動選擇（後續 PR）
- `internal/orchestrator/executor_pipeline.go` 中新增 `MethodologyAdvisor`
- 根據當前時期自動調整 `ExecutionPolicy` 中的因子權重
- 前端展示「當前市場時期」卡片

### Phase 4：回測驗證（後續 PR）
- 歷史 replay 數據回測七時期分類的準確性
- 驗證策略矩陣在歷史數據上的表現
- 必要時調校時期判斷閾值

---

## 附錄 A：台灣壓力指數閾值參考

| 壓力指數 | Regime 標籤 | 建議行動 |
|---------|-----------|---------|
| 0-30 | low | 正常配置 |
| 31-50 | alert | 降低攻擊部位 10-20% |
| 51-70 | high | 轉為防禦配置，現金 > 30% |
| 71-100 | crisis | 現金 > 50%，暫停新進場 |

## 附錄 B：敘事事件觸發器與時期的關係

`internal/narrative/detector_impls.go` 中的 24 個 detector 在不同時期有不同的敏感度：

- **US_rates_up**：所有時期都重要，但在高原和轉折下壓時權重加倍
- **JPY_carry_unwind**：上升期和黑天鵝期最關鍵（carry trade 反轉）
- **tariff_shock**：任何時期都可能觸發，直接跳轉至轉折下壓或黑天鵝
- **AI_capex_surge**：上升期和多頭期權重加倍
- **earnings_blackout**：高原期和盤整期最相關

## 附錄 C：詞彙對照表

| 中文 | 英文（程式碼） | 說明 |
|------|-------------|------|
| 市場時期 | Regime | 七時期之一 |
| 風險狀態 | Regime（domain） | RISK_ON / RISK_OFF / NEUTRAL |
| 風險層級 | RiskLevel（macroflow） | yellow / orange / red |
| 壓力指數 | StressIndex / TaiwanStressIndex | 0-100 分 |
| 資金開關 | Capital Flow Gate | DXY + US10Y + USD/JPY |
| 聰明錢 | Smart Money | 外資 + 投信 + 公司派 |
| 散戶情緒 | Retail Sentiment | 融資餘額 + 當沖佔比 + Google Trends |
| 防禦策略 | Defensive / All-Weather | 低 beta、高股息、低波動 |
| 攻擊策略 | Aggressive / Growth / Momentum | 高 beta、高成長、高動能 |
| 事件套利 | Event Arbitrage | ETF 調整、MSCI、營收公告 |

---

> **最後更新**: 2026-07-27
> **維護責任**: 任何修改策略邏輯或新增市場指標的 PR，必須檢查本文件是否需要同步更新。

## 附錄 D：憲章審計追蹤表

> **實施總表**: `.omo/manifests/manifest-constitution-implementation.md`
> **差距審計**: `.omo/manifests/manifest-constitution-gap-audit.md`
> **審計報告**: `docs/ATLAS_CONSTITUTION_AUDIT.md`（2026-07-27）
> **更新規則**: 每次修復一個審計項目後，將狀態從 ⬜ 改為 ✅，並標註 PR 編號。

### P0 — 會產生錯誤信號

| # | 項目 | 狀態 | PR |
|---|------|------|-----|
| A1 | 七時期判斷邏輯（DetectPeriod） | ✅ | #1372 |
| A2 | 七時期→三態向下相容映射 | ✅ | #1372 |
| A3 | 三套 regime 系統統一 | ✅ | #1372 |
| B1 | 管線順序重排（MacroFlow 移到推薦之前） | ✅ | #1372 |
| B2 | 每層輸出強制影響下一層 | ✅ | #1381 |
| B4 | VIX key mismatch 修復 + macro evidence 注入 | ✅ | #1372 |
| C1 | 壽險/銀行 + 公司派/內部人 數據源 | ✅ | #1372 |
| C2 | 公股行庫自動化數據通道 | ✅ | #1372 |
| C3 | orchestrator PrimaryFlow 改用 capitalflow | ✅ | #1372 |
| D1 | detector 時期敏感度（5 個 × 7 時期） | ✅ | #1372 |
| D2 | YAML consumer：MethodologyAdvisor | ✅ | #1372 |
| D3 | 推薦引擎按時期過濾策略（GetApplicableStrategies） | ✅ | #1372 |
| D5 | RegimeAllocator 擴展為六策略×七時期 | ✅ | #1372 |
| E1 | YAML config loader（JSON→YAML 擴展） | ✅ | #1372 |
| E2 | 七時期閾值參數化（進入 ParametersConfig） | ✅ | #1372 |
| E3 | API 輸出時期結構化欄位 | ⚠️ | partial — struct exists, API builder not wired |

### P1 — 信號不完整

| # | 項目 | 狀態 | PR |
|---|------|------|-----|
| A4 | macroflow RiskLevel 自動推導（多指標複合） | ✅ | #1372 |
| B3 | MacroDataSnapshot 補漏指標（8 項） | ✅ | #1372 |
| B5 | Causal chain tracing（layer-0...layer-7 ID） | ✅ | #1372 |
| C4 | capitalflow 4-layer Assessment 消費鏈 | ✅ | #1378 |
| C6 | 散戶反向指標統一口徑（RSI-Tw vs capitalflow） | ✅ | #1372 |
| D4 | Narrative 19/24 themes 進入 regime inference | ✅ | #1372 |
| E4 | 前端七時期 UI 卡片 | ✅ | TBD |
| E5 | 策略類別三分類（defensive/aggressive/tactical） | ⬜ | — |

### P2 — 工程品質

| # | 項目 | 狀態 | PR |
|---|------|------|-----|
| C5-p2 | cfScore 常數權重→動態權重 | ✅ | #1372 |

### 已對齊（無需修改）

| # | 項目 |
|---|------|
| ✅ | macroflow/rules.go 6 組規則數值 |
| ✅ | QualityScore 公式 = F+Inst–Retail |
| ✅ | capitalflow 七大 ForceName 定義 |
| ✅ | 部分 stress/VIX 參數可配置（ParametersConfig） |

### DeepSeek 方法論覆核新增（2026-07-27）

| # | 項目 | 狀態 | PR |
|---|------|------|-----|
| F1 | 外資雙重動機模型（結構性 vs 投機性分流） | ⬜ | — |
| F2 | 自營商大小分流（大型可納宏觀，小型用 AI 分點） | ⬜ | — |
| F3 | 投信主動 vs 被動分流（ETF 被動買盤 vs 主動基金） | ⬜ | — |
| F4 | 公股分點追蹤作為 BK-13 替代方案 | ⬜ | — |
| F5 | 選股層策略庫設計（Phase 4，憲章目前僅組合層） | ⬜ | — |

### MCP 工具對齊（2026-07-27）

| # | 項目 | 狀態 | PR |
|---|------|------|-----|
| M1 | 時期判斷 MCP 工具公開 | ⬜ | — |
| M2 | 資金流品質分數 MCP 工具公開 | ⬜ | — |
| M3 | 因果鏈 tracing MCP 工具公開 | ⬜ | — |
| M4 | 策略適用時期 MCP 工具公開 | ⬜ | — |
| M5 | 壓力指數元件 MCP 工具公開 | ⬜ | — |
| M6 | 審計狀態 MCP 工具公開 | ⬜ | — |

### 憲章強制執行機制（2026-07-27）

| # | 項目 | 狀態 | PR |
|---|------|------|-----|
| X1 | PR 合併前憲章對齊檢查（CI gate） | ⬜ | — |
| X2 | 方法論變更強制更新追蹤表 | ⬜ | — |
| X3 | 憲章漂移自動警報（nightly scan） | ⬜ | — |
