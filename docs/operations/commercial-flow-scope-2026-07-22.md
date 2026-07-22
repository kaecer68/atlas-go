# #1068 Commercial Flow — scope assessment (2026-07-22)

> Status: **DEFERRED** (post-onboarding). Product-market fit first.

---

## Current auth architecture

### HTTP Dashboard API (`internal/monitoring/api/shared/handler.go:128`)
- `AuthMiddleware` checks `ATLAS_API_KEY` (sha256 of env var vs request header)
- `isAuthFreePath()` — public paths: `/health`, `/ready`, `/metrics`, `/admin/...`, `/client/...`, `/api/dashboard/sessions`
- **Tiers**: binary — either `ATLAS_API_KEY` matches (full admin) or doesn't (no /api/ access)
- Dev mode: no key = permissive

### MCP Server (`cmd/atlas-mcp/server/auth.go`)
- `TokenAuth` validates bearer tokens
- DB-backed `TokenStore` (PostgreSQL, SHA-256 hashes) with registration/rotation/revocation
- Env var `ATLAS_MCP_TOKEN` fallback for dev
- **Tiers**: binary — has valid token = all tools, no token = dev mode (all tools)
- `BearerAuth` HTTP middleware on SSE + streamable-HTTP transports
- Stdio transport: no auth (process-level)

### Guest mode
- Pre-commercial auth bypass: `ATLAS_API_KEY=""` in dev → all endpoints unauthenticated
- No user concept, no session, no tier

### Tier enforcement at tool level
- **Does NOT exist**. All MCP tools are equally accessible with a valid token.
- Some tool descriptions mention "Requires ATLAS_API_KEY" as documentation hint only
- No middleware checks tool-level authorization

---

## What exists vs what's needed

| Component | Exists? | Notes |
|-----------|---------|-------|
| API key format | ✅ SHA-256 hash | Simple but not JWT (can't embed claims) |
| Key storage (DB) | ✅ `TokenStore` (PG) | Registration/rotation/revocation supported |
| Admin key management | ⚠️ Partial | Low-level HTTP API exists; no web UI |
| User registration | ❌ | No web form, no OAuth, no passkey |
| Tier definitions | ❌ | Free/premium/admin undefined |
| Tool-level tier gate | ❌ | No middleware for tool-tier authorization |
| Usage tracking | ❌ | No per-key request counting or metering |
| Rate limiting per key | ❌ | Global `RateLimitPerMinute=120` only |
| Billing integration | ❌ | No Stripe or any payment system |
| Self-serve key issuance | ❌ | No POST /v1/keys endpoint |
| Key rotation portal | ❌ | Admin can rotate via low-level API; no user portal |
| Key revocation UI | ❌ | Supported in DB layer; no admin or user UI |

---

## Estimated effort (when unblocked): 2–3 weeks

1. **Tier model design** (1 day)
   - Define which MCP tools are free vs premium
   - Define rate limits per tier
   - Document in CONSTITUTION.md

2. **Auth middleware upgrade — MCP** (3 days)
   - Add tier claim to token (JWT migration or DB column)
   - Add `ToolAuthorizationMiddleware` that checks tool→tier mapping
   - Wire into `countedAddTool` or dispatch layer

3. **Auth middleware upgrade — HTTP API** (2 days)
   - Add tier concept to `AuthMiddleware` (if needed for web UI)
   - Add rate limit per tenant

4. **Usage tracking** (2 days)
   - Per-key request counter (Prometheus or DB)
   - Usage dashboard (admin can view key usage)

5. **Web UI for key management** (3 days)
   - Logged-in user portal: generate, list, revoke keys
   - Admin panel: view all keys, override tiers

6. **User registration & login** (3 days)
   - Email/password or OAuth (GitHub, Google)
   - Session management
   - "Forgot key" flow

7. **Billing integration** (3 days)
   - Stripe checkout → tier assignment
   - Webhook handler for subscription events
   - Invoice/receipt generation

8. **Docs + onboarding** (1 day)
   - Update AGENT_QUICKSTART.md, README.md
   - Document tier tools matrix

---

## Prerequisites before unblocking

- [x] MCP server → atlas HTTP API pipeline working (hermes onboarding complete)
- [ ] Product-market fit validation: ≥3 external users consistently using public tier
- [ ] Tool-tier classification agreed (which tools are free vs premium)
- [ ] Pricing decided (monthly vs per-call vs tiered)
- [ ] Legal: ToS, privacy policy, data processing agreement

---

## Recommendation

**Keep deferred**. Current priorities:
1. 🟢 Hermes MCP onboarding (done — #1067, #1269–#1278)
2. 🟠 C07 Day 14 gate (7/30 deadline)
3. 🔴 L2.4 sector agent qualification (observation window)

Commercial flow should be the **last** thing built — we need proof that people
actually use the tools before spending 2-3 weeks on a payment system.
