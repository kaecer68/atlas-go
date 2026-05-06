#!/bin/bash
#
# OpenClaw Experiment Judge
# 判斷最新完成的實驗並提供決策建議
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

cd "$PROJECT_ROOT"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

FORCE_MODE=false
JSON_MODE=false
AUTO_MODE=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --force)
            FORCE_MODE=true
            shift
            ;;
        --json)
            JSON_MODE=true
            shift
            ;;
        --auto)
            AUTO_MODE=true
            shift
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Judge the latest completed experiment and provide recommendations."
            echo ""
            echo "Options:"
            echo "  --force    Force judge even if status is not 'running'"
            echo "  --json     Output results as JSON"
            echo "  --auto     Skip confirmation prompts"
            echo "  --help     Show this help"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

print_header() {
    if [ "$JSON_MODE" = false ]; then
        echo -e "\n${BLUE}╔════════════════════════════════════════════════╗${NC}"
        echo -e "${BLUE}║       OpenClaw Experiment Judge                ║${NC}"
        echo -e "${BLUE}╚════════════════════════════════════════════════╝${NC}\n"
    fi
}

# Find latest experiment
find_latest_experiment() {
    local status_filter="running"
    if [ "$FORCE_MODE" = true ]; then
        status_filter=""
    fi
    
    if [ -f "data/state/experiments.jsonl" ]; then
        local latest
        if [ -n "$status_filter" ]; then
            latest=$(grep -i '"status":"'$status_filter'"' data/state/experiments.jsonl | tail -1)
        else
            latest=$(tail -1 data/state/experiments.jsonl)
        fi

        if [ -n "$latest" ]; then
            local exp_id=$(echo "$latest" | jq -r '.id // .ID // empty')
            echo "$exp_id"
            return 0
        fi
    fi
    return 1
}

# Find result file
find_result_file() {
    local exp_id="$1"
    
    local result_file="data/state/experiments/${exp_id}.json"
    if [ -f "$result_file" ]; then
        echo "$result_file"
        return 0
    fi
    
    # Try pattern matching
    result_file=$(find data/state/experiments -name "*${exp_id}*" -type f 2>/dev/null | head -1)
    if [ -n "$result_file" ]; then
        echo "$result_file"
        return 0
    fi
    
    return 1
}

# Display experiment results
display_results() {
    local result_file="$1"
    
    if [ "$JSON_MODE" = true ]; then
        cat "$result_file"
        return
    fi
    
    echo -e "${CYAN}Experiment Results:${NC}"
    echo ""
    
    # Extract key data with dual-read for snake_case / PascalCase
    local exp_id
    exp_id=$(jq -r '.experiment.id // .Experiment.ID // empty' "$result_file" 2>/dev/null)
    local agent
    agent=$(jq -r '.experiment.target_agent_id // .Experiment.TargetAgentID // empty' "$result_file" 2>/dev/null)
    local status
    status=$(jq -r '.experiment.status // .Experiment.Status // empty' "$result_file" 2>/dev/null)
    local baseline
    baseline=$(jq -r '.experiment.baseline_value // .Experiment.BaselineValue // empty' "$result_file" 2>/dev/null)
    local candidate
    candidate=$(jq -r '.experiment.candidate_value // .Experiment.CandidateValue // empty' "$result_file" 2>/dev/null)
    local mutation
    mutation=$(jq -r '.experiment.mutation_type // .Experiment.MutationType // empty' "$result_file" 2>/dev/null)
    
    echo "  Experiment ID: $exp_id"
    echo "  Agent: $agent"
    echo "  Mutation Type: ${mutation:-prompt_tightening}"
    echo "  Status: $status"
    echo ""
    echo "Performance Comparison:"
    echo "  Baseline:  $baseline"
    echo "  Candidate: $candidate"
    
    if [ -n "$baseline" ] && [ -n "$candidate" ]; then
        local improvement=$(echo "$candidate - $baseline" | bc -l 2>/dev/null || echo "N/A")
        local pct_improve=$(echo "scale=4; (($candidate - $baseline) / $baseline) * 100" | bc -l 2>/dev/null || echo "N/A")
        
        echo "  Improvement: $improvement"
        if [ "$pct_improve" != "N/A" ]; then
            echo "  % Change: $pct_improve%"
        fi
    fi
    
    echo ""
    echo "Acceptance Checks:"
    local checks
    checks=$(jq -r '(.judge_checks // .JudgeChecks // [])[]?' "$result_file" 2>/dev/null)
    if [ -n "$checks" ]; then
        while IFS= read -r check; do
            [ -n "$check" ] || continue
            echo "  ✓ $check"
        done <<< "$checks"
    else
        echo "  (No checks recorded)"
    fi
}

# Provide recommendation
provide_recommendation() {
    local result_file="$1"
    
    local status
    status=$(jq -r '.experiment.status // .Experiment.Status // empty' "$result_file" 2>/dev/null)
    local baseline
    baseline=$(jq -r '.experiment.baseline_value // .Experiment.BaselineValue // empty' "$result_file" 2>/dev/null)
    local candidate
    candidate=$(jq -r '.experiment.candidate_value // .Experiment.CandidateValue // empty' "$result_file" 2>/dev/null)
    local exp_id
    exp_id=$(jq -r '.experiment.id // .Experiment.ID // empty' "$result_file" 2>/dev/null)
    
    local recommendation=""
    local reason=""
    
    if [ "$status" = "accepted" ]; then
        recommendation="--promote $exp_id"
        reason="Experiment passed all acceptance gates"
    elif [ "$status" = "rejected" ]; then
        recommendation="SKIP"
        reason="Experiment failed acceptance criteria"
    else
        # Need to judge
        if [ -n "$baseline" ] && [ -n "$candidate" ]; then
            local improvement=$(echo "$candidate > $baseline" | bc -l 2>/dev/null || echo "0")
            if [ "$improvement" = "1" ]; then
                recommendation="--promote $exp_id"
                reason="Candidate improved over baseline"
            else
                recommendation="SKIP"
                reason="Candidate did not improve over baseline"
            fi
        else
            recommendation="PENDING"
            reason="Insufficient data to make recommendation"
        fi
    fi
    
    if [ "$JSON_MODE" = true ]; then
        echo "{"
        echo "  \"recommendation\": \"$recommendation\","
        echo "  \"reason\": \"$reason\","
        echo "  \"status\": \"$status\","
        echo "  \"experiment_id\": \"$exp_id\""
        echo "}"
    else
        echo ""
        echo -e "${CYAN}Recommendation:${NC}"
        echo "  Action: $recommendation"
        echo "  Reason: $reason"
        echo ""
        echo "Next step:"
        if [ "$recommendation" != "SKIP" ] && [ "$recommendation" != "PENDING" ]; then
            echo "  ./scripts/openclaw/decide.sh $recommendation --reason \"$reason\""
        else
            echo "  (No action needed)"
        fi
    fi
}

# Execute judge
execute_judge() {
    local exp_id="$1"
    local result_file=$(find_result_file "$exp_id")
    
    if [ -z "$result_file" ]; then
        if [ "$JSON_MODE" = true ]; then
            echo '{"error": "Result file not found"}'
        else
            echo -e "${RED}Error: Cannot find result file for $exp_id${NC}"
        fi
        return 1
    fi
    
    local needs_replay_judge="false"
    local eval_mode
    eval_mode=$(jq -r '.experiment.evaluation_mode // .Experiment.EvaluationMode // empty' "$result_file" 2>/dev/null)
    local exp_status
    exp_status=$(jq -r '.experiment.status // .Experiment.Status // empty' "$result_file" 2>/dev/null)
    if [ "$eval_mode" = "policy_checked_pending_replay" ]; then
        needs_replay_judge="true"
    fi
    if [ "$exp_status" = "running" ]; then
        needs_replay_judge="true"
    fi

    if [ "$needs_replay_judge" = "true" ]; then
        echo -e "${CYAN}Running replay judge to compute baseline/candidate metrics...${NC}"
        go run ./cmd/judge-experiment -result "$result_file"
        result_file=$(find_result_file "$exp_id")
    fi

    display_results "$result_file"
    provide_recommendation "$result_file"
}

# Interactive mode
interactive_mode() {
    local latest_exp=$(find_latest_experiment)
    
    if [ -z "$latest_exp" ]; then
        echo -e "${YELLOW}No experiments found to judge.${NC}"
        echo ""
        echo "To create a new experiment:"
        echo "  ./scripts/openclaw/propose-mutation.sh"
        echo "  ./scripts/openclaw/execute-next.sh"
        return 1
    fi
    
    echo "Latest experiment: $latest_exp"
    echo ""
    
    echo -n "Judge this experiment? [Y/n]: "
    read -r confirm
    if [[ ! "$confirm" =~ ^[Nn]$ ]]; then
        execute_judge "$latest_exp"
    else
        echo "Cancelled."
    fi
}

# Main
main() {
    print_header
    
    local latest_exp=$(find_latest_experiment)
    
    if [ -z "$latest_exp" ]; then
        if [ "$JSON_MODE" = true ]; then
            echo '{"error": "No experiments found"}'
        else
            echo -e "${YELLOW}No experiments found to judge.${NC}"
            echo ""
            echo "Create a new experiment:"
            echo "  ./scripts/openclaw/propose-mutation.sh"
        fi
        exit 1
    fi
    
    if [ "$AUTO_MODE" = false ] && [ "$JSON_MODE" = false ]; then
        interactive_mode
    else
        execute_judge "$latest_exp"
    fi
}

main "$@"
