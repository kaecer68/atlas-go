# Pre-Deployment Security Audit Checklist

> **用途**：正式部署前（首次 deploy、重大 dependency 升級、季度 baseline 複審）跑的稽核 checklist。
> **覆蓋範圍**：PR #918（含）之後所有安全相關變更。
> **產出**：所有 checkbox ✅ + 簽核紀錄。
> **相關文件**：
> - [`SECURITY.md`](../../SECURITY.md) — 通用安全政策
> - [`internal/apigateway/CONSTITUTION.md`](../../internal/apigateway/CONSTITUTION.md) — 數據源憲法（6 條）
> - [`docs/specs/security-audit-spec.md`](../specs/security-audit.md) — 細項安全檢查
> - [`docs/operations/l2-4-runbook.md`](../operations/l2-4-runbook.md) — L2.4 操作 SOP（live trading 範本）
> - [`docs/operations/mcp-deploy.md`](../operations/mcp-deploy.md) — atlas-mcp 部署細節

---

## Section 1：PR-level 自動化檢查（5 分鐘）

```bash
cd /Users/kaecer/workspace/atlas
git fetch origin main && git pull --ff-only
test -z "$(gofmt -l .)" && go build ./... && go test ./...
go vet ./... && staticcheck ./...
gosec ./...                                                    # CI 已有，但部署前手動跑一次
golangci-lint run --timeout=5m
```

- [ ] 全部 CI check 通過（gofmt / go vet / staticcheck / gosec / golangci-lint）
- [ ] `go test ./...` 全綠（含 coverage ≥ 60%）
- [ ] pre-commit hook 全綠（PR #795 `go generate` drift check）
- [ ] `gitnexus detect_changes` 顯示無 unexpected blast radius

## Section 2：Secret 與 Configuration 稽核（15 分鐘）

```bash
# 2.1 確認 production code 沒有 hardcoded secrets
grep -rn "LLM_DEEPSEEK\|LLM_MINIMAX\|LLM_ANNOTATOR\|FUBON\|FUGLE\|FINMIND\|TEJ_API_KEY\|TWSE_API\|API_KEY.*=.*\"\|SECRET.*=.*\"\|TOKEN.*=.*\"" \
  --include="*.go" . | grep -v "_test.go" | grep -v "configs/allowed_env_vars.md"

# 2.2 確認 os.Getenv 沒有違反 apigateway 憲法第一條（白名單外）
grep -rn "os.Getenv" --include="*.go" . | grep -v "configs/allowed_env_vars.md" \
  | grep -v "internal/config/config.go"                           # config.go 是祖父條款

# 2.3 確認 ATLAS_API_KEY rotation 在 90 天內
stat -f "%Sm %N" ~/.config/atlas-go/.env 2>/dev/null || echo "no .env"

# 2.4 DATABASE_URL sslmode
grep "DATABASE_URL" docker-compose.yml .env 2>/dev/null
# 必須是 sslmode=verify-full（不是 prefer / disable / allow）
```

- [ ] 2.1 無 hardcoded secrets（含 LLM / broker / data provider keys）
- [ ] 2.2 無白名單外的 `os.Getenv`
- [ ] 2.3 `ATLAS_API_KEY` 90 天內有 rotation 紀錄（首次部署：建立 rotation 計畫）
- [ ] 2.4 `ATLAS_ADMIN_KEY` 已設定且與 `ATLAS_API_KEY` 不同
- [ ] 2.5 `DATABASE_URL` sslmode=`verify-full`（首次部署：`sslmode=prefer` 是 dev 預設）
- [ ] 2.6 `LLM_*_API_KEY` 透過 `envOrKeychain`（首次部署：實作 keychain 整合）
- [ ] 2.7 `docker-compose.yml`、`.env` 在 `.gitignore` 且 repo 內無 secrets

## Section 3：Audit Log Integrity（10 分鐘）

```bash
# 3.1 schema version 確認
grep -rn "SchemaVersion" cmd/atlas-mcp/server/audit.go

# 3.2 forward-secure hash chain 確認（PR #918 未含，需評估是否部署前補）
grep -rn "prev_hash\|PrevHash" cmd/atlas-mcp/server/ --include="*.go"

# 3.3 error/unauthorized fsync 確認
grep -A 3 "Status == \"error\"" cmd/atlas-mcp/server/audit.go

# 3.4 retention 政策
grep -rn "retention" cmd/atlas-mcp/server/audit.go
```

- [ ] 3.1 `AuditEntry` schema 強制 `schema_version=2`
- [ ] 3.2 retention ≥ 30 天（建議金管會規範 ≥ 90 天）— 若 < 30 天需有正當理由
- [ ] 3.3 forward-secure hash chain：**首次部署必補**；後續 re-audit 確認
- [ ] 3.4 `f.Sync()` 在 `Status="error" || "unauthorized"` 時觸發（PR #918 已 ship）
- [ ] 3.5 MCP tool 提供 audit chain verification（首次部署必補）
- [ ] 3.6 audit log 路徑不在 container overlay（避免重啟遺失）

## Section 4：Auth 與 Rate Limit（10 分鐘）

```bash
# 4.1 所有 /api/* 端點都過 AuthMiddleware
grep -rn "mux.HandleFunc\|mux.Handle(" internal/monitoring/api/ \
  | grep -v "AuthMiddleware\|wrapAdminAuth\|RequireAdmin\|isPublicPath" \
  | head -30

# 4.2 admin routes 都有保護
grep -n "wrapAdminAuth\|RequireAdmin" cmd/atlas/admin_routes.go

# 4.3 rate limit 常數
grep -rn "rate.Every\|rate.NewLimiter" internal/apigateway/limits.go

# 4.4 circuit breaker 常數
grep -n "CircuitBreakerFailureThreshold\|CircuitBreakerRecoveryTimeout" \
  internal/apigateway/*.go
```

- [ ] 4.1 全部 `/api/*` 端點在 `AuthMiddleware` 後（無 bypass）
- [ ] 4.2 全部 admin routes 在 `wrapAdminAuth` 或 `RequireAdmin` 後
- [ ] 4.3 15+ 個 channel 都有 `rate.Limiter` 註冊（`internal/apigateway/limits.go`）
- [ ] 4.4 Circuit breaker 仍維持 `failure=3 / recovery=5min`（PR #890 規格）
- [ ] 4.5 production 模式（`ATLAS_ENV=production`）+ `ATLAS_API_KEY` 缺失 → fail-closed（503）
- [ ] 4.6 dev mode（`ATLAS_API_KEY=""`）的 warning log 仍會出現（PR #918 已 ship）

## Section 5：Live Trading Guardrails（10 分鐘）

```bash
# 5.1 double-gate 在 source code 中存在
grep -n "ATLAS_ALLOW_LIVE_BROKER\|ATLAS_ALLOW_HTTP_BROKER\|ATLAS_ALLOW_REAL_SIGNER" \
  cmd/atlas/main.go

# 5.2 broker dry-run 預設
grep -n "BrokerMode.*envOr\|broker-mode" cmd/atlas/main.go internal/config/config.go

# 5.3 fubon-proxy supervisor F1-F9 invariants
grep -rn "F[1-9]:" .claude/skills/atlas-fubon-supervisor-invariants/SKILL.md
```

- [ ] 5.1 `--allow-live-broker` / `--allow-http-broker` / `--allow-real-signer` 各需對應 env（PR #918 已 ship）
- [ ] 5.2 broker mode 預設 `dry-run`，adapter 預設 `guarded`，signer 預設 `placeholder`
- [ ] 5.3 fubon-proxy `ProcessManager` 符合 F1-F9 invariants（[`.claude/skills/atlas-fubon-supervisor-invariants/SKILL.md`](../../.claude/skills/atlas-fubon-supervisor-invariants/SKILL.md)）
- [ ] 5.4 live mode 啟用鏈完整：CLI flag + env var + audit log entry 三者皆有
- [ ] 5.5 無未授權的下單路徑（`grep -rn "order.place\|PlaceOrder" internal/live/`）

## Section 6：Operational Drills（1-2 小時）

```bash
# 6.1 API key rotation drill
old_key=$(grep ATLAS_API_KEY ~/.config/atlas-go/.env | cut -d= -f2)
# 換新 key（手動編輯 .env + 重啟）
# 確認：graceful period 內 client 仍可連線
# 確認：audit log 內可見 rotation event

# 6.2 Anomaly detection drill
# 故意 burst 100 calls → 確認 mcp_anomaly_get_recent 抓到
# 跑 anomaly response playbook（首次部署：建立 SOP）

# 6.3 Crash + audit recovery test
# SIGKILL process mid-burst
# 確認：error / unauthorized 記錄在重啟後仍存在（fsync 驗證）

# 6.4 TLS cert rotation
# 換 cert → 確認 atlas-mcp HTTP transport 仍 bind 127.0.0.1
# 確認：reverse proxy 仍用新 cert

# 6.5 Backup + restore audit log
tar czf audit-$(date +%Y%m%d).tar.gz ~/.config/atlas-go/audit/
# 模擬 corrupt + restore → 確認 audit chain 驗證通過
```

- [ ] 6.1 API key rotation drill 通過（**首次部署：建立 SOP**）
- [ ] 6.2 Anomaly detection drill 通過（**首次部署：建立 SOP**）
- [ ] 6.3 Crash + audit recovery 通過（fsync 驗證）
- [ ] 6.4 TLS cert rotation drill 通過
- [ ] 6.5 Audit log backup + restore drill 通過
- [ ] 6.6 Keychain integration 通過（**首次部署：實作 + 驗證**）

## Section 7：Documentation 與 Sign-off

- [ ] 7.1 `SECURITY.md` 反映當前狀態（特別是 hash 比較、SafeKey、double-gate）
- [ ] 7.2 `docs/specs/security-audit-spec.md` 更新（含本 checklist 引用）
- [ ] 7.3 `docs/reference/tool-catalog.md` 反映任何 auth_status 等新欄位
- [ ] 7.4 `internal/apigateway/CONSTITUTION.md` 無 obsolete appendices
- [ ] 7.5 運維團隊已 briefed rotation SOP
- [ ] 7.6 Incident response playbook 存在（anomaly burst / key compromise / crash recovery）

---

## 簽核紀錄

| 日期 | 稽核者 | 環境 | 結果 | 備註 |
|------|--------|------|------|------|
| | | staging / prod / first-deploy | PASS / FAIL with follow-up | |

---

## 版本歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| 1.0 | 2026-07-03 | 初版，從 PR #918 安全 hardening 後規劃。7 sections、40+ check items。對應 SECURITY.md / apigateway Constitution / 安全 audit spec 的部署前複核入口。 |