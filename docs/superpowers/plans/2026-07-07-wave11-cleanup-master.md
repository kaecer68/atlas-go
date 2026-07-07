# Wave 11 清理與對齊 — 主實作規劃

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement each phase task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修復 PR #971–975 合併後 Wave 11 投資核心在 routing、auth、MCP 暴露、前端合約、資料串接、文件一致性上的 P0/P1 缺口，使 atlas-go 達到可對台灣投資人釋出的水準。

**Architecture:** 分六個獨立 PR 階段推進，每個階段聚焦一個切面（routing/auth → MCP → frontend → data providers → docs → security/tests），降低審查風險並確保每階段單獨可測試、可合併。

**Tech Stack:** Go 1.26, PostgreSQL 15, Redis 8, vanilla JS/esbuild, MCP protocol, JWT, GitHub PR.

## Global Constraints

- 所有 Go 檔案必須通過 `gofmt`、`go vet`、`golangci-lint`、`staticcheck`、`gosec`。
- 覆蓋率門檻 60%；新增程式碼須補測試。
- 禁止修改現有測試邏輯來掩蓋問題；只修正因介面變更導致的編譯錯誤。
- 每個 Phase 結束時必須：commit → push → 開 PR → 通過 CI → 才能進入下一 Phase。
- 文件變更須與程式碼同步，禁止文件超前實作或實作超前文件。
- 生產環境 (`ATLAS_ENV=production`) 不得使用硬編碼 fallback secret、不得使用 `Secure=false` cookie。

---

## Phase 1: Auth / Routing / Public Path 止血

**Branch:** `feat/wave11-phase1-auth-routing`
**Scope:** 讓 Wave 11 新 API 對 browser 可公開存取、修復登入狀態在重新整理後遺失、讓 login/register/premium page-shell 真正初始化、提供 logout 端點與 UI。

**Key files:**
- `cmd/atlas/main.go`
- `internal/monitoring/api/shared/handler.go`
- `internal/subscription/handler.go`
- `internal/subscription/auth.go`
- `shared_web/static/js/services/auth.js`
- `client_web/static/js/main.js`
- `client_web/static/index.html`
- `cmd/atlas/main_test.go`

**Deliverables:**
1. `/api/capital-flow`、`/api/events`、`/api/recommendations`、`/api/reports` 加入 `isPublicPath`。
2. `/api/capital-flow/`、`/api/events/` 加入 `authFreePrefixPaths`。
3. `/api/user/profile` 回傳扁平的 `email` + `tier`/`effective_tier`，讓 `auth.js` 正確判斷登入狀態。
4. `POST /api/auth/logout` 清除 HttpOnly cookie。
5. `client_web/static/js/main.js` 在載入 page-shell 後呼叫 `mod.init()`。
6. 側邊欄加入「登出」按鈕並呼叫 logout。
7. 新增/更新 `cmd/atlas/main_test.go` 中 `TestIsPublicPath` 測試案例。

**詳細規劃:** `2026-07-07-wave11-phase1-auth-routing.md`

---

## Phase 2: MCP Tool 暴露與 Prompt 對齊

**Branch:** `feat/wave11-phase2-mcp-tools`
**Scope:** 把 Wave 11 新增的 HTTP 能力正確暴露為 MCP tools，修復 prompt 引用不存在 tool 的問題，統一 tool 數量文件。

**Key files:**
- `cmd/atlas-mcp/server/server.go`
- `cmd/atlas-mcp/server/tools_capitalflow.go` (new)
- `cmd/atlas-mcp/server/tools_recommendation.go` (new)
- `cmd/atlas-mcp/server/tools_strategy.go`
- `cmd/atlas-mcp/server/prompts.go`
- `cmd/atlas-mcp/server/tools_briefing.go`
- `docs/AGENT_TOOLS.md`
- `cmd/atlas-mcp/server/AGENTS.md`
- `docs/specs/agent-mcp-server.md`

**Deliverables:**
1. 新增 `capital_flow_daily`、`capital_flow_summary` MCP tools。
2. 新增 `get_recommendations` MCP tool（或包裝 `strategy_ranker`）。
3. 修復 `server.go` tool count assertion 支援 79–83。
4. 修復 prompts 引用不存在 tool 的問題（`capital_flow_daily`、`strategy_ranker`）。
5. 補上 `tools_events_test.go`、`tools_briefing_test.go`、`tools_capitalflow_test.go`。
6. 統一 README / AGENTS.md / AGENT_TOOLS.md / spec 的 tool 總數與分類。

---

## Phase 3: 前端欄位合約與頁面生命週期

**Branch:** `feat/wave11-phase3-frontend-contracts`
**Scope:** 修復前端讀取錯誤欄位、完成 page-shell 初始化後的資料渲染、補上中文在地化與 trust footer 資料來源。

**Key files:**
- `client_web/static/js/components/home-tier-sections.js`
- `client_web/static/js/page-shells/premium.js`
- `shared_web/static/js/pages/home.js`
- `shared_web/static/js/components/trust-footer.js`
- `shared_web/static/js/shared/field_types.ts`
- `client_web/smoke/run.mjs`

**Deliverables:**
1. 資金流向區塊使用正確欄位（`force`/`z_score`/`trend` 或切換到 `/api/capital-flow/daily`）。
2. 每日報告區塊使用正確欄位（新增 `summary` 或讀取既有欄位）。
3. Premium 頁面「立即升級」按鈕綁定事件（至少呼叫 `POST /api/reports/subscribe` 或顯示 WIP）。
4. 修復 `DATA_SOURCES` filter 錯誤，讓 trust footer 顯示資料來源。
5. 為 `tier`/`impact`/`strategy active` 增加繁體中文對照表。
6. 補上 `mcp`、`errors/404` 到 smoke test。

---

## Phase 4: 後端資料提供者串接（停掉 stub）

**Branch:** `feat/wave11-phase4-data-providers`
**Scope:** 把 `eventdriven`、`dailyreport`、`recommender` 從靜態/寫死資料改為接真實資料來源，並啟用 `strategy_ranker`/`strategy_validator`。

**Key files:**
- `internal/eventdriven/predictor.go`
- `internal/dailyreport/report.go`
- `internal/recommender/handler.go`
- `internal/strategy_ranker/ranker.go`
- `internal/strategy_validator/validator.go`
- `cmd/atlas/main.go`
- `internal/capitalflow/forces.go`

**Deliverables:**
1. `eventdriven.Predictor` 透過 `SetCapitalFlow` 接入真實 `CapitalFlowProvider`。
2. `dailyreport.Generator` 透過 `SetProvider` 接入真實 macro/marketdata provider。
3. `recommender` 呼叫 `strategy_ranker.RankAndTier` 產生真實策略排名。
4. 補上 `extractFutures` / `extractGovernment` 真實資料或暫時從文件/API 中移除。
5. `/api/reports/subscribe` 實作持久化與通知機制，或標示為 WIP。

---

## Phase 5: 文件體系對齊

**Branch:** `feat/wave11-phase5-docs-alignment`
**Scope:** 清理 README merge conflict、統一計數、補齊新模組索引、更新 AGENT_TOOLS.md 分類。

**Key files:**
- `README.md`
- `AGENTS.md`
- `docs/AGENT_TOOLS.md`
- `internal/AGENTS_INDEX.md`
- `internal/MATURITY.md`
- `docs/specs/agent-mcp-server.md`
- `docs/TRAPS.md`
- `docs/WORKFLOW_MAP.md`
- `CHANGELOG.md`

**Deliverables:**
1. 移除 `README.md` 的 conflict marker 與重複區段。
2. 在 `internal/AGENTS_INDEX.md` 加入 Wave 11 七個新模組。
3. 統一 tool 總數為 79（或 feature-gate 全開 81）。
4. 修正 AGENTS.md 21/22 矛盾與 Markdown 連結語法錯誤。
5. 修正 AGENT_TOOLS.md 分類與未實作 tool 標示。
6. 更新 agent-mcp-server.md 狀態與資源數量。

---

## Phase 6: 安全強化與測試補強

**Branch:** `feat/wave11-phase6-security-tests`
**Scope:** 強化 subscription 安全、補齊測試與環境變數管理。

**Key files:**
- `internal/subscription/store.go`
- `internal/subscription/handler.go`
- `internal/subscription/auth.go`
- `cmd/atlas/main.go`
- `configs/allowed_env_vars.md`
- `.env.example`
- `docker-compose.yml`
- `docs/ENVIRONMENT.md`
- `internal/subscription/handler_test.go`

**Deliverables:**
1. 密碼儲存改用 bcrypt（或 Argon2id）。
2. 登入/註冊端點加入 rate limiting。
3. 生產環境強制 `ATLAS_JWT_SECRET >= 32` chars，`Secure=true`。
4. `ATLAS_JWT_SECRET` / `ATLAS_SUBSCRIPTION_DB_PATH` 加入 allowlist 與環境模板。
5. 補齊新模組單元測試至 60% 以上。
6. 實作 `ValidateTier` middleware 並套用到敏感端點。

---

## Phase 7: 總驗收測試與文件一致性確認

**Branch:** `feat/wave11-phase7-acceptance`（或直接在 main 上驗收）
**Scope:** 所有 PR 合併後進行端到端驗證，確保程式碼、文件、測試、CI 一致。

**Checklist:**
- [ ] `go test ./...` 全綠（排除已知的 flaky test 並記錄）。
- [ ] `go test -cover` 顯示新模組覆蓋率 >= 60%。
- [ ] `gofmt`、`go vet`、`golangci-lint`、`staticcheck`、`gosec` 全綠。
- [ ] `npm run build` 在 `client_web` 與 `admin_web` 成功。
- [ ] `client_web/smoke/run.mjs` 通過。
- [ ] `cmd/atlas-mcp/server` 在 feature flags 全開/全關都能啟動。
- [ ] 文件中的 tool 數量、endpoint 清單、模組數量與程式碼一致。
- [ ] 從瀏覽器驗證：登入 → 重新整理仍維持登入 → 首頁資金流/事件/報告區塊正常渲染 → 登出後狀態清除。
- [ ] 從 MCP client 驗證：可呼叫 `capital_flow_daily`、`event_calendar`、`daily_report`、`get_recommendations`。

---

## 依賴與風險

| 風險 | 影響 | 緩解 |
|------|------|------|
| Phase 1 改動 routing/auth 影響現有 endpoint | 可能讓既有公開/私有 API 權限錯置 | 每次只加新的 public path，不刪除既有規則；補測試 |
| Phase 4 資料提供者串接牽涉多個模組 | blast radius 大 | 使用 interface 注入，保持現有 stub 作為 fallback |
| Phase 6 bcrypt 改動影響既有測試帳號 | 舊測試帳號無法登入 | 提供 migration 或允許舊 hash 在 dev mode 驗證 |
| cmd/atlas flaky test (`TestAPIModeAdminReloadConfigRejectsGet`) | 已存在，會在 full test 時偶發失敗 | 獨立運行時通過即可；如有餘力再修 flaky |

---

## 分支策略

每個 Phase 從 `main` 切出獨立分支（若後續 Phase 強烈依賴前一 Phase，則可基於前一 Phase 分支開出 stacked PR，但預設採用 sequential merge 以降低審查複雜度）。

```
main
├── feat/wave11-phase1-auth-routing        → PR #976
├── feat/wave11-phase2-mcp-tools           → PR #977
├── feat/wave11-phase3-frontend-contracts  → PR #978
├── feat/wave11-phase4-data-providers      → PR #979
├── feat/wave11-phase5-docs-alignment      → PR #980
└── feat/wave11-phase6-security-tests       → PR #981
```

Phase 7 在所有 Phase 1–6 合併回 `main` 後執行，不再開新 PR，只做驗收與必要 hotfix。
