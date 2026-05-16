# Security Posture Report: atlas-go

**Report Date**: 2026-05-16  
**Auditor**: Sisyphus (AI Security Audit)  
**Scope**: Full repository security audit before public GitHub release  
**Repository**: github.com/kaecer68/atlas-go  
**Branch**: main  
**Commit**: Latest  

---

## Executive Summary

This report presents the findings of a comprehensive security audit conducted on the `atlas-go` repository prior to its planned transition to a public GitHub repository. The audit covered secrets management, infrastructure security, code vulnerabilities, dependency supply chain, and attack surface mapping.

**Overall Risk Rating**: **MEDIUM-HIGH**

While the codebase demonstrates good security practices (macOS Keychain integration, environment variable externalization, no secrets in git history), several critical issues must be addressed before public release, primarily related to local credential exposure and authentication gaps.

---

## Critical Findings (Must Fix Before Release)

### CRITICAL-1: `.env` File Contains Real Production Credentials

**Severity**: CRITICAL  
**File**: `.env` (working directory, NOT tracked by git)  
**Status**: Not in git history, but present locally

The `.env` file in the working directory contains actual production credentials:

| Variable | Type | Risk |
|----------|------|------|
| `FINMIND_API_KEY` | JWT Token | Financial data access |
| `FUGLE_API_KEY` | Base64-encoded API key | Real-time market data |
| `TEJ_API_KEY` | API key | Premium financial data |
| `FUBON_API_KEY` | API key | Broker access |
| `FUBON_PERSONAL_ID` | Taiwan National ID | Broker identity |
| `FUBON_PASSWORD` | Password | Broker login |
| `FUBON_CERT_PASSWORD` | Certificate password | Digital signature |

**Impact**: If this file is accidentally committed or the directory is archived/shared, all credentials are compromised.

**Remediation**:
1. **Immediately rotate ALL API keys and passwords** listed above
2. Move `.env` outside the repository directory (e.g., `~/.config/atlas-go/.env`)
3. Update application code to read from the new location
4. Add `.env` to `.gitignore` (already present, verify it's effective)

---

### CRITICAL-2: API Authentication is Opt-In (Disabled by Default)

**Severity**: CRITICAL  
**File**: `internal/monitoring/api/shared/handler.go:18-37`  

The authentication middleware is a no-op if `ATLAS_API_KEY` environment variable is not set. This means:
- Default deployment has **NO authentication**
- All `/api/*` endpoints are publicly accessible
- Admin endpoints (`/admin/*`) bypass auth entirely

**Impact**: Anyone with network access can:
- Promote/revert experiments (`/api/experiment/promote`)
- Modify system parameters (`/api/parameters`)
- Write to `.env` file (`/api/dashboard/api-keys/update`)
- Execute backtests and simulations

**Remediation**:
1. Make authentication **mandatory** in production mode
2. Add `--require-auth` flag or `ATLAS_ENV=production` enforcement
3. Protect `/admin/*` routes with AuthMiddleware
4. Add startup warning if auth is disabled

---

### CRITICAL-3: API Key Update Endpoint Writes Arbitrary Content to `.env`

**Severity**: CRITICAL  
**File**: `internal/monitoring/dashboard_api.go:566`  

The `/api/dashboard/api-keys/update` endpoint allows writing arbitrary key-value pairs to the `.env` file. No validation on keys or values.

**Impact**: Potential privilege escalation, credential injection, or configuration tampering.

**Remediation**:
1. Restrict to known-safe API key variables only
2. Validate input format and characters
3. Consider using a dedicated secrets store instead of `.env`

---

## High Findings

### HIGH-1: `Dockerfile.cron` Runs as Root

**Severity**: HIGH  
**File**: `Dockerfile.cron`  

The cron job Dockerfile does not create or switch to a non-root user. All cron containers run as root.

**Remediation**: Add `USER` directive (see main `Dockerfile` for pattern).

---

### HIGH-2: Docker Compose Mounts `.env` into Containers

**Severity**: HIGH  
**File**: `docker-compose.yml` lines 24, 149, 171, 193, 215, 237, 259, 281  

The `.env` file is mounted read-only into all Atlas services and cron containers. While `:ro` prevents modification, secrets can still be read from running containers.

**Remediation**: Pass secrets via Docker secrets or runtime environment variables, not file mounts.

---

### HIGH-3: Default Passwords in Docker Compose

**Severity**: HIGH  
**File**: `docker-compose.yml`  

Multiple services use fallback default passwords:
- `DB_PASSWORD:-atlas_secret` (database)
- `GRAFANA_PASSWORD:-admin` (Grafana admin)
- `POSTGRES_PASSWORD: atlas_test` (CI tests)

**Remediation**: Remove defaults; fail hard if required variables are not set.

---

### HIGH-4: GitHub Actions Not SHA-Pinned

**Severity**: HIGH  
**Files**: `.github/workflows/*.yml`  

Third-party actions use version tags instead of SHA commits:
- `securego/gosec@master`
- `softprops/action-gh-release@v2`
- `docker/build-push-action@v5`
- `docker/login-action@v3`

**Remediation**: Pin all third-party actions to specific SHA commits.

---

### HIGH-5: Admin Endpoints Bypass AuthMiddleware

**Severity**: HIGH  
**File**: `cmd/atlas/main.go:273-286`  

The `/admin/reload-config` and `/api/admin/calibrate-thresholds` endpoints are registered inline in `main.go` and do not go through `AuthMiddleware`.

**Remediation**: Add authentication checks or integrate with middleware chain.

---

## Medium Findings

### MEDIUM-1: PostgreSQL SSL Disabled

**Severity**: MEDIUM  
**File**: `docker-compose.yml`  

All `DATABASE_URL` values use `sslmode=disable`, meaning unencrypted database connections.

**Remediation**: Enable `sslmode=require` in production; use `verify-full` with proper certs.

---

### MEDIUM-2: Fubon Proxy Dockerfile Runs as Root

**Severity**: MEDIUM  
**File**: `services/fubon-proxy/Dockerfile`  

No `USER` directive; Python service runs as root.

**Remediation**: Add non-root user (`useradd -m appuser && USER appuser`).

---

### MEDIUM-3: Ports Exposed on All Interfaces

**Severity**: MEDIUM  
**File**: `docker-compose.yml`  

Services bind to `0.0.0.0` by default:
- 8080 (API), 8081 (Proxy), 6379 (Redis), 5432 (PostgreSQL), 9090 (Prometheus), 3000 (Grafana)

**Remediation**: Bind to `127.0.0.1` where possible, or use firewall rules.

---

### MEDIUM-4: `.fubon-env` Directory Contains Certificate File

**Severity**: MEDIUM  
**File**: `.fubon-env/M120628569_20270430.p12`  

A `.p12` certificate file exists in the `.fubon-env` directory. While the directory has its own `.gitignore` (`*`), it's not explicitly listed in the root `.gitignore`.

**Remediation**: Add `.fubon-env/` to root `.gitignore` as defense in depth.

---

### MEDIUM-5: No Rate Limiting on API Endpoints

**Severity**: MEDIUM  
**Scope**: All API endpoints  

No rate limiting could allow resource exhaustion via:
- Backtest triggering (`/api/backtest/run`)
- Experiment promotion (`/api/experiment/promote`)
- Data ingestion (`/api/macro/ingest`)

**Remediation**: Add rate limiting middleware (e.g., token bucket per IP).

---

### MEDIUM-6: Input Validation Gaps on Control Endpoints

**Severity**: MEDIUM  
**File**: `internal/monitoring/api/control/handlers.go`  

Sector names, agent IDs, and model names are not validated against known values before processing.

**Remediation**: Add allowlist validation for all user-provided identifiers.

---

## Low Findings

### LOW-1: Grafana Default Admin Password in Compose

**Severity**: LOW  
**File**: `docker-compose.yml:122`  

`${GRAFANA_PASSWORD:-admin}` defaults to `admin`. Acceptable for local dev, risky for production.

**Remediation**: Document production hardening requirements.

---

### LOW-2: Test Credentials in CI Workflow

**Severity**: LOW  
**File**: `.github/workflows/ci-cd.yml:178-180`  

`POSTGRES_PASSWORD: atlas_test` is visible in the workflow file. This is acceptable for ephemeral test databases.

**Remediation**: No action required for test-only credentials.

---

## Positive Security Findings

The following practices are working well and should be maintained:

1. **No Secrets in Git History**: Comprehensive scanning found no leaked credentials in git history
2. **macOS Keychain Integration**: Secrets can be stored in system Keychain instead of files
3. **`.env.example` Template**: Provides clear documentation without real values
4. **`.env` in `.gitignore`**: Prevents accidental commits (verify effectiveness)
5. **Go Modules Verified**: `go mod verify` passes successfully
6. **Security Scanning in CI**: `gosec` runs automatically in CI pipeline
7. **Non-Root Main Container**: Main `Dockerfile` correctly uses `USER atlas`
8. **No Hardcoded Secrets in Code**: All API keys use struct fields or env vars
9. **SSL/TLS for External APIs**: Outbound connections use HTTPS
10. **Health Checks**: All containers have health checks configured

---

## Attack Surface Summary

### Endpoint Count by Category

| Category | Count | Auth Required |
|----------|-------|---------------|
| Public (no auth) | 5 | No |
| Authenticated API | 89 | Yes (if ATLAS_API_KEY set) |
| Admin (no auth gate) | 2 | No |
| SSE Streaming | 2 | Yes |
| File Write | 3 | Yes |

### Critical Operations (Require Extra Protection)

| Operation | Endpoint | Current Protection |
|-----------|----------|-------------------|
| Experiment promotion | `POST /api/experiment/promote` | Optional API key |
| Baseline revert | `POST /api/experiment/revert` | Optional API key |
| Parameter modification | `POST /api/parameters` | Optional API key |
| Config reload | `POST /admin/reload-config` | None |
| API key update | `POST /api/dashboard/api-keys/update` | Optional API key |
| Backtest execution | `POST /api/backtest/run` | Optional API key |

---

## External Integrations

| Provider | Auth Method | Data Type | Risk |
|----------|-------------|-----------|------|
| TWSE OpenAPI | None | Market data | Low |
| FinMind | API key | Historical data | Medium |
| Fugle | API key | Real-time quotes | Medium |
| TEJ | API key | Premium data | Medium |
| Fubon | Account + Password | Real-time quotes | High |
| Yahoo Finance | None (scraping) | Macro data | Low |
| Frankfurter | None | FX rates | Low |
| GDELT | None | Geopolitical | Low |

---

## Remediation Checklist

### Before Public Release (Must Do)

- [ ] **CRITICAL**: Rotate all API keys in `.env`
- [ ] **CRITICAL**: Move `.env` outside repository directory
- [ ] **CRITICAL**: Make API authentication mandatory in production
- [ ] **CRITICAL**: Add auth protection to `/admin/*` endpoints
- [ ] **CRITICAL**: Restrict `/api/dashboard/api-keys/update` to safe variables only
- [ ] **HIGH**: Add non-root user to `Dockerfile.cron`
- [ ] **HIGH**: Remove `.env` mounts from `docker-compose.yml`
- [ ] **HIGH**: Remove default passwords from `docker-compose.yml`
- [ ] **HIGH**: Pin GitHub Actions to SHA commits
- [ ] **MEDIUM**: Add `.fubon-env/` to root `.gitignore`
- [ ] **MEDIUM**: Enable PostgreSQL SSL in production
- [ ] **MEDIUM**: Add non-root user to `services/fubon-proxy/Dockerfile`

### Recommended (Post-Release)

- [ ] Add rate limiting middleware
- [ ] Add input validation for control endpoints
- [ ] Bind Docker Compose ports to localhost only
- [ ] Set up secrets scanning workflow (TruffleHog)
- [ ] Add API request logging and audit trail
- [ ] Implement CSRF protection for state-changing endpoints
- [ ] Add Content Security Policy headers
- [ ] Regular dependency vulnerability scanning

---

## Documentation Created

The following files were created to support public repository status:

| File | Purpose |
|------|---------|
| `LICENSE` | Apache License 2.0 |
| `SECURITY.md` | Security policy and vulnerability reporting |
| `CONTRIBUTING.md` | Contribution guidelines and development setup |
| `.github/CODEOWNERS` | Code ownership for review requirements |

---

## Conclusion

The `atlas-go` repository is **close to ready for public release** but requires addressing the critical findings related to credential exposure and authentication before making it public.

The most important action is to **rotate all exposed credentials** and **move the `.env` file outside the repository**. Once these are done, the repository can be safely made public with a MEDIUM risk rating.

**Recommended Timeline**:
1. **Day 1**: Rotate credentials, move `.env`, fix auth
2. **Day 2**: Address high-severity findings
3. **Day 3**: Final verification, make repository public

---

*This report was generated by automated security audit tooling. Manual review is recommended for high-severity findings.*
