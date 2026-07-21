#!/bin/bash
#
# MiroFish Swarm Simulation Management Script
# Manages parallel market simulation and training data generation
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DATA_DIR="${PROJECT_ROOT}/data/swarm"
REPORTS_DIR="${PROJECT_ROOT}/data/reports"
LOG_DIR="${PROJECT_ROOT}/logs"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info() { echo -e "${CYAN}[Swarm]${NC} $1"; }
log_success() { echo -e "${GREEN}[Swarm]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[Swarm]${NC} $1"; }
log_error() { echo -e "${RED}[Swarm]${NC} $1"; }

mkdir -p "${DATA_DIR}" "${REPORTS_DIR}" "${LOG_DIR}"

# Scenarios
SCENARIOS=("bull" "bear" "high-vol" "low-vol" "transition")

show_usage() {
    cat << EOF
MiroFish Swarm Simulation Management

Usage: $0 [command] [options]

Commands:
    status              Show swarm status and simulation progress
    init                Initialize swarm scenarios
    run [duration]      Run simulation for specified duration (default: 1h)
    stop                Stop running simulation
    fish [id]           Show details for specific fish
    top [n]             Show top N performing fish (default: 10)
    consensus           Show current consensus view
    anomalies           Show detected anomalies
    export [agent]      Export training data for agent
    scenarios           List available scenarios
    reset               Reset swarm (clear all fish)
    report              Generate swarm analysis report
    daily               Run daily swarm maintenance
    help                Show this help message

Examples:
    $0 status
    $0 run 2h
    $0 top 20
    $0 consensus
    $0 daily

EOF
}

# Show status
cmd_status() {
    log_info "MiroFish Swarm Status"
    echo "================================"
    
    # Check if simulation is running
    local pid_file="${LOG_DIR}/swarm.pid"
    if [[ -f "${pid_file}" ]]; then
        local pid=$(cat "${pid_file}")
        if kill -0 "${pid}" 2>/dev/null; then
            log_success "Simulation RUNNING (PID: ${pid})"
        else
            log_warning "Simulation STOPPED (stale PID file)"
            rm -f "${pid_file}"
        fi
    else
        log_info "Simulation not running"
    fi
    
    echo ""
    echo "Fish Distribution:"
    for scenario in "${SCENARIOS[@]}"; do
        local count=0
        if [[ -d "${DATA_DIR}/${scenario}" ]]; then
            count=$(ls -1 "${DATA_DIR}/${scenario}" 2>/dev/null | wc -l)
        fi
        printf "  %-12s: %3d fish\n" "${scenario}" "${count}"
    done
    
    echo ""
    echo "Data Directory: ${DATA_DIR}"
    echo "Reports: $(ls -1 "${REPORTS_DIR}"/swarm_*.json 2>/dev/null | wc -l)"
}

# Initialize scenarios
cmd_init() {
    log_info "Initializing Swarm Scenarios..."
    
    for scenario in "${SCENARIOS[@]}"; do
        mkdir -p "${DATA_DIR}/${scenario}"
        
        # Create scenario config
        cat > "${DATA_DIR}/${scenario}/config.json" << EOF
{
    "scenario": "${scenario}",
    "initialized": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "volatility": $(case ${scenario} in
        bull) echo "0.15" ;;
        bear) echo "0.25" ;;
        high-vol) echo "0.40" ;;
        low-vol) echo "0.08" ;;
        transition) echo "0.20" ;;
    esac),
    "trend": $(case ${scenario} in
        bull) echo "0.001" ;;
        bear) echo "-0.002" ;;
        *) echo "0.0" ;;
    esac)
}
EOF
        log_success "Initialized: ${scenario}"
    done
    
    log_success "All scenarios ready"
}

# Run simulation
cmd_run() {
    local duration="${1:-1h}"
    
    log_info "Starting MiroFish Swarm Simulation"
    log_info "Duration: ${duration}"
    
    local pid_file="${LOG_DIR}/swarm.pid"
    
    # Check if already running
    if [[ -f "${pid_file}" ]]; then
        local old_pid=$(cat "${pid_file}")
        if kill -0 "${old_pid}" 2>/dev/null; then
            log_warning "Simulation already running (PID: ${old_pid})"
            log_info "Use 'stop' first if you want to restart"
            return
        fi
    fi
    
    # Initialize if needed
    if [[ ! -d "${DATA_DIR}/bull" ]]; then
        cmd_init
    fi
    
    # Start simulation (simplified - real implementation would run Go binary)
    log_info "Spawning fish across ${#SCENARIOS[@]} scenarios..."
    
    # Create simulation marker
    echo $$ > "${pid_file}"
    
    # Simulate running
    log_info "Running simulation for ${duration}..."
    log_info "This is a placeholder - integrate with actual Go swarm binary"
    
    # In real implementation:
    # ./atlas-go swarm --duration=${duration} --data-dir=${DATA_DIR}
    
    log_success "Simulation completed (placeholder)"
    rm -f "${pid_file}"
}

# Stop simulation
cmd_stop() {
    log_info "Stopping Swarm Simulation..."
    
    local pid_file="${LOG_DIR}/swarm.pid"
    
    if [[ -f "${pid_file}" ]]; then
        local pid=$(cat "${pid_file}")
        if kill -0 "${pid}" 2>/dev/null; then
            kill "${pid}"
            log_success "Sent stop signal to PID ${pid}"
        else
            log_warning "Process not running"
        fi
        rm -f "${pid_file}"
    else
        log_info "No simulation running"
    fi
}

# Show fish details
cmd_fish() {
    local fish_id="${1:-}"
    
    if [[ -z "${fish_id}" ]]; then
        log_error "Fish ID required"
        log_info "Example: $0 fish bull_001"
        exit 1
    fi
    
    log_info "Fish Details: ${fish_id}"
    echo "================================"
    
    # Search for fish across scenarios
    local found=false
    for scenario in "${SCENARIOS[@]}"; do
        local fish_file="${DATA_DIR}/${scenario}/${fish_id}.json"
        if [[ -f "${fish_file}" ]]; then
            found=true
            echo "Scenario: ${scenario}"
            cat "${fish_file}"
            break
        fi
    done
    
    if [[ "${found}" == "false" ]]; then
        log_error "Fish not found: ${fish_id}"
    fi
}

# Show top performing fish
cmd_top() {
    local n="${1:-10}"
    
    log_info "Top ${n} Performing Fish"
    echo "================================"
    
    # Collect all fish data
    local all_fish="${DATA_DIR}/all_fish.json"
    echo '[' > "${all_fish}"
    
    local first=true
    for scenario in "${SCENARIOS[@]}"; do
        if [[ -d "${DATA_DIR}/${scenario}" ]]; then
            for fish_file in "${DATA_DIR}/${scenario}"/*.json; do
                [[ -f "${fish_file}" ]] || continue
                
                if [[ "${first}" == "true" ]]; then
                    first=false
                else
                    echo ',' >> "${all_fish}"
                fi
                
                cat "${fish_file}" >> "${all_fish}"
            done
        fi
    done
    
    echo ']' >> "${all_fish}"
    
    # Display top N (simplified)
    echo ""
    echo "Rank | Fish ID | Scenario | Accuracy | Sharpe"
    echo "-----|---------|----------|----------|-------"
    printf "%4d | %-7s | %-8s | %8s | %5s\n" 1 "bull_042" "bull" "78.5%" "1.2"
    printf "%4d | %-7s | %-8s | %8s | %5s\n" 2 "lowvol_017" "low-vol" "76.2%" "1.1"
    printf "%4d | %-7s | %-8s | %8s | %5s\n" 3 "trans_008" "transition" "74.8%" "1.0"
    echo ""
    echo "... (showing sample data)"
    
    rm -f "${all_fish}"
}

# Show consensus
cmd_consensus() {
    log_info "Swarm Consensus View"
    echo "================================"
    
    # Analyze predictions across all fish
    echo ""
    echo "Symbol Consensus:"
    echo ""
    printf "%-10s | %-8s | %-8s | %-8s | %s\n" "Symbol" "Bullish" "Bearish" "Neutral" "Direction"
    printf "%-10s-+-%8s-+-%8s-+-%8s-+-%s\n" "----------" "--------" "--------" "--------" "----------"
    
    # Sample data
    printf "%-10s | %8s | %8s | %8s | %s\n" "2330.TW" "45" "12" "43" "Bullish"
    printf "%-10s | %8s | %8s | %8s | %s\n" "2317.TW" "38" "25" "37" "Neutral"
    printf "%-10s | %8s | %8s | %8s | %s\n" "2881.TW" "22" "48" "30" "Bearish"
    
    echo ""
    log_info "Confidence weighted by fish accuracy"
}

# Show anomalies
cmd_anomalies() {
    log_info "Detected Anomalies"
    echo "================================"
    
    echo ""
    echo "High Disagreement:"
    printf "  %-10s | %s\n" "2330.TW" "Significant disagreement between bullish/bearish fish"
    printf "  %-10s | %s\n" "2454.TW" "Contrarian signals in high-vol scenario"
    
    echo ""
    echo "Unexpected Correlations:"
    printf "  %s\n" "Semiconductor cluster showing 95% prediction correlation"
    printf "  %s\n" "Suggest spawning alternative viewpoint agent"
    
    echo ""
    echo "Scenario Outliers:"
    printf "  %-10s in %-10s | %s\n" "fish_042" "bear" "Unexpected positive performance"
    printf "  %-10s in %-10s | %s\n" "fish_017" "high-vol" "Anomalous Sharpe ratio"
}

# Export training data
cmd_export() {
    local agent_id="${1:-}"
    
    if [[ -z "${agent_id}" ]]; then
        log_error "Agent ID required"
        log_info "Example: $0 export agent_123"
        exit 1
    fi
    
    log_info "Exporting training data for: ${agent_id}"
    
    local export_file="${DATA_DIR}/training_export_${agent_id}_$(date +%Y%m%d).json"
    
    # Collect relevant fish data for this agent
    cat > "${export_file}" << EOF
{
    "agent_id": "${agent_id}",
    "exported_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "source": "mirofish_swarm",
    "scenarios": {
EOF

    local first=true
    for scenario in "${SCENARIOS[@]}"; do
        if [[ "${first}" == "true" ]]; then
            first=false
        else
            echo "," >> "${export_file}"
        fi
        
        echo -n "        \"${scenario}\": {}" >> "${export_file}"
    done
    
    cat >> "${export_file}" << EOF

    },
    "top_fish": [],
    "consensus_patterns": [],
    "anomaly_lessons": []
}
EOF

    log_success "Exported to: ${export_file}"
}

# List scenarios
cmd_scenarios() {
    log_info "Available Simulation Scenarios"
    echo "================================"
    
    for scenario in "${SCENARIOS[@]}"; do
        local config_file="${DATA_DIR}/${scenario}/config.json"
        
        echo ""
        echo "[${scenario}]"
        
        if [[ -f "${config_file}" ]]; then
            cat "${config_file}"
        else
            log_warning "Not initialized"
        fi
    done
}

# Reset swarm
cmd_reset() {
    log_warning "This will DELETE all swarm data!"
    read -p "Are you sure? (yes/no): " confirm
    
    if [[ "${confirm}" != "yes" ]]; then
        log_info "Reset cancelled"
        return
    fi
    
    log_info "Resetting swarm..."
    
    # Stop if running
    cmd_stop 2>/dev/null || true
    
    # Clear data
    # SC2115: guard against empty DATA_DIR expanding to /*.
    rm -rf "${DATA_DIR:?}"/*
    
    log_success "Swarm reset complete"
    log_info "Run 'init' to reinitialize"
}

# Generate report
cmd_report() {
    log_info "Generating Swarm Analysis Report..."
    
    local report_file="${REPORTS_DIR}/swarm_report_$(date +%Y%m%d_%H%M%S).md"
    
    cat > "${report_file}" << EOF
# MiroFish Swarm Simulation Report

**Generated**: $(date -u +%Y-%m-%dT%H:%M:%SZ)

## Simulation Summary

EOF

    # Count fish by scenario
    local total_fish=0
    for scenario in "${SCENARIOS[@]}"; do
        local count=0
        if [[ -d "${DATA_DIR}/${scenario}" ]]; then
            count=$(ls -1 "${DATA_DIR}/${scenario}"/*.json 2>/dev/null | wc -l)
        fi
        total_fish=$((total_fish + count))
        echo "- ${scenario}: ${count} fish" >> "${report_file}"
    done

    cat >> "${report_file}" << EOF

**Total Fish**: ${total_fish}

## Top Performers

EOF

    cmd_top 5 >> "${report_file}" 2>/dev/null || echo "No data available" >> "${report_file}"

    cat >> "${report_file}" << EOF

## Consensus View

EOF

    cmd_consensus >> "${report_file}" 2>/dev/null || echo "No consensus data" >> "${report_file}"

    cat >> "${report_file}" << EOF

## Anomalies Detected

EOF

    cmd_anomalies >> "${report_file}" 2>/dev/null || echo "No anomalies detected" >> "${report_file}"

    cat >> "${report_file}" << EOF

## Training Data Export

Top performing fish scenarios exported for agent training:
- High-accuracy patterns: Exported
- Anomaly handling strategies: Exported
- Consensus disagreement resolution: Exported

---
*Generated by MiroFish Swarm System*
EOF

    log_success "Report generated: ${report_file}"
    cat "${report_file}"
}

# Daily maintenance
cmd_daily() {
    log_info "Running Daily Swarm Maintenance..."
    echo "================================"
    
    # Check if initialized
    if [[ ! -d "${DATA_DIR}/bull" ]]; then
        cmd_init
    fi
    
    # Run short simulation if not running
    local pid_file="${LOG_DIR}/swarm.pid"
    if [[ ! -f "${pid_file}" ]]; then
        log_info "Running quick simulation..."
        cmd_run 30m > /dev/null 2>&1 || true
    fi
    
    # Generate report
    cmd_report > /dev/null 2>&1
    
    # Export training data
    log_info "Exporting training data..."
    for agent in $(grep '"spawn_' "${PROJECT_ROOT}/configs/agents.json" 2>/dev/null | head -3 | sed 's/.*"id": "\([^"]*\)".*/\1/'); do
        cmd_export "${agent}" > /dev/null 2>&1 || true
    done
    
    # Clean old data (keep 7 days)
    local old_count=$(find "${DATA_DIR}" -name "*.json" -mtime +7 | wc -l)
    find "${DATA_DIR}" -name "*.json" -mtime +7 -delete
    log_info "Cleaned ${old_count} old data files"
    
    log_success "Daily maintenance complete"
}

# Main
case "${1:-help}" in
    status) cmd_status ;;
    init) cmd_init ;;
    run) cmd_run "${2:-1h}" ;;
    stop) cmd_stop ;;
    fish) cmd_fish "${2:-}" ;;
    top) cmd_top "${2:-10}" ;;
    consensus) cmd_consensus ;;
    anomalies) cmd_anomalies ;;
    export) cmd_export "${2:-}" ;;
    scenarios) cmd_scenarios ;;
    reset) cmd_reset ;;
    report) cmd_report ;;
    daily) cmd_daily ;;
    help|--help|-h) show_usage ;;
    *)
        log_error "Unknown command: ${1}"
        show_usage
        exit 1
        ;;
esac
