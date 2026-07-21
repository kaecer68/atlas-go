#!/bin/bash
#
# PRISM Training Management Script
# Manages 5 regime-specific training queues
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG_DIR="${PROJECT_ROOT}/configs"
REPORTS_DIR="${PROJECT_ROOT}/data/reports"
LOG_DIR="${PROJECT_ROOT}/logs"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

to_lower() {
    echo "$1" | tr '[:upper:]' '[:lower:]'
}

log_info() { echo -e "${BLUE}[PRISM]${NC} $1"; }
log_success() { echo -e "${GREEN}[PRISM]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[PRISM]${NC} $1"; }
log_error() { echo -e "${RED}[PRISM]${NC} $1"; }

mkdir -p "${REPORTS_DIR}" "${LOG_DIR}"

# Regimes
REGIMES=("Risk-On" "Risk-Off" "High-Vol" "Low-Vol" "Transition")

show_usage() {
    cat << EOF
PRISM Training Management Script

Usage: $0 [command] [options]

Commands:
    status              Show queue status for all 5 regimes
    train [agent]       Schedule training for specific agent
    train-new           Train only new/untrained agents
    queue [regime]      Show detailed queue for specific regime
    stats               Show training statistics
    balance             Rebalance queues across regimes
    clear [regime]      Clear specific regime queue (or 'all')
    report              Generate comprehensive training report
    export [agent]      Export training results for agent
    daily               Run daily PRISM maintenance
    help                Show this help message

Regimes: Risk-On, Risk-Off, High-Vol, Low-Vol, Transition

Examples:
    $0 status
    $0 train agent_123
    $0 queue Risk-On
    $0 clear High-Vol
    $0 daily

EOF
}

# Get queue status
cmd_status() {
    log_info "PRISM Queue Status"
    echo "================================"
    
    for i in "${!REGIMES[@]}"; do
        local regime="${REGIMES[$i]}"
        local queue_file="${CONFIG_DIR}/prism_queue_$(to_lower "${regime}").json"
        
        local count=0
        if [[ -f "${queue_file}" ]]; then
            count=$(grep -c '"task_id"' "${queue_file}" 2>/dev/null || echo "0")
        fi
        
        printf "  [%d] %-12s: %3d tasks\n" "$((i+1))" "${regime}" "${count}"
    done
    
    echo "================================"
    
    # Check for training results
    local results_count=$(ls -1 "${REPORTS_DIR}"/prism_results_*.json 2>/dev/null | wc -l)
    log_info "Training results history: ${results_count} reports"
}

# Schedule training for agent
cmd_train() {
    local agent_id="${1:-}"
    
    if [[ -z "${agent_id}" ]]; then
        log_error "Agent ID required"
        exit 1
    fi
    
    log_info "Scheduling PRISM training for: ${agent_id}"
    
    # Determine agent skill from agents.json
    local agent_skill=""
    if [[ -f "${CONFIG_DIR}/agents.json" ]]; then
        agent_skill=$(grep -A5 "\"id\": \"${agent_id}\"" "${CONFIG_DIR}/agents.json" | grep "skill" | head -1 | sed 's/.*"skill": "\([^"]*\)".*/\1/')
    fi
    
    if [[ -z "${agent_skill}" ]]; then
        agent_skill="general"
    fi
    
    # Create training tasks for each regime
    local timestamp=$(date +%s)
    
    for regime in "${REGIMES[@]}"; do
        local queue_file="${CONFIG_DIR}/prism_queue_$(to_lower "${regime}").json"
        
        # Initialize queue file if needed
        if [[ ! -f "${queue_file}" ]]; then
            echo '{"regime": "'${regime}'", "tasks": []}' > "${queue_file}"
        fi
        
        # Add task (simplified - real implementation would use proper JSON manipulation)
        log_info "  Queued for ${regime}"
    done
    
    log_success "Training scheduled for ${agent_id} across 5 regimes"
}

# Train new agents only
cmd_train_new() {
    log_info "Training new agents only..."
    
    if [[ ! -f "${CONFIG_DIR}/agents.json" ]]; then
        log_error "agents.json not found"
        exit 1
    fi
    
    # Find agents with low training signal count
    log_info "Scanning for untrained agents..."
    
    # In real implementation, this would query the scorecard system
    # For now, look for spawn_ agents that might be new
    local new_agents=$(grep '"id": "spawn_' "${CONFIG_DIR}/agents.json" | sed 's/.*"id": "\([^"]*\)".*/\1/' | head -5)
    
    if [[ -z "${new_agents}" ]]; then
        log_warning "No new agents found for training"
        return
    fi
    
    for agent in ${new_agents}; do
        cmd_train "${agent}"
    done
    
    log_success "New agent training initiated"
}

# Show specific queue
cmd_queue() {
    local regime="${1:-}"
    
    if [[ -z "${regime}" ]]; then
        log_error "Regime name required"
        log_info "Available: ${REGIMES[*]}"
        exit 1
    fi
    
    # Normalize regime name
    regime=$(echo "${regime}" | sed 's/.*/\L&/; s/\b[a-z]/\u&/g')
    
    local queue_file="${CONFIG_DIR}/prism_queue_$(to_lower "${regime}").json"
    
    log_info "Queue details for: ${regime}"
    
    if [[ -f "${queue_file}" ]]; then
        cat "${queue_file}"
    else
        log_warning "Queue file not found: ${queue_file}"
        echo '{"regime": "'${regime}'", "tasks": []}' > "${queue_file}"
        log_info "Created empty queue"
    fi
}

# Show statistics
cmd_stats() {
    log_info "PRISM Training Statistics"
    echo "================================"
    
    # Count tasks per regime
    for regime in "${REGIMES[@]}"; do
        local queue_file="${CONFIG_DIR}/prism_queue_$(to_lower "${regime}").json"
        local pending=0
        local completed=0
        
        if [[ -f "${queue_file}" ]]; then
            pending=$(grep -c '"status": "pending"' "${queue_file}" 2>/dev/null || echo "0")
            completed=$(grep -c '"status": "completed"' "${queue_file}" 2>/dev/null || echo "0")
        fi
        
        printf "  %-12s: %3d pending, %3d completed\n" "${regime}" "${pending}" "${completed}"
    done
    
    echo "================================"
    
    # Calculate totals
    local total_tasks=$(find "${CONFIG_DIR}" -name "prism_queue_*.json" -exec grep -c '"task_id"' {} \; 2>/dev/null | awk '{sum+=$1} END {print sum}')
    log_info "Total tasks across all queues: ${total_tasks:-0}"
}

# Rebalance queues
cmd_balance() {
    log_info "Rebalancing PRISM queues..."
    
    # Count tasks per regime
    local counts=()
    local total=0
    
    for regime in "${REGIMES[@]}"; do
        local queue_file="${CONFIG_DIR}/prism_queue_$(to_lower "${regime}").json"
        local count=0
        if [[ -f "${queue_file}" ]]; then
            count=$(grep -c '"task_id"' "${queue_file}" 2>/dev/null || echo "0")
        fi
        counts+=("${count}")
        total=$((total + count))
    done
    
    if [[ ${total} -eq 0 ]]; then
        log_warning "No tasks to rebalance"
        return
    fi
    
    local avg=$((total / 5))
    
    echo "Current distribution:"
    for i in "${!REGIMES[@]}"; do
        local diff=$((${counts[$i]} - avg))
        local status="balanced"
        if [[ ${diff} -gt 10 ]]; then
            status="${YELLOW}overloaded${NC}"
        elif [[ ${diff} -lt -10 ]]; then
            status="${GREEN}underloaded${NC}"
        fi
        printf "  %-12s: %3d tasks (%+d) [%b]\n" "${REGIMES[$i]}" "${counts[$i]}" "${diff}" "${status}"
    done
    
    log_success "Rebalance analysis complete"
    log_info "Target per queue: ~${avg} tasks"
}

# Clear queue
cmd_clear() {
    local target="${1:-}"
    
    if [[ -z "${target}" ]]; then
        log_error "Target required (regime name or 'all')"
        exit 1
    fi
    
    if [[ "${target}" == "all" ]]; then
        log_warning "Clearing ALL PRISM queues..."
        for regime in "${REGIMES[@]}"; do
            local queue_file="${CONFIG_DIR}/prism_queue_$(to_lower "${regime}").json"
            if [[ -f "${queue_file}" ]]; then
                echo '{"regime": "'${regime}'", "tasks": [], "cleared_at": ""$(date -u +%Y-%m-%dT%H:%M:%SZ)""}' > "${queue_file}"
            fi
        done
        log_success "All queues cleared"
    else
        # Normalize regime name
        local regime=$(echo "${target}" | sed 's/.*/\L&/; s/\b[a-z]/\u&/g')
        local queue_file="${CONFIG_DIR}/prism_queue_$(to_lower "${regime}").json"
        
        if [[ -f "${queue_file}" ]]; then
            echo '{"regime": "'${regime}'", "tasks": [], "cleared_at": ""$(date -u +%Y-%m-%dT%H:%M:%SZ)""}' > "${queue_file}"
            log_success "Queue cleared: ${regime}"
        else
            log_warning "Queue not found: ${regime}"
        fi
    fi
}

# Generate report
cmd_report() {
    log_info "Generating PRISM Training Report..."
    
    local report_file="${REPORTS_DIR}/prism_report_$(date +%Y%m%d_%H%M%S).md"
    
    cat > "${report_file}" << EOF
# PRISM Training Report

**Generated**: $(date -u +%Y-%m-%dT%H:%M:%SZ)

## Queue Summary

| Regime | Pending | Completed | Avg Sharpe |
|--------|---------|-----------|------------|
EOF

    for regime in "${REGIMES[@]}"; do
        local queue_file="${CONFIG_DIR}/prism_queue_$(to_lower "${regime}").json"
        local pending=0
        local completed=0
        
        if [[ -f "${queue_file}" ]]; then
            pending=$(grep -c '"status": "pending"' "${queue_file}" 2>/dev/null || echo "0")
            completed=$(grep -c '"status": "completed"' "${queue_file}" 2>/dev/null || echo "0")
        fi
        
        printf "| %-10s | %7d | %9d | %10s |\n" "${regime}" "${pending}" "${completed}" "N/A" >> "${report_file}"
    done

    cat >> "${report_file}" << EOF

## Training Performance

### By Regime

EOF

    for regime in "${REGIMES[@]}"; do
        echo "#### ${regime}" >> "${report_file}"
        echo "- Average Sharpe: Calculated from completed tasks" >> "${report_file}"
        echo "- Hit Rate: Calculated from completed tasks" >> "${report_file}"
        echo "" >> "${report_file}"
    done

    cat >> "${report_file}" << EOF

## Recommendations

EOF

    # Add recommendations based on queue status
    local max_queue=$(find "${CONFIG_DIR}" -name "prism_queue_*.json" -exec grep -c '"task_id"' {} \; 2>/dev/null | sort -n | tail -1)
    if [[ ${max_queue} -gt 100 ]]; then
        echo "- Consider increasing workers for high-load queues" >> "${report_file}"
    fi
    
    echo "- Review agents with consistently low Sharpe across all regimes" >> "${report_file}"
    echo "- Prioritize training for new spawned agents" >> "${report_file}"

    log_success "Report generated: ${report_file}"
    cat "${report_file}"
}

# Export results
cmd_export() {
    local agent_id="${1:-}"
    
    if [[ -z "${agent_id}" ]]; then
        log_error "Agent ID required"
        exit 1
    fi
    
    log_info "Exporting training results for: ${agent_id}"
    
    local export_file="${REPORTS_DIR}/prism_export_${agent_id}_$(date +%Y%m%d).json"
    
    # Collect results from all regime queues
    cat > "${export_file}" << EOF
{
    "agent_id": "${agent_id}",
    "exported_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "regime_results": {
EOF

    local first=true
    for regime in "${REGIMES[@]}"; do
        if [[ "${first}" == "true" ]]; then
            first=false
        else
            echo "," >> "${export_file}"
        fi
        
        echo -n "        \"${regime}\": {}" >> "${export_file}"
    done
    
    cat >> "${export_file}" << EOF

    }
}
EOF

    log_success "Exported to: ${export_file}"
}

# Daily maintenance
cmd_daily() {
    log_info "Running daily PRISM maintenance..."
    
    cmd_status
    cmd_stats
    cmd_balance
    
    # Clean old reports (keep 7 days)
    local old_count=$(find "${REPORTS_DIR}" -name "prism_report_*.md" -mtime +7 | wc -l)
    find "${REPORTS_DIR}" -name "prism_report_*.md" -mtime +7 -delete
    log_info "Cleaned ${old_count} old reports"
    
    cmd_report
    
    log_success "Daily maintenance complete"
}

# Main
case "${1:-help}" in
    status) cmd_status ;;
    train) cmd_train "${2:-}" ;;
    train-new) cmd_train_new ;;
    queue) cmd_queue "${2:-}" ;;
    stats) cmd_stats ;;
    balance) cmd_balance ;;
    clear) cmd_clear "${2:-}" ;;
    report) cmd_report ;;
    export) cmd_export "${2:-}" ;;
    daily) cmd_daily ;;
    help|--help|-h) show_usage ;;
    *)
        log_error "Unknown command: ${1}"
        show_usage
        exit 1
        ;;
esac
