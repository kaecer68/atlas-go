# 憲章對齊 — 問題盤查清單（Gap Audit Manifest）

> **版本**: v1.1
> **更新日期**: 2026-07-27
> **用途**: 驗收清單 — 所有項目完成後，憲章才算真正落地
> **對應實作**: `docs/manifest-constitution-implementation.md`
> **來源**: `docs/ATLAS_CONSTITUTION_AUDIT.md` + 手動盤查（MCP / 前端 / 文件體系 / 執行機制）
> **Phase 1 完成**: T04+T05+T06（API 時期輸出 + regime history 七時期擴展）

---

## 類別 A：原始審計未完成項（4 項）

| # | 項目 | 等級 | 現狀 | 說明 |
|---|------|------|------|------|
| A1 | **E3: API 輸出時期結構化欄位** | P1 | ✅ 完成 | T04+T05: `DailySummaryReport` 透過 PeriodProvider callback 填充；`dailyreport.Report` 新增 `Period` section 含 market_period/period_name_zh/cash_reserve/allowed_strategies |
| A2 | **E4: 前端七時期 UI 卡片** | P1 | ❌ 未啟動 | 前端 `src/` 目錄下無任何 period/MarketPeriod 程式碼；憲章 Phase 3 "當前市場時期卡片" 未實作 |
| A3 | **E5: 前端策略類別三分類** | P1 | ❌ 未啟動 | 前端無 defensive/aggressive/tactical 分類；`home-tier-sections.js` 仍用 free/registered/premium |
| A4 | **C4: capitalflow 4-layer Assessment 生產環境接線** | P1 | ⚠️ 部分完成 | 後端介面+欄位已就緒（#1378），但 `Assessor` 與 `capitalflow.Service` 在不同 goroutine scope，生產環境完整接線 pending |

---

## 類別 B：MCP 工具對齊缺失（6 項）

> atlas-mcp 為外部 AI agent 主要操作介面。七時期判斷、策略過濾、時期敏感度、因果追蹤等全部憲章能力，在 MCP 層**零輸出**。

| # | MCP 工具 | 缺失內容 | 等級 | 狀態 |
|---|---------|---------|------|------|
| B1 | `daily_report` | 無 `market_period` 欄位、無 `allowed_strategies`、無時期對應的 cash_reserve | P0 | ⚠️ API 層完成（T05），MCP 層 pending（T07） |
| B2 | `macro_get_snapshot_latest` | 無當前時期 ID / 名稱 | P0 | ❌ → T08 |
| B3 | `regime_get_history` | 僅輸出三態（RISK_ON/OFF/NEUTRAL），無七時期歷史 | P0 | ⚠️ API 層完成（T06），MCP 層 pending（T10） |
| B4 | `get_recommendations` | 未根據當前時期過濾策略；RISK_OFF 仍可能推薦 growth/momentum | P0 | ❌ → T09 |
| B5 | `narrative_get_events` | detector 輸出無 `period_weight`（時期敏感度係數） | P1 | ❌ → T11 |
| B6 | `explain_market_move` | 無憲章因果鏈標註（layer-0...layer-7 ID / parent reference） | P1 | ❌ → T12 |

---

## 類別 C：文件體系矛盾（3 項）

| # | 文件 | 問題 | 等級 | 狀態 |
|---|------|------|------|------|
| C1 | `ATLAS_METHODOLOGY.md` 附錄 D | **全部 ⬜** — 儘管 #1372/#1378/#1381 已完成大量工作 | P0 | ✅ 已修復 |
| C2 | `ATLAS_CONSTITUTION_AUDIT.md` 附錄 D | 過時（commit `1c60cbaf`），未反映 #1378/#1381 修復 | P1 | ✅ 已修復 |
| C3 | `ATLAS_METHODOLOGY.md` §現狀報告 | 仍描述舊版三態系統，未更新為七時期架構 | P1 | ✅ 已修復 |

---

## 類別 D：憲章強制執行機制缺失（4 項）

| # | 機制 | 現狀 | 等級 |
|---|------|------|------|
| D1 | CI：策略邏輯符合憲章驗證 | 無 | P0 |
| D2 | CI：methodology_rules.yaml 與程式碼一致性檢查 | 無 | P1 |
| D3 | pre-commit：禁止繞過 MethodologyAdvisor 直接呼叫 RankedStrategies | 無 | P1 |
| D4 | 文件漂移偵測：憲章文件欄位與 domain struct 不一致時警告 | 無 | P1 |

---

## 類別 E：隱性風險（3 項）

| # | 風險 | 說明 | 等級 |
|---|------|------|------|
| E1 | 三套 regime 系統並存 | `domain.Regime`(3 態)、`realtime.RegimeType`(7 微觀)、`sim.RegimeType`(4 閾值) 各自獨立 | P1 |
| E2 | 策略類別命名未對齊 | `home-tier-sections.js` 用 free/registered/premium，憲章用 defensive/aggressive/tactical | P2 |
| E3 | 散戶反向指標雙口徑 | `portfolio.factor_engine_institutional` 與 `capitalflow.ForceRetail` 兩套計算並存 | P1 |

---

## 類別 F：DeepSeek 覆核新增（5 項，全未啟動）

| # | 項目 | 說明 | 等級 |
|---|------|------|------|
| F1 | 外資雙重動機模型 | 結構性配置資金 vs 投機性資金分流 | P1 |
| F2 | 自營商大小分流 | 大型自營商納入宏觀，小型用 AI 分點追蹤 | P2 |
| F3 | 投信主動 vs 被動分流 | ETF 被動買盤 vs 主動基金行為信號區分 | P1 |
| F4 | 公股分點追蹤 | 作為 BK-13 自建證交所分點加總的替代方案 | P1 |
| F5 | 選股層策略庫 | Phase 4 選股層設計（憲章目前僅組合層） | P2 |

---

## 統計

| 類別 | 總計 | ✅ | ⚠️ 部分 | ❌ 未啟動 |
|------|------|----|---------|----------|
| A: 未完成項 | 4 | 1 | 1 | 2 |
| B: MCP 對齊 | 6 | 0 | 2 | 4 |
| C: 文件矛盾 | 3 | 3 | 0 | 0 |
| D: 執行機制 | 4 | 0 | 0 | 4 |
| E: 隱性風險 | 3 | 0 | 0 | 3 |
| F: DeepSeek | 5 | 0 | 0 | 5 |
| **合計** | **25** | **4** | **3** | **18** |

---

> **驗收規則**: 本清單每一項必須由 `manifest-constitution-implementation.md` 的對應任務覆蓋；實作完成後逐項打 ✅。T27 強制交叉驗收本清單全部項目。
