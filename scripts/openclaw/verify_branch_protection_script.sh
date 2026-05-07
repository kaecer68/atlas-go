#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "$PROJECT_ROOT"

TARGET="scripts/openclaw/setup_branch_protection.sh"

log() {
	echo "[verify-branch-protection] $*"
}

assert_file_contains() {
	local file="$1"
	local pattern="$2"
	local hint="$3"
	if ! grep -qE -- "$pattern" "$file"; then
		echo "[error] expected pattern not found in $file: $hint"
		exit 1
	fi
}

if [[ ! -f "$TARGET" ]]; then
	echo "[error] target script not found: $TARGET"
	exit 1
fi

log "Verifying CLI contract for required reviews range (0..6)"
assert_file_contains "$TARGET" '--required-reviews <n>\s+Required approving reviews, 0\.\.6 \(default: 1\)' 'usage must document 0..6 range'

log "Verifying interactive prompt contract"
assert_file_contains "$TARGET" 'Required approving reviews \(0\.\.6\)' 'interactive prompt must show 0..6 range'

log "Verifying input validation guard"
assert_file_contains "$TARGET" 'if \(\( REQUIRED_REVIEWS < 0 \|\| REQUIRED_REVIEWS > 6 \)\); then' 'validation guard must allow 0 and reject outside 0..6'
assert_file_contains "$TARGET" 'required reviews out of range \(0\.\.6\): \$REQUIRED_REVIEWS' 'error message must match 0..6 range'

log "Verifying payload still includes required_approving_review_count"
assert_file_contains "$TARGET" 'required_approving_review_count: \$required_reviews' 'payload must carry required review count'

log "Branch protection script contract verification passed"
