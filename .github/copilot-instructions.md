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
| [AGENTS.md](AGENTS.md) | **Start here** — Build commands, architecture boundaries, project pitfalls |
| [docs/GUIDELINES_INDEX.md](docs/GUIDELINES_INDEX.md) | Authority hierarchy, use-case routing |
| [internal/apigateway/CONSTITUTION.md](internal/apigateway/CONSTITUTION.md) | Data source governance — Gateway rules, rate limits, circuit breakers |
| [.github/instructions/go-core.instructions.md](.github/instructions/go-core.instructions.md) | Go coding rules, interface design, error handling |
| [.github/instructions/experiments-guardrails.instructions.md](.github/instructions/experiments-guardrails.instructions.md) | Baseline policy, experiment flow, acceptance logic |
| [.github/instructions/live-trading.guardrails.instructions.md](.github/instructions/live-trading.guardrails.instructions.md) | Live trading path, replay prioritization |
| [docs/PARAMETER_SYSTEM.md](docs/PARAMETER_SYSTEM.md) | Parameter management with provenance tracking |
| [docs/architecture.md](docs/architecture.md) | Layered design, component responsibilities |

## 🎯 Common Workflows by Task

| Task | First Read |
|------|-----------|
| Modify Go code | `AGENTS.md` + `go-core.instructions.md` |
| Add/modify agents or prompts | `AGENTS.md` §重要陷阱；verify `configs/agents.json` ↔ `prompts/agents/` |
| Run experiment / modify baseline | `experiments-guardrails.instructions.md`; verify `data/state/baseline_policy.json` |
| Fix a bug | `AGENTS.md` §關鍵跨模組陷阱 + `docs/architecture.md` |
| Live trading paths | `live-trading.guardrails.instructions.md`; replay-first is the safe default |
| Add data source / API call | `internal/apigateway/CONSTITUTION.md` ALL 6 articles; never bare `&http.Client{}` |
| Add/change parameter | `docs/PARAMETER_SYSTEM.md`; add to `internal/config/parameters.go` |

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

## ⚠️ Top 5 Gotchas

| Gotcha | Prevention |
|--------|-----------|
| **Sparse replay window** | Validate n≥10 before judgment |
| **Reusing recommendation slices across runs** | Rebuild/copy each iteration |
| **Baseline policy not loaded** | Verify `data/state/baseline_policy.json` exists first |
| **Darwinian weight clipping (0.3–2.5) is silent** | Values outside range auto-clamp with no warning |
| **Missing prompt files for enabled agents** | Every `agents.json` entry needs `prompts/agents/<name>.md` |

## 🔗 Additional Resources

- [docs/operations_playbook.md](docs/operations_playbook.md) — Day-to-day workflows
- [docs/iteration_playbook.md](docs/iteration_playbook.md) — Mutation and evolution cycle
- [docs/evolution_loop.md](docs/evolution_loop.md) — Acceptance gate logic
- [docs/data_sources.md](docs/data_sources.md) — Market data import, replay format (JSONL)
- [docs/guides/ai-productivity.md](docs/guides/ai-productivity.md) — Detailed gotchas & command cheat sheet
- `docs/archive/phase2-implementation.md` ~ `phase5-architecture.md` — Historical phase decisions

## 📝 History & Context

- **Language preference**: 繁體中文 throughout codebase
- **Project origin**: Taiwan equity investment research with AI agent orchestration
- **Current phase**: Phase 5 — full evolution loop, evaluation gates, baseline promotion

**Last Updated**: June 2026
