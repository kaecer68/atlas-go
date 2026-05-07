#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "$PROJECT_ROOT"

KEEP_TEMP=false
TMP_ROOT=""

usage() {
  cat <<'EOF'
Usage: ./scripts/openclaw/verify_human_approval_event.sh [OPTIONS]

Verifies human approval event schema and replayability.

Checks:
  1) human-approval emits contract-compliant JSON
  2) event file persisted under data/state/approvals matches emitted payload
  3) replay-approval-event can replay from stored event in dry-run mode

Options:
  --keep-temp    Keep temporary files for inspection
  --help         Show this help
EOF
}

log() {
  echo "[verify-approval] $*"
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "[error] missing command: $cmd"
    exit 1
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --keep-temp)
      KEEP_TEMP=true
      shift
      ;;
    --help)
      usage
      exit 0
      ;;
    *)
      echo "[error] unknown option: $1"
      usage
      exit 1
      ;;
  esac
done

require_cmd jq

TMP_ROOT="$(mktemp -d /tmp/atlas-approval-verify.XXXXXX)"
if [[ "$KEEP_TEMP" != true ]]; then
  trap 'rm -rf "$TMP_ROOT"' EXIT
fi

EVENT_JSON="$TMP_ROOT/event.json"
SCHEMA_CHECK="$TMP_ROOT/schema_check.json"

log "Generating reject decision event in dry-run mode"
./scripts/openclaw/human_approval.sh \
  --reject \
  --experiment "exp-verify-human-approval" \
  --reason "schema-replay verification" \
  --actor "verify-bot" \
  --dry-run \
  --json > "$EVENT_JSON"

log "Validating required schema fields"
jq -e '
  .decision_id | type == "string" and length > 0
' "$EVENT_JSON" >/dev/null
jq -e '
  .timestamp | type == "string" and length > 0
' "$EVENT_JSON" >/dev/null
jq -e '
  .actor == "verify-bot"
' "$EVENT_JSON" >/dev/null
jq -e '
  .action == "reject"
' "$EVENT_JSON" >/dev/null
jq -e '
  .reason == "schema-replay verification"
' "$EVENT_JSON" >/dev/null
jq -e '
  .dry_run == true
' "$EVENT_JSON" >/dev/null
jq -e '
  .experiment_id == "exp-verify-human-approval"
' "$EVENT_JSON" >/dev/null

EVENT_ID="$(jq -r '.decision_id' "$EVENT_JSON")"
EVENT_FILE="data/state/approvals/${EVENT_ID}.json"

if [[ ! -f "$EVENT_FILE" ]]; then
  echo "[error] expected event file not found: $EVENT_FILE"
  exit 1
fi

log "Comparing emitted JSON and persisted JSON"
jq -S . "$EVENT_JSON" > "$TMP_ROOT/event.sorted.json"
jq -S . "$EVENT_FILE" > "$TMP_ROOT/file.sorted.json"
if ! diff -u "$TMP_ROOT/event.sorted.json" "$TMP_ROOT/file.sorted.json" >/dev/null; then
  echo "[error] persisted event file differs from emitted payload"
  diff -u "$TMP_ROOT/event.sorted.json" "$TMP_ROOT/file.sorted.json" || true
  exit 1
fi

log "Replaying stored event in dry-run mode"
./scripts/openclaw/replay_approval_event.sh --event "$EVENT_FILE" --dry-run --json > "$SCHEMA_CHECK"
jq -e '.replayed_from == "'"$EVENT_ID"'"' "$SCHEMA_CHECK" >/dev/null
jq -e '.replay_output.action == "reject"' "$SCHEMA_CHECK" >/dev/null

log "Human approval event verification passed"
if [[ "$KEEP_TEMP" == true ]]; then
  log "Temp files kept at: $TMP_ROOT"
fi
