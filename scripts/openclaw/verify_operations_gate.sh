#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "$PROJECT_ROOT"

KEEP_TEMP=false
WITH_GOVERNANCE=false
TMP_ROOT=""

usage() {
	cat <<'EOF'
Usage: ./scripts/openclaw/verify_operations_gate.sh [OPTIONS]

Verifies M8 operations readiness with staging-safe drills.

Checks:
  1) Runbook command coverage for rollback and replay operations
  2) Prometheus config sanity for atlas metrics scraping
  3) Dry-run rollback approval event generation and replay
  4) Human approval event schema/replay validation

Options:
  --with-governance  Also run strict governance gate verification
  --keep-temp        Keep temporary files for inspection
  --help             Show this help
EOF
}

log() {
	echo "[verify-ops] $*"
}

require_cmd() {
	local cmd="$1"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		echo "[error] missing command: $cmd"
		exit 1
	fi
}

assert_file_contains() {
	local file="$1"
	local pattern="$2"
	local hint="$3"
	if ! grep -qE "$pattern" "$file"; then
		echo "[error] expected pattern not found in $file: $hint"
		exit 1
	fi
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		--with-governance)
			WITH_GOVERNANCE=true
			shift
			;;
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
require_cmd grep

TMP_ROOT="$(mktemp -d /tmp/atlas-ops-verify.XXXXXX)"
if [[ "$KEEP_TEMP" != true ]]; then
	trap 'rm -rf "$TMP_ROOT"' EXIT
fi

validate_runbook_contract() {
	local runbook="docs/operations-playbook.md"
	if [[ ! -f "$runbook" ]]; then
		echo "[error] runbook not found: $runbook"
		exit 1
	fi

	log "Validating runbook coverage for rollback and replay workflow"
	assert_file_contains "$runbook" 'human_approval\.sh --revert' 'revert decision entry'
	assert_file_contains "$runbook" 'replay_approval_event\.sh --event' 'approval event replay command'
	assert_file_contains "$runbook" 'verify_human_approval_event\.sh' 'approval event verifier command'
	assert_file_contains "$runbook" 'verify_governance_gates\.sh --require-scenario-diversity' 'strict governance gate command'
}

validate_prometheus_config() {
	local prometheus="monitoring/prometheus.yml"
	if [[ ! -f "$prometheus" ]]; then
		echo "[error] prometheus config not found: $prometheus"
		exit 1
	fi

	log "Validating Prometheus scrape config for atlas metrics"
	assert_file_contains "$prometheus" '^global:' 'global section'
	assert_file_contains "$prometheus" '^scrape_configs:' 'scrape_configs section'
	assert_file_contains "$prometheus" "job_name: 'atlas-go'" 'atlas-go scrape job'
	assert_file_contains "$prometheus" '^\s+metrics_path: /metrics' 'metrics endpoint path'
	assert_file_contains "$prometheus" '^\s+scrape_timeout: 5s' 'scrape timeout guard'
}

run_rollback_dryrun_drill() {
	local replay_log="$TMP_ROOT/revert.replay.log"
	local event_file

	log "Running dry-run rollback drill via human approval wrapper"
	./scripts/openclaw/human_approval.sh \
		--revert \
		0 \
		--reason "operations gate rollback drill" \
		--actor "verify-ops-bot" \
		--dry-run >/dev/null

	event_file="$(ls -t data/state/approvals/decision-*.json 2>/dev/null | head -n 1 || true)"
	if [[ -z "$event_file" ]]; then
		echo "[error] rollback drill did not produce approval event file"
		exit 1
	fi

	jq -e '.action == "revert"' "$event_file" >/dev/null
	jq -e '.dry_run == true' "$event_file" >/dev/null
	jq -e '.actor == "verify-ops-bot"' "$event_file" >/dev/null
	jq -e '.reason == "operations gate rollback drill"' "$event_file" >/dev/null
	jq -e '.revert_target == "0"' "$event_file" >/dev/null

	if [[ ! -f "$event_file" ]]; then
		echo "[error] rollback drill event not persisted: $event_file"
		exit 1
	fi

	log "Replaying rollback drill event (dry-run)"
	./scripts/openclaw/replay_approval_event.sh --event "$event_file" --dry-run > "$replay_log"
	assert_file_contains "$replay_log" '^\[replay\] action: revert' 'replay action log'
}

run_human_approval_contract_check() {
	log "Running human approval event schema/replay verifier"
	./scripts/openclaw/verify_human_approval_event.sh
}

run_optional_governance_check() {
	if [[ "$WITH_GOVERNANCE" != true ]]; then
		return 0
	fi

	log "Running strict governance verification (optional)"
	./scripts/openclaw/verify_governance_gates.sh --require-scenario-diversity
}

log "Start operations gate verification"

validate_runbook_contract
validate_prometheus_config
run_rollback_dryrun_drill
run_human_approval_contract_check
run_optional_governance_check

log "Operations gate verification passed (M8 staging drill baseline)"
if [[ "$KEEP_TEMP" == true ]]; then
	log "Temp files kept at: $TMP_ROOT"
fi