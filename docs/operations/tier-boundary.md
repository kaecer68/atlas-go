# Tier Boundary Audit — HTTP API & MCP Tools

> 對應 PR #1041（`test/tier-boundary-audit` branch）。
> 本文件定義 atlas-go 對外 HTTP 端點與 MCP tools 的「應用場景分類」與「正式棄用清單」。
> 目的：避免後續開發時再次重複實作、誤加前端呼叫、或在錯誤的 tier 曝光內部校準/開發端點。

---

## 1. Tier 分類

atlas-go 的對外介面分為 4 個 tier（與 401 攔截器的 `excludedPages` / `onUnauthorized` 行為對齊）：

| Tier | 認證 | 適用對象 | Web UI | MCP | 範例 |
|------|------|----------|--------|-----|------|
| **Public** | 不需登入 | 訪客、未登入用戶 | ✅ | ✅ (部分) | `/health`, `/api/macro/snapshot/latest` |
| **Free** | JWT (free tier) | 註冊但未付費用戶 | ✅ | ✅ | `/api/dashboard/system-health`, `/api/narrative/bundle` |
| **Premium** | JWT (premium) | 付費訂閱用戶 | ✅ | ✅ (全 117 tools) | `/api/recommendations`, `/api/stock/*` |
| **Admin** | JWT (admin role) | 開發者、營運、admin_web 用戶 | ✅ (admin_web) | ✅ (token_admin) | `/api/control/*`, `/api/parameters/*` |

> **設計原則**：每個 HTTP 端點只能服務 **1 個 tier**；跨 tier 的資料需求透過「在 handler 內組合多個資料源」實作，而不是開放多個 tier 都能呼叫的端點。

---

## 2. 正式 Deprecated 端點（21 條，已加 `// Deprecated:` 標記）

> 對應 PR #1041。每條都加了 `// Deprecated:` 標記與指向本文件的連結。靜態分析工具（`staticcheck`、Go IDE）會自動警告後續呼叫端。

### 2.1 內部校準資料（8 條，純內部使用）

| 端點 | 替代方案 | 為何 deprecated |
|------|----------|-----------------|
| `GET /api/dashboard/industry-linkage` | (無) | 內部 linkage 演算法輔助資料，不對外暴露 |
| `GET /api/dashboard/industry-risk` | (無) | 內部 risk surface 計算，不對外暴露 |
| `GET /api/dashboard/industry-calibration` | (無) | 內部校準參數 |
| `GET /api/dashboard/industry-odm-channel` | (無) | ODM channel 內部資料 |
| `GET /api/dashboard/industry-data-aggregator` | (無) | Data aggregator 內部狀態 |
| `GET /api/dashboard/industry-seasonal-health` | (無) | 季節性健康度內部指標 |
| `GET /api/dashboard/industry-correlation-loader` | (無) | 相關性載入器內部狀態 |
| `GET /api/dashboard/rsi-tw-calibration` | (無) | RSI 台股校準參數 |

### 2.2 命名重複或重疊（5 條，請改用 canonical）

| 端點 | Canonical 替代 | 為何 deprecated |
|------|----------------|-----------------|
| `GET /api/macro/capital-flow/latest` | `/api/macro/snapshot/latest`（已包含 capital flow 欄位） | 同一份資料兩個端點，前端只需一個 |
| `GET /api/narrative/stress-index/current` | `/api/taiwan/stress-index` | 完整覆蓋，無須雙路徑 |
| `GET /api/narrative/stress-index/history` | `/api/taiwan/stress-index` | 同上 |
| `GET /api/narrative/stress-index/thresholds` | `/api/taiwan/stress-index` | 同上 |
| `GET /api/strategies/active` | `/api/strategies`（回傳所有策略，呼叫端 filter） | 同一份資料 |

### 2.3 細節已被 supersede（1 條）

| 端點 | 替代方案 | 為何 deprecated |
|------|----------|-----------------|
| `GET /api/strategies/{id}/summary` | `/api/strategies/{id}` + `/api/strategies/{id}/attribution` | 摘要資料已含於主回應 |

### 2.4 命名統一：report vs reports（1 條，單數標記 deprecated）

| 端點 | Canonical 替代 | 為何 deprecated |
|------|----------------|-----------------|
| `GET /api/report/latest` | `/api/reports/latest` | 路徑命名不一致；統一律定複數為 canonical |

### 2.5 重複摘要（1 條）

| 端點 | Canonical 替代 | 為何 deprecated |
|------|----------------|-----------------|
| `GET /api/dashboard/daily-summary` | `/api/reports/latest` | 摘要邏輯已整合到 dailyreport 模組 |

### 2.6 開發工具（2 條，生產環境不應開放）

| 端點 | 替代方案 | 為何 deprecated |
|------|----------|-----------------|
| `GET /api/docs` | (內部部署) | Swagger UI，僅開發用 |
| `GET /api/docs/swagger.json` | (內部部署) | OpenAPI spec，僅開發用 |
| `GET /api/health/data-integrity` | `/health` (liveness) + `/ready` (readiness) | 開發/維運 deep check |

### 2.7 尚未實作（2 條，預留 v0.0.0.32）

| 端點 | 預定用途 | 為何 deprecated |
|------|----------|-----------------|
| `GET /api/reports/archive` | 歷史報告查詢 | 尚未實作完成 |
| `POST /api/reports/subscribe` | 報告訂閱 | 尚未實作完成 |

### 2.8 手動觸發（1 條，建議改 BackgroundTaskManager）

| 端點 | 替代方案 | 為何 deprecated |
|------|----------|-----------------|
| `POST /api/macro/ingest` | `BackgroundTaskManager` | 手動觸發頻率極低；正式排程已由 BackgroundTaskManager 處理 |

---

## 3. Canonical 端點對照表

> 凡是前端/MCP 應該使用的「正式端點」清單。新增功能請優先使用 canonical 名稱。

### 3.1 投資人（Free + Premium tier）

| 領域 | Canonical 端點 | 備註 |
|------|---------------|------|
| 個股報價 | `/api/stock/quote` | 需 FUGLE_API_KEY |
| 個股基本面 | `/api/stock/fundamentals` | |
| 個股籌碼 | `/api/stock/chips` | |
| 個股技術 | `/api/stock/technical` | |
| 巨集快照 | `/api/macro/snapshot/latest` | 包含 capital flow |
| 巨集歷史 | `/api/macro/snapshot/history` | |
| 台灣壓力指數 | `/api/taiwan/stress-index` | |
| 敘事 bundle | `/api/narrative/bundle` | |
| 推薦 | `/api/recommendations` | premium only |
| 每日報告 | `/api/reports/latest` | |

### 3.2 投資人（Free tier — 公開燈號）

| 領域 | Canonical 端點 | 備註 |
|------|---------------|------|
| 系統健康 | `/health` | liveness |
| 巨集摘要 | `/api/macro/snapshot/latest` | 公開欄位 |
| 壓力指數 | `/api/taiwan/stress-index` | 公開 |

### 3.3 管理員（Admin tier）

| 領域 | Canonical 端點 | 備註 |
|------|---------------|------|
| 系統成熟度 | `/api/dashboard/maturity` | PR #1039 新增 |
| 資料通道詳情 | `/api/dashboard/data-channels/{name}` | PR #1039 新增 |
| 排程狀態 | `/api/scheduler/status` | |
| 任務管理 | `/api/tasks` | |
| 控制指令 | `/api/control/*` | |
| 參數管理 | `/api/parameters` | |
| 策略演化 | `/api/synergy/darwinian/status` + `/trend` | PR #1039 修復 |
| 觀察窗口 | `/api/synergy/l2-4-schedule` | |

---

## 4. 新增端點的紀律

新增任何 HTTP 端點或 MCP tool 時：

1. **先在 §1 對齊 tier**：這個端點是給 public / free / premium / admin 哪個 tier 用的？
2. **檢查 §2 Deprecated 清單**：是否已有相同資料的端點可重用？
3. **檢查 §3 Canonical 清單**：如果新端點屬於已存在的領域，請沿用 canonical 路徑。
4. **更新本文件**：在 §3 加入新端點到對應分類。
5. **不暴露內部校準**：§2.1 的 8 條模式不應擴增；新內部校準端點應標 private package，不走 HTTP。

違反以上紀律的 PR 應在 code review 階段被擋下。

---

## 5. MCP Tool 對應

MCP tool 與 HTTP 端點的對應關係（節錄）：

| MCP Tool | HTTP 端點 | Tier |
|----------|-----------|------|
| `stock_get_quote` | `GET /api/stock/quote` | Premium |
| `stock_get_fundamentals` | `GET /api/stock/fundamentals` | Premium |
| `stock_get_chips` | `GET /api/stock/chips` | Premium |
| `stock_get_technical` | `GET /api/stock/technical` | Premium |
| `macro_get_snapshot` | `GET /api/macro/snapshot/latest` | Free |
| `macro_get_snapshot_history` | `GET /api/macro/snapshot/history` | Free |
| `taiwan_get_stress_index` | `GET /api/taiwan/stress-index` | Free |
| `system_get_maturity` | `GET /api/dashboard/maturity` | Admin |
| `data_get_channel_detail` | `GET /api/dashboard/data-channels/{name}` | Admin |
| `synergy_get_darwinian_status` | `GET /api/synergy/darwinian/status` | Admin |
| `synergy_get_darwinian_trend` | `GET /api/synergy/darwinian/trend` | Admin |
| `regime_get_history` | `GET /api/dashboard/regime-history` | Free |
| `report_get_export_link` | `GET /api/reports/{id}/export` | Premium |

完整 117 個 MCP tools 對照見 [`docs/reference/tool-catalog.md`](../reference/tool-catalog.md)。

---

## 6. 變更紀錄

| 日期 | 變更 | 對應 PR |
|------|------|---------|
| 2026-07-09 | 初次盤點 21 條 Tier 3 端點並標記 Deprecated | PR #1041 |
| 2026-07-09 | 命名統一：/api/report → /api/reports (複數為 canonical) | PR #1041 |
| 2026-07-09 | 新增 canonical 端點對照表（§3） | PR #1041 |
