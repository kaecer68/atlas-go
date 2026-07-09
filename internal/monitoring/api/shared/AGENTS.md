# AGENTS.md — internal/monitoring/api/shared

本目錄是 `monitoring/api` 的共用 HTTP 中介層輔助套件,被 `stocktools` / `subscription` / `crossmarket` 等多個 module 透過 `shared.Adapt` / `shared.Get` / `shared.Post` 引用。

---

## 檔案清單

| 檔案 | 角色 |
|------|------|
| `handler.go` | `Handler` 型別 + `Adapt/Get/Post` 介面卡 + `AuthMiddleware`(API-key 驗證)+ `RequireAdmin`(admin key 驗證)+ `authFreeExactPaths` / `authFreePrefixPaths` 白名單 |
| `respond.go` | `WriteJSON` / `WriteJSONError` 統一 JSON response writer(取代 73 個重複 call site) |
| `paths.go` | 共用路徑常數與 helper |
| `handler_test.go` | AuthMiddleware / Adapt / Get 測試 |
| `handler_extra_test.go` | RequireAdmin / edge case 測試 |
| `respond_test.go` | WriteJSON edge case 測試 |
| `paths_test.go` | paths helper 測試 |
| `jwt_auth.go` | `RequireUserJWT(jwtManager, next)` middleware — 從 `subscription.JWTManager` 重用,驗 HttpOnly cookie 或 `Authorization: Bearer <token>`,用於 user-specific endpoint(per #1055) |

---

## Adapt / Get / Post 介面卡

新的 monitoring API handler 應該使用 `Adapt` / `Get` / `Post`(取代直接 mux.Handle + WriteJSON):

```go
mux.Handle("GET /api/foo", shared.Get(func(r *http.Request) (int, any) {
    return http.StatusOK, map[string]string{"hello": "world"}
}))
```

**優點**:
- 單一 source of truth 寫 JSON(避免 73 個重複 `WriteJSON` 呼叫)
- 內建 1MB body limit 防 large request attack
- 自動 wrap `AuthMiddleware`(API-key based)

---

## AuthMiddleware vs RequireUserJWT

兩個 middleware 共存在 `internal/monitoring/api/shared/`,**用途不同**,勿混用:

| Middleware | 驗證 | 適用 |
|------------|------|------|
| `AuthMiddleware` | `X-API-Key` header(預設)或 `Authorization: Bearer <key>`(env: `ATLAS_API_KEY`) | 公共資料 + 跨服務呼叫(atlas-mcp, internal worker) |
| `RequireUserJWT` | HttpOnly `token` cookie + `Authorization: Bearer <token>`(用 `subscription.JWTManager`) | User-specific 資料(saved searches, watchlists, portfolio) |

**AGENTS.md 高頻陷阱**(per `LLM health 401`):
- `AuthMiddleware` 對 API-key 驗證,**未持有 `ATLAS_API_KEY` 的 dev 環境會 pass through**(line 104-107)
- 任何新增的 `/api/*` 端點若要公開,必須**同步**加到 `cmd/atlas/main.go isPublicPath` + 本檔 `authFreeExactPaths/authFreePrefixPaths`,只改一處會 404/401

---

## JSON 慣例

- 統一 `Content-Type: application/json`
- Error 格式:`{"error": "<message>"}`
- 成功資料可任意 JSON shape(由 handler 決定)
