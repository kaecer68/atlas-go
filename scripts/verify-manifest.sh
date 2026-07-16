#!/usr/bin/env bash
set -euo pipefail

# verify-manifest.sh — external validation for docs/manifests/*.md
# Usage: ./scripts/verify-manifest.sh <path-to-manifest.md>
#
# Checks:
# 1. Every invariant row with Status == done has non-empty evidence in the Notes column.
# 2. Phase D close-out items are present (template fields are not left as placeholders).

manifest="${1:-}"
if [[ -z "$manifest" || ! -f "$manifest" ]]; then
  echo "Usage: $0 <path-to-manifest.md>" >&2
  exit 2
fi

errors=0

# Extract the Invariant Tracker table rows (skip header and separator lines).
# Expected columns:
# | ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
mapfile -t rows < <(awk '
  BEGIN { in_table = 0 }
  /^## Invariant Tracker/ { in_table = 1; next }
  in_table && /^---$/ { exit }
  in_table && /^\|[- ]+\|/ { next }
  in_table && /^\|[^!]/ { print }
' "$manifest")

for row in "${rows[@]}"; do
  # Split on "|" and trim whitespace.
  id=$(echo "$row" | awk -F'|' '{gsub(/^[ \t]+|[ \t]+$/, ""); print $2}')
  status=$(echo "$row" | awk -F'|' '{gsub(/^[ \t]+|[ \t]+$/, ""); print $7}')
  notes=$(echo "$row" | awk -F'|' '{gsub(/^[ \t]+|[ \t]+$/, ""); print $9}')

  # Skip template/example rows and non-done rows.
  [[ "$id" == "ID" ]] && continue
  [[ "$id" == "A01" && "$status" == "Status" ]] && continue
  [[ "$status" != "done" ]] && continue

  if [[ -z "$notes" || "$notes" == "<link to evidence>" || "$notes" == "-" ]]; then
    echo "ERROR: $manifest: invariant '$id' is marked done but has no evidence in Notes." >&2
    errors=$((errors + 1))
  fi
done

# Check that Phase D close-out placeholder rows are populated when work is done.
has_done_invariant=$(grep -cE '^\|[^|]+\|[^|]+\|[^|]+\|[^|]+\|[^|]+\| *done *\|' "$manifest" || true)

if (( has_done_invariant > 0 )); then
  phase_d_empty=$(awk '
    BEGIN { in_phase_d = 0 }
    /^### Phase D/ { in_phase_d = 1; next }
    in_phase_d && /^### / { exit }
    in_phase_d && /^\|.*\|.*\| pending \|.*\|$/ {
      gsub(/^[ \t]+|[ \t]+$/, "")
      print
    }
  ' "$manifest" | grep -cE '<PR #>|<CI link>|<target doc path>|^\|.*\|.*\| pending \|.*\|' || true)

  if (( phase_d_empty > 0 )); then
    echo "ERROR: $manifest: Phase D has $phase_d_empty unpopulated close-out item(s) while invariants are marked done." >&2
    errors=$((errors + 1))
  fi
fi

if (( errors > 0 )); then
  echo "FAIL: $manifest has $errors verification error(s)." >&2
  exit 1
fi

echo "OK: $manifest verified."
