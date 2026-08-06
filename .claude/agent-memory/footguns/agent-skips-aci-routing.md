# Agent Skips ACI Routing and Reinvents or Mis-Patches Code

- **Discovered**: 2026-08-06
- **Related incident**: Recurring pattern across sessions — agent answers
  project-specific questions, edits hot-path modules, or adds new
  functions/types without first calling gitnexus_query / gitnexus_impact /
  codebase-memory_explore, leading to duplicate implementations, missed
  blast-radius, or factually wrong answers about existing systems.
- **Prevention**: `atlas-pre-change-protocol` mandates Steps 0/1/1.5/6
  before any code change. The new `.agent-hooks/aci-read-prompt.sh` is a
  **soft reminder layer** that injects a routing hint into the model
  context whenever the agent touches `internal/` or `cmd/` Go files,
  nudging it toward those existing tools.

## Symptom

The agent:

1. Adds a new function/type/module without running
   `gitnexus_query` to check whether similar functionality already
   exists. Result: a parallel duplicate implementation surfaces later
   in code review, or two slightly-different algorithms co-exist and
   drift apart (classic `traps.md § 同一件事不可有三種算法` violation).
2. Edits a function without first running `gitnexus_impact(target,
   direction="upstream")`. Result: callers break in production; the
   fix needs a second PR to repair the regression.
3. Greps `internal/<mod>/` directly with bash/grep to find callers,
   instead of asking `codebase-memory_explore` for the structured
   result. Result: noisy matches miss reflective / config-driven
   reference paths (`internal/<mod>/AGENTS.md` lists many of these).
4. Answers user questions about whether a feature exists by
   guessing, instead of running `gitnexus_query` or
   `codebase-memory_search_graph`. Result: the agent confidently says
   "no, that doesn't exist" or "yes, it's at <wrong path>" and the
   user wastes a turn correcting it (generalizes the
   `assumes-without-reading-docs` footgun).

## Root Cause

1. `atlas-pre-change-protocol` is a **passive** skill. The agent
   has to remember to load it (`skill(name="...")`) and follow the 8
   steps. When the agent is mid-task and the user just asked "fix
   this", loading an 8-step skill feels like overhead, so the agent
   skips it.
2. The skills catalogue (26 skills, see `SKILLS-MAP.md`) is large.
   Knowing which skill applies to which tool call requires
   pre-loaded routing knowledge, which new sessions don't always
   have.
3. Without a deterministic reminder at the moment of file access,
   there's no signal that the agent's mental "I'll just read this
   one file" path is the same anti-pattern as the documented footgun.

## Prevention

1. **Mandatory pre-tool-use reminder** (this PR):
   `.agent-hooks/aci-read-prompt.sh` runs as a `PreToolUse` hook and
   injects a ~150-token routing reminder into the model's context
   when the agent attempts to Read/Edit/Write a `*.go` file under
   `internal/` or `cmd/`, or runs a `grep`/`rg`/`find` against
   those paths. The reminder cites Steps 0/1/1.5 of the existing
   protocol and the 15 hot-path modules that have local
   `AGENTS.md` files. Per-session, per-file dedup keeps it from
   being noisy.
2. **Soft layer, not hard block**: the hook never denies the tool
   call. The agent retains autonomy; the reminder just puts the
   routing tools top-of-mind. Hard blocking would prevent the
   agent from reading source code at all, which contradicts the
   goal.
3. **Per-user opt-in**: the hook is registered in
   `.claude/settings.local.json` (gitignored), not the team-shared
   `.claude/settings.json`. The team isn't forced to adopt it; this
   is one developer's reinforcement of an existing convention.

## Evidence

- `atlas-pre-change-protocol` Steps 0/1/1.5/6 (the canonical
  routing rules this hook reminds about).
- `traps.md § 造輪子前先搜尋既有 infrastructure` (worked example
  showing the cost of skipping this check).
- `.claude/agent-memory/footguns/assumes-without-reading-docs.md`
  (same family of footgun, originally recorded 2026-07-16).
- `.omo/plans/2026-08-06-aci-pretooluse-prompt.md` (design
  rationale and 13-scenario validation log).
- `.agent-hooks/aci-read-prompt.sh` (implementation; tested
  2026-08-06 against 13 Read/Edit/Write/Grep/Bash scenarios).
