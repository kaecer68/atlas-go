# 決策：C-02 atlas-jwt-trust — atlas 會員接入 go-member JWT（現況盤查 + 實作清單）

> 狀態：**準備中**（2026-08-19 業主定案 C-02；待平台修正派工完成後再動 code）
> 契約: C-02 atlas-jwt-trust（2026-08-18 業主拍板）
> 範圍：本文件是 **atlas 側**（consumer）實作準備，不包含 go-member 側。

## 契約定案（業主）
1. **auth 源 = go-member**（會員/身份中心，唯一 provider，發 RS256 JWT）。hermes-authz 只管 hermes 平台，不管 atlas 網頁登入。
2. **GUEST_MODE 維持 true**（allowGuest=true，訪客可看免費層）直到 go-member 正式上線 + 有真實付費會員，再切 false（強制登入）。
3. **premium tier 判定**：從 go-member JWT 的 `tier` claim（access token 含 tier + membershipExpiresAt；refresh 時重查 DB 寫最新值）。
4. **§5.5 四問**：
   - UserID: go-member uuid（atlas 只存 sub claim 字串，不重造）
   - 驗證方式: A 用 JWKS（go-member 內建 /.well-known/jwks.json + RSA keypair）
   - tier 映射: registered→basic、premium→pro
   - allowGuest: 維持 true（訪客 free）

## 現況盤查（2026-08-19）
### go-member 側（已就緒）
- `src/app.ts`：`/.well-known/jwks.json` 暴露 RS256 公鑰；jwks_uri + id_token_signing_alg_values_supported:["RS256"]
- `src/utils/crypto.ts`：RS256+JWKS（註明「atlas 透過 /.well-known/jwks.json 拿公鑰驗 token」）；RS256 簽 token
- `src/config/index.ts`：RS256 私鑰（dev 自動生成 ephemeral keypair）
### atlas 側（現況 → 需遷移）
- 目前 `internal/subscription`：**自帶 HS256 JWT**（auth.go `alg HS256` secret）+ **自帶 users DB 表**（store.go）
- routes: GET /api/user/profile / GET /api/user/subscription（atlas 自發 token）
- 前端 `auth.js`：GUEST_MODE=true；login/register 打 atlas /api/auth/login；getTier 讀 /api/user/profile

## C-02 atlas 側實作清單（deferred，等通知再動）
1. **AuthMiddleware 改 JWKS client**（`internal/subscription/auth.go`）：
   - 由「HS256 自帶 secret 驗證」改為「拉 `GO_MEMBER_JWKS_URL` → RS256 驗 → 讀 `tier` claim」
   - JWKS 快取 + 定時刷新（key rotation）
2. **env**：`GO_MEMBER_JWKS_URL=https://<go-member-host>/.well-known/jwks.json`（sTartup 讀）
3. **tier 映射**：go-member `tier` claim → atlas tier（registered→basic / premium→pro）；無/無效 token → free（allowGuest）
4. **前端 auth.js**：login/register 改走 go-member（或 atlas 代理）；getTier 從 go-member token 的 tier claim 解；**GUEST_MODE 維持 true**（訪客 free）
5. **整合測試**：mock go-member 簽 RS256 token → atlas 驗證 → tier 正確；含過期/無效簽名/錯誤 tier 案例
6. **向後相容**：atlas 自己的 admin/ATLAS_API_KEY（monitoring handler，寫入保護）與 user subscription 是兩套，C-02 不動 admin 那道；HS256 自發會員 token 可於遷移後退役
7. **profile/subscription route**：改由 go-member token 解（不再自帶 users 表做會員來源），或保持薄代理

## 依賴/注意
- atlas 容器需能達 go-member（網路/Tailscale/DNS）
- go-member JWKS URL 在 atlas 的 .env/compose prod 設定
- GUEST_MODE 切換 false 的時機：go-member 正式上線 + 有真實付費會員（業主決定）

## 執行時機
待「平台修正（Discord/Telegram 社群反轉）」派工完成後，連同策略閾值（dealer/cb-fx）一起處理（業主指示，避免同時動太多）。


---

## 附：C-02 JWKS 實作清單（atlas 側，2026-08-19 盤查細化）

### 現況（internal/subscription/auth.go）
- `JWTManager`：**HMAC-SHA256（HS256）** 自帶 secret 簽/驗 token（`Generate`/`Verify` 用 `hmac.New(sha256.New, secret)`）
- `TokenClaims{UserID,Email,Tier,Exp}`；`guestClaims()` fallback TierFree（allowGuest）
- `AuthMiddleware`：ExtractToken（Bearer/cookie）→ Verify → 注入 context claims
- handler：GET /api/user/profile / /api/user/subscription 由 atlas 自帶 users 表解

### C-02 改動點（檔案 + 函式）
1. **internal/subscription/auth.go** — `JWTManager` 改造：
   - 新增 JWKS client 欄位：`jwkSet`（RSA 公鑰清單）+ `jwksURL` + `fetchedAt`（TTL 刷新）
   - `NewJWTManager` 改接收 `GO_MEMBER_JWKS_URL`；啟動拉 `/.well-known/jwks.json`（`http.Get`→`json` 解析 n 陣列→`rsa.ParsePKIXPublicKey`）
   - `Verify`：由 HMAC 驗證 → 依 header `alg=RS256` 用 JWKS kid 對應公鑰 `rsa.VerifyPKCS1v15(sha256, sig, payload)` 驗證
   - `tier 映射`：token `tier` claim → atlas tier（`registered→basic`、`premium→pro`、其他→free）；維持 `guestClaims` free fallback
   - `Generate()`（HS256 簽發）：遷移後**不再由 atlas 簽會員 token**（登入改 go-member），可標 deprecated / 保留僅 admin 測試
   - JWKS 快取：拉取結果帶 TTL（如 10min），逾時重新拉以支援 key rotation
2. **internal/config** — 新增 `GO_MemberJwksURL` 設定項，從環境讀
3. **cmd/atlas**（subscription 注入處）— 把 `GO_MEMBER_JWKS_URL` 傳入；`ATLAS_SUBSCRIPTION_JWKS_URL` 或直接 env
4. **handler.go** — `handleProfile` 改從已驗證 claims 組 profile（不再自帶 users 表）；`/api/user/subscription` 改由 JWT claims（tier+membershipExpiresAt）解
5. **前端 shared_web/static/js/services/auth.js**：
   - `login/register`：改走 go-member 登入（取得 go-member token），不再打 atlas `/api/auth/login`（或 atlas 提供代理）
   - `getTier`：解 go-member token 的 tier claim（不再 profile.tier 主來源）
   - `GUEST_MODE` 維持 `true`（allowGuest）
6. **env / compose（iMac prod）**：`~/.config/atlas-go/.env` + docker-compose.prod.yml 加 `GO_MEMBER_JWKS_URL=https://<go-member-host>/.well-known/jwks.json`

### 測試計畫
- **單測 internal/subscription/auth_test.go**：
  - 用 go-member 相同方式（RSA 產生 keypair）簽一個 RS256 token → `Verify` 通過 + tier 映射正確（registered→basic / premium→pro）
  - 無效簽名（換其它金鑰）、過期、錯 kid → reject
  - allowGuest：無/無效 token → fallback guestClaims TierFree
  - JWKS 快取刷新（TTL 後重新拉）
- **整合測試**：mock/輕量 go-member `/.well-known/jwks.json` → atlas AuthMiddleware 驗 → handler 拿對 tier
- **既有 HS256 測試遷移**：改為 RS256 mock 或對應更新

### 執行時機
go-member Phase 2（data-permission-matrix SSOT + iMac 部署穩定）完成後，連同策略閾值（dealer/cb-fx）一起實作。
