#!/usr/bin/env bash
set -euo pipefail

ROOT=${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}
MAKE_BIN=${MAKE_BIN:-make}

if [ ! -f "$ROOT/Makefile" ] || [ ! -x "$ROOT/scripts/check-binary-freshness.sh" ]; then
    exit 0
fi

cd "$ROOT"
if "$MAKE_BIN" check-binaries; then
    exit 0
fi

"$MAKE_BIN" rebuild-all
"$MAKE_BIN" check-binaries
