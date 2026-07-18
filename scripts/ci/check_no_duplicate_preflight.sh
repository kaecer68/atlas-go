#!/usr/bin/env bash
#
# scripts/ci/check_no_duplicate_preflight.sh
#
# Enforces the "unique launch gate" pattern from
# docs/specs/experimental-feature-launch-gate.md. Scans the repo for any
# preflight / gate / launch_gate source files, compares against the
# allow-list, and emits a WARNING (not a failure) if a new instance appears
# without being added to the allow-list.
#
# Exit code: always 0 (warning only). The warning is the audit signal;
# reviewer decides whether the new instance is justified.
#
# Rationale for warning-not-fail: "uniqueness" is a design intent, not a
# hard contract. Too-strict CI blocks legitimate deviations (e.g. test-only
# preflight). Warning still surfaces the question to the reviewer.
#
# Usage:
#   bash scripts/ci/check_no_duplicate_preflight.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Allow-list: paths that match preflight/gate patterns and are intentionally
# part of the canonical pattern. New instances must be added here with a
# comment explaining the rationale.
ALLOW_LIST=(
    "cmd/experimental/l2-4-preflight"            # L2.4 manual preflight (PR #1027)
    "cmd/experimental/c07-preflight"             # C07 sector prediction manual preflight (PR #1200+, Wave 11 C07 follow-up)
    "cmd/experimental/sector-allocation-closure-preflight"  # SA11.A sector allocation closure manual preflight
    "internal/scheduler/l2_4_auto_cron.go"       # L2.4 auto-cron gate (PR #1029)
)

# Match patterns: any directory or .go file with "preflight", "launch_gate",
# or "_gate.go" in its name. Conservative on purpose — false negatives
# preferred over false positives.
MATCH_PATTERNS=(
    "*preflight*"
    "*launch_gate*"
    "*_gate.go"
)

cd "${REPO_ROOT}"

# Scan canonical pattern locations only:
#   1. cmd/experimental/*-preflight/ directories (manual preflight)
#   2. internal/scheduler/*_auto_cron.go files (auto-cron gate)
# Narrow scan (instead of grepping all "*preflight*" / "*_gate.go" repo-wide)
# avoids false positives like internal/live/risk_gate.go (risk control, not
# launch gate) and internal/startup/preflight.go (process bootstrap, not
# experimental feature launch).
declare -a matches=()

# Pattern 1: cmd/experimental/*-preflight/
while IFS= read -r -d '' path; do
    matches+=("${path#./}")
done < <(find ./cmd/experimental -mindepth 1 -maxdepth 1 -type d -name "*-preflight" -print0 2>/dev/null)

# Pattern 2: internal/scheduler/*_auto_cron.go
while IFS= read -r -d '' path; do
    matches+=("${path#./}")
done < <(find ./internal/scheduler -maxdepth 1 -type f -name "*_auto_cron.go" -print0 2>/dev/null)

if [ "${#matches[@]}" -eq 0 ]; then
    echo "OK: no preflight/auto_cron instances found (clean baseline)"
    exit 0
fi

unknown=()
for match in "${matches[@]}"; do
    allowed=false
    for allow in "${ALLOW_LIST[@]}"; do
        if [[ "${match}" == "${allow}"* ]]; then
            allowed=true
            break
        fi
    done
    if [ "${allowed}" = "false" ]; then
        unknown+=("${match}")
    fi
done

if [ "${#unknown[@]}" -eq 0 ]; then
    echo "OK: ${#matches[@]} preflight/launch_gate instance(s), all in allow-list:"
    for match in "${matches[@]}"; do
        echo "  - ${match}"
    done
    exit 0
fi

echo "WARN: ${#unknown[@]} preflight/launch_gate path(s) NOT in allow-list:"
for u in "${unknown[@]}"; do
    echo "  - ${u}"
done
echo ""
echo "If this is a new canonical instance, add it to ALLOW_LIST in this script"
echo "and reference docs/specs/experimental-feature-launch-gate.md."
echo "If this is a test-only or one-off gate, consider whether it should follow"
echo "the canonical pattern instead."
echo ""
echo "Reviewer: please confirm whether the new path is justified or should be"
echo "replaced with a clone of the L2.4 preflight pattern."
echo ""
echo "(warning only — exit 0; uniqueness is a design intent, not a hard contract)"

exit 0
