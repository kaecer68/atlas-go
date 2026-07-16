# Agent Confuses Development and Production Environments

- **Discovered**: 2026-07-16
- **Related incident**: GCP Cloud Run deployment destroyed because agent applied dev-debug commands to production environment
- **Prevention**: Environment-isolation rules in `atlas-pre-change-protocol`; deny-dangerous hook blocks dev commands when `ATLAS_ENV=production`; separate worktrees per environment

## Symptom

An agent was asked to debug the development environment. It executed commands that also affected a deployed production instance on GCP Cloud Run. The production state became inconsistent and had to be destroyed and rebuilt from scratch.

## Root Cause

1. The agent inferred environment from conversational context rather than from explicit, enforced boundaries.
2. Production and development shared the same credentials/mental model.
3. There was no hard rule preventing a dev-oriented command from running when `ATLAS_ENV=production`.

## Prevention

1. **Always record environment in session start**: `atlas-pre-change-protocol` requires noting `ATLAS_ENV` and worktree before any edit.
2. **Use separate worktrees for prod**: Production worktrees are named clearly (e.g., `atlas-prod`) and run with `ATLAS_ENV=production`.
3. **Run deny-dangerous hook**: Before any state-changing command, run `./agent-guard --check '<command>'`. It blocks dev commands in production worktrees.
4. **Never ask an agent to "debug" without specifying environment**: Scope every debugging request to a specific worktree and env.

## Evidence

- `docs/environment.md` documents the dev environment setup.
- `.agent-hooks/deny-dangerous.sh` blocks commands like `docker compose up` and experiment CLIs when `ATLAS_ENV=production`.
