#!/bin/bash
#
# OpenClaw Decision Assistant
# 輔助 Promote/Revert 決策，提供建議和確認流程
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

# Decision parameters
ACTION=""  # promote, revert, skip
TARGET=""
REASON=""
DRY_RUN=false
AUTO_CONFIRM=false

print_header() {
    echo -e "\n${BLUE}╔════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║      OpenClaw Decision Assistant               ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════╝${NC}\n"
}

show_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Description:"
    echo "  Assists in making promote/revert decisions with safety checks."
    echo ""
    echo "Options:"
    echo "  --promote ID       Promote experiment result"
    echo "  --revert [N]       Revert to version N (default: previous)"
    echo "  --reason TEXT      Required: reason for decision"
    echo "  --dry-run          Preview decision without executing"
    echo "  --yes              Auto-confirm (skip interactive prompt)"
    echo "  --help             Show this help"
    echo ""
    echo "Examples:"
    echo "  $0 --promote exp-001 --reason \"Improved Sharpe by 5%\""
    echo "  $0 --revert 2 --reason \"Unexpected drawdown increase\""
    echo "  $0 --promote exp-001 --dry-run  # Preview only"
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --promote)
            ACTION="promote"
            TARGET="$2"
            shift 2
            ;;
        --revert)
            ACTION="revert"
            TARGET="${2:-}"
            shift
            [ -n "$TARGET" ] && shift
            ;;
        --reason)
            REASON="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --yes)
            AUTO_CONFIRM=true
            shift
            ;;
        --help)
            show_usage
            exit 0
            ;;
        *)
            echo -e "${RED}Error: Unknown option $1${NC}"
            show_usage
            exit 1
            ;;
    esac
done

# Validate inputs
validate_inputs() {
    if [ -z "$ACTION" ]; then
        echo -e "${RED}Error: Must specify --promote or --revert${NC}"
        show_usage
        exit 1
    fi
    
    if [ -z "$REASON" ]; then
        echo -e "${RED}Error: --reason is required${NC}"
        echo "Example: --reason 'Improved Sharpe ratio and reduced drawdown'"
        exit 1
    fi
    
    if [ "$ACTION" = "promote" ] && [ -z "$TARGET" ]; then
        echo -e "${RED}Error: --promote requires an experiment ID${NC}"
        exit 1
    fi
}

# Find latest experiment result
find_latest_experiment() {
    local exp_id="$1"
    local result_file="data/state/experiments/${exp_id}.json"
    
    if [ -f "$result_file" ]; then
        echo "$result_file"
        return 0
    fi
    
    # Try to find by pattern
    local found=$(find data/state/experiments -name "*${exp_id}*.json" -type f 2>/dev/null | head -1)
    if [ -n "$found" ]; then
        echo "$found"
        return 0
    fi
    
    return 1
}

# Analyze experiment result
analyze_experiment() {
    local result_file="$1"
    
    echo -e "${CYAN}Analyzing experiment result...${NC}\n"
    
    if [ ! -f "$result_file" ]; then
        echo -e "${RED}✗ Result file not found: ${result_file}${NC}"
        return 1
    fi
    
    # Extract key metrics using simple parsing
	local status=$(cat "$result_file" | grep '"Status"' | head -1 | sed 's/.*"Status"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
	local baseline=$(cat "$result_file" | grep '"BaselineValue"' | head -1 | sed 's/.*"BaselineValue"[[:space:]]*:[[:space:]]*\([0-9.\-]*\).*/\1/')
	local candidate=$(cat "$result_file" | grep '"CandidateValue"' | head -1 | sed 's/.*"CandidateValue"[[:space:]]*:[[:space:]]*\([0-9.\-]*\).*/\1/')
	local agent=$(cat "$result_file" | grep '"TargetAgentID"' | head -1 | sed 's/.*"TargetAgentID"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    
    echo "Experiment Summary:"
    echo "  Agent: ${agent:-unknown}"
    echo "  Status: ${status:-unknown}"
    echo "  Baseline: ${baseline:-N/A}"
    echo "  Candidate: ${candidate:-N/A}"
    
    if [ -n "$baseline" ] && [ -n "$candidate" ]; then
        local improvement=$(echo "$candidate - $baseline" | bc -l 2>/dev/null || echo "N/A")
        echo "  Improvement: ${improvement}"
        
        # Simple check
        if [ "$improvement" != "N/A" ]; then
            local better=$(echo "$improvement > 0" | bc -l 2>/dev/null || echo "0")
            if [ "$better" = "1" ]; then
                echo -e "  ${GREEN}✓ Candidate improves over baseline${NC}"
            else
                echo -e "  ${YELLOW}⚠ Candidate does not improve over baseline${NC}"
            fi
        fi
    fi
    
    echo ""
}

# Check if promotion is safe
check_promotion_safety() {
    local result_file="$1"
    
    echo -e "${CYAN}Running safety checks...${NC}\n"
    
    local checks_passed=0
    local checks_total=0
    
    # Check 1: Status is accepted or ready
    ((checks_total++))
	local status=$(cat "$result_file" | grep '"Status"' | head -1 | sed 's/.*"Status"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    if [ "$status" = "accepted" ] || [ "$status" = "running" ]; then
        echo -e "  ${GREEN}✓${NC} Experiment status: ${status}"
        ((checks_passed++))
    else
        echo -e "  ${YELLOW}⚠${NC} Experiment status: ${status} (may need judge first)"
    fi
    
    # Check 2: Has valid metrics
    ((checks_total++))
    if grep -q '"CandidateValue":' "$result_file"; then
        echo -e "  ${GREEN}✓${NC} Has candidate metrics"
        ((checks_passed++))
    else
        echo -e "  ${RED}✗${NC} Missing candidate metrics"
    fi
    
    # Check 3: Reason provided
    ((checks_total++))
    if [ -n "$REASON" ]; then
        echo -e "  ${GREEN}✓${NC} Reason provided"
        ((checks_passed++))
    else
        echo -e "  ${RED}✗${NC} No reason provided"
    fi
    
    echo ""
    echo "Safety: ${checks_passed}/${checks_total} checks passed"
    
    if [ "$checks_passed" -lt "$checks_total" ]; then
        echo -e "${YELLOW}Warning: Not all safety checks passed${NC}"
        return 1
    fi
    
    return 0
}

# Execute promotion
execute_promote() {
    local exp_id="$1"
    local result_file=$(find_latest_experiment "$exp_id")
    
    if [ -z "$result_file" ]; then
        echo -e "${RED}✗ Cannot find experiment result: ${exp_id}${NC}"
        exit 1
    fi
    
    analyze_experiment "$result_file"
    check_promotion_safety "$result_file"
    
    if [ "$DRY_RUN" = true ]; then
        echo -e "\n${YELLOW}=== DRY RUN ===${NC}"
        echo "Would execute:"
        echo "  go run ./cmd/promote-baseline -result ${result_file}"
        echo ""
        echo "Reason: ${REASON}"
        return 0
    fi
    
    if [ "$AUTO_CONFIRM" = false ]; then
        echo ""
        echo -n "Confirm promotion? [y/N]: "
        read -r confirm
        if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
            echo "Cancelled."
            exit 0
        fi
    fi
    
    echo ""
    echo -e "${CYAN}Executing promotion...${NC}"
    go run ./cmd/promote-baseline -result "$result_file"
    
    echo ""
    echo -e "${GREEN}✓ Promotion complete${NC}"
    echo "Reason logged: ${REASON}"
}

# Execute revert
execute_revert() {
    local version="$1"
    
    if [ -z "$version" ]; then
        version=""  # Will revert to last
    fi
    
    echo -e "${CYAN}Preparing revert...${NC}\n"
    
    # Show current state
    if [ -f "data/state/baseline_policy.json" ]; then
        local current_version=$(cat data/state/baseline_policy.json | grep -o '"Version": [0-9]*' | head -1 | grep -o '[0-9]*')
        echo "Current version: ${current_version}"
        if [ -n "$version" ]; then
            echo "Target version: ${version}"
        else
            echo "Target: previous version"
        fi
    fi
    
    if [ "$DRY_RUN" = true ]; then
        echo -e "\n${YELLOW}=== DRY RUN ===${NC}"
        if [ -n "$version" ]; then
            echo "Would execute:"
            echo "  go run ./cmd/revert-baseline -to-version ${version} -reason \"${REASON}\""
        else
            echo "Would execute:"
            echo "  go run ./cmd/revert-baseline -reason \"${REASON}\""
        fi
        return 0
    fi
    
    if [ "$AUTO_CONFIRM" = false ]; then
        echo ""
        echo -e "${YELLOW}⚠ Warning: Revert will roll back policy changes${NC}"
        echo -n "Confirm revert? [y/N]: "
        read -r confirm
        if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
            echo "Cancelled."
            exit 0
        fi
    fi
    
    echo ""
    echo -e "${CYAN}Executing revert...${NC}"
    
    if [ -n "$version" ]; then
        go run ./cmd/revert-baseline -to-version "$version" -reason "$REASON"
    else
        go run ./cmd/revert-baseline -reason "$REASON"
    fi
    
    echo ""
    echo -e "${GREEN}✓ Revert complete${NC}"
}

# Interactive mode
interactive_mode() {
    echo ""
    echo "Decision Mode"
    echo "=============="
    echo ""
    
    # List recent experiments
    if [ -f "data/state/experiments.jsonl" ]; then
        echo "Recent experiments:"
        tail -5 data/state/experiments.jsonl | while read line; do
            local id=$(echo "$line" | grep -o '"ID":"[^"]*"' | cut -d'"' -f4)
	local status=$(echo "$line" | grep '"Status"' | sed 's/.*"Status"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
            local agent=$(echo "$line" | grep -o '"TargetAgentID":"[^"]*"' | cut -d'"' -f4)
            echo "  - ${id} (${status}) - ${agent}"
        done
        echo ""
    fi
    
    echo -n "Action (promote/revert/skip): "
    read -r ACTION
    
    case "$ACTION" in
        promote)
            echo -n "Experiment ID to promote: "
            read -r TARGET
            echo -n "Reason for promotion: "
            read -r REASON
            execute_promote "$TARGET"
            ;;
        revert)
            echo -n "Revert to version (blank=previous): "
            read -r TARGET
            echo -n "Reason for revert: "
            read -r REASON
            if [ -z "$REASON" ]; then
                echo -e "${RED}Error: Reason is required${NC}"
                exit 1
            fi
            execute_revert "$TARGET"
            ;;
        skip)
            echo "No action taken."
            exit 0
            ;;
        *)
            echo "Unknown action: $ACTION"
            exit 1
            ;;
    esac
}

# Main
main() {
    print_header
    
    if [ -z "$ACTION" ] && [ -z "$TARGET" ] && [ -z "$REASON" ]; then
        interactive_mode
    else
        validate_inputs
        
        case "$ACTION" in
            promote)
                execute_promote "$TARGET"
                ;;
            revert)
                execute_revert "$TARGET"
                ;;
            *)
                echo -e "${RED}Error: Unknown action ${ACTION}${NC}"
                exit 1
                ;;
        esac
    fi
}

main "$@"
