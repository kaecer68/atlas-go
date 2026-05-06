#!/usr/bin/env bash
set -euo pipefail

# Daily execution bootstrap (non-interactive):
# status -> propose(auto) -> execute(auto) -> judge(auto) -> decision reminder

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

AGENT=""
MUTATION_TYPE=""
FALLBACK_TYPE="risk_rule_change"
ENABLE_FALLBACK=true
LAST_JUDGE_LOG=""
PREPARE_WINDOW=false
WINDOW_START=""
WINDOW_END=""
REPLAY_DATA_PATH=""
AUTO_PIVOT_ON_SKIP=true
MIN_SAMPLE_FOR_RANK=2

build_window_id() {
	local start_date="$1"
	local end_date="$2"
	if [[ -z "$start_date" || -z "$end_date" ]]; then
		echo ""
		return 0
	fi
	echo "window-${start_date//-/}-${end_date//-/}"
}

choose_alternative_mutation_type() {
	local current_type="$1"
	local agent_id="$2"
	local window_id="$3"
	local candidate_type
	local fallback_type=""
	local best_type=""
	local best_score=""
	for candidate_type in prompt_tightening risk_rule_change portfolio_constraint_revision; do
		local stats candidate_avg candidate_n candidate_score
		candidate_avg=""
		candidate_n=""
		candidate_score=""
		if [[ "$candidate_type" == "$current_type" ]]; then
			continue
		fi
		if is_mutation_futile "$agent_id" "$candidate_type" "$window_id"; then
			continue
		fi
		if [[ -z "$fallback_type" ]]; then
			fallback_type="$candidate_type"
		fi
		stats=$(mutation_recent_stats "$agent_id" "$candidate_type" "$window_id")
		if [[ -n "$stats" ]]; then
			read -r candidate_avg candidate_n candidate_score <<<"$stats"
		fi
		if [[ -z "$candidate_n" || "$candidate_n" -lt "$MIN_SAMPLE_FOR_RANK" ]]; then
			continue
		fi
		if [[ -z "$best_type" ]]; then
			best_type="$candidate_type"
			if [[ -n "$candidate_score" ]]; then
				best_score="$candidate_score"
			fi
			continue
		fi
		if [[ -z "$best_score" ]]; then
			if [[ -n "$candidate_score" ]]; then
				best_type="$candidate_type"
				best_score="$candidate_score"
			fi
			continue
		fi
		if [[ -n "$candidate_score" ]] && awk "BEGIN{exit !($candidate_score > $best_score)}"; then
			best_type="$candidate_type"
			best_score="$candidate_score"
		fi
	done
	if [[ -n "$best_type" ]]; then
		echo "$best_type"
		return 0
	fi
	echo "$fallback_type"
}

mutation_recent_stats() {
	local agent_id="$1"
	local mutation_type="$2"
	local window_id="$3"
	local lookback=5
	local total=0
	local sum=0
	local f

	if [[ ! -d "data/state/experiments" ]]; then
		echo ""
		return 0
	fi

	for f in $(ls -t data/state/experiments/*.json 2>/dev/null); do
		local agent mtype win baseline candidate delta
		agent=$(jq -r '.experiment.target_agent_id // .Experiment.TargetAgentID // empty' "$f" 2>/dev/null)
		mtype=$(jq -r '.experiment.mutation_type // .Experiment.MutationType // .brief.mutation_type // .Brief.mutation_type // empty' "$f" 2>/dev/null)
		win=$(jq -r '.brief.window_id // .Brief.window_id // empty' "$f" 2>/dev/null)
		if [[ "$agent" != "$agent_id" || "$mtype" != "$mutation_type" ]]; then
			continue
		fi
		if [[ -n "$window_id" && "$win" != "$window_id" ]]; then
			continue
		fi

		baseline=$(jq -r '.experiment.baseline_value // .Experiment.BaselineValue // empty' "$f" 2>/dev/null)
		candidate=$(jq -r '.experiment.candidate_value // .Experiment.CandidateValue // empty' "$f" 2>/dev/null)
		if [[ -z "$baseline" || -z "$candidate" || "$baseline" == "null" || "$candidate" == "null" ]]; then
			continue
		fi

		delta=$(awk "BEGIN{printf \"%.12f\", ($candidate - $baseline)}")
		sum=$(awk "BEGIN{printf \"%.12f\", ($sum + $delta)}")
		total=$((total + 1))
		if [[ $total -ge $lookback ]]; then
			break
		fi
	done

	if [[ $total -eq 0 ]]; then
		echo ""
		return 0
	fi

	local avg weighted
	avg=$(awk "BEGIN{printf \"%.12f\", ($sum / $total)}")
	weighted=$(awk "BEGIN{w=($total<$lookback)?($total/$lookback):1; printf \"%.12f\", ($avg*w)}")
	echo "$avg $total $weighted"
}

mutation_recent_average_delta() {
	local stats
	stats=$(mutation_recent_stats "$1" "$2" "$3")
	if [[ -z "$stats" ]]; then
		echo ""
		return 0
	fi
	awk '{print $1}' <<<"$stats"
}

print_rankable_mutation_candidates() {
	local current_type="$1"
	local agent_id="$2"
	local window_id="$3"
	local candidate_type

	echo "[pivot] Ranking candidates (min sample for rank: ${MIN_SAMPLE_FOR_RANK})"
	for candidate_type in prompt_tightening risk_rule_change portfolio_constraint_revision; do
		local stats candidate_avg candidate_n candidate_weighted
		local eligible="yes"
		if [[ "$candidate_type" == "$current_type" ]]; then
			continue
		fi
		if is_mutation_futile "$agent_id" "$candidate_type" "$window_id"; then
			echo "[pivot] - ${candidate_type}: skipped (futile in this window)"
			continue
		fi
		stats=$(mutation_recent_stats "$agent_id" "$candidate_type" "$window_id")
		if [[ -n "$stats" ]]; then
			read -r candidate_avg candidate_n candidate_weighted <<<"$stats"
		else
			candidate_avg="n/a"
			candidate_n=0
			candidate_weighted="n/a"
		fi
		if [[ "$candidate_n" -lt "$MIN_SAMPLE_FOR_RANK" ]]; then
			eligible="no"
		fi
		echo "[pivot] - ${candidate_type}: avg=${candidate_avg}, n=${candidate_n}, weighted=${candidate_weighted}, eligible=${eligible}"
	done
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		--agent)
			AGENT="$2"
			shift 2
			;;
		--type)
			MUTATION_TYPE="$2"
			shift 2
			;;
		--fallback-type)
			FALLBACK_TYPE="$2"
			shift 2
			;;
		--no-fallback)
			ENABLE_FALLBACK=false
			shift
			;;
		--prepare-window)
			PREPARE_WINDOW=true
			shift
			;;
		--window-start)
			WINDOW_START="$2"
			shift 2
			;;
		--window-end)
			WINDOW_END="$2"
			shift 2
			;;
		--replay-data)
			REPLAY_DATA_PATH="$2"
			shift 2
			;;
		--no-auto-pivot)
			AUTO_PIVOT_ON_SKIP=false
			shift
			;;
		--min-sample-for-rank)
			MIN_SAMPLE_FOR_RANK="$2"
			shift 2
			;;
		--help)
			echo "Usage: $0 [--agent <agent-id>] [--type <mutation-type>] [--fallback-type <type>] [--no-fallback] [--prepare-window] [--window-start <YYYY-MM-DD>] [--window-end <YYYY-MM-DD>] [--replay-data <path>] [--no-auto-pivot] [--min-sample-for-rank <n>]"
			echo ""
			echo "Options:"
			echo "  --agent ID      Target specific agent for proposal"
			echo "  --type TYPE     prompt_tightening|risk_rule_change|portfolio_constraint_revision"
			echo "  --fallback-type TYPE  Mutation type used for retry when no improvement (default: risk_rule_change)"
			echo "  --no-fallback   Disable automatic retry"
			echo "  --prepare-window  Run backtest-window before mutation cycle"
			echo "  --window-start YYYY-MM-DD  Start date for prepare-window"
			echo "  --window-end YYYY-MM-DD    End date for prepare-window"
			echo "  --replay-data PATH  Override ATLAS_REPLAY_DATA_PATH for this run"
			echo "  --no-auto-pivot  Do not run an additional auto-agent cycle when fallback is skipped"
			echo "  --min-sample-for-rank N  Minimum historical sample count required for weighted ranking (default: 2)"
			exit 0
			;;
		*)
			echo "Unknown option: $1"
			exit 1
			;;
	esac
done

if [[ ! "$MIN_SAMPLE_FOR_RANK" =~ ^[0-9]+$ ]]; then
	echo "Invalid --min-sample-for-rank value: $MIN_SAMPLE_FOR_RANK"
	exit 1
fi

if [[ -n "$REPLAY_DATA_PATH" ]]; then
	export ATLAS_REPLAY_DATA_PATH="$REPLAY_DATA_PATH"
elif [[ -z "${ATLAS_REPLAY_DATA_PATH:-}" ]]; then
	if [[ -f "data/replay/tw_combined_for_judge.csv" ]]; then
		export ATLAS_REPLAY_DATA_PATH="data/replay/tw_combined_for_judge.csv"
	elif [[ -f "samples/replay/twse_stock_day_all_sample.csv" ]]; then
		export ATLAS_REPLAY_DATA_PATH="samples/replay/twse_stock_day_all_sample.csv"
	fi
fi

run_cycle() {
	local cycle_label="$1"
	local cycle_type="$2"
	local judge_log
	local latest_brief
	local -a propose_cmd
	local cycle_observed_count
	local required_for_cycle

	echo "[2/5] ${cycle_label}: generate mutation brief (auto)"
	propose_cmd=(./scripts/openclaw/propose-mutation.sh --auto)
	if [[ -n "$AGENT" ]]; then
		propose_cmd+=(--agent "$AGENT")
	fi
	if [[ -n "$cycle_type" ]]; then
		propose_cmd+=(--type "$cycle_type")
	fi
	"${propose_cmd[@]}"

	latest_brief=""
	if [[ -d "data/state/mutation-briefs" ]]; then
		latest_brief=$(ls -t data/state/mutation-briefs/*.json 2>/dev/null | head -1 || true)
	fi
	if [[ -z "$latest_brief" ]]; then
		echo "No mutation brief found after proposal step."
		exit 1
	fi

	cycle_observed_count=$(jq -r '.observed_window_count // 0' "$latest_brief" 2>/dev/null)
	if [[ ! "$cycle_observed_count" =~ ^[0-9]+$ ]]; then
		cycle_observed_count=0
	fi

	required_for_cycle=1
	case "$cycle_type" in
		risk_rule_change)
			required_for_cycle=5
			;;
		portfolio_constraint_revision)
			required_for_cycle=6
			;;
		*)
			required_for_cycle=1
			;;
	esac

	if [[ "$cycle_observed_count" -lt "$required_for_cycle" ]]; then
		echo "[skip] ${cycle_label}: observed_window_count=$cycle_observed_count is below required $required_for_cycle for $cycle_type"
		echo "[skip] ${cycle_label}: skipping execute/judge for low-confidence cycle"
		LAST_JUDGE_LOG=""
		return 0
	fi

	echo "[3/5] ${cycle_label}: execute generated mutation brief (auto)"
	echo "Using brief: $latest_brief"
	./scripts/openclaw/execute-next.sh --auto --brief "$latest_brief"

	echo "[4/5] ${cycle_label}: judge latest experiment (auto, json)"
	judge_log=$(mktemp)
	./scripts/openclaw/judge-latest.sh --auto --json | tee "$judge_log"
	LAST_JUDGE_LOG="$judge_log"
}

is_mutation_futile() {
	local agent_id="$1"
	local mutation_type="$2"
	local window_id="$3"
	local threshold=3
	local total=0
	local non_improving=0
	local f

	if [[ ! -d "data/state/experiments" ]]; then
		return 1
	fi

	for f in $(ls -t data/state/experiments/*.json 2>/dev/null); do
		local agent mtype win baseline candidate
		agent=$(jq -r '.experiment.target_agent_id // .Experiment.TargetAgentID // empty' "$f" 2>/dev/null)
		mtype=$(jq -r '.experiment.mutation_type // .Experiment.MutationType // .brief.mutation_type // .Brief.mutation_type // empty' "$f" 2>/dev/null)
		win=$(jq -r '.brief.window_id // .Brief.window_id // empty' "$f" 2>/dev/null)
		if [[ "$agent" != "$agent_id" || "$mtype" != "$mutation_type" ]]; then
			continue
		fi
		if [[ -n "$window_id" && "$win" != "$window_id" ]]; then
			continue
		fi

		baseline=$(jq -r '.experiment.baseline_value // .Experiment.BaselineValue // empty' "$f" 2>/dev/null)
		candidate=$(jq -r '.experiment.candidate_value // .Experiment.CandidateValue // empty' "$f" 2>/dev/null)
		if [[ -z "$baseline" || -z "$candidate" || "$baseline" == "null" || "$candidate" == "null" ]]; then
			continue
		fi

		total=$((total + 1))
		if awk "BEGIN{exit !($candidate <= $baseline)}"; then
			non_improving=$((non_improving + 1))
		fi
		if [[ $total -ge $threshold ]]; then
			break
		fi
	done

	if [[ $total -ge $threshold && $non_improving -eq $total ]]; then
		return 0
	fi
	return 1
}

is_constraint_fallback_futile() {
	is_mutation_futile "$1" "$2" "$3"
}

echo "[1/5] System status"
if [[ -n "${ATLAS_REPLAY_DATA_PATH:-}" ]]; then
	echo "Replay data path: ${ATLAS_REPLAY_DATA_PATH}"
fi
./scripts/openclaw/status.sh

if [[ "$PREPARE_WINDOW" == true ]]; then
	if [[ -z "$WINDOW_END" ]]; then
		WINDOW_END=$(date +%Y-%m-%d)
	fi
	if [[ -z "$WINDOW_START" ]]; then
		WINDOW_START=$(date -v-30d +%Y-%m-%d)
	fi

	echo "[prep] Build replay window evidence: $WINDOW_START -> $WINDOW_END"
	if ! go run ./cmd/backtest-window -start "$WINDOW_START" -end "$WINDOW_END"; then
		echo "[prep] Warning: backtest-window failed for replay data path ${ATLAS_REPLAY_DATA_PATH:-<unset>}"
		echo "[prep] Continue without prewarm; use --replay-data <csv-path> to override"
	fi
fi

PRIMARY_TYPE="${MUTATION_TYPE:-prompt_tightening}"
WINDOW_ID_FOR_GUARDS="$(build_window_id "$WINDOW_START" "$WINDOW_END")"
if [[ -z "$WINDOW_ID_FOR_GUARDS" && -d "data/state/mutation-briefs" ]]; then
	latest_primary_brief=$(ls -t data/state/mutation-briefs/*.json 2>/dev/null | head -1 || true)
	if [[ -n "$latest_primary_brief" ]]; then
		WINDOW_ID_FOR_GUARDS=$(jq -r '.window_id // empty' "$latest_primary_brief" 2>/dev/null)
	fi
fi

SKIP_PRIMARY_DUE_FUTILITY=false
if [[ -n "$AGENT" ]] && is_mutation_futile "$AGENT" "$PRIMARY_TYPE" "$WINDOW_ID_FOR_GUARDS"; then
	echo ""
	echo "Primary mutation marked futile: recent ${PRIMARY_TYPE} runs for ${AGENT} in this window were all non-improving."
	if [[ "$AUTO_PIVOT_ON_SKIP" == true ]]; then
		print_rankable_mutation_candidates "$PRIMARY_TYPE" "$AGENT" "$WINDOW_ID_FOR_GUARDS"
		alt_type=$(choose_alternative_mutation_type "$PRIMARY_TYPE" "$AGENT" "$WINDOW_ID_FOR_GUARDS")
		if [[ -n "$alt_type" ]]; then
			alt_stats=$(mutation_recent_stats "$AGENT" "$alt_type" "$WINDOW_ID_FOR_GUARDS")
			if [[ -n "$alt_stats" ]]; then
				read -r alt_avg alt_n alt_weighted <<<"$alt_stats"
				echo "[pivot] Switching primary mutation type to ${alt_type} (recent avg delta: ${alt_avg}, n=${alt_n}, weighted=${alt_weighted})."
			else
				echo "[pivot] Switching primary mutation type to ${alt_type}."
			fi
			PRIMARY_TYPE="$alt_type"
		else
			echo "[pivot] No non-futile mutation type found for ${AGENT} in this window; skipping primary cycle."
			SKIP_PRIMARY_DUE_FUTILITY=true
		fi
	else
		echo "Primary cycle skipped due to futility guard (auto-pivot disabled)."
		SKIP_PRIMARY_DUE_FUTILITY=true
	fi
fi

primary_log=""
if [[ "$SKIP_PRIMARY_DUE_FUTILITY" != true ]]; then
	run_cycle "Primary cycle (${PRIMARY_TYPE})" "$PRIMARY_TYPE"
	primary_log="$LAST_JUDGE_LOG"
fi

if [[ -n "$primary_log" && "$ENABLE_FALLBACK" == true ]] && [[ "$FALLBACK_TYPE" != "$PRIMARY_TYPE" ]]; then
	fallback_skipped_futility=false
	latest_window_for_fallback=""
	if [[ -d "data/state/mutation-briefs" ]]; then
		latest_fallback_brief=$(ls -t data/state/mutation-briefs/*.json 2>/dev/null | head -1 || true)
		if [[ -n "$latest_fallback_brief" ]]; then
			latest_window_for_fallback=$(jq -r '.window_id // empty' "$latest_fallback_brief" 2>/dev/null)
		fi
	fi

	if [[ -n "$AGENT" ]] && is_constraint_fallback_futile "$AGENT" "$FALLBACK_TYPE" "$latest_window_for_fallback"; then
		echo ""
		echo "Fallback skipped: recent ${FALLBACK_TYPE} runs for ${AGENT} in this window were all non-improving."
		fallback_skipped_futility=true
		if [[ "$AUTO_PIVOT_ON_SKIP" == true ]]; then
			echo "[pivot] Running one additional primary cycle using auto-selected weakest agent."
			original_agent="$AGENT"
			AGENT=""
			run_cycle "Pivot cycle (${PRIMARY_TYPE}, auto-agent)" "$PRIMARY_TYPE"
			if [[ -n "$LAST_JUDGE_LOG" ]]; then
				pivot_log="$LAST_JUDGE_LOG"
				rm -f "$pivot_log"
			fi
			AGENT="$original_agent"
		else
			echo "Pivot suggestion: keep prompt_tightening iteration or switch target agent."
		fi
	fi

	if [[ "$fallback_skipped_futility" != true ]]; then
		observed_count=$(jq -r '.brief.observed_window_count // .Brief.ObservedWindowCount // 0' "$primary_log" 2>/dev/null | head -1)
		if [[ -z "$observed_count" ]]; then
			observed_count=0
		fi

		required_for_fallback=1
		case "$FALLBACK_TYPE" in
			risk_rule_change)
				required_for_fallback=5
				;;
			portfolio_constraint_revision)
				required_for_fallback=6
				;;
			*)
				required_for_fallback=1
				;;
		esac

		if grep -iq '"status": "rejected"' "$primary_log" && grep -iq 'candidate did not improve over baseline' "$primary_log"; then
			if [[ "$observed_count" -lt "$required_for_fallback" ]]; then
				echo ""
				echo "Fallback skipped: observed_window_count=$observed_count is too low for $FALLBACK_TYPE (need >= $required_for_fallback)."
				echo "Use prompt_tightening or gather more replay windows before risk/constraint mutations."
			else
			echo ""
			echo "Primary cycle rejected due to no improvement; running fallback mutation type: $FALLBACK_TYPE"
			run_cycle "Fallback cycle (${FALLBACK_TYPE})" "$FALLBACK_TYPE"
			if [[ -n "$LAST_JUDGE_LOG" ]]; then
				fallback_log="$LAST_JUDGE_LOG"
				rm -f "$fallback_log"
			fi
		fi
		fi
	fi
fi

if [[ -n "$primary_log" ]]; then
	rm -f "$primary_log"
fi

echo "[5/5] Decision reminder"
echo "Use decide.sh to promote or revert after reviewing guard/audit evidence."
echo "  Promote: ./scripts/openclaw/decide.sh --promote <EXP-ID> --reason \"<reason>\""
echo "  Revert : ./scripts/openclaw/decide.sh --revert --reason \"<reason>\""
