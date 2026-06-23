#!/usr/bin/env bash
# scripts/ci/check_layer3_snapshots.sh
#
# CI gate for Layer 3 (Issue #611 sub-issue-9): snapshot / golden test gate.
# Runs all snapshot + golden tests across internal/config, cmd/atlas,
# internal/narrative. Any API snapshot drift or golden mismatch fails CI.
#
# Behavior:
#   - golden exists and matches → PASS
#   - golden exists and mismatches → FAIL (intentional, forces conscious review)
#   - golden missing → FAIL with clear "regenerate locally" hint
#     (the snapshot helper would otherwise silently regenerate it)
#
# The silent-regeneration protection is enforced by checking that no
# testdata/<file>.golden.json file was newly created or modified during
# this run. If you intentionally want to regenerate, run the test locally
# then commit the new golden.
#
# Usage:
#   ./scripts/ci/check_layer3_snapshots.sh
#
# Exit codes:
#   0  — all snapshot + golden tests pass, no testdata/ changes
#   1  — test failure OR golden file drift detected

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

cd "$REPO_ROOT"

declare -a TARGETS=(
    "internal/config"
    "cmd/atlas"
    "internal/narrative"
    "internal/orchestrator"
    "internal/portfolio"
    "internal/sim"
    "internal/risk"
)

# 1) Capture pre-run state of all testdata/ JSON golden files.
PRE_SNAPSHOTS="$(mktemp)"
trap 'rm -f "$PRE_SNAPSHOTS"' EXIT

for target in "${TARGETS[@]}"; do
    if [ -d "${target}/testdata" ]; then
        find "${target}/testdata" -type f -name '*.golden.json' -print0 \
            | xargs -0 sha256sum 2>/dev/null >> "${PRE_SNAPSHOTS}" || true
    fi
done

# 2) Run snapshot + golden tests.
echo "[layer3-snap] running snapshot + golden tests"
FAILED=0
for target in "${TARGETS[@]}"; do
    echo "[layer3-snap] TEST ${target}"
    if ! go test "./${target}/..."; then
        echo "[layer3-snap] FAIL ${target}: snapshot or golden test failed" >&2
        FAILED=1
    fi
done

# 3) Detect silent golden regeneration.
POST_SNAPSHOTS="$(mktemp)"
trap 'rm -f "${PRE_SNAPSHOTS}" "${POST_SNAPSHOTS}"' EXIT

for target in "${TARGETS[@]}"; do
    if [ -d "${target}/testdata" ]; then
        find "${target}/testdata" -type f -name '*.golden.json' -print0 \
            | xargs -0 sha256sum 2>/dev/null >> "${POST_SNAPSHOTS}" || true
    fi
done

if ! diff -q "${PRE_SNAPSHOTS}" "${POST_SNAPSHOTS}" > /dev/null 2>&1; then
    echo "[layer3-snap] FAIL: golden file drift detected" >&2
    echo "  Files added or modified during this run:" >&2
    diff "${PRE_SNAPSHOTS}" "${POST_SNAPSHOTS}" | sed 's/^/    /' >&2
    echo "  To regenerate intentionally, run the failing test locally, then commit." >&2
    FAILED=1
fi

if [ "${FAILED}" -ne 0 ]; then
    echo "[layer3-snap] one or more snapshot/golden checks failed" >&2
    exit 1
fi

echo "[layer3-snap] all snapshot + golden tests pass, no testdata drift"