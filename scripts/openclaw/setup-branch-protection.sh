#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "$PROJECT_ROOT"

OWNER=""
REPO=""
BRANCH="main"
DRY_RUN=true
INTERACTIVE=true
PROFILE="recommended"
YES=false

CHECKS="ci / governance,ci / operations"
STRICT_UP_TO_DATE=true
REQUIRE_CONVERSATION_RESOLUTION=true
ENFORCE_ADMINS=false
REQUIRED_REVIEWS=1
DISMISS_STALE_REVIEWS=true
REQUIRE_CODE_OWNER_REVIEWS=false
BACKUP_DIR="data/state/branch-protection-snapshots"
RESTORE_FROM=""

CURRENT_JSON=""
PROPOSED_JSON=""
SNAPSHOT_JSON=""

CHECKS_SET=false
STRICT_UP_TO_DATE_SET=false
REQUIRE_CONVERSATION_RESOLUTION_SET=false
ENFORCE_ADMINS_SET=false
REQUIRED_REVIEWS_SET=false
DISMISS_STALE_REVIEWS_SET=false
REQUIRE_CODE_OWNER_REVIEWS_SET=false

usage() {
  cat <<'EOF'
Usage: ./scripts/openclaw/setup-branch-protection.sh [OPTIONS]

Automate branch protection setup with guided human approval.
Default mode is DRY RUN (no changes).

Options:
  --owner <owner>              GitHub owner (default: infer from origin remote)
  --repo <repo>                GitHub repository name (default: infer from origin remote)
  --branch <name>              Branch to protect (default: main)
  --profile <name>             recommended|strict|relaxed (default: recommended)
  --checks <a,b,c>             Required status checks (default: "ci / governance,ci / operations")
  --required-reviews <n>       Required approving reviews, 1..6 (default: 1)
  --enforce-admins <bool>      true|false (default: false)
  --require-conversation <bool> true|false (default: true)
  --strict-up-to-date <bool>   true|false (default: true)
  --dismiss-stale <bool>       true|false (default: true)
  --require-codeowners <bool>  true|false (default: false)
  --backup-dir <path>          Snapshot output dir before apply (default: data/state/branch-protection-snapshots)
  --restore-from <path>        Restore protection payload from snapshot JSON
  --apply                      Apply changes (otherwise dry-run)
  --non-interactive            Do not prompt; use provided options
  --yes                        Skip final confirmation prompt (only with --non-interactive --apply)
  --help                       Show help

Examples:
  ./scripts/openclaw/setup-branch-protection.sh
  ./scripts/openclaw/setup-branch-protection.sh --apply
  ./scripts/openclaw/setup-branch-protection.sh --profile strict --apply
  ./scripts/openclaw/setup-branch-protection.sh --non-interactive --apply --yes
  ./scripts/openclaw/setup-branch-protection.sh --restore-from data/state/branch-protection-snapshots/<file>.json --apply
EOF
}

print_section() {
  echo
  echo "== $1 =="
}

die() {
  echo "[error] $1" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

normalize_bool() {
  case "$1" in
    true|TRUE|True|1|yes|YES|y|Y) echo "true" ;;
    false|FALSE|False|0|no|NO|n|N) echo "false" ;;
    *) die "invalid boolean value: $1" ;;
  esac
}

infer_repo_from_git_remote() {
  local remote
  remote="$(git remote get-url origin 2>/dev/null || true)"
  [[ -n "$remote" ]] || die "cannot infer owner/repo from origin remote; use --owner and --repo"

  # Supports:
  # - git@github.com:owner/repo.git
  # - https://github.com/owner/repo.git
  # - https://github.com/owner/repo
  if [[ "$remote" =~ github.com[:/]([^/]+)/([^/.]+)(\.git)?$ ]]; then
    OWNER="${BASH_REMATCH[1]}"
    REPO="${BASH_REMATCH[2]}"
  else
    die "unsupported remote format: $remote"
  fi
}

parse_checks_csv() {
  local input="$1"
  local cleaned
  cleaned="$(echo "$input" | tr ',' '\n' | sed 's/^ *//; s/ *$//' | sed '/^$/d')"
  [[ -n "$cleaned" ]] || die "required checks cannot be empty"
  echo "$cleaned"
}

apply_profile_defaults() {
  case "$PROFILE" in
    recommended)
      if [[ "$CHECKS_SET" != true ]]; then CHECKS="ci / governance,ci / operations"; fi
      if [[ "$STRICT_UP_TO_DATE_SET" != true ]]; then STRICT_UP_TO_DATE=true; fi
      if [[ "$REQUIRE_CONVERSATION_RESOLUTION_SET" != true ]]; then REQUIRE_CONVERSATION_RESOLUTION=true; fi
      if [[ "$ENFORCE_ADMINS_SET" != true ]]; then ENFORCE_ADMINS=false; fi
      if [[ "$REQUIRED_REVIEWS_SET" != true ]]; then REQUIRED_REVIEWS=1; fi
      if [[ "$DISMISS_STALE_REVIEWS_SET" != true ]]; then DISMISS_STALE_REVIEWS=true; fi
      if [[ "$REQUIRE_CODE_OWNER_REVIEWS_SET" != true ]]; then REQUIRE_CODE_OWNER_REVIEWS=false; fi
      ;;
    strict)
      if [[ "$CHECKS_SET" != true ]]; then CHECKS="ci / governance,ci / operations"; fi
      if [[ "$STRICT_UP_TO_DATE_SET" != true ]]; then STRICT_UP_TO_DATE=true; fi
      if [[ "$REQUIRE_CONVERSATION_RESOLUTION_SET" != true ]]; then REQUIRE_CONVERSATION_RESOLUTION=true; fi
      if [[ "$ENFORCE_ADMINS_SET" != true ]]; then ENFORCE_ADMINS=true; fi
      if [[ "$REQUIRED_REVIEWS_SET" != true ]]; then REQUIRED_REVIEWS=2; fi
      if [[ "$DISMISS_STALE_REVIEWS_SET" != true ]]; then DISMISS_STALE_REVIEWS=true; fi
      if [[ "$REQUIRE_CODE_OWNER_REVIEWS_SET" != true ]]; then REQUIRE_CODE_OWNER_REVIEWS=true; fi
      ;;
    relaxed)
      if [[ "$CHECKS_SET" != true ]]; then CHECKS="ci / governance,ci / operations"; fi
      if [[ "$STRICT_UP_TO_DATE_SET" != true ]]; then STRICT_UP_TO_DATE=false; fi
      if [[ "$REQUIRE_CONVERSATION_RESOLUTION_SET" != true ]]; then REQUIRE_CONVERSATION_RESOLUTION=false; fi
      if [[ "$ENFORCE_ADMINS_SET" != true ]]; then ENFORCE_ADMINS=false; fi
      if [[ "$REQUIRED_REVIEWS_SET" != true ]]; then REQUIRED_REVIEWS=1; fi
      if [[ "$DISMISS_STALE_REVIEWS_SET" != true ]]; then DISMISS_STALE_REVIEWS=false; fi
      if [[ "$REQUIRE_CODE_OWNER_REVIEWS_SET" != true ]]; then REQUIRE_CODE_OWNER_REVIEWS=false; fi
      ;;
    *)
      die "unknown profile: $PROFILE"
      ;;
  esac
}

read_current_protection() {
  local tmp
  tmp="$(mktemp)"
  if gh api "repos/${OWNER}/${REPO}/branches/${BRANCH}/protection" >"$tmp" 2>/dev/null; then
    CURRENT_JSON="$tmp"
  else
    CURRENT_JSON=""
    rm -f "$tmp"
  fi
}

print_current_summary() {
  print_section "Current Configuration"
  if [[ -z "$CURRENT_JSON" ]]; then
    echo "No existing branch protection found on ${OWNER}/${REPO}:${BRANCH}."
    return
  fi

  jq -r '
    "Branch: " + ($ENV.BRANCH) + "\n" +
    "Require up-to-date: " + ((.required_status_checks.strict // false)|tostring) + "\n" +
    "Required checks: " + ((.required_status_checks.contexts // []) | join(", ")) + "\n" +
    "Required reviews: " + ((.required_pull_request_reviews.required_approving_review_count // 0)|tostring) + "\n" +
    "Dismiss stale reviews: " + ((.required_pull_request_reviews.dismiss_stale_reviews // false)|tostring) + "\n" +
    "Require code owner reviews: " + ((.required_pull_request_reviews.require_code_owner_reviews // false)|tostring) + "\n" +
    "Require conversation resolution: " + ((.required_conversation_resolution.enabled // false)|tostring) + "\n" +
    "Enforce admins: " + ((.enforce_admins.enabled // false)|tostring)
  ' "$CURRENT_JSON"
}

prompt_bool() {
  local label="$1"
  local default="$2"
  local raw
  while true; do
    read -r -p "$label [${default}]: " raw
    raw="${raw:-$default}"
    if v="$(normalize_bool "$raw" 2>/dev/null)"; then
      echo "$v"
      return
    fi
    echo "Please enter true/false (or yes/no)."
  done
}

interactive_customize() {
  if [[ -n "$RESTORE_FROM" ]]; then
    return
  fi

  print_section "Profile Selection"
  echo "1) recommended - balanced governance (default)"
  echo "2) strict      - stronger controls, higher merge friction"
  echo "3) relaxed     - faster merge, higher governance risk"
  echo "4) custom      - guided custom configuration"

  local choice
  read -r -p "Select profile [1]: " choice
  choice="${choice:-1}"

  case "$choice" in
    1) PROFILE="recommended" ; apply_profile_defaults ;;
    2) PROFILE="strict" ; apply_profile_defaults ;;
    3) PROFILE="relaxed" ; apply_profile_defaults ;;
    4)
      PROFILE="recommended"
      apply_profile_defaults
      read -r -p "Required checks (comma-separated) [${CHECKS}]: " checks_in
      if [[ -n "${checks_in:-}" ]]; then
        CHECKS="$checks_in"
      fi
      read -r -p "Required approving reviews (1..6) [${REQUIRED_REVIEWS}]: " rev_in
      if [[ -n "${rev_in:-}" ]]; then
        REQUIRED_REVIEWS="$rev_in"
      fi
      STRICT_UP_TO_DATE="$(prompt_bool "Require branches up to date before merge" "$STRICT_UP_TO_DATE")"
      REQUIRE_CONVERSATION_RESOLUTION="$(prompt_bool "Require conversation resolution" "$REQUIRE_CONVERSATION_RESOLUTION")"
      ENFORCE_ADMINS="$(prompt_bool "Enforce for administrators" "$ENFORCE_ADMINS")"
      DISMISS_STALE_REVIEWS="$(prompt_bool "Dismiss stale reviews on new push" "$DISMISS_STALE_REVIEWS")"
      REQUIRE_CODE_OWNER_REVIEWS="$(prompt_bool "Require code owner reviews" "$REQUIRE_CODE_OWNER_REVIEWS")"
      ;;
    *) die "invalid profile choice: $choice" ;;
  esac
}

load_restore_payload() {
  [[ -n "$RESTORE_FROM" ]] || return
  [[ -f "$RESTORE_FROM" ]] || die "restore snapshot not found: $RESTORE_FROM"

  local s_owner s_repo s_branch
  s_owner="$(jq -r '.owner // empty' "$RESTORE_FROM")"
  s_repo="$(jq -r '.repo // empty' "$RESTORE_FROM")"
  s_branch="$(jq -r '.branch // empty' "$RESTORE_FROM")"

  [[ -n "$s_owner" && -n "$s_repo" && -n "$s_branch" ]] || die "invalid snapshot: missing owner/repo/branch"

  if [[ -z "$OWNER" ]]; then
    OWNER="$s_owner"
  fi
  if [[ -z "$REPO" ]]; then
    REPO="$s_repo"
  fi
  if [[ -z "$BRANCH" ]]; then
    BRANCH="$s_branch"
  fi

  [[ "$OWNER" == "$s_owner" ]] || die "snapshot owner mismatch: expected $OWNER, got $s_owner"
  [[ "$REPO" == "$s_repo" ]] || die "snapshot repo mismatch: expected $REPO, got $s_repo"
  [[ "$BRANCH" == "$s_branch" ]] || die "snapshot branch mismatch: expected $BRANCH, got $s_branch"

  jq -e '.protection_exists == true and (.protection | type == "object")' "$RESTORE_FROM" >/dev/null || die "snapshot does not contain restorable protection payload"

  PROPOSED_JSON="$(mktemp)"
  jq '.protection' "$RESTORE_FROM" > "$PROPOSED_JSON"
}

validate_inputs() {
  [[ "$REQUIRED_REVIEWS" =~ ^[0-9]+$ ]] || die "required reviews must be integer"
  if (( REQUIRED_REVIEWS < 1 || REQUIRED_REVIEWS > 6 )); then
    die "required reviews out of range (1..6): $REQUIRED_REVIEWS"
  fi

  STRICT_UP_TO_DATE="$(normalize_bool "$STRICT_UP_TO_DATE")"
  REQUIRE_CONVERSATION_RESOLUTION="$(normalize_bool "$REQUIRE_CONVERSATION_RESOLUTION")"
  ENFORCE_ADMINS="$(normalize_bool "$ENFORCE_ADMINS")"
  DISMISS_STALE_REVIEWS="$(normalize_bool "$DISMISS_STALE_REVIEWS")"
  REQUIRE_CODE_OWNER_REVIEWS="$(normalize_bool "$REQUIRE_CODE_OWNER_REVIEWS")"

  local lines
  lines="$(parse_checks_csv "$CHECKS")"
  CHECKS="$(echo "$lines" | paste -sd ',' -)"
}

build_proposed_payload() {
  if [[ -n "$RESTORE_FROM" ]]; then
    load_restore_payload
    return
  fi

  local checks_json
  checks_json="$(parse_checks_csv "$CHECKS" | jq -R . | jq -s .)"

  PROPOSED_JSON="$(mktemp)"
  jq -n \
    --argjson checks "$checks_json" \
    --argjson strict "$STRICT_UP_TO_DATE" \
    --argjson enforce_admins "$ENFORCE_ADMINS" \
    --argjson required_reviews "$REQUIRED_REVIEWS" \
    --argjson dismiss_stale "$DISMISS_STALE_REVIEWS" \
    --argjson require_codeowners "$REQUIRE_CODE_OWNER_REVIEWS" \
    --argjson require_conversation "$REQUIRE_CONVERSATION_RESOLUTION" \
    '{
      required_status_checks: {
        strict: $strict,
        contexts: $checks
      },
      enforce_admins: $enforce_admins,
      required_pull_request_reviews: {
        dismiss_stale_reviews: $dismiss_stale,
        require_code_owner_reviews: $require_codeowners,
        required_approving_review_count: $required_reviews
      },
      restrictions: null,
      required_conversation_resolution: $require_conversation
    }' > "$PROPOSED_JSON"
}

print_risk_guidance() {
  print_section "Options and Risk Guidance"
  if [[ -n "$RESTORE_FROM" ]]; then
    echo "Restore mode: true"
    echo "Restore source: $RESTORE_FROM"
    echo
    echo "Potential risks by option:"
    echo "- restoring old snapshot may overwrite newer governance settings."
    echo "- restored required checks may not exist in current workflow names."
    echo "- restored review settings may increase or decrease merge friction unexpectedly."
    return
  fi

  echo "Selected profile: $PROFILE"
  echo "Required checks: $CHECKS"
  echo "Required reviews: $REQUIRED_REVIEWS"
  echo "Require up-to-date: $STRICT_UP_TO_DATE"
  echo "Require conversation resolution: $REQUIRE_CONVERSATION_RESOLUTION"
  echo "Enforce admins: $ENFORCE_ADMINS"
  echo "Dismiss stale reviews: $DISMISS_STALE_REVIEWS"
  echo "Require code owner reviews: $REQUIRE_CODE_OWNER_REVIEWS"

  echo
  echo "Potential risks by option:"
  [[ "$STRICT_UP_TO_DATE" == "false" ]] && echo "- strict-up-to-date=false: stale branches may merge without latest checks."
  [[ "$REQUIRE_CONVERSATION_RESOLUTION" == "false" ]] && echo "- conversation-resolution=false: unresolved review threads can be merged."
  [[ "$ENFORCE_ADMINS" == "false" ]] && echo "- enforce-admins=false: admins can bypass branch rules."
  [[ "$DISMISS_STALE_REVIEWS" == "false" ]] && echo "- dismiss-stale=false: old approvals remain valid after new pushes."
  [[ "$REQUIRE_CODE_OWNER_REVIEWS" == "false" ]] && echo "- require-codeowners=false: domain owners may be skipped in review."
  if ! grep -q "ci / governance" <<<"$CHECKS"; then
    echo "- missing ci / governance: governance regressions may merge."
  fi
  if ! grep -q "ci / operations" <<<"$CHECKS"; then
    echo "- missing ci / operations: operations readiness regressions may merge."
  fi
}

print_diff_hint() {
  print_section "Proposed Payload"
  cat "$PROPOSED_JSON" | jq .
}

create_snapshot_backup() {
  if [[ "$DRY_RUN" == "true" ]]; then
    return
  fi

  mkdir -p "$BACKUP_DIR"
  local ts
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  SNAPSHOT_JSON="${BACKUP_DIR}/${OWNER}-${REPO}-${BRANCH}-${ts}.json"

  if [[ -n "$CURRENT_JSON" && -f "$CURRENT_JSON" ]]; then
    jq -n \
      --arg generated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      --arg owner "$OWNER" \
      --arg repo "$REPO" \
      --arg branch "$BRANCH" \
      --arg backup_type "pre-apply" \
      --slurpfile protection "$CURRENT_JSON" \
      '{
        generated_at: $generated_at,
        owner: $owner,
        repo: $repo,
        branch: $branch,
        backup_type: $backup_type,
        protection_exists: true,
        protection: $protection[0]
      }' > "$SNAPSHOT_JSON"
  else
    jq -n \
      --arg generated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      --arg owner "$OWNER" \
      --arg repo "$REPO" \
      --arg branch "$BRANCH" \
      --arg backup_type "pre-apply" \
      '{
        generated_at: $generated_at,
        owner: $owner,
        repo: $repo,
        branch: $branch,
        backup_type: $backup_type,
        protection_exists: false,
        protection: null
      }' > "$SNAPSHOT_JSON"
  fi

  print_section "Snapshot Backup"
  echo "Saved current configuration snapshot to: $SNAPSHOT_JSON"
}

confirm_apply() {
  if [[ "$DRY_RUN" == "true" ]]; then
    return
  fi

  if [[ "$INTERACTIVE" == "true" ]]; then
    local phrase expected
    if [[ -n "$RESTORE_FROM" ]]; then
      expected="RESTORE ${OWNER}/${REPO}:${BRANCH}"
    else
      expected="APPLY ${OWNER}/${REPO}:${BRANCH}"
    fi
    echo
    echo "Type this exact phrase to apply changes:"
    echo "  $expected"
    read -r -p "> " phrase
    [[ "$phrase" == "$expected" ]] || die "confirmation phrase mismatch; aborted"
  else
    [[ "$YES" == "true" ]] || die "non-interactive apply requires --yes"
  fi
}

apply_changes() {
  if [[ "$DRY_RUN" == "true" ]]; then
    print_section "Dry Run"
    echo "No changes were applied."
    echo "To apply, run with: --apply"
    return
  fi

  print_section "Applying Branch Protection"
  create_snapshot_backup
  gh api \
    -X PUT \
    -H "Accept: application/vnd.github+json" \
    "repos/${OWNER}/${REPO}/branches/${BRANCH}/protection" \
    --input "$PROPOSED_JSON" >/dev/null

  echo "Applied successfully."
}

print_post_apply_summary() {
  if [[ "$DRY_RUN" == "true" ]]; then
    return
  fi

  print_section "Post-Apply Summary"
  local after
  after="$(mktemp)"
  gh api "repos/${OWNER}/${REPO}/branches/${BRANCH}/protection" > "$after"
  BRANCH="$BRANCH" jq -r '
    "Branch: " + ($ENV.BRANCH) + "\n" +
    "Require up-to-date: " + ((.required_status_checks.strict // false)|tostring) + "\n" +
    "Required checks: " + ((.required_status_checks.contexts // []) | join(", ")) + "\n" +
    "Required reviews: " + ((.required_pull_request_reviews.required_approving_review_count // 0)|tostring) + "\n" +
    "Require conversation resolution: " + ((.required_conversation_resolution.enabled // false)|tostring) + "\n" +
    "Enforce admins: " + ((.enforce_admins.enabled // false)|tostring)
  ' "$after"
  rm -f "$after"

  if [[ -n "$SNAPSHOT_JSON" ]]; then
    print_section "Rollback Hint"
    echo "To restore from snapshot:"
    echo "  jq '.protection' \"$SNAPSHOT_JSON\" > /tmp/branch-protection-restore.json"
    echo "  gh api -X PUT -H \"Accept: application/vnd.github+json\" \"repos/${OWNER}/${REPO}/branches/${BRANCH}/protection\" --input /tmp/branch-protection-restore.json"
  fi

  if [[ -n "$RESTORE_FROM" ]]; then
    echo
    echo "Applied from snapshot source: $RESTORE_FROM"
  fi
}

cleanup() {
  if [[ -n "$PROPOSED_JSON" && -f "$PROPOSED_JSON" ]]; then
    rm -f "$PROPOSED_JSON"
  fi
  if [[ -n "$CURRENT_JSON" && -f "$CURRENT_JSON" ]]; then
    rm -f "$CURRENT_JSON"
  fi
  return 0
}
trap cleanup EXIT

while [[ $# -gt 0 ]]; do
  case "$1" in
    --owner) OWNER="$2"; shift 2 ;;
    --repo) REPO="$2"; shift 2 ;;
    --branch) BRANCH="$2"; shift 2 ;;
    --profile) PROFILE="$2"; shift 2 ;;
    --checks) CHECKS="$2"; CHECKS_SET=true; shift 2 ;;
    --required-reviews) REQUIRED_REVIEWS="$2"; REQUIRED_REVIEWS_SET=true; shift 2 ;;
    --enforce-admins) ENFORCE_ADMINS="$2"; ENFORCE_ADMINS_SET=true; shift 2 ;;
    --require-conversation) REQUIRE_CONVERSATION_RESOLUTION="$2"; REQUIRE_CONVERSATION_RESOLUTION_SET=true; shift 2 ;;
    --strict-up-to-date) STRICT_UP_TO_DATE="$2"; STRICT_UP_TO_DATE_SET=true; shift 2 ;;
    --dismiss-stale) DISMISS_STALE_REVIEWS="$2"; DISMISS_STALE_REVIEWS_SET=true; shift 2 ;;
    --require-codeowners) REQUIRE_CODE_OWNER_REVIEWS="$2"; REQUIRE_CODE_OWNER_REVIEWS_SET=true; shift 2 ;;
    --backup-dir) BACKUP_DIR="$2"; shift 2 ;;
    --restore-from) RESTORE_FROM="$2"; shift 2 ;;
    --apply) DRY_RUN=false; shift ;;
    --non-interactive) INTERACTIVE=false; shift ;;
    --yes) YES=true; shift ;;
    --help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

require_cmd git
require_cmd jq
require_cmd gh

if [[ -z "$OWNER" || -z "$REPO" ]]; then
  infer_repo_from_git_remote
fi

if [[ "$INTERACTIVE" == "true" ]]; then
  interactive_customize
else
  if [[ -z "$RESTORE_FROM" ]]; then
    apply_profile_defaults
  fi
fi

if [[ -z "$RESTORE_FROM" ]]; then
  validate_inputs
fi

read_current_protection
BRANCH="$BRANCH" print_current_summary
build_proposed_payload
print_risk_guidance
print_diff_hint
confirm_apply
apply_changes
print_post_apply_summary

print_section "Done"
if [[ "$DRY_RUN" == "true" ]]; then
  echo "Review the payload and risk notes above, then rerun with --apply when ready."
else
  echo "Branch protection updated for ${OWNER}/${REPO}:${BRANCH}."
fi
