---
description: |
  atlas-go workspace-level AI instructions.
  Central hub for understanding the codebase, conventions, workflows, and safe practices.
---

# atlas-go AI Instruction Hub

Welcome to atlas-go! This is a **simulation-first, audit-driven investment research system** for Taiwan equities.

## 🚀 Get Oriented in 2 Minutes

- **Language**: 繁體中文 (Traditional Chinese)
- **Project Type**: Go layered architecture + investment simulation
- **Key Pattern**: Market data → Orchestrator → Layered executors → Control filters → Simulator → Ledger
- **Testing**: `go test ./...` (CI includes gofmt check)

### Core Files to Know

| File | Purpose |
|------|---------|
| [AGENTS.md](AGENTS.md) | **Start here** — Cross-tool AI guidance, file routing, trap quick-reference, tool rules |
| [CLAUDE.md](CLAUDE.md) | Claude Code-specific config (deployment, frontend architecture, token efficiency) |
| [internal/apigateway/CONSTITUTION.md](internal/apigateway/CONSTITUTION.md) | Data source governance — Gateway rules, rate limits, circuit breakers |
| [.github/instructions/go-core.instructions.md](.github/instructions/go-core.instructions.md) | Go coding rules (applies to all `internal/**/*.go`) |

> Full file routing and rule hierarchy → **[AGENTS.md §文件路由](AGENTS.md)** and **[docs/REFERENCE/GUIDELINES_INDEX.md](docs/REFERENCE/GUIDELINES_INDEX.md)**.

## ⚙️ Build & Test Quick Reference

```bash
# Format check (CI blocker)
test -z "$(gofmt -l .)"
gofmt -w .

# Build & test
go build ./...
go test ./...

# Focused iteration
go test ./internal/<package>/...

# Quality checks
go vet ./...
staticcheck ./...

# Coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -n 1
```

## 🔗 Additional Resources

- [docs/QUICKSTART.md](docs/QUICKSTART.md) — First launch & CI commands
- [docs/operations_playbook.md](docs/operations_playbook.md) — Day-to-day workflows
- [docs/iteration_playbook.md](docs/iteration_playbook.md) — Mutation and evolution cycle
- [docs/evolution_loop.md](docs/evolution_loop.md) — Acceptance gate logic
- [docs/data_sources.md](docs/data_sources.md) — Market data import, replay format (JSONL)
- [docs/REFERENCE/PARAMETER_SYSTEM.md](docs/REFERENCE/PARAMETER_SYSTEM.md) — Parameter management with provenance tracking
- `docs/archive/phase2-implementation.md` ~ `phase5-architecture.md` — Historical phase decisions

## 📝 History & Context

- **Language preference**: 繁體中文 throughout codebase (enforced in AGENTS.md)
- **Project origin**: Taiwan equity investment research with AI agent orchestration
- **Current wave**: Wave 11 — L2.3 PoC shipped, L2.4 observation window PLANNED

**Last Updated**: June 2026
