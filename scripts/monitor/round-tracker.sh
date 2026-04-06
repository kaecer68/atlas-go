#!/bin/bash
#
# Round Tracker - 实验轮次追踪与停止管理
# 记录轮次历史，判断是否达到停止条件
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
STATE_DIR="${PROJECT_DIR}/data/state"
TRACKER_FILE="${STATE_DIR}/round-tracker.jsonl"
CONFIG_FILE="${PROJECT_DIR}/configs/monitor-limits.json"

# 颜色输出
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

# 默认停止条件（平衡配置）
DEFAULT_MAX_ROUNDS=20
DEFAULT_CONSECUTIVE_REJECTS=3
DEFAULT_MIN_ACCEPTANCE_RATE=0.15

ensure_state_dir() {
    mkdir -p "$STATE_DIR"
}

# 读取配置
load_config() {
    local max_rounds=$DEFAULT_MAX_ROUNDS
    local consecutive=$DEFAULT_CONSECUTIVE_REJECTS
    local min_rate=$DEFAULT_MIN_ACCEPTANCE_RATE
    
    if [[ -f "$CONFIG_FILE" ]]; then
        if command -v jq &> /dev/null; then
            max_rounds=$(jq -r '.stop_conditions.max_total_rounds // empty' "$CONFIG_FILE" 2>/dev/null || echo "$DEFAULT_MAX_ROUNDS")
            consecutive=$(jq -r '.stop_conditions.consecutive_rejects // empty' "$CONFIG_FILE" 2>/dev/null || echo "$DEFAULT_CONSECUTIVE_REJECTS")
            min_rate=$(jq -r '.stop_conditions.min_acceptance_rate // empty' "$CONFIG_FILE" 2>/dev/null || echo "$DEFAULT_MIN_ACCEPTANCE_RATE")
        fi
    fi
    
    echo "${max_rounds:-$DEFAULT_MAX_ROUNDS} ${consecutive:-$DEFAULT_CONSECUTIVE_REJECTS} ${min_rate:-$DEFAULT_MIN_ACCEPTANCE_RATE}"
}

# 获取当前轮次编号
get_current_round() {
    if [[ -f "$TRACKER_FILE" ]]; then
        wc -l < "$TRACKER_FILE" | tr -d ' '
    else
        echo "0"
    fi
}

# 记录轮次结果
record_round() {
    local round=$1
    local result=$2      # accepted | rejected | error
    local experiment_id=${3:-""}
    local agent=${4:-""}
    local mutation_type=${5:-""}
    local improvement=${6:-"0"}
    
    ensure_state_dir
    
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    local entry=$(cat <<EOF
{"round":$round,"timestamp":"$timestamp","result":"$result","experiment_id":"$experiment_id","agent":"$agent","mutation_type":"$mutation_type","improvement":$improvement}
EOF
)
    
    echo "$entry" >> "$TRACKER_FILE"
    echo -e "${BLUE}[RoundTracker]${NC} 记录轮次 $round: $result"
}

# 检查停止条件
check_stop_conditions() {
    local config=$(load_config)
    local max_rounds=$(echo "$config" | awk '{print $1}')
    local consecutive_limit=$(echo "$config" | awk '{print $2}')
    local min_rate=$(echo "$config" | awk '{print $3}')
    
    local current_round=$(get_current_round)
    local reasons=""
    local should_stop=0
    
    # 检查1: 总轮次上限
    if [[ $current_round -ge $max_rounds ]]; then
        reasons="${reasons}达到最大轮次限制($max_rounds); "
        should_stop=1
    fi
    
    # 检查2: 连续拒绝次数
    if [[ -f "$TRACKER_FILE" ]] && [[ $current_round -gt 0 ]]; then
        local recent=$(tail -n $consecutive_limit "$TRACKER_FILE" 2>/dev/null | grep -c '"result":"rejected"' || echo "0")
        if [[ $recent -ge $consecutive_limit ]]; then
            reasons="${reasons}连续${consecutive_limit}轮被拒绝; "
            should_stop=1
        fi
    fi
    
    # 检查3: 接受率过低（至少5轮后检查）
    if [[ -f "$TRACKER_FILE" ]] && [[ $current_round -ge 5 ]]; then
        local total=$(wc -l < "$TRACKER_FILE" | tr -d ' ')
        local accepted=$(grep -c '"result":"accepted"' "$TRACKER_FILE" 2>/dev/null || echo "0")
        if [[ $total -gt 0 ]]; then
            local rate=$(echo "scale=2; $accepted / $total" | bc)
            local rate_percent=$(echo "scale=0; $rate * 100" | bc)
            local min_percent=$(echo "scale=0; $min_rate * 100" | bc)
            
            if [[ $rate_percent -lt $min_percent ]]; then
                reasons="${reasons}接受率过低(${rate_percent}% < ${min_percent}%); "
                should_stop=1
            fi
        fi
    fi
    
    # 检查4: 所有agent已优化完成
    local optimized_agents=$(grep '"result":"accepted"' "$TRACKER_FILE" 2>/dev/null | jq -s '[.[] | .agent] | unique | length' 2>/dev/null || echo "0")
    if [[ $optimized_agents -ge 7 ]]; then  # 假设7个agents
        reasons="${reasons}所有agents已完成优化($optimized_agents/7); "
        should_stop=1
    fi
    
    # 输出结果
    if [[ $should_stop -eq 1 ]]; then
        echo -e "${YELLOW}[RoundTracker]${NC} 建议停止: $reasons"
        echo "{\"should_stop\":true,\"current_round\":$current_round,\"reasons\":\"$reasons\"}"
        return 1
    else
        echo -e "${GREEN}[RoundTracker]${NC} 轮次 $current_round/$max_rounds, 继续执行"
        echo "{\"should_stop\":false,\"current_round\":$current_round,\"max_rounds\":$max_rounds}"
        return 0
    fi
}

# 获取统计信息
get_stats() {
    if [[ ! -f "$TRACKER_FILE" ]]; then
        echo "暂无轮次记录"
        return 0
    fi
    
    local total=$(wc -l < "$TRACKER_FILE" | tr -d ' ')
    local accepted=$(grep -c '"result":"accepted"' "$TRACKER_FILE" 2>/dev/null || echo "0")
    local rejected=$(grep -c '"result":"rejected"' "$TRACKER_FILE" 2>/dev/null || echo "0")
    local errors=$(grep -c '"result":"error"' "$TRACKER_FILE" 2>/dev/null || echo "0")
    
    echo "========== 轮次统计 =========="
    echo "总轮次: $total"
    echo "接受: $accepted ($(echo "scale=1; $accepted * 100 / $total" | bc)%)"
    echo "拒绝: $rejected ($(echo "scale=1; $rejected * 100 / $total" | bc)%)"
    echo "错误: $errors ($(echo "scale=1; $errors * 100 / $total" | bc)%)"
    echo "=============================="
    
    # 按agent统计
    echo ""
    echo "按Agent统计:"
    grep '"result":"accepted"' "$TRACKER_FILE" 2>/dev/null | \
        jq -s 'group_by(.agent) | map({agent: .[0].agent, count: length}) | sort_by(-.count) | .[] | "  \(.agent): \(.count) 次改进"' 2>/dev/null || echo "  暂无接受记录"
}

# 重置追踪器
reset_tracker() {
    if [[ -f "$TRACKER_FILE" ]]; then
        local backup="${TRACKER_FILE}.backup.$(date +%Y%m%d%H%M%S)"
        mv "$TRACKER_FILE" "$backup"
        echo -e "${BLUE}[RoundTracker]${NC} 已重置追踪器 (备份: $backup)"
    else
        echo -e "${BLUE}[RoundTracker]${NC} 无追踪器需要重置"
    fi
}

# 主函数
main() {
    case "${1:-check}" in
        record)
            record_round "${2:-}" "${3:-}" "${4:-}" "${5:-}" "${6:-}" "${7:-0}"
            ;;
        check)
            check_stop_conditions
            exit $?
            ;;
        stats)
            get_stats
            ;;
        reset)
            reset_tracker
            ;;
        --help|-h)
            cat <<EOF
Round Tracker - 实验轮次追踪

用法: $0 [command] [options]

命令:
  record <round> <result> [exp_id] [agent] [type] [improvement]
                     记录轮次结果
  check              检查是否应停止 (默认)
  stats              显示统计信息
  reset              重置追踪器 (自动备份)
  --help             显示此帮助

停止条件 (可在 configs/monitor-limits.json 配置):
  - 达到最大轮次 (默认: 20)
  - 连续拒绝次数 (默认: 3)
  - 接受率过低 (默认: <15%)
  - 所有agents优化完成 (7/7)

返回值:
  0 - 继续执行
  1 - 建议停止
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
