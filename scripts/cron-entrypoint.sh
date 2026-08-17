#!/bin/sh
# cron-entrypoint.sh — lightweight cron scheduler for Docker (no crond dependency)
# Replaces dcron to avoid Docker Desktop macOS seccomp setpgid block.
# Checks schedule every 60s; matches cron fields (min hour day month weekday).
set -e

# Timezone note: schedule fields below are matched against `date` output in
# the container's local timezone. Cron images are Alpine with no TZ set, so
# local time == UTC; all CRON_SCHEDULE values in docker-compose.yml are
# therefore UTC-based. To run a job in another zone, set TZ on its compose
# service (the entrypoint then matches in that zone). See also
# docker-compose.yml (cron services) and Dockerfile.cron.

if [ -z "$CRON_SCHEDULE" ] || [ -z "$CRON_COMMAND" ]; then
    echo "WARN: CRON_SCHEDULE or CRON_COMMAND not set. No cron job installed."
    echo "Sleeping forever to keep container alive."
    exec tail -f /dev/null
fi

SCHED_MIN=$(echo "$CRON_SCHEDULE" | awk '{print $1}')
SCHED_HOUR=$(echo "$CRON_SCHEDULE" | awk '{print $2}')
SCHED_DAY=$(echo "$CRON_SCHEDULE" | awk '{print $3}')
SCHED_MONTH=$(echo "$CRON_SCHEDULE" | awk '{print $4}')
SCHED_WDAY=$(echo "$CRON_SCHEDULE" | awk '{print $5}')

crontab_field_match() {
    local sched="$1" now="$2"
    case "$sched" in
        "*") return 0 ;;
        *)
            local saved_ifs="$IFS"
            IFS=','
            for part in $sched; do
                IFS="$saved_ifs"
                case "$part" in
                    *-*) local lo=${part%-*} hi=${part#*-}
                         [ "$((10#$now))" -ge "$((10#$lo))" ] && [ "$((10#$now))" -le "$((10#$hi))" ] && return 0 ;;
                    *)
                        # Only plain integer values are supported here; skip
                        # anything else (e.g. step syntax) so it never matches.
                        case "$part" in *[!0-9]*) continue ;; esac
                        [ "$((10#$part))" -eq "$((10#$now))" ] && return 0 ;;
                esac
            done
            IFS="$saved_ifs"
            return 1
            ;;
    esac
}

# Match the full CRON_SCHEDULE against the current (or simulated) instant.
# NOW_MIN/NOW_HOUR/NOW_DAY/NOW_MONTH/NOW_WDAY come from date(1) and are
# zero-padded ("08"); CRON_SCHEDULE fields are usually not ("8"). Compare
# numerically via base-10 arithmetic ($((10#x))) so "08" == "8" and
# "00" == "0" — plain string equality never matches and silently disables
# the schedule (P0-E' leading-zero bug).
cron_schedule_matches() {
    MATCH=1
    [ "$SCHED_MIN" != "*" ] && [ "$((10#$NOW_MIN))" -ne "$((10#$SCHED_MIN))" ] && MATCH=0
    [ "$SCHED_HOUR" != "*" ] && [ "$((10#$NOW_HOUR))" -ne "$((10#$SCHED_HOUR))" ] && MATCH=0
    [ "$SCHED_DAY" != "*" ] && [ "$((10#$NOW_DAY))" -ne "$((10#$SCHED_DAY))" ] && MATCH=0
    [ "$SCHED_MONTH" != "*" ] && [ "$((10#$NOW_MONTH))" -ne "$((10#$SCHED_MONTH))" ] && MATCH=0
    [ "$SCHED_WDAY" != "*" ] && ! crontab_field_match "$SCHED_WDAY" "$NOW_WDAY" && MATCH=0
    [ "$MATCH" -eq 1 ]
}

# Test hook (not used in production): evaluate CRON_SCHEDULE against a
# simulated instant and exit 0 (match) / 1 (no match). CRON_MATCH_TEST is a
# colon-separated MIN:HOUR:DAY:MONTH:WDAY string injected by
# tests/scripts/test-cron-entrypoint.sh — same code path as the live loop.
if [ -n "${CRON_MATCH_TEST:-}" ]; then
    IFS=: read -r NOW_MIN NOW_HOUR NOW_DAY NOW_MONTH NOW_WDAY <<EOF
$CRON_MATCH_TEST
EOF
    cron_schedule_matches || exit 1
    exit 0
fi

echo "Cron job scheduled: $CRON_SCHEDULE $CRON_COMMAND"
mkdir -p /var/log/cron

while true; do
    NOW_MIN=$(date +%M)
    NOW_HOUR=$(date +%H)
    NOW_DAY=$(date +%d)
    NOW_MONTH=$(date +%m)
    NOW_WDAY=$(date +%w)

    if cron_schedule_matches; then
        echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Running: $CRON_COMMAND" >> /var/log/cron/cron.log
        eval "$CRON_COMMAND" >> /var/log/cron/cron.log 2>&1 || true
        echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Done" >> /var/log/cron/cron.log
    fi

    sleep 60
done