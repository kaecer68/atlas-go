#!/usr/bin/env bash
# .agent-hooks/aci-read-prompt.sh — PreToolUse hook for ACI routing compliance.
#
# Usage: registered as PreToolUse hook in .claude/settings.local.json
#        via `.agent-hooks/install.sh`.
#
# Design: soft reminder (NOT a hard block). When the AI agent attempts to
# Read/Edit/Write/Grep/Bash on a hot-path Go file (under internal/ or cmd/),
# the hook injects a ~150-token routing hint into the model's context
# pointing it at the existing ACI tools (gitnexus_impact,
# codebase-memory_explore, atlas-pre-change-protocol skill). The agent
# retains full autonomy — the hook never denies the call.
#
# Per-file dedup: each (tool_name, file_path) pair is prompted at most once
# per Claude Code session. State lives in $XDG_CACHE_HOME/atlas-aci-prompted/
# so it survives across CLI restarts but is naturally scoped per session.
#
# Requirements:
#   - bash 3.2+ (uses arrays, [[, etc.)
#   - jq 1.6+  (read JSON tool input from stdin)
#   - git      (resolve internal/cmd/ paths to repo root)
#
# Why soft-only: hard-blocking Read/Edit would prevent the agent from ever
# reading source code, which contradicts the goal. The 3 sister hard-block
# hooks in this repo (deny-dangerous.sh, .githooks/pre-push, deny-pr-merge)
# are reserved for destructive / irreversible actions. ACI compliance is a
# guidance problem, not a safety problem.
#
# See docs/archive/2026-08-06-aci-pretooluse-prompt-plan.md for design rationale.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# ─── 1. Read JSON input from stdin ─────────────────────────────
INPUT="$(cat)"

# jq is required. If missing, fall back silently (no prompt) rather than
# crash the agent session — the existing session-start.sh handles other
# prerequisite checks; we mirror its graceful-degradation pattern.
if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

TOOL_NAME="$(printf '%s' "$INPUT" | jq -r '.tool_name // ""')"
SESSION_ID="$(printf '%s' "$INPUT" | jq -r '.session_id // "unknown"')"

# Sanitize session_id for filesystem use.
SAFE_SESSION="$(printf '%s' "$SESSION_ID" | tr -c '[:alnum:]._-' '_')"

# ─── 2. Decide whether the call touches a hot-path Go file ─────
HIT=0
FILE_KEY=""

case "$TOOL_NAME" in
  Read|Edit|Write)
    # tool_input.file_path is the canonical field; .path is a fallback
    # for some tool variants. Both Read and Edit/Write use file_path.
    FILE_PATH="$(printf '%s' "$INPUT" | jq -r '.tool_input.file_path // .tool_input.path // ""')"
    # Normalize: strip leading ./ if any
    FILE_PATH="${FILE_PATH#./}"
    # Must end in .go and live under internal/ or cmd/ at the repo root.
    case "$FILE_PATH" in
      internal/*.go|cmd/*.go)
        HIT=1
        FILE_KEY="$TOOL_NAME:$FILE_PATH"
        ;;
    esac
    ;;
  Grep)
    # Grep tool_input has both .pattern and .path. We trigger if the path
    # is under internal/ or cmd/ (regardless of pattern, because any
    # grep into hot-path code without first running gitnexus is the
    # common anti-pattern this hook addresses).
    GREP_PATH="$(printf '%s' "$INPUT" | jq -r '.tool_input.path // ""')"
    GREP_PATH="${GREP_PATH#./}"
    case "$GREP_PATH" in
      internal/*|cmd/*)
        HIT=1
        FILE_KEY="$TOOL_NAME:$GREP_PATH"
        ;;
    esac
    ;;
  Bash)
    # Bash is broad. Only trigger when the command clearly searches inside
    # internal/ or cmd/ (grep/rg/find). Running unit tests, builds, or
    # `go vet` against those paths is normal — we don't nag on those.
    CMD="$(printf '%s' "$INPUT" | jq -r '.tool_input.command // ""')"
    # Match either:
    #   (a) explicit .go file path under internal/ or cmd/  → narrow
    #   (b) bare directory path (grep -r foo internal/orchestrator/) → broad
    # We dedup by whichever matched first; the dedup table scopes by
    # session so cross-call frequency is bounded.
    MATCHED_PATH="$(printf '%s' "$CMD" | grep -oE '\b(internal|cmd)/[A-Za-z0-9_./-]*' | head -n 1 || true)"
    if [ -n "$MATCHED_PATH" ] && printf '%s' "$CMD" | grep -qE '(^|[[:space:]])(grep|rg|find)[[:space:]]'; then
      HIT=1
      FILE_KEY="$TOOL_NAME:$MATCHED_PATH"
    fi
    ;;
esac

if [ "$HIT" -ne 1 ]; then
  exit 0
fi

# ─── 3. Per-session dedup ──────────────────────────────────────
CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/atlas-aci-prompted"
CACHE_FILE="$CACHE_DIR/${SAFE_SESSION}.list"

# Sanity: cap dedup table at 500 entries to prevent unbounded growth in
# long sessions. If we exceed, rotate (rename to .list.1 and start fresh).
mkdir -p "$CACHE_DIR"
if [ -f "$CACHE_FILE" ]; then
  LINE_COUNT="$(wc -l < "$CACHE_FILE" 2>/dev/null || echo 0)"
  if [ "$LINE_COUNT" -gt 500 ]; then
    mv "$CACHE_FILE" "$CACHE_FILE.1" 2>/dev/null || true
  fi
fi

if [ -f "$CACHE_FILE" ] && grep -qxF "$FILE_KEY" "$CACHE_FILE" 2>/dev/null; then
  exit 0
fi

# ─── 4. Emit soft reminder via additionalContext ───────────────
# Note: For SessionStart / UserPromptSubmit, plain stdout is enough.
# For PreToolUse we MUST return the JSON envelope with
# hookSpecificOutput.additionalContext, otherwise the string is ignored.
# (See code.claude.com/docs/en/hooks § PreToolUse decision control.)
#
# The reminder is ~150 tokens, derived directly from:
#   - .claude/skills/atlas-pre-change-protocol/SKILL.md (Steps 0/1/1.5)
#   - .claude/SKILLS-MAP.md (routing tool table)
#   - internal/AGENTS_INDEX.md (15 hot-path modules with AGENTS.md)
# No new conventions are introduced; this is curation, not invention.

CONTEXT_TEXT='AC: atlas-go ACI routing — 你正要讀/改/搜 hot-path Go 檔(屬 internal/ 或 cmd/)。先想一下:

  Step 0 (overlap):  已有對等實作嗎?
                     → gitnexus_query(query="<concept>")
                     → 或 codebase-memory_search_graph(query="<concept>")

  Step 1 (blast radius):  改這個 symbol 會影響誰?
                          → gitnexus_impact(target="<symbol>", direction="upstream")

  Step 1.5 (source context): 要看 caller source code?
                            → codebase-memory_explore(query="<symbol>")

  hot-path 模組 (有 internal/<mod>/AGENTS.md): apigateway / capitalflow / fubonproxy / live / llm / marketdata / monitoring / orchestrator / strategy_techniques / admin_web / client_web / cmd/atlas-mcp / cmd/experimental — 先讀該 AGENTS.md 再改。

  完整 8 步(必跑): 載入 atlas-pre-change-protocol skill。

  本提醒每檔每 session 只一次,不會再打擾。'

jq -n --arg ctx "$CONTEXT_TEXT" \
  '{ hookSpecificOutput: { hookEventName: "PreToolUse", additionalContext: $ctx } }'

# ─── 5. Record dedup key ───────────────────────────────────────
# Append-only; grep -qxF above makes this O(1) for the dedup check
# at session scale. If the line count explodes across very long
# sessions, the rotate above keeps the table bounded.
printf '%s\n' "$FILE_KEY" >> "$CACHE_FILE"

exit 0
