#!/bin/bash
#
# OpenClaw Experiment Executor
# 執行下一個準備好的實驗
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

BRIEF_PATH=""
WINDOW_DAYS=10
AUTO_MODE=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --brief)
            BRIEF_PATH="$2"
            shift 2
            ;;
        --window)
            WINDOW_DAYS="$2"
            shift 2
            ;;
        --auto)
            AUTO_MODE=true
            shift
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Execute the next planned experiment or specified mutation brief."
            echo ""
            echo "Options:"
            echo "  --brief PATH     Path to mutation brief JSON file"
            echo "  --window DAYS    Backtest window in days (default: 10)"
            echo "  --auto           Skip confirmation prompts"
            echo "  --help           Show this help"
            echo ""
            echo "Examples:"
            echo "  $0                               # Execute next planned experiment"
            echo "  $0 --brief path/to/brief.json   # Execute specific brief"
            echo "  $0 --window 20                  # Use 20-day backtest window"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

print_header() {
    echo -e "\n${BLUE}╔════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║     OpenClaw Experiment Executor               ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════╝${NC}\n"
}

# Find next planned experiment
find_next_experiment() {
    if [ -f "data/state/experiments.jsonl" ]; then
        local next_exp=$(grep '"status":"planned"' data/state/experiments.jsonl | tail -1)
        if [ -n "$next_exp" ]; then
            local exp_id=$(echo "$next_exp" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
            echo "$exp_id"
            return 0
        fi
    fi
    return 1
}

# Find mutation brief
find_mutation_brief() {
    if [ -n "$BRIEF_PATH" ]; then
        if [ -f "$BRIEF_PATH" ]; then
            echo "$BRIEF_PATH"
            return 0
        else
            echo -e "${RED}Error: Brief file not found: $BRIEF_PATH${NC}"
            return 1
        fi
    fi
    
    # Look for latest brief in mutation-briefs directory
    if [ -d "data/state/mutation-briefs" ]; then
        local latest=$(ls -t data/state/mutation-briefs/*.json 2>/dev/null | head -1)
        if [ -n "$latest" ]; then
            echo "$latest"
            return 0
        fi
    fi
    
    return 1
}

# Display experiment info
display_experiment_info() {
    local exp_id="$1"
    
    echo -e "${CYAN}Experiment Information:${NC}"
    echo "  ID: $exp_id"
    
    if [ -f "data/state/experiments.jsonl" ]; then
        local exp_data=$(grep "\"id\":\"$exp_id\"" data/state/experiments.jsonl | head -1)
        if [ -n "$exp_data" ]; then
            local agent=$(echo "$exp_data" | grep -o '"target_agent_id":"[^"]*"' | cut -d'"' -f4)
            local skill=$(echo "$exp_data" | grep -o '"skill":"[^"]*"' | cut -d'"' -f4)
            local mutation=$(echo "$exp_data" | grep -o '"mutation_type":"[^"]*"' | cut -d'"' -f4)
            
            echo "  Agent: $agent"
            echo "  Skill: $skill"
            echo "  Mutation Type: ${mutation:-prompt_tightening}"
        fi
    fi
    echo ""
}

# Run experiment
execute_experiment() {
    local exp_id="$1"
    local brief_path="$2"

    if [ -n "$exp_id" ]; then
        echo -e "${CYAN}Running experiment: $exp_id${NC}"
    else
        echo -e "${CYAN}Running experiment from explicit brief${NC}"
    fi
    echo ""
    
    # Build execute command
    local cmd="go run ./cmd/run-experiment"
    if [ -n "$brief_path" ]; then
        cmd="$cmd --brief $brief_path"
    fi
    
    if [ -n "$exp_id" ]; then
        # Find experiment file
        local exp_file="data/state/experiments/${exp_id}.json"
        if [ ! -f "$exp_file" ]; then
            exp_file=$(find data/state/experiments -name "*${exp_id}*" -type f 2>/dev/null | head -1)
        fi
        
        if [ -f "$exp_file" ]; then
            echo "Found experiment file: $exp_file"
        else
            echo -e "${YELLOW}Warning: Cannot find experiment file for $exp_id, fallback to brief execution.${NC}"
        fi
    fi
    
    echo "Command: $cmd"
    echo ""
    
    # Execute
    eval "$cmd"
    
    if [ $? -eq 0 ]; then
        echo ""
        echo -e "${GREEN}✓ Experiment execution started${NC}"
        echo ""
        echo "Next steps:"
        echo "  1. Run replay judge to fill baseline/candidate metrics"
        echo "  2. Run: ./scripts/openclaw/judge-latest.sh --auto"
    else
        echo -e "${RED}✗ Experiment execution failed${NC}"
        return 1
    fi
}

# Check prerequisites
check_prerequisites() {
    local missing=()
    
    if [ ! -d "data/replay" ] || [ -z "$(ls -A data/replay 2>/dev/null)" ]; then
        missing+=("replay data")
    fi
    
    if [ ! -f "configs/agents.json" ]; then
        missing+=("agent configuration")
    fi
    
    if [ ${#missing[@]} -gt 0 ]; then
        echo -e "${RED}Error: Missing prerequisites:${NC}"
        printf '  - %s\n' "${missing[@]}"
        echo ""
        echo "Please ensure:"
        echo "  1. Replay data is imported: go run ./cmd/import-replay ..."
        echo "  2. Agent configuration exists: configs/agents.json"
        return 1
    fi
    
    return 0
}

# Interactive mode
interactive_mode() {
    echo ""
    echo "Execute Experiment"
    echo "=================="
    echo ""
    
    local next_exp=$(find_next_experiment)
    local brief=$(find_mutation_brief)
    
    if [ -n "$next_exp" ]; then
        echo "Found planned experiment: $next_exp"
        display_experiment_info "$next_exp"
        
        echo -n "Execute this experiment? [Y/n]: "
        read -r confirm
        if [[ ! "$confirm" =~ ^[Nn]$ ]]; then
            execute_experiment "$next_exp" "$brief"
        else
            echo "Cancelled."
        fi
    elif [ -n "$brief" ]; then
        echo "Found mutation brief: $brief"
        echo ""
        echo "Brief content preview:"
        head -20 "$brief"
        echo ""
        
        echo -n "Execute this brief? [Y/n]: "
        read -r confirm
        if [[ ! "$confirm" =~ ^[Nn]$ ]]; then
            execute_experiment "" "$brief"
        else
            echo "Cancelled."
        fi
    else
        echo -e "${YELLOW}No planned experiments or briefs found.${NC}"
        echo ""
        echo "To create a new experiment:"
        echo "  ./scripts/openclaw/propose-mutation.sh"
    fi
}

# Main
main() {
    print_header
    
    if ! check_prerequisites; then
        exit 1
    fi

    if [ -n "$BRIEF_PATH" ]; then
        execute_experiment "" "$BRIEF_PATH"
        exit 0
    fi
    
    if [ "$AUTO_MODE" = false ] && [ -z "$BRIEF_PATH" ]; then
        interactive_mode
    else
        # Auto mode or brief specified
        local target_exp=$(find_next_experiment)
        
        if [ -z "$target_exp" ]; then
            echo -e "${YELLOW}No planned experiments found.${NC}"
            
            local brief=$(find_mutation_brief)
            if [ -n "$brief" ]; then
                echo "Using mutation brief: $brief"
                execute_experiment "" "$brief"
            else
                echo "No mutation briefs found either."
                echo "Run: ./scripts/openclaw/propose-mutation.sh --auto"
                exit 1
            fi
        else
            echo "Executing: $target_exp"
            local brief=$(find_mutation_brief)
            if [ -z "$brief" ]; then
                echo -e "${RED}Error: No mutation brief found.${NC}"
                echo "Run: ./scripts/openclaw/propose-mutation.sh --auto"
                exit 1
            fi
            execute_experiment "$target_exp" "$brief"
        fi
    fi
}

main "$@"
