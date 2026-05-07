#!/bin/bash
#
# Resource Guard - 资源监控与保护脚本
# 监控系统资源，在超过阈值时发出警告或暂停执行
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
CONFIG_FILE="${PROJECT_DIR}/configs/monitor_limits.json"

# 默认阈值（平衡配置）
DEFAULT_CPU_THRESHOLD=75
DEFAULT_MEM_THRESHOLD=80
DEFAULT_DISK_THRESHOLD=85

# 颜色输出
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m'

log_warn() {
    echo -e "${YELLOW}⚠ [ResourceGuard]${NC} $1"
}

log_error() {
    echo -e "${RED}✗ [ResourceGuard]${NC} $1"
}

log_ok() {
    echo -e "${GREEN}✓ [ResourceGuard]${NC} $1"
}

# 读取配置文件
load_config() {
    local cpu_threshold=$DEFAULT_CPU_THRESHOLD
    local mem_threshold=$DEFAULT_MEM_THRESHOLD
    local disk_threshold=$DEFAULT_DISK_THRESHOLD
    
    if [[ -f "$CONFIG_FILE" ]]; then
        if command -v jq &> /dev/null; then
            cpu_threshold=$(jq -r '.cpu_threshold_percent // empty' "$CONFIG_FILE" 2>/dev/null || echo "$DEFAULT_CPU_THRESHOLD")
            mem_threshold=$(jq -r '.memory_threshold_percent // empty' "$CONFIG_FILE" 2>/dev/null || echo "$DEFAULT_MEM_THRESHOLD")
            disk_threshold=$(jq -r '.disk_threshold_percent // empty' "$CONFIG_FILE" 2>/dev/null || echo "$DEFAULT_DISK_THRESHOLD")
        fi
    fi
    
    echo "${cpu_threshold:-$DEFAULT_CPU_THRESHOLD} ${mem_threshold:-$DEFAULT_MEM_THRESHOLD} ${disk_threshold:-$DEFAULT_DISK_THRESHOLD}"
}

# 获取CPU使用率 (macOS)
get_cpu_usage() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS: 使用 top 命令
        local cpu_idle=$(top -l 1 -n 0 | grep "CPU usage" | awk '{print $7}' | tr -d '%')
        if [[ -n "$cpu_idle" ]]; then
            echo "$(echo "100 - $cpu_idle" | bc)"
        else
            echo "0"
        fi
    else
        # Linux
        grep 'cpu ' /proc/stat | awk '{usage=($2+$4)*100/($2+$4+$5)} END {print usage}'
    fi
}

# 获取内存使用率 (macOS)
get_memory_usage() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS: 使用 vm_stat
        local page_size=$(vm_stat | grep "page size" | awk '{print $8}' || echo "4096")
        [[ -z "$page_size" ]] && page_size=4096
        
        local vm_stats=$(vm_stat)
        local free_pages=$(echo "$vm_stats" | grep "Pages free" | awk '{print $3}' | tr -d '.')
        local active_pages=$(echo "$vm_stats" | grep "Pages active" | awk '{print $3}' | tr -d '.')
        local inactive_pages=$(echo "$vm_stats" | grep "Pages inactive" | awk '{print $3}' | tr -d '.')
        local wired_pages=$(echo "$vm_stats" | grep "Pages wired down" | awk '{print $4}' | tr -d '.')
        
        [[ -z "$free_pages" ]] && free_pages=0
        [[ -z "$active_pages" ]] && active_pages=0
        [[ -z "$inactive_pages" ]] && inactive_pages=0
        [[ -z "$wired_pages" ]] && wired_pages=0
        
        local used_pages=$((active_pages + inactive_pages + wired_pages))
        local total_pages=$((used_pages + free_pages))
        
        if [[ $total_pages -gt 0 ]]; then
            echo "$(echo "scale=2; $used_pages * 100 / $total_pages" | bc)"
        else
            echo "0"
        fi
    else
        # Linux
        free | grep Mem | awk '{print ($3/$2) * 100.0}'
    fi
}

# 获取磁盘使用率
get_disk_usage() {
    df -h / | tail -1 | awk '{print $5}' | tr -d '%'
}

# 检查资源状态
check_resources() {
    local config=$(load_config)
    local cpu_threshold=$(echo "$config" | awk '{print $1}')
    local mem_threshold=$(echo "$config" | awk '{print $2}')
    local disk_threshold=$(echo "$config" | awk '{print $3}')
    
    local cpu_usage=$(get_cpu_usage)
    local mem_usage=$(get_memory_usage)
    local disk_usage=$(get_disk_usage)
    
    # 转换为整数进行比较
    local cpu_int=$(echo "$cpu_usage" | cut -d. -f1)
    local mem_int=$(echo "$mem_usage" | cut -d. -f1)
    
    [[ -z "$cpu_int" ]] && cpu_int=0
    [[ -z "$mem_int" ]] && mem_int=0
    [[ -z "$disk_usage" ]] && disk_usage=0
    
    local status=0
    local warnings=""
    
    if [[ $cpu_int -gt $cpu_threshold ]]; then
        log_warn "CPU 使用率过高: ${cpu_usage}% (阈值: ${cpu_threshold}%)"
        warnings="${warnings}CPU:${cpu_usage}% "
        status=1
    fi
    
    if [[ $mem_int -gt $mem_threshold ]]; then
        log_warn "内存使用率过高: ${mem_usage}% (阈值: ${mem_threshold}%)"
        warnings="${warnings}MEM:${mem_usage}% "
        status=1
    fi
    
    if [[ $disk_usage -gt $disk_threshold ]]; then
        log_warn "磁盘使用率过高: ${disk_usage}% (阈值: ${disk_threshold}%)"
        warnings="${warnings}DISK:${disk_usage}% "
        status=1
    fi
    
    if [[ $status -eq 0 ]]; then
        log_ok "资源状态正常 (CPU: ${cpu_usage}%, MEM: ${mem_usage}%, DISK: ${disk_usage}%)"
    fi
    
    # 输出JSON格式供其他脚本使用
    if [[ "${1:-}" == "--json" ]]; then
        cat <<EOF
{
  "cpu_percent": $cpu_usage,
  "memory_percent": $mem_usage,
  "disk_percent": $disk_usage,
  "cpu_threshold": $cpu_threshold,
  "memory_threshold": $mem_threshold,
  "disk_threshold": $disk_threshold,
  "status": $(if [[ $status -eq 0 ]]; then echo '"ok"'; else echo '"warning"'; fi),
  "warnings": "$warnings"
}
EOF
    fi
    
    return $status
}

# 等待资源恢复
wait_for_resources() {
    local max_wait=${1:-300}  # 默认最多等待5分钟
    local waited=0
    
    log_warn "等待资源恢复... (最多 ${max_wait} 秒)"
    
    while [[ $waited -lt $max_wait ]]; do
        if check_resources > /dev/null 2>&1; then
            log_ok "资源已恢复，继续执行"
            return 0
        fi
        
        sleep 5
        waited=$((waited + 5))
        echo -n "."
    done
    
    log_error "等待超时，资源仍未恢复"
    return 1
}

# 主函数
main() {
    case "${1:-check}" in
        check)
            check_resources "${2:-}"
            exit $?
            ;;
        wait)
            wait_for_resources "${2:-300}"
            exit $?
            ;;
        --help|-h)
            cat <<EOF
Resource Guard - 资源监控脚本

用法: $0 [command] [options]

命令:
  check [--json]     检查资源状态 (默认)
  wait [seconds]     等待资源恢复，默认最多等待300秒
  --help             显示此帮助

返回值:
  0 - 资源正常
  1 - 资源超过阈值

配置文件: configs/monitor_limits.json
EOF
            exit 0
            ;;
        *)
            echo "未知命令: $1"
            echo "使用 --help 查看帮助"
            exit 1
            ;;
    esac
}

main "$@"
