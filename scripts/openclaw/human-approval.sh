#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "$PROJECT_ROOT"

ACTION=""
EXPERIMENT_ID=""
REASON=""
REVERT_TARGET=""
DRY_RUN=false
AUTO_CONFIRM=false
ACTOR="${USER:-unknown}"
JSON_MODE=false

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

usage() {
  cat <<'EOF'
Usage: ./scripts/openclaw/human-approval.sh [OPTIONS]

Human-in-the-loop decision wrapper for experiment approval/rejection/revert.

Options:
  --approve              Approve and promote an experiment
  --reject               Reject an experiment (audit only, no promotion)
  --revert [N]           Revert baseline (optional version N)
  --experiment ID        Target experiment ID (default: latest from judge-latest)
  --reason TEXT          Required reason for audit trail
  --actor NAME           Decision actor (default: $USER)
  --dry-run              Preview without mutating state
  --yes                  Auto-confirm underlying decide.sh actions
  --json                 Print decision event JSON
  --help                 Show this help

Examples:
  ./scripts/openclaw/human-approval.sh --approve --experiment exp-123 --reason "Passes gates"
  ./scripts/openclaw/human-approval.sh --reject --reason "Insufficient evidence"
  ./scripts/openclaw/human-approval.sh --revert --reason "Rollback after monitoring alert"
EOF
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo -e "${RED}Error:${NC} missing command: $cmd"
    exit 1
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --approve)
      ACTION="approve"
      shift
      ;;
    --reject)
      ACTION="reject"
      shift
      ;;
    --revert)
      ACTION="revert"
      if [[ $# -gt 1 && "$2" != --* ]]; then
        REVERT_TARGET="$2"
        shift 2
      else
        shift
      fi
      ;;
    --experiment)
      EXPERIMENT_ID="$2"
      shift 2
      ;;
    --reason)
      REASON="$2"
      shift 2
      ;;
    --actor)
      ACTOR="$2"
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
    --json)
      JSON_MODE=true
      shift
      ;;
    --help)
      usage
      exit 0
      ;;
    *)
      echo -e "${RED}Error:${NC} unknown option $1"
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$ACTION" ]]; then
  echo -e "${RED}Error:${NC} must choose one action: --approve | --reject | --revert"
  exit 1
fi

if [[ -z "$REASON" ]]; then
  echo -e "${RED}Error:${NC} --reason is required"
  exit 1
fi

require_cmd jq

resolve_latest_experiment() {
  local judge_json
  judge_json="$(./scripts/openclaw/judge-latest.sh --auto --json 2>/dev/null || true)"
  if [[ -z "$judge_json" ]]; then
    echo ""
    return 0
  fi
  echo "$judge_json" | jq -r '.experiment_id // empty' 2>/dev/null || true
}

if [[ "$ACTION" != "revert" && -z "$EXPERIMENT_ID" ]]; then
  EXPERIMENT_ID="$(resolve_latest_experiment)"
  if [[ -z "$EXPERIMENT_ID" ]]; then
    echo -e "${RED}Error:${NC} cannot infer latest experiment id; pass --experiment"
    exit 1
  fi
fi

EXP_STATUS=""
if [[ -n "$EXPERIMENT_ID" ]]; then
  RESULT_FILE="data/state/experiments/${EXPERIMENT_ID}.json"
  if [[ -f "$RESULT_FILE" ]]; then
    EXP_STATUS="$(jq -r '.experiment.status // empty' "$RESULT_FILE" 2>/dev/null || true)"
  fi
fi

if [[ "$ACTION" == "approve" && -n "$EXP_STATUS" && "$EXP_STATUS" != "accepted" ]]; then
  echo -e "${YELLOW}Warning:${NC} experiment status is '${EXP_STATUS}', not 'accepted'."
  echo -e "${YELLOW}Warning:${NC} approval will continue only if you explicitly run with --yes and reason justifies override."
  if [[ "$AUTO_CONFIRM" != true ]]; then
    echo "Approval blocked without --yes for non-accepted experiment."
    exit 1
  fi
fi

APPROVAL_DIR="data/state/approvals"
mkdir -p "$APPROVAL_DIR"
DECISION_ID="decision-$(date +%Y%m%d%H%M%S)"
EVENT_FILE="$APPROVAL_DIR/${DECISION_ID}.json"
TIMESTAMP="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

jq -n \
  --arg decision_id "$DECISION_ID" \
  --arg timestamp "$TIMESTAMP" \
  --arg actor "$ACTOR" \
  --arg action "$ACTION" \
  --arg experiment_id "$EXPERIMENT_ID" \
  --arg experiment_status "$EXP_STATUS" \
  --arg reason "$REASON" \
  --arg revert_target "$REVERT_TARGET" \
  --argjson dry_run "$DRY_RUN" \
  '{
    decision_id: $decision_id,
    timestamp: $timestamp,
    actor: $actor,
    action: $action,
    experiment_id: $experiment_id,
    experiment_status: $experiment_status,
    reason: $reason,
    revert_target: $revert_target,
    dry_run: $dry_run
  }' > "$EVENT_FILE"

run_decide() {
  local -a cmd
  cmd=(./scripts/openclaw/decide.sh)
  if [[ "$ACTION" == "approve" ]]; then
    cmd+=(--promote "$EXPERIMENT_ID" --reason "$REASON")
  elif [[ "$ACTION" == "revert" ]]; then
    cmd+=(--revert)
    if [[ -n "$REVERT_TARGET" ]]; then
      cmd+=("$REVERT_TARGET")
    fi
    cmd+=(--reason "$REASON")
  else
    return 0
  fi

  if [[ "$DRY_RUN" == true ]]; then
    cmd+=(--dry-run)
  fi
  if [[ "$AUTO_CONFIRM" == true ]]; then
    cmd+=(--yes)
  fi

  "${cmd[@]}"
}

if [[ "$JSON_MODE" == true ]]; then
  cat "$EVENT_FILE"
else
  echo -e "${CYAN}[human-approval]${NC} decision logged: $EVENT_FILE"
  echo "  action: $ACTION"
  if [[ -n "$EXPERIMENT_ID" ]]; then
    echo "  experiment: $EXPERIMENT_ID"
  fi
  if [[ -n "$EXP_STATUS" ]]; then
    echo "  status: $EXP_STATUS"
  fi
fi

if [[ "$ACTION" == "reject" ]]; then
  if [[ "$JSON_MODE" == false ]]; then
    echo -e "${GREEN}[human-approval]${NC} reject recorded (no promotion executed)."
  fi
  exit 0
fi

run_decide

if [[ "$JSON_MODE" == false ]]; then
  echo -e "${GREEN}[human-approval]${NC} completed: $ACTION"
fi
