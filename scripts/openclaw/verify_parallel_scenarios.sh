#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

START_DATE="2026-03-20"
END_DATE="2026-03-27"
REPLAY_DATA_PATH="${ATLAS_REPLAY_DATA_PATH:-samples/replay/twse_stock_day_all_sample.csv}"
BASE_POLICY_PATH="${ATLAS_BASELINE_POLICY_PATH:-data/state/baseline_policy.json}"
OUTPUT_PATH=""
KEEP_TEMP=false
REQUIRE_DIVERSITY=false

usage() {
	cat <<'EOF'
Usage: ./scripts/openclaw/verify_parallel_scenarios.sh [options]

Options:
  --start YYYY-MM-DD      Backtest window start date (default: 2026-03-26)
  --end YYYY-MM-DD        Backtest window end date (default: 2026-03-27)
  --replay-data PATH      Replay CSV/JSONL path (default: samples/replay/twse_stock_day_all_sample.csv)
  --baseline-policy PATH  Baseline policy path (default: data/state/baseline_policy.json)
  --output PATH           Output comparison JSON path (default: data/state/windows/scenario-compare-<window>.json)
	--require-diversity     Fail when base/stress/shock scenario signatures are identical
  --keep-temp             Keep temp artifacts under /tmp for debugging
  --help                  Show this help

This script verifies M5 by:
  1) Running base/stress/shock scenarios with isolated ledger dirs
  2) Re-running each scenario and asserting deterministic output hash
  3) Writing a scenario comparison report JSON for audit
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
	echo "[verify-scenarios] $*"
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
		--baseline-policy)
			BASE_POLICY_PATH="$2"
			shift 2
			;;
		--output)
			OUTPUT_PATH="$2"
			shift 2
			;;
		--require-diversity)
			REQUIRE_DIVERSITY=true
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

WINDOW_ID="window-${START_DATE//-/}-${END_DATE//-/}"
if [[ -z "$OUTPUT_PATH" ]]; then
	OUTPUT_PATH="data/state/windows/scenario-compare-${WINDOW_ID}.json"
fi
mkdir -p "$(dirname "$OUTPUT_PATH")"

TMP_ROOT="$(mktemp -d /tmp/atlas-scenario-verify.XXXXXX)"
if [[ "$KEEP_TEMP" != true ]]; then
	trap 'rm -rf "$TMP_ROOT"' EXIT
fi

if [[ ! -f "$BASE_POLICY_PATH" ]]; then
	log "baseline policy not found at $BASE_POLICY_PATH; bootstrapping default policy for verification"
	BASE_POLICY_PATH="$TMP_ROOT/default-baseline-policy.json"
	cat > "$BASE_POLICY_PATH" <<'EOF'
{
	"version": 1,
	"prompt_overrides": {},
	"constraints": {
		"starting_cash": 3000000,
		"max_position_weight": 0.18,
		"max_open_positions": 5,
		"min_tradable_volume": 1000000,
		"min_recommendation_conviction": 60,
		"require_cro_pass": true,
		"transaction_cost_bps": 1.425,
		"slippage_bps": 4,
		"reserve_cash_fraction": 0.1
	},
	"execution_policy": {
		"conviction_floor": 60,
		"require_cro_pass": true
	},
	"promotions": []
}
EOF
fi

BASE_POLICY_JSON="$TMP_ROOT/base.policy.json"
STRESS_POLICY_JSON="$TMP_ROOT/stress.policy.json"
SHOCK_POLICY_JSON="$TMP_ROOT/shock.policy.json"
cp "$BASE_POLICY_PATH" "$BASE_POLICY_JSON"

jq '.constraints.min_recommendation_conviction = 55
    | .execution_policy.conviction_floor = 55
    | .constraints.max_position_weight = 0.18
    | .constraints.reserve_cash_fraction = 0.15' "$BASE_POLICY_JSON" > "$STRESS_POLICY_JSON"

jq '.constraints.min_recommendation_conviction = 70
    | .execution_policy.conviction_floor = 70
    | .constraints.max_position_weight = 0.12
    | .constraints.max_open_positions = 3
    | .constraints.reserve_cash_fraction = 0.25
    | .constraints.slippage_bps = 8' "$BASE_POLICY_JSON" > "$SHOCK_POLICY_JSON"

run_once() {
	local policy_path="$1"
	local ledger_dir="$2"
	local normalized_out="$3"

	mkdir -p "$ledger_dir"
	ATLAS_LEDGER_DIR="$ledger_dir" \
	ATLAS_REPLAY_DATA_PATH="$REPLAY_DATA_PATH" \
	ATLAS_BASELINE_POLICY_PATH="$policy_path" \
		go run ./cmd/backtest-window -start "$START_DATE" -end "$END_DATE" >/dev/null

	local window_path="$ledger_dir/windows/${WINDOW_ID}.json"
	if [[ ! -f "$window_path" ]]; then
		echo "[error] expected window artifact not found: $window_path"
		exit 1
	fi
	jq -S 'del(.GeneratedAt)' "$window_path" > "$normalized_out"
}

latest_session_dir() {
	local ledger_dir="$1"
	local session_dir

	session_dir="$(find "$ledger_dir/sessions" -mindepth 1 -maxdepth 1 -type d -print0 2>/dev/null | xargs -0 ls -td 2>/dev/null | head -n 1 || true)"
	if [[ -z "$session_dir" ]]; then
		echo "[error] no session directory found under: $ledger_dir/sessions"
		exit 1
	fi

	echo "$session_dir"
}

extract_scenario_signal() {
	local ledger_dir="$1"
	local window_normalized="$2"
	local signal_out="$3"
	local metrics_out="$4"
	local session_dir
	local session_summary
	local outcomes_file
	local session_normalized="$TMP_ROOT/$(basename "$ledger_dir").session.normalized.json"
	local outcomes_stats="$TMP_ROOT/$(basename "$ledger_dir").outcomes.stats.json"

	session_dir="$(latest_session_dir "$ledger_dir")"
	session_summary="$session_dir/summary.json"
	outcomes_file="$session_dir/recommendation_outcomes.jsonl"

	if [[ ! -f "$session_summary" ]]; then
		echo "[error] session summary not found: $session_summary"
		exit 1
	fi
	if [[ ! -f "$outcomes_file" ]]; then
		echo "[error] session outcomes not found: $outcomes_file"
		exit 1
	fi

	jq -S 'del(.recorded_at, .proposal_id, .commit_id, .approval_id)' "$session_summary" > "$session_normalized"
	jq -s '{
		total: length,
		hit_count: (map(select(.hit == true)) | length),
		avg_forward_return: (if length == 0 then 0 else (map(.forward_return) | add / length) end),
		avg_benchmark_delta: (if length == 0 then 0 else (map(.benchmark_delta) | add / length) end),
		unique_symbols: (map(.symbol) | unique | length),
		unique_agents: (map(.agent_id) | unique | length)
	}' "$outcomes_file" > "$outcomes_stats"

	jq -n \
		--slurpfile window "$window_normalized" \
		--slurpfile session "$session_normalized" \
		--slurpfile outcomes "$outcomes_stats" \
		'{
			window_summary: $window[0],
			session_summary: $session[0],
			outcomes_stats: $outcomes[0]
		}' > "$signal_out"

	jq -n \
		--slurpfile session "$session_normalized" \
		--slurpfile outcomes "$outcomes_stats" \
		'{
		order_count: ($session[0].order_count // 0),
		position_count: ($session[0].position_count // 0),
		ending_cash: ($session[0].ending_cash // 0),
		outcome_count: ($session[0].outcome_count // 0),
		next_experiment_agent_id: ($session[0].next_experiment_agent_id // ""),
			guard: {
				total: (($session[0].guard_outcomes // []) | length),
				hard_total: (($session[0].guard_outcomes // []) | map(select(.severity == "hard")) | length),
				hard_blocked: (($session[0].guard_outcomes // []) | map(select(.severity == "hard" and (.passed | not))) | length),
				soft_total: (($session[0].guard_outcomes // []) | map(select(.severity == "soft")) | length),
				soft_blocked: (($session[0].guard_outcomes // []) | map(select(.severity == "soft" and (.passed | not))) | length)
			},
			outcomes_stats: $outcomes[0]
		}' > "$metrics_out"
}

verify_scenario() {
	local scenario="$1"
	local policy_path="$2"
	local window_sha_out="$3"
	local signal_sha_out="$4"
	local signal_det_out="$5"
	local summary_out="$6"
	local constraints_out="$7"
	local execution_out="$8"
	local metrics_out="$9"
	local run1_ledger="$TMP_ROOT/${scenario}.run1.ledger"
	local run2_ledger="$TMP_ROOT/${scenario}.run2.ledger"
	local run1_normalized="$TMP_ROOT/${scenario}.run1.normalized.json"
	local run2_normalized="$TMP_ROOT/${scenario}.run2.normalized.json"
	local run1_signal="$TMP_ROOT/${scenario}.run1.signal.json"
	local run2_signal="$TMP_ROOT/${scenario}.run2.signal.json"
	local run1_window="$run1_ledger/windows/${WINDOW_ID}.json"
	local window_sha1
	local window_sha2
	local signal_sha1
	local signal_sha2

	log "Running scenario ${scenario} (run 1)"
	run_once "$policy_path" "$run1_ledger" "$run1_normalized"
	extract_scenario_signal "$run1_ledger" "$run1_normalized" "$run1_signal" "$metrics_out"
	log "Running scenario ${scenario} (run 2)"
	run_once "$policy_path" "$run2_ledger" "$run2_normalized"
	extract_scenario_signal "$run2_ledger" "$run2_normalized" "$run2_signal" "$TMP_ROOT/${scenario}.run2.metrics.json"

	window_sha1="$(shasum "$run1_normalized" | awk '{print $1}')"
	window_sha2="$(shasum "$run2_normalized" | awk '{print $1}')"
	signal_sha1="$(shasum "$run1_signal" | awk '{print $1}')"
	signal_sha2="$(shasum "$run2_signal" | awk '{print $1}')"
	echo "[verify-scenarios] ${scenario}_window_sha_run1=${window_sha1}"
	echo "[verify-scenarios] ${scenario}_window_sha_run2=${window_sha2}"
	echo "[verify-scenarios] ${scenario}_signal_sha_run1=${signal_sha1}"
	echo "[verify-scenarios] ${scenario}_signal_sha_run2=${signal_sha2}"

	if [[ "$window_sha1" != "$window_sha2" ]]; then
		echo "[error] scenario determinism failed for: ${scenario}"
		echo "[error] window hash mismatch"
		diff -u "$run1_normalized" "$run2_normalized" | head -120 || true
		exit 1
	fi

	if [[ "$signal_sha1" != "$signal_sha2" ]]; then
		echo "[verify-scenarios] warning: ${scenario} signal hash differs between runs"
		echo "[verify-scenarios] warning: default mode continues; strict mode will fail"
		echo false > "$signal_det_out"
	else
		echo true > "$signal_det_out"
	fi

	echo "$window_sha1" > "$window_sha_out"
	echo "$signal_sha1" > "$signal_sha_out"
	cp "$run1_window" "$summary_out"
	jq '.Constraints' "$policy_path" > "$constraints_out"
	jq '.ExecutionPolicy' "$policy_path" > "$execution_out"
}

BASE_WINDOW_SHA_FILE="$TMP_ROOT/base.window.sha"
STRESS_WINDOW_SHA_FILE="$TMP_ROOT/stress.window.sha"
SHOCK_WINDOW_SHA_FILE="$TMP_ROOT/shock.window.sha"

BASE_SIGNAL_SHA_FILE="$TMP_ROOT/base.signal.sha"
STRESS_SIGNAL_SHA_FILE="$TMP_ROOT/stress.signal.sha"
SHOCK_SIGNAL_SHA_FILE="$TMP_ROOT/shock.signal.sha"

BASE_SIGNAL_DET_FILE="$TMP_ROOT/base.signal.det"
STRESS_SIGNAL_DET_FILE="$TMP_ROOT/stress.signal.det"
SHOCK_SIGNAL_DET_FILE="$TMP_ROOT/shock.signal.det"

BASE_SUMMARY_FILE="$TMP_ROOT/base.summary.json"
STRESS_SUMMARY_FILE="$TMP_ROOT/stress.summary.json"
SHOCK_SUMMARY_FILE="$TMP_ROOT/shock.summary.json"

BASE_CONSTRAINTS_FILE="$TMP_ROOT/base.constraints.json"
STRESS_CONSTRAINTS_FILE="$TMP_ROOT/stress.constraints.json"
SHOCK_CONSTRAINTS_FILE="$TMP_ROOT/shock.constraints.json"

BASE_EXECUTION_FILE="$TMP_ROOT/base.execution.json"
STRESS_EXECUTION_FILE="$TMP_ROOT/stress.execution.json"
SHOCK_EXECUTION_FILE="$TMP_ROOT/shock.execution.json"

BASE_METRICS_FILE="$TMP_ROOT/base.metrics.json"
STRESS_METRICS_FILE="$TMP_ROOT/stress.metrics.json"
SHOCK_METRICS_FILE="$TMP_ROOT/shock.metrics.json"

log "Start M5 parallel scenario verification"
log "Window: ${START_DATE} -> ${END_DATE}"
log "Replay data: ${REPLAY_DATA_PATH}"

verify_scenario "base" "$BASE_POLICY_JSON" "$BASE_WINDOW_SHA_FILE" "$BASE_SIGNAL_SHA_FILE" "$BASE_SIGNAL_DET_FILE" "$BASE_SUMMARY_FILE" "$BASE_CONSTRAINTS_FILE" "$BASE_EXECUTION_FILE" "$BASE_METRICS_FILE"
verify_scenario "stress" "$STRESS_POLICY_JSON" "$STRESS_WINDOW_SHA_FILE" "$STRESS_SIGNAL_SHA_FILE" "$STRESS_SIGNAL_DET_FILE" "$STRESS_SUMMARY_FILE" "$STRESS_CONSTRAINTS_FILE" "$STRESS_EXECUTION_FILE" "$STRESS_METRICS_FILE"
verify_scenario "shock" "$SHOCK_POLICY_JSON" "$SHOCK_WINDOW_SHA_FILE" "$SHOCK_SIGNAL_SHA_FILE" "$SHOCK_SIGNAL_DET_FILE" "$SHOCK_SUMMARY_FILE" "$SHOCK_CONSTRAINTS_FILE" "$SHOCK_EXECUTION_FILE" "$SHOCK_METRICS_FILE"

BASE_WINDOW_SHA="$(cat "$BASE_WINDOW_SHA_FILE")"
STRESS_WINDOW_SHA="$(cat "$STRESS_WINDOW_SHA_FILE")"
SHOCK_WINDOW_SHA="$(cat "$SHOCK_WINDOW_SHA_FILE")"

BASE_SIGNAL_SHA="$(cat "$BASE_SIGNAL_SHA_FILE")"
STRESS_SIGNAL_SHA="$(cat "$STRESS_SIGNAL_SHA_FILE")"
SHOCK_SIGNAL_SHA="$(cat "$SHOCK_SIGNAL_SHA_FILE")"

BASE_SIGNAL_DETERMINISTIC="$(cat "$BASE_SIGNAL_DET_FILE")"
STRESS_SIGNAL_DETERMINISTIC="$(cat "$STRESS_SIGNAL_DET_FILE")"
SHOCK_SIGNAL_DETERMINISTIC="$(cat "$SHOCK_SIGNAL_DET_FILE")"
SIGNAL_DETERMINISTIC=true

if [[ "$BASE_SIGNAL_DETERMINISTIC" != true || "$STRESS_SIGNAL_DETERMINISTIC" != true || "$SHOCK_SIGNAL_DETERMINISTIC" != true ]]; then
	SIGNAL_DETERMINISTIC=false
fi
SCENARIO_DIVERSITY=true

if [[ "$BASE_SIGNAL_SHA" == "$STRESS_SIGNAL_SHA" && "$STRESS_SIGNAL_SHA" == "$SHOCK_SIGNAL_SHA" ]]; then
	SCENARIO_DIVERSITY=false
	log "warning: base/stress/shock scenario signal hashes are identical on this window"
fi

jq -n \
	--arg generated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
	--arg start_date "$START_DATE" \
	--arg end_date "$END_DATE" \
	--arg replay_data_path "$REPLAY_DATA_PATH" \
	--arg baseline_policy_path "$BASE_POLICY_PATH" \
	--arg output_version "v1" \
	--arg base_window_sha "$BASE_WINDOW_SHA" \
	--arg stress_window_sha "$STRESS_WINDOW_SHA" \
	--arg shock_window_sha "$SHOCK_WINDOW_SHA" \
	--arg base_signal_sha "$BASE_SIGNAL_SHA" \
	--arg stress_signal_sha "$STRESS_SIGNAL_SHA" \
	--arg shock_signal_sha "$SHOCK_SIGNAL_SHA" \
	--argjson signal_deterministic "$SIGNAL_DETERMINISTIC" \
	--argjson require_diversity "$REQUIRE_DIVERSITY" \
	--argjson scenario_diversity "$SCENARIO_DIVERSITY" \
	--slurpfile base_summary "$BASE_SUMMARY_FILE" \
	--slurpfile stress_summary "$STRESS_SUMMARY_FILE" \
	--slurpfile shock_summary "$SHOCK_SUMMARY_FILE" \
	--slurpfile base_metrics "$BASE_METRICS_FILE" \
	--slurpfile stress_metrics "$STRESS_METRICS_FILE" \
	--slurpfile shock_metrics "$SHOCK_METRICS_FILE" \
	--slurpfile base_constraints "$BASE_CONSTRAINTS_FILE" \
	--slurpfile stress_constraints "$STRESS_CONSTRAINTS_FILE" \
	--slurpfile shock_constraints "$SHOCK_CONSTRAINTS_FILE" \
	--slurpfile base_execution "$BASE_EXECUTION_FILE" \
	--slurpfile stress_execution "$STRESS_EXECUTION_FILE" \
	--slurpfile shock_execution "$SHOCK_EXECUTION_FILE" \
	'{
		version: $output_version,
		generated_at: $generated_at,
		window: {
			start_date: $start_date,
			end_date: $end_date
		},
		sources: {
			replay_data_path: $replay_data_path,
			baseline_policy_path: $baseline_policy_path
		},
		evaluation: {
			deterministic_per_scenario: true,
			signal_deterministic_per_scenario: $signal_deterministic,
			require_diversity: $require_diversity,
			scenario_diversity: $scenario_diversity
		},
		scenarios: {
			base: {
				window_deterministic_sha: $base_window_sha,
				signal_deterministic_sha: $base_signal_sha,
				constraints: $base_constraints[0],
				execution_policy: $base_execution[0],
				metrics: $base_metrics[0],
				summary: $base_summary[0]
			},
			stress: {
				window_deterministic_sha: $stress_window_sha,
				signal_deterministic_sha: $stress_signal_sha,
				constraints: $stress_constraints[0],
				execution_policy: $stress_execution[0],
				metrics: $stress_metrics[0],
				summary: $stress_summary[0]
			},
			shock: {
				window_deterministic_sha: $shock_window_sha,
				signal_deterministic_sha: $shock_signal_sha,
				constraints: $shock_constraints[0],
				execution_policy: $shock_execution[0],
				metrics: $shock_metrics[0],
				summary: $shock_summary[0]
			}
		}
	}' > "$OUTPUT_PATH"

if [[ "$REQUIRE_DIVERSITY" == true ]]; then
	if [[ "$SCENARIO_DIVERSITY" != true ]]; then
		echo "[error] scenario diversity requirement not met (--require-diversity)"
		echo "[error] see artifact: $OUTPUT_PATH"
		exit 1
	fi
	if [[ "$SIGNAL_DETERMINISTIC" != true ]]; then
		echo "[error] signal-level determinism requirement not met in strict mode"
		echo "[error] see artifact: $OUTPUT_PATH"
		exit 1
	fi
fi

log "Scenario comparison artifact written: $OUTPUT_PATH"
log "M5 parallel scenario verification passed"
if [[ "$KEEP_TEMP" == true ]]; then
	log "Temp artifacts kept at: $TMP_ROOT"
fi
