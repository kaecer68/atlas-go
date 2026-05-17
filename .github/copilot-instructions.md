---
description: |
  atlas-go workspace-level AI instructions.
  Central hub for understanding the codebase, conventions, workflows, and safe practices.
---

# atlas-go AI Instruction Hub

Welcome to atlas-go! This is a **simulation-first, audit-driven investment research system** for Taiwan equities. This document is your entry point for productive collaboration with AI agents.

## 🚀 Get Oriented in 2 Minutes

- **Language**: 繁體中文 (Traditional Chinese)
- **Project Type**: Go layered architecture + investment simulation
- **Key Pattern**: Market data → Orchestrator → Layered executors → Control filters → Simulator → Ledger
- **Testing**: `go test ./...` (CI requirement includes gofmt check)
- **Common Entry Points**: See [Common Workflows](#common-workflows-by-task) below

### Core Files to Know

| File | Purpose |
|------|---------|
| [agents.md](agents.md) | **Start here** — Build commands, architecture boundaries, project pitfalls |
| [.github/instructions/go-core.instructions.md](.github/instructions/go-core.instructions.md) | Go coding rules, interface design, error handling, import order |
| [.github/instructions/experiments-guardrails.instructions.md](.github/instructions/experiments-guardrails.instructions.md) | Baseline policy, experiment flow, acceptance logic, mutation safety |
| [.github/instructions/live-trading.guardrails.instructions.md](.github/instructions/live-trading.guardrails.instructions.md) | Live trading path, replay prioritization, TODO boundaries |
| [ai_productivity_guide.md](ai_productivity_guide.md) | Detailed pitfalls lookup, gotchas, anti-patterns, idioms |
| [docs/architecture.md](docs/architecture.md) | Layered design, component responsibilities, failure modes |

## 🎯 Common Workflows by Task

### I want to modify Go code (internal/ or cmd/)
1. ✅ Read: [agents.md](agents.md) (§架構邊界) + [.github/instructions/go-core.instructions.md](.github/instructions/go-core.instructions.md)
2. 🔍 Check: `gofmt -w .` before editing
3. 🧪 Test: Focus on relevant package (e.g., `go test ./internal/orchestrator/...`)
4. ✔️ Verify: `test -z "$(gofmt -l .)"` && `go build ./...`

### I want to add/modify prompts or agents
1. ✅ Read: [agents.md](agents.md) (§重要陷阱 near configs/agents.json)
2. 🔍 Check: `configs/agents.json` for enabled agents, `prompts/agents/` for corresponding prompt files
3. ⚠️ Remember: Every enabled agent needs a valid prompt file
4. 📋 Validate: Run experiments with small replay window first

### I want to run an experiment or modify baseline
1. ✅ Read: [.github/instructions/experiments-guardrails.instructions.md](.github/instructions/experiments-guardrails.instructions.md)
2. 🏗️ Check: Baseline policy loaded (`data/state/baseline_policy.json`)
3. ✅ Verify: Replay window has sufficient data (n≥10 preferred)
4. 🔄 Flow: propose mutation → execute → judge → accept/revert (explicit gates)

### I want to understand/fix a bug
1. ✅ Read: [agents.md](agents.md) (§重要陷阱) + [ai_productivity_guide.md](ai_productivity_guide.md)
2. 🔍 Search gotchas: Sparse replay? Reused slices? Silent Darwinian clipping? Missing baseline?
3. 🧪 Run focused tests on the relevant package
4. 📖 Check: [docs/architecture.md](docs/architecture.md) for component flow

### I'm working on live trading paths
1. ✅ Read: [.github/instructions/live-trading.guardrails.instructions.md](.github/instructions/live-trading.guardrails.instructions.md)
2. ⚠️ Remember: `internal/live/` has TODO boundaries; replay-first is the safe default
3. 🔄 Validate: Decision flow vs. `internal/orchestrator/system.go` / `plugin_host.go`

---

## ⚙️ Build & Test Quick Reference

```bash
# Format check (CI blocker)
test -z "$(gofmt -l .)"

# Fix formatting
gofmt -w .

# Build everything
go build ./...

# All tests (same as CI)
go test ./...

# Focused test (faster iteration)
go test ./internal/orchestrator/...

# Quality checks
go vet ./...
staticcheck ./...

# Coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -n 1
```

**Focus on one package** during iteration. When ready to merge, run CI checks.

---

## 🏗️ Architecture at a Glance

```
Market Data (configs/agents.json, replay/*.jsonl)
         ↓
Orchestrator (internal/orchestrator/)
    ├─ RouteAgents() calls layer executors
    ├─ RegimeExecutor (context layer)
    ├─ AgentExecutor (sector/style/superinvestor layers)
    └─ ControlExecutor (CRO/CIO filters)
         ↓
Simulator (internal/sim/)
    ├─ RunSymbol() executes each symbol
    ├─ ApplyRecommendations() + ApplyControl()
    └─ Produces Position mutations
         ↓
Ledger (internal/ledger/)
    └─ Persists outcomes for judgment
```

**Key boundary**: Domain types stay in `internal/domain/types.go`. Coordination logic stays in `internal/orchestrator/`. No global mutable state for execution.

---

## ⚠️ Top 5 Gotchas

| Gotcha | Prevention |
|--------|-----------|
| **Sparse replay window** — low confidence | Validate n≥10 before judgment; check `data/state/sessions/` |
| **Reusing recommendation slices across runs** | Rebuild/copy each iteration; check simulator for stale references |
| **Baseline policy not loaded before experiment** — results show invalid values | Verify `data/state/baseline_policy.json` exists; run baseline load first |
| **Darwinian weight clipping (0.3–2.5) is silent** | Don't assume boundaries; values outside range auto-clamp with no warning |
| **Missing prompt files for enabled agents** | Every entry in agents.json must have matching `prompts/agents/<name>.md` |

**See** [ai_productivity_guide.md](ai_productivity_guide.md) for the full gotcha lookup table with symptoms & fixes.

---

## 📦 Key Patterns

### Executor Interface Pattern
```go
// Small, focused interface (one operation + Supports check)
type MyExecutor interface {
    Supports(layer string) bool
    Execute(reqs ...) (results [], error)
}
```

### Error Wrapping
```go
// Always add context
return fmt.Errorf("context: %w", err)
```

### Domain Strings (not enums)
```go
// Prefer string for JSON roundtrip clarity
type Regime string
const (
    Bull Regime = "bull"
    Bear Regime = "bear"
)
```

### Test Organization
```
internal/orchestrator/
  ├─ orchestrator.go
  └─ orchestrator_test.go  ← same package, same directory
```

---

## 🔗 Additional Resources

### Architecture & Design
- [docs/architecture.md](docs/architecture.md) — Layered architecture, component responsibilities
- [docs/ai_agent_architecture.md](docs/ai_agent_architecture.md) — Agent coordination, decision flow
- [agents.md](agents.md) — Repository boundaries and conventions

### Operations & Experimentation
- [docs/operations_playbook.md](docs/operations_playbook.md) — Day-to-day workflows
- [docs/iteration_playbook.md](docs/iteration_playbook.md) — Mutation and evolution cycle
- [docs/evolution_loop.md](docs/evolution_loop.md) — Full acceptance gate logic
- [docs/script_usage_guide.md](docs/script_usage_guide.md) — Helper scripts (darwinian-adjust, swarm-manage, etc.)

### Data & Configuration
- [docs/data_sources.md](docs/data_sources.md) — Market data import, replay format (JSONL)
- [ai_productivity_guide.md](ai_productivity_guide.md) — Detailed gotchas, configuration patterns, command cheat sheet

### Implementation History
- `docs/archive/phase2-implementation.md` through `docs/archive/phase5-architecture.md` — Historical phase decisions and architecture details

---

## ✅ Verification Checklist

Before submitting changes, verify:

- [ ] Code is formatted: `test -z "$(gofmt -l .)"`
- [ ] Tests pass for impacted package(s): `go test ./internal/<package>/...`
- [ ] Import groups are correct (stdlib, external, internal)
- [ ] Errors are wrapped with context: `fmt.Errorf("context: %w", err)`
- [ ] No global mutable state introduced for execution coordination
- [ ] Domain types preserved in `internal/domain/types.go`
- [ ] If touching experiments: baseline policy loaded, replay window verified (n≥10)
- [ ] If modifying agents: corresponding prompt files exist in `prompts/agents/`

---

## 🤝 Quick Answers

**Q: Where do I put a new Executor?**  
A: Implement the executor interface from `internal/orchestrator/plugin_registry.go` in a new `internal/<domain>/executor.go`. If it needs lifecycle hooks (attach / before-sim / after-sim), also implement `Plugin` from `internal/orchestrator/plugin.go` and register it via `PluginHost` in `internal/orchestrator/factory.go`.

**Q: How do I add a new prompt to an agent?**  
A: Create `prompts/agents/<agent-name>.prompt.md`, update `configs/agents.json` with enabled=true, then test with `go run ./cmd/run-experiment -brief <test.brief>`.

**Q: What's the difference between replay and simulate?**  
A: **Replay**: deterministic run-through of historical data with current config. **Simulate**: forward-looking portfolio updates via orchestrator recommendations. Both feed Ledger.

**Q: How do I know if my change broke something?**  
A: Run `go test ./...` (full suite) + spot-check key packages. Check `go vet ./...` for common issues. Review git diff for unintended side effects.

**Q: When should I read each instruction file?**  
A: Start with [agents.md](agents.md) for orientation. Then pick the domain-specific instruction matching your task (go-core, experiments-guardrails, or live-trading).

---

## 📝 History & Context

- **Language preference**: 繁體中文 throughout codebase
- **Project origin**: Taiwan equity investment research with AI agent orchestration
- **Current phase**: Phase 5 — full evolution loop, evaluation gates, baseline promotion
- **CI alignment**: All commands in this file match CI expectations

---

**Last Updated**: April 2026  
**Maintainers**: AI development agents following this instruction set
