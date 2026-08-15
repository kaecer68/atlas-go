#!/bin/bash
#
# OpenClaw 交互式引導腳本
# 幫助新用戶快速了解系統並執行基本操作
#

set -euo pipefail

# Auto mode detection
AUTO_MODE=false
if [[ "${1:-}" == "--auto" ]]; then
    AUTO_MODE=true
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

print_header() {
    echo -e "${CYAN}"
    echo "╔════════════════════════════════════════════════════════════╗"
    echo "║              OpenClaw 交互式引導嚮導                      ║"
    echo "║         快速了解 atlas-go 系統並執行實驗                   ║"
    echo "╚════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

print_step() {
    echo -e "${BLUE}[Step $1]${NC} $2"
}

print_info() {
    echo -e "${GREEN}ℹ${NC}  $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC}  $1"
}

print_success() {
    echo -e "${GREEN}✓${NC}  $1"
}

ask_yes_no() {
    local question=$1
    local default=${2:-y}
    local response
    
    # Auto mode: always use default
    if [[ "$AUTO_MODE" == true ]]; then
        echo -e "${YELLOW}?${NC} $question [y/n]: $default (auto)"
        [[ "$default" == "y" ]]
        return
    fi
    
    while true; do
        read -r -p "$(echo -e "${YELLOW}?${NC} $question [y/n]: ")" response
        case $response in
            [Yy]*) return 0 ;;
            [Nn]*) return 1 ;;
            *) echo "請回答 y 或 n" ;;
        esac
    done
}

show_system_overview() {
    print_step "1" "系統概覽"
    echo ""
    echo "  OpenClaw 是 atlas-go 的實驗驅動改進系統，用於："
    echo ""
    echo "  • 識別表現最差的 Agent（weak-agent selection）"
    echo "  • 生成針對性的改進建議（mutation brief）"
    echo "  • 執行 A/B 測試比較 baseline 和 candidate"
    echo "  • 自動判斷是否接受改進（judge & promote）"
    echo ""
    echo "  核心流程: propose → execute → judge → promote"
    echo ""
    
    if ask_yes_no "查看當前系統狀態?"; then
        echo ""
        "${SCRIPT_DIR}/status.sh"
        echo ""
    fi
}

show_mutation_guide() {
    print_step "2" "Mutation 類型選擇指南"
    echo ""
    echo "  ┌─────────────────────────────────────────────────────────┐"
    echo "  │ 類型                    │ 改進效果    │ 推薦度         │"
    echo "  ├─────────────────────────────────────────────────────────┤"
    echo "  │ risk_rule_change        │ +40%       │ ★★★★★ 首選   │"
    echo "  │ portfolio_constraint    │ +26%       │ ★★★★☆ 次選   │"
    echo "  │ prompt_tightening       │ ~0%        │ ★☆☆☆☆ 不推薦 │"
    echo "  └─────────────────────────────────────────────────────────┘"
    echo ""
    echo "  建議：新手從 risk_rule_change 開始，效果最顯著"
    echo ""
    
    if ask_yes_no "運行一個 risk_rule_change 實驗?"; then
        echo ""
        print_info "即將運行: ./scripts/openclaw/run_validated_round.sh --type risk_rule_change"
        echo ""
        read -r -p "按 Enter 繼續..."
        "${SCRIPT_DIR}/run_validated_round.sh" --type risk_rule_change
    fi
}

show_quick_commands() {
    print_step "3" "常用快捷命令"
    echo ""
    echo "  一鍵實驗循環："
    echo "    ${CYAN}./scripts/openclaw/run_validated_round.sh${NC}"
    echo ""
    echo "  指定 mutation 類型："
    echo "    ${CYAN}./scripts/openclaw/run_validated_round.sh --type risk_rule_change${NC}"
    echo "    ${CYAN}./scripts/openclaw/run_validated_round.sh --agent growth-momentum-01${NC}"
    echo ""
    echo "  分步執行（手動控制）："
    echo "    ${CYAN}./scripts/openclaw/propose_mutation.sh --auto${NC}    # 生成建議"
    echo "    ${CYAN}./scripts/openclaw/execute_next.sh --auto${NC}        # 執行實驗"
    echo "    ${CYAN}./scripts/openclaw/judge_latest.sh --auto${NC}        # 判斷結果"
    echo ""
    echo "  查看狀態："
    echo "    ${CYAN}./scripts/openclaw/status.sh${NC}                     # 完整狀態"
    echo "    ${CYAN}./scripts/openclaw/revert-baseline --list${NC}        # 版本歷史"
    echo ""
}

show_troubleshooting() {
    print_step "4" "常見問題自助診斷"
    echo ""
    
    # Check for common issues
    local issues_found=0
    
    # Check data directory
    if [ ! -d "${PROJECT_DIR}/data/replay" ]; then
        print_warning "缺少 data/replay 目錄"
        issues_found=$((issues_found + 1))
    fi
    
    # Check for replay data
    local replay_count=$(find "${PROJECT_DIR}/data/replay" -name "*.jsonl" 2>/dev/null | wc -l)
    if [ "$replay_count" -eq 0 ]; then
        print_warning "沒有找到回測數據文件 (*.jsonl)"
        echo "    解決: 運行 go run ./cmd/import-replay 導入數據"
        issues_found=$((issues_found + 1))
    else
        print_success "找到 ${replay_count} 個回測數據文件"
    fi
    
    # Check window
    if [ ! -d "${PROJECT_DIR}/data/state/windows" ] || [ -z "$(ls -A "${PROJECT_DIR}/data/state/windows" 2>/dev/null)" ]; then
        print_warning "缺少回測窗口 (backtest window)"
        echo "    解決: 運行 go run ./cmd/backtest-window -start 2026-01-01 -end 2026-03-31"
        issues_found=$((issues_found + 1))
    else
        local window_count=$(find "${PROJECT_DIR}/data/state/windows" -name "*.json" 2>/dev/null | wc -l)
        print_success "找到 ${window_count} 個回測窗口"
    fi
    
    # Check agents.json
    if [ ! -f "${PROJECT_DIR}/configs/agents.json" ]; then
        print_warning "缺少 configs/agents.json"
        issues_found=$((issues_found + 1))
    else
        print_success "Agent 配置存在"
    fi
    
    # Check prompt files
    local prompt_count=$(find "${PROJECT_DIR}/prompts/agents" -name "*.md" 2>/dev/null | wc -l)
    if [ "$prompt_count" -eq 0 ]; then
        print_warning "沒有找到 prompt 文件"
        issues_found=$((issues_found + 1))
    else
        print_success "找到 ${prompt_count} 個 prompt 文件"
    fi
    
    echo ""
    if [ $issues_found -eq 0 ]; then
        print_success "系統檢查通過，可以開始實驗！"
    else
        print_warning "發現 ${issues_found} 個問題，建議先解決"
    fi
    echo ""
}

show_next_steps() {
    print_step "5" "下一步建議"
    echo ""
    echo "  您現在可以："
    echo ""
    echo "  1. 運行完整實驗循環："
    echo "     ${CYAN}./scripts/openclaw/run_validated_round.sh${NC}"
    echo ""
    echo "  2. 閱讀詳細文檔："
    echo "     • README.md - 系統概覽和快速開始"
    echo "     • agents.md - 實驗執行流程詳解"
    echo "     • docs/skills-map.md - 技能系統說明"
    echo "     • scripts/openclaw/QUICK_REFERENCE.md - 命令速查"
    echo ""
    echo "  3. 查看實驗報告："
    echo "     • .omo/audit/2026-06-15-experiment-baseline-report.md - 基準測試報告（已私有化）"
    echo "     • .omo/audit/2026-06-15-experiment-optimization-supplement.md - 優化補充報告（已私有化）"
    echo ""
}

main() {
    # Check for help
    if [[ "${1:-}" == "--help" ]] || [[ "${1:-}" == "-h" ]]; then
        echo "用法: ./scripts/openclaw/onboard.sh [--auto]"
        echo ""
        echo "選項:"
        echo "  --auto    自動模式，無需人工交互"
        echo "  --help    顯示此幫助信息"
        echo ""
        echo "示例:"
        echo "  ./scripts/openclaw/onboard.sh          # 交互式嚮導"
        echo "  ./scripts/openclaw/onboard.sh --auto   # 自動執行所有檢查"
        exit 0
    fi
    
    print_header
    
    if [[ "$AUTO_MODE" == true ]]; then
        echo -e "${CYAN}[自動模式]${NC} 將自動執行所有步驟，無需人工確認"
        echo ""
    fi
    
    echo -e "${CYAN}歡迎使用 OpenClaw 交互式引導！${NC}"
    echo ""
    echo "本嚮導將幫助您："
    echo "  • 了解系統基本架構"
    echo "  • 選擇最有效的 mutation 策略"
    echo "  • 執行第一個實驗"
    echo "  • 診斷常見問題"
    echo ""
    
    if ! ask_yes_no "開始引導?" "y"; then
        echo ""
        print_info "已取消。您可以隨時運行: ./scripts/openclaw/onboard.sh"
        exit 0
    fi
    
    echo ""
    show_system_overview
    echo ""
    show_mutation_guide
    echo ""
    show_quick_commands
    echo ""
    show_troubleshooting
    echo ""
    show_next_steps
    
    print_header
    echo -e "${GREEN}引導完成！${NC}"
    echo ""
    echo "隨時運行 ./scripts/openclaw/onboard.sh 重新查看本引導"
    echo ""
}

# Run main function
main "$@"
