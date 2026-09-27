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
# Severity modes (2026-09-27, E23 — `enforce` is now the default):
#   enforce — print the reason and exit non-zero. DEFAULT, in every worktree
#             (dev and production alike): a dangerous command is stopped, not
#             only commented on.
#   warn    — print a warning and allow. This is the documented escape hatch:
#               export ATLAS_HOOK_MODE=warn      # for the whole agent session
#             The PreToolUse hook runs in the AGENT's environment, not in the
#             environment of the command it inspects, so setting the variable
#             inside a tool call cannot lift a block. Full rollback (removing the
#             PreToolUse entry) is documented in .agent-hooks/README.md.
#
# `enforce` became the default only AFTER the three measured false blocks were
# removed. tests/scripts/test-agent-hook-wiring.sh keeps them red if they return:
#   ① pattern 4 judges the TARGET of a read command, not the presence of the
#      word "secret" anywhere on the line — a plain `grep -rn secret internal/`
#      used to be reported as "reading a secret file".
#   ② the documented deploy path still passes in a production worktree
#      (`bash scripts/deploy-staging.sh`, `make rebuild-all`, and a raw compose
#      command that carries the documented `ATLAS_GIT_COMMIT=` pin).
#   ③ `go test` / `make test` / `make ci-*` are allowed in a production worktree
#      (test-fix loops happen there); only genuinely dev-only targets are blocked.
#
# Known approximations (deliberate — the command is read as TEXT, not executed):
#   * a dangerous string inside quotes is not evaluated as if it ran;
#   * a secret file reached through a variable (`cat $SECRET_FILE`) is not seen;
#   * pattern 3 matches a root/home target as a WHOLE argument (`rm -rf /`,
#     `rm -rf ~/`, `rm -rf $HOME/`, `rm -rf /Users/`, `rm -rf /home/`). `rm -rf ~`
#     (no slash), `rm -rf /*` and `rm -rf /Users/x` are NOT covered — unchanged
#     from before E23, recorded here so the gap is not mistaken for coverage.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Default mode. Deliberately `enforce` since 2026-09-27 (E23): the guard used to
# default to `warn`, which meant every Claude Code Bash call in this repo was
# inspected and then allowed anyway.
MODE="${ATLAS_HOOK_MODE:-enforce}"
CHECK=""
DRY_RUN=0

# Parse CLI arguments with a while loop so shift works correctly.
while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode=*) MODE="${1#--mode=}"; shift ;;
    --check) shift; CHECK="$*"; break ;;
    --dry-run) DRY_RUN=1; shift ;;
    --help|-h)
      # Print the whole comment header (it now carries the mode policy and the
      # escape hatch), stopping at the first executable line.
      awk '/^set -euo/{exit} {sub(/^# ?/, ""); print}' "$0"
      exit 0
      ;;
    *) shift ;;
  esac
done

# In production worktrees, always enforce unless explicitly overridden.
# Redundant for mode selection now that `enforce` is the default, but kept:
# `:-` treats an EMPTY ATLAS_HOOK_MODE as unset, so this still pins production to
# enforce even if a caller exports ATLAS_HOOK_MODE="".
if [[ "${ATLAS_ENV:-development}" == "production" && -z "${ATLAS_HOOK_MODE:-}" ]]; then
  MODE="enforce"
fi

# Helpers
#
# NOTE (machine-readable output contract): the first line emitted by block()
# starts with one of the two markers below. `.agent-hooks/pretooluse-deny-dangerous.sh`
# (the PreToolUse adapter registered in `.claude/settings.json`) tells "blocked"
# from "guard broke" by matching them, so do not reword them without updating the
# adapter and tests/scripts/test-agent-hook-wiring.sh. The two markers are:
#   enforce mode -> "DENIED (enforce mode):"      (this function exits 1)
#   warn mode    -> "WARNING (warn mode):"        (this function exits 0)
# block REASON [HINT] — refuse (enforce) or warn (warn mode) about a command.
#
# $2 is the "do this instead" hint. Every block MUST tell the blocked agent what
# the legitimate path is (2026-09-27, E23): enforce is the default now, so a block
# that only says "no" costs the agent a round trip and tempts it to force things.
block() {
  local reason="$1"
  local hint="${2:-}"
  if [[ "$MODE" == "enforce" ]]; then
    echo ""
    echo "❌ DENIED (enforce mode): $reason"
    echo "   Command: $CHECK"
    if [[ -n "$hint" ]]; then
      echo "   Do this instead: $hint"
    fi
    echo "   To lift the block: start the agent session with ATLAS_HOOK_MODE=warn"
    echo "   (an env var set inside the blocked command cannot lift it — the hook"
    echo "   runs in the agent's environment). Rollback: .agent-hooks/README.md."
    echo ""
    exit 1
  else
    echo ""
    echo "⚠️  WARNING (warn mode): $reason"
    echo "   Command: $CHECK"
    if [[ -n "$hint" ]]; then
      echo "   Do this instead: $hint"
    fi
    echo "   This would be blocked in enforce mode (the default)."
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

# ── Structural helpers (used by the argument-aware patterns 1, 2, 4, 5, 8) ────
#
# A guard that only looks for substrings reports nonsense: `grep -rn secret
# internal/` is a SEARCH, not a secret read; `git push origin fix/x && git
# checkout main` is not "a push to main"; `curl ... | shasum -a 256` is not
# "download and execute". So the patterns below look at the command STRUCTURE:
# the line is cut into segments at the operators that start a new command, and
# every segment is then word-split.
#
# `read -ra` is used instead of `set -- $x` on purpose: it neither glob-expands
# nor re-interprets quoting (a word like `*.env` stays `*.env`, and `*` does not
# turn into the contents of the current directory).
COMMAND_SEGMENTS=""

SEG_TOKENS=()
SEG_N=0

split_segment() { # $1 = one shell segment
  SEG_TOKENS=()
  read -ra SEG_TOKENS <<<"$1"
  SEG_N=${#SEG_TOKENS[@]}
}

# Index of the command word inside SEG_TOKENS: skips leading `VAR=value`
# assignments and the usual wrappers. Prints nothing if the segment has none.
segment_cmd_index() {
  local idx=0 word expect_value=0
  while [ "$idx" -lt "$SEG_N" ]; do
    word="${SEG_TOKENS[$idx]}"
    idx=$((idx + 1))
    if [ "$expect_value" -eq 1 ]; then
      # Value of a wrapper flag: `sudo -u root cat .env` must still see `cat` as
      # the command word (only the unambiguous wrapper flags are listed; `-p`,
      # `-S` and friends stay plain flags because `time -p cat .env` /
      # `sudo -S cat .env` are real forms too).
      expect_value=0
      continue
    fi
    case "$word" in
      -u|--user|-g|--group|-C|--chdir|--prompt|--unset) expect_value=1; continue ;;
      -*) continue ;;
      *=*) continue ;;
      sudo|command|env|nohup|time|xargs) continue ;;
    esac
    printf '%s' "$((idx - 1))"
    return 0
  done
  return 1
}

# Does ANY segment run a command whose basename matches the ERE in $1?
any_segment_runs() {
  local segment idx word
  while IFS= read -r segment; do
    [ -n "$segment" ] || continue
    split_segment "$segment"
    idx="$(segment_cmd_index)" || continue
    word="${SEG_TOKENS[$idx]}"
    word="${word##*/}"
    if [[ "$word" =~ $1 ]]; then
      return 0
    fi
  done <<<"$COMMAND_SEGMENTS"
  return 1
}

# Emit the arguments of every segment that runs `git push` (one word per line).
git_push_args() {
  local segment idx
  while IFS= read -r segment; do
    [ -n "$segment" ] || continue
    split_segment "$segment"
    idx="$(segment_cmd_index)" || continue
    [ "${SEG_TOKENS[$idx]}" = "git" ] || continue
    idx=$((idx + 1))
    [ "${SEG_TOKENS[$idx]:-}" = "push" ] || continue
    idx=$((idx + 1))
    while [ "$idx" -lt "$SEG_N" ]; do
      printf '%s\n' "${SEG_TOKENS[$idx]}"
      idx=$((idx + 1))
    done
  done <<<"$COMMAND_SEGMENTS"
}

# Read/access commands (pattern 4) and how their arguments must be read.
READ_CMDS_RE='^(cat|less|more|head|tail|grep|egrep|fgrep|rg|sed|awk|gawk|open|cp|mv|rsync|scp|vim|vi|nano|code)$'

# Does this word NAME a secret file? Names, not words: `secret` alone is not a
# secret file name, so `grep -rn secret internal/` stays allowed, while `.env`,
# `~/.aws/credentials`, `id_rsa` and `foo.pem` are secret files.
# `.env.example`/`.env.sample`/`.env.template` are tracked TEMPLATES (no values),
# so reading them stays allowed.
is_secret_target() {
  local word="$1" base
  word="${word#\'}"; word="${word%\'}"
  word="${word#\"}"
  word="${word%\"}"
  word="${word##\*}"
  word="${word%\*}"
  base="${word##*/}"
  case "$base" in
    ""|"."|"..") return 1 ;;
    .env.example|.env.sample|.env.template) return 1 ;;
    .env|.env.*|.netrc|.npmrc|.pypirc|.htpasswd|.secrets|.fubon-env) return 0 ;;
    id_rsa|id_dsa|id_ecdsa|id_ed25519) return 0 ;;
    secret.yaml|secret.yml|secret.json|secret.env) return 0 ;;
    secrets.yaml|secrets.yml|secrets.json|secrets.env) return 0 ;;
    credentials|credentials.*|credentials-*) return 0 ;;
    *.p12|*.pfx|*.key|*.pem|*.jks|*.keystore|*.ppk) return 0 ;;
  esac
  return 1
}

# Does this segment run `docker compose build|up` (v1 `docker-compose` included)?
# Global flags are skipped, and the flags that take a value consume the next
# word, so `docker compose -f docker-compose.yml up -d` is recognised while
# `docker compose ps` / `docker compose logs` are not.
segment_runs_compose_action() {
  local idx=0 word
  while [ "$idx" -lt "$SEG_N" ]; do
    word="${SEG_TOKENS[$idx]}"
    idx=$((idx + 1))
    case "$word" in
      docker-compose) : ;;
      docker)
        [ "$idx" -lt "$SEG_N" ] || return 1
        [ "${SEG_TOKENS[$idx]}" = "compose" ] || continue
        idx=$((idx + 1)) ;;
      *) continue ;;
    esac
    while [ "$idx" -lt "$SEG_N" ]; do
      word="${SEG_TOKENS[$idx]}"
      case "$word" in
        -f|--file|-p|--project-name|--profile|--env-file|--project-directory|--ansi)
          idx=$((idx + 2))
          continue ;;
        -*) idx=$((idx + 1)); continue ;;
      esac
      break
    done
    [ "$idx" -lt "$SEG_N" ] || return 1
    case "${SEG_TOKENS[$idx]}" in
      build|up) return 0 ;;
    esac
    return 1
  done
  return 1
}

# ── Pattern 6 (destructive SQL: the STATEMENT, not the words) ────────────────
# 2026-09-27 (E23): `grep -rn 'drop table' internal/db/` used to trip this rule
# because the phrase appeared anywhere on the line — a search for destructive SQL
# was punished like running it. The phrase is now ignored in a segment that runs a
# TEXT reader/searcher (there the phrase is quoted text), and only the remaining
# segments are inspected. `--i-know-what-im-doing` still lifts the whole rule.
check_destructive_sql() {
  local segment idx word
  while IFS= read -r segment; do
    [ -n "$segment" ] || continue
    [[ "$segment" =~ (drop|truncate)[[:space:]]+table ]] || continue
    split_segment "$segment"
    idx="$(segment_cmd_index)" || continue
    word="${SEG_TOKENS[$idx]}"
    word="${word##*/}"
    case "$word" in
      grep|egrep|fgrep|rg|ag|sed|awk|gawk|cat|head|tail|less|more|bat|git) continue ;;
    esac
    block "Destructive SQL requires the --i-know-what-im-doing flag and user approval." \
      "re-run with '--i-know-what-im-doing' after a human confirms, or ship a migration instead (internal/db/migrations)."
  done <<<"$COMMAND_SEGMENTS"
}

# ── Pattern 4 (secret FILES, not the word "secret") ──────────────────────────
# 2026-09-27 (E23): the old rule was "the command line contains the word
# `secret`/`.env`/..." — so an ordinary search such as `grep -rn secret internal/`
# was reported as "reading a secret file" and (now that enforce is the default)
# would have been blocked. The rule is about the TARGET now:
#   * a read/access command (READ_CMDS_RE) whose argument NAMES a secret file;
#   * search tools (grep/rg/sed/awk) skip their first positional argument, which
#     is the pattern / script, not a file name;
#   * copy tools (cp/mv/rsync/scp) are judged on their SOURCE only, so the
#     documented fresh-clone step `cp .env.example .env` stays allowed while
#     `cp .env /tmp/` is still blocked.
check_secret_reads() {
  local segment idx word positional max_positional is_search
  while IFS= read -r segment; do
    [ -n "$segment" ] || continue
    split_segment "$segment"
    idx="$(segment_cmd_index)" || continue
    word="${SEG_TOKENS[$idx]}"
    [[ "$word" =~ $READ_CMDS_RE ]] || continue

    is_search=0
    max_positional=0
    case "$word" in
      grep|egrep|fgrep|rg|sed|awk|gawk) is_search=1 ;;
      cp|mv|rsync|scp) max_positional=1 ;;
    esac

    positional=0
    idx=$((idx + 1))
    while [ "$idx" -lt "$SEG_N" ]; do
      word="${SEG_TOKENS[$idx]}"
      idx=$((idx + 1))
      case "$word" in
        ""|-) continue ;;
        -*) continue ;;
      esac
      if [ "$is_search" -eq 1 ] && [ "$positional" -eq 0 ]; then
        positional=1
        continue
      fi
      if is_secret_target "$word"; then
        block "Reading a secret/credential file is prohibited: the target '$word' names a secret file." \
          "read the tracked template or the docs instead ('.env.example'); for a real value, ask the human. If this file is genuinely needed, the human can start the session with ATLAS_HOOK_MODE=warn."
      fi
      positional=$((positional + 1))
      if [ "$max_positional" -gt 0 ] && [ "$positional" -ge "$max_positional" ]; then
        break
      fi
    done
  done <<<"$COMMAND_SEGMENTS"
}

# Pattern checks
# Lowercase with `tr`, not `${CHECK,,}`: this script is invoked through
# `#!/usr/bin/env bash`, which on stock macOS resolves to bash 3.2 and
# `${var,,}` is a bash 4 feature. Under bash 3.2 the expansion aborts the
# script with "bad substitution" (exit 1) for EVERY command - i.e. the guard
# would report benign commands as blocked. See
# tests/scripts/test-agent-hook-wiring.sh (runs the whole verdict matrix under
# the system bash) for the regression proof.
normalized="$(printf '%s' "$CHECK" | tr '[:upper:]' '[:lower:]')"

# Segments for the structural patterns (1/2/4/5/8): cut the line at the operators
# that START a new command (`&&`, `||`, `;`, `|`) and at the redirection/grouping
# characters `(`/`)`/`>`, so that
#     cat .env.example > .env
# no longer looks like "reading .env" (the `>` target is a write) while
#     cat .env | grep x
# is still inspected segment by segment.
# `tr` is byte-based and exists on bash 3.2 (macOS /bin/bash); no `${x//}` here.
COMMAND_SEGMENTS="$(printf '%s' "$normalized" | tr '&|;()>' '\n\n\n\n\n\n')"

# 1 + 2. Push hygiene, judged from the refspec arguments of the `git push`
# segment only (2026-09-27, E23). The old test was "the words `git push` and
# `main` appear anywhere on the line", so `git push origin fix/x && git checkout
# main` was reported as a push to main, and `git push ... && rm -f /tmp/x` was
# reported as a force push — both would now be hard blocks.
PUSH_PROTECTED=""
PUSH_FORCE=0
PUSH_NO_VERIFY=0
while IFS= read -r arg; do
  [ -n "$arg" ] || continue
  case "$arg" in
    --no-verify) PUSH_NO_VERIFY=1 ;;
  esac
  case "$arg" in
    # `--force-with-lease` is deliberately NOT a force push here: it refuses to
    # overwrite work you have not seen, which is the safety property that
    # matters, and it is the documented way to update a rebased PR branch.
    --force|-f) PUSH_FORCE=1 ;;
  esac
  case "${arg#+}" in
    main|master|main:*|master:*|*:main|*:master|*refs/heads/main|*refs/heads/master)
      PUSH_PROTECTED="${arg#+}" ;;
  esac
done <<<"$(git_push_args)"

if [ -n "$PUSH_PROTECTED" ] && [ "$PUSH_NO_VERIFY" -eq 0 ]; then
  block "Direct push to main/master is prohibited (refspec: $PUSH_PROTECTED)." \
    "push a branch and open a PR: 'git push -u origin <branch>' then 'gh pr create'. A human can override with 'git push --no-verify origin main' if they really want the direct push."
fi

if [ "$PUSH_FORCE" -eq 1 ]; then
  block "Force push is prohibited." \
    "push the branch normally and open a PR; 'git push --force-with-lease' is allowed by this guard when a remote branch must be rewritten."
fi

# 3. Recursive deletion of system or home directories.
if [[ "$normalized" =~ (^|[[:space:]])rm[[:space:]]+(-[a-zA-Z]*[fr]|--force|--recursive)*[[:space:]]+(-[a-zA-Z]*[fr]|--force|--recursive)*[[:space:]]*(/|~/|\$home/|\$HOME/|/Users/|/home/)([[:space:]]|$) ]]; then
  block "Recursive deletion of system/home directories is prohibited." \
    "delete the specific path you mean, e.g. 'rm -rf ./tmp/build', and keep it inside the worktree."
fi

# 4. Reading secret files — the TARGET decides whether this is a secret read
# (see check_secret_reads above; pattern logic lives in one place on purpose).
check_secret_reads

# 5. eval / bash -c with piped download (common malware/script injection pattern).
#
# 2026-09-27 (E23): `(bash|sh)` is matched as a WORD now. The old regex matched
# the `sh` inside `shasum`, so a normal checksum/pipeline such as
#     curl -s ... | shasum -a 256
#     curl -s localhost:18080/api/version | grep sha
# counted as "download and execute".
if [[ "$normalized" =~ (curl|wget).*(\||\$\().*(^|[^a-z0-9_-])(bash|sh)([[:space:]]|-|$) ]] || \
   [[ "$normalized" =~ (eval|bash[[:space:]]+-c|sh[[:space:]]+-c).*(curl|wget) ]]; then
  block "Executing downloaded scripts via eval/pipe is prohibited." \
    "download the file first, inspect it, then run the local copy ('curl -fsSL <url> -o /tmp/x.sh && less /tmp/x.sh && bash /tmp/x.sh')."
fi

# 6. Destructive SQL without explicit safety flag — the STATEMENT decides
# (see check_destructive_sql above), not the appearance of the words on the line.
if [[ ! "$normalized" =~ --i-know-what-im-doing ]]; then
  check_destructive_sql
fi

# 7. Live broker activation without both env and CLI gate.
if [[ "$normalized" =~ (\-allow-live-broker|--allow-live-broker) ]]; then
  if [[ "${ATLAS_ALLOW_LIVE_BROKER:-}" != "true" ]]; then
    block "Live broker requires ATLAS_ALLOW_LIVE_BROKER=true env var in addition to the CLI flag." \
      "run against the simulated broker; live trading needs a human to export ATLAS_ALLOW_LIVE_BROKER=true first (docs/reference/traps.md 'Live 旗標')."
  fi
  # NOTE (2026-09-27): unreachable as written — the outer `if` already requires
  # the flag, so this branch can never fire. It is kept (with a hint, so every
  # block() call in this file tells the caller what to do) because removing it
  # would change pattern 7's shape in a PR that is about enforcement defaults;
  # flagged here instead of silently leaving a misleading gate.
  if [[ "$normalized" =~ -broker-mode[[:space:]]+live && ! "$normalized" =~ -allow-live-broker ]]; then
    block "Live broker mode requires the -allow-live-broker flag." \
      "add '-allow-live-broker' together with ATLAS_ALLOW_LIVE_BROKER=true, and only after a human approves live trading."
  fi
fi

# 8. Cross-environment operations: running dev-only commands in production worktree.
#
# 2026-09-27 (E23) — three measured false blocks were removed here. In every case
# the rule had been written loosely enough that the work it was meant to protect
# was blocked in the production worktree, which is exactly where deployment and
# test-fix loops happen:
#   * `docker compose build`/`up` IS the documented deploy path
#     (scripts/deploy-staging.sh runs `docker compose build && up -d`;
#     `make rebuild-all` is the Mac Mini entry; docs/operations/local-deploy.md
#     §坑③ documents the bare form with the `ATLAS_GIT_COMMIT=` pin);
#   * `go test` / `make test` are read-only and the test-fix loop needs them;
#   * `make ci-*` was never listed but had to stay allowed.
# `go run` stays blocked (it bypasses the deployed binary + freshness gate), and
# only genuinely dev-only make targets are blocked.
if [[ "${ATLAS_ENV:-development}" == "production" ]]; then
  # Dev-only Go commands.
  if [[ "$normalized" =~ (^|[[:space:]])go[[:space:]]+run([[:space:]]|$) ]]; then
    block "'go run' is prohibited when ATLAS_ENV=production. Use a built binary or deployment artifact." \
      "run the deployed binary ('bin/atlas ...'); if it is stale, deploy with 'make rebuild-all' (docs/operations/local-deploy.md)."
  fi
  # `go test` used to be blocked here. Removed 2026-09-27 (E23): running tests is
  # read-only, and the test-fix loop runs in this very worktree.

  # Dev-only experiment / backfill / backtest CLIs: matched on the COMMAND WORD of
  # each segment (basename), not as a substring anywhere on the line, so
  # `grep -rn backtest-window docs/` is a search again instead of a violation of
  # a rule about running dev CLIs.
  if any_segment_runs '^(run-experiment|judge-experiment|promote-baseline|backtest-window|backfill-[a-z0-9-]+)$'; then
    block "Dev/experiment/backfill CLI commands are prohibited when ATLAS_ENV=production." \
      "run experiments in a development worktree; the production worktree only runs deployed artifacts (tests are fine: 'go test ./...', 'make test', 'make ci-gate')."
  fi

  # Dev-only Makefile targets. `test|test-frontend|test-backend` were removed
  # 2026-09-27 (E23) — see above; `make ci-*` was never in this list.
  if [[ "$normalized" =~ (^|[[:space:]])make[[:space:]]+(dev|dev-stop|dev-status|dev-logs|watch-frontend|smoke|setup-mcp|setup-mcp-agent|verify-mcp-setup|install-frontend|clean)([[:space:]]|$) ]]; then
    block "Dev-only Makefile targets are prohibited when ATLAS_ENV=production." \
      "check with 'make ci-gate' / 'make ci-full', test with 'make test' or 'go test ./...', and deploy with 'make rebuild-all'."
  fi

  # Raw `docker compose build|up` in a prod worktree: allowed on the documented
  # deploy path, blocked otherwise. The documented paths are
  #   (a) `make rebuild-all` / `bash scripts/deploy-staging.sh` (never reach this
  #       branch as raw compose),
  #   (b) a compose command carrying the documented pin `ATLAS_GIT_COMMIT=...`
  #       (local-deploy.md §坑③: bare compose in production MUST pin the commit),
  #   (c) an explicit opt-in ATLAS_ALLOW_PROD_COMPOSE=true.
  PROD_COMPOSE_ACTION=0
  while IFS= read -r segment; do
    [ -n "$segment" ] || continue
    split_segment "$segment"
    if segment_runs_compose_action; then
      PROD_COMPOSE_ACTION=1
      break
    fi
  done <<<"$COMMAND_SEGMENTS"

  if [ "$PROD_COMPOSE_ACTION" -eq 1 ] \
     && [ "${ATLAS_ALLOW_PROD_COMPOSE:-}" != "true" ] \
     && [[ ! "$normalized" =~ atlas_git_commit= ]]; then
    block "Raw 'docker compose build/up' in a production worktree is prohibited without an explicit deploy pin." \
      "deploy with 'make rebuild-all' or 'bash scripts/deploy-staging.sh'; for the bare documented rollback form add the pin: 'ATLAS_GIT_COMMIT=\$(git rev-parse HEAD) docker compose up -d'; a human can also export ATLAS_ALLOW_PROD_COMPOSE=true."
  fi
fi

# 9. Modifying .env.example without updating docs (common agent mistake).
if [[ "$normalized" =~ (edit|sed|vim|nano|code|echo|cat[[:space:]]*<<).*\.env\.example && ! "$normalized" =~ documentation-standard|quickstart ]]; then
  block "Modifying .env.example requires updating docs/quickstart.md and docs/documentation-standard.md in the same change." \
    "add a new variable to .env.example together with its documentation, or leave .env.example alone."
fi

# Default allow.
allow "$CHECK"

if [[ "$DRY_RUN" -eq 1 ]]; then
  echo ""
  echo "Mode: $MODE"
  echo "No dangerous patterns detected in supplied commands."
fi

exit 0
