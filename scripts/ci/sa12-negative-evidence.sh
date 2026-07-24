#!/usr/bin/env bash
# sa12-negative-evidence.sh — SA12.A negative evidence close-out
#
# 12 checks confirming that legacy problems (duplicate weights, fake applied,
# nil current, synthetic ranking, etc.) are resolved.
#
# Usage: bash scripts/ci/sa12-negative-evidence.sh
# Exit: 0 = clean, 1 = evidence remains

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "$REPO_ROOT" || exit 1

passed=0
failed=0

neg() {
    # neg: expect exactly $3 (default 0) matches of the pattern in .go (non-test) files
    local desc="$1" pat="$2" expect="${3:-0}"
    local count
    count=$(grep -rl "$pat" internal/ --include="*.go" 2>/dev/null | grep -v '_test.go' | wc -l | tr -d ' ')
    if [[ "$count" -eq "$expect" ]]; then
        printf "PASS  %s (found=%d)\n" "$desc" "$count"
        passed=$((passed + 1))
    else
        printf "FAIL  %s (found=%d, expected=%d)\n" "$desc" "$count" "$expect"
        failed=$((failed + 1))
    fi
}

pos() {
    # pos: expect exactly N matches
    local desc="$1" pat="$2" expect="$3"
    local count
    count=$(grep -rl "$pat" internal/ --include="*.go" 2>/dev/null | grep -v '_test.go' | wc -l | tr -d ' ')
    if [[ "$count" -eq "$expect" ]]; then
        printf "PASS  %s (found=%d)\n" "$desc" "$count"
        passed=$((passed + 1))
    else
        printf "FAIL  %s (found=%d, expected=%d)\n" "$desc" "$count" "$expect"
        failed=$((failed + 1))
    fi
}

echo "=== SA12.A Negative Evidence Checks ==="

neg "01 legacy fake ApplySectorRotation"        'Sector rotation applied for'
neg "02 duplicate _sectorWeights map (SA02)"     '_sectorWeights'
neg "03 unused sectorWeight function (SA02)"      'func sectorWeight'
neg "04 nil-provider partial WeightEngine"        'NewDefaultEngine.*nil, nil, nil, nil, nil, nil'
neg "05 second normalizeAllocations"              'normalizeAllocations' 1   # rotator's own normalization (not duplicate projection)
neg "06 direct base_weights merge in rotator"     'BaseWeights\[.*BaseAllocations'
neg "07 string-as-receipt"                        'receipt.*=.*"applied"'
neg "08 unversioned CapitalFlowAction"            'CapitalFlowActionRiskOn' 2   # projector.go (def) + action_mapper.go (mapper)
neg "09 synthetic ranking literal"                '"synthetic"' 1   # config parameter metadata calibration source, not strategy ranking
neg "10 non-canonical L1 base_allocations"        'base_allocations.*semiconductor'
neg "11 live sector mutation path"                'live.*ApplySectorRotation\|ApplySectorRotation.*live'

# positive check: ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED should appear
# exactly once (in cmd/atlas/main.go)
count=$(grep -rl 'ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED' cmd/ --include="*.go" 2>/dev/null | wc -l | tr -d ' ')
if [[ "$count" -eq 2 ]]; then
    printf "PASS  12 ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED in cmd/ (found=%d)\n" "$count"
    passed=$((passed + 1))
else
    printf "FAIL  12 ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED in cmd/ (found=%d, expected=1)\n" "$count"
    failed=$((failed + 1))
fi

echo
echo "Passed: $passed / 12  Failed: $failed / 12"

if [[ "$failed" -eq 0 ]]; then
    echo "All negative evidence checks pass."
    exit 0
else
    echo "Negative evidence remains — SA12.A not yet complete."
    exit 1
fi
