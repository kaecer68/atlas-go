---
description: |
  atlas-go workspace-level AI instructions.
  Central hub for understanding the codebase, conventions, workflows, and safe practices.
---

@CLAUDE.md

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
| [AGENTS.md](../AGENTS.md) | **Start here** — Cross-tool AI guidance, file routing, trap quick-reference, tool rules |
| [CLAUDE.md](../CLAUDE.md) | Claude Code-specific config (deployment, frontend architecture, token efficiency) |
| [internal/apigateway/CONSTITUTION.md](../internal/apigateway/CONSTITUTION.md) | Data source governance — Gateway rules, rate limits, circuit breakers |
| [.github/instructions/go-core.instructions.md](./instructions/go-core.instructions.md) | Go coding rules (applies to all `internal/**/*.go`) |

> Full file routing and rule hierarchy → **[AGENTS.md §文件路由](../AGENTS.md)** and **[docs/reference/guidelines-index.md](../docs/reference/guidelines-index.md)**.

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

- [docs/quickstart.md](../docs/quickstart.md) — First launch & CI commands
- [docs/operations-playbook.md](../docs/operations-playbook.md) — Day-to-day workflows
- [docs/iteration-playbook.md](../docs/iteration-playbook.md) — Mutation and evolution cycle
- [docs/evolution-loop.md](../docs/evolution-loop.md) — Acceptance gate logic
- [docs/data-sources.md](../docs/data-sources.md) — Market data import, replay format (JSONL)
- [docs/reference/parameter-system.md](../docs/reference/parameter-system.md) — Parameter management with provenance tracking
- `docs/archive/2026-06-15-phase2-implementation.md` ~ `2026-06-15-phase5-architecture.md` — Historical phase decisions

## 📝 History & Context

- **Language preference**: 繁體中文 throughout codebase (enforced in AGENTS.md)
- **Project origin**: Taiwan equity investment research with AI agent orchestration
- **Current wave**: Wave 11 — L2.3 PoC shipped, L2.4 observation window **SHIPPED**（PR #821, 2026-06-29）

**Last Updated**: July 2026
