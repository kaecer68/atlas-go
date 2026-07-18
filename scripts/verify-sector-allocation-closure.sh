#!/usr/bin/env bash
# verify-sector-allocation-closure.sh — closure verifier for SA00-SA12
#
# 對應：
#   docs/specs/sector-allocation-simulation-closure-spec.md §10
#   docs/manifests/sector-allocation-simulation-closure-manifest.md
#
# SA01 階段僅做 scaffold 驗證。SA12 擴充為 17 條完整 check。
#
# Usage:
#   bash scripts/verify-sector-allocation-closure.sh [path-to-manifest]
# Exit: 0=pass, 1=violation, 2=config error

set -euo pipefail

MANIFEST="${1:-docs/manifests/sector-allocation-simulation-closure-manifest.md}"
RETAIL_MANIFEST="docs/manifests/2026-07-17-retail-positioning-gap-fix-manifest.md"

if [[ ! -f "$MANIFEST" ]]; then
    echo "FAIL: manifest $MANIFEST not found" >&2
    exit 2
fi

errors=0
passes=0

check() {
    local desc="$1"; shift
    local result
    result=$(eval "$@" 2>&1)
    local rc=$?
    if [[ $rc -eq 0 ]]; then
        printf "PASS  %s\n" "$desc"
        passes=$((passes + 1))
    else
        printf "FAIL  %s: %s\n" "$desc" "${result:-rc=$rc}"
        errors=$((errors + 1))
    fi
}

echo "=== Closure Verifier (17 checks) ==="

# Check 1-5: manifest structure (original 5)
expected_ids=(SA00 SA01 SA02 SA03 SA04 SA05 SA06 SA07 SA08 SA09 SA10 SA11 SA12)
for id in "${expected_ids[@]}"; do
    if ! grep -qE "^\|\s*${id}\s*\|" "$MANIFEST"; then
        echo "FAIL: manifest $MANIFEST missing ID row $id" >&2
        errors=$((errors + 1))
    fi
done

# Check 6: SA12 status must be done or implemented
check "06 SA12 status is done/implemented" \
    "grep -qE '^\|\s*SA12\s*\|.*\|.*(done|implemented).*\|' '$MANIFEST'"

# Check 7: SA06 status done
check "07 SA06 status is done" \
    "grep -qE '^\|\s*SA06\s*\|.*done.*\|' '$MANIFEST'"

# Check 8: SA08 status done
check "08 SA08 status is done" \
    "grep -qE '^\|\s*SA08\s*\|.*done.*\|' '$MANIFEST'"

# Check 9: retail F05 done
if [[ -f "$RETAIL_MANIFEST" ]]; then
    check "09 retail manifest F05 done" \
        "grep -qE '^\|\s*F05\s*\|.*\*\*done\*\*.*\|' '$RETAIL_MANIFEST'"
fi

# Check 10: no source=empirical (heuristic lock)
check "10 source lock: no empirical" \
    "! grep -qE 'source=empirical' '$MANIFEST'"

# Check 11: calibration_status lock
check "11 calibration_status: no calibrated" \
    "! grep -qE 'calibration_status=calibrated' '$MANIFEST'"

# Check 12: Binding Invariants section present
check "12 Binding Invariants section" \
    "grep -qE '## Binding Invariants' '$MANIFEST'"

# Check 13: SA-INV-20 present
check "13 SA-INV-20 (no-look-ahead)" \
    "grep -qE 'SA-INV-20' '$MANIFEST'"

# Check 14: live-sync isolation documented
check "14 live-sync isolation gated" \
    "grep -qE 'ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED' '$MANIFEST'"

# Check 15: SA-INV-11 (Applied=true → receipt) present
check "15 SA-INV-11 (receipt required)" \
    "grep -qE 'SA-INV-11' '$MANIFEST'"

# Check 16: negative evidence script exists
check "16 sa12-negative-evidence.sh exists" \
    "test -f scripts/ci/sa12-negative-evidence.sh"

# Check 17: composition root tests exist
check "17 composition root tests" \
    "test -f internal/orchestrator/composition/root_test.go"

echo
echo "=== Summary ==="
echo "Passed: $passes / 17  Failed: $errors / 17"

if [[ $errors -eq 0 ]]; then
    echo "All closure verifier checks pass."
    exit 0
else
    echo "$errors verifier error(s) found." >&2
    exit 1
fi
