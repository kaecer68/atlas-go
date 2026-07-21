# Guest Mode — Default-Off User Authentication Bypass

> **Audience**: developers touching `/api/auth/*`, `/api/user/*`, `internal/subscription/*`, or any module that needs to know "is this request authenticated?".
> **Implementation**: `cmd/atlas/main.go`, `internal/subscription/auth.go`, `internal/subscription/handler.go`, `shared_web/static/js/services/auth.js`
> **Tests**: `internal/subscription/handler_test.go` (`TestAuthGuestMode*`)
> **Related**: [`tier-boundary.md`](../operations/tier-boundary.md), [`traps.md`](../reference/traps.md), [`.claude/skills/atlas-guest-mode/SKILL.md`](../../.claude/skills/atlas-guest-mode/SKILL.md)

> 對應 PR #1084。  
> 本文件定義 atlas-go **pre-commercialisation** 階段的「匿名訪客 = free tier」模式，並列出**未來商業化時的單一翻轉點**以及**禁止事項**，避免 AI agent 重複造輪子或引入並行的 user-auth 機制。

---

## 1. 設計意圖

atlas-go 在商業化前需要讓**任何訪客**都能直接使用 client_web 的所有功能，而**不必註冊 / 登入**。同時：

1. **所有現有程式碼必須保留** — 未來商業化時只翻兩個 flag，零架構改動。
2. **不引入並行 user-auth 機制** — 沒有「dev token」「magic header」「auto-register」等 bypass 路徑。
3. **Cookie bug 一併修復** — `POST /api/auth/register` 原本只回 JWT 在 JSON body，沒設 HttpOnly cookie，導致使用者強制刷新就被踢出。這個 bug 在引入 guest mode 的同 PR 修掉，避免商業化時還要回頭 debug。

## 2. 雙重 Flag 架構

atlas-go 用**兩個獨立 flag** 分別控制 backend 與 frontend：

| Flag | 位置 | 預設 | 翻轉意義 |
|------|------|------|---------|
| `ATLAS_REQUIRE_USER_AUTH` | `cmd/atlas/main.go` 讀取的 env var | `false`（guest 模式開啟）| `true` = backend JWT middleware 強制驗證 |
| `GUEST_MODE` | `shared_web/static/js/services/auth.js` top-level const | `true` | `false` = 前端恢復正常 login / register flow |

兩個 flag **必須同步翻轉** — 只翻 backend 不翻前端，會出現「前端以為有 session，但下一個 API call 收到 401」的鬼打牆狀況。

## 3. Backend 行為

`AuthMiddleware.Wrap`（`internal/subscription/auth.go`）行為決策樹：

```
request → AuthMiddleware.Wrap
  ├── token != "" + JWT.Verify ok        → 注入真實 claims
  ├── token != "" + JWT.Verify fail
  │     ├── allowGuest=true              → 注入 guest claims（避免 server 重啟時 secret 旋轉踢出所有 demo user）
  │     └── allowGuest=false             → 401
  └── token == ""
        ├── allowGuest=true              → 注入 guest claims
        └── allowGuest=false             → 401
```

`guestClaims()` 內容：`UserID=0, Email="", Tier=free`。

`handleProfile` / `handleSubscription` 看到 `claims.Email == ""` 時**不查 store**，直接 synth 一個 free tier response。這避免 guest 模式下去打 `GetByEmail("")` 撞 SQL。

## 4. Frontend 行為

`shared_web/static/js/services/auth.js`：

1. `initAuth()` 在 `GUEST_MODE && !_authValid` 時自動 fallback：`_authValid = true; _claims = {tier: 'free', ...}; _token = 'guest'`。
2. `renderNavState()` 在 `GUEST_MODE` 時把 `#navAccountSection` 整個加上 `.hidden` class（CSS 規則已存在於 `shared_web/static/css/components/utilities.css`）。
3. `/client/login` 與 `/client/register` URL 仍可達（bookmark 友善），但 sidebar 沒有入口。

## 5. 翻轉 SOP（商業化時）

```bash
# 1. Backend env（建議寫進 production .env）
ATLAS_REQUIRE_USER_AUTH=true

# 2. Frontend（單行改動）
# shared_web/static/js/services/auth.js:21
const GUEST_MODE = false;
```

```bash
# 3. 驗證（4 個 curl 必須全綠才能 ship）
# (a) anon 應被擋
curl -i http://localhost:18080/api/user/profile
# → HTTP/1.1 401

# (b) register 應同時回 token + 設 HttpOnly cookie
curl -i -X POST http://localhost:18080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"new@atlas-go.test","password":"x"}'
# → Set-Cookie: token=...; HttpOnly; SameSite=Lax; Max-Age=86400
# → body 含 {"token":"...","user":{...}}

# (c) 帶 cookie 的後續 profile call 應 200
curl -i --cookie "token=<jwt-from-b>" http://localhost:18080/api/user/profile
# → HTTP/1.1 200

# (d) sidebar 應看到「帳戶」section
curl -fsS http://localhost:18080/client/ | grep -c navAccountSection
# → 1
```

## 6. 禁止事項（AI Agent 必讀）

> **新增 user-auth / login wall / tier gate 相關功能前，先讀這節。**

| ❌ 不要做 | 為什麼 | 應該做 |
|----------|-------|-------|
| 從零實作新的 JWT middleware | 已有 `internal/subscription/auth.go` 的 `AuthMiddleware.allowGuest`，加 if-else 路徑即可 | 翻 `ATLAS_REQUIRE_USER_AUTH=true` |
| 寫「dev-mode token」或 magic header bypass | 平行 bypass 路徑會跟 guest mode 衝突，導致測試結果不一致 | 用 `ATLAS_REQUIRE_USER_AUTH=false` 走 guest 模式 |
| Hardcode token 跳過 auth | 永久寫死無法商業化 | 翻 `GUEST_MODE` flag |
| 修改 `recommender/handler.go` 的 tier fallback 邏輯 | `recommender` 已有 `TierFree` fallback（line 108），跟 guest 模式天然相容 | 不動 |
| 在 `handleProfile` 直接呼叫 `store.GetByEmail("")` | guest email 為空字串，會撞 SQL | 維持現有 `claims.Email == ""` 短路 synth response |
| 引入 OAuth / SSO / Magic Link | 商業化議題，尚未啟動；屬於未來獨立 spec | 等商業化 PR 開啟後再開新 spec |
| 刪除 `/api/auth/*` 或 `/api/user/*` 端點 | 已驗證程式碼保留，翻 flag 即可恢復 | 不要動 |

## 7. 對 AI agent 的明確指示

如果你（AI coding agent）接到任務包含以下任一條件，**先讀本文件再動手**：

- 任務描述含「加 SSO」「加 OAuth」「加登入」「加 login」「加認證」「加 auth」「加 user」「加 tier」「加 member」「加帳號」「升級 free tier」「要使用者付費」「商業化」
- 修改 `internal/subscription/*` 任何檔案
- 修改 `shared_web/static/js/services/auth.js`
- 修改 `client_web/static/index.html`（可能影響 nav section）
- 修改 `cmd/atlas/main.go` 的 subscription handler 段落

完成任務後的 closure：確保 `go test ./internal/subscription/...` 通過、`golangci-lint run ./internal/subscription/...` 0 issues、不要新增 `//nolint:gosec` 註解。

## 8. 變更歷史

| 版本 | PR | 日期 | 變更 |
|------|----|------|------|
| 1.0 | #1084 | 2026-07-12 | 初版：guest mode default-off + register cookie 修復 |

---

**驗證清單（修改 guest mode 相關程式碼後必跑）**：

```bash
gofmt -l internal/subscription/ cmd/atlas/main.go
go vet ./internal/subscription/...
go test ./internal/subscription/...
golangci-lint run --timeout=5m ./internal/subscription/...
```
