#!/usr/bin/env bash
# Hand-off script for the stock-quote coverage-notice fix (commit 793fc79c).
# Run in the MAIN WORKTREE only — rebuilds atlas-atlas binary, rebuilds host
# bin/atlas-mcp, rebuilds the `atlas` docker service image, restarts the
# `atlas-go` container, and runs the 15-curl live verification.
#
# DO NOT change to use `atlas-mcp` as a docker compose service name — it is
# NOT in docker-compose.yml. atlas-mcp is a host binary, not a container.
#
# Verified against: docker-compose.yml 2026-08-06 (atlas=true, atlas-mcp absent)
#                   git HEAD 793fc79c on fix/20260806-stock-coverage-notice

set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

echo "[1/6] Rebuild host binaries (atlas-go + atlas-mcp + crons) ..."
make rebuild-atlas-bins

echo "[2/6] Rebuild host bin/atlas-mcp ..."
make rebuild-host-bin

echo "[3/6] Rebuild the `atlas` docker image (service name) ..."
docker compose build atlas

echo "[4/6] Restart atlas-go container (container_name) ..."
docker compose up -d atlas

echo "[5/6] Wait for HTTP readiness on :18080 ..."
for i in {1..30}; do
  if curl -sS -o /dev/null -w '%{http_code}' http://localhost:18080/api/health/live 2>/dev/null \
       | grep -q '^2'; then
    echo "  atlas-go ready after ${i}s"
    break
  fi
  sleep 1
done

echo "[6/6] Verify coverage endpoint + 4 stocktools endpoints × {3131, 3587, 6641} ..."
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
echo "Done. Expected: 3131/3587 → chips/fundamentals/technical return 200 + coverage_note, NOT 503."
echo "              6641 → all 5 endpoints normal 200."
