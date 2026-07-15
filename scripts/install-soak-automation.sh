#!/usr/bin/env bash
# install-soak-automation.sh — install the staging soak test
# automation on a developer Mac. Idempotent: safe to re-run.
#
# Installs:
#   - scripts/staging-soak-check.sh → ~/bin/
#   - scripts/com.atlas.soak-check.plist → ~/Library/LaunchAgents/
#   - log directory at ~/logs/atlas-soak/
#   - LaunchAgent loaded (so the 06:00 UTC daily schedule is active)
#
# Run once per machine. Use `launchctl kickstart -k gui/$(id -u)/com.atlas.soak-check`
# to force an immediate run, or wait for 06:00 UTC.

set -euo pipefail

REPO_DIR="${REPO_DIR:-$(pwd)}"
SCRIPT_SRC="$REPO_DIR/scripts/staging-soak-check.sh"
PLIST_SRC="$REPO_DIR/scripts/com.atlas.soak-check.plist"

SCRIPT_DST="$HOME/bin/staging-soak-check.sh"
PLIST_DST="$HOME/Library/LaunchAgents/com.atlas.soak-check.plist"
LOG_DIR="$HOME/logs/atlas-soak"

LABEL="com.atlas.soak-check"

echo "=== Atlas staging soak automation installer ==="

# 1. script
echo "[1/4] install script to $SCRIPT_DST"
mkdir -p "$HOME/bin"
cp "$SCRIPT_SRC" "$SCRIPT_DST"
chmod +x "$SCRIPT_DST"

# 2. plist
echo "[2/4] install LaunchAgent to $PLIST_DST"
mkdir -p "$HOME/Library/LaunchAgents"
cp "$PLIST_SRC" "$PLIST_DST"
chmod 644 "$PLIST_DST"

# 3. log dir
echo "[3/4] create log dir at $LOG_DIR"
mkdir -p "$LOG_DIR"

# 4. load
echo "[4/4] launchctl bootstrap $LABEL"
if launchctl list | grep -q "$LABEL"; then
    echo "  already loaded, kickstarting immediate run"
    launchctl kickstart -k "gui/$(id -u)/$LABEL"
else
    launchctl bootstrap "gui/$(id -u)" "$PLIST_DST"
    echo "  loaded; first run will be at 06:00 UTC"
fi

echo "=== install complete ==="
echo "verify: cat $LOG_DIR/$(date -u +%Y-%m-%d).json | jq ."
echo "manual run: launchctl kickstart -k gui/\$(id -u)/$LABEL"
