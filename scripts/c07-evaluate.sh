#!/bin/bash
#
# scripts/c07-evaluate.sh — Day-7 / Day-14 evaluator wrapper that self-detects
# the current observation day from the earliest record in the obs log.
#
# Purpose:
#   - Reads the obs log, finds the earliest record date (= observation start).
#   - Computes today's observation day (calendar days from start; weekends
#     count as days so the Day 7/14 gate matches calendar-week cadence).
#   - On Day 7 / Day 14: runs the evaluator, writes report to a dated file.
#   - On other days: exits 0 silently (no-op).
#
# Why this wrapper exists:
#   The cron-entrypoint (scripts/cron-entrypoint.sh) supports a single static
#   cron expression. But Day-7 / Day-14 are date-anchored, not weekday-anchored.
#   This wrapper handles the date arithmetic so we can run a single daily cron
#   at 09:00 weekdays and have it do the right thing on the right days.
#
# Exit codes (matching c07-day-evaluator semantics):
#   - 0: today is not Day 7 nor Day 14 (normal no-op)
#   - 1: today is Day 7 or Day 14 AND at least one MUST criterion FAILED
#        (the underlying c07-day-evaluator already returns 1)
#   - 2: missing obs log / parse failure
#
# See: docs/specs/experimental-feature-launch-gate-spec.md §2.3, §3 and PR #1201.

set -uo pipefail

C07_OBS_LOG="${C07_OBS_LOG:-/app/.omo/evidence/sector-prediction-observation-log.md}"
C07_REPORT_DIR="${C07_REPORT_DIR:-/app/reports/c07-evaluation}"
TZ="${C07_TZ:-Asia/Taipei}"
export TZ

TODAY="$(date +%Y-%m-%d)"

if [ ! -f "${C07_OBS_LOG}" ]; then
    echo "[c07-evaluate ${TODAY}] obs log not found at ${C07_OBS_LOG} — Day 7 cannot anchor" >&2
    exit 2
fi

# Extract the earliest YYYY-MM-DD that appears in a records-row (| YYYY-MM-DD | …).
# grep on the records table rows only; date format is fixed.
START_DATE="$(
    grep -oE '^\| [0-9]{4}-[0-9]{2}-[0-9]{2} \|' "${C07_OBS_LOG}" \
    | head -n 1 \
    | awk '{print $2}'
)"

if [ -z "${START_DATE}" ]; then
    echo "[c07-evaluate ${TODAY}] no dated records found in ${C07_OBS_LOG}" >&2
    exit 2
fi

# Day N: calendar-day difference + 1 (Day 1 = start_date itself).
# Cross-platform date-to-epoch: GNU date uses -d, BSD/macOS date uses -j -f.
to_epoch() {
    # Portable YYYY-MM-DD → epoch: GNU/busybox → BSD/macOS → python3 fallback.
    # BusyBox's `date -d "YYYY-MM-DD"` works in the Dockerfile.cron alpine image
    # (GNU date in most distros also accepts the same form). BSD's `date -j -f`
    # is the equivalent on macOS dev hosts. Python is the last-resort catch-all
    # for whatever exotic env we end up running on.
    local d="$1"
    local out
    out="$(date -d "${d}" +%s 2>/dev/null)" && [ -n "${out}" ] && echo "${out}" && return 0
    out="$(date -j -f "%Y-%m-%d" "${d}" +%s 2>/dev/null)" && [ -n "${out}" ] && echo "${out}" && return 0
    out="$(python3 -c "import datetime, sys; sys.stdout.write(str(int(datetime.datetime.strptime(sys.argv[1], '%Y-%m-%d').timestamp())))" "${d}" 2>/dev/null)" && [ -n "${out}" ] && echo "${out}" && return 0
    return 1
}

START_EPOCH="$(to_epoch "${START_DATE}")" || {
    echo "[c07-evaluate ${TODAY}] date parse failed for start=${START_DATE}" >&2
    exit 2
}
TODAY_EPOCH="$(to_epoch "${TODAY}")" || {
    echo "[c07-evaluate ${TODAY}] date parse failed for today=${TODAY}" >&2
    exit 2
}

DAY_NUMBER="$(( (TODAY_EPOCH - START_EPOCH) / 86400 + 1 ))"

echo "[c07-evaluate ${TODAY}] start=${START_DATE} day=${DAY_NUMBER}"

# We only emit a formal report on Day 7 / Day 14. Anything else is a silent
# no-op so the cron doesn't spam the operator.
if [ "${DAY_NUMBER}" -ne 7 ] && [ "${DAY_NUMBER}" -ne 14 ]; then
    echo "[c07-evaluate ${TODAY}] not an evaluation day (only Day 7 / Day 14 trigger); no-op"
    exit 0
fi

mkdir -p "${C07_REPORT_DIR}"
REPORT_FILE="${C07_REPORT_DIR}/day-${DAY_NUMBER}-${TODAY}.md"

echo "[c07-evaluate ${TODAY}] running evaluator --day ${DAY_NUMBER} → ${REPORT_FILE}"

# Generate the report and capture exit code.
EVALUATOR_BIN="${C07_EVALUATOR_BIN:-}"
if [ -z "${EVALUATOR_BIN}" ]; then
    if [ -x /app/c07-day-evaluator ]; then
        EVALUATOR_BIN="/app/c07-day-evaluator"
    elif [ -x ./cmd/experimental/c07-day-evaluator/c07-day-evaluator ]; then
        EVALUATOR_BIN="./cmd/experimental/c07-day-evaluator/c07-day-evaluator"
    else
        EVALUATOR_BIN="go run ./cmd/experimental/c07-day-evaluator"
    fi
fi

set +e
${EVALUATOR_BIN} \
    -obs-log "${C07_OBS_LOG}" \
    -day "${DAY_NUMBER}" \
    -output "${REPORT_FILE}"
RC=$?
set -e

if [ "${RC}" -eq 0 ]; then
    echo "[c07-evaluate ${TODAY}] PASS — see ${REPORT_FILE}"
elif [ "${RC}" -eq 1 ]; then
    echo "[c07-evaluate ${TODAY}] FAIL (Day ${DAY_NUMBER} criteria) — see ${REPORT_FILE}; per runbook §4 consider rollback" >&2
else
    echo "[c07-evaluate ${TODAY}] evaluator exit=${RC}" >&2
fi

exit "${RC}"
