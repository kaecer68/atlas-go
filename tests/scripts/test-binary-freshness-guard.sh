#!/usr/bin/env bash
set -euo pipefail

# This file creates throwaway git repositories. Git exports GIT_DIR (and
# friends) to hooks, so a hook-invoked run would send every `git -C <temp>`
# command to the CALLER's repository: on 2026-09-23 the pre-push run of
# `make ci-gate` committed this test's fixture commits ("docs seed",
# "go change", "compose + docs only") straight onto the branch being pushed,
# which then failed the commit-message check. Drop the repo-selecting
# environment so each git call honours its own -C/cwd.
# See issue #1927.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
CHECK="$ROOT/scripts/check-binary-freshness.sh"
SESSION_START="$ROOT/scripts/session-start.sh"
RETAG_TARGET=retag-cron-images

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# Physical (symlink-resolved) path. macOS reports /var/... for mktemp but git
# may report /private/var/...; comparing unresolved strings would hide a leak.
physical_path() {
  ( cd "$1" 2>/dev/null && pwd -P )
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
  # Drift guard (2026-09-23, #1898): derive the expected set from
  # docker-compose.yml instead of hardcoding a count. A cron service added to
  # compose without a matching CRON_IMAGE_TAGS entry used to surface only at
  # deploy time (atlas-cron-darwinian -> "No such image", Mac Mini 2026-09-23).
  local expected expected_count produced
  expected=$(sed -nE 's/^  ((cron|atlas-cron)-[a-z0-9-]+):.*/\1/p' "$ROOT/docker-compose.yml" \
    | sed 's/^/atlas-/' | sort -u)
  [ -n "$expected" ] || fail "could not derive cron services from docker-compose.yml"
  expected_count=$(printf '%s\n' "$expected" | wc -l | tr -d ' ')
  for tag in $expected; do
    grep -Fq -- "tag atlas-cron-rebuilt:local ${tag}:latest" "$dir/docker.log" || \
      fail "$RETAG_TARGET is missing an image tag for compose service $tag"
  done
  produced=$(grep -c '^tag ' "$dir/docker.log" || true)
  test "$produced" -eq "$expected_count" || \
    fail "$RETAG_TARGET retagged $produced images but docker-compose.yml declares $expected_count cron services"
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

  # Derive the expectation from the script itself (pre-existing drift fixed
  # 2026-09-23: this used to hardcode 6 while check-binary-freshness.sh inspects
  # 5 image binaries -> the guard test was red on main). The real invariant is
  # "every inspected image is cleaned up", not a magic number.
  local expected_rm
  expected_rm=$(grep -c '^check_image_binary ' "$CHECK")
  [ "$expected_rm" -gt 0 ] || fail "could not derive image checks from $CHECK"
  test "$(wc -l <"$dir/rm.log" | tr -d ' ')" -eq "$expected_rm" || \
    fail "successful freshness check did not clean all temporary containers (rm=$(wc -l <"$dir/rm.log" | tr -d ' ') expected=$expected_rm)"
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
case "$*" in
  check-binaries)
    n=$(wc -l <"${FAKE_MAKE_LOG}")
    if [ "$n" -eq 1 ]; then exit 1; fi
    ;;
  # Current session-start contract (2026-09-23): a stale binary triggers a
  # host/bin-only rebuild (no docker); the container rebuild stays a manual step
  # for the operator (see ~/.agents/AGENTS.md docker ban).
  rebuild-host-bin\ rebuild-atlas-bins\ rebuild-cron-bins) ;;
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
  sed -n '2p' "$dir/make.log" | grep -Fq -- "rebuild-host-bin rebuild-atlas-bins rebuild-cron-bins" || \
    fail "second session-start command was not the docker-free host/bin rebuild (got: $(sed -n '2p' "$dir/make.log"))"
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

# ── Freshness rule: judge the binary against the last BUILD INPUT commit ────
# 2026-09-23: `make check-binaries` went red whenever HEAD had moved past the
# deployed binaries, even when the commits in between could not possibly
# change a binary (docker-compose*.yml, docs/**, scripts/** other than
# cron-entrypoint.sh). That is the state the project is in most of the time,
# so the gate stopped carrying information. These cases pin the corrected rule
# down with a throwaway git repo + a docker stub, so they need no daemon.

# git with an identity/commit shape that never depends on the developer config.
fake_git() {
  local repo=$1
  shift
  git -C "$repo" -c commit.gpgsign=false -c user.email=test@example.com \
    -c user.name=test "$@"
}

# Build <dir>/repo with this history:
#   FAKE_EARLY_COMMIT  docs/seed.md                      (not a build input)
#   FAKE_BUILD_COMMIT  main.go                           (last build input)
#   FAKE_HEAD_COMMIT   docker-compose.yml + docs/readme.md  (HEAD, no build input)
make_fake_repo() {
  local dir=$1
  local fake_top root_top
  FAKE_REPO="$dir/repo"
  mkdir -p "$FAKE_REPO/docs"
  fake_git "$FAKE_REPO" init -q

  # Own-repo assertion (issue #1927). If a hook exported GIT_DIR, `git -C
  # <temp>` still acts on the caller's repo — `git init` then does not even
  # create <temp>/.git, and the fixture commits below would land on the branch
  # under test. Fail loudly and never commit.
  test -d "$FAKE_REPO/.git" || \
    fail "throwaway repo $FAKE_REPO has no .git directory (GIT_DIR leak?)"
  fake_top=$(physical_path "$FAKE_REPO")
  root_top=$(physical_path "$ROOT")
  test "$(cd "$FAKE_REPO" && git rev-parse --show-toplevel)" = "$fake_top" || \
    fail "throwaway repo $FAKE_REPO does not resolve to itself (GIT_DIR leak?); refusing to commit fixtures"
  test "$fake_top" != "$root_top" || \
    fail "throwaway repo $FAKE_REPO resolves to the caller's checkout; refusing to commit fixtures"

  printf 'seed\n' >"$FAKE_REPO/docs/seed.md"
  fake_git "$FAKE_REPO" add docs/seed.md
  fake_git "$FAKE_REPO" commit -q -m "docs seed"
  FAKE_EARLY_COMMIT=$(git -C "$FAKE_REPO" rev-parse HEAD)

  printf 'package main\n' >"$FAKE_REPO/main.go"
  fake_git "$FAKE_REPO" add main.go
  fake_git "$FAKE_REPO" commit -q -m "go change"
  FAKE_BUILD_COMMIT=$(git -C "$FAKE_REPO" rev-parse HEAD)

  printf 'services: {}\n' >"$FAKE_REPO/docker-compose.yml"
  printf 'readme\n' >"$FAKE_REPO/docs/readme.md"
  fake_git "$FAKE_REPO" add docker-compose.yml docs/readme.md
  fake_git "$FAKE_REPO" commit -q -m "compose + docs only"
  FAKE_HEAD_COMMIT=$(git -C "$FAKE_REPO" rev-parse HEAD)
}

# docker stub reporting FAKE_BINARY_COMMIT_LINE as every binary's buildinfo.
write_freshness_stub_docker() {
  cat >"$1/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  create) printf 'fake-container\n' ;;
  cp) printf '%s\n' "${FAKE_BINARY_COMMIT_LINE}" >"${@: -1}" ;;
  *) : ;;
esac
EOF
  chmod +x "$1/docker"
}

# Run the checker with cwd=$1 and the docker stub reporting $2, from an
# isolated copy of the script ($3/tool) so its REPO_ROOT — used only for
# bin/atlas-mcp — is not this checkout. Prints output; returns its exit status.
run_check_with_stub() {
  local cwd=$1
  local commit_line=$2
  local dir=$3
  mkdir -p "$dir/freshness"
  ( cd "$cwd" && \
    FAKE_BINARY_COMMIT_LINE="$commit_line" \
    DOCKER_BIN="$dir/docker" \
    FRESHNESS_TMPDIR="$dir/freshness" \
    bash "$dir/tool/check-binary-freshness.sh" 2>&1 )
}

run_freshness_rule_tests() {
  local dir out status
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  make_fake_repo "$dir"
  mkdir -p "$dir/tool"
  cp "$CHECK" "$dir/tool/check-binary-freshness.sh"
  write_freshness_stub_docker "$dir"

  # (a) HEAD leads the last build-input commit, but the commits in between only
  #     touched docker-compose.yml + docs → FRESH. This is the exact state that
  #     used to be reported as STALE (2026-09-23: binary=7126c3a7, HEAD=17e67439).
  if out=$(run_check_with_stub "$FAKE_REPO" "Commit=$FAKE_BUILD_COMMIT" "$dir"); then
    status=0
  else
    status=$?
  fi
  test "$status" -eq 0 || fail "(a) binary built at the last build-input commit was reported STALE (exit $status): $out"
  printf '%s\n' "$out" | grep -Fq -- "last build input commit: $FAKE_BUILD_COMMIT" || \
    fail "(a) checker did not print the last build input commit"

  # (b) The last build-input commit is NEWER than the binary → STALE.
  if out=$(run_check_with_stub "$FAKE_REPO" "Commit=$FAKE_EARLY_COMMIT" "$dir"); then
    status=0
  else
    status=$?
  fi
  test "$status" -eq 1 || fail "(b) binary older than the last build-input commit was not reported STALE (exit $status): $out"
  printf '%s\n' "$out" | grep -Fq -- "STALE" || fail "(b) STALE did not appear in the summary"
  printf '%s\n' "$out" | grep -Fq -- "last build input=$FAKE_BUILD_COMMIT" || \
    fail "(b) STALE reason did not name the last build-input commit"

  # (c) Binary without buildinfo.Commit → MISSING_BUILDINFO, still exit 1.
  if out=$(run_check_with_stub "$FAKE_REPO" "no buildinfo in this binary" "$dir"); then
    status=0
  else
    status=$?
  fi
  test "$status" -eq 1 || fail "(c) binary without buildinfo was not reported missing (exit $status): $out"
  printf '%s\n' "$out" | grep -Fq -- "MISSING_BUILDINFO" || fail "(c) MISSING_BUILDINFO did not appear in the summary"

  # (d) A build commit this clone does not know cannot be proven fresh.
  if out=$(run_check_with_stub "$FAKE_REPO" "Commit=deadbeefdeadbeefdeadbeefdeadbeefdeadbeef" "$dir"); then
    status=0
  else
    status=$?
  fi
  test "$status" -eq 1 || fail "(d) unknown build commit was accepted as FRESH (exit $status): $out"

  # (e) Real history: a binary built at this repo's last build-input commit is
  #     FRESH even though HEAD is ahead of it. Guards against the checker
  #     silently going back to "binary commit must equal HEAD".
  local real_build
  real_build=$(git -C "$ROOT" log -1 --format=%H -- '*.go' go.mod go.sum 'Dockerfile*' scripts/cron-entrypoint.sh)
  if [ -n "$real_build" ]; then
    if out=$(run_check_with_stub "$ROOT" "Commit=$real_build" "$dir"); then
      status=0
    else
      status=$?
    fi
    test "$status" -eq 0 || fail "(e) this repo's last build-input commit was reported STALE (exit $status): $out"
    printf '%s\n' "$out" | grep -Fq -- "last build input commit: $real_build" || \
      fail "(e) checker disagreed with 'git log -1 -- *.go go.mod go.sum Dockerfile* scripts/cron-entrypoint.sh'"
  fi
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
  # The freshness gate judges against the last build input, not HEAD.
  assert_contains "$ROOT/scripts/check-binary-freshness.sh" "LAST_BUILD_COMMIT"
  assert_contains "$ROOT/scripts/check-binary-freshness.sh" "'scripts/cron-entrypoint.sh'"
  # This test builds throwaway repos, so it must drop a hook's GIT_DIR and
  # assert the fixtures resolve to themselves (issue #1927).
  assert_contains "$ROOT/tests/scripts/test-binary-freshness-guard.sh" 'unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE'
  assert_contains "$ROOT/tests/scripts/test-binary-freshness-guard.sh" 'rev-parse --show-toplevel'
}

run_cron_retag_test
run_cleanup_failure_test
run_missing_image_test
run_success_cleanup_test
run_freshness_rule_tests
run_session_start_test
run_static_contract_tests

echo "PASS: binary freshness guard contract tests"
