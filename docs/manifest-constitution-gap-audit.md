# 憲章對齊差距審計清單 v1.2

> **版本**: v1.2
> **產出日期**: 2026-07-27
> **審計源**: `docs/ATLAS_METHODOLOGY.md`、`docs/ATLAS_CONSTITUTION_AUDIT.md`
> **實施總表**: `docs/manifest-constitution-implementation.md`
> **更新規則**: 每完成一項，將狀態改為 ✅ 並標註 PR；發現新差距時新增項目並保留版本歷史。

---

## 總覽

| 指標 | 數值 |
|------|------|
| 審計項目 | **25 項** |
| 已完成 | **17 項** |
| 部分完成 | **1 項** |
| 待啟動 | **7 項** |
| 分類數 | 6 類（A–F） |

---

## A. 時期判斷系統

| # | 項目 | 狀態 | 備註 / PR |
|---|------|------|-----------|
| A1 | 七時期判斷（DetectPeriod） | ✅ | #1372 — `internal/portfolio/period_detector.go` |
| A2 | 七時期→三態向下相容映射 | ✅ | #1372 — `PeriodToRegime(MarketPeriod)` |
| A3 | 三套 regime 系統統一 | ✅ | #1372 — `realtime`、`sim`、`domain` 映射對齊 |
| A4 | macroflow RiskLevel 自動推導 | ✅ | #1372 — 多指標複合推導 |

## B. 因果傳導鏈

| # | 項目 | 狀態 | 備註 / PR |
|---|------|------|-----------|
| B1 | 管線順序重排（MacroFlow 前置） | ✅ | #1372 — 由外而內、由上而下 |
| B2 | 每層輸出強制影響下一層 | ✅ | #1381 — 層級依賴約束 |
| B3 | MacroDataSnapshot 補漏指標 | ✅ | #1372 — 補齊 8 項憲章指標 |
| B4 | macro data 進入 regime inference + Causal tracing | ✅ | #1372 — VIX key 修復、layer-0..7 trace |

## C. 資金流向與勢力分析

| # | 項目 | 狀態 | 備註 / PR |
|---|------|------|-----------|
| C1 | 七大勢力完整數據源 | ✅ | #1372 — 壽險/銀行、公司派/內部人、散戶維度補齊 |
| C2 | 公股行庫自動化數據通道 | ✅ | #1372 — 自動化分點加總 seam |
| C3 | capitalflow 輸出進入主決策鏈 | ✅ | #1372 — orchestrator 改取 capitalflow.PrimaryFlow |
| C4 | capitalflow 4-layer Assessment 消費者 | ✅ | #1378 — Assessor 接線完成 |
| C6 | 散戶反向指標統一口徑 | ✅ | #1372 — RSI-Tw 與 capitalflow 對齊 |

## D. 敘事引擎與策略映射

| # | 項目 | 狀態 | 備註 / PR |
|---|------|------|-----------|
| D1 | detector 時期敏感度 | ✅ | #1372 — 5 個 detector × 7 時期 |
| D2 | 時期→策略自動選擇 | ✅ | #1372 — `MethodologyAdvisor` 消費 YAML |
| D3 | 推薦引擎按時期過濾策略 | ✅ | #1372 — `GetApplicableStrategies()` |
| D4 | Narrative 24 themes 進入 regime inference | ✅ | #1372 — 全 themes 權重加成 |

## E. 前端與配置

| # | 項目 | 狀態 | 備註 / PR |
|---|------|------|-----------|
| E3 | API 輸出時期結構化欄位 | ⚠️ | struct exists，API builder 尚未接線 → T04/T05 |
| E4 | 前端七時期 UI 卡片 | ⬜ | 待 Phase 4 啟動 |
| E5 | 策略類別三分類（defensive/aggressive/tactical） | ⬜ | 待 Phase 4 啟動 |

## F. 方法論新增覆核

| # | 項目 | 狀態 | 備註 / PR |
|---|------|------|-----------|
| F1 | 外資雙重動機模型（結構性 vs 投機性分流） | ⬜ | DeepSeek 覆核，參見憲章第三層 |
| F2 | 自營商大小分流（大型可納宏觀，小型用 AI 分點） | ⬜ | 第四層與第五層行為差異 |
| F3 | 投信主動 vs 被動分流（ETF 被動買盤 vs 主動基金） | ⬜ | 第五層資金性質區分 |
| F4 | 公股分點追蹤作為 BK-13 替代方案 | ⬜ | 數據源 fallback |
| F5 | 選股層策略庫設計（Phase 4） | ⬜ | 憲章目前僅組合層 |

---

## 進度統計

| 分類 | 總計 | ✅ 完成 | ⚠️ 部分 | ⬜ 待啟動 |
|------|------|--------|--------|----------|
| A. 時期判斷 | 4 | 4 | 0 | 0 |
| B. 因果傳導 | 4 | 4 | 0 | 0 |
| C. 資金流向 | 5 | 5 | 0 | 0 |
| D. 敘事策略 | 4 | 4 | 0 | 0 |
| E. 前端配置 | 3 | 0 | 1 | 2 |
| F. 方法論新增 | 5 | 0 | 0 | 5 |
| **合計** | **25** | **17** | **1** | **7** |

---

## 版本歷史

| 版本 | 日期 | 變更摘要 |
|------|------|---------|
| v1.2 | 2026-07-27 | 恢復 Phase 0–2 憲章對齊變更，納入 25 項差距清單；17/25 完成 |
| v1.1 | — | 草稿 |
| v1.0 | — | 初稿 |

---

> **最後更新**: 2026-07-27，commit `47ebdecf`
> **維護責任**: 任何修改方法論實作的 PR，必須同步更新本文件與 `docs/manifest-constitution-implementation.md`。
