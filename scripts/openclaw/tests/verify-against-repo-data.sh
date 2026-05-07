#!/usr/bin/env bash
#
# Regression verification: updated OpenClaw scripts against real repo data
# Ensures dual-read patterns still work with existing PascalCase artifacts.
#

set -euo pipefail

FAILED=0
fail() {
	echo "FAIL: $1"
	FAILED=1
}
pass() {
	echo "PASS: $1"
}

echo "========================================"
echo "Spot-check: updated patterns against"
echo "real PascalCase repo data"
echo "========================================"

# Pick a real experiment result file
REAL_EXP=$(ls -t data/state/experiments/*.json 2>/dev/null | head -1)
if [[ -z "$REAL_EXP" ]]; then
	echo "SKIP: no experiment result files found"
	exit 0
fi

# Test experiment result reads against real file
real_status=$(jq -r '.experiment.status // .Experiment.Status // empty' "$REAL_EXP" 2>/dev/null)
real_agent=$(jq -r '.experiment.target_agent_id // .Experiment.TargetAgentID // empty' "$REAL_EXP" 2>/dev/null)
real_id=$(jq -r '.experiment.id // .Experiment.ID // empty' "$REAL_EXP" 2>/dev/null)
real_baseline=$(jq -r '.experiment.baseline_value // .Experiment.BaselineValue // empty' "$REAL_EXP" 2>/dev/null)
real_candidate=$(jq -r '.experiment.candidate_value // .Experiment.CandidateValue // empty' "$REAL_EXP" 2>/dev/null)
real_mutation=$(jq -r '.experiment.mutation_type // .Experiment.MutationType // empty' "$REAL_EXP" 2>/dev/null)

if [[ -n "$real_status" ]]; then pass "real exp status ($real_status)"; else fail "real exp status empty"; fi
if [[ -n "$real_agent" ]]; then pass "real exp agent ($real_agent)"; else fail "real exp agent empty"; fi
if [[ -n "$real_id" ]]; then pass "real exp id ($real_id)"; else fail "real exp id empty"; fi
if [[ -n "$real_baseline" ]]; then pass "real exp baseline ($real_baseline)"; else fail "real exp baseline empty"; fi
if [[ -n "$real_candidate" ]]; then pass "real exp candidate ($real_candidate)"; else fail "real exp candidate empty"; fi
if [[ -n "$real_mutation" ]]; then pass "real exp mutation ($real_mutation)"; else fail "real exp mutation empty"; fi

# Test baseline policy read
if [[ -f "data/state/baseline_policy.json" ]]; then
	real_version=$(jq -r '.version // .Version // empty' data/state/baseline_policy.json 2>/dev/null)
	if [[ -n "$real_version" ]]; then pass "real baseline version ($real_version)"; else fail "real baseline version empty"; fi
fi

# Test experiments.jsonl grep -i counts
if [[ -f "data/state/experiments.jsonl" ]]; then
	planned=$(grep -ic '"status":"planned"' data/state/experiments.jsonl 2>/dev/null || echo "0")
	planned=$(echo "$planned" | tr -d '\n\r')
	running=$(grep -ic '"status":"running"' data/state/experiments.jsonl 2>/dev/null || echo "0")
	running=$(echo "$running" | tr -d '\n\r')
	accepted=$(grep -ic '"status":"accepted"' data/state/experiments.jsonl 2>/dev/null || echo "0")
	accepted=$(echo "$accepted" | tr -d '\n\r')
	rejected=$(grep -ic '"status":"rejected"' data/state/experiments.jsonl 2>/dev/null || echo "0")
	rejected=$(echo "$rejected" | tr -d '\n\r')
	total=$(wc -l < data/state/experiments.jsonl 2>/dev/null || echo "0")
	total=$(echo "$total" | tr -d ' \n\r\t')
	sum=$((planned + running + accepted + rejected))
	if [[ "$sum" -le "$total" ]]; then
		pass "jsonl status counts consistent (sum=$sum <= total=$total)"
	else
		fail "jsonl status counts inconsistent (sum=$sum > total=$total)"
	fi

	# Test per-line jq read on last line
	last_line=$(tail -1 data/state/experiments.jsonl)
	last_id=$(echo "$last_line" | jq -r '.id // .ID // empty')
	last_status=$(echo "$last_line" | jq -r '.status // .Status // empty')
	if [[ -n "$last_id" ]]; then pass "jsonl last line id ($last_id)"; else fail "jsonl last line id empty"; fi
	if [[ -n "$last_status" ]]; then pass "jsonl last line status ($last_status)"; else fail "jsonl last line status empty"; fi
fi

echo ""
echo "========================================"
if [[ "$FAILED" -eq 0 ]]; then
	echo "All repo data checks PASSED"
	exit 0
else
	echo "Some repo data checks FAILED"
	exit 1
fi
