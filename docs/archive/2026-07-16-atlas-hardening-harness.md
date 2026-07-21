# Audit Manifest: Atlas Hardening Harness

> **Audit source**: User reports agents confusing dev/prod environments, modifying files without checking facts, assuming without reading docs, and repeatedly causing cleanup work. Goal is to adopt mature harness-engineering patterns (Deuk Agent Flow, goat-flow, Lattice Core) to harden atlas-go against agent errors.
> **Goal**: Add deterministic guardrails, agent memory, mode-aware gates, and environment isolation so agents cannot accidentally break atlas-go or confuse contexts.
> **Scope**: Harness engineering only. No deployment infrastructure, no cloud topology, no new features.
> **Created**: 2026-07-16
> **Status**: complete (pending PR merge)

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| H01 | Agent can accidentally run destructive commands (`rm -rf`, `git push`, DB truncate). | No deterministic pre-action hooks; only soft instructions in skills. | `.githooks/pre-commit`, `.agent-hooks/deny-dangerous.sh`, `AGENTS.md` | Running blocked commands in a controlled test fails with a clear error. | done | promote-to-docs | Phase 1: deny-dangerous hooks |
| H02 | Agent reads `.env`, secrets, or `.p12` certificates without authorization. | No file-access guardrails beyond convention. | `.agent-hooks/`, `atlas-pre-change-protocol` skill | A test attempt to read `.env` inside an agent session is blocked or logged. | done | promote-to-docs | Phase 1: deny-dangerous hooks |
| H03 | Agent confuses dev/prod/staging environments. | Environment is inferred from context, not enforced by worktree/secrets isolation. | `docs/environment.md`, `atlas-pre-change-protocol` skill, `.env` layout convention | Each environment has a distinct worktree + env file path; cross-env commands are blocked. | done | promote-to-docs | Phase 5: Environment Isolation Contract + hook rules |
| H04 | Agent starts live trading by mistake. | Live broker opt-in is instruction-based, not hook-enforced. | `.agent-hooks/`, `internal/live/` AGENTS.md | `-allow-live-broker` requires explicit env + CLI + hook confirmation. | done | none | Phase 1: deny-dangerous hook gates live broker |
| H05 | Agent edits code while in planning/audit mode. | No mode-aware gate; plan mode is convention only. | `atlas-pre-change-protocol` skill, `.agent-hooks/` | Plan mode refuses file writes; execute mode requires manifest ID binding. | done | promote-to-docs | Phase 3: mode-aware gates |
| H06 | Same mistakes recur across sessions. | No persistent footguns/lessons/decisions repository. | `.agent-memory/footguns/`, `.agent-memory/lessons/`, `.agent-memory/decisions/` | Every blocked action or significant mistake produces a footgun or lesson entry. | done | promote-to-docs | Phase 2: agent-memory repository |
| H07 | Agent claims work is done without running verification. | Verification commands are scattered; no one-command verify. | `scripts/verify-atlas.sh` (new), `docs/manifests/README.md` | `./scripts/verify-atlas.sh` runs format, vet, test, staticcheck, manifest verify, drift check. | done | new-doc | Phase 4: one-command verification |
| H08 | Agent asks questions whose answers are already in code/docs. | MCP tools exist but agent does not have a "check first" discipline. | `atlas-pre-change-protocol` skill, MCP tool usage prompts | Skill mandates querying MCP / gitnexus before asking project-specific questions. | done | none | Phase 3: mode-aware gates + "check first" prompts |
| H09 | Agent scope drifts to unrelated files. | Scope lock is instruction-based, not enforced. | `atlas-pre-change-protocol` skill, `.agent-hooks/` | Execute mode rejects edits outside declared manifest IDs. | done | none | Phase 3: scope-lock in session start + red flags |
| H10 | Atlas-MCP lacks hard rate limiting and auth defaults for external agents. | Rate limit defaults to 0; stdio mode has no token enforcement. | `cmd/atlas-mcp/main.go`, `cmd/atlas-mcp/server/ratelimit.go` | HTTP/SSE mode has non-zero default rate limit; stdio mode documents auth requirement. | done | none | Phase 6: default 120 req/min + stdio auth docs |

---

## Phase Tracker

### Phase A — Audit (read-only)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Survey Deuk/goat-flow/Lattice harness patterns | all | done | Web search + fetch of project pages |
| Map atlas-go existing harness mechanisms | all | done | Read `AGENTS.md`, `.claude/skills/`, `docs/documentation-standard.md` |
| Identify gaps causing repeated agent errors | all | done | See Invariant Tracker |

### Phase B — Plan

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Design hook architecture | H01-H04 | done | Plan file |
| Design agent-memory layout | H06 | done | Plan file |
| Design verify-atlas.sh | H07 | done | Plan file |
| Define mode-aware gate states | H05, H09 | done | Plan file |
| Define environment isolation contract | H03 | done | Plan file |

### Phase C — Implement

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Add deny-dangerous hooks | H01, H02, H04 | done | `4c6bbb1d` |
| Create agent-memory repository | H06 | done | `273fb366` |
| Add mode-aware gates | H05, H09 | done | `64386eaa` |
| Create verify-atlas.sh | H07 | done | `0cebb5eb` |
| Strengthen "check first" discipline | H08 | done | `64386eaa` |
| Add environment isolation rules | H03 | done | `46b2387f` |
| Harden atlas-mcp rate limit defaults | H10 | done | This commit |

### Phase D — Close Out

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Update manifest status | all | done | This commit |
| Run CI / verify | all | done | `./scripts/verify-atlas.sh --skip-frontend` passed |
| Delete `.omo/plans/` files | all | done | `.omo/` does not exist |
| Verify no unauthorized `.omo/` dirs | all | done | `.omo/` does not exist |
| Push branch / open PR | all | done | PR #1209 https://github.com/kaecer68/atlas-go/pull/1209 |

---

## Backlog

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|---------------|----------------|
| B01 | Add cost/token guardrails to prevent runaway LLM loops | 2026-07-16 | Future — out of scope |
| B02 | Add worktree pool automation for multi-agent parallelism | 2026-07-16 | Future — out of scope |
| B03 | Evaluate adopting Deuk Agent Flow or goat-flow wholesale | 2026-07-16 | Future — after internal harness dogfood |

> **Rule**: only move one backlog item into scope per session, and only after all current IDs are done or paused.

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- One commit per ID group (logical phase)
- No commit without acceptance criteria passing for that phase
- PR body must reference this manifest: `See docs/manifests/2026-07-16-atlas-hardening-harness.md`

---

## Session-End State

- **Done this session**: Phases 1-7 complete (all implementation + close-out; branch pushed, PR opened).
- **Remaining**: PR review + merge; post-merge cleanup per `docs/multi-cli-protocol.md`.
- **Next action**: Monitor PR CI and merge on green.
- **Uncommitted code**: no
- **Branch / PR**: `feat/atlas-hardening-harness` / PR #1209
- **Paused because**: waiting for PR #1209 CI to finish and merge

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-16 | 1.0 | Initial manifest | agent |
| 2026-07-16 | 1.1 | Phases 1-6 implemented, manifest closed out | agent |
