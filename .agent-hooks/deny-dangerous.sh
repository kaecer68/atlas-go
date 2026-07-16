#!/usr/bin/env bash
# deny-dangerous.sh — deterministic guardrails for AI agent shell commands.
#
# Usage:
#   .agent-hooks/deny-dangerous.sh --check "<command>"
#   .agent-hooks/deny-dangerous.sh --dry-run
#   .agent-hooks/deny-dangerous.sh --mode=enforce --check "<command>"
#
# Design: this script is a hard boundary, not a suggestion. Agents MUST run it
# before executing any command that modifies state, reads secrets, or touches
# production. It exits non-zero when the command is blocked.
#
# Severity modes:
#   warn    — print a warning but allow (default for local dev worktrees)
#   enforce — exit non-zero (default when ATLAS_ENV=production)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

MODE="${ATLAS_HOOK_MODE:-warn}"
CHECK=""
DRY_RUN=0

# Parse CLI arguments with a while loop so shift works correctly.
while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode=*) MODE="${1#--mode=}"; shift ;;
    --check) shift; CHECK="$*"; break ;;
    --dry-run) DRY_RUN=1; shift ;;
    --help|-h)
      sed -n '1,/^# Design:/p' "$0" | sed 's/^# //'
      exit 0
      ;;
    *) shift ;;
  esac
done

# In production worktrees, always enforce unless explicitly overridden.
if [[ "${ATLAS_ENV:-development}" == "production" && -z "${ATLAS_HOOK_MODE:-}" ]]; then
  MODE="enforce"
fi

# Helpers
block() {
  local reason="$1"
  if [[ "$MODE" == "enforce" ]]; then
    echo ""
    echo "❌ DENIED (enforce mode): $reason"
    echo "   Command: $CHECK"
    echo "   If you are certain this is intentional, bypass with:"
    echo "     ATLAS_HOOK_MODE=warn .agent-hooks/deny-dangerous.sh --check '$CHECK'"
    echo ""
    exit 1
  else
    echo ""
    echo "⚠️  WARNING (warn mode): $reason"
    echo "   Command: $CHECK"
    echo "   This would be blocked in enforce/production mode."
    echo ""
    # warn mode does not exit non-zero; it just surfaces the risk.
  fi
}

allow() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "✅ ALLOWED: $1"
  fi
}

# If no command supplied and not dry-run, print usage.
if [[ -z "$CHECK" && "$DRY_RUN" -eq 0 ]]; then
  echo "Usage: .agent-hooks/deny-dangerous.sh --check '<command>'"
  echo "       .agent-hooks/deny-dangerous.sh --dry-run"
  exit 2
fi

# Pattern checks
normalized="${CHECK,,}"

# 1. Direct push to protected branches.
if [[ "$normalized" =~ git[[:space:]]+push[[:space:]]+ && "$normalized" =~ (main|master)([[:space:]]|$) && ! "$normalized" =~ --no-verify ]]; then
  block "Direct push to main/master is prohibited. Use PR workflow."
fi

# 2. Force push (any branch).
if [[ "$normalized" =~ git[[:space:]]+push[[:space:]]+ && "$normalized" =~ (^|[[:space:]])(--force|-f)([[:space:]]|$) ]]; then
  block "Force push is prohibited."
fi

# 3. Recursive deletion of system or home directories.
if [[ "$normalized" =~ (^|[[:space:]])rm[[:space:]]+(-[a-zA-Z]*[fr]|--force|--recursive)*[[:space:]]+(-[a-zA-Z]*[fr]|--force|--recursive)*[[:space:]]*(/|~/|\$home/|\$HOME/|/Users/|/home/)([[:space:]]|$) ]]; then
  block "Recursive deletion of system/home directories is prohibited."
fi

# 4. Reading secret files.
if [[ "$normalized" =~ (^|[[:space:]])(cat|less|more|head|tail|grep|awk|sed|open|cp|mv|vim|nano|code)[[:space:]]+.*(\.env|\.env\.example|\.env\.local|\.p12|\.pfx|\.key|\.pem|secret|password|credential|\.fubon-env) ]]; then
  block "Reading secret/credential files requires explicit user approval."
fi

# 5. eval / bash -c with piped download (common malware/script injection pattern).
if [[ "$normalized" =~ (curl|wget).*(\||\$\().*(bash|sh) ]] || \
   [[ "$normalized" =~ (eval|bash[[:space:]]+-c|sh[[:space:]]+-c).*(curl|wget) ]]; then
  block "Executing downloaded scripts via eval/pipe is prohibited in production worktrees."
fi

# 6. Destructive SQL without explicit safety flag.
if [[ "$normalized" =~ (drop[[:space:]]+table|truncate[[:space:]]+table) && ! "$normalized" =~ --i-know-what-im-doing ]]; then
  block "Destructive SQL requires --i-know-what-im-doing flag and user approval."
fi

# 7. Live broker activation without both env and CLI gate.
if [[ "$normalized" =~ (\-allow-live-broker|--allow-live-broker) ]]; then
  if [[ "${ATLAS_ALLOW_LIVE_BROKER:-}" != "true" ]]; then
    block "Live broker requires ATLAS_ALLOW_LIVE_BROKER=true env var in addition to the CLI flag."
  fi
  if [[ "$normalized" =~ -broker-mode[[:space:]]+live && ! "$normalized" =~ -allow-live-broker ]]; then
    block "Live broker mode requires -allow-live-broker flag."
  fi
fi

# 8. Cross-environment operations: running dev-only commands in production worktree.
if [[ "${ATLAS_ENV:-development}" == "production" ]]; then
  # Dev-only Go commands.
  if [[ "$normalized" =~ (^|[[:space:]])go[[:space:]]+run([[:space:]]|$) ]]; then
    block "'go run' is prohibited when ATLAS_ENV=production. Use a built binary or deployment artifact."
  fi
  if [[ "$normalized" =~ (^|[[:space:]])go[[:space:]]+test([[:space:]]|$) ]]; then
    block "'go test' is prohibited when ATLAS_ENV=production. Run tests in development/staging worktrees."
  fi
  # Dev-only experiment / backfill / backtest commands.
  if [[ "$normalized" =~ (go[[:space:]]+run[[:space:]]+./cmd/(run-experiment|judge-experiment|promote-baseline|backtest-window|backfill-[a-z0-9-]+)|run-experiment|judge-experiment|promote-baseline|backtest-window) ]]; then
    block "Dev/experiment/backfill CLI commands are prohibited when ATLAS_ENV=production."
  fi
  # Dev-only Makefile targets.
  if [[ "$normalized" =~ (^|[[:space:]])make[[:space:]]+(dev|dev-stop|dev-status|dev-logs|watch-frontend|smoke|setup-mcp|setup-mcp-agent|verify-mcp-setup|test|test-frontend|test-backend|install-frontend|clean)([[:space:]]|$) ]]; then
    block "Dev-only Makefile targets are prohibited when ATLAS_ENV=production."
  fi
  # Local docker compose build/up in a prod worktree.
  if [[ "$normalized" =~ (docker[[:space:]]+compose[[:space:]]+build|docker[[:space:]]+compose[[:space:]]+up) ]]; then
    block "docker compose build/up in a production worktree is prohibited; use deployment-specific runbooks."
  fi
fi

# 9. Modifying .env.example without updating docs (common agent mistake).
if [[ "$normalized" =~ (edit|sed|vim|nano|code|echo|cat[[:space:]]*<<).*\.env\.example && ! "$normalized" =~ documentation-standard|quickstart ]]; then
  block "Modifying .env.example requires同步更新 docs/quickstart.md and docs/documentation-standard.md."
fi

# Default allow.
allow "$CHECK"

if [[ "$DRY_RUN" -eq 1 ]]; then
  echo ""
  echo "Mode: $MODE"
  echo "No dangerous patterns detected in supplied commands."
fi

exit 0
