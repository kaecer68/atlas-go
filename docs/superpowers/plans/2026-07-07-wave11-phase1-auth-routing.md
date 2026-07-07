# Phase 1: Auth / Routing / Public Path 止血

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 讓 Wave 11 新增的 `/api/capital-flow`、`/api/events`、`/api/recommendations`、`/api/reports` 在 production（`ATLAS_API_KEY` 已設定）時能被 browser 公開存取；修復 `/api/user/profile` 與前端的欄位錯配，讓重新整理後仍保持登入；讓 login/register/premium page-shell 的 `init()` 真正被呼叫；新增 `POST /api/auth/logout` 與側邊欄登出入口。

**Architecture:** 只改變「哪些 path 繞過 API-key middleware」與「subscription 模組對外合約」，不改變業務邏輯。前端 auth service 與後端 profile 回傳格式對齊後，所有後續 Phase 才能穩定驗證。

**Tech Stack:** Go 1.26, vanilla JS, JWT cookie, `net/http` ServeMux。

## Global Constraints

- 所有 Go 檔案必須通過 `gofmt`、`go vet`、`golangci-lint`、`staticcheck`、`gosec`。
- 每個 task 結束後須執行相關測試並確認通過。
- 禁止修改現有測試邏輯來掩蓋問題；只修正因介面變更導致的編譯錯誤。
- 本 Phase 結束時須 commit → push → 開 PR。

---

## Task 1: 開啟 Phase 1 分支並建立基線

**Files:**
- 工作區根目錄

**Interfaces:**
- Consumes: `main` branch at `7740ef9`
- Produces: 本地分支 `feat/wave11-phase1-auth-routing`

- [ ] **Step 1: 從 main 切出 Phase 1 分支**

```bash
git checkout main
git pull origin main
git checkout -b feat/wave11-phase1-auth-routing
```

- [ ] **Step 2: 確認工作區乾淨且測試基線可預期**

```bash
git status
# 期望：On branch feat/wave11-phase1-auth-routing, nothing to commit

go test ./internal/capitalflow/... ./internal/eventdriven/... ./internal/recommender/... ./internal/subscription/... ./cmd/atlas-mcp/server/...
# 期望：全部 PASS
```

- [ ] **Step 3: Commit 標記（可選）**

```bash
git commit --allow-empty -m "chore(phase1): start auth/routing/public-path fixes"
```

---

## Task 2: 把 Wave 11 API 加入 public path 白名單

**Files:**
- Modify: `cmd/atlas/main.go:103-134`
- Test: `cmd/atlas/main_test.go`

**Interfaces:**
- Consumes: HTTP request path string
- Produces: `isPublicPath(p string) bool` 對新 API 回傳 true

- [ ] **Step 1: 在 `cmd/atlas/main.go` 的 `isPublicPath` 中加入新 API**

在 `case p == "/api/auth" || strings.HasPrefix(p, "/api/auth/"):` 之前（或之後）加入：

```go
	case p == "/api/capital-flow" || strings.HasPrefix(p, "/api/capital-flow/"):
		return true
	case p == "/api/events" || strings.HasPrefix(p, "/api/events/"):
		return true
	case p == "/api/recommendations" || strings.HasPrefix(p, "/api/recommendations/"):
		return true
	case p == "/api/reports" || strings.HasPrefix(p, "/api/reports/"):
		return true
```

完整 `isPublicPath` 應如下（僅示意新增區段，保留既有 case）：

```go
func isPublicPath(p string) bool {
	switch {
	case p == "/" || p == "/health" || p == "/ready" || p == "/metrics":
		return true
	case p == "/api/llm/health":
		return true
	case p == "/api/dashboard" || strings.HasPrefix(p, "/api/dashboard/"):
		return true
	case p == "/api/taiwan" || strings.HasPrefix(p, "/api/taiwan/"):
		return true
	case p == "/api/narrative" || strings.HasPrefix(p, "/api/narrative/"):
		return true
	case p == "/api/macro" || strings.HasPrefix(p, "/api/macro/"):
		return true
	case p == "/api/alerts" || strings.HasPrefix(p, "/api/alerts/"):
		return true
	case p == "/api/synergy" || strings.HasPrefix(p, "/api/synergy/"):
		return true
	case p == "/api/cross-market" || strings.HasPrefix(p, "/api/cross-market/"):
		return true
	case p == "/api/capital-flow" || strings.HasPrefix(p, "/api/capital-flow/"):
		return true
	case p == "/api/events" || strings.HasPrefix(p, "/api/events/"):
		return true
	case p == "/api/recommendations" || strings.HasPrefix(p, "/api/recommendations/"):
		return true
	case p == "/api/reports" || strings.HasPrefix(p, "/api/reports/"):
		return true
	case p == "/api/auth" || strings.HasPrefix(p, "/api/auth/"):
		return true
	case p == "/api/user" || strings.HasPrefix(p, "/api/user/"):
		return true
	case p == "/admin" || strings.HasPrefix(p, "/admin/"):
		return true
	case p == "/client" || strings.HasPrefix(p, "/client/"):
		return true
	default:
		return false
	}
}
```

- [ ] **Step 2: 在 `internal/monitoring/api/shared/handler.go` 的 `authFreePrefixPaths` 加入使用 `shared.Get`/`shared.Adapt` 的前綴**

新增：

```go
var authFreePrefixPaths = []string{
	"/admin/",
	"/client/",
	"/api/dashboard/",
	"/api/taiwan/",
	"/api/narrative/",
	"/api/macro/",
	"/api/alerts/",
	"/api/synergy/",
	"/api/cross-market/",
	"/api/capital-flow/",
	"/api/events/",
}
```

注意：`/api/recommendations` 與 `/api/reports/*` 使用 `mux.HandleFunc` 直接註冊，不經過 `shared.Adapt`，因此不需要在 `authFreePrefixPaths` 中加入。`/api/auth` 與 `/api/user` 也有自己的 JWT middleware，同樣不需要加入。

- [ ] **Step 3: 新增/更新 `cmd/atlas/main_test.go` 的 public path 測試**

如果 `TestIsPublicPath` 已存在，在其 table 中加入：

```go
{path: "/api/capital-flow/daily", want: true},
{path: "/api/capital-flow/summary", want: true},
{path: "/api/events/calendar", want: true},
{path: "/api/events/prediction", want: true},
{path: "/api/recommendations", want: true},
{path: "/api/reports/latest", want: true},
{path: "/api/reports/archive", want: true},
```

如果尚未存在，新增：

```go
func TestIsPublicPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/", true},
		{"/health", true},
		{"/api/dashboard/metrics", true},
		{"/api/capital-flow/daily", true},
		{"/api/capital-flow/summary", true},
		{"/api/events/calendar", true},
		{"/api/events/prediction", true},
		{"/api/recommendations", true},
		{"/api/reports/latest", true},
		{"/api/reports/archive", true},
		{"/api/auth/login", true},
		{"/api/user/profile", true},
		{"/api/private/anything", false},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			if got := isPublicPath(c.path); got != c.want {
				t.Errorf("isPublicPath(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 4: 執行測試**

```bash
go test ./cmd/atlas/... -run TestIsPublicPath -v
# 期望：PASS
```

- [ ] **Step 5: Commit**

```bash
git add cmd/atlas/main.go internal/monitoring/api/shared/handler.go cmd/atlas/main_test.go
git commit -m "fix(routing): add Wave 11 APIs to public path whitelist

- Add /api/capital-flow, /api/events, /api/recommendations, /api/reports
  to isPublicPath so browser can reach them with ATLAS_API_KEY set.
- Add /api/capital-flow/ and /api/events/ to authFreePrefixPaths
  because they use shared.Get/Adapt.
- Add TestIsPublicPath coverage for new paths."
```

---

## Task 3: 修復 `/api/user/profile` 與前端 auth service 的欄位錯配

**Files:**
- Modify: `internal/subscription/handler.go:100-115`
- Modify: `shared_web/static/js/services/auth.js:66-84`

**Interfaces:**
- Consumes: JWT claims
- Produces: `/api/user/profile` 回傳 `email` + `tier`/`effective_tier`

- [ ] **Step 1: 修改 `internal/subscription/handler.go` 的 `handleProfile`**

將：

```go
	writeJSON(w, http.StatusOK, map[string]any{
		"user":           user,
		"effective_tier": user.EffectiveTier(),
	})
```

改為：

```go
	writeJSON(w, http.StatusOK, map[string]any{
		"user":           user,
		"email":          user.Email,
		"tier":           user.Tier,
		"effective_tier": user.EffectiveTier(),
		"trial_end":      user.TrialEnd,
	})
```

- [ ] **Step 2: 修改 `shared_web/static/js/services/auth.js` 的 `isLoggedIn`**

將：

```js
    if (profile && profile.email) {
      _authValid = true;
      if (profile.tier) {
        if (!_claims) _claims = {};
        _claims.tier = profile.tier;
      }
    }
```

改為：

```js
    const email = profile.email || (profile.user && profile.user.email);
    const tier = profile.effective_tier || profile.tier || (profile.user && profile.user.tier);
    if (profile && email) {
      _authValid = true;
      if (tier) {
        if (!_claims) _claims = {};
        _claims.tier = tier;
      }
    }
```

- [ ] **Step 3: 執行相關測試**

```bash
go test ./internal/subscription/... -v
# 期望：PASS

# 如果 subscription handler_test.go 有針對 profile response 的斷言，需要同步更新
```

- [ ] **Step 4: Commit**

```bash
git add internal/subscription/handler.go shared_web/static/js/services/auth.js
git commit -m "fix(auth): align /api/user/profile response with frontend auth service

Backend now returns email/tier/effective_tier/trial_end alongside user
object. Frontend reads effective_tier for tier and falls back to user.*."
```

---

## Task 4: 新增 `POST /api/auth/logout` 端點

**Files:**
- Modify: `internal/subscription/handler.go:30-37`
- Modify: `internal/subscription/handler.go:98-133`
- Test: `internal/subscription/handler_test.go`

**Interfaces:**
- Consumes: HTTP POST with `token` cookie
- Produces: 200 OK + cleared `token` cookie

- [ ] **Step 1: 在 `RegisterRoutes` 中加入 logout 路由**

```go
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mid := NewAuthMiddleware(h.jwt)

	mux.HandleFunc("POST /api/auth/register", h.handleRegister)
	mux.HandleFunc("POST /api/auth/login", h.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", h.handleLogout)
	mux.Handle("GET /api/user/profile", mid.Wrap(http.HandlerFunc(h.handleProfile)))
	mux.Handle("GET /api/user/subscription", mid.Wrap(http.HandlerFunc(h.handleSubscription)))
}
```

- [ ] **Step 2: 新增 `handleLogout` 方法**

在 `handleLogin` 之後加入：

```go
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	//nolint:gosec // local dev without HTTPS; Secure flag is environment-dependent
	c := &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
	http.SetCookie(w, c)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}
```

- [ ] **Step 3: 新增測試**

在 `internal/subscription/handler_test.go` 加入：

```go
func TestLogout(t *testing.T) {
	h := NewHandler(NewTestStore(), NewJWTManager("test-secret-min-32-characters-long"))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	cookie := rr.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "token=;") && !strings.Contains(cookie, "Max-Age=0") && !strings.Contains(cookie, "Expires=Thu, 01 Jan 1970") {
		t.Fatalf("expected token cookie to be cleared, got %q", cookie)
	}
}
```

> 注意：如果 `NewTestStore` 不存在，請檢視現有 handler_test.go 的 helper 名稱並調整。

- [ ] **Step 4: 執行測試**

```bash
go test ./internal/subscription/... -run TestLogout -v
# 期望：PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/subscription/handler.go internal/subscription/handler_test.go
git commit -m "feat(auth): add POST /api/auth/logout endpoint

Clears the HttpOnly token cookie and returns {status: logged_out}."
```

---

## Task 5: 更新前端 `auth.js` 的 logout 以呼叫後端

**Files:**
- Modify: `shared_web/static/js/services/auth.js:52-60`

**Interfaces:**
- Consumes: `POST /api/auth/logout`
- Produces: 清除 local state 與 server cookie

- [ ] **Step 1: 修改 `logout` 函式**

將：

```js
export function logout() {
  _token = null;
  _claims = null;
  _authValid = false;
  _authChecked = true;
}
```

改為：

```js
export async function logout() {
  try {
    await postJSON('/api/auth/logout', {});
  } catch (e) {
    // Best-effort server cookie clear; local state is cleared regardless.
  }
  _token = null;
  _claims = null;
  _authValid = false;
  _authChecked = true;
}
```

- [ ] **Step 2: Commit**

```bash
git add shared_web/static/js/services/auth.js
git commit -m "fix(auth): call backend logout to clear HttpOnly cookie

logout() now POSTs to /api/auth/logout before clearing local state."
```

---

## Task 6: 在側邊欄加入登出按鈕

**Files:**
- Modify: `client_web/static/index.html`
- Modify: `client_web/static/js/event-listeners.js`（或適合綁定 sidebar 事件的檔案）

**Interfaces:**
- Consumes: `logout()` from `auth.js`
- Produces: 點擊「登出」後呼叫 logout 並重導向首頁

- [ ] **Step 1: 在 `client_web/static/index.html` 的 user nav 區塊加入登出按鈕**

找到類似：

```html
<a href="#" class="nav-user hidden" data-page="premium">
  <span class="nav-icon">✨</span>
  <span>升級 Premium</span>
</a>
```

在之後加入：

```html
<button type="button" id="navLogoutBtn" class="nav-user hidden">
  <span class="nav-icon">🚪</span>
  <span>登出</span>
</button>
```

- [ ] **Step 2: 在 `client_web/static/js/event-listeners.js` 綁定事件**

如果該檔案存在且負責 sidebar 事件，加入：

```js
import { logout, renderNavState } from '../services/auth.js';

function initSidebarLogout() {
  const btn = document.getElementById('navLogoutBtn');
  if (!btn) return;
  btn.addEventListener('click', async (e) => {
    e.preventDefault();
    await logout();
    await renderNavState();
    window.location.hash = '#home';
  });
}

// 在現有的 DOMContentLoaded 或 init 函式中呼叫：
initSidebarLogout();
```

如果 `event-listeners.js` 不存在或結構不同，請在 `client_web/static/js/main.js` 的初始化區塊中加入綁定。

- [ ] **Step 3: Commit**

```bash
git add client_web/static/index.html client_web/static/js/event-listeners.js
git commit -m "feat(client): add sidebar logout button

Calls auth.logout() and redirects to #home after logout."
```

---

## Task 7: 修復 page-shell `init()` 未被呼叫的問題

**Files:**
- Modify: `client_web/static/js/main.js:42-57`

**Interfaces:**
- Consumes: page-shell module `{ template, init }`
- Produces: 載入 template 後執行 `init()`

- [ ] **Step 1: 找到 `_ensureShellLoaded` 函式**

應類似：

```js
async function _ensureShellLoaded(name) {
  if (_loadedShells[name]) return;
  const mod = await _loadPageShell(name);
  const el = document.getElementById('page-shell-container');
  if (el && mod.template) {
    el.innerHTML = mod.template;
  }
  _loadedShells[name] = true;
}
```

- [ ] **Step 2: 在 `innerHTML = mod.template` 後呼叫 `mod.init()`**

改為：

```js
async function _ensureShellLoaded(name) {
  if (_loadedShells[name]) return;
  const mod = await _loadPageShell(name);
  const el = document.getElementById('page-shell-container');
  if (el && mod.template) {
    el.innerHTML = mod.template;
  }
  if (typeof mod.init === 'function') {
    await mod.init();
  }
  _loadedShells[name] = true;
}
```

- [ ] **Step 3: Commit**

```bash
git add client_web/static/js/main.js
git commit -m "fix(client): invoke page-shell init() after template injection

login/register/premium/mcp/404 page-shells export init() but it was
never called, leaving event listeners and state uninitialized."
```

---

## Task 8: 本地驗證與 Phase 1 收尾

**Files:**
- 全部 Phase 1 修改過的檔案

- [ ] **Step 1: 執行 Go 測試**

```bash
go test ./cmd/atlas/... ./internal/subscription/... ./internal/monitoring/api/shared/... -v
# 期望：相關測試 PASS
```

- [ ] **Step 2: 執行前端 build**

```bash
cd client_web && npm run build
cd ../admin_web && npm run build
# 期望：兩者都成功
```

- [ ] **Step 3: 執行靜態檢查**

```bash
gofmt -w cmd/atlas/main.go cmd/atlas/main_test.go internal/monitoring/api/shared/handler.go internal/subscription/handler.go internal/subscription/handler_test.go
go vet ./cmd/atlas/... ./internal/subscription/... ./internal/monitoring/api/shared/...
# 期望：無輸出（通過）
```

- [ ] **Step 4: 預覽 diff**

```bash
git diff --stat main
# 期望：只包含 Phase 1 相關檔案
```

- [ ] **Step 5: Push 並開 PR**

```bash
git push -u origin feat/wave11-phase1-auth-routing
```

開 PR 時標題與描述：

```
Title: fix(auth/routing): Wave 11 public path whitelist, profile contract, logout, page-shell init

Body:
- Add /api/capital-flow, /api/events, /api/recommendations, /api/reports
  to isPublicPath and /api/capital-flow/, /api/events/ to authFreePrefixPaths.
- Align /api/user/profile response with frontend auth service
  (email/tier/effective_tier/trial_end).
- Add POST /api/auth/logout and wire it to the sidebar logout button.
- Invoke page-shell init() after template injection.
- Add/update tests for public paths and logout.

Closes Phase 1 of Wave 11 cleanup plan.
```

- [ ] **Step 6: 標記 Phase 1 完成**

在 `docs/superpowers/plans/2026-07-07-wave11-cleanup-master.md` 的 Phase 1 checklist 手動打勾，或等 PR 合併後統一更新。

---

## 風險與注意事項

1. `cmd/atlas/main_test.go` 可能已經有 `TestIsPublicPath` 或其他測試；若新增測試導致命名衝突，請合併而非覆蓋。
2. `client_web/static/js/event-listeners.js` 若不存在，請把 logout 綁定放在 `client_web/static/js/main.js` 中適當的初始化位置。
3. `internal/subscription/handler_test.go` 的 helper 名稱（如 `NewTestStore`）請以實際檔案為準；若不存在，可直接在測試中建立 `NewStore(":memory:")` 或等效 store。
4. 本 Phase **不**處理 bcrypt、rate limiting、Secure cookie 等安全強化，留在 Phase 6。
5. 本 Phase **不**處理 MCP tool 新增，留在 Phase 2。
