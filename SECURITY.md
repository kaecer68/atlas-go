# Security Policy

## Supported Versions

We release patches for security vulnerabilities. Which versions are eligible
to receive such patches depend on the CVSS v3.0 Rating:

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| < latest| :x:                |

## Reporting a Vulnerability

Please report security vulnerabilities by email to the repository owner.

**Please do not open public issues for security vulnerabilities.**

When reporting a vulnerability, please include:

- A description of the vulnerability
- Steps to reproduce (if applicable)
- Possible impact
- Suggested fix (if any)

## Security Measures

This project implements the following security practices:

- **Secret Management**: API keys and credentials are stored in macOS Keychain (production) or environment variables (CI/CD)
- **No Hardcoded Secrets**: No production credentials are committed to the repository
- **Dependency Scanning**: Go modules are regularly audited for vulnerabilities
- **Code Quality**: Static analysis with `go vet`, `staticcheck`, and `golangci-lint`
- **Security Scanning**: `gosec` is used in CI to detect security issues
- **Docker Security**: Containers run as non-root user where applicable
- **Data Visibility Safeguards**: A four-layer control (gateway `FetchResult.Fallback` / `Stale` / `LastError`, adapter `ChannelErrors`, service `data_status` / `failed_channels`, and UI red-badge rendering) detects and surfaces silent fetch failures, preventing zero-value displays that could otherwise mask data integrity issues. See `.claude/skills/atlas-data-visibility/SKILL.md` for the full design.

## Data Source Governance

All external data ingestion is governed by `internal/apigateway/CONSTITUTION.md`, which codifies six binding articles plus three appendices. Any code that adds or modifies a data source must comply with these rules. CI enforces the constitution via `scripts/ci/check_constitution.sh`.

- **Article 1: Unified Entry**. Every external API call must go through `apigateway.Fetch(channelID)` or `ProviderRegistry`. Direct `os.Getenv` and bare `http.Client` construction are forbidden, except for a small whitelist of non-data-source variables (`ATLAS_API_KEY`, `ATLAS_STATE_DIR`, `ATLAS_WORK_DIR`).
- **Article 2: Mandatory Rate Limiting**. Every channel must register a `rate.Limiter` with conservative defaults (for example 1 req / 5s). Limits are hardcoded in `internal/apigateway/limits.go`.
- **Article 3: Health Tracking**. Every call must return `FetchMetadata` (latency, rate-limit remaining, timestamp). All health checks go through `UnifiedHealthStore`; direct writes to `channel_health.json` are forbidden.
- **Article 4: Background Task Registration**. All scheduled data ingestion and periodic computation must register with `BackgroundTaskManager`. Bare goroutines that construct a provider directly are forbidden, with documented exceptions for one-shot waits, WebSocket keepalives, and tests.
- **Article 5: Circuit Breaker**. After three consecutive failures, a channel opens for five minutes. While open, the gateway returns stale cache plus a `stale` flag rather than blocking callers.
- **Article 6: Constitution Priority**. Any PR that changes a data source must pass the constitution checklist (channel registry, env-var whitelist, rate limiter, health check, background task, gateway call, doc update).

## Live Trading Guardrails

`internal/live` and the related orchestration paths in `cmd/atlas` are subject to the guardrails in `.github/instructions/live-trading.guardrails.instructions.md`. The full rules live in that file. This section summarizes the security-relevant boundaries.

- **Replay-first safety policy**: Replay and simulation paths are the default reliable execution surface. Live orchestration is treated as having partial integration gaps unless a task explicitly closes them.
- **`-allow-live-broker` flag off by default**: The flag is defined in `cmd/atlas/main.go` and defaults to `false`. Local development and CI must never pass it. Only an explicit, audited turn-on in a controlled environment should bring live order routing online. The companion `-allow-http-broker` and `-allow-real-signer` flags follow the same default-off pattern.
- **Auditable control flow**: Risk filters, gate ordering, and execution steps must remain reviewable. Changes touching order-execution logic must keep fail-safe behavior when market data is missing.
- **External API access**: Live trading still routes every external call through the gateway per Article 1 of the data source constitution.
- **Verification baseline**: Any change in this area must pass `go test ./internal/live/... ./internal/orchestrator/... ./internal/sim/...` and a manual smoke check via `go run ./cmd/atlas`.

## Known Limitations

- This is a research/simulation system, not a production trading platform
- Database connections in Docker Compose use `sslmode=prefer` for local development. Production deployments should use `sslmode=verify-full` with proper CA certificates.
- Grafana credentials are env-driven via `GF_SECURITY_ADMIN_USER` and `GF_SECURITY_ADMIN_PASSWORD` (sourced from `GRAFANA_USER` and `GRAFANA_PASSWORD` in `docker-compose.yml`). The operator must populate `.env` before deployment and never commit it.

## Responsible Disclosure

We follow responsible disclosure practices:

1. Report received and acknowledged within 48 hours
2. Investigation and fix within 30 days for critical issues
3. Public disclosure after fix is released

## Security-Related Configuration

### Environment Variables

Sensitive configuration is managed through environment variables:

```bash
# Copy template and fill in your values
cp .env.example .env
```

**Never commit `.env` to version control.**

### API Keys

This system integrates with multiple market data providers:

- **TWSE OpenAPI**: Free, no auth required
- **FinMind**: Requires free API key
- **Fugle**: Requires paid API key
- **TEJ**: Requires premium API key
- **Fubon**: Requires broker account

Each user must obtain their own API keys.

#### LLM Providers

The system also routes several LLM-backed capabilities through an internal router. All LLM API keys follow the same storage rules as market-data keys (macOS Keychain on macOS, environment variables elsewhere).

- `LLM_DEEPSEEK_API_KEY` for DeepSeek V4-Pro / V4-Flash (obtain from https://platform.deepseek.com)
- `LLM_MINIMAX_API_KEY` for MiniMax M3 coding-plan keys, with the `sk-cp-` prefix. The router skips calls gated by this key when the request `DataClass` is at or above `Regulated`.
- `LLM_ANNOTATOR_API_KEY` is a backward-compatibility alias that reads the same value as `LLM_MINIMAX_API_KEY`.

Capability feature flags (all default `false`):

- `LLM_RATIONALE_TRANSLATION_ENABLED` enables the `CapabilityRationaleGeneration` hook
- `LLM_PRISM_SCENARIO_ENABLED` enables the `CapabilityScenarioSimulation` hook
- `LLM_NARRATIVE_EXPLAIN_ENABLED` enables `CapabilityRegimeExplanation` and `CapabilitySentimentExplanation`
- `LLM_RISK_FORENSICS_ENABLED` enables `CapabilityPerformanceForensics`
- `LLM_SECTOR_AGENTS_ENABLED` enables the `SectorAgentLLM` Plan to ToolCall to Reflect loop (Issue #719)

### macOS Keychain (Recommended)

On macOS, secrets are stored in Keychain:

```bash
security add-generic-password -a "FINMIND_API_KEY" -s com.kaecer68.atlas-go -w "your_key"
security find-generic-password -a "FINMIND_API_KEY" -s com.kaecer68.atlas-go -w
```

## Previous Security Audits

- **2026-05-16**: Pre-public release security audit completed
  - No secrets found in git history
  - All credentials properly externalized to environment variables
  - macOS Keychain integration verified
- **2026-06-29**: SECURITY.md refresh
  - Fixed P0 factual errors: `sslmode=prefer` (not `disable`); Grafana credentials are env-driven (`GF_SECURITY_ADMIN_USER` and `GF_SECURITY_ADMIN_PASSWORD`) rather than the default `admin/admin`
  - Added P1 sections: Data Source Governance (six articles of `internal/apigateway/CONSTITUTION.md`); Live Trading Guardrails (`.github/instructions/live-trading.guardrails.instructions.md` and the `-allow-live-broker` flag default-off in `cmd/atlas/main.go`); LLM Provider (env-driven LLM keys and capability flags); Data Visibility Safeguards (four-layer control over silent fetch failures, per `.claude/skills/atlas-data-visibility/SKILL.md`)
