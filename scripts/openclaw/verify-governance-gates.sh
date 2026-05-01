#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

START_DATE="2026-03-20"
END_DATE="2026-03-27"
REPLAY_DATA_PATH="${ATLAS_REPLAY_DATA_PATH:-samples/replay/twse_stock_day_all_sample.csv}"
KEEP_TEMP=false
REQUIRE_SCENARIO_DIVERSITY=false

usage() {
	cat <<'EOF'
Usage: ./scripts/openclaw/verify-governance-gates.sh [options]

Options:
	--start YYYY-MM-DD     Backtest window start date (default: 2026-03-26)
	--end YYYY-MM-DD       Backtest window end date (default: 2026-03-27)
	--replay-data PATH     Replay CSV/JSONL path (default: samples/replay/twse_stock_day_all_sample.csv)
	--require-scenario-diversity
	                       Fail if M5 base/stress/shock scenario signals are identical
	--keep-temp            Keep temp artifacts under /tmp for debugging
	--help                 Show this help

This script verifies the full governance gate set (G1-G4) plus milestone checks:
	G1 contract gate (agent registry schema + prompt files + baseline policy)
	G2 replay determinism (clean ledger dirs, hash compare)
	G3 hard-guard blocking behavior (targeted test)
	G4 trace and dashboard persistence checks (targeted tests)
	M5 base/stress/shock scenario determinism + comparison artifact
	M7 approval event schema + replayability (dry-run)

Note: G5 (operations gate: rollback drill, runbook, prometheus) is verified
separately by ./scripts/openclaw/verify-operations-gate.sh
EOF
}

require_cmd() {
	local cmd="$1"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		echo "[error] required command not found: $cmd"
		exit 1
	fi
}

log() {
	echo "[verify] $*"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		--start)
			START_DATE="$2"
			shift 2
			;;
		--end)
			END_DATE="$2"
			shift 2
			;;
		--replay-data)
			REPLAY_DATA_PATH="$2"
			shift 2
			;;
		--require-scenario-diversity)
			REQUIRE_SCENARIO_DIVERSITY=true
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

if [[ ! -f "$REPLAY_DATA_PATH" ]]; then
	echo "[error] replay data path not found: $REPLAY_DATA_PATH"
	exit 1
fi

require_cmd go
require_cmd jq
require_cmd shasum

TMP_ROOT="$(mktemp -d /tmp/atlas-governance-gates.XXXXXX)"
if [[ "$KEEP_TEMP" != true ]]; then
	trap 'rm -rf "$TMP_ROOT"' EXIT
fi

run_contract_gate_check() {
	log "Running contract gate verification (G1)"

	# G1-1: Agent registry schema validation via go build + registry test
	if ! go build ./internal/orchestrator/... >/dev/null 2>&1; then
		echo "[error] G1 failed: orchestrator build failed (registry schema incompatible)"
		exit 1
	fi

	# G1-2: Verify all enabled agents have corresponding prompt files
	local agents_config="configs/agents.json"
	if [[ -f "$agents_config" ]]; then
		local missing_prompts=()
		while IFS= read -r pf; do
			if [[ -n "$pf" && ! -f "$pf" ]]; then
				missing_prompts+=("$pf")
			fi
		done < <(jq -r '.agents[] | select(.enabled == true) | .promptFile // empty' "$agents_config")

		if [[ ${#missing_prompts[@]} -gt 0 ]]; then
			echo "[error] G1 failed: enabled agents missing prompt files:"
			printf '%s\n' "${missing_prompts[@]}"
			exit 1
		fi
		log "Agent registry: all enabled agents have prompt files"
	fi

	# G1-3: Verify baseline policy exists and is valid JSON
	local baseline_policy="${ATLAS_BASELINE_POLICY_PATH:-data/state/baseline_policy.json}"
	if [[ -f "$baseline_policy" ]]; then
		if ! jq empty "$baseline_policy" >/dev/null 2>&1; then
			echo "[error] G1 failed: baseline policy is not valid JSON: $baseline_policy"
			exit 1
		fi
		log "Baseline policy: valid JSON at $baseline_policy"
	else
		log "Baseline policy: not found at $baseline_policy (will use default for backtest)"
	fi

	log "G1 contract gate passed"
}

run_targeted_tests() {
	log "Running governance-focused tests"
	go test -count=2 \
		./internal/domain/... \
		./internal/experiment/... \
		./internal/orchestrator/... \
		./internal/monitoring/... \
		./internal/ledger/...

	go test ./internal/orchestrator/... -run TestControlLayerHardGuardCanBlockAllRecommendations -count=1
	go test ./internal/ledger/... -run TestRecordSessionSummaryPersistsTraceIDs -count=1
	go test ./internal/monitoring/... -run TestDashboardAPIEndpoints -count=1
}

run_clean_backtest() {
	local ledger_dir="$1"
	local normalized_out="$2"
	local window_id="window-${START_DATE//-/}-${END_DATE//-/}"
	local window_path

	mkdir -p "$ledger_dir"
	ATLAS_LEDGER_DIR="$ledger_dir" \
	ATLAS_REPLAY_DATA_PATH="$REPLAY_DATA_PATH" \
		go run ./cmd/backtest-window -start "$START_DATE" -end "$END_DATE" >/dev/null

	window_path="$ledger_dir/windows/${window_id}.json"
	if [[ ! -f "$window_path" ]]; then
		echo "[error] expected window artifact not found: $window_path"
		exit 1
	fi

	jq -S 'del(.GeneratedAt)' "$window_path" > "$normalized_out"
}

run_determinism_check() {
	local run1_ledger="$TMP_ROOT/ledger1"
	local run2_ledger="$TMP_ROOT/ledger2"
	local run1_json="$TMP_ROOT/window1.normalized.json"
	local run2_json="$TMP_ROOT/window2.normalized.json"
	local sha1_1 sha1_2

	log "Running replay determinism check in clean ledger dirs"
	run_clean_backtest "$run1_ledger" "$run1_json"
	run_clean_backtest "$run2_ledger" "$run2_json"

	sha1_1="$(shasum "$run1_json" | awk '{print $1}')"
	sha1_2="$(shasum "$run2_json" | awk '{print $1}')"

	echo "[verify] deterministic_sha_run1=$sha1_1"
	echo "[verify] deterministic_sha_run2=$sha1_2"

	if [[ "$sha1_1" != "$sha1_2" ]]; then
		echo "[error] replay determinism check failed"
		diff -u "$run1_json" "$run2_json" | head -120 || true
		exit 1
	fi
}

run_human_approval_event_check() {
	log "Running human approval event verification (M7)"
	./scripts/openclaw/verify-human-approval-event.sh
}

run_parallel_scenario_check() {
	local args=(
		--start "$START_DATE"
		--end "$END_DATE"
		--replay-data "$REPLAY_DATA_PATH"
	)

	if [[ "$REQUIRE_SCENARIO_DIVERSITY" == true ]]; then
		args+=(--require-diversity)
	fi

	log "Running parallel scenario verification (M5)"
	./scripts/openclaw/verify-parallel-scenarios.sh "${args[@]}"
}

log "Start governance gate verification"
log "Window: ${START_DATE} -> ${END_DATE}"
log "Replay data: ${REPLAY_DATA_PATH}"

run_contract_gate_check
run_targeted_tests
run_determinism_check
run_parallel_scenario_check
run_human_approval_event_check

log "All governance gates passed (G1/G2/G3/G4 + M5 + M7)"
if [[ "$KEEP_TEMP" == true ]]; then
	log "Temp artifacts kept at: $TMP_ROOT"
fi
