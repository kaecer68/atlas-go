#!/bin/bash
#
# scripts/c07-collect.sh — Daily collector wrapper for the C07 sector
# prediction observation window.
#
# Purpose:
#   Append one row per Taiwan trading day to the obs log, fire alerts
#   on threshold breaches, and exit non-zero so cron-detectors can flag
#   persistent collector failures.
#
# Environment overrides:
#   ATLAS_BASE_URL       — atlas base URL (default http://atlas:18080 inside
#                          the docker network; use http://localhost:18080 on
#                          host crontab).
#   C07_OBS_LOG          — absolute path to the obs log markdown.
#   C07_ALERT_DIR        — alert store dir (same path atlas-go writes to).
#   C07_TZ               — TZ for date stamping (default Asia/Taipei).
#
# Failures:
#   - Any curl/HTTP failure → exit 1 (alert store will still record via atlas).
#   - jq missing or malformed obs log → exit 2 (operator must fix).
#
# See: docs/specs/experimental-feature-launch-gate-spec.md（Reference Implementations）and PR #1201.

set -uo pipefail

ATLAS_BASE_URL="${ATLAS_BASE_URL:-http://atlas:18080}"
C07_OBS_LOG="${C07_OBS_LOG:-/app/.omo/evidence/sector-prediction-observation-log.md}"
C07_ALERT_DIR="${C07_ALERT_DIR:-/app/data/state/alerts}"
TZ="${C07_TZ:-Asia/Taipei}"
export TZ

DATE="$(date +%Y-%m-%d)"
LOG_TAG="[c07-collect ${DATE}]"

echo "${LOG_TAG} base_url=${ATLAS_BASE_URL} obs_log=${C07_OBS_LOG}"

# Quick reachability check: if /health is not 200 we still record an empty row
# (with a note) so that obs-window gaps are visible. This is friendlier than
# silently skipping the day.
if ! HEALTH="$(curl -fsS -m 5 "${ATLAS_BASE_URL}/health" 2>/dev/null)"; then
    echo "${LOG_TAG} WARN: atlas /health unreachable; appending unreachable row"
    mkdir -p "$(dirname "${C07_OBS_LOG}")"
    echo "| ${DATE} | - | - | - | - | - | - | atlas /health unreachable @ ${DATE} |" >> "${C07_OBS_LOG}"
    exit 1
fi

# Run the collector (won't fail just because flag is off — it'll record
# 'flag off' in the notes and exit 0).
# Path resolution: prefer /app/c07-obs-collector (inside Dockerfile.cron
# image), fall back to on-host path so the same script works under host
# crontab without rebuilding the container.
COLLECTOR_BIN="${C07_COLLECTOR_BIN:-}"
if [ -z "${COLLECTOR_BIN}" ]; then
    if [ -x /app/c07-obs-collector ]; then
        COLLECTOR_BIN="/app/c07-obs-collector"
    elif [ -x ./cmd/experimental/c07-obs-collector/c07-obs-collector ]; then
        COLLECTOR_BIN="./cmd/experimental/c07-obs-collector/c07-obs-collector"
    else
        COLLECTOR_BIN="go run ./cmd/experimental/c07-obs-collector"
    fi
fi

${COLLECTOR_BIN} \
    -url "${ATLAS_BASE_URL}" \
    -obs-log "${C07_OBS_LOG}" \
    -alert-dir "${C07_ALERT_DIR}" \
    -date "${DATE}" 2>&1 | sed "s|^|${LOG_TAG} |"

RC="${PIPESTATUS[0]}"
echo "${LOG_TAG} collector exit=${RC}"
exit "${RC}"
