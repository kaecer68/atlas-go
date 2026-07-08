# AGENTS.md — internal/subscription

**成熟度**: experimental (X-tier, Wave 11)
**模組職責**: 使用者註冊/登入 + JWT 簽發/驗證 + tier 解析

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `Store` | `store.go` | 持久化 store (`Register(email, passwordHash)` / `GetByEmail`) — **呼叫端負責 hash**（見 L20 認證鏈）,store 只接收已 hash 過的字串 |
| `Tier` | `types.go` | 列舉：`TierFree` / `TierRegistered` / `TierPremium` |
| `EffectiveTier` | `types.go` | 從 user 物件解析 tier（Premium 試用期、Trial 處理） |
| `ExtractToken` / `Verify` | `auth.go` | HTTP 認證：JWT cookie/Authorization header 解析 |

## 認證鏈

```
呼叫端: 收到密碼 → 用 argon2/bcrypt hash → Register(email, passwordHash)
     ↓
Store 端:  收到 passwordHash → bcrypt 二次 hash (per-user salt) → 寫入磁碟
     ↓
JWT 簽發 (簽 secret + exp)
     ↓
客戶端後續請求 → ExtractToken(r) → Verify(token) → tier 解析
     ↓
     → Free
     → Registered
     → Premium (trial / monthly / annual / etc.)
```

## 與 P0-2 的關係

P0-2（commit `7abe4c2f`）修復了 `recommender` 的 X-User-Email fallback **並未動到本模組**。X-User-Email 偽造可發生在：

1. **Recommender handler**（已修）— `ATLAS_DEV_MODE` ENV flag 控制 fallback
2. **任何 call site** 若直接呼叫 `subscription.GetByEmail` 從 header 取 user，仍可能被偽造

**整合檢查點**：所有 call site 若用 `r.Header.Get("X-User-Email")` 模式，都應走 `EffectiveTier` helper 而非直接 store lookup。

## 已知陷阱

| 陷阱 | 說明 |
|------|------|
| **JWT secret 環境變數** | production 部署必須設置 `ATLAS_JWT_SECRET`；dev mode 無設定時降級為 unsign token（不安全）。 |
| **Trial 試用期** | `Premium` 有試用期邏輯，時間到期自動降級。`EffectiveTier` 處理這層。 |
| **無 rate limit** | `Register` endpoint 沒有爆破防護。生產部署需 reverse-proxy 層補。 |

## 與其他模組整合

- `internal/recommender/handler.go` — 使用 `subStore.GetByEmail` + `EffectiveTier` 推導 tier
- `cmd/atlas/main.go:591` — `subHandler.RegisterRoutes(mux)`
- `internal/atlas-mcp/server/auth.go` — MCP JSON-RPC 也用同樣的 JWT chain

## 測試

- `handler_test.go` 既有 Register/Login/ExtractToken/Verify tests
- Trial expiry 測試在 `types_test.go`（如存在）
- Mock store 用 `t.Setenv` + tmpDir 模式
