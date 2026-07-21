#!/usr/bin/env bash
# scripts/ci/zero-metric-guard.sh
#
# P2 structural guardrail: prevent missing-data values from silently rendering
# as "0.0 / 0.0% / 0.00" on audited pages/components.
#
# Scans shared_web/static/js/pages/*.js and components/*.js for direct calls to
# non-safe formatting helpers (formatNumber, fmtPct, fmtSignedPct, fmtDrawdown,
# fmtCurrency, fmtLargeNumber, formatSigned) without the fmtSafe* wrapper.
#
# Usage:
#   scripts/ci/zero-metric-guard.sh [--fail]
#   --fail exits with non-zero status when unsafe patterns are found.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SEARCH_DIR="${ROOT_DIR}/shared_web/static/js"

UNSAFE_RE='\b(formatNumber|fmtPct|fmtSignedPct|fmtDrawdown|fmtCurrency|fmtLargeNumber|formatSigned)\s*\('

# Helpers that are allowed to keep using the low-level functions internally.
EXCLUDE_FILES=(
  'shared/format-metric.js'
  '__tests__/*'
)

FAIL_MODE=false
if [[ "${1:-}" == "--fail" ]]; then
  FAIL_MODE=true
fi

FOUND=0

for file in $(find "${SEARCH_DIR}/pages" "${SEARCH_DIR}/components" -maxdepth 2 -name '*.js' | sort); do
  rel="${file#${ROOT_DIR}/}"

  skip=false
  for pat in "${EXCLUDE_FILES[@]}"; do
    if [[ "${rel}" == "${pat}" ]]; then
      skip=true
      break
    fi
  done
  if [[ "${skip}" == true ]]; then
    continue
  fi

  matches=$(grep -nE "${UNSAFE_RE}" "${file}" || true)
  if [[ -n "${matches}" ]]; then
    FOUND=1
    echo "=== ${rel} ==="
    echo "${matches}"
    echo
  fi
done

if [[ "${FOUND}" -eq 0 ]]; then
  echo '✅ zero-metric guard: no unsafe formatting patterns found in audited pages/components.'
  exit 0
fi

if [[ "${FAIL_MODE}" == true ]]; then
  echo '❌ zero-metric guard failed: direct calls to non-safe formatting helpers found.' >&2
  exit 1
fi

echo '⚠️  zero-metric guard: unsafe patterns found (treated as warning).'
exit 0
