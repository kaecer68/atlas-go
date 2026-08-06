#!/usr/bin/env bash
# Full rebuild script for atlas-go with FRESH frontend dist.
# The `coverage-verify.sh` script earlier did not rebuild frontend; this
# version does, so the new coverage-notice UX (sq-scope-hint under the
# input + sq-scope-notice badge per missing card) actually reaches the
# browser.
#
# Run in MAIN WORKTREE only — invokes docker compose which the AI may not.
#
# pipeline:
#   (1) frontend build (client_web + shared_web + admin_web → dist/)
#   (2) host binaries rebuilt (Go picks up the new dist via //go:embed)
#   (3) docker image built (image bakes the new binary into atlas-atlas)
#   (4) atlas-go container restarted with the new image
#   (5) host bin/atlas-mcp rebuilt and restarted (stdio MCP, not a container)

set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

echo "[1/5] Frontend build (client_web + shared_web + admin_web) ..."
pushd client_web >/dev/null
npm run build
popd >/dev/null

pushd shared_web >/dev/null
npm run build
popd >/dev/null

pushd admin_web >/dev/null
npm run build
popd >/dev/null

echo "[2/5] Rebuild host binaries (atlas-go + crons) ..."
make rebuild-atlas-bins

echo "[3/5] Rebuild host bin/atlas-mcp ..."
make rebuild-host-bin

echo "[4/5] Rebuild atlas docker image ..."
docker compose build atlas

echo "[5/5] Restart atlas-go container + warmup wait + 5-endpoint smoke ..."
docker compose up -d atlas

# Wait for readiness (auth-free /api/health/aggregate)
for i in {1..60}; do
  code=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 3 "http://localhost:18080/api/health/aggregate" 2>/dev/null || echo 000)
  if [[ "$code" == 2* ]]; then
    echo "  atlas-go ready after ${i}s (health=$code)"
    break
  fi
  sleep 1
done

# 5 endpoints × {3131, 3587, 6641}
for sym in 3131 3587 6641; do
  echo
  echo "=== $sym coverage ==="
  curl -sS -w "\nHTTP=%{http_code} T=%{time_total}s\n" "http://localhost:18080/api/stock/coverage?symbol=$sym"
  for ep in quote fundamentals chips technical; do
    echo "=== $sym /api/stock/$ep ==="
    curl -sS -w "\nHTTP=%{http_code} T=%{time_total}s\n" --max-time 20 "http://localhost:18080/api/stock/$ep?symbol=$sym"
  done
done

echo
echo "Manual frontend verification still required: open browser,"
echo "navigate to /client/stock-quote?symbol=3131 and confirm the"
echo "sq-scope-hint text under the search input AND a sq-scope-notice"
echo "in each of fundamentals/chips/technical cards."
