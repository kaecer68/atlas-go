# ATLAS 憲章審計報告 v1.0

> **產出日期**: 2026-07-27
> **審計源**: `docs/ATLAS_METHODOLOGY.md` v1.0
> **審計範圍**: `internal/` 全部模組 + 前端 + 配置
> **審計方法**: 五個平行 scout agent 各審一個領域，主 agent 綜合
> **關聯 PR**: #1371（`internal/live/` CircuitBreakerOps interface 提取 — 不影響方法論）

---

## 總覽

| 指標 | 數值 |
|------|------|
| 審計項目 | 20 個（覆蓋憲章六個章節） |
| P0 (會產生錯誤信號) | **13 項** |
| P1 (信號不完整) | **6 項** |
| P2 (工程品質) | **1 項** |
| 已對齊 | macroflow rules 數值、QualityScore 公式、capitalflow 七大 ForceName、部分 stress/VIX 參數可配置 |

### 核心發現：憲章是目前「孤兒文件」

- `configs/methodology_rules.yaml`（503 行 YAML）— **零個 Go consumer**
- `docs/ATLAS_METHODOLOGY.md` Phase 2/3/4 規劃 — **全部未啟動**
- 七時期判斷 — **不存在**（僅有三態 RISK_ON/OFF/NEUTRAL）
- 憲章因果傳導鏈 — **管線順序與憲章層級相反**
- 資本流向權威 — **orchestrator 繞過 capitalflow 模組**

---

## 審計結果明細

### A. 時期判斷系統

| # | 憲章需求 | 現狀 | 差距 | 優先級 |
|---|---------|------|------|--------|
| A1 | 七時期判斷（低迷/轉折開高/上升/高原/盤整/轉折下壓/黑天鵝） | `domain.Regime` 僅三態（RISK_ON/OFF/NEUTRAL）；`RegimeDetector.Detect()` 僅用 RSI+VIX+SPXTrend 三個指標 | 缺少七時期判斷邏輯、缺少憲章定義的 30+ 指標（外資連續買超、融資減少%、公股連買天數、新台幣波動、SOX 均線、當沖佔比等） | **P0** |
| A2 | 七時期 → 三態向下相容 | 無 | `RegimeDetector.DetectDetailed()` 不存在；憲章 Phase 2 未啟動 | **P0** |
| A3 | 多套 regime 系統統一 | 存在三套獨立分類：`domain.Regime`(3 態)、`realtime.RegimeType`(7 種微觀)、`sim.RegimeType`(4 種閾值) | 三套系統無映射關係，各自獨立運作 | **P0** |
| A4 | macroflow 風險層級自動推導 | `Engine.Compute()` 需要 caller 預先提供 RiskLevel；`regimeToRiskLevel()` 僅做 RISK_OFF→red、其他→yellow | 缺少從七時期指標自動推導 RiskLevel 的邏輯層；黑天鵝判定過於粗略（僅 VIX≥35，無外資/融資/匯率/國安基金等複合條件） | **P1** |
| A5 | macroflow 權重調整數值 | `macroflow/rules.go` 的 6 組規則（yellow/orange/red × calm/stress）與憲章第五節表格**完全一致** | ✅ 已對齊 | — |

### B. 因果傳導鏈

| # | 憲章需求 | 現狀 | 差距 | 優先級 |
|---|---------|------|------|--------|
| B1 | 8 層因果鏈依序執行（第〇層→第一層→...→第七層） | `ExecuteWithContext()` 管線順序為：regime → collection → momentum → weights → **macroflow** → control | MacroFlow 位於推薦與權重之後，與「由上而下、由外而內」相反；且所有 production 呼叫都未注入 MacroFlow/MacroDataSnapshot → macroflow 實際不執行 | **P0** |
| B2 | 每層輸出強制影響下一層輸入 | Regime 的四種 evidence（Macro/Technical/Narrative/AgentSignal）是平行加權平均，非逐層傳導；Sector/Style/Superinvestor 在同一迴圈各自呼叫，前層輸出不成為後層輸入 | 無層級依賴模型；憲章「不可反向推導」「信號必須標註位置」無法由程式保證 | **P0** |
| B3 | MacroDataSnapshot 覆蓋憲章 32 個明列指標 | Snapshot 約 14/32 精確覆蓋、2/32 代理覆蓋；缺 Fed 預期、半導體設備進口、集中市場成交量、當沖佔比、壽險/銀行、公司派/內部人、散戶買賣超/融資維持率/Google Trends、第七層事件鏈接 | Schema 基礎不差但缺漏影響完整性；replay 的 `QuotesToMacroDataSnapshot()` 只轉換 7 項且不傳入 ExecuteWithContext | **P1** |
| B4 | Macro data 進入 regime inference | `DefaultRegimeInferenceStrategy` 不接收 MacroDataSnapshot；Macro evidence 只查 `$VIX`（但 provider 用 `^VIX` → key 不一致 → 此證據大多退化為 0）；US10Y/DXY/SOX/TSM ADR/NVDA 等快照欄位完全不進 regime | VIX key mismatch 使現有 macro evidence 幾乎失效；其他憲章指標全數缺席 | **P0** |
| B5 | Causal chain tracing（每個推薦可追溯到憲章層級） | `/api/narrative/chains` 僅是敘事模板配對；`/api/dashboard/reasoning-trace` 僅有 orchestrator phase trace | 無 layer-0...layer-7 ID、每層輸入快照、輸出結論、parent reference | **P1** |

### C. 資金流向與勢力分析

| # | 憲章需求 | 現狀 | 差距 | 優先級 |
|---|---------|------|------|--------|
| C1 | 七大勢力完整數據 | 5/7 勢力有數據源（外資/投信/自營商/公股(人工)/散戶(僅融資)）；2/7 完全缺失 | **壽險/銀行**勢力無 provider/adapter/dimension；**公司派/內部人**勢力無任何資料源；散戶缺融資維持率/當沖佔比/券商分行/Google Trends | **P0** |
| C2 | 公股行庫自動化數據 | `GovernmentFlowProvider` 僅 file-driven seam（手動 JSON dump）；`adapter_government_flow.go` 掛 `rate.Inf` 等上游 | BK-13 自建證交所分點加總通道尚未實作 | **P0** |
| C3 | capitalflow 輸出進入主決策鏈 | **斷點**：`orchestrator.strategy_evolver.go:360` 讀的是 `plan.PrimaryFlow`（來自 `narrative.MacroRiskAssessmentEngine`），不是 `capitalflow.CapitalFlowAssessment.PrimaryFlow`；同名異物 | orchestrator 繞過 capitalflow → 憲章要求的資本流向權威在最高層決策缺位 | **P0** |
| C4 | capitalflow 4-layer Assessment 有消費者 | `DominantActor`/`DominantSignal`/`Resonance` 等 E07 設計的權威欄位**無消費者**；`eventdriven.Predictor` 僅讀 QualityScore ×0.3 | E07 4-layer assessment 是 dead letter | **P1** |
| C5 | QualityScore 公式 | `Foreign(Z) + Institutional(Z) – Retail(Z)` → 完美對應憲章原則 #3（外資+投信=聰明錢正向，散戶=反向） | ✅ 已對齊 | — |
| C6 | 散戶反向指標進入 portfolio 因子 | `portfolio.factor_engine_institutional` 使用自家 RSI-Tw 而非 `capitalflow.ForceRetail` | 兩個獨立計算散戶反向，口徑漂移風險；`scoreRetail` 用 ChangePct 而非水位百分位 | **P1** |

### D. 敘事引擎與策略映射

| # | 憲章需求 | 現狀 | 差距 | 優先級 |
|---|---------|------|------|--------|
| D1 | 24 個 detector 按時期調整敏感度 | `Detector` interface 無 Period/Regime 欄位；`Detect()` 只收 MarketData+MacroSnapshot | 憲章附錄 B 要求 5 個 detector 的差異化敏感度（US_rates_up 高原加倍、JPY_carry_unwind 上升/黑天鵝關鍵等）完全未實作 | **P0** |
| D2 | 時期→策略自動選擇 | `configs/methodology_rules.yaml` 定義了完整的 `regimes[*].strategies` + `strategies[*].applicable_regimes` | **零個 Go consumer**；憲章 Phase 3 `MethodologyAdvisor` 未啟動 | **P0** |
| D3 | 推薦引擎按時期過濾策略 | `buildPremiumStrategy()` 直接回傳 `RankedStrategies()` 全部結果；TierRegistered 硬寫死 `["all_weather", "defensive"]` | 無 `GetApplicableStrategies(regime)` 介面；RISK_OFF 時期仍可能推薦 growth/momentum | **P0** |
| D4 | Narrative events 進入 regime inference | `NarrativeEvidenceSource` 僅用 5/24 themes（US_rates_up/geopolitical/oil/JPY/AI）做 ±0.5 加成 | 19 個 detector 被靜默忽略；權重不隨時期調整 | **P1** |
| D5 | RegimeAllocator 風格權重與憲章對齊 | 僅有三態配置（Growth/Value/Momentum/Quality）；無憲章定義的 6 策略類別（all_weather/value/growth/momentum/event_arbitrage/cash_only） | 命名空間錯位；憲章第五節完整的 7×6 策略矩陣無任何執行點 | **P0** |

### E. 前端與配置

| # | 憲章需求 | 現狀 | 差距 | 優先級 |
|---|---------|------|------|--------|
| E1 | config YAML 被程式載入 | `internal/config/parameters_load.go` 僅支援 JSON（`json.Unmarshal`） | 無 YAML parser/loader；`methodology_rules.yaml` 無法被現有機制讀取 | **P0** |
| E2 | 七時期參數可配置 | VIX/stress weights 已在 ParametersConfig 中可配置；但 YAML 中的外資買賣超金額、連續天數、融資、當沖、SOX、TSM ADR、期貨未平倉等七時期閾值無對應參數欄位 | 憲章時期條件無法透過參數系統調校 | **P0** |
| E3 | API 輸出時期資訊 | `/api/dashboard/daily-summary` 僅由事件情緒推導三態 NEUTRAL/RISK_ON/OFF；`/api/reports/latest` 無結構化時期欄位 | 缺 period id/name、觸發指標、cash reserve、allowed strategies | **P0** |
| E4 | 前端展示七時期 UI | 前端有三態 regime label、stress index、regime history、regimeColor；但無七時期專用元件/轉換矩陣/指標明細/策略映射 | 憲章 Phase 3「當前市場時期卡片」未實作 | **P1** |
| E5 | Tier gating 與策略類別一致 | `home-tier-sections.js` 依 free/registered/premium 做內容 gating；未按憲章 defensive/aggressive/tactical 分類或 YAML primary/secondary 過濾 | 策略命名無 category mapping | **P1** |

---

## 優先級排序（建議執行順序）

### 第一批：P0 — 會產生錯誤信號（建議本週啟動）

| 序 | 項目 | 影響 | 估計工時 |
|----|------|------|---------|
| 1 | **A1+A2: 七時期判斷 + 向下相容** | 所有下游決策的基礎。無七時期判斷 = 憲章整套方法論無執行點 | 3-5d |
| 2 | **D2+D3: YAML consumer + 策略過濾** | 推薦引擎目前在 RISK_OFF 仍推薦 growth/momentum → 散戶跟單可能賠錢 | 2-3d |
| 3 | **C3: orchestrator PrimaryFlow 修復** | 最高層策略決策繞過 capitalflow → 憲章核心原則 #3「跟隨聰明錢」無效 | 1-2d |
| 4 | **B1+B4: 管線重排 + VIX key 修復** | 執行順序與憲章相反；macro evidence 因 key mismatch 幾乎失效 | 2-3d |
| 5 | **E1+E2: YAML loader + 參數可配置** | 憲章權威配置無法被程式讀取；時期閾值無法調校 | 1-2d |
| 6 | **D1: detector 時期敏感度** | 憲章附錄 B 要求 5 個 detector 差異化，目前一視同仁 | 1d |
| 7 | **C1+C2: 壽險/公司派/散戶數據缺口** | 七大勢力缺二，影響「內資抗衡」判斷 | 3-5d（取決於數據源可行性） |
| 8 | **D5: RegimeAllocator 擴展為六策略** | 三態配置 → 七時期×六策略矩陣 | 2-3d |

### 第二批：P1 — 信號不完整（本月）

| 序 | 項目 |
|----|------|
| 9 | **C4: capitalflow 4-layer Assessment 消費鏈** |
| 10 | **C6: 散戶反向指標統一口徑** |
| 11 | **B3: MacroDataSnapshot 補漏指標** |
| 12 | **B5: Causal chain tracing** |
| 13 | **D4: Narrative 19/24 themes 進入 regime inference** |
| 14 | **A4: macroflow RiskLevel 自動推導** |

### 第三批：P1 — 前端（本月）

| 序 | 項目 |
|----|------|
| 15 | **E3: API 輸出時期結構化欄位** |
| 16 | **E4: 前端七時期 UI 卡片** |
| 17 | **E5: 策略類別三分類（defensive/aggressive/tactical）** |

### 第四批：P2 — 工程品質

| 序 | 項目 |
|----|------|
| 18 | **C5 (p2): cfScore 常數權重 → 動態權重（跟隨憲章第四節「外資權威」）** |

---

## PR #1371 影響評估

`refactor(live): C10 extract CircuitBreakerOps interface` — 純 `internal/live/` 內部 interface 提取，**不影響任何憲章審計項目**。live trading 的 circuit breaker 與方法論時期判斷是正交關注點。

---

## 下一步建議

1. **優先執行 P0-1（七時期判斷）** — 這是所有其他工作的前提
2. **P0-2~P0-8 可以部分平行** — 但 P0-1 的 `DetectDetailed()` interface 需要先定義，其他模組才能消費
3. **建議建立 `docs/ATLAS_METHODOLOGY.md#附錄 D：審計追蹤表`** — 將本報告的 18 個項目納入憲章文件，每次修復後更新狀態

---

> **審計工具**: 5 個平行 CodeGraph/Codebase Memory scout agent
> **審計覆蓋**: 40+ 檔案，包含所有 `internal/` 模組、前端、配置
