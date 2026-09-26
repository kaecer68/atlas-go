#!/usr/bin/env bash
# .agent-hooks/pretooluse-deny-dangerous.sh — thin PreToolUse adapter for deny-dangerous.sh.
#
# ── Why this file exists (E5, 2026-09-26) ────────────────────────────────────
# `deny-dangerous.sh` was a complete guard that **nothing ever called
# automatically**. AGENTS.md only asked agents to run `./agent-guard --check`
# by hand, and `.agent-hooks/install.sh` explicitly stops short of registering a
# hook ("cp then add the PreToolUse entry manually"). A guard that is not on the
# effective path is a dead gate: it only fires when the very agent it is meant to
# constrain remembers to call it.
#
# This adapter puts it on the path. It is registered in `.claude/settings.json`
# (repo-shared, tracked) as a `PreToolUse` hook with matcher `Bash`, so every
# Claude Code Bash tool call in this repo passes through `deny-dangerous.sh`.
#
# ── Design: one source of truth ──────────────────────────────────────────────
# ALL pattern logic stays in `deny-dangerous.sh`. This file is glue only:
#   (a) parse the Claude Code hook payload from stdin,
#   (b) take `.tool_input.command` when `.tool_name == "Bash"`,
#   (c) run `deny-dangerous.sh --check "<command>"` (existing CLI, unchanged),
#   (d) translate the guard's verdict into the Claude Code hook protocol.
# If you want to add/remove a dangerous pattern, edit `deny-dangerous.sh` only.
#
# ── Mode policy (conservative; the owner can tighten) ────────────────────────
# This adapter never picks a mode. `deny-dangerous.sh` does:
#   ATLAS_HOOK_MODE set          -> that mode wins
#   else ATLAS_ENV=production    -> enforce
#   else                         -> warn
# So the default in a dev worktree is `warn`: the model and the user get a clear
# message, but the Bash call still runs (no surprise blocking of normal work).
#
# To block locally (no file edit, per session / per shell):
#   export ATLAS_HOOK_MODE=enforce
# To make a whole host enforce (production boxes already do this via ATLAS_ENV):
#   export ATLAS_HOOK_MODE=enforce     # in the shell that starts the agent
#
# ── Verdict mapping ──────────────────────────────────────────────────────────
#  guard warns  (marker `WARNING (warn mode):`)  -> exit 0, guard text is injected
#      into the model context (`additionalContext`) and shown to the user
#      (`systemMessage`). Visibility only.
#  guard denies (marker `DENIED (enforce mode):`) -> message on stderr, exit 2.
#      Exit 2 is Claude Code's "block this tool call" code. Other non-zero codes
#      would only be reported as a non-blocking hook error.
#  guard failed / verdict unreadable -> FAIL-OPEN: exit 0 plus a loud stderr
#      notice. A broken guard must not brick every Bash call in the session; the
#      wiring itself is proven by tests/scripts/test-agent-hook-wiring.sh instead.
#
# ── Contract (asserted by tests/scripts/test-agent-hook-wiring.sh) ───────────
#   * `.claude/settings.json` registers this file under PreToolUse / matcher Bash
#   * both verdict markers above still exist inside `deny-dangerous.sh`
#   * `git status`, `ls`, `make ci-gate` pass; `rm -rf /` warns in warn mode and
#     exits 2 in enforce mode; a non-Bash tool and a malformed payload pass
#
# ── Environment overrides (tests / unusual hosts) ────────────────────────────
#   ATLAS_HOOK_PYTHON   interpreter used to parse the payload (default: python3)
#   ATLAS_HOOK_BASH     interpreter used to run the guard (default: bash)
# Both interpreters are needed: python3 parses JSON (the repo already requires
# python3 for its script tests), bash runs the guard. If either is missing the
# hook fails open with a notice — never silently, never blocking.
#
# Exit codes: 0 = allow (also used for warn and for fail-open), 2 = deny.
#
# Manual smoke test:
#   printf '{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}' \
#     | .agent-hooks/pretooluse-deny-dangerous.sh ; echo "rc=$?"
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="${SCRIPT_DIR}/deny-dangerous.sh"

PYTHON="${ATLAS_HOOK_PYTHON:-python3}"
GUARD_BASH="${ATLAS_HOOK_BASH:-bash}"

# Verdict markers, copied verbatim from deny-dangerous.sh's block() messages.
# They are the machine-readable half of the guard's output contract; the wiring
# test asserts they still exist there, so drift turns CI red instead of silently
# degrading this adapter to "always allow".
DENY_MARKER="DENIED (enforce mode):"
WARN_MARKER="WARNING (warn mode):"

BLOCK_EXIT=2

notice() {
  printf '%s %s\n' "[agent-guard PreToolUse]" "$1" >&2
}

# ─── 1. The hook payload arrives on stdin as JSON ────────────────────────────
PAYLOAD="$(cat)"

if ! command -v "$PYTHON" >/dev/null 2>&1; then
  notice "python interpreter '$PYTHON' not found - the guard did NOT run (fail-open)."
  exit 0
fi

TOOL_NAME="$(printf '%s' "$PAYLOAD" | "$PYTHON" -c '
import json
import sys

try:
    payload = json.load(sys.stdin)
except Exception:
    sys.exit(3)

if not isinstance(payload, dict):
    sys.exit(3)

value = payload.get("tool_name")
print(value if isinstance(value, str) else "")
')"
parse_rc=$?

if [ "$parse_rc" -ne 0 ]; then
  # Malformed / unexpected payload: report loudly instead of silently skipping
  # (a guard that quietly stops running is the bug this whole file fixes).
  notice "hook payload is not valid JSON (rc=$parse_rc) - guard did NOT run (fail-open). The Claude Code payload shape may have changed."
  exit 0
fi

# Only Bash tool calls are guarded. Everything else is none of our business.
if [ "$TOOL_NAME" != "Bash" ]; then
  exit 0
fi

COMMAND="$(printf '%s' "$PAYLOAD" | "$PYTHON" -c '
import json
import sys

try:
    payload = json.load(sys.stdin)
except Exception:
    sys.exit(3)

tool_input = payload.get("tool_input")
command = tool_input.get("command") if isinstance(tool_input, dict) else None
print(command if isinstance(command, str) else "")
')"
parse_rc=$?

if [ "$parse_rc" -ne 0 ]; then
  notice "could not read .tool_input.command from the payload (rc=$parse_rc) - guard did NOT run (fail-open)."
  exit 0
fi

if [ -z "$COMMAND" ]; then
  exit 0
fi

if [ ! -f "$GUARD" ]; then
  notice "guard not found at '$GUARD' - guard did NOT run (fail-open)."
  exit 0
fi

# ─── 2. Delegate the verdict to the guard (single source of truth) ───────────
GUARD_OUTPUT="$("$GUARD_BASH" "$GUARD" --check "$COMMAND" 2>&1)"
guard_rc=$?

# ─── 3. Translate the verdict ────────────────────────────────────────────────
case "$GUARD_OUTPUT" in
  *"$DENY_MARKER"*)
    printf '%s\n' "$GUARD_OUTPUT" >&2
    notice "blocked this Bash call (enforce mode, exit $BLOCK_EXIT). To allow it for now, set ATLAS_HOOK_MODE=warn in the environment that starts the agent."
    exit "$BLOCK_EXIT"
    ;;
  *"$WARN_MARKER"*)
    # Warn mode: do not block. Surface the guard's own text to both the model
    # (additionalContext) and the user (systemMessage) so the risk is visible.
    GUARD_TRIMMED="$(printf '%s\n' "$GUARD_OUTPUT" | sed -e '/./,$!d')"

    CONTEXT_ONE="[agent-guard] PreToolUse guard flagged this Bash command (warn mode - NOT blocked).

$GUARD_TRIMMED

To make this class of command actually block, set ATLAS_HOOK_MODE=enforce (or ATLAS_ENV=production) for the agent session."

    USER_ONE="[agent-guard] warn: flagged a possibly dangerous Bash command (NOT blocked): ${COMMAND:0:200}"

    if ! "$PYTHON" -c '
import json
import sys

context, user_message = sys.argv[1], sys.argv[2]
print(json.dumps({
    "hookSpecificOutput": {
        "hookEventName": "PreToolUse",
        "additionalContext": context,
    },
    "systemMessage": user_message,
}, ensure_ascii=False))
' "$CONTEXT_ONE" "$USER_ONE"; then
      notice "failed to emit the warn JSON envelope (guard verdict was warn = allow)."
    fi
    exit 0
    ;;
esac

# No marker: either the guard allowed the command (rc 0 + no output) or the
# guard itself failed. A failing guard must not become a blanket deny.
if [ "$guard_rc" -ne 0 ]; then
  notice "guard exited $guard_rc without a verdict marker - fail-open, NOT blocking. Guard output follows:"
  printf '%s\n' "$GUARD_OUTPUT" >&2
fi

exit 0
