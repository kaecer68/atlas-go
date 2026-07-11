---
name: atlas-guest-mode
description: "Canonical pattern for atlas-go's user authentication bypass in pre-commercialisation deploys. Use when touching /api/auth/*, /api/user/*, internal/subscription/*, frontend auth.js, or any task involving login, SSO, OAuth, tier gate, user signup, JWT, session, cookie, commercialize auth, or 'add login wall'. Prevents agents from re-implementing parallel user-auth middleware."
version: "1.0"
category: "security"
auto_load: true
load_policy: "auto"
created: "2026-07-12"
updated: "2026-07-12"
target_audience: "developer"
---

# Atlas Guest Mode — Canonical User-Auth Bypass Pattern

## 描述（Description）

atlas-go 在 pre-commercialisation 階段用 **guest mode** 讓匿名訪客自動以 `TierFree` 身份存取 `client_web` 所有功能。這個模式由 **兩個獨立 flag** 控制：

- Backend：`ATLAS_REQUIRE_USER_AUTH`（env var，預設 `false`）
- Frontend：`GUEST_MODE`（`shared_web/static/js/services/auth.js` top-level const，預設 `true`）

未來商業化時**只翻這兩個 flag**，零架構改動。

**這個技能存在的目的**：避免 AI agent 在 user-auth / login / tier-gate 相關任務中**重複造輪子**或引入並行的 bypass 機制（例如 dev-mode token、magic header、新 middleware）。所有 user-auth 變更都應該沿用 guest mode 的 canonical pattern。

## 何時觸發（When to Trigger）

當任務描述包含以下任一條件時，**必載**此技能：

- 中文：「加 SSO」「加 OAuth」「加登入」「加登入驗證」「加認證」「加 auth」「加使用者」「加 user」「加 tier」「加 member」「加帳號」「升級 free tier」「要使用者付費」「商業化」「加付費牆」「付費升級」
- 英文："add SSO", "add OAuth", "add login", "add login wall", "add auth", "add authentication", "add signup", "add tier gate", "add paywall", "commercialize", "monetize", "user session", "JWT", "session cookie", "user middleware"
- 修改檔案：`internal/subscription/*`、`shared_web/static/js/services/auth.js`、`client_web/static/index.html` 的 sidebar section、`cmd/atlas/main.go` 的 subscription handler 段落
- 修改任何呼叫 `subscription.ExtractToken`、`subscription.GetClaims`、`NewAuthMiddleware` 的程式碼

**未觸發此技能的場景**（不要 force load）：純 macro/strategy/risk 計算、recommender tier 邏輯（已有 `TierFree` fallback，與 guest mode 天然相容，不需動）、資料 ingest pipeline。

## 核心概念（Core Concepts）

### 雙 Flag 設計

| Flag | 位置 | 預設 | 控制對象 |
|------|------|------|---------|
| `ATLAS_REQUIRE_USER_AUTH` | `cmd/atlas/main.go` env var | `false`（guest 開啟）| backend JWT 強制驗證 |
| `GUEST_MODE` | `shared_web/static/js/services/auth.js:21` | `true` | frontend login UI + auto-promote |

**必須同步翻轉**。只翻一個會出現「前端以為登入成功但下個 API call 收 401」的鬼打牆。

### 行為決策樹（Backend）

`AuthMiddleware.Wrap` 在 `internal/subscription/auth.go`：

```
request → AuthMiddleware.Wrap
  ├── token 有效              → 注入真實 claims
  ├── token 無效 / 過期
  │     ├── allowGuest=true   → 注入 guest claims（避免 secret 旋轉踢出 demo user）
  │     └── allowGuest=false  → 401
  └── 沒有 token
        ├── allowGuest=true   → 注入 guest claims
        └── allowGuest=false  → 401
```

`guestClaims()`：`UserID=0, Email="", Tier=free`。

### Guest Path 的 Handler 短路

`handleProfile` / `handleSubscription` 看到 `claims.Email == ""` 時**不查 store**，直接 synth free tier response。這避免 `store.GetByEmail("")` 撞 SQL。

### 既有 Recommender 相容性

`internal/recommender/handler.go:108` 已有 `tier := subscription.TierFree` fallback（`if token := ExtractToken(r); token != ""` 失敗時自動 fallback），跟 guest mode 天然相容 — 不需改。

### 已修復的 Cookie Bug

PR #1084 同時修了 `POST /api/auth/register` 的 cookie bug：原本只回 JWT 在 JSON body，沒設 HttpOnly cookie。修法是抽 `setAuthCookie(w, token)` helper，`handleLogin` 與 `handleRegister` 共用。

## 數據來源（Data Sources）

| 概念 | 檔案 | 說明 |
|------|------|------|
| Backend flag | `cmd/atlas/main.go:625-630` | `config.GetSecret("ATLAS_REQUIRE_USER_AUTH")` |
| Middleware | `internal/subscription/auth.go` | `AuthMiddleware.allowGuest` + `guestClaims()` + `Wrap()` |
| Handler 路由 | `internal/subscription/handler.go:30-41` | `RegisterRoutes(mux, allowGuest)` |
| Guest path short-circuit | `internal/subscription/handler.go:132-141, 162-171` | `claims.Email == ""` 短路 |
| Cookie helper | `internal/subscription/handler.go:46-58` | `setAuthCookie(w, token)` |
| Frontend flag | `shared_web/static/js/services/auth.js:21` | `const GUEST_MODE = true` |
| Init fallback | `shared_web/static/js/services/auth.js:155-165` | `initAuth()` auto-promote |
| Nav hide | `shared_web/static/js/services/auth.js:195-205` | `renderNavState()` 加 `#navAccountSection.hidden` |
| Nav target | `client_web/static/index.html:117` | `<div id="navAccountSection">` |
| Tests | `internal/subscription/handler_test.go` | `TestAuthGuestModeAllowsAnon`、`TestAuthGuestModeDemotesInvalidToken` |

## 使用範例（Usage Examples）

### 範例 1：商業化翻轉
```bash
# 1. env (建議寫進 production .env)
ATLAS_REQUIRE_USER_AUTH=true

# 2. frontend (一行改動)
# shared_web/static/js/services/auth.js:21
const GUEST_MODE = false;

# 3. 驗證（4 個 curl）
curl -i http://localhost:18080/api/user/profile   # → 401
curl -i -X POST http://localhost:18080/api/auth/register \
  -H "Content-Type: application/json" -d '{"email":"x@y.z","password":"p"}'  # → 201 + Set-Cookie
curl -i --cookie "token=<jwt>" http://localhost:18080/api/user/profile  # → 200
curl -fsS http://localhost:18080/client/ | grep -c navAccountSection      # → 1
```

### 範例 2：寫新 handler 需要用 claims
```go
// ✅ 正確：trust AuthMiddleware 已注入的 claims
func (h *MyHandler) handle(w http.ResponseWriter, r *http.Request) {
    claims := subscription.GetClaims(r)
    if claims == nil {
        // 嚴格模式下永遠不會走到這；guest 模式下會拿到 guest claims
        http.Error(w, "no claims", 500)
        return
    }
    if claims.Email == "" {
        // guest path — 不要查 store
        writeJSON(w, 200, guestResponse())
        return
    }
    // 真實 user path
    user, _ := h.store.GetByEmail(claims.Email)
    ...
}
```

## 禁止事項（Forbidden Patterns）

| ❌ 不要做 | 為什麼 |
|----------|-------|
| 從零實作新的 JWT middleware | 已有 `AuthMiddleware.allowGuest`，加 path 即足 |
| 寫「dev-mode token」/「magic header bypass」 | 平行路徑會跟 guest mode 衝突、測試結果不一致 |
| Hardcode token 跳過 auth | 永久寫死無法商業化 |
| 修改 recommender tier fallback（`handler.go:108`） | 已有 `TierFree` fallback 跟 guest 模式天然相容 |
| `store.GetByEmail("")` 在 guest path | 撞 SQL；用 `claims.Email == ""` 短路 synth |
| 引入 OAuth/SSO/Magic Link | 屬於未來獨立 spec；現在做是 premature |
| 刪除 `/api/auth/*` 或 `/api/user/*` 端點 | 程式碼已驗證保留，翻 flag 即可恢復 |

## 驗證規則（Validation Rules）

修改 guest-mode 相關程式碼後必跑：

- [ ] `gofmt -l internal/subscription/ cmd/atlas/main.go`（無輸出）
- [ ] `go vet ./internal/subscription/...`（無錯誤）
- [ ] `go test ./internal/subscription/...`（全 pass）
- [ ] `golangci-lint run --timeout=5m ./internal/subscription/...`（0 issues）
- [ ] `npm run build` 在 `client_web/` 與 `admin_web/`（無 esbuild error）
- [ ] 4 個 curl 驗證（見上面「範例 1」）

## 相關技能（Related Skills）

| 技能 | 關聯 |
|------|------|
| `atlas-pre-change-protocol` | 修改前必用；這個 skill 在 pre-change Step 0 overlap detection 階段就會引導到本技能 |
| `atlas-monitoring-observability` | `/api/llm/health` 是另一個 auth-free path 議題（PR #931 教訓） |
| `atlas-fubon-supervisor-invariants` | 同樣是「不要造輪子，先讀既有 pattern」的範例（ProcessManager F1-F9） |

## 變更紀錄（Changelog）

| 版本 | 日期 | 變更 |
|------|------|------|
| 1.0 | 2026-07-12 | 初版，對應 PR #1084 |
