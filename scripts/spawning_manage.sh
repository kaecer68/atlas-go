#!/bin/bash
#
# Agent Spawning Management Script
# Provides daily operations for the auto-spawning system
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG_DIR="${PROJECT_ROOT}/configs"
REPORTS_DIR="${PROJECT_ROOT}/data/reports"
LOG_DIR="${PROJECT_ROOT}/logs"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Ensure directories exist
mkdir -p "${REPORTS_DIR}" "${LOG_DIR}"

# Show usage
show_usage() {
    cat << EOF
Agent Spawning Management Script

Usage: $0 [command] [options]

Commands:
    status              Show current spawning system status
    detect              Run gap detection cycle
    spawn [type]        Manually spawn agent for gap type
                        Types: sector, style, regime, correlation
    accept [agent-id]   Accept a candidate agent
    reject [agent-id]   Reject a candidate agent
    cleanup [days]      Remove rejected agents older than N days (default: 30)
    report              Generate comprehensive spawning report
    daily               Run full daily cycle (detect + report)
    help                Show this help message

Examples:
    $0 status
    $0 detect
    $0 spawn sector biotech
    $0 accept spawn_biotech_123_4567890
    $0 cleanup 30
    $0 daily

EOF
}

# Get current timestamp
timestamp() {
    date +"%Y%m%d_%H%M%S"
}

# Run spawning status check
cmd_status() {
    log_info "Checking Agent Spawning System Status..."
    
    local report_file="${REPORTS_DIR}/spawning_status_$(timestamp).json"
    
    # Check if spawning config exists
    if [[ -f "${CONFIG_DIR}/spawning_config.json" ]]; then
        log_success "Spawning config found"
        cat "${CONFIG_DIR}/spawning_config.json"
    else
        log_warning "No spawning config found, using defaults"
    fi
    
    # Check for spawned agents in agents.json
    log_info "Checking spawned agents in registry..."
    if [[ -f "${CONFIG_DIR}/agents.json" ]]; then
        local spawn_count=$(grep -c '"spawn_' "${CONFIG_DIR}/agents.json" 2>/dev/null || echo "0")
        log_info "Found ${spawn_count} spawned agents"
    fi
    
    # Check for gap detection reports
    log_info "Recent gap detection reports:"
    ls -1t "${REPORTS_DIR}"/gap_detection_*.json 2>/dev/null | head -5 || log_warning "No gap detection reports found"
    
    # Check for prompt files
    log_info "Spawned agent prompts:"
    # SC2010: count spawn_* prompt files via glob, not `ls | grep`.
    local spawn_count=0
    for f in "${PROJECT_ROOT}/prompts/agents/"spawn_*; do
        [[ -e "$f" ]] || continue
        spawn_count=$((spawn_count + 1))
    done
    echo "  ${spawn_count} prompt files"
    
    log_success "Status check complete"
}

# Run gap detection
cmd_detect() {
    log_info "Running Gap Detection Cycle..."
    
    local report_file="${REPORTS_DIR}/gap_detection_$(date +%Y%m%d).json"
    
    # Create gap detection report structure
    cat > "${report_file}" << EOF
{
    "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "gaps_detected": [],
    "summary": {
        "total_gaps": 0,
        "critical": 0,
        "high": 0,
        "medium": 0,
        "low": 0
    },
    "recommendations": []
}
EOF
    
    log_info "Gap detection report created: ${report_file}"
    
    # Analyze current agent coverage
    log_info "Analyzing agent coverage..."
    
    if [[ -f "${CONFIG_DIR}/agents.json" ]]; then
        # Count agents by layer
        local sector_count=$(grep -c '"layer": "sector"' "${CONFIG_DIR}/agents.json" 2>/dev/null || echo "0")
        local style_count=$(grep -c '"layer": "style"' "${CONFIG_DIR}/agents.json" 2>/dev/null || echo "0")
        local super_count=$(grep -c '"layer": "superinvestor"' "${CONFIG_DIR}/agents.json" 2>/dev/null || echo "0")
        
        log_info "Current coverage:"
        log_info "  Sector agents: ${sector_count}"
        log_info "  Style agents: ${style_count}"
        log_info "  Superinvestor agents: ${super_count}"
        
        # Detect potential gaps
        if [[ ${sector_count} -lt 5 ]]; then
            log_warning "Low sector coverage detected (${sector_count} agents)"
        fi
        
        if [[ ${style_count} -lt 3 ]]; then
            log_warning "Low style coverage detected (${style_count} agents)"
        fi
    fi
    
    log_success "Gap detection complete. Report: ${report_file}"
}

# Manual spawn command
cmd_spawn() {
    local gap_type="${1:-sector}"
    local target="${2:-}"
    
    if [[ -z "${target}" ]]; then
        log_error "Target required for manual spawn"
        log_info "Usage: $0 spawn [type] [target]"
        log_info "Example: $0 spawn sector biotech"
        exit 1
    fi
    
    log_info "Manual spawning agent..."
    log_info "  Type: ${gap_type}"
    log_info "  Target: ${target}"
    
    # Generate agent ID
    local timestamp=$(date +%s)
    local agent_id="spawn_${target}_${timestamp}"
    local prompt_file="${PROJECT_ROOT}/prompts/agents/${agent_id}.md"
    
    # Create prompt template
    cat > "${prompt_file}" << EOF
# Auto-Spawned Agent: ${agent_id}

## Identity
You are an adaptive investment specialist auto-generated to address a specific knowledge gap.

## Purpose
**Gap Type**: ${gap_type}
**Target**: ${target}
**Created**: $(date +%Y-%m-%d)

## Specialization
This agent specializes in the **${target}** ${gap_type}.

## Operating Guidelines
- Focus on your assigned specialization
- Coordinate with existing agents to avoid duplication
- Report unusual patterns that might indicate new gaps
- Maintain diversity of perspective from other agents

## Constraints
- Do not recommend outside your assigned universe without explicit reason
- Always provide conviction score (1-100)
- Include both bullish and bearish scenarios when relevant

## Output Format
RECOMMENDATION: [SYMBOL] | [BUY/SELL/HOLD] | [CONVICTION 1-100]
RATIONALE: [2-3 sentence clear explanation]
CATALYST: [Near-term trigger or None]
RISK: [Primary risk factor]

---
*This agent was automatically generated by the Atlas spawning system.*
EOF
    
    log_success "Created prompt file: ${prompt_file}"
    
    # Note: Actual agent registration would require modifying agents.json
    # This is a placeholder for the manual spawn operation
    log_info "Agent ID: ${agent_id}"
    log_warning "Note: Manual registration to agents.json required"
    log_info "Add the following to configs/agents.json:"
    cat << EOF
    {
        "id": "${agent_id}",
        "name": "${target} Specialist (Auto)",
        "layer": "${gap_type}",
        "skill": "${gap_type}_${target}_specialist",
        "promptFile": "prompts/agents/${agent_id}.md",
        "enabled": false,
        "darwinian_weight": 1.0
    }
EOF
}

# Accept agent command
cmd_accept() {
    local agent_id="${1:-}"
    
    if [[ -z "${agent_id}" ]]; then
        log_error "Agent ID required"
        exit 1
    fi
    
    log_info "Accepting agent: ${agent_id}"
    
    # Check if agent exists
    if [[ ! -f "${PROJECT_ROOT}/prompts/agents/${agent_id}.md" ]]; then
        log_error "Agent prompt file not found: ${agent_id}"
        exit 1
    fi
    
    # Note: Actual acceptance would require modifying agents.json
    log_success "Agent ${agent_id} accepted for production"
    log_warning "Note: Manual enable in agents.json required"
}

# Reject agent command
cmd_reject() {
    local agent_id="${1:-}"
    
    if [[ -z "${agent_id}" ]]; then
        log_error "Agent ID required"
        exit 1
    fi
    
    log_info "Rejecting agent: ${agent_id}"
    
    # Move to rejected directory
    local rejected_dir="${PROJECT_ROOT}/prompts/agents/rejected"
    mkdir -p "${rejected_dir}"
    
    if [[ -f "${PROJECT_ROOT}/prompts/agents/${agent_id}.md" ]]; then
        mv "${PROJECT_ROOT}/prompts/agents/${agent_id}.md" "${rejected_dir}/${agent_id}_$(timestamp).md"
        log_success "Agent moved to rejected: ${rejected_dir}"
    else
        log_warning "Agent prompt file not found"
    fi
}

# Cleanup old rejected agents
cmd_cleanup() {
    local days="${1:-30}"
    
    log_info "Cleaning up rejected agents older than ${days} days..."
    
    local rejected_dir="${PROJECT_ROOT}/prompts/agents/rejected"
    
    if [[ -d "${rejected_dir}" ]]; then
        local count=$(find "${rejected_dir}" -name "*.md" -mtime +${days} | wc -l)
        find "${rejected_dir}" -name "*.md" -mtime +${days} -delete
        log_success "Removed ${count} old rejected agent files"
    else
        log_info "No rejected agents directory found"
    fi
    
    # Clean old reports
    local reports_count=$(find "${REPORTS_DIR}" -name "gap_detection_*.json" -mtime +${days} | wc -l)
    find "${REPORTS_DIR}" -name "gap_detection_*.json" -mtime +${days} -delete
    log_success "Removed ${reports_count} old gap detection reports"
}

# Generate comprehensive report
cmd_report() {
    log_info "Generating Spawning System Report..."
    
    local report_file="${REPORTS_DIR}/spawning_report_$(date +%Y%m%d).md"
    
    cat > "${report_file}" << EOF
# Agent Spawning System Report

**Generated**: $(date -u +%Y-%m-%dT%H:%M:%SZ)

## Summary

### Current Spawned Agents
EOF

    # Count spawned agents
    # SC2010: count spawn_* prompts via glob, not `ls | grep`.
    local total_spawned=0
    for _f in "${PROJECT_ROOT}/prompts/agents/"spawn_*; do
        [[ -e "$_f" ]] || continue
        total_spawned=$((total_spawned + 1))
    done
    local active_spawned=$(grep -c '"spawn_' "${CONFIG_DIR}/agents.json" 2>/dev/null || echo "0")
    
    cat >> "${report_file}" << EOF
- Total spawned (all time): ${total_spawned}
- Active in registry: ${active_spawned}

### Gap Detection History
EOF

    # List recent gap detection reports
    ls -1t "${REPORTS_DIR}"/gap_detection_*.json 2>/dev/null | head -7 | while read f; do
        echo "- $(basename $f)" >> "${report_file}"
    done

    cat >> "${report_file}" << EOF

### System Health

- Spawning system: Operational
- Last check: $(date)

## Recommendations

EOF

    if [[ ${active_spawned} -lt 3 ]]; then
        echo "- Consider spawning more agents to cover identified gaps" >> "${report_file}"
    fi
    
    echo "- Review rejected agents periodically for patterns" >> "${report_file}"
    echo "- Monitor sector coverage for underserved areas" >> "${report_file}"

    log_success "Report generated: ${report_file}"
    cat "${report_file}"
}

# Run daily cycle
cmd_daily() {
    log_info "Running Daily Spawning Cycle..."
    
    cmd_detect
    cmd_report
    cmd_cleanup 30
    
    log_success "Daily cycle complete"
}

# Main command dispatcher
main() {
    case "${1:-help}" in
        status)
            cmd_status
            ;;
        detect)
            cmd_detect
            ;;
        spawn)
            cmd_spawn "${2:-}" "${3:-}"
            ;;
        accept)
            cmd_accept "${2:-}"
            ;;
        reject)
            cmd_reject "${2:-}"
            ;;
        cleanup)
            cmd_cleanup "${2:-30}"
            ;;
        report)
            cmd_report
            ;;
        daily)
            cmd_daily
            ;;
        help|--help|-h)
            show_usage
            ;;
        *)
            log_error "Unknown command: ${1}"
            show_usage
            exit 1
            ;;
    esac
}

main "$@"
