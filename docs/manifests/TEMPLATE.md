# Audit Manifest: <name>

> **Audit source**: <who asked / what triggered this audit>
> **Goal**: <one sentence describing the outcome>
> **Scope**: <what is in scope and what is explicitly out of scope>
> **Created**: YYYY-MM-DD
> **Status**: <in-progress / done / paused>

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| A01 | <short description> | <hypothesis, not conclusion> | <exact file paths> | <how to verify> | pending | none / plan-delete / promote-to-docs / new-doc | <link to evidence> |

---

## Phase Tracker

### Phase A — Audit (read-only)

Status values: `pending` → `hypothesis` → `accepted` | `rejected`. No hypothesis may move to `accepted` without a concrete evidence link.

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Reproduce the symptom | - | pending | <commands / URLs / logs> |
| Identify suspect code | - | pending | <gitnexus / grep / log evidence> |
| Form root cause hypothesis | - | pending | <which hypothesis, why> |
| Validate hypothesis with evidence | - | pending | <commit / test / trace that proves or disproves> |

### Phase B — Plan

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Map each ID to files + changes | - | pending | <writing-plans output> |
| Define acceptance criteria | - | pending | <test / curl / UI check> |
| Review blast radius | - | pending | <gitnexus-impact-analysis> |

### Phase C — Implement

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| <change 1> | A01 | pending | <commit hash> |

### Phase D — Close Out

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Update manifest status | - | pending | - |
| Push branch / open PR | - | pending | <PR #> |
| Run CI / verify | - | pending | <CI link> |
| Delete `.omo/plans/` files for this audit | - | pending | `ls .omo/plans/` |
| Promote stable `.omo/briefs/` content to `docs/` | - | pending | <target doc path> |
| Run `scripts/cleanup-manifests.sh` to check stale manifests | - | pending | <output> |
| **Decide manifest fate**: archive (teaching value), promote (spec-level invariants), or delete | - | pending | see `docs/documentation-standard.md` §Manifest 完成後 Promotion 路徑 |
| Verify no new unauthorized `.omo/` directories | - | pending | `ls .omo/` vs whitelist |

---

## Backlog

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|---------------|----------------|
| - | <new issue found but not in scope> | YYYY-MM-DD | <which phase to address> |

> **Rule**: only move one backlog item into scope per session, and only after all current IDs are done or paused.

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- One commit per ID
- No commit without acceptance criteria passing
- PR body must reference this manifest: `See docs/manifests/<this-file>.md`

---

## Session-End State

- **Done this session**: <IDs completed>
- **Remaining**: <IDs still open>
- **Next action**: <concrete next step>
- **Uncommitted code**: yes / no
- **Branch / PR**: <branch name> / <PR #>
- **Paused because**: <reason, if any>

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| YYYY-MM-DD | 1.0 | Initial manifest | <agent / owner> |
