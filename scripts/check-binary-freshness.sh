#!/usr/bin/env bash
# check-binary-freshness.sh
#
# Verifies that every deployed binary's buildinfo.Commit CONTAINS the last
# commit that touched a build input (BUILD_INPUT_PATHS below) — it does NOT
# require equality with HEAD.
#
# Why not HEAD: HEAD also moves for commits that cannot possibly invalidate a
# binary (docker-compose*.yml, docs/**, scripts/** other than
# cron-entrypoint.sh). Judging against HEAD turned the gate red right after
# every compose/docs/script commit. Measured on the production host
# (2026-09-23): the deployed images and host binaries were built at 7126c3a7
# (#1920, the last commit that touched Go code) while HEAD stood at 17e67439
# (#1921 compose / #1923 script / #1925 CI+docs) — the old rule reported five
# STALE binaries and exit 1 for a perfectly deployed system. A gate that is red
# at both session start and session end stops being a signal: it either forces
# a pointless rebuild (rebuilding recreates containers — a measured risk on
# this project) or teaches everyone to ignore it.
#
# Temporary Docker containers and extracted files are always cleaned up, including
# when Docker copy fails or the shell exits early.
#
# Modes (2026-09-26, FU-20260926-15):
#   (default)                 Docker images + host binaries. Requires docker.
#   --host-only               Host binaries ONLY. Never invokes docker at all, so
#                             it is safe inside the pre-push hook (a push must not
#                             depend on a docker daemon) and on the dev box.
#   --diff-base <ref>         Additionally skip the whole check when the push
#                             origin/<ref>...HEAD moves NO build input: a push
#                             that cannot invalidate a binary must not be blocked
#                             by a binary that was already stale. Without this
#                             flag the check always evaluates the binaries.
#
# The host binaries are judged with exactly the same rule as the images
# (buildinfo.Commit must contain LAST_BUILD_COMMIT, and be an ancestor of HEAD),
# so there is one freshness rule in this repo, not two.

set -euo pipefail

HOST_ONLY=0
DIFF_BASE=""
while [ $# -gt 0 ]; do
    case "$1" in
        --host-only) HOST_ONLY=1 ;;
        --diff-base)
            shift
            [ $# -gt 0 ] || { echo "ERROR: --diff-base needs a ref" >&2; exit 2; }
            DIFF_BASE=$1
            ;;
        --diff-base=*) DIFF_BASE=${1#--diff-base=} ;;
        -h|--help)
            sed -n '2,40p' "$0"
            exit 0
            ;;
        *)
            echo "ERROR: unknown argument: $1" >&2
            exit 2
            ;;
    esac
    shift
done

DOCKER_BIN="${DOCKER_BIN:-docker}"
FRESHNESS_TMPDIR="${FRESHNESS_TMPDIR:-${TMPDIR:-/tmp}}"
# Host-native build outputs only (Makefile: build-backend → bin/atlas,
# build-mcp / rebuild-host-bin → bin/atlas-mcp). Deliberately an explicit list,
# not a glob: bin/atlas-linux is a legacy cross-build artifact nothing rebuilds,
# and *.bak-* files are backups — neither is a host deploy target, so neither
# may block a push.
HOST_BINARIES=('bin/atlas' 'bin/atlas-mcp')
declare -a TEMP_CONTAINERS=()
declare -a TEMP_FILES=()
HOST_CHECKED=0   # host binaries actually evaluated (0 ⇒ nothing to judge)

declare -a STALE=()
declare -a MISSING_BUILDINFO=()

cleanup() {
    local status=$?
    set +e
    # Guarded loops: under `set -u`, expanding an EMPTY array
    # ("${TEMP_FILES[@]}") is an "unbound variable" error on bash < 4.4
    # (macOS ships 3.2), which made cleanup abort with
    # "TEMP_FILES[@]: unbound variable" whenever no temp file had been
    # created yet (2026-09-23: make check-binaries died on the Mac Mini
    # before printing the summary).
    if [ "${#TEMP_FILES[@]}" -gt 0 ]; then
        for file in "${TEMP_FILES[@]}"; do
            rm -f "$file"
        done
    fi
    if [ "${#TEMP_CONTAINERS[@]}" -gt 0 ]; then
        for container in "${TEMP_CONTAINERS[@]}"; do
            "$DOCKER_BIN" rm -f "$container" >/dev/null 2>&1
        done
    fi
    # Belt-and-suspenders: catch any atlas.binary-freshness-labeled container
    # the explicit rm loop may have missed (daemon hiccup, race, etc).
    # Runs regardless of TEMP_CONTAINERS contents — but NEVER in --host-only
    # mode: that mode must not touch docker at all (no daemon required, and
    # `docker ... prune` mutates host state).
    if [ "$HOST_ONLY" -eq 0 ]; then
        "$DOCKER_BIN" container prune -f --filter "label=atlas.binary-freshness=true" >/dev/null 2>&1 || true
    fi
    exit "$status"
}
trap cleanup EXIT

HEAD=$(git rev-parse HEAD 2>/dev/null) || { echo "ERROR: not in a git repo"; exit 2; }

# Build inputs: only these files can change the compiled binaries. Everything
# else in the tree (docker-compose*.yml, docs/**, Dockerfile-independent
# scripts) is deployed configuration/documentation, not binary content.
# - Dockerfile* catches Dockerfile, Dockerfile.cron, Dockerfile.atlas.local, ...
# - scripts/cron-entrypoint.sh is COPY'd into the cron image, so it is a build input.
BUILD_INPUT_PATHS=('*.go' 'go.mod' 'go.sum' 'Dockerfile*' 'scripts/cron-entrypoint.sh')
LAST_BUILD_COMMIT=$(git log -1 --format=%H -- "${BUILD_INPUT_PATHS[@]}" 2>/dev/null || true)
if [ -z "$LAST_BUILD_COMMIT" ]; then
    # No build-input commit in this clone's history (e.g. very shallow clone).
    # Fall back to HEAD so the gate still fails closed instead of silently passing.
    LAST_BUILD_COMMIT=$HEAD
    echo "WARN: no commit touching build inputs found; falling back to HEAD=$HEAD" >&2
fi

echo "checking binaries against HEAD=$HEAD"
echo "last build input commit: $LAST_BUILD_COMMIT"
echo ""

# Helper: extract buildinfo.Commit from a host binary file.
extract_commit_host() {
    local bin_path=$1
    strings "$bin_path" 2>/dev/null | grep 'Commit=' | head -1 \
        | sed 's/.*Commit=\([a-f0-9]*\).*/\1/' | grep -E '^[a-f0-9]{7,}$' || echo ""
}

# A binary is FRESH when its build commit contains the last build-input commit
# (equal counts as containing) and is itself an ancestor of HEAD. The second
# check rejects binaries built from a commit that is not part of this branch.
is_fresh_commit() {
    local commit=$1
    # Unknown commit (not fetched in this clone) cannot be proven fresh.
    git rev-parse --verify --quiet "${commit}^{commit}" >/dev/null 2>&1 || return 1
    git merge-base --is-ancestor "$LAST_BUILD_COMMIT" "$commit" || return 1
    git merge-base --is-ancestor "$commit" "$HEAD" || return 1
    return 0
}

check_one() {
    local label=$1
    local commit=$2
    if [ -z "$commit" ]; then
        MISSING_BUILDINFO+=("$label (no buildinfo.Commit found)")
        echo "  ⚠ $label: buildinfo.Commit NOT FOUND"
    elif is_fresh_commit "$commit"; then
        echo "  ✓ $label: $commit"
    else
        STALE+=("$label: $commit (last build input=$LAST_BUILD_COMMIT)")
        echo "  ✗ STALE  $label: $commit"
    fi
}

# Copy one binary from a temporary container. Cleanup is deferred to EXIT so a
# failed docker cp cannot leave a random Created container behind.
check_image_binary() {
    local image=$1
    local binary=$2
    local label=$3
    local tmp_bin=$4
    local container

    container=$("$DOCKER_BIN" create --label atlas.binary-freshness=true "$image" 2>/dev/null || true)
    if [ -z "$container" ]; then
        MISSING_BUILDINFO+=("$label (image unavailable: $image)")
        echo "  ⚠ $label: image unavailable ($image)" >&2
        return 0
    fi
    TEMP_CONTAINERS+=("$container")
    TEMP_FILES+=("$tmp_bin")
    "$DOCKER_BIN" cp "$container:$binary" "$tmp_bin"
    check_one "$label" "$(extract_commit_host "$tmp_bin")"
}

if [ "$HOST_ONLY" -eq 0 ]; then
    echo "=== Docker images ==="
    check_image_binary "atlas-atlas:latest" /app/atlas-go \
        "atlas-atlas image → /app/atlas-go" "$FRESHNESS_TMPDIR/.atlas-go-freshness-check-$$"
    check_image_binary "atlas-atlas:latest" /app/atlas-mcp \
        "atlas-atlas image → /app/atlas-mcp" "$FRESHNESS_TMPDIR/.atlas-mcp-freshness-check-$$"
    check_image_binary "atlas-atlas:latest" /app/daily-replay-sync \
        "atlas-atlas image → /app/daily-replay-sync" "$FRESHNESS_TMPDIR/.daily-replay-sync-freshness-check-$$"
    check_image_binary "atlas-atlas:latest" /app/calibrate-seasonal \
        "atlas-atlas image → /app/calibrate-seasonal" "$FRESHNESS_TMPDIR/.calibrate-seasonal-freshness-check-$$"
    check_image_binary "atlas-prism-worker:latest" /app/atlas-go \
        "atlas-prism-worker image → /app/atlas-go" "$FRESHNESS_TMPDIR/.prism-worker-freshness-check-$$"
    check_image_binary "atlas-cron-rebuilt:local" /app/macro-ingest \
        "atlas-cron-rebuilt:local → /app/macro-ingest" "$FRESHNESS_TMPDIR/.macro-ingest-freshness-check-$$"

    echo ""
fi

echo "=== Host binaries ==="
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# --diff-base <ref>: a push that carries no build-input change cannot make a
# host binary stale (it was already stale, if it was), so it must not be
# blocked by one. The build-input list below is the single definition; the
# pathspec is reused verbatim so predicate and rule cannot drift.
if [ -n "$DIFF_BASE" ]; then
    if ! git rev-parse --verify --quiet "${DIFF_BASE}^{commit}" >/dev/null 2>&1; then
        echo "ERROR: --diff-base ${DIFF_BASE} does not resolve in this repo" >&2
        exit 2
    fi
    changed=$(git diff --name-only "${DIFF_BASE}...HEAD" -- "${BUILD_INPUT_PATHS[@]}" 2>/dev/null || true)
    if [ -z "$changed" ]; then
        echo "  ℹ no build input changed in ${DIFF_BASE}...HEAD → host binary freshness not required for this push"
        echo "  ✓ (host binaries are judged only when the push can change them)"
        exit 0
    fi
    echo "  build input(s) changed in ${DIFF_BASE}...HEAD:"
    while IFS= read -r changed_file; do
        [ -n "$changed_file" ] && echo "    - $changed_file"
    done <<<"$changed"
fi

# One host binary evaluated per iteration; a clone without bin/ (fresh worktree,
# or a machine that never built here) has nothing to judge and must NOT be
# blocked — that is the "not found (skipping)" branch, kept deliberately soft.
for host_rel in "${HOST_BINARIES[@]}"; do
    host_path="$REPO_ROOT/$host_rel"
    if [ -f "$host_path" ]; then
        HOST_CHECKED=$((HOST_CHECKED + 1))
        check_one "$host_rel" "$(extract_commit_host "$host_path")"
    else
        echo "  ⚠ $host_rel not found at $host_path (skipping)"
    fi
done
if [ "$HOST_CHECKED" -eq 0 ]; then
    echo "  ℹ no host binaries in this clone → nothing to judge (gate not applicable)"
fi

echo ""
echo "=== Summary ==="
echo "  HEAD:              $HEAD"
echo "  last build input:  $LAST_BUILD_COMMIT"
if [ "$HOST_ONLY" -eq 1 ]; then
    echo "  mode:              --host-only (docker not consulted)"
fi
echo "  (FRESH = the binary's buildinfo.Commit contains the last build input"
echo "   commit. Commits that only touch non-build-input files —"
echo "   docker-compose*.yml, docs/**, scripts/** except cron-entrypoint.sh —"
echo "   do not require a rebuild.)"
if [ ${#STALE[@]} -eq 0 ] && [ ${#MISSING_BUILDINFO[@]} -eq 0 ]; then
    echo "  ✓ ALL BINARIES FRESH"
    exit 0
fi
if [ ${#STALE[@]} -gt 0 ]; then
    echo "  ✗ STALE (${#STALE[@]}):"
    printf '    %s\n' "${STALE[@]}"
fi
if [ ${#MISSING_BUILDINFO[@]} -gt 0 ]; then
    echo "  ⚠ MISSING_BUILDINFO (${#MISSING_BUILDINFO[@]}):"
    printf '    %s\n' "${MISSING_BUILDINFO[@]}"
fi
echo ""
if [ "$HOST_ONLY" -eq 1 ]; then
    echo "Fix: a build input (*.go, go.mod, go.sum, Dockerfile*, scripts/cron-entrypoint.sh)"
    echo "     changed after the HOST binaries were built. Rebuild them (pure go build):"
    echo "       make rebuild-host-bin   # bin/atlas-mcp"
    echo "       make build-backend      # bin/atlas"
    echo "     (or: make build-mcp for bin/atlas-mcp; this check never touches docker)"
else
    echo "Fix: a build input (*.go, go.mod, go.sum, Dockerfile*, scripts/cron-entrypoint.sh)"
    echo "     changed after the deployed binaries were built. Run 'make rebuild-all' to"
    echo "     realign the binaries with $LAST_BUILD_COMMIT."
fi
exit 1
