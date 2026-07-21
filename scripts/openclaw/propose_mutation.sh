#!/bin/bash
#
# OpenClaw Mutation Proposal Assistant
# 協助生成 mutation 建議，供 OpenClaw 或人類審查
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

# Default values
AGENT=""
DRY_RUN=false
AUTO_MODE=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --agent)
            AGENT="$2"
            shift 2
            ;;
        --type)
            MUTATION_TYPE="$2"
            shift 2
            ;;
        --auto)
            AUTO_MODE=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --agent ID       Target specific agent (optional)"
            echo "  --type TYPE      Mutation type: prompt_tightening|risk_rule_change|portfolio_constraint_revision"
            echo "  --auto           Auto-generate without interactive prompts"
            echo "  --dry-run        Show proposal without creating files"
            echo "  --help           Show this help"
            echo ""
            echo "Examples:"
            echo "  $0                              # Interactive mode"
            echo "  $0 --agent growth-momentum-01  # Target specific agent"
            echo "  $0 --type risk_rule_change      # Force risk rule mutation"
            echo "  $0 --auto --type risk_rule_change  # Auto with specific type"
            echo "  $0 --auto --dry-run            # Auto-generate and preview"
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
    echo -e "${BLUE}║     OpenClaw Mutation Proposal Assistant       ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════╝${NC}\n"
}

# Check prerequisites
check_prerequisites() {
    if [ ! -f "data/state/recommendation_outcomes.jsonl" ]; then
        echo -e "${RED}Error: No recommendation outcomes found.${NC}"
        echo "Run backtest first:"
        echo "  go run ./cmd/backtest-window -start 2026-03-01 -end 2026-03-30"
        exit 1
    fi

    local latest_window
    # SC2010: pick the most recent matching window via glob + mtime sort,
# instead of `ls -t | grep -v` (broken on non-alpha filenames).
    latest_window=$(
        for f in data/state/windows/window-[0-9]*-[0-9]*.json; do
            [[ "$f" == *-mutation-brief.json ]] && continue
            [[ ! -e "$f" ]] && continue
            echo "$f"
        done | xargs ls -t 2>/dev/null | head -1
    )
    if [ -z "$latest_window" ]; then
        echo -e "${RED}Error: No backtest window summary found.${NC}"
        echo "Run backtest first:"
        echo "  go run ./cmd/backtest-window -start 2026-03-01 -end 2026-03-30"
        exit 1
    fi
}

# Find weakest agent
find_weakest_agent() {
    if [ -n "$AGENT" ]; then
        echo "$AGENT"
        return
    fi

    if [ ! -f "data/state/recommendation_outcomes.jsonl" ]; then
        echo "growth-momentum-01"
        return
    fi

    local weakest
    weakest=$(awk '
    {
        if (match($0, /"agent_id":"[^"]+"/)) {
            agent = substr($0, RSTART+12, RLENGTH-13)
        } else {
            next
        }

        if (match($0, /"forward_return":-?[0-9.]+([eE][-+]?[0-9]+)?/)) {
            val = substr($0, RSTART+17, RLENGTH-17) + 0
        } else {
            next
        }

        n[agent] += 1
        sum[agent] += val
        sumsq[agent] += val*val
    }
    END {
        worstAgent = ""
        worstScore = 1e9
        minObs = 20
        for (a in n) {
            if (n[a] < minObs) {
                continue
            }
            mean = sum[a] / n[a]
            variance = (sumsq[a] / n[a]) - (mean * mean)
            if (variance < 0) {
                variance = 0
            }
            stdev = sqrt(variance)
            sharpeLike = mean / (stdev + 1e-9)

            if (sharpeLike < worstScore) {
                worstScore = sharpeLike
                worstAgent = a
            }
        }

        if (worstAgent != "") {
            print worstAgent
        }
    }
    ' data/state/recommendation_outcomes.jsonl)

    if [ -n "$weakest" ]; then
        echo "$weakest"
    else
        echo "growth-momentum-01"
    fi
}

latest_window_id() {
    local latest_window
    # SC2010: pick the most recent matching window via glob + mtime sort.
    latest_window=$(
        for f in data/state/windows/window-[0-9]*-[0-9]*.json; do
            [[ "$f" == *-mutation-brief.json ]] && continue
            [[ ! -e "$f" ]] && continue
            echo "$f"
        done | xargs ls -t 2>/dev/null | head -1
    )
    if [ -z "$latest_window" ]; then
        echo ""
        return
    fi
    basename "$latest_window" .json
}

observed_windows_for_agent() {
    local target_agent="$1"
    if [ ! -f "data/state/recommendation_outcomes.jsonl" ]; then
        echo 0
        return
    fi

    awk -v agent="$target_agent" '
    {
        id = ""
        win = ""
        if (match($0, /"agent_id":"[^"]+"/)) {
            id = substr($0, RSTART+12, RLENGTH-13)
        }
        if (id != agent) {
            next
        }
        if (match($0, /"window":"[^"]+"/)) {
            win = substr($0, RSTART+9, RLENGTH-10)
        }
        if (win != "") {
            seen[win] = 1
        }
    }
    END {
        c = 0
        for (k in seen) {
            c++
        }
        print c
    }
    ' data/state/recommendation_outcomes.jsonl
}

recent_judged_observations_for_agent_profile() {
    local target_agent="$1"
    local mutation_type="$2"
    local window_id="$3"
    local max_obs=0
    local f

    if [ ! -d "data/state/experiments" ]; then
        echo 0
        return
    fi

    for f in data/state/experiments/*.json; do
        [ -f "$f" ] || continue
        local agent
        agent=$(jq -r '.experiment.target_agent_id // empty' "$f" 2>/dev/null)
        if [ "$agent" != "$target_agent" ]; then
            continue
        fi
        local mtype win
        mtype=$(jq -r '.experiment.mutation_type // .brief.mutation_type // empty' "$f" 2>/dev/null)
        win=$(jq -r '.brief.window_id // empty' "$f" 2>/dev/null)
        if [ -n "$mutation_type" ] && [ "$mtype" != "$mutation_type" ]; then
            continue
        fi
        if [ -n "$window_id" ] && [ "$win" != "$window_id" ]; then
            continue
        fi
        local bobs cobs
        bobs=$(jq -r '.baseline_observations // 0' "$f" 2>/dev/null)
        cobs=$(jq -r '.candidate_observations // 0' "$f" 2>/dev/null)
        if [[ "$bobs" =~ ^[0-9]+$ ]] && [ "$bobs" -gt "$max_obs" ]; then
            max_obs="$bobs"
        fi
        if [[ "$cobs" =~ ^[0-9]+$ ]] && [ "$cobs" -gt "$max_obs" ]; then
            max_obs="$cobs"
        fi
    done

    echo "$max_obs"
}

recent_judged_observations_for_agent_window() {
    local target_agent="$1"
    local window_id="$2"
    local max_obs=0
    local f

    if [ ! -d "data/state/experiments" ]; then
        echo 0
        return
    fi

    for f in data/state/experiments/*.json; do
        [ -f "$f" ] || continue
        local agent win
        agent=$(jq -r '.experiment.target_agent_id // empty' "$f" 2>/dev/null)
        win=$(jq -r '.brief.window_id // empty' "$f" 2>/dev/null)
        if [ "$agent" != "$target_agent" ]; then
            continue
        fi
        if [ -n "$window_id" ] && [ "$win" != "$window_id" ]; then
            continue
        fi
        local bobs cobs
        bobs=$(jq -r '.baseline_observations // 0' "$f" 2>/dev/null)
        cobs=$(jq -r '.candidate_observations // 0' "$f" 2>/dev/null)
        if [[ "$bobs" =~ ^[0-9]+$ ]] && [ "$bobs" -gt "$max_obs" ]; then
            max_obs="$bobs"
        fi
        if [[ "$cobs" =~ ^[0-9]+$ ]] && [ "$cobs" -gt "$max_obs" ]; then
            max_obs="$cobs"
        fi
    done

    echo "$max_obs"
}

# Generate mutation brief
generate_mutation_brief() {
    local target_agent="$1"
    local timestamp=$(date +%s)
    local brief_id="brief-${target_agent}-${timestamp}"
    local mutation_type="${MUTATION_TYPE:-prompt_tightening}"
    local hypothesis="${HYPOTHESIS:-Refine ${target_agent} to improve risk-adjusted recommendation quality}"
    local window_id
    window_id=$(latest_window_id)
    local observed_window_count=1
    local outcomes_window_count=0
    local judged_observation_count=0
    local judged_window_count=0
    outcomes_window_count=$(observed_windows_for_agent "$target_agent")
    if [[ "$outcomes_window_count" =~ ^[0-9]+$ ]] && [ "$outcomes_window_count" -gt "$observed_window_count" ]; then
        observed_window_count="$outcomes_window_count"
    fi
    judged_observation_count=$(recent_judged_observations_for_agent_profile "$target_agent" "$mutation_type" "$window_id")
    if [[ "$judged_observation_count" =~ ^[0-9]+$ ]] && [ "$judged_observation_count" -gt "$observed_window_count" ]; then
        observed_window_count="$judged_observation_count"
    fi
    judged_window_count=$(recent_judged_observations_for_agent_window "$target_agent" "$window_id")
    if [[ "$judged_window_count" =~ ^[0-9]+$ ]] && [ "$judged_window_count" -gt "$observed_window_count" ]; then
        observed_window_count="$judged_window_count"
    fi
    if [ -n "$window_id" ] && [ -f "data/state/windows/${window_id}.json" ]; then
        local detected_count
        detected_count=$(jq -r '.WorstAgentWindowCount // .SessionCount // 1' "data/state/windows/${window_id}.json" 2>/dev/null)
        if [[ "$detected_count" =~ ^[0-9]+$ ]] && [ "$detected_count" -gt "$observed_window_count" ]; then
            observed_window_count="$detected_count"
        fi
    fi
    
    # Dynamic rationale and proposed changes based on type
    case $mutation_type in
        risk_rule_change)
            RATIONALE='[
    "Agent shows inconsistent filtering of weak setups",
    "Conviction thresholds may be too low for current market regime",
    "Risk-adjusted returns suffer from false positive rate"
  ]'
            PROPOSED_CHANGES='[
    "Raise minimum conviction threshold from current level",
    "Add explicit liquidity filtering requirements",
    "Strengthen regime-aware downgrade rules"
  ]'
            ESTIMATED_COMPLEXITY="medium"
            ;;
        portfolio_constraint_revision)
            RATIONALE='[
    "Portfolio shows concentration risk in replay analysis",
    "Drawdown periods correlate with high position sizing",
    "Reserve cash levels may be insufficient for volatile periods"
  ]'
            PROPOSED_CHANGES='[
    "Reduce max position weight to limit concentration",
    "Increase reserve cash fraction for defensive positioning",
    "Add correlation-based position limits"
  ]'
            ESTIMATED_COMPLEXITY="high"
            ;;
        *)
            RATIONALE='[
    "Agent shows lower Sharpe ratio compared to peers",
    "Recommendations may lack sufficient conviction filtering",
    "Prompt could benefit from clearer risk context"
  ]'
            PROPOSED_CHANGES='[
    "Add explicit conviction threshold requirements",
    "Clarify regime-aware downgrade rules",
    "Strengthen liquidity filtering language"
  ]'
            ESTIMATED_COMPLEXITY="low"
            ;;
    esac
    
    echo -e "${CYAN}Analyzing agent: ${target_agent}${NC}\n" >&2
    
    # Read current prompt - get promptFile from configs/agents.json
    local prompt_file=""
    if [ -f "configs/agents.json" ]; then
        prompt_file=$(jq -r ".agents[] | select(.id == \"${target_agent}\") | .promptFile" configs/agents.json 2>/dev/null)
    fi
    
    # Fallback to old derivation method if not found in config
    if [ -z "$prompt_file" ] || [ "$prompt_file" = "null" ]; then
        local agent_base="${target_agent%-*}"  # Remove -01, -02, etc.
        prompt_file="prompts/agents/${agent_base//-/_}.md"
    fi
    
    # Extract target_skill from agent id or use prompt file name
    local target_skill=$(basename "$prompt_file" .md)

    local target_layer="style"
    local acceptance_metric="sharpe_like"
    local acceptance_gates='["improve_sharpe_like","no_material_drawdown_degradation","no_constraint_bypass"]'
    local required_skills="[\"${target_skill}\"]"
    local forbidden_actions='["illiquid_breakout_chasing"]'
    local iteration_guidance='["preserve required skills while tightening setup quality","avoid weakening control-layer constraints"]'
    local maturity_level="level_1_exploratory"
    local recommended_window="next short validation window before broader promotion"
    if [ "$observed_window_count" -ge 12 ]; then
        maturity_level="level_3_regime_aware"
        recommended_window="next cross-regime replay window"
    elif [ "$observed_window_count" -ge 8 ]; then
        maturity_level="level_2_window_validated"
        recommended_window="next multi-session replay window"
    fi
    
    if [ -f "$prompt_file" ]; then
        echo -e "${GREEN}✓ Found prompt file: ${prompt_file}${NC}" >&2
        local current_prompt=$(cat "$prompt_file" | head -20)
        echo "" >&2
        echo "Current prompt (first 20 lines):" >&2
        echo "---" >&2
        echo "$current_prompt" >&2
        echo "---" >&2
    else
        echo -e "${YELLOW}⚠ Prompt file not found: ${prompt_file}${NC}" >&2
    fi
    
    # Generate proposal
    echo "" >&2
    echo -e "${CYAN}Generating mutation proposal (type: ${mutation_type})...${NC}\n" >&2
    
    cat <<EOF
{
  "id": "${brief_id}",
  "window_id": "${window_id}",
  "target_agent": "${target_agent}",
  "target_agent_id": "${target_agent}",
  "target_skill": "${target_skill}",
  "target_layer": "${target_layer}",
  "prompt_file": "${prompt_file}",
  "mutation_type": "${mutation_type}",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "acceptance_metric": "${acceptance_metric}",
  "acceptance_gates": ${acceptance_gates},
  "required_skills": ${required_skills},
  "forbidden_actions": ${forbidden_actions},
  "observed_window_count": ${observed_window_count},
  "maturity_level": "${maturity_level}",
  "iteration_guidance": ${iteration_guidance},
  "generated_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "hypothesis": "${hypothesis}",
  "suggested_mutation_type": "${mutation_type}",
  "rationale": ${RATIONALE},
  "proposed_changes": ${PROPOSED_CHANGES},
  "acceptance_criteria": [
    "Sharpe-like score improvement > 0.001",
    "No material drawdown degradation",
    "Maintains required skills coverage"
  ],
  "estimated_complexity": "${ESTIMATED_COMPLEXITY}",
    "recommended_window": "${recommended_window}",
  "notes_for_reviewer": "This is a ${mutation_type} mutation targeting ${target_agent}"
}
EOF
}

# Interactive mode
interactive_mode() {
    echo ""
    echo "Mutation Proposal Configuration"
    echo "==============================="
    echo ""
    
    if [ -z "$AGENT" ]; then
        echo -n "Target agent (or press Enter for auto-selection): "
        read -r AGENT
    fi
    
    if [ -z "$AGENT" ]; then
        echo -e "${CYAN}Auto-selecting weakest agent...${NC}"
        AGENT=$(find_weakest_agent)
        echo "Selected: ${AGENT}"
    fi
    
    echo ""
    echo "Available mutation types:"
    echo "  1) prompt_tightening         - Conservative prompt refinement (default)"
    echo "  2) risk_rule_change          - Tighten conviction/liquidity thresholds"
    echo "  3) portfolio_constraint_revision - Adjust position sizing/cash reserves"
    echo ""
    
    if [ -z "$MUTATION_TYPE" ]; then
        echo -n "Select mutation type [1/2/3] (default: 1): "
        read -r type_choice
        case $type_choice in
            2) MUTATION_TYPE="risk_rule_change" ;;
            3) MUTATION_TYPE="portfolio_constraint_revision" ;;
            *) MUTATION_TYPE="prompt_tightening" ;;
        esac
    fi
    
    echo "Selected type: ${MUTATION_TYPE}"
    echo ""
    
    # Dynamic hypothesis based on type
    case $MUTATION_TYPE in
        risk_rule_change)
            DEFAULT_HYPOTHESIS="Raise conviction and liquidity thresholds to filter out weak recommendations"
            ;;
        portfolio_constraint_revision)
            DEFAULT_HYPOTHESIS="Reduce position concentration and increase cash reserves for better risk management"
            ;;
        *)
            DEFAULT_HYPOTHESIS="Refine prompt to improve risk-adjusted recommendation quality"
            ;;
    esac
    
    echo -n "Hypothesis: [${DEFAULT_HYPOTHESIS}] "
    read -r hypothesis
    HYPOTHESIS=${hypothesis:-$DEFAULT_HYPOTHESIS}
    
    echo ""
    echo -n "Acceptance window (days): [10] "
    read -r window_days
    window_days=${window_days:-10}
}

# Save brief to file
save_brief() {
    local brief_content="$1"
    local brief_id=$(echo "$brief_content" | grep '"id":' | head -1 | sed 's/.*"id": "\([^"]*\)".*/\1/')
    local output_file="data/state/mutation-briefs/${brief_id}.json"
    
    mkdir -p "data/state/mutation-briefs"
    
    if [ "$DRY_RUN" = true ]; then
        echo -e "\n${YELLOW}=== DRY RUN - Not saving ===${NC}"
        echo "Would save to: ${output_file}"
    else
        echo "$brief_content" > "$output_file"
        echo -e "\n${GREEN}✓ Mutation brief saved to:${NC} ${output_file}"
    fi
}

# Print next steps
print_next_steps() {
    echo ""
    echo -e "${GREEN}Next Steps:${NC}"
    echo "==========="
    echo ""
    
    if [ "$DRY_RUN" = true ]; then
        echo "1. Review the proposal above"
        echo "2. Run without --dry-run to save the brief"
        echo "3. Execute: ./scripts/openclaw/execute_next.sh"
    else
        echo "1. Review the generated brief"
        echo "2. Execute experiment:"
        echo "   ./scripts/openclaw/execute_next.sh"
        echo ""
        echo "Or manually:"
        echo "   go run ./cmd/run-experiment ..."
    fi
}

# Main
main() {
    print_header
    check_prerequisites
    
    if [ "$AUTO_MODE" = false ]; then
        interactive_mode
    else
        if [ -z "$AGENT" ]; then
            AGENT=$(find_weakest_agent)
            echo -e "${CYAN}Auto-selected agent: ${AGENT}${NC}"
        fi
        if [ -z "$MUTATION_TYPE" ]; then
            MUTATION_TYPE="prompt_tightening"
        fi
        if [ -z "$HYPOTHESIS" ]; then
            HYPOTHESIS="Refine ${AGENT} to improve risk-adjusted recommendation quality"
        fi
        echo -e "${CYAN}Auto-selected mutation type: ${MUTATION_TYPE}${NC}"
    fi
    
    echo ""
    local brief=$(generate_mutation_brief "$AGENT")
    
    echo "$brief"
    
    if [ "$DRY_RUN" = false ] && [ "$AUTO_MODE" = false ]; then
        echo ""
        echo -n "Save this brief? [Y/n]: "
        read -r confirm
        if [[ ! "$confirm" =~ ^[Nn]$ ]]; then
            save_brief "$brief"
        else
            echo "Brief not saved."
        fi
    else
        save_brief "$brief"
    fi
    
    print_next_steps
}

main "$@"
