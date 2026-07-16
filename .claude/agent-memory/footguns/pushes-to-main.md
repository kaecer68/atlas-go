# Agent Pushes Directly to main

- **Discovered**: 2026-07-16
- **Related incident**: Repeated incidents where agents pushed implementation commits directly to `main`, bypassing PR/CI review
- **Prevention**: `atlas-pre-change-protocol` session-start mandates worktree + feature branch; `.githooks/pre-push` blocks direct main pushes; deny-dangerous hook also catches `git push origin main`

## Symptom

Agents working on implementation tasks would commit directly to the `main` branch and push. This bypassed PR review, CI checks, and the audit manifest discipline. Cleanup required reverting commits, rebuilding branches, and re-running CI.

## Root Cause

1. `main` was treated as a workspace rather than a protected branch.
2. There was no hard technical block preventing `git push origin main`.
3. Session-start discipline was instruction-based and not enforced.

## Prevention

1. **Worktree mandate**: `atlas-pre-change-protocol` requires opening a feature branch in a dedicated worktree for any implementation work.
2. **Git pre-push hook**: `.githooks/pre-push` blocks pushes to `main` and redundant empty-diff pushes.
3. **Agent hook**: `.agent-hooks/deny-dangerous.sh` blocks `git push origin main` in enforce mode.
4. **Post-merge cleanup**: After every merge, delete the feature branch and worktree to prevent reuse.

## Evidence

- `.githooks/pre-push` enforces the rule at git level.
- `.agent-hooks/deny-dangerous.sh` provides agent-level enforcement.
- `docs/multi-cli-protocol.md` documents post-merge cleanup.
