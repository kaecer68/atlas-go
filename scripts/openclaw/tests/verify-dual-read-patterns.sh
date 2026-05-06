#!/usr/bin/env bash
#
# Regression verification for snake_case / PascalCase dual-read compatibility
# Tests the jq/grep patterns used by OpenClaw scripts against mock data.
#

set -euo pipefail

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

FAILED=0
fail() {
	echo "FAIL: $1"
	FAILED=1
}
pass() {
	echo "PASS: $1"
}

# ---------------------------------------------------------------------------
# Mock data: snake_case experiment result
# ---------------------------------------------------------------------------
cat > "$TMPDIR/exp-snake.json" <<'EOF'
{
  "experiment": {
    "id": "exp-snake-001",
    "target_agent_id": "agent-01",
    "status": "accepted",
    "baseline_value": 0.5,
    "candidate_value": 0.7,
    "mutation_type": "prompt_tightening",
    "evaluation_mode": "full"
  },
  "judge_checks": [
    "improve_sharpe_like",
    "no_material_drawdown_degradation"
  ],
  "brief": {
    "window_id": "window-20260101",
    "mutation_type": "prompt_tightening"
  },
  "baseline_observations": 10,
  "candidate_observations": 12
}
EOF

# ---------------------------------------------------------------------------
# Mock data: PascalCase experiment result (legacy)
# ---------------------------------------------------------------------------
cat > "$TMPDIR/exp-pascal.json" <<'EOF'
{
  "Experiment": {
    "ID": "exp-pascal-001",
    "TargetAgentID": "agent-01",
    "Status": "accepted",
    "BaselineValue": 0.5,
    "CandidateValue": 0.7,
    "MutationType": "prompt_tightening",
    "EvaluationMode": "full"
  },
  "JudgeChecks": [
    "improve_sharpe_like",
    "no_material_drawdown_degradation"
  ],
  "Brief": {
    "window_id": "window-20260101",
    "mutation_type": "prompt_tightening"
  },
  "BaselineObservations": 10,
  "CandidateObservations": 12
}
EOF

# ---------------------------------------------------------------------------
# Mock data: mixed experiments.jsonl
# ---------------------------------------------------------------------------
cat > "$TMPDIR/experiments.jsonl" <<'EOF'
{"id":"exp-001","target_agent_id":"agent-01","status":"planned","mutation_type":"prompt_tightening"}
{"ID":"exp-002","TargetAgentID":"agent-02","Status":"running","MutationType":"risk_rule_change"}
{"id":"exp-003","target_agent_id":"agent-03","status":"accepted","mutation_type":"portfolio_constraint_revision"}
{"ID":"exp-004","TargetAgentID":"agent-04","Status":"rejected","MutationType":"prompt_tightening"}
EOF

# ---------------------------------------------------------------------------
# Mock data: baseline policies
# ---------------------------------------------------------------------------
cat > "$TMPDIR/baseline-snake.json" <<'EOF'
{"version": 5, "constraints": {"max_position_weight": 0.25}}
EOF
cat > "$TMPDIR/baseline-pascal.json" <<'EOF'
{"Version": 19, "Constraints": {"MaxPositionWeight": 0.25}}
EOF

echo "========================================"
echo "Phase A: OLD patterns against snake_case"
echo "(must show empty/missing results)"
echo "========================================"

# Old PascalCase-only jq patterns against snake_case data
old_status=$(jq -r '.Experiment.Status // empty' "$TMPDIR/exp-snake.json" 2>/dev/null || true)
old_agent=$(jq -r '.Experiment.TargetAgentID // empty' "$TMPDIR/exp-snake.json" 2>/dev/null || true)
old_baseline=$(jq -r '.Experiment.BaselineValue // empty' "$TMPDIR/exp-snake.json" 2>/dev/null || true)
old_candidate=$(jq -r '.Experiment.CandidateValue // empty' "$TMPDIR/exp-snake.json" 2>/dev/null || true)
old_id=$(jq -r '.Experiment.ID // empty' "$TMPDIR/exp-snake.json" 2>/dev/null || true)

if [[ -z "$old_status" && -z "$old_agent" && -z "$old_baseline" && -z "$old_candidate" && -z "$old_id" ]]; then
	pass "Old PascalCase-only jq patterns return empty for snake_case data"
else
	fail "Old PascalCase-only jq patterns should return empty for snake_case data (got status='$old_status' agent='$old_agent')"
fi

# Old grep patterns against snake_case experiments.jsonl
old_planned=$(grep -c '"Status":"planned"' "$TMPDIR/experiments.jsonl" 2>/dev/null) || old_planned="0"
if [[ "$old_planned" == "0" ]]; then
	pass "Old grep '"Status":"planned"' finds 0 in snake_case JSONL"
else
	fail "Old grep '"Status":"planned"' should find 0 in snake_case JSONL (got '$old_planned')"
fi

old_version=$(grep -o '"Version": [0-9]*' "$TMPDIR/baseline-snake.json" 2>/dev/null | head -1 | grep -o '[0-9]*' || true)
if [[ -z "$old_version" ]]; then
	pass "Old grep '"Version":' returns empty for snake_case baseline"
else
	fail "Old grep '"Version":' should return empty for snake_case baseline (got '$old_version')"
fi

echo ""
echo "========================================"
echo "Phase B: NEW dual-read patterns"
echo "========================================"

# Helper to test jq dual-read
test_jq_dual() {
	local name="$1"
	local file="$2"
	local query="$3"
	local expected="$4"
	local result
	result=$(jq -r "$query" "$file" 2>/dev/null)
	if [[ "$result" == "$expected" ]]; then
		pass "$name"
	else
		fail "$name (expected '$expected', got '$result')"
	fi
}

# Experiment result: snake_case
test_jq_dual "snake exp status"     "$TMPDIR/exp-snake.json" '.experiment.status // .Experiment.Status // empty' "accepted"
test_jq_dual "snake exp agent"      "$TMPDIR/exp-snake.json" '.experiment.target_agent_id // .Experiment.TargetAgentID // empty' "agent-01"
test_jq_dual "snake exp baseline"   "$TMPDIR/exp-snake.json" '.experiment.baseline_value // .Experiment.BaselineValue // empty' "0.5"
test_jq_dual "snake exp candidate"  "$TMPDIR/exp-snake.json" '.experiment.candidate_value // .Experiment.CandidateValue // empty' "0.7"
test_jq_dual "snake exp mutation"   "$TMPDIR/exp-snake.json" '.experiment.mutation_type // .Experiment.MutationType // empty' "prompt_tightening"
test_jq_dual "snake exp id"         "$TMPDIR/exp-snake.json" '.experiment.id // .Experiment.ID // empty' "exp-snake-001"
test_jq_dual "snake exp eval_mode"  "$TMPDIR/exp-snake.json" '.experiment.evaluation_mode // .Experiment.EvaluationMode // empty' "full"
test_jq_dual "snake brief window"   "$TMPDIR/exp-snake.json" '.brief.window_id // .Brief.window_id // empty' "window-20260101"
test_jq_dual "snake baseline_obs"   "$TMPDIR/exp-snake.json" '.baseline_observations // .BaselineObservations // 0' "10"
test_jq_dual "snake candidate_obs"  "$TMPDIR/exp-snake.json" '.candidate_observations // .CandidateObservations // 0' "12"
test_jq_dual "snake judge_check 1"  "$TMPDIR/exp-snake.json" '(.judge_checks // .JudgeChecks // [])[0] // empty' "improve_sharpe_like"
test_jq_dual "snake judge_check 2"  "$TMPDIR/exp-snake.json" '(.judge_checks // .JudgeChecks // [])[1] // empty' "no_material_drawdown_degradation"

# Experiment result: PascalCase legacy
test_jq_dual "pascal exp status"     "$TMPDIR/exp-pascal.json" '.experiment.status // .Experiment.Status // empty' "accepted"
test_jq_dual "pascal exp agent"      "$TMPDIR/exp-pascal.json" '.experiment.target_agent_id // .Experiment.TargetAgentID // empty' "agent-01"
test_jq_dual "pascal exp baseline"   "$TMPDIR/exp-pascal.json" '.experiment.baseline_value // .Experiment.BaselineValue // empty' "0.5"
test_jq_dual "pascal exp candidate"  "$TMPDIR/exp-pascal.json" '.experiment.candidate_value // .Experiment.CandidateValue // empty' "0.7"
test_jq_dual "pascal exp mutation"   "$TMPDIR/exp-pascal.json" '.experiment.mutation_type // .Experiment.MutationType // empty' "prompt_tightening"
test_jq_dual "pascal exp id"         "$TMPDIR/exp-pascal.json" '.experiment.id // .Experiment.ID // empty' "exp-pascal-001"
test_jq_dual "pascal exp eval_mode"  "$TMPDIR/exp-pascal.json" '.experiment.evaluation_mode // .Experiment.EvaluationMode // empty' "full"
test_jq_dual "pascal brief window"   "$TMPDIR/exp-pascal.json" '.brief.window_id // .Brief.window_id // empty' "window-20260101"
test_jq_dual "pascal baseline_obs"   "$TMPDIR/exp-pascal.json" '.baseline_observations // .BaselineObservations // 0' "10"
test_jq_dual "pascal candidate_obs"  "$TMPDIR/exp-pascal.json" '.candidate_observations // .CandidateObservations // 0' "12"
test_jq_dual "pascal judge_check 1"  "$TMPDIR/exp-pascal.json" '(.judge_checks // .JudgeChecks // [])[0] // empty' "improve_sharpe_like"
test_jq_dual "pascal judge_check 2"  "$TMPDIR/exp-pascal.json" '(.judge_checks // .JudgeChecks // [])[1] // empty' "no_material_drawdown_degradation"

# Baseline policy
test_jq_dual "snake baseline version"  "$TMPDIR/baseline-snake.json"  '.version // .Version // empty' "5"
test_jq_dual "pascal baseline version" "$TMPDIR/baseline-pascal.json" '.version // .Version // empty' "19"

# experiments.jsonl: per-line jq reads
line1_id=$(sed -n '1p' "$TMPDIR/experiments.jsonl" | jq -r '.id // .ID // empty')
line1_agent=$(sed -n '1p' "$TMPDIR/experiments.jsonl" | jq -r '.target_agent_id // .TargetAgentID // empty')
line1_status=$(sed -n '1p' "$TMPDIR/experiments.jsonl" | jq -r '.status // .Status // empty')
if [[ "$line1_id" == "exp-001" ]]; then pass "jsonl line1 id"; else fail "jsonl line1 id (expected 'exp-001', got '$line1_id')"; fi
if [[ "$line1_agent" == "agent-01" ]]; then pass "jsonl line1 agent"; else fail "jsonl line1 agent (expected 'agent-01', got '$line1_agent')"; fi
if [[ "$line1_status" == "planned" ]]; then pass "jsonl line1 status"; else fail "jsonl line1 status (expected 'planned', got '$line1_status')"; fi

line2_id=$(sed -n '2p' "$TMPDIR/experiments.jsonl" | jq -r '.id // .ID // empty')
line2_agent=$(sed -n '2p' "$TMPDIR/experiments.jsonl" | jq -r '.target_agent_id // .TargetAgentID // empty')
line2_status=$(sed -n '2p' "$TMPDIR/experiments.jsonl" | jq -r '.status // .Status // empty')
if [[ "$line2_id" == "exp-002" ]]; then pass "jsonl line2 id"; else fail "jsonl line2 id (expected 'exp-002', got '$line2_id')"; fi
if [[ "$line2_agent" == "agent-02" ]]; then pass "jsonl line2 agent"; else fail "jsonl line2 agent (expected 'agent-02', got '$line2_agent')"; fi
if [[ "$line2_status" == "running" ]]; then pass "jsonl line2 status"; else fail "jsonl line2 status (expected 'running', got '$line2_status')"; fi

# experiments.jsonl: grep -i status counts
test_grep_count() {
	local name="$1"
	local file="$2"
	local pattern="$3"
	local expected="$4"
	local result
	result=$(grep -ic "$pattern" "$file" 2>/dev/null || echo "0")
	if [[ "$result" == "$expected" ]]; then
		pass "$name"
	else
		fail "$name (expected '$expected', got '$result')"
	fi
}

test_grep_count "grep -i planned count" "$TMPDIR/experiments.jsonl" '"status":"planned"' "1"
test_grep_count "grep -i running count" "$TMPDIR/experiments.jsonl" '"status":"running"' "1"
test_grep_count "grep -i accepted count" "$TMPDIR/experiments.jsonl" '"status":"accepted"' "1"
test_grep_count "grep -i rejected count" "$TMPDIR/experiments.jsonl" '"status":"rejected"' "1"

echo ""
echo "========================================"
if [[ "$FAILED" -eq 0 ]]; then
	echo "All checks PASSED"
	exit 0
else
	echo "Some checks FAILED"
	exit 1
fi
