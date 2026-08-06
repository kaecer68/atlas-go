# ATLAS 憲章審計報告 v1.1

> **產出日期 (v1.0)**: 2026-07-27
> **本版 (v1.1) 更新**: 2026-08-07
> **對應 HEAD**: `0f8667a6` (`docs(investigation): append §8 external-verified affirmation of TPEX scope` #1478)
> **審計源**: `docs/ATLAS_METHODOLOGY.md` v1.0
> **審計範圍**: `internal/` 全部模組 + 前端 + 配置
> **審計方法**: 5 個平行 scout agent 各審一個領域，主 agent 綜合；v1.1 為 7/27 → 8/7 期間追加 patch audit
> **關聯 PR (主要)**: #1372（憲章對齊 26 task）、#1381（causal layer dependency）、#1422（B5-3 PR-B 填充）、#1424（govflow cadence）、#1426（Hermes period 接源）、#1429（Docker 禁令）、#1437（govflow daily-once guard）、#1440（stress_test_daily latestDrawdown）、#1446/#1448/#1451（FinMind quota 系列）、#1464（ACI hook）、#1467（L2.4 cleanup）、#1471（prism autoBalancer BTM 遷移）、#1474（FinMind/Quota traps 索引）

> **v1.1 變更**：原 §附錄 D（審計追蹤表）保留不變；新增 §附錄 E（v1.0 → v1.1 期間 patch audit）與 §附錄 F（F1–F5 / M1–M6 / X1–X3 追蹤表更新）。多個 F / M / X 項目在 7/27 → 8/7 之間由 ⬜ 轉進 ✅ / ⚠️ partial。

---

## 總覽（v1.0 原始）

| 指標 | 數值 |
|------|------|
| 審計項目 | 20 個（覆蓋憲章六個章節） |
| P0 (會產生錯誤信號) | **13 項** |
| P1 (信號不完整) | **6 項** |
| P2 (工程品質) | **1 項** |
| 已對齊 | macroflow rules 數值、QualityScore 公式、capitalflow 七大 ForceName、部分 stress/VIX 參數可配置 |

### 核心發現（v1.0 原始）：憲章是「孤兒文件」

- `configs/methodology_rules.yaml`（503 行 YAML）— **零個 Go consumer**
- `docs/ATLAS_METHODOLOGY.md` Phase 2/3/4 規劃 — **全部未啟動**
- 七時期判斷 — **不存在**（僅有三態 RISK_ON/OFF/NEUTRAL）
- 憲章因果傳導鏈 — **管線順序與憲章層級相反**
- 資本流向權威 — **orchestrator 繞過 capitalflow 模組**

> **v1.1 註**：上述核心發現已在 #1372/#1381 解決（見 §附錄 D）。本審計焦點改為「憲章方法論對齊後，系統治理 / MCP 對外 / 自動化檢查三條線的進展與剩餘風險」（§附錄 E、F）。

---

## 審計結果明細（v1.0 原文，保留供 audit trail 對照）

### A. 時期判斷系統

| # | 憲章需求 | 現狀 | 差距 | 優先級 |
|---|---------|------|------|--------|
| A1 | 七時期判斷（低迷/轉折開高/上升/高原/盤整/轉折下壓/黑天鵝） | v1.0：`domain.Regime` 僅三態；`RegimeDetector.Detect()` 僅用 RSI+VIX+SPXTrend 三個指標。**v1.1**：已被 #1372 / #1413（B2 信心度與觸發指標）取代為 `PeriodDetector.DetectPeriod()` 與 `DetectAssessment()`。 | 缺七時期判斷 | ✅ **已修復** (P0 → 完成) |
| A2 | 七時期 → 三態向下相容 | v1.1：`PeriodToRegime()` 映射函數；`market_period` 與 `regime` 兩欄位同源共存。 | — | ✅ **已修復** (P0 → 完成) |
| A3 | 多套 regime 系統統一 | v1.1：三套都映射到 `PeriodToRegime`。 | — | ✅ **已修復** (P0 → 完成) |
| A4 | macroflow 風險層級自動推導 | v1.1：`Engine.Compute()` 改由七時期 + 壓力指標複合推導 RiskLevel。 | — | ✅ **已修復** (P1 → 完成) |
| A5 | macroflow 權重調整數值 | `macroflow/rules.go` 6 組規則與憲章表格一致 | ✅ 已對齊 | — |

### B. 因果傳導鏈

| # | 憲章需求 | v1.1 現狀 | 優先級 |
|---|---------|----------|--------|
| B1 | 8 層因果鏈依序執行 | v1.1：`ExecuteWithContext()` 已將 MacroFlow 移至推薦/權重之前，順序符合由上而下由外而內 | ✅ 完成 (#1372) |
| B2 | 每層輸出強制影響下一層 | v1.1：layer-0..7 parent reference + struct 強制依賴 (#1381) | ✅ 完成 (#1381) |
| B3 | MacroDataSnapshot 補漏 | v1.1：已補 Fed 預期、半導體設備進口、集中市場成交量、當沖佔比、壽險/銀行、公司派/內部人、融資維持率、事件鏈 | ✅ 完成 (#1372) |
| B4 | macro data 進入 regime | v1.1：VIX key 修正（`^VIX` ↔ `$VIX`），US10Y/DXY/SOX/TSM ADR/NVDA 全部注入 | ✅ 完成 (#1372) |
| B5 | Causal chain tracing | v1.1：layer-0..7 ID、輸入快照、輸出結論、parent reference；對外 MCP `trace_get_*` 系列公開 | ✅ 完成 (#1372 + L1-L5 MCP 公開) |

> **v1.1 註**：B5 的 MCP 公開在 F1-F5 / M1-M6 段重複計入 M3（見 §附錄 F）。

### C. 資金流向與勢力分析

| # | 憲章需求 | v1.1 現狀 | 優先級 |
|---|---------|----------|--------|
| C1 | 七大勢力數據源（壽險/公司派/散戶） | v1.1：壽險/銀行、公司派/內部人 provider/adapter/dimension、融資維持率/當沖佔比/Google Trends 已上 | ✅ 完成 (#1372) |
| C2 | 公股行庫自動化通道 | v1.1：BK-13 通道已上（PR #1421 parser 修復），**實際資料日仍受 CAPTCHA 限制**——由 #1424（節律）+ #1437（daily-once guard）配合退避 | ✅ 完成 (#1372 + #1421)；剩 CAPTCHA 解封風險 |
| C3 | orchestrator PrimaryFlow 改用 capitalflow | v1.1：已修 | ✅ 完成 (#1372) |
| C4 | 4-layer Assessment 消費鏈 | v1.1：`DominantActor` / `DominantSignal` / `Resonance` 由 `eventdriven.Predictor` 與權重消費 | ✅ 完成 (#1378) |
| C5 | QualityScore 公式 + 動態權重 | v1.1：cfScore 從常數權重改為外資權威動態權重 | ✅ 完成 (#1372 + P2) |
| C6 | 散戶反向指標統一口徑 | v1.1：`portfolio.factor_engine_institutional` 與 `capitalflow.ForceRetail` 口徑對齊 | ✅ 完成 (#1372) |

### D. 敘事引擎與策略映射

| # | 憲章需求 | v1.1 現狀 | 優先級 |
|---|---------|----------|--------|
| D1 | 24 detector 按時期敏感度 | v1.1：5 個關鍵 detector 7 時期差異化權重已上 | ✅ 完成 (#1372) |
| D2 | 時期→策略自動選擇 | v1.1：`MethodologyAdvisor` 消費 `configs/methodology_rules.yaml`，YAML consumer 落地 | ✅ 完成 (#1372) |
| D3 | 推薦引擎按時期過濾 | v1.1：`buildPremiumStrategy()` 與 `TierRegistered` 改走 `GetApplicableStrategies(regime)` | ✅ 完成 (#1372) |
| D4 | Narrative 19/24 themes | v1.1：全部 24 themes 進入 `NarrativeEvidenceSource`，權重隨時期調整 | ✅ 完成 (#1372) |
| D5 | RegimeAllocator 六策略×七時期 | v1.1：擴展為 all_weather/value/growth/momentum/event_arbitrage/cash_only 六策略 | ✅ 完成 (#1372) |

### E. 前端與配置

| # | 憲章需求 | v1.1 現狀 | 優先級 |
|---|---------|----------|--------|
| E1 | config YAML loader | v1.1：`internal/config/parameters_load.go` 支援 YAML（methodology_rules.yaml 可讀） | ✅ 完成 (#1372) |
| E2 | 七時期參數可配置 | v1.1：外資買賣超金額、連續天數、融資、當沖、SOX、TSM ADR、期貨未平倉閾值納入 `ParametersConfig` | ✅ 完成 (#1372) |
| E3 | API 輸出時期結構化欄位 | v1.1：`/api/reports/latest`、regime history、recommendations 對外帶 `period` / `period_name_zh` / `cash_reserve` / `allowed_strategies` 等結構化欄位（#1406 已記） | ✅ 完成 (PR #1406 + #1426) |
| E4 | 前端七時期 UI 卡片 | v1.1：E4 (PR #1397, #1398) 完成方法論頁 + 因果傳導鏈頁；首頁 chip + 走勢軸 (PR #1408) | ✅ 完成 (#1397, #1398, #1408) |
| E5 | 策略類別三分類 | v1.1：E5a 三分類（防禦/攻擊/戰術）已上 (PR #1404) | ✅ 完成 (#1404) |

---

## 優先級排序（建議執行順序）

### 第一批：P0 — 會產生錯誤信號（v1.1 全部完成）

| 序 | 項目 | 影響 | 狀態 |
|----|------|------|------|
| 1 | A1+A2: 七時期判斷 + 向下相容 | 全下游決策的基礎 | ✅ (#1372 / #1413) |
| 2 | D2+D3: YAML consumer + 策略過濾 | RISK_OFF 不再誤推薦 | ✅ (#1372) |
| 3 | C3: orchestrator PrimaryFlow 修復 | 最高層決策不再繞過 capitalflow | ✅ (#1372) |
| 4 | B1+B4: 管線重排 + VIX key 修復 | macro evidence 真正生效 | ✅ (#1372) |
| 5 | E1+E2: YAML loader + 參數可配置 | 憲章配置可被程式讀取 | ✅ (#1372) |
| 6 | D1: detector 時期敏感度 | 5 detector 差異化權重 | ✅ (#1372) |
| 7 | C1+C2: 七大勢力數據源 | 壽險/公司派/散戶上線 | ✅ (#1372 / #1421)；CAPTCHA 為下游依賴 |
| 8 | D5: RegimeAllocator 六策略 | 六策略×七時期矩陣 | ✅ (#1372) |

### 第二批：P1 — 信號不完整（v1.1 全部完成）

| 序 | 項目 | 狀態 |
|----|------|------|
| 9  | C4: capitalflow 4-layer Assessment 消費鏈 | ✅ (#1378) |
| 10 | C6: 散戶反向指標統一口徑 | ✅ (#1372) |
| 11 | B3: MacroDataSnapshot 補漏指標 | ✅ (#1372) |
| 12 | B5: Causal chain tracing | ✅ (#1372 + MCP `trace_get_*`) |
| 13 | D4: Narrative 19/24 themes 進入 regime inference | ✅ (#1372) |
| 14 | A4: macroflow RiskLevel 自動推導 | ✅ (#1372) |

### 第三批：P1 — 前端（v1.1 全部完成）

| 序 | 項目 | 狀態 |
|----|------|------|
| 15 | E3: API 輸出時期結構化欄位 | ✅ (#1406) |
| 16 | E4: 前端七時期 UI 卡片 | ✅ (#1397, #1398, #1408) |
| 17 | E5: 策略類別三分類（防禦/攻擊/戰術） | ✅ (#1404) |

### 第四批：P2 — 工程品質

| 序 | 項目 | 狀態 |
|----|------|------|
| 18 | C5 (p2): cfScore 常數權重 → 動態權重 | ✅ (#1372) |

---

## PR #1371 影響評估（v1.0 原文保留）

`refactor(live): C10 extract CircuitBreakerOps interface` — 純 `internal/live/` 內部 interface 提取，**不影響任何憲章審計項目**。

---

## v1.1 下一步建議（取代 v1.0 下一步）

1. **完成 E3 結構化欄位的下游消費驗證** — `period_source` / `cash_reserve` / `allowed_strategies` 對 MCP agent 是否被正確消費（PR #1426 接源已上、API schema 已上，下游未必全面更新）。
2. **持續 FinMind quota 系列維運** — 監看 traps.md 的 FinMind/Quota 群組與 known-issue badge 是否被 trigger；CAPTCHA 解封後公股欄位復活的黃金路徑。
3. **補強 MPI (MCP 公開清單)** — M1–M6 中至少 M1（PeriodDetector MCP）、M4（GetApplicableStrategies）、M6（憲章審計追蹤 MCP）尚未公開為 atlas-mcp 工具。
4. **補強憲章強制執行機制 X1–X3** — PR #1423（憲章檢查提速）已部分接 X1 但**未在 CI gate 上強制**；建議下波把 X1 升級為 mandatory CI 步驟。
5. **回顧 DeepSeek 覆核 F1–F5** — 仍未啟動；建議 sprint plan 列入下個 L2.x wave。
6. **文件治理**：本審計 + `docs/manifest-constitution-implementation.md` + `docs/manifest-constitution-gap-audit.md` + `docs/ATLAS_METHODOLOGY.md` 附錄 D 必須**同步更新**（v1.0 下一步 #4 仍有效）。

---

> **審計工具**: 5 個平行 CodeGraph/Codebase Memory scout agent + v1.1 期間 git log / GitHub PR API 對賬
> **審計覆蓋**: 40+ 檔案（v1.0 基準）+ 60+ 新增 PR commits（v1.1）

---

## 附錄 D：審計追蹤表（v1.0 原文 + v1.1 進度更新）

> **實施總表**: `docs/manifest-constitution-implementation.md`
> **差距審計**: `docs/manifest-constitution-gap-audit.md`

| # | 項目 | 等級 | 狀態 (v1.1) | 修復 PR/Commit |
|---|------|------|------|------|
| 1 | A1: 七時期判斷（DetectPeriod） | P0 | ✅ 完成 | #1372 |
| 2 | A2: 七時期→三態向下相容 | P0 | ✅ 完成 | #1372 |
| 3 | A3: 三套 regime 系統統一 | P0 | ✅ 完成 | #1372 (PeriodToRegime 映射) |
| 4 | B1: 管線重排（MacroFlow 前置） | P0 | ✅ 完成 | #1372 |
| 5 | B2: 每層輸出強制影響下一層 | P0 | ✅ 完成 | #1381 |
| 6 | B4: VIX key 修復 + macro evidence 注入 | P0 | ✅ 完成 | #1372 |
| 7 | C1: 七大勢力數據源（壽險/公司派/散戶） | P0 | ✅ 完成 | #1372 |
| 8 | C2: 公股行庫自動化通道 | P0 | ✅ 完成（**+ CAPTCHA 退避**）| #1372 + #1421 + #1424 + #1437 |
| 9 | C3: orchestrator PrimaryFlow 改用 capitalflow | P0 | ✅ 完成 | #1372 |
| 10 | D1: detector 時期敏感度 | P0 | ✅ 完成 | #1372 |
| 11 | D2: YAML consumer | P0 | ✅ 完成 | #1372 |
| 12 | D3: 推薦引擎按時期過濾 | P0 | ✅ 完成 | #1372 |
| 13 | D5: RegimeAllocator 六策略×七時期 | P0 | ✅ 完成 | #1372 |
| 14 | A4: macroflow RiskLevel 自動推導 | P1 | ✅ 完成 | #1372 |
| 15 | B3: MacroDataSnapshot 補漏指標 | P1 | ✅ 完成 | #1372 |
| 16 | C4: capitalflow 4-layer Assessment 消費鏈 | P1 | ✅ 完成 | #1378 |
| 17 | E3: API 輸出時期結構化欄位 | P1 | ✅ 完成 | #1406（結構化欄位）+ #1426（period 接源） |
| 18 | E4: 前端七時期 UI 卡片 | P1 | ✅ 完成 | #1397, #1398（方法論頁） + #1408（首頁 chip/走勢軸） |
| 19 | E5: 策略類別三分類 | P1 | ✅ 完成 | #1404（防禦/攻擊/戰術） |
| 20 | C5: QualityScore 公式 + cfScore 動態權重 | P2 | ✅ 完成 | #1372 |
| 21 | B5: Causal chain tracing | 已對齊 | ✅ 完成 | #1372 + MCP `trace_get_*` |
| 22 | C6: 散戶反向指標統一口徑 | 已對齊 | ✅ 完成 | #1372 |

### 進度統計（v1.1）

| 等級 | 總計 | ✅ 完成 | ⚠️ 部分 | ⬜ 未完成 |
|------|------|--------|--------|----------|
| P0 | 13 | 13 | 0 | 0 |
| P1 | 6 | 6 | 0 | 0 |
| P2 | 1 | 1 | 0 | 0 |
| 已對齊 | 2 | 2 | 0 | 0 |
| **合計** | **22** | **22** | **0** | **0** |

> v1.0 統計：19 ✅ / 1 ⚠️ partial / 2 ⬜ → v1.1：22 ✅ / 0 ⚠️ / 0 ⬜。**審計原始 20 項 + 已對齊 2 項全部完成**。
> 殘餘風險已轉移至 §附錄 E（v1.1 新發現差距）與 §附錄 F（F/M/X 治理追蹤表）。

> **v1.1 最後更新**：2026-08-07，commit `0f8667a6`。

### v1.0 期間「憲章審計外」新增項目（M1–M6 / F1–F5 / X1–X3）見 §附錄 F（v1.1 已逐項回填 7/27→8/7 進展）。

---

## 附錄 E：v1.0 → v1.1 patch audit（2026-07-30 → 2026-08-07）

> 本附錄列出 7/30 → 8/7 期間合併的 PR 中**與憲章方法論 / 治理直接相關**者，並指出它們對原審計項目的補強或新增風險。

### E1. 治理 / SOP 補強

| PR | 標題 | 對憲章審計影響 |
|---|---|---|
| #1423 | fix(ci): 憲章檢查腳本提速 25× | **直接補強 X1**：憲章檢查從幾分鐘降到秒級，使 X1 在 CI gate 上變得可行 |
| #1429 | fix(cleanup): Docker 禁令 — Makefile 防呆 | 新增工程紀律：禁止 AI 在 worktree 重建容器，避免憲章審計的測試/部署循環被打斷 |
| #1442 | docs(skills): todo schema discipline | ACI hook (#1464) 同源的 session-end checklist 補強；憲章審計對應 ACP 文件治理 |
| #1464 | feat(aci-hook): PreToolUse soft reminder for hot-path Go access | **補強 X2**：ACI 自動輕推 agent 更新憲章審計追蹤文件，降低審計漂移風險 |
| #1467 | chore(l2-4): close #825 #826 — dead code cleanup + docs alignment | L2.4 cleanup 對憲章文件治理意義：把憲章追蹤表從「無驗收」改為「驗收清單導向」（§3.0/§4.0/§5.0/§7.2 都補 ACI 盤查清單） |
| #1468 | docs(manifest): #825 #826 cleanup manifest §7.2 AC-1..AC-8 驗收清單 | 同上（驗收清單入憲章追蹤） |
| #1469 | docs(manifest): #825 #826 cleanup manifest §3.0/§4.0/§5.0 ACI 盤查清單 | 同上（ACI 清單入憲章追蹤） |

### E2. 對外 MCP 欄位接源（補強憲章審計對外契約）

| PR | 標題 | 對憲章審計影響 |
|---|---|---|
| #1426 | fix(mcp): period field sourced from period_history | **補強 E3 + B5**：原本對外 `period` 欄位是用 `RegimeToPeriod(regime)` 反推，會失真（典型：7/29 `regime=RISK_ON` → 推導 `bull`，但 `period_history` 真值是 `consolidation`）。修後對外欄位是 `period_history` 真值。新回應欄位：`period` + `market_period` (deprecated alias) + `period_source`（regime 與 period 兩源拆開） |
| #1441 | fix(atlas-mcp): experiment_diff passes experiment_id | 補強實驗評估鏈；對審計 E3 / D5 有資料源完整度 |
| #1443 | feat(experiment): expose judge-collected metrics in experiment_diff | 同上，補強實驗可追溯性 |

### E3. 資料源 / 觀測性（補強 B3、C2 完整性）

| PR | 標題 | 對憲章審計影響 |
|---|---|---|
| #1424 | fix(govflow): BTM 28h→24h + weekday 15:00+ + CAPTCHA 24h cooldown | **補強 C2**：CAPTCHA 解封前已不會再被上游封鎖，但日次最大 1 次 |
| #1437 | fix(govflow): BTM 1h + daily-once guard — 修復 24h 排程餓死 | 同上 |
| #1422 | feat(b5-batch3): PR-B calculator fill | **補強 B3**：sector rotation + 公股連買進 calculator，B5-3 PR-A 跨 schema 問題一併修 |
| #1421 | feat(b5-batch3-data-infra) | 同上（SectorIndexReader、GovernmentBrokerAggregator per-broker） |
| #1418 | fix: B5-R TAIEX 抓取韌性 | **補強 B3** TAIEX 鍵穩定性；`DataStatus` 增加可見性（degraded / stale） |
| #1446 / #1448 / #1449 / #1450 | Fugle v0.3 → v1.0 migration + rate limiter unification + Fugle key 註解校正 | **補強 B3**：Fugle rate limiter 統一進 Constitution（與憲章 §4.5.2 對齊）；TEJ_API_KEY 未設置時寫 inactive health record（治理） |
| #1451 | unified quota management | **補強 B3 + X3**：跨 apigateway/marketdata/monitoring 三層 quota 管理；陷阱：2026-08-06 investigation 結論此 PR 自稱 unified 但**未對應 server-side 600/hr 細粒度**（詳見 `docs/investigations/2026-08-06-finmind-quota-collision.md`） |
| #1452 | capture FinMind error body | **補強 B3**：channel health 內能看 FinMind 原始錯誤，避免「無值=假資料」風險 |
| #1453 | tw_vol stale auto-refetch on trading day rollover | **補強 B3**：陳舊快取自動 refetch |
| #1454 / #1455 / #1457 / #1458 | known-issue badges、crossmarket recovery 擴充 | **補強 X3**：long-stale channels 浮現 known-issue badge；`taifex-daily` dead alias 註冊；dash-separated runtime alias 收斂；crossmarket recovery 含 stale status |
| #1456 | compute FinMind endDate from actual last day of month | **補強 B3** FinMind 月底資料完整性 |
| #1461 | feat(industry): auto_cycle_update 失敗 metric + symbol coverage validator | **補強 X3**：自動暴露 cycle_update 失敗，補強章程 §監控原則 |
| #1462 | fix(industry): classifyFinMindError 識別彙總錯誤 | **補強 B3 + X3**：識別「no valid data for industry X」彙總錯誤，靜默 fallback 變顯式 |
| #1463 | test(industry): live FinMind symbol coverage 驗證 | **補強 B3**：build tag `livefinmind` 把 symbol coverage 拉成 dev-time 驗證 |
| #1472 | HF-1a+b: 透傳 rate-limit/402 error + classify 402→quota | **補強 B3 + X3**：HF-1 hotfix 系列，把 server-side 402 識別為 quota 而非 generic error |
| #1473 | HF-1c: fetch ctx 5s→10s 對齊 rate limiter 6s token | **補強 B3**：rate limiter 餘裕對齊 |
| #1474 | traps.md M1 — frontmatter + FinMind/Quota trap 群組 | **補強 X3**：把 14+ FinMind 修補循環（#1451 → #1463 系列）沉澱為 trap 知識庫，避免未來再撞牆時從零研判 |

### E4. PRISM autoBalancer 遷移 — Issue #1447 closure

| PR | 標題 | 對憲章審計影響 |
|---|---|---|
| #1471 | feat(prism): migrate autoBalancer from rogue ticker to BTM | **憲章 §4.5.2 違規 closure**：`prism_manager.go:564` 的 `RegisterAutoBalancer` 從直接 ticker 改成 BTM 任務排程，配 `AutoBalanceEnabled` gate 測試 |
| #1475 | docs(manifest): BTM ticker 評估 closure (17 ticker 重新分類) | 17 ticker 評估：15 個屬例外合法 / 1 個已遷 (#1471) / 1 個 dead code（待 deprecation）→ **Article 4 違規實際需遷移 = 0** |

### E5. Pipeline / experiment 觀測性

| PR | 標題 | 對憲章審計影響 |
|---|---|---|
| #1440 | fix(stress_test_daily): update dashboard latestDrawdown after stress scenarios complete | **補強 X3**：原本 `RunDailyStressTests` 跑完 stress scenarios 後只 log 不送 reporter，dashboard drawdown 永遠為 nil。修後路徑正常，可即時觀測壓力指數變化 |
| #1444 | feat(pipeline): paginate sessions endpoint + zero-outcome data-loss monitor | **補強 X3**：sessions endpoint 分頁（避免 agent 一次拉過大量 sessions）；zero-outcome monitor 防 data-loss |

### E6. SESSION_END / Multi-CLI 治理

| PR | 標題 | 對憲章審計影響 |
|---|---|---|
| #1427 | fix(ci): main 修復列車 | 補強憲章 CI gate 的細項 |
| #1428 | fix(20260730-ci-test-isolation) | Yahoo rate limiter 隔離 → CI 確定性（憲章 C1 測試基線） |
| #1430 / #1431 / #1432 / #1433 / #1434 | industry fallback_reason / reporting corrupted / sectorallocation / sector gitignore | 加固 C1 / C2 與 reporting / sectorallocation 完整性 |
| #1435 | docs(operations): MCP 2026-07-28 migration roadmap | 補強 M1–M6 的 migration timeline |
| #1436 | fix(test/eventdriven): time-anchored calendar + saturated bullish baseline | eventdriven test 確定性 |
| #1438 | docs(skills): align gitnexus SKILL.md with project-local runner | skill alignment |
| #1460 | docs(operations): consolidate PR lifecycle spec + AGENTS.md reference | PR lifecycle 文件彙整 |

### E7. 資料治理與邊界

| PR | 標題 | 對憲章審計影響 |
|---|---|---|
| #1439 | chore(tej): disable channel + scheduler | **新發現**：TEJ channel 在憲章 §4.5.2 框架下長期 `enabled=true` 但無資料流入（T3-A47 enable inconsistency）；#1439 強制關閉 channel + scheduler |
| #1477 | fix(stocktools): add coverage notice for out-of-scope TWSE symbols | **新發現**：MCP `stock_get_*` 系列以 TWSE 上市普通股為主（≈1070 names），常被誤用為 TPEX 上櫃；以 `stocktools-tool-result-2-of-4-out-of-scope.html` 方式顯式告知 |
| #1478 | docs(investigation): §8 external-verified affirmation of TPEX scope | 對應 #1477 配套，TPEX vs TWSE scope 的外部可驗證記錄 |

### E1–E7 綜合風險

本段期間**沒有發現新的 P0（會產生錯誤信號）**。已浮現的次級風險：
- **CAPTCHA 解封後公股欄位復活尚未實證** — 必須 CAPTCHA 解封 + 黃金測試再跑一次確認 `PublicBankConsecBuyDays` 由 0 → 真實值
- **Fugle v1.0 API + TEJ channel disable**：對 `stock_get_*` / `data_get_channels` MCP 工具的可用性會受影響，需驗 MCP user 是否察覺
- **MCP agent 對 out-of-scope TWSE/TPEX 的錯誤理解**：即使有了 #1477 coverage notice，仍可能誤用，列為 M6 治理追蹤項

---

## 附錄 F：憲章治理追蹤表（M1–M6 / F1–F5 / X1–X3，v1.1 更新）

> 對應 `docs/manifest-constitution-gap-audit.md` §憲章審計外新增項目。本附錄把 v1.0 的 ⬜ 全部走查 7/27 → 8/7 進展。

### F1–F5：DeepSeek 方法論覆核

| # | 項目 | v1.0 | v1.1 | 備註 |
|---|------|------|------|------|
| F1 | 外資雙重動機模型（結構性 vs 投機性分流） | ⬜ | ⬜ | 仍在 backlog；期間無相關 PR |
| F2 | 自營商大小分流（大型可納宏觀，小型用 AI 分點） | ⬜ | ⬜ | 同上 |
| F3 | 投信主動 vs 被動分流（ETF 被動買盤 vs 主動基金） | ⬜ | ⬜ | 同上 |
| F4 | 公股分點追蹤作為 BK-13 替代方案 | ⬜ | ⬜ **降級風險→ 啟動為 C2 平行方案** | 因 TWSE CAPTCHA 啟用，F4 由 fallback 升級為**主要**方案；#1421 parser 仍可走 CAPTCHA 解封路徑 |
| F5 | 選股層策略庫設計（Phase 4） | ⬜ | ⬜ | 仍待 T27 |

### M1–M6：MCP 工具對齊（部分已實現）

atlas-mcp 在 v1.1 期間已公開 80+ 工具（見 `cmd/atlas-mcp/server/tools*.go`），對照 M1–M6：

| # | 項目 | v1.0 | v1.1 | 對應 MCP 工具 |
|---|------|------|------|------|
| M1 | 時期判斷 MCP 工具公開 | ⬜ | ⚠️ partial | `macro_get_snapshot_latest`、`macro_get_snapshot_history`、`macro_get_stress_index_current` 對外暴露 7 時期結構化欄位，但**沒有獨立的 PeriodDetector.MCP**（最強寫死的 `period_detector.go` 呼叫路徑） |
| M2 | 資金流品質分數 MCP 工具公開 | ⬜ | ✅ 已實現（部分） | `capital_flow_summary`、`capital_flow_daily`、`macro_get_capital_flow_latest` 對外暴露 QualityScore、Z-score、force 名稱；**`QualityScore` 計算公式仍需 MCP 工具獨立公開** |
| M3 | 因果鏈 tracing MCP 工具公開 | ⬜ | ✅ 已實現 | `trace_get_decision_chain`、`trace_get_reasoning`、`trace_get_sim_latest`、`narrative_get_chains` 等 |
| M4 | 策略適用時期 MCP 工具公開 | ⬜ | ⚠️ partial | `get_recommendations`、`strategy_list_active`、`strategy_ranker`、`strategy_get_layers` 間接可用；**`GetApplicableStrategies(regime)` 未獨立公開** |
| M5 | 壓力指數元件 MCP 工具公開 | ⬜ | ✅ 已實現 | `taiwan_stress_index`、`macro_get_stress_index_current`、`narrative_stress_index_thresholds` |
| M6 | 審計狀態 MCP 工具公開 | ⬜ | ⬜ | 仍未實作；可由 `manifest_constitution_audit` 工具集取代或新增 |

### X1–X3：憲章強制執行機制

| # | 項目 | v1.0 | v1.1 | 備註 |
|---|------|------|------|------|
| X1 | PR 合併前憲章對齊檢查（CI gate） | ⬜ | ⚠️ partial | **#1423 提速 25× 後憲章檢查已可在 CI 內 ≤ 秒級跑完**；但**尚未強制為 mandatory gate**（僅在 `ci-full` 內） |
| X2 | 方法論變更強制更新追蹤表 | ⬜ | ⚠️ partial | **#1464 ACI hook**：PreToolUse soft reminder for hot-path Go access——會輕推 agent 在改憲章相關 hot-path 時同步更新追蹤表。**未升級為 PR template 強制檢查** |
| X3 | 憲章漂移自動警報（nightly scan） | ⬜ | ⚠️ partial | **#1454/#1455/#1457/#1458 known-issue badge + crossmarket recovery + alias 收斂**——對 channel 健康有顯式警報機制；但**憲章文件 ↔ code diff 的 nightly scan 仍未建**。traps.md (FinMind/Quota) 是手動建檔的 trap 群組，可視為手動版 X3 |

### v1.1 統計

| 群組 | v1.0 ⬜ | v1.1 ⬜ | ✅ | ⚠️ partial |
|------|--------|---------|----|-----------|
| F1–F5 | 5 | 4 (F1/F2/F3/F5) | 0 | 1 (F4 升級為主要方案) |
| M1–M6 | 6 | 1 (M6) | 3 (M2/M3/M5) | 2 (M1/M4) |
| X1–X3 | 3 | 0 | 0 | 3 |
| **總計** | **14** | **5** | **3** | **6** |

> v1.1 ⬜ = 4 + 1 + 0 = 5；⚠️ partial = 1 + 2 + 3 = 6；✅ = 0 + 3 + 0 = 3。**v1.0 全 ⬜ 14 項 → v1.1 已實作 3 項 / partial 6 項 / 仍 ⬜ 5 項**。


1. **本 sprint**：把 X1 升為 mandatory CI gate（#1423 已把時間成本降到秒級，最後一步是 gate config）。
2. **下個 sprint**：實作 M1 (`period_detector_mcp`) 與 M4 (`get_applicable_strategies_mcp`) 兩個 atlas-mcp 工具，把 7 時期 × 6 策略矩陣透過 MCP 直接回答「這個 regime 應該買什麼」。
3. **下下個 sprint**：實作 M6 — 把本追蹤表直接由 MCP 工具讀取（讓 agent 可以 self-audit 憲章對齊狀態）。
4. **2026-Q3 wave**：啟動 F1–F4 DeepSeek 覆核；F5 仍待 T27 選股層策略庫。

---

> **v1.1 最後更新**：2026-08-07，commit `0f8667a6`（`docs(investigation): append §8 external-verified affirmation of TPEX scope`）
> **下一次審計 (v1.2)** 預計在 F1–F4 啟動後
