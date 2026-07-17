#!/usr/bin/env bash
# verify-sector-allocation-closure.sh — closure verifier for SA00-SA12
#
# 對應：
#   docs/specs/sector-allocation-simulation-closure-spec.md §10
#   docs/manifests/sector-allocation-simulation-closure-manifest.md
#
# SA01 階段僅做 scaffold 驗證（manifest 存在、ID 全列出、結構合規）。
# 完整 5 條基礎 check + SA12 擴充 check 由 Go 測試與 CI 腳本執行。
#
# Usage:
#   bash scripts/verify-sector-allocation-closure.sh [path-to-manifest]
#
# Exit codes:
#   0  pass
#   1  violation
#   2  config error (missing/invalid manifest)

set -euo pipefail

MANIFEST="${1:-docs/manifests/sector-allocation-simulation-closure-manifest.md}"

if [[ ! -f "$MANIFEST" ]]; then
    echo "FAIL: manifest $MANIFEST not found" >&2
    exit 2
fi

errors=0
expected_ids=(
    "SA00" "SA01" "SA02" "SA03" "SA04" "SA05" "SA06"
    "SA07" "SA08" "SA09" "SA10" "SA11" "SA12"
)

for id in "${expected_ids[@]}"; do
    if ! grep -qE "^\|\s*${id}\s*\|" "$MANIFEST"; then
        echo "FAIL: manifest $MANIFEST missing ID row $id" >&2
        errors=$((errors + 1))
    fi
done

if ! grep -qE '\| NamespaceKind' "$MANIFEST" 2>/dev/null; then
    if ! grep -qE '## Binding Invariants' "$MANIFEST"; then
        echo "FAIL: manifest $MANIFEST missing Binding Invariants section" >&2
        errors=$((errors + 1))
    fi
fi

if ! grep -qE 'SA-INV-20' "$MANIFEST"; then
    echo "FAIL: manifest $MANIFEST missing SA-INV-20 (no-look-ahead + next-session consumption)" >&2
    errors=$((errors + 1))
fi

if grep -qE 'source=empirical' "$MANIFEST"; then
    echo "FAIL: manifest $MANIFEST contains source=empirical in notes; this is a permanent lock" >&2
    errors=$((errors + 1))
fi

if grep -qE 'calibration_status=calibrated' "$MANIFEST"; then
    echo "FAIL: manifest $MANIFEST contains calibration_status=calibrated; heuristic lock" >&2
    errors=$((errors + 1))
fi

if (( errors == 0 )); then
    echo "OK: closure verifier scaffold passes for $MANIFEST"
    echo "     (full check set is enforced via go test ./internal/sectorallocation/... and scripts/ci/sa12-negative-evidence.sh)"
    exit 0
fi

echo "FAIL: $errors closure verifier error(s) in $MANIFEST" >&2
exit 1
