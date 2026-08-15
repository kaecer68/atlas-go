#!/usr/bin/env bash
# check_doc_ports.sh — validate port references against internal/constants/port.go
#
# Reads the canonical port values from internal/constants/port.go and scans
# all non-archive files for stale port references (8080, 8081).  Any file
# outside the allowlist that references a legacy port is flagged as a
# potential doc-code drift.
#
# Usage:
#   bash scripts/ci/check_doc_ports.sh           # strict mode (exit 1 on violations)
#   bash scripts/ci/check_doc_ports.sh --warn-only  # warn but exit 0 (probation period)
#
# Exit codes:
#   0 — clean (no violations) or --warn-only active
#   1 — violations found (strict mode)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
PORT_GO="$REPO_ROOT/internal/constants/port.go"

# ── colour helpers ──────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
pass() { echo -e "${GREEN}[PASS]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
info() { echo -e "${CYAN}[INFO]${NC} $*"; }

# ── mode ────────────────────────────────────────────────────────────────────
STRICT=true
for arg in "$@"; do
  case "$arg" in
    --warn-only) STRICT=false; info "Running in --warn-only mode (violations are warnings, exit 0)" ;;
    *) fail "Unknown flag: $arg"; exit 2 ;;
  esac
done

# ── canonical values from port.go ────────────────────────────────────────────
# Extract AdminHTTPPort = ":18080" → bare port 18080
CANONICAL_HTTP_PORT=$(grep '	AdminHTTPPort' "$PORT_GO" | grep -o '[0-9]\{5\}')
# Extract FubonProxyPort = 18081 (word-boundary to avoid matching FubonProxyAddr)
CANONICAL_FUBON_PORT=$(grep '	FubonProxyPort ' "$PORT_GO" | grep -o '[0-9]\{5\}')

if [[ -z "$CANONICAL_HTTP_PORT" || -z "$CANONICAL_FUBON_PORT" ]]; then
  fail "Cannot parse canonical ports from $PORT_GO"
  exit 1
fi

info "Canonical ports from port.go: atlas=$CANONICAL_HTTP_PORT  fubon=$CANONICAL_FUBON_PORT"

# ── scan ─────────────────────────────────────────────────────────────────────
# Use git grep to find all :8080 / :8081 references in tracked text files.
# Then filter out allowlisted paths and non-port occurrences (stock codes, etc.)
#
# Allowlist:
#   docs/incidents/        — incident reports
#   CHANGELOG.md           — historical releases
#   data/                  — market data (stock codes like 8081.TW, 17808)
#   internal/constants/port.go — source of truth itself
#   internal/industry/representative_stocks.go — stock codes (8081.TW)
#   internal/marketdata/fubon_url_guard_test.go — intentional negative test
#   internal/fubonproxy/AGENTS.md — docstrings referencing old port for context
#   internal/fubonproxy/manager.go — doc comments about legacy :8081 behaviour

VIOLATIONS=0
VIOLATION_LOG=$(mktemp)
TOTAL_FILES=$(git -C "$REPO_ROOT" ls-files | wc -l | tr -d ' ')

# Search for port-like references: :8080/:8081 or port 8080/8081 or EXPOSE 8080/8081
while IFS=: read -r file line content; do
  case "$file" in
    docs/incidents/*|CHANGELOG.md|data/*) continue ;;
    internal/constants/port.go)  continue ;;
    internal/industry/representative_stocks.go) continue ;;
    internal/marketdata/fubon_url_guard_test.go) continue ;;
    internal/fubonproxy/AGENTS.md) continue ;;
    internal/fubonproxy/manager.go|scripts/ci/check_doc_ports.sh) continue ;;
  esac

  # Skip non-port occurrences: stock codes (8081.TW, 78081, etc.)
  echo "$content" | grep -qiE '[0-9]808[01]|808[01]\.[A-Z]' && continue

  VIOLATIONS=$((VIOLATIONS + 1))
  printf "  %s:%s — %s\n" "$file" "$line" "$content" >> "$VIOLATION_LOG"
done < <(cd "$REPO_ROOT" && git grep -n -i -E ':(8080|8081)|port.*(8080|8081)|EXPOSE (8080|8081)|targets.*(8080|8081)|localhost.?(8080|8081)|127\.0\.0\.1.?(8080|8081)|0\.0\.0\.0.?(8080|8081)|atlas-go.?(8080|8081)|fubon-proxy.?(8080|8081)' 2>/dev/null || true)

# ── report ───────────────────────────────────────────────────────────────────
if [ "$VIOLATIONS" -eq 0 ]; then
  pass "No legacy port drift detected (scanned $TOTAL_FILES tracked files)"
  rm -f "$VIOLATION_LOG"
  exit 0
fi

warn "Found $VIOLATIONS legacy port reference(s) — port.go uses :$CANONICAL_HTTP_PORT / $CANONICAL_FUBON_PORT"

if [ -s "$VIOLATION_LOG" ]; then
  echo ""
  echo "Files with legacy port references:"
  sort -u "$VIOLATION_LOG"
fi

if $STRICT; then
  echo ""
  fail "Legacy port references must be updated to $CANONICAL_HTTP_PORT / $CANONICAL_FUBON_PORT"
  rm -f "$VIOLATION_LOG"
  exit 1
fi

warn "Probation mode: exit 0 despite violations (remove --warn-only when cleaned)"
rm -f "$VIOLATION_LOG"
exit 0
