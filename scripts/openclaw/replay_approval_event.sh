#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "$PROJECT_ROOT"

EVENT_FILE=""
DRY_RUN=true
AUTO_CONFIRM=false
JSON_MODE=false

usage() {
	cat <<'EOF'
Usage: ./scripts/openclaw/replay_approval_event.sh --event <event.json> [OPTIONS]

Replay one stored human-approval event.

Options:
	--event PATH      Event JSON file under data/state/approvals/
	--dry-run         Replay without mutating state (default)
	--apply           Allow state mutation replay (disables default dry-run)
	--yes             Auto-confirm underlying decision command
	--json            Output replay result as JSON
	--help            Show this help
EOF
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
		--event)
			EVENT_FILE="$2"
			shift 2
			;;
		--dry-run)
			DRY_RUN=true
			shift
			;;
		--apply)
			DRY_RUN=false
			shift
			;;
		--yes)
			AUTO_CONFIRM=true
			shift
			;;
		--json)
			JSON_MODE=true
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

if [[ -z "$EVENT_FILE" ]]; then
	echo "[error] --event is required"
	exit 1
fi
if [[ ! -f "$EVENT_FILE" ]]; then
	echo "[error] event file not found: $EVENT_FILE"
	exit 1
fi

require_cmd jq

decision_id="$(jq -r '.decision_id // empty' "$EVENT_FILE")"
action="$(jq -r '.action // empty' "$EVENT_FILE")"
experiment_id="$(jq -r '.experiment_id // empty' "$EVENT_FILE")"
reason="$(jq -r '.reason // empty' "$EVENT_FILE")"
actor="$(jq -r '.actor // "replay-bot"' "$EVENT_FILE")"
revert_target="$(jq -r '.revert_target // empty' "$EVENT_FILE")"

if [[ -z "$decision_id" || -z "$action" || -z "$reason" ]]; then
	echo "[error] malformed event: required fields missing"
	exit 1
fi

cmd=(./scripts/openclaw/human_approval.sh)
case "$action" in
	approve)
		if [[ -z "$experiment_id" ]]; then
			echo "[error] approve event requires experiment_id"
			exit 1
		fi
		cmd+=(--approve --experiment "$experiment_id" --reason "$reason")
		;;
	reject)
		if [[ -n "$experiment_id" ]]; then
			cmd+=(--reject --experiment "$experiment_id" --reason "$reason")
		else
			cmd+=(--reject --reason "$reason")
		fi
		;;
	revert)
		cmd+=(--revert)
		if [[ -n "$revert_target" ]]; then
			cmd+=("$revert_target")
		fi
		cmd+=(--reason "$reason")
		;;
	*)
		echo "[error] unsupported action in event: $action"
		exit 1
		;;
esac

cmd+=(--actor "$actor")
if [[ "$DRY_RUN" == true ]]; then
	cmd+=(--dry-run)
fi
if [[ "$AUTO_CONFIRM" == true ]]; then
	cmd+=(--yes)
fi

if [[ "$JSON_MODE" == true ]]; then
	cmd+=(--json)
	output="$("${cmd[@]}")"
	jq -n \
		--arg replay_of "$decision_id" \
		--arg action "$action" \
		--arg event_file "$EVENT_FILE" \
		--arg replay_output "$output" \
		'{replayed_from: $replay_of, action: $action, event_file: $event_file, replay_output: ($replay_output | fromjson)}'
else
	echo "[replay] source event: $EVENT_FILE"
	echo "[replay] action: $action"
	"${cmd[@]}"
fi
