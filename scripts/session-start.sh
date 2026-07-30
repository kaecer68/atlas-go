#!/usr/bin/env bash
set -euo pipefail

ROOT=${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}
MAKE_BIN=${MAKE_BIN:-make}
GIT_BIN=${GIT_BIN:-git}

if [ ! -f "$ROOT/Makefile" ] || [ ! -x "$ROOT/scripts/check-binary-freshness.sh" ]; then
    exit 0
fi
git_dir=$("$GIT_BIN" -C "$ROOT" rev-parse --git-dir 2>/dev/null || true)
git_common=$("$GIT_BIN" -C "$ROOT" rev-parse --git-common-dir 2>/dev/null || true)
if [ "$git_dir" != "$git_common" ] && [ "${ATLAS_ALLOW_LINKED_WORKTREE_REBUILD:-0}" != 1 ]; then
    echo "Skipping binary rebuild in linked worktree; Docker images are shared with the primary checkout."
    exit 0
fi

cd "$ROOT"
if "$MAKE_BIN" check-binaries; then
    exit 0
fi
"$MAKE_BIN" rebuild-host-bin rebuild-atlas-bins rebuild-cron-bins
if ! "$MAKE_BIN" check-binaries; then
    echo "⚠️  docker images 與 HEAD 不齊。AI 已用純 go build 對齊 host binaries；"
    echo "    docker 部署請 kaecer 在主 worktree 手動執行 make rebuild-all。"
fi
exit 0
