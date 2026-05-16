#!/bin/bash
# Daily TWSE data collection — run after market close (e.g., 14:30 CST)
# Usage: ./scripts/daily-twse-fetch.sh [YYYY-MM-DD]
# Default: yesterday (T-1). Cron: 30 14 * * 1-5 cd /path/to/atlas && ./scripts/daily-twse-fetch.sh

set -euo pipefail

YESTERDAY="${1:-$(date -v-1d +%Y-%m-%d)}"
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MAIN_DATASET="${ATLAS_REPLAY_DATA_PATH:-data/replay/merged_5y.jsonl}"
LOG_DIR="data/replay/logs"

cd "$PROJECT_ROOT"
mkdir -p "$LOG_DIR"

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Fetching TWSE data for $YESTERDAY"

go run ./cmd/fetch-historical \
    -start "$YESTERDAY" \
    -end "$YESTERDAY" \
    -output "data/replay/twse_${YESTERDAY}.jsonl" \
    -merge-with "$MAIN_DATASET" 2>&1 | tee "$LOG_DIR/fetch_${YESTERDAY}.log"

# Validate: expect 800-1200 records on a normal trading day
RECORDS=$(grep -c '"date"' "data/replay/twse_${YESTERDAY}.jsonl" 2>/dev/null || echo 0)
if [ "$RECORDS" -lt 100 ]; then
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] ⚠️  WARNING: Only $RECORDS records for $YESTERDAY (possible holiday)"
else
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] ✅ Imported $RECORDS records for $YESTERDAY"
fi
