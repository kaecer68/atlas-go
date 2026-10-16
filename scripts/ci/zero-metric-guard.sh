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
#   scripts/ci/zero-metric-guard.sh            # 報告模式（有違規仍 rc=0；給人看的）
#   scripts/ci/zero-metric-guard.sh --fail     # 閘門模式（有違規 ⇒ rc=1）
#   --fail exits with non-zero status when unsafe patterns are found.
#
# 呼叫端（FU-20261016-26 補上）:
#   * `make ci-gate` 的 pre-push 段
#   * `.github/workflows/quality.yml` 的 `frontend-tests` job（PR 階段）
#   背景：本守門自 2026-07 建立後**零呼叫端** ⇒ `--fail` 這顆牙齒只在「有人記得帶它」
#   時才成立；預設路徑在 `shared_web/static/js/pages/portfolio.js` 找到違規仍 rc=0
#   ＝ 與死旗標同型的網路效果。現在由上面兩個入口以 `--fail` 呼叫。
#
# 未知旗標 ⇒ rc=2 ＋ 用法（不得靜默退回報告模式）。

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
for arg in "$@"; do
  case "$arg" in
    --fail) FAIL_MODE=true ;;
    *)
      echo "unknown option: $arg" >&2
      echo "usage: zero-metric-guard.sh [--fail]" >&2
      exit 2
      ;;
  esac
done

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

echo '⚠️  zero-metric guard: unsafe patterns found — report mode (rc=0); pass --fail to gate.'
exit 0
