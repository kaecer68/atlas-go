# Audit Manifest: Agent Discipline Guardrails

> **Audit source**: User reports agents hallucinating during audits, losing focus during implementation, failing to use worktrees/stash/PR discipline, and polluting knowledge with ad-hoc files.
> **Goal**: Harden Atlas agent workflow skills and templates so every debugging/implementation session follows: investigate-before-conclude, plan-before-code, worktree isolation, per-issue commits, PR-based integration, and post-merge documentation governance.
> **Scope**: Modify project workflow skills (`atlas-audit-manifest-protocol`, `atlas-pre-change-protocol`, session-start skill) and templates; add `.omo/` governance automation; clean up existing `.omo/` violations.
> **Created**: 2026-07-16
> **Status**: in-progress

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| A01 | Agent jumps to conclusions during audit/debugging without verifying facts. | Audit-manifest skill lacks explicit "evidence before hypothesis" gate and does not mandate systematic-debugging skill loading. | `.claude/skills/atlas-audit-manifest-protocol/SKILL.md` | Skill requires loading `systematic-debugging` and recording evidence links before marking hypothesis accepted. | done | none | commit 48abc45a adds Phase A evidence gate and red flag |
| A02 | Audit findings are not linked to concrete code evidence. | Manifest template has no mandatory "Evidence" column linking to gitnexus/grep/test output. | `docs/manifests/TEMPLATE.md` | Every invariant row requires an Evidence link or command before status can move to `accepted`. | done | none | commit 48abc45a adds Phase A Evidence column and status values |
| A03 | Agents skip planning and start coding immediately. | `atlas-pre-change-protocol` does not enforce a TODO list / plan mode entry before edits when triggered by audit. | `.claude/skills/atlas-pre-change-protocol/SKILL.md` | Protocol requires TODO list + plan mode (or manifest binding) before first file edit in audit-derived work. | done | none | commit 8599e494 binds session to manifest/TODO before edits |
| A04 | Agents work in main worktree and push directly to `main`. | No workflow skill mandates `using-git-worktrees` for implementation work. | `.claude/skills/atlas-pre-change-protocol/SKILL.md`, `.claude/skills/atlas-audit-manifest-protocol/SKILL.md` | Both skills require opening a feature branch in a dedicated worktree before any implementation. | done | none | commit 8599e494 mandates worktree + feature branch |
| A05 | Stash pollution and unclear state. | No project protocol tells agents to list, label, or clear stashes before/after sessions. | `.claude/skills/atlas-pre-change-protocol/SKILL.md` | Session-start checklist includes `git stash list`; session-end checklist requires either applying+committing or dropping stashes. | done | none | commit 9b9d7199 adds Session-End Cleanup stash governance |
| A06 | Agents lose track of original goal mid-implementation. | No periodic re-focus prompt exists during long implementation sessions. | `.claude/skills/atlas-pre-change-protocol/SKILL.md` (or new skill) | Add mid-session re-focus checkpoint triggered every N turns or after error bursts. | done | none | commit 0acbf1d7 adds Mid-Session Re-focus Checkpoint |
| A07 | Agents poach work from other CLI sessions. | Session-start skill does not lock scope to the current manifest/branch. | `.claude/skills/atlas-pre-change-protocol/SKILL.md` | Session-start binds agent to current branch, manifest file, and issue IDs; warns when asked to touch files outside scope. | done | none | commit 8599e494 records branch/manifest/IDs and warns on scope drift |
| A08 | Manifest close-out is not externally validated. | Manifest only contains self-reported status; no script/CI checks that acceptance criteria actually ran. | `scripts/verify-manifest.sh` (new), `.github/workflows/` (optional) | A shell script parses the manifest, verifies each "done" ID has a commit hash, CI link, or test command output. | done | new-doc | commit 375b2404 creates `scripts/verify-manifest.sh` and `docs/manifests/README.md`; verified with test run |
| A09 | Documentation governance rules exist but are not enforced by workflow skills. | `docs/documentation-standard.md` rules are not referenced in audit/pre-change skills or manifest template. | `.claude/skills/atlas-audit-manifest-protocol/SKILL.md`, `docs/manifests/TEMPLATE.md` | Manifest template adds "Documentation Impact" column; close-out checklist requires deleting `.omo/plans/` and promoting stable knowledge to `docs/`. | done | promote-to-docs | commit 9ad93ca2 wires docs governance into audit skill, template, and red flags |
| A10 | Existing `.omo/` structure already violates governance rules. | Legacy `handoff/`, `research/`, and standalone files were created before the standard. | `.omo/handoff/`, `.omo/research/`, `.omo/2026-07-11-agents-md-audit.md` | Remove or relocate files per `docs/documentation-standard.md`; directory matches allowed categories. | done | plan-delete | commit f2d11837 moves standalone audit to `docs/audit/` and removes empty `handoff/` and `research/` dirs |

---

## Phase Tracker

### Phase A — Audit (read-only)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Confirm existing mechanisms (`docs/documentation-standard.md`, `docs/documentation-map.md`, skills) | A09 | done | Read files; rules exist but not wired into workflow. |
| Identify gaps between rules and enforcement | A01-A08 | done | See Invariant Tracker above. |
| Confirm existing `.omo/` violations | A10 | done | `ls -la .omo/` shows `handoff/`, `handoffs/`, `research/`, and standalone audit file. |

### Phase B — Plan

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Map each ID to files + changes | all | done | Implementation plan approved |
| Define acceptance criteria | all | done | Per-row acceptance in Invariant Tracker |
| Review blast radius | all | done | Skills and templates only; no production code paths affected. |

### Phase C — Implement

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Phase 0: Document-governance hardening | A09 | done | 9ad93ca2 |
| Phase 1: Session-start scope lock + worktree mandate | A04, A07 | done | 8599e494 |
| Phase 2: Evidence-before-conclusion in audit skill | A01, A02 | done | 48abc45a |
| Phase 3: Stash governance protocol | A05 | done | 9b9d7199 |
| Phase 4: Manifest verification script | A08 | done | 375b2404 + `./scripts/verify-manifest.sh` pass |
| Phase 5: Mid-session re-focus checkpoint | A06 | done | 0acbf1d7 |
| Phase 6: Clean up `.omo/` violations | A10 | done | f2d11837 + `ls -la .omo/` clean |

### Phase D — Close Out

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Update manifest status | all | done | this update |
| Push branch / open PR | all | pending | PR # |
| Run CI / verify | all | pending | CI link |
| Delete `.omo/plans/` files for this audit | all | done | no `.omo/plans/` file created for this audit |
| Promote stable `.omo/briefs/` content to `docs/` | all | done | n/a for this audit |
| Verify no new unauthorized `.omo/` directories | all | done | `ls .omo/` matches whitelist |

---

## Backlog

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|---------------|----------------|
| B01 | Consider adding a GitHub Action that rejects direct push to `main` or enforces manifest presence on PRs. | 2026-07-16 | Future — out of scope for this manifest (requires repository admin/CI changes). |
| B02 | Consider adding a `gitnexus` query that detects files modified outside the current manifest scope during a session. | 2026-07-16 | Future — out of scope. |

> **Rule**: only move one backlog item into scope per session, and only after all current IDs are done or paused.

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- One commit per ID group (logical phase)
- No commit without acceptance criteria passing for that phase
- PR body must reference this manifest: `See docs/manifests/2026-07-16-agent-discipline-guardrails.md`

---

## Session-End State

- **Done this session**: A01-A10 implemented and verified.
- **Remaining**: Push branch, open PR, wait for CI.
- **Next action**: Open PR referencing this manifest.
- **Uncommitted code**: no
- **Branch / PR**: `feat/agent-discipline-guardrails` / PR #TBD
- **Paused because**: waiting for PR/CI.

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-16 | 1.0 | Initial manifest | agent |
