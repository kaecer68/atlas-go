#!/usr/bin/env bash
# ci-wait-healthy.sh — wait for a docker compose service to become healthy.
#
# Fail-fast replacement for the old inline wait loops in ci-cd.yml
# (canary-test / hermes-smoke). The old loops kept running the tests even
# when the container never became healthy, and dumped zero diagnostics on
# timeout — every failure mode looked identical ("connection refused").
#
# On timeout: exits 1 AND dumps docker compose ps + container inspect +
# service logs so the real startup failure is visible in the Actions log.
#
# Usage: ci-wait-healthy.sh <service> <timeout_seconds>
set -euo pipefail

SERVICE="${1:?usage: ci-wait-healthy.sh <service> <timeout_seconds>}"
TIMEOUT_SEC="${2:?usage: ci-wait-healthy.sh <service> <timeout_seconds>}"
CONTAINER="atlas-go"

echo "⏳ Waiting up to ${TIMEOUT_SEC}s for ${SERVICE} to become healthy..."
for i in $(seq 1 $((TIMEOUT_SEC / 2))); do
  status="$(docker compose ps --format '{{.Status}}' "${SERVICE}" 2>/dev/null || true)"
  if echo "${status}" | grep -q healthy; then
    echo "✅ ${SERVICE} healthy after ~$((i * 2))s"
    exit 0
  fi
  if (( i % 15 == 0 )); then
    echo "… still waiting (~$((i * 2))s): ${status:-<no status>}"
  fi
  sleep 2
done

echo "❌ ${SERVICE} did not become healthy within ${TIMEOUT_SEC}s" >&2
echo "" >&2
echo "── docker compose ps ──" >&2
docker compose ps 2>&1 || true
echo "" >&2
echo "── docker inspect ${CONTAINER} ──" >&2
docker inspect "${CONTAINER}" --format 'State={{.State.Status}} Restarts={{.RestartCount}} ExitCode={{.State.ExitCode}} Error={{.State.Error}} Health={{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}} StartedAt={{.State.StartedAt}}' 2>&1 || true
echo "" >&2
echo "── docker logs ${CONTAINER} (tail 200) ──" >&2
docker logs "${CONTAINER}" --tail 200 2>&1 || true
echo "" >&2
echo "── docker logs atlas-postgres (tail 80) ──" >&2
docker logs atlas-postgres --tail 80 2>&1 || true
echo "" >&2
echo "── docker logs atlas-redis (tail 40) ──" >&2
docker logs atlas-redis --tail 40 2>&1 || true
exit 1
