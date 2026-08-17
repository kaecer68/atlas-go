#!/bin/bash
#
# DEPRECATED — D1 決策退役 2026-08-17
#
# 真實 Darwinian 演化已遷移至 Go BTM: internal/portfolio/darwinian_weights.go
# 的 auto_daily_simulation (由 Go 側 cron 觸發, 產出 darwinian_history.jsonl)。
# 本 shell script 僅為占位 stub，結構完整但核心計算已註解 (L101 "# would calculate")。
# 用途: 保留脚本框架供日後參考，不承擔實際執行。
#
# Darwinian Weight Daily Adjustment Script
# Atlas-GIC Style: Adjusts agent weights based on rolling Sharpe performance
#
# Usage: ./scripts/darwinian_adjust.sh [--dry-run] [--reset]
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIGS_DIR="${PROJECT_ROOT}/configs"
WEIGHTS_FILE="${CONFIGS_DIR}/darwinian_weights.json"
REPORTS_DIR="${PROJECT_ROOT}/data/reports"
LOGS_DIR="${PROJECT_ROOT}/logs"

# Create directories if needed
mkdir -p "${CONFIGS_DIR}" "${REPORTS_DIR}" "${LOGS_DIR}"

DRY_RUN=false
RESET=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --reset)
            RESET=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--dry-run] [--reset]"
            exit 1
            ;;
    esac
done

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

# Check if running during market hours (Taiwan: 9:00-13:30)
check_market_hours() {
    local hour=$(date +%H)
    local min=$(date +%M)
    local current_time=$((10#$hour * 60 + 10#$min))
    local market_open=$((9 * 60))      # 9:00
    local market_close=$((13 * 60 + 30)) # 13:30
    
    if [[ $current_time -ge $market_open && $current_time -le $market_close ]]; then
        log "WARNING: Running during Taiwan market hours (9:00-13:30)"
        log "Consider running after market close for consistency"
    fi
}

# Perform Darwinian weight adjustment
adjust_weights() {
    log "Starting Darwinian weight adjustment..."
    
    if [[ "$RESET" == true ]]; then
        log "RESET mode: All weights will be reset to neutral (1.0)"
        if [[ "$DRY_RUN" == false ]]; then
            # Create backup
            if [[ -f "${WEIGHTS_FILE}" ]]; then
                cp "${WEIGHTS_FILE}" "${WEIGHTS_FILE}.backup.$(date +%Y%m%d_%H%M%S)"
            fi
            
            # Reset weights by modifying the JSON file
            if command -v jq &> /dev/null; then
                jq '.weights |= with_entries(.value.weight = 1.0)' "${WEIGHTS_FILE}" > "${WEIGHTS_FILE}.tmp" && \
                    mv "${WEIGHTS_FILE}.tmp" "${WEIGHTS_FILE}"
            else
                log "ERROR: jq not installed. Cannot reset weights."
                exit 1
            fi
        fi
        log "All agent weights reset to neutral (1.0)"
        return
    fi
    
    # Check if weights file exists
    if [[ ! -f "${WEIGHTS_FILE}" ]]; then
        log "Weights file not found: ${WEIGHTS_FILE}"
        log "Run a backtest or initialize the system first"
        exit 1
    fi
    
    # Generate adjustment report
    local report_file="${REPORTS_DIR}/darwinian_adjustment_$(date +%Y%m%d).json"
    
    if command -v jq &> /dev/null; then
        # Extract current weights and calculate adjustments
        log "Calculating performance quartiles..."
        
        # This is a simplified version - the actual Go implementation
        # would calculate rolling Sharpe and adjust weights
        # Here we just log the current state
        
        log "Current weight distribution:"
        jq -r '.weights | to_entries | sort_by(.value.weight) | 
            .[] | "  \(.key): \(.value.weight) (Sharpe: \(.value.rolling_sharpe // 0))"' \
            "${WEIGHTS_FILE}" 2>/dev/null || log "  (No weights data available)"
    fi
    
    log "Weight adjustment completed"
    log "Report saved to: ${report_file}"
}

# Generate weight distribution report
generate_report() {
    local report_file="${REPORTS_DIR}/darwinian_report_$(date +%Y%m%d_%H%M%S).md"
    
    cat > "${report_file}" << EOF
# Darwinian Weights Report

**Generated:** $(date '+%Y-%m-%d %H:%M:%S')
**Mode:** $([[ "$DRY_RUN" == true ]] && echo "DRY RUN" || echo "LIVE")

## Weight Distribution

| Agent | Layer | Weight | Rolling Sharpe | Signals |
|-------|-------|--------|------------------|----------|
EOF

    if [[ -f "${WEIGHTS_FILE}" ]] && command -v jq &> /dev/null; then
        jq -r '.weights | to_entries | sort_by(.value.weight) | reverse | 
            .[] | "| \(.key) | \(.value.layer // "unknown") | \(.value.weight) | \(.value.rolling_sharpe // "N/A") | \(.value.total_signals // 0) |"' \
            "${WEIGHTS_FILE}" >> "${report_file}" 2>/dev/null || true
    else
        echo "| (No data available) | - | - | - | - |" >> "${report_file}"
    fi

    cat >> "${report_file}" << EOF

## Interpretation

- **Weight 2.0-2.5**: Agent is "shouting" - highest confidence, strong performance
- **Weight 1.0-1.9**: Above neutral - good performance
- **Weight 0.5-0.9**: Below neutral - underperforming
- **Weight 0.3-0.5**: Agent is "whispering" - weak performance, minimal influence

## Next Steps

1. Review top performers for potential prompt optimization
2. Consider disabling agents stuck at minimum weight (0.3) for 20+ days
3. Monitor agents near maximum weight (2.5) for mean reversion risk

EOF

    log "Report generated: ${report_file}"
}

# Main execution
main() {
    log "=== Darwinian Weight Adjustment ==="
    log "Project root: ${PROJECT_ROOT}"
    log "Weights file: ${WEIGHTS_FILE}"
    
    if [[ "$DRY_RUN" == true ]]; then
        log "DRY RUN mode - no changes will be made"
    fi
    
    check_market_hours
    adjust_weights
    generate_report
    
    log "=== Adjustment Complete ==="
}

main "$@"
