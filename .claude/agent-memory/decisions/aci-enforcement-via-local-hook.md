# ACI Enforcement via Per-User Local PreToolUse Hook

- **Date**: 2026-08-06
- **Status**: active
- **Supersedes**: (none)

## Context

`atlas-pre-change-protocol` is a passive skill: the AI agent has to
remember to load it (`skill(name="atlas-pre-change-protocol")`) and
walk through its 8 steps before any code change. In practice, when
the user asks a quick "fix this" question, agents skip the skill
load and dive straight into `Read` / `Edit`, which leads to the
recurring footgun `agent-skips-aci-routing` (parallel
implementations, missed blast-radius, factually wrong answers about
existing systems).

The existing `.githooks/pre-commit` and `.githooks/pre-push` only
catch the **output** of agent work (binary pollution, push-to-main).
The existing `.agent-hooks/deny-dangerous.sh` is a hard block for
**destructive / secrets / production** operations. None of these
catch the **input** side — the agent's choice of which tool to
call next.

We need a **soft, deterministic, file-access-time** reminder that
puts the existing ACI routing tools (gitnexus_impact,
codebase-memory_explore, etc.) top-of-mind without blocking the
agent's ability to read or edit code.

## Decision

Add a new `.agent-hooks/aci-read-prompt.sh` PreToolUse hook that:

1. **Triggers on**: `Read` / `Edit` / `Write` of `*.go` under
   `internal/` or `cmd/`; `Grep` with `path` under `internal/` or
   `cmd/`; `Bash` containing `grep` / `rg` / `find` with a path
   argument under `internal/` or `cmd/`. All other tool calls pass
   through unchanged.
2. **Injects** a `hookSpecificOutput.additionalContext` string
   (~150 tokens) pointing the agent at:
   - Step 0 → `gitnexus_query` / `codebase-memory_search_graph`
   - Step 1 → `gitnexus_impact`
   - Step 1.5 → `codebase-memory_explore`
   - 15 hot-path modules' local `AGENTS.md`
   - The `atlas-pre-change-protocol` skill (8-step reminder)
3. **Per-session, per-file dedup** so the agent isn't reminded
   every time it touches the same file in the same session.
4. **Does not block** the tool call. The agent retains full
   autonomy; the reminder is a nudge, not a wall.
5. **Per-user opt-in** via `.claude/settings.local.json`
   (gitignored), not the team-shared `.claude/settings.json`. The
   hook script and its companion `README.md` / agent-memory
   entries are committed; the registration entry stays local.

## Rationale

**Why soft prompt, not hard block?**

- Hard-blocking `Read` / `Edit` would prevent the agent from
  reading source code at all, which is its primary job. The goal
  is to push the agent toward the right tool *first* (gitnexus,
  codebase-memory), not to prevent reading. Hard blocks are
  reserved for "actions you can't take back" — the same
  principle that `deny-dangerous.sh` already follows for
  destructive operations, secrets, and production-state changes.
- A soft prompt leaves a traceable signal: if the agent later
  produces duplicate work, the developer can see "the agent was
  reminded but chose to skip the routing" and discuss that
  behavior. A hard block has no such signal — the failure mode
  just becomes "the agent can't read code" with no negotiation.

**Why per-user `.local.json`, not team `.json`?**

- This is one developer's reinforcement of an existing
  convention (`atlas-pre-change-protocol`), not a new
  cross-cutting team rule. Forcing all atlas developers to
  receive this reminder would be premature without evidence
  that it actually reduces the footgun in their workflow.
- `.claude/settings.json` already carries team-shared config
  (e.g., the existing `SessionStart` binary-freshness hook).
  Mixing personal reminder hooks into it would dilute the
  signal value of the team config.
- The official Claude Code convention is that
  `settings.local.json` is gitignored and per-user, which
  matches the intent.

**Why jq for stdin parsing?**

- The hook protocol is JSON-on-stdin (see
  `code.claude.com/docs/en/hooks`). `jq` is the most portable
  JSON processor with native support on macOS via Homebrew.
- The hook **gracefully degrades** when `jq` is missing: it
  exits 0 without emitting a reminder, instead of crashing
  the agent session. This mirrors the existing
  `scripts/session-start.sh` pattern.

**Why dedup at session scope (not turn scope)?**

- A turn scope would re-prompt every time the agent re-reads
  a file, which is common when iterating on a fix. Session
  scope is the right grain: once reminded about file X in
  this session, the agent has had the chance to act on it.
- Session id is provided in the hook input, so dedup state
  lives at `${XDG_CACHE_HOME:-~/.cache}/atlas-aci-prompted/${session_id}.list`
  and naturally rolls over when the session ends.

## Consequences

- **Positive**: agents running in this developer's worktree
  get a top-of-mind nudge to use the right ACI tool before
  reading / editing hot-path Go code. The reminder is curated
  from `atlas-pre-change-protocol`, not invented, so it stays
  consistent with the existing skill.
- **Positive**: the reminder is observable in the session
  transcript (the additionalContext is visible), so the
  developer can audit whether the agent acted on it.
- **Positive**: opt-in via `.local.json` means the rest of
  the team is unaffected until someone explicitly tries the
  hook and confirms it works for them.
- **Negative**: 13 of 13 tested scenarios pass, but Bash
  detection uses a narrow regex. A sufficiently obfuscated
  `Bash` call (e.g. `bash -c "grep ..."` with environment
  variable indirection) could evade the check. Acceptable
  for a soft layer; hard-block is `deny-dangerous.sh`'s job.
- **Negative**: the hook lives in the user's `.local.json`
  and won't activate for new clones until the user copies
  the registration. Mitigated by clear install-time messaging
  in `install.sh` and `docs/operations/aci-hook-usage.md`.

## References

- `atlas-pre-change-protocol` SKILL — the 8-step protocol
  this hook reminds about.
- `.agent-hooks/deny-dangerous.sh` — the hard-block pattern
  this hook deliberately does NOT follow.
- `docs/multi-cli-protocol.md` — context for why per-user
  `.local.json` fits the multi-CLI workflow.
- `code.claude.com/docs/en/hooks` — PreToolUse decision
  control schema (`hookSpecificOutput.additionalContext`).
- `.omo/plans/2026-08-06-aci-pretooluse-prompt.md` —
  full design plan and 13-scenario validation.
- `.claude/agent-memory/footguns/agent-skips-aci-routing.md`
  — the footgun this hook mitigates.
