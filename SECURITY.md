# Security Policy

## Supported Versions

We release patches for security vulnerabilities. Which versions are eligible
receiving such patches depend on the CVSS v3.0 Rating:

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

## Known Limitations

- This is a research/simulation system, not a production trading platform
- Database connections in Docker Compose use `sslmode=disable` for local development
- Grafana default admin password should be changed in production

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
