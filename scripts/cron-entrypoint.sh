#!/bin/sh
set -e

if [ -n "$CRON_SCHEDULE" ] && [ -n "$CRON_COMMAND" ]; then
    echo "$CRON_SCHEDULE $CRON_COMMAND >> /var/log/cron/cron.log 2>&1" | crontab -
    echo "Installed cron job: $CRON_SCHEDULE $CRON_COMMAND"
else
    echo "WARN: CRON_SCHEDULE or CRON_COMMAND not set. No cron job installed."
fi

mkdir -p /var/log/cron

exec crond -f -l 2
