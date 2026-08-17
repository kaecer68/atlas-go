#!/usr/bin/env bash
# Test scripts/check-routes.sh — route uniqueness check must NOT scan _test.go.
#
# Regression for the Phase 0 false-positive: tests register fake/virtual
# routes (e.g. mux.Handle("/test/sse-fake", ...) or mux.Handle on a path the
# prod code registers via mux.HandleFunc) to exercise middleware; check-routes.sh
# used to count those as real routes and could flag phantom duplicates/conflicts.
#
# Scenarios:
#   1. _test.go registers mux.Handle on a path prod registers via mux.HandleFunc
#      → must PASS (old code FAILed with a phantom DUPLICATE).
#      Also asserts fake test-only paths never appear in the route list.
#   2. A REAL handle-vs-handlefunc duplicate in non-test files → must still FAIL.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT="$ROOT/scripts/check-routes.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# Build a fake repo tree under $tmp that mirrors the dirs check-routes.sh scans.
make_fixture() {
  local tmp=$1
  mkdir -p "$tmp/scripts" "$tmp/cmd/atlas" "$tmp/internal/apigateway"
  cp "$SCRIPT" "$tmp/scripts/check-routes.sh"
}

# Scenario 1: test file uses a fake route + duplicates a prod path via mux.Handle
# while prod uses mux.HandleFunc → old code flagged DUPLICATE; new code passes.
scenario1_false_positive_eliminated() {
  local tmp
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' RETURN
  make_fixture "$tmp"

  cat >"$tmp/cmd/atlas/routes.go" <<'EOF'
package main

// real production route registered via HandleFunc
func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", healthHandler)
}
EOF

  cat >"$tmp/internal/apigateway/route_test.go" <<'EOF'
package apigateway

// Test-only: fake virtual route to exercise middleware. Uses mux.Handle on a
// path the production code registers via mux.HandleFunc — the exact pattern
// that used to make check-routes.sh report a phantom DUPLICATE.
func TestMiddlewareWithFakeRoute(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/api/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	mux.Handle("/test/sse-fake", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
}
EOF

  local output
  output=$(bash "$tmp/scripts/check-routes.sh" 2>&1) || fail "scenario1: expected PASS, got exit $?"
  echo "$output" | grep -q "✅ PASS" || fail "scenario1: output did not end with PASS:\n$output"
  echo "$output" | grep -q "/test/sse-fake" && fail "scenario1: fake test route leaked into real route list"
  echo "  ✓ scenario1: _test.go fake routes excluded (no phantom DUPLICATE)"
}

# Scenario 2: a real duplicate between two non-test files must still be caught.
scenario2_real_duplicate_still_detected() {
  local tmp
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' RETURN
  make_fixture "$tmp"

  cat >"$tmp/cmd/atlas/routes.go" <<'EOF'
package main

func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", healthHandler)
}
EOF

  cat >"$tmp/cmd/atlas/extra.go" <<'EOF'
package main

// real second registration of the same path via a different mux call — a true
// duplicate the check exists to catch.
func registerExtra(mux *http.ServeMux) {
	mux.Handle("/api/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
}
EOF

  local output
  if output=$(bash "$tmp/scripts/check-routes.sh" 2>&1); then
    fail "scenario2: expected FAIL on real duplicate, got exit 0"
  fi
  echo "$output" | grep -q "DUPLICATE: /api/health" || \
    fail "scenario2: duplicate not reported:\n$output"
  echo "  ✓ scenario2: real non-test duplicate still FAILs"
}

echo "=== test-check-routes.sh ==="
scenario1_false_positive_eliminated
scenario2_real_duplicate_still_detected
echo "✅ test-check-routes.sh PASSED (2/2)"
