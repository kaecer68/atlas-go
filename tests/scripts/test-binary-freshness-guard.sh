#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
CHECK="$ROOT/scripts/check-binary-freshness.sh"
SESSION_START="$ROOT/scripts/session-start.sh"
RETAG_TARGET=retag-cron-images

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local file=$1
  local pattern=$2
  grep -Fq -- "$pattern" "$file" || fail "$file does not contain: $pattern"
}

run_cron_retag_test() {
  local dir
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN

  cat >"$dir/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${FAKE_DOCKER_LOG:?}"
EOF
  chmod +x "$dir/docker"
  : >"$dir/docker.log"

  if ! make -s -C "$ROOT" "$RETAG_TARGET" DOCKER_BIN="$dir/docker" FAKE_DOCKER_LOG="$dir/docker.log"; then
    fail "Makefile does not provide a working $RETAG_TARGET target"
  fi
  test "$(wc -l <"$dir/docker.log" | tr -d ' ')" -eq 10 || \
    fail "$RETAG_TARGET did not create all ten cron image tags"
}
run_cleanup_failure_test() {
  local dir
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN

  cat >"$dir/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  create)
    echo fake-freshness-container
    ;;
  cp)
    exit 42
    ;;
  rm)
    echo "$*" >>"${FAKE_DOCKER_RM_LOG:?}"
    ;;
  *)
    exit 99
    ;;
esac
EOF
  chmod +x "$dir/docker"
  : >"$dir/rm.log"

  if DOCKER_BIN="$dir/docker" FAKE_DOCKER_RM_LOG="$dir/rm.log" "$CHECK" >/dev/null 2>&1; then
    fail "freshness check unexpectedly succeeded when docker cp failed"
  fi
  grep -Fxq -- 'rm -f fake-freshness-container' "$dir/rm.log" || \
    fail "freshness check did not remove container after docker cp failure"
}

run_missing_image_test() {
  local dir
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  cat >"$dir/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  create) exit 1 ;;
  *) exit 99 ;;
esac
EOF
  chmod +x "$dir/docker"
  if DOCKER_BIN="$dir/docker" "$CHECK" >/dev/null 2>&1; then
    fail "freshness check unexpectedly succeeded when images were unavailable"
  fi
}

run_success_cleanup_test() {
  local dir
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN

  cat >"$dir/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
counter="${FAKE_DOCKER_COUNTER:?}"
case "${1:-}" in
  create)
    n=$(cat "$counter")
    echo "fake-container-$n"
    ;;
  cp)
    destination=${@: -1}
    case "$destination" in
      "${FAKE_FRESHNESS_TMPDIR:?}"/*) ;;
      *) exit 43 ;;
    esac
    printf 'Commit=%s\n' "${FAKE_DOCKER_HEAD:?}" >"$destination"
    ;;
  rm)
    echo "$2" >>"${FAKE_DOCKER_RM_LOG:?}"
    ;;
esac
EOF
  chmod +x "$dir/docker"
  printf '0\n' >"$dir/counter"
  : >"$dir/rm.log"
  head=$(git -C "$ROOT" rev-parse HEAD)

  mkdir -p "$dir/freshness"
  DOCKER_BIN="$dir/docker" \
    FRESHNESS_TMPDIR="$dir/freshness" \
    FAKE_FRESHNESS_TMPDIR="$dir/freshness" \
    FAKE_DOCKER_COUNTER="$dir/counter" \
    FAKE_DOCKER_RM_LOG="$dir/rm.log" \
    FAKE_DOCKER_HEAD="$head" \
    "$CHECK" >/dev/null

  test "$(wc -l <"$dir/rm.log" | tr -d ' ')" -eq 6 || \
    fail "successful freshness check did not clean all temporary containers"
  leftover=$(cd "$dir/freshness" && shopt -s nullglob dotglob && echo *)
  if [ -n "$leftover" ]; then
    fail "freshness check left files in its isolated temporary directory: $leftover"
  fi
}

run_session_start_test() {
  local dir
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN

  cat >"$dir/make" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${FAKE_MAKE_LOG:?}"
case "${*: -1}" in
  check-binaries)
    n=$(wc -l <"${FAKE_MAKE_LOG}")
    if [ "$n" -eq 1 ]; then exit 1; fi
    ;;
  rebuild-all) ;;
  *) exit 99 ;;
esac
EOF
  cat >"$dir/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *'rev-parse --git-dir') printf '%s\n' "${FAKE_GIT_DIR:?}" ;;
  *'rev-parse --git-common-dir') printf '%s\n' "${FAKE_GIT_COMMON:?}" ;;
  *) exit 99 ;;
esac
EOF
  chmod +x "$dir/make" "$dir/git"
  : >"$dir/make.log"
  chmod +x "$dir/make"
  : >"$dir/make.log"

  CLAUDE_PROJECT_DIR="$ROOT" \
    GIT_BIN="$dir/git" \
    MAKE_BIN="$dir/make" \
    FAKE_GIT_DIR=.git \
    FAKE_GIT_COMMON=.git \
    FAKE_MAKE_LOG="$dir/make.log" \
    "$SESSION_START" >/dev/null

  test "$(wc -l <"$dir/make.log" | tr -d ' ')" -eq 3 || \
    fail "session-start did not run check, rebuild, check"
  sed -n '1p' "$dir/make.log" | grep -Fq -- "check-binaries" || fail "first session-start command was not check-binaries"
  sed -n '2p' "$dir/make.log" | grep -Fq -- "rebuild-all" || fail "second session-start command was not rebuild-all"
  sed -n '3p' "$dir/make.log" | grep -Fq -- "check-binaries" || fail "third session-start command was not check-binaries"

  : >"$dir/make.log"
  CLAUDE_PROJECT_DIR="$ROOT" \
    GIT_BIN="$dir/git" \
    MAKE_BIN="$dir/make" \
    FAKE_GIT_DIR=.git/worktrees/test \
    FAKE_GIT_COMMON=.git \
    FAKE_MAKE_LOG="$dir/make.log" \
    "$SESSION_START" >/dev/null
  test ! -s "$dir/make.log" || fail "linked worktree session-start modified shared Docker state"
}

run_static_contract_tests() {
  assert_contains "$ROOT/docker-compose.yml" 'GIT_COMMIT: ${ATLAS_GIT_COMMIT:?'
  assert_contains "$ROOT/Dockerfile" 'GIT_COMMIT must be set'
  assert_contains "$ROOT/Dockerfile.cron" 'GIT_COMMIT must be set'
  assert_contains "$ROOT/.claude/settings.json" 'scripts/session-start.sh'
  for binary in atlas-go daily-replay-sync atlas-mcp calibrate-seasonal; do
    assert_contains "$ROOT/Dockerfile" "-o $binary"
  done
  assert_contains "$ROOT/docker-compose.yml" 'prism-worker:'
  assert_contains "$ROOT/.claude/settings.json" '"timeout": 600'
  for binary in daily-replay-sync calibrate-seasonal; do
    assert_contains "$ROOT/scripts/check-binary-freshness.sh" "/app/$binary"
  done
  assert_contains "$ROOT/scripts/deploy-staging.sh" 'ATLAS_GIT_COMMIT="$(git rev-parse HEAD)"'
}

run_cron_retag_test
run_cleanup_failure_test
run_missing_image_test
run_success_cleanup_test
run_session_start_test
run_static_contract_tests

echo "PASS: binary freshness guard contract tests"
