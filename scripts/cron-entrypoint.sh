#!/bin/sh
# cron-entrypoint.sh — lightweight cron scheduler for Docker (no crond dependency)
# Replaces dcron to avoid Docker Desktop macOS seccomp setpgid block.
# Checks schedule every 60s; matches cron fields (min hour day month weekday).
set -e

if [ -z "$CRON_SCHEDULE" ] || [ -z "$CRON_COMMAND" ]; then
    echo "WARN: CRON_SCHEDULE or CRON_COMMAND not set. No cron job installed."
    echo "Sleeping forever to keep container alive."
    exec tail -f /dev/null
fi

echo "Cron job scheduled: $CRON_SCHEDULE $CRON_COMMAND"
mkdir -p /var/log/cron

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
                         [ "$now" -ge "$lo" ] && [ "$now" -le "$hi" ] && return 0 ;;
                    *)    [ "$part" = "$now" ] && return 0 ;;
                esac
            done
            IFS="$saved_ifs"
            return 1
            ;;
    esac
}

while true; do
    NOW_MIN=$(date +%M)
    NOW_HOUR=$(date +%H)
    NOW_DAY=$(date +%d)
    NOW_MONTH=$(date +%m)
    NOW_WDAY=$(date +%w)

    MATCH=1
    [ "$SCHED_MIN" != "*" ] && [ "$NOW_MIN" != "$SCHED_MIN" ] && MATCH=0
    [ "$SCHED_HOUR" != "*" ] && [ "$NOW_HOUR" != "$SCHED_HOUR" ] && MATCH=0
    [ "$SCHED_DAY" != "*" ] && [ "$NOW_DAY" != "$SCHED_DAY" ] && MATCH=0
    [ "$SCHED_MONTH" != "*" ] && [ "$NOW_MONTH" != "$SCHED_MONTH" ] && MATCH=0
    [ "$SCHED_WDAY" != "*" ] && ! crontab_field_match "$SCHED_WDAY" "$NOW_WDAY" && MATCH=0

    if [ $MATCH -eq 1 ]; then
        echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Running: $CRON_COMMAND" >> /var/log/cron/cron.log
        eval "$CRON_COMMAND" >> /var/log/cron/cron.log 2>&1 || true
        echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Done" >> /var/log/cron/cron.log
    fi

    sleep 60
done