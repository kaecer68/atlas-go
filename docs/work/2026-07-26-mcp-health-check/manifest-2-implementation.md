# atlas-mcp UX/UI Health Check — Manifest II：根因分析與實作追蹤

> **版本**: v1.0 | **日期**: 2026-07-26
> **依賴**: Manifest I（`manifest-1-inventory.md`）

## 追蹤狀態說明
| 狀態 | 標記 |
|------|------|
| 待修復 | ⬜ |
| 修復中 | 🔧 |
| 待驗收 | ✅ |
| 已完成 | ✔️ |

---

## 🔴 P0 — Critical Bugs 根因與修復

### P0-1: `stock_get_quote` timeout → Fugle API key 未設定

**根因**:
- `cmd/atlas-mcp/server/tools_stock.go:14` 定義 tool，`GET /api/stock/quote?symbol=2330` → timeout
- Backend 在 `internal/config/config.go:103` 讀取 `FUGLE_API_KEY`
- `internal/monitoring/api/dashboard/data_channels.go:15` 會檢查 `FUGLE_API_KEY`，未設定時 channel 標記為 disabled
- 但 `stock_get_quote` 的 backend handler **沒有在呼叫 Fugle API 之前做 key check**，直接發 HTTP request，導致 timeout
- `dashboard_api.go:804` 的 `quote_warmup_skipped` 警告 "FUGLE_API_KEY not set" — 證明當前環境確實沒 key

**影響檔案**:
- `cmd/atlas-mcp/server/tools_stock.go`
- Backend handler for `/api/stock/quote` (待定位)

**修復方案**:
1. Backend `/api/stock/quote` handler 加前置檢查：`FUGLE_API_KEY` 未設定時回 503 with `{"error":"FUGLE_API_KEY not configured"}`
2. MCP tool 在 client timeout 時提供更明確的錯誤訊息
3. (Optional) 為 `stock_get_quote` 加入 fallback — 回傳最後一次 fetches 的快取

**狀態**: ⬜

---

### P0-2: `llm_get_cost` 503 → KimiClient 未注入

**根因**:
- `internal/monitoring/dashboard_api.go:124` 的 struct 欄位: `kimiClient *llm_annotator.KimiClient` 註解寫明 "nil if strategiesAnnotator is not a KimiClient"
- `dashboard_api.go:1417`: `mux.HandleFunc("/api/llm_annotator/cost", metrics.HandleCost(a.kimiClient, 0.001))`
- `internal/monitoring/metrics/handlers.go:61-68`: `HandleCost` 中的 guard: `if client == nil → 503 with "no KimiClient wired"`
- `cmd/atlas/main.go` 在創建 dashboard API 時沒有將 KimiClient 傳入

**影響檔案**:
- `cmd/atlas/main.go` (wiring)
- `internal/monitoring/dashboard_api.go` (struct + route)
- `internal/monitoring/metrics/handlers.go` (handler)

**修復方案**:
1. 在 `cmd/atlas/main.go` 中創建 `llm_annotator.KimiClient`（需 `LLM_KIMI_API_KEY` env var）
2. 若 API key 不存在 → `kimiClient` 仍為 nil → 保持 503 但 message 改為 "KimiClient not available (set LLM_KIMI_API_KEY)"
3. 或：提供 mock/empty cost reporter（回傳 `{"total_calls":0}`）

**狀態**: ⬜

---

### P0-3a: `detector_registry_list` JSON unmarshal → type mismatch

**根因**:
- `cmd/atlas-mcp/server/tools_template_detector.go:42-43`: `var out map[string]any` 然後 `s.cli.Get(ctx, path, nil, &out)`
- Backend handler: `cmd/atlas/template_detector.go:82-91`: 回傳 `[]detectorView` — **JSON array** `[{theme:"...", enabled:true}, ...]`
- `http_client.go:42-48`: `Get` 內部 `json.Unmarshal(body, result)` — 無法把 JSON array unmarshal into `map[string]any`
- **型別 mismatch: MCP client expects object, backend returns array**

**影響檔案**:
- `cmd/atlas-mcp/server/tools_template_detector.go:42-43` (MCP client decode)
- `cmd/atlas/template_detector.go:82-91` (backend handler)

**修復方案** (二選一):
- **方案 A (推薦)**: MCP client 改 `var out any` 或 `var out []any`，同時 decode 後 wrap 在 `out["detectors"]` 讓 response schema 保持 object format
- **方案 B**: Backend handler 包裝一層 `json.Encode(map[string]any{"detectors": out})` — 但會改變現有 API contract

**狀態**: ⬜

---

### P0-3b: `template_detector_status` JSON unmarshal → type mismatch

**根因**:
- `cmd/atlas-mcp/server/tools_template_detector.go:33`: `var out map[string]any` + `s.cli.Get(ctx, path, nil, &out)`
- Backend handler: `cmd/atlas/template_detector.go:63`: `json.NewEncoder(w).Encode(rows)` — rows is `[]*ScanResultRow` — **JSON array** or `null` / `[]` if empty
- "raw=3 bytes" = 空的 `[]\n` (2+1 bytes) 或 `null\n` (4 bytes minus 1)

**影響檔案**:
- `cmd/atlas-mcp/server/tools_template_detector.go:33` (MCP client decode)
- `cmd/atlas/template_detector.go:43-63` (backend handler)

**修復方案**: 同 P0-3a

**狀態**: ⬜

---

## 🟡 P1 — 文件缺陷修復

### P1-1: Hermes `${ATLAS_API_KEY}` 變數未展開

**根因**:
- `~/.hermes/config.yaml` 中 `ATLAS_API_KEY: ${ATLAS_API_KEY}` 是 literal 字串
- Hermes 的 config loader 不會做 shell variable expansion
- `make setup-mcp-agent` 在 `AGENT_QUICKSTART.md §1.5` 被描述為會自動 source `.env`，但實際效果未驗證

**修復方案**:
1. 修正 `setup-mcp-agent` 或 Makefile 目標，確保 API key 在寫入 config 前已展開
2. 在 `AGENT_QUICKSTART.md` 加注：「請勿直接複製貼上 — `${ATLAS_API_KEY}` 需替換為實際 key」
3. 可用 `hermes mcp add` 的 `--env ATLAS_API_KEY=$ATLAS_API_KEY` （shell 層展開）

**狀態**: ⬜

### P1-2: Tool count 不一致 → 統一來源

**修復方案**:
1. 在 `cmd/atlas-mcp/server/server.go` 加上 startup 時 `log.Printf("registered %d tools", n)` — 成為 source of truth
2. 所有文件引用這個數字或寫「見 `bin/atlas-mcp 2>&1 | head -3`」

**狀態**: ⬜

### P1-3: 移除「registered N tools」誤導描述

**修復方案**:
1. 在 `mcp-integration-local.md §5` 中修正驗證步驟：改為實際輸出
2. 若 P1-2 實作後，文件改為引用新訊息

**狀態**: ⬜

### P1-4: `calendar_events` vs `event_calendar` 命名重複

**修復方案**:
1. 確認兩個 tool 的回傳結構差異
2. 若完全重疊 → 標記 `calendar_events` deprecated + redirect to `event_calendar`
3. 若功能有差異 → 在 `tool-catalog.md` 中明確說明差異

**狀態**: ⬜

---

## 🟠 P2 — 數據品質修復

### P2-1: Alert noise → 降低重複告警（dedup race fix）
- **修正**: `dedup.Track()` 從 async goroutine 移至 sync（`Check()` 後立即呼叫）
- **PR**: #1357
- **狀態**: ✔️

### P2-2: `risk_get_drawdown` not available → 明確狀態

### P2-3: Channel unknown → 標記 disabled
- **修正**: 未接 static builder 的 registry channel 從 `status: "unknown"` 改為 `status: "inactive"`
- **位置**: `internal/monitoring/service/data_channels.go:376-386`
- **PR**: #1357
- **狀態**: ✔️


### P2-5: Scheduler summary → 摘要
- **狀態**: ⬜

---

## 🔵 P3 — UX 改善追蹤

| ID | 項目 | 狀態 |
|----|------|------|
| P3-1 | First Contact SOP | ⬜ |
| P3-2 | Sector 中文標籤 | ⬜ |
| P3-3 | Parameters 摘要 | ⬜ |
| P3-4 | Emoji 可選 | ⬜ |

---

## 實作順序建議

```
Wave 1 (P0 修復):
  ├── P0-3a: detector_registry_list JSON fix (1 file, simple)
  ├── P0-3b: template_detector_status JSON fix (1 file, simple)  
  ├── P0-1: stock_get_quote graceful degradation (2 files)
  └── P0-2: llm_get_cost KimiClient wiring (1 file)

Wave 2 (P1 文件):
  ├── P1-1: Hermes config env expansion
  ├── P1-2: Unified tool count
  ├── P1-3: Fix startup message doc
  └── P1-4: Deduplicate calendar tools

Wave 3 (P2/P3 改善):
  └── (依優先級排序)
```
