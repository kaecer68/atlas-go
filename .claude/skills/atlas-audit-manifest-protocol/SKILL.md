---
name: atlas-audit-manifest-protocol
description: "MUST use when debugging, auditing, investigating bugs, or doing code/design review follow-ups that lead to implementation. Triggers: 'why is X failing', 'trace this error', 'audit this code', 'review this and fix'. Connects root-cause investigation → invariant manifest → implementation plan → commit → PR. Prevents agents from guessing fixes, losing track of issues, and leaving unverified code in the working tree."
---

# Atlas Audit Manifest Protocol

**Iron Law: No fix without a recorded audit manifest.**
Every debugging session, bug hunt, or design review that may result in code changes must produce an audit manifest before implementation begins. The manifest is the single source of truth for what was found, what will be changed, and how it will be verified.

This skill does **not** replace root-cause investigation. It wraps the existing investigation skills (`systematic-debugging`, `gitnexus-debugging`) into a traceable workflow that survives context switches, agent handoffs, and multi-session work.

---

## When to Use

| Trigger | Example |
|---------|---------|
| Debugging / bug hunt | "why is X failing", "trace this error", "this endpoint returns 500" |
| Code / design audit | "audit this module", "review this code", "find what's wrong" |
| Review follow-up | "fix the issues from this review", "address these comments" |
| Investigation that might lead to changes | "look into why this data is stale" |

If the investigation is **purely read-only** with no expected changes, use `atlas-pre-change-protocol` Investigation Mode instead. If the user asks you to fix something, load this skill **immediately**.

---

## The Four Phases

```
Phase A — Audit (read-only)
  ├─ Load systematic-debugging + gitnexus-debugging
  ├─ Load atlas-pre-change-protocol Investigation Mode
  ├─ Record findings in a manifest with per-issue IDs
  └─ Every hypothesis must have evidence BEFORE it is marked accepted

Phase B — Plan (before code)
  ├─ Load writing-plans
  ├─ Bind each ID to files, changes, and acceptance criteria
  └─ New issues discovered → Backlog (do NOT expand scope)

Phase C — Implement (one ID at a time)
  ├─ One branch per logical change set
  ├─ One commit per issue: <type>(manifest): #<ID> <short>
  ├─ Run the acceptance criteria for that ID before marking done
  └─ Load atlas-pre-change-protocol before editing

Phase D — Close out
  ├─ Update manifest status columns
  ├─ Push branch, open PR, wait for CI
  ├─ Clean up merged branch (see atlas-pre-change-protocol Post-Merge Cleanup)
  └─ If unfinished: context-save or document state in manifest
```

---

## Manifest Format

Create the manifest at `docs/manifests/YYYY-MM-DD-<audit-name>.md` or `docs/manifests/<feature>-audit.md`.

Use the template at `docs/manifests/TEMPLATE.md`.

Minimum required sections:

| Section | Purpose |
|---------|---------|
| Goal | What this audit is trying to answer/fix |
| Invariant table | ID / problem / root-cause hypothesis / files / acceptance / status / evidence link |
| Phase tracker | Logical groupings of work with evidence |
| Backlog | New issues found but not in scope |
| Commit discipline | How each commit maps to IDs |
| Session-end state | What is done, what is left, next action |

---

## Red Flags — STOP and Return to Audit

Catch yourself thinking any of these? STOP.

- "I have a hypothesis" → Record it, then find evidence. A hypothesis without evidence is a guess.
- "I see the problem, let me fix it" → Seeing a symptom is not root cause. Document it first.
- "This is just a small fix" → Every fix gets an ID and a test.
- "I'll add multiple changes and run tests" → One ID per commit.
- "I found another issue, let me fix it too" → Add to Backlog.
- "I don't need to write a manifest" → This is the protocol you are reading. Use it.
- "I'll commit everything at the end" → Commit per ID as you go.
- "Tests should pass" → Run them and confirm.
- "I'll push to main to save time" → Always open a PR.
- "This session is too short for a manifest" → A short manifest is still a manifest.
- "The user didn't ask for a manifest" → This skill is mandatory for debugging/audit tasks.
- "I created a new `.omo/<dir>/` without checking the whitelist" → STOP. Read `docs/documentation-standard.md` § `.omo/` whitelist. New directories are forbidden unless the standard is updated in the same PR.

---

## Commit & PR Discipline

- **Commit format**: `<type>(manifest): #<ID> <short description>`
  - Examples: `fix(manifest): #B01 remove duplicated event rendering`, `feat(manifest): #C03 add direction probability distribution`
- **One commit per ID** unless the change is purely documentation or a single logical fix spans multiple files.
- **No commit without acceptance criteria passing** for that ID.
- **PR body must reference the manifest**: `See docs/manifests/YYYY-MM-DD-<audit-name>.md`
- **No direct push to main** — always open a PR and wait for CI green.

---

## Session-End Checklist

Before ending any session that used this protocol:

```
□ Manifest status column updated for every ID touched this session
□ All done IDs have passing acceptance criteria
□ Unfinished IDs have a clear next action and owner
□ No uncommitted implementation code in working tree
□ Backlog populated with any new issues found
□ Branch pushed or PR opened (if any code was changed)
□ If session must pause with uncommitted code: run context-save
□ If todo list was used this session: verify each item is a standalone string
  (run view after init; expected count = number of tasks, not 1 array-as-string)
□ Run ./scripts/check-binary-freshness.sh — exit 0 REQUIRED（先跑 make rebuild-host-bin rebuild-atlas-bins rebuild-cron-bins 純 go build；docker images 仍 stale 則回報 kaecer，禁止自行執行含 docker 的 target）。
  See ~/.agents/AGENTS.md "Binary freshness gate" and "Docker／正式服務禁令" for full procedure.
```

### Documentation Governance (Close-Out)

Every audit that produces ephemeral working files must reconcile them with `docs/documentation-standard.md` before the PR is considered complete:

```
□ If `.omo/plans/` files were created for this audit: delete them after merge
□ If `.omo/briefs/` content stabilized during the audit: promote to `docs/specs/` or `docs/guides/`, then delete the `.omo/` copy
□ If new long-term runbook/spec knowledge emerged: place it under `docs/` using the归属 table in `docs/documentation-standard.md`
□ No new `.omo/` subdirectories were created unless `documentation-standard.md` whitelist was updated in the same PR
□ Any standalone `.md` files created directly under `.omo/` were moved or deleted
```

If you cannot check all boxes, do not claim the work is done. Tell the user what is left and where the manifest lives.

---

## Relationship to Other Skills

| Skill | When |
|-------|------|
| `systematic-debugging` | Phase A — root cause investigation |
| `gitnexus-debugging` | Phase A — code-level tracing |
| `gitnexus-impact-analysis` | Phase B/C — blast radius before implementation |
| `atlas-pre-change-protocol` | Phase C — mandatory safety checks before editing |
| `writing-plans` | Phase B — detailed implementation plan |
| `test-driven-development` | Phase C — failing test before fix |
| `verification-before-completion` | Phase D — prove it works before claiming done |

---

## Example Session Flow

User: "The daily report shows RISK_ON but system health says RISK_OFF."

1. Load this skill.
2. Create `docs/manifests/2026-07-16-regime-divergence-audit.md`.
3. Phase A: `systematic-debugging` → `gitnexus-debugging` → identify root cause: `defaultProvider.FetchMacro()` hardcodes `GlobalOverview.Status = "RISK_ON"`.
4. Phase B: `writing-plans` → ID A02: inject real regime provider, acceptance: daily_report and system health agree.
5. Phase C: `atlas-pre-change-protocol` → implement → commit `fix(manifest): #A02 inject real regime source into daily report`.
6. Phase D: verify with curl, update manifest, push PR.

---

## Token Efficiency Notes

- This skill is intentionally short. All technical investigation details are delegated to the linked skills above.
- The manifest template is minimal. Do not expand it into a full project plan unless the audit is large.
- One manifest per coherent debugging/audit thread. Do not create a manifest for every one-line fix; do create one for any task with multiple issues, multiple files, or multiple sessions.
