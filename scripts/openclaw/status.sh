#!/bin/bash
#
# OpenClaw Status Reporter
# 報告系統當前狀態，供 OpenClaw 或人類操作者快速了解現況
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

cd "$PROJECT_ROOT"

# Colors for terminal output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
print_header() {
    echo -e "\n${BLUE}=== $1 ===${NC}"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# Check if baseline policy exists
check_baseline() {
    print_header "Baseline Policy Status"
    
    if [ -f "data/state/baseline_policy.json" ]; then
        VERSION=$(cat data/state/baseline_policy.json | grep -o '"version": [0-9]*' | head -1 | grep -o '[0-9]*')
        PROMOTIONS=$(cat data/state/baseline_policy.json | grep -o '"promotions":' | wc -l)
        
        print_success "Baseline policy exists (version ${VERSION})"
        echo "  Version: ${VERSION}"
        echo "  Last updated: $(stat -c %y data/state/baseline_policy.json 2>/dev/null || stat -f %Sm data/state/baseline_policy.json 2>/dev/null || echo 'unknown')"
    else
        print_warning "No baseline policy found (will use default)"
    fi
}

# Check experiments
check_experiments() {
    print_header "Experiment Status"
    
    if [ -f "data/state/experiments.jsonl" ]; then
        TOTAL=$(wc -l < data/state/experiments.jsonl 2>/dev/null || echo "0")
        PLANNED=$(grep -c '"status":"planned"' data/state/experiments.jsonl 2>/dev/null || echo "0")
        RUNNING=$(grep -c '"status":"running"' data/state/experiments.jsonl 2>/dev/null || echo "0")
        ACCEPTED=$(grep -c '"status":"accepted"' data/state/experiments.jsonl 2>/dev/null || echo "0")
        REJECTED=$(grep -c '"status":"rejected"' data/state/experiments.jsonl 2>/dev/null || echo "0")
        
        echo "  Total experiments: ${TOTAL}"
        echo "  📝 Planned: ${PLANNED}"
        echo "  🔄 Running: ${RUNNING}"
        echo "  ✅ Accepted: ${ACCEPTED}"
        echo "  ❌ Rejected: ${REJECTED}"
        
        if [ "$RUNNING" -gt 0 ]; then
            print_warning "${RUNNING} experiment(s) currently running"
        fi
    else
        print_warning "No experiments found"
    fi
}

# Check replay data availability
check_replay_data() {
    print_header "Replay Data Status"
    
    if [ -d "data/replay" ]; then
        FILE_COUNT=$(find data/replay -type f -name "*.jsonl" -o -name "*.csv" 2>/dev/null | wc -l)
        if [ "$FILE_COUNT" -gt 0 ]; then
            print_success "Replay data available (${FILE_COUNT} files)"
            ls -lh data/replay/ | tail -n +2 | awk '{print "  " $9 " (" $5 ")"}'
        else
            print_warning "Replay directory exists but no data files"
        fi
    else
        print_error "No replay data directory"
    fi
}

# Identify weakest agent
find_weakest_agent() {
    print_header "Weakest Agent Analysis"
    
    # Check if we have recommendation outcomes
    if [ -f "data/state/recommendation_outcomes.jsonl" ]; then
        echo "  Analyzing recommendation outcomes..."
        
        # Use Go to analyze (more reliable than shell for JSON)
        go run cmd/atlas/main.go 2>/dev/null | grep -i "weakest\|scorecard" | head -5 || {
            print_warning "Could not determine weakest agent (run 'go run ./cmd/backtest-window' for detailed analysis)"
        }
    else
        print_warning "No recommendation outcomes available"
        echo "  Run: go run ./cmd/backtest-window -start YYYY-MM-DD -end YYYY-MM-DD"
    fi
}

# Show recent activity
show_recent_activity() {
    print_header "Recent Activity (Last 24h)"
    
    # Check for recent files
    RECENT_FILES=$(find data/state -type f -mtime -1 2>/dev/null | wc -l)
    if [ "$RECENT_FILES" -gt 0 ]; then
        echo "  Recent modifications: ${RECENT_FILES} files"
        find data/state -type f -mtime -1 -exec ls -lt {} + 2>/dev/null | head -3 | awk '{print "  - " $9 " (" $6 " " $7 " " $8 ")"}'
    else
        echo "  No activity in last 24 hours"
    fi
}

# Recommend next action
recommend_action() {
    print_header "Recommended Next Action"
    
    # Check for running experiments
    if [ -f "data/state/experiments.jsonl" ]; then
        RUNNING=$(grep -c '"status":"running"' data/state/experiments.jsonl 2>/dev/null || echo "0")
        PLANNED=$(grep -c '"status":"planned"' data/state/experiments.jsonl 2>/dev/null || echo "0")
        
        if [ "$RUNNING" -gt 0 ]; then
            print_warning "Finish running experiment first:"
            echo "  ./scripts/openclaw/judge-latest.sh"
        elif [ "$PLANNED" -gt 0 ]; then
            print_success "Execute planned experiment:"
            echo "  ./scripts/openclaw/execute-next.sh"
        else
            print_success "Start new iteration cycle:"
            echo "  ./scripts/openclaw/propose-mutation.sh"
        fi
    else
        print_success "Initialize system:"
        echo "  1. Import replay data: go run ./cmd/import-replay ..."
        echo "  2. Run backtest: go run ./cmd/backtest-window ..."
        echo "  3. Generate mutation: ./scripts/openclaw/propose-mutation.sh"
    fi
}

# Main execution
main() {
    echo -e "${GREEN}╔════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║        Atlas OpenClaw Status Report            ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════════╝${NC}"
    
    check_baseline
    check_experiments
    check_replay_data
    find_weakest_agent
    show_recent_activity
    recommend_action
    
    print_header "Quick Commands"
    echo "  ./scripts/openclaw/status.sh          # Show this report"
    echo "  ./scripts/openclaw/propose-mutation.sh # Generate mutation suggestion"
    echo "  ./scripts/openclaw/execute-next.sh    # Execute next planned experiment"
    echo "  ./scripts/openclaw/judge-latest.sh    # Judge latest experiment"
    echo "  ./scripts/openclaw/decide.sh          # Promote or revert decision"
}

main "$@"
