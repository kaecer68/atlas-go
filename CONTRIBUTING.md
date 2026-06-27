# Contributing to atlas-go

Thank you for your interest in contributing to atlas-go! This document provides
guidelines and instructions for contributing.

## Development Setup

### Prerequisites

- Go 1.25+
- PostgreSQL 15 (optional, for persistence)
- Redis 7 (optional, for caching)
- Docker & Docker Compose (optional, for full stack)

### Quick Start

```bash
# Clone the repository
git clone https://github.com/kaecer68/atlas-go.git
cd atlas-go

# Copy environment template
cp .env.example .env

# Build
go build ./...

# Run tests
go test ./...
```

## Code Style

We follow standard Go conventions:

- Format code with `gofmt`
- Run `go vet ./...` before submitting
- Use `staticcheck` for static analysis
- Follow the interface pattern: small, focused interfaces with `Supports(...) bool`
- Wrap errors with context: `fmt.Errorf("context: %w", err)`
- Import order: stdlib → external → `github.com/kaecer68/atlas-go/...`

## Testing

- Write tests in `*_test.go` files in the same package
- Run `go test ./...` before submitting
- Integration tests use `//go:build integration` tag
- Coverage threshold: 60% minimum

## Pull Request Process

1. **Branch naming**: Use `feat/<name>`, `fix/<name>`, or `refactor/<name>`
2. **Commit messages**: Follow `type(scope): description` format
3. **Pre-commit checks**:
   ```bash
   test -z "$(gofmt -l .)"
   go build ./...
   go test ./...
   go vet ./...
   staticcheck ./...
   ```
4. **Update documentation** if your change affects user-facing behavior
5. **Ensure `.env` is not committed** — it should always be gitignored

## Architecture Guidelines

- Domain types stay in `internal/domain/`
- Coordination logic stays in `internal/orchestrator/`
- No global mutable state for execution coordination
- Each enabled agent in `configs/agents.json` needs a prompt file in `prompts/agents/`

## Security

- Never commit secrets, API keys, or credentials
- Use macOS Keychain for local secret storage
- All API keys are user-provided via environment variables
- See [SECURITY.md](SECURITY.md) for full security policy

## Questions?

- Read [AGENTS.md](AGENTS.md) for architecture overview
- Check [docs/architecture.md](docs/architecture.md) for detailed design
- Review [docs/guides/ai-productivity.md](docs/guides/ai-productivity.md) for common pitfalls

## License

By contributing, you agree that your contributions will be licensed under the
Apache License 2.0.
