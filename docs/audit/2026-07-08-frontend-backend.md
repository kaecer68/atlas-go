# 前後端交叉稽核報告

**稽核日期**: 2026-07-08
**稽核執行**: Sisyphus (OhMyOpenCode)
**稽核範圍**: atlas-go v0.0.0.31 — client_web / admin_web 前端 vs internal/monitoring 後端 + cmd/atlas-mcp MCP server
**主索引**: [docs/operations/README.md](../operations/README.md)

---

## 0. 修正紀錄 (Correction Log)

| 日期 | 修正內容 |
|------|---------|
| 2026-07-09 | 修正「`shared_web/static/js/pages/performance-report.js` 為 dead code」的誤判。實際驗證該檔被 `client_web/static/js/component-init.js:1` 與 `client_web/static/js/main.js:400-405` 動態 import 引用，是 `client_web` 績效報告頁的活動 shell。原始 PR #1039 開發者正確判斷只刪除 3 個真正 dead 的檔（`synergy.js` / `prism.js` / `evolution.js`），保留 `performance-report.js` 是對的。 |

> 教訓：在判定 dead code 前必須以 `grep -r "<symbol>"` 確認無動態 import / 字串引用，不能只靠 `ls` 與 `git log` 推測。

---

## 1. 稽核方法

盤查三個面向並交叉比對：

1. **後端能力盤查**：91 個 MCP tools、110+ 條 HTTP 路由、25+ 個 internal/monitoring handler
2. **前端呼叫盤查**：client_web 113 個 fetch 端點呼叫、admin_web 95 個 fetch 端點呼叫
3. **頁面結構盤查**：24 個 shared_web pages、9 個 client_web pages、10 個 admin_web pages

工具：GitNexus MCP（`codegraph_explore` / `gitnexus_query` / `gitnexus_impact`）+ shell grep + 手動讀取關鍵檔案。

---

## 2. 交叉比對結果

### 2.1 後端 API vs 前端呼叫（35+ 條孤兒 API）

| 類別 | 數量 | 處理 |
|------|------|------|
| **Tier 1** 對投資人有高生產力 | 7 | 透過 MCP bot 對外可用 + 部分 Web UI 強化（資本流明細、策略排名） |
| **Tier 2** 對管理員有明確價值 | 9 | 修復 404（darwinian-status/trend、maturity、data-channels/{name}）與 JSON parse（reports/export、regime-history） |
| **Tier 3** 重複或非必要 | 19 | 標記 `// Deprecated:`，不另開 Web UI |

### 2.2 前端頁面 vs 後端領域

- **Dead pages**（3 個）→ 已刪除：`shared_web/static/js/pages/{synergy,prism,evolution}.js`（commit `37c58444`）
- **誤判 dead**（1 個）→ 保留：`performance-report.js` 是 client_web 績效報告頁的活動 shell，見 §0 修正紀錄
- **壞掉的 UI 入口**：6 處（`#upgradeBtn` 無 handler、admin sidebar 缺 metrics/config、`switchPage('decision')` 指向不存在 pageId、metrics/config 頁無 main.js 引用、inbox/alerts 模組被 dynamic import 但未對外暴露等）

---

## 3. 投資人視角缺口評估

| 缺口 | 影響 | 處置 |
|------|------|------|
| `/api/stock/*` 4 個個股 API | **最高** — 投資人最常問的問題 | ✅ MCP 已對外（OpenClaw/Hermes bot 可用） + issue #1038 開立 Web UI 開發 |
| `/api/capital-flow/daily` | 中 — summary 已涵蓋主要資訊 | ✅ P3.1 home 加入「展開明細」按鈕（PR #1042） |
| `/api/strategy-ranker/rank` | 中 — 投資人會想比較策略 | ✅ P3.2 strategies 頁面嵌入排名表（PR #1042） |
| `/api/macro/snapshot/history` | 中 — 適合 MCP 對話查詢 | ✅ MCP 已對外，無 Web UI（圖表已在 narrative 頁） |
| `/api/capital-flow/daily` 細節 | 低 — 已併入 P3.1 實作 | ✅ 同上 |
| Web UI 個股快查 | **高** — 但屬大 scope | 📋 issue #1038（下一輪開發） |

## 4. 網站管理者視角缺口評估

| 缺口 | 影響 | 處置 |
|------|------|------|
| 5 條後端 404/schema 錯誤 | **高** — 阻擋 MCP bot 對外功能 | ✅ P1.1-P1.6 全部修復（commit `37c58444`） |
| 4 條前端入口斷裂 | 中 — admin 點擊無反應 | ✅ P1.7-P1.9 + admin sidebar 補齊（PR #1040、commit `37c58444`） |
| Admin 401 攔截器 | 中 — admin 存取 tier-gated API 變沉默失敗 | ✅ P3.6 新增 fetch-wrapper（PR #1040） |
| Admin home 無 scheduler / data-pipeline 視圖 | 低 — 開發者用 MCP 查就夠 | ✅ P3.4 + P3.5 嵌入（PR #1043） |
| Tier 3 API 無標記 | 低 — 未來維護者困惑 | ✅ P4.2/4.3 加 `// Deprecated:` 註解（PR #1041） |

---

## 5. 實作清單（5 個 PR 全部完成）

| PR | Commit | 內容 |
|------|--------|------|
| [#1039](https://github.com/kaecer68/atlas-go/pull/1039) | `bb181722` | Stock MCP E2E 測試（`tools_stock_e2e_test.go` 220 行）+ 投資人查詢模板（`docs/operations/stock-mcp-query-templates.md`）+ 刪除 3 個 dead pages + Phase 1 9 個 bug 修復 |
| [#1040](https://github.com/kaecer68/atlas-go/pull/1040) | `7b612304` | Admin 401 攔截器 + tier-gated redirect（`shared_web/static/js/shared/fetch-wrapper.js` 113 行 + test 165 行） |
| [#1041](https://github.com/kaecer68/atlas-go/pull/1041) | `f628a5c1` | Tier 邊界 audit（`docs/operations/tier-boundary.md` 178 行）+ 10 處 `// Deprecated:` 標記 |
| [#1042](https://github.com/kaecer68/atlas-go/pull/1042) | `369b61cd` | Client 資金流明細展開 + strategies 排名嵌入（home-tier-sections.js + strategies.js 改動） |
| [#1043](https://github.com/kaecer68/atlas-go/pull/1043) | `f2f79fa3` | Admin scheduler status + datachannels data-pipeline 嵌入（dashboard.js + datachannels.js 改動） |

**PR #1041 修補 commit**：`26ed31f8`（修正 `tier-boundary.md:168` 斷掉的內部連結 `AGENT_tools.md` → `../AGENT_tools.md`）。

---

## 6. 相關交付物

| 檔案 | 用途 |
|------|------|
| `docs/operations/tier-boundary.md` | Tier 邊界 audit：MCP tool / HTTP / Web UI 對照表 + Deprecated 標記彙整 |
| `docs/operations/stock-mcp-query-templates.md` | 投資人透過 OpenClaw/Hermes 詢問 stock_get_* 工具的查詢模板範例 |
| `cmd/atlas-mcp/server/tools_stock_e2e_test.go` | 4 個 stock MCP tools 的 E2E 測試（mock HTTP server） |
| `shared_web/static/js/shared/fetch-wrapper.js` | Admin 401 攔截器（tier-gated redirect + classifyFetchError） |
| `internal/monitoring/api/pipeline/report_handlers.go` | `// Deprecated:` 標記範例 |
| `internal/monitoring/api/strategies/handlers.go` | `// Deprecated:` 標記範例 |
| `internal/monitoring/api/narrative/handlers.go` | `// Deprecated:` 標記範例 |
| `internal/monitoring/api/dashboard/handlers.go` | `// Deprecated:` 標記範例 |
| `internal/monitoring/api/system/swagger_handlers.go` | `// Deprecated:` 標記範例 |

---

## 7. 待辦（給下一輪開發）

| Issue | 標題 | 原因 |
|------|------|------|
| [#1038](https://github.com/kaecer68/atlas-go/issues/1038) | client_web 新增「個股快查」功能 | 投資人最常問的問題，但 scope 較大，本輪先確保 MCP bot 可用（已 ship），Web UI 留給下一輪 |

---

## 8. 教訓彙整（給未來 AI Agent）

1. **不要在判定 dead code 前只靠 `ls`** — 必須以 `grep -r "<symbol>" <dir>` 確認無動態 import / 字串引用
2. **不要在 `gh pr merge` 前用不完整的 grep pattern** — 應該用結構化查詢（`gh pr view --json statusCheckRollup`）並掃描所有 `conclusion` 欄位（SUCCESS / FAILURE / NEUTRAL / TIMED_OUT / CANCELLED / SKIPPED）
3. **對既有判斷保持懷疑** — PR review 中若發現與原始 audit 矛盾（例如這份報告誤判 performance-report.js 為 dead），應立即驗證並修正，不應盲從
