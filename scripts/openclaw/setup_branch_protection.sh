#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${PROJECT_ROOT}"

OWNER=""
REPO=""
BRANCH="main"
DRY_RUN=true
INTERACTIVE=true
PROFILE="derive"
YES=false
ALLOW_DOWNGRADE=false
ALLOW_UNKNOWN_CHECKS=false
LIST_KNOWN_CHECKS=false

# Stable exit-code contract (documented in usage). The refusal codes start at 20
# on purpose: jq itself exits 2/3/4/5 on failure, so a lower code could not be
# told apart from a broken jq call. 126/127/128+ are reserved by the shell.
#   0  = success (dry-run plan printed, or apply finished)
#   1  = generic error (bad flag, missing tool, confirmation mismatch, bad snapshot)
#   20 = refused: the plan would downgrade (remove/weaken) existing branch protection
#   21 = refused: the plan contains a required check name that nothing reports
#   22 = refused: no live required checks to derive from and no --checks given
#   23 = refused: the pre-apply snapshot could not be written and validated
#   24 = refused: internal invariant violated (write API call attempted in dry-run)
EXIT_GENERIC=1
EXIT_DOWNGRADE=20
EXIT_UNKNOWN_CHECKS=21
EXIT_NO_LIVE_CHECKS=22
EXIT_SNAPSHOT=23
EXIT_DRY_RUN_WRITE=24

# Required status checks: EMPTY means "derive from the live branch protection".
# This script deliberately keeps NO hardcoded check list. A stale list silently
# replaced the real contexts with names no workflow reports, which blocks every
# PR forever. See docs/operations-playbook.md "Branch Protection Setup".
CHECKS=""
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

# True when the live payload has no required_pull_request_reviews object at all.
# In that case the proposed payload must also send null, not an object with a
# zero review count: a non-null object turns "require a pull request" back on.
LIVE_REVIEWS_NULL=false

CHECKS_SET=false
CHECKS_DERIVED=false
STRICT_UP_TO_DATE_SET=false
REQUIRE_CONVERSATION_RESOLUTION_SET=false
ENFORCE_ADMINS_SET=false
REQUIRED_REVIEWS_SET=false
DISMISS_STALE_REVIEWS_SET=false
REQUIRE_CODE_OWNER_REVIEWS_SET=false

usage() {
  cat <<'EOF'
Usage: ./scripts/openclaw/setup_branch_protection.sh [OPTIONS]

Automate branch protection setup with guided human approval.
Default mode is DRY RUN (no changes).

Required status checks default to the CURRENT LIVE branch protection (whatever
owner/repo/branch is targeted). There is no hardcoded check list: with no
--checks flag the script plans "keep exactly what is required today".

Options:
  --owner <owner>              GitHub owner (default: infer from origin remote)
  --repo <repo>                GitHub repository name (default: infer from origin remote)
  --branch <name>              Branch to protect (default: main)
  --profile <name>             derive|recommended|strict|relaxed (default: derive)
                               derive = keep the live values for every setting
  --checks <a,b,c>             Required status checks (default: derive from live protection)
  --required-reviews <n>       Required approving reviews, 0..6 (default: live value; derive profile)
  --enforce-admins <bool>      true|false (default: live value; derive profile)
  --require-conversation <bool> true|false (default: live value; derive profile)
  --strict-up-to-date <bool>   true|false (default: live value; derive profile)
  --dismiss-stale <bool>       true|false (default: live value; derive profile)
  --require-codeowners <bool>  true|false (default: live value; derive profile)
  --backup-dir <path>          Snapshot output dir before apply (default: data/state/branch-protection-snapshots)
  --restore-from <path>        Restore protection payload from snapshot JSON
  --allow-downgrade            Allow a plan that removes existing required checks or weakens settings.
                               Without this flag such a plan is refused with exit 20 (fail-closed).
                               RISK: a removed context is protection that silently disappears.
  --allow-unknown-checks       Allow required check names that no workflow job name, live context,
                               or recent check run reports. Without this flag such a plan is
                               refused with exit 21. RISK: GitHub waits forever for a context
                               that never reports, so every PR stays blocked.
  --list-known-checks          Print the check names this repo can actually report, then exit
  --apply                      Apply changes (otherwise dry-run)
  --non-interactive            Do not prompt; use provided options
  --yes                        Skip final confirmation prompt (only with --non-interactive --apply)
  --help                       Show help

Exit codes:
  0 ok | 1 generic error | 20 downgrade refused | 21 unknown check refused
  22 no live checks to derive from | 23 snapshot failed | 24 write API in dry-run

Examples:
  ./scripts/openclaw/setup_branch_protection.sh
  ./scripts/openclaw/setup_branch_protection.sh --list-known-checks
  ./scripts/openclaw/setup_branch_protection.sh --apply
  ./scripts/openclaw/setup_branch_protection.sh --profile strict --apply
  ./scripts/openclaw/setup_branch_protection.sh --non-interactive --apply --yes
  ./scripts/openclaw/setup_branch_protection.sh --allow-downgrade --non-interactive --apply
  ./scripts/openclaw/setup_branch_protection.sh --restore-from data/state/branch-protection-snapshots/<file>.json --apply
EOF
}

print_section() {
  echo
  echo "== $1 =="
}

die() {
  echo "[error] $1" >&2
  exit "${EXIT_GENERIC}"
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

# Single choke point for every mutating GitHub API call. Dry-run must never
# reach the API with a write, so this refuses instead of trusting call order.
gh_api_write() {
  if [[ "${DRY_RUN}" == "true" ]]; then
    echo "[error] internal invariant violated: write API call attempted during dry-run (exit ${EXIT_DRY_RUN_WRITE})." >&2
    exit "${EXIT_DRY_RUN_WRITE}"
  fi
  gh api "$@"
}

infer_repo_from_git_remote() {
  local remote
  remote="$(git remote get-url origin 2>/dev/null || true)"
  [[ -n "${remote}" ]] || die "cannot infer owner/repo from origin remote; use --owner and --repo"

  # Supports:
  # - git@github.com:owner/repo.git
  # - https://github.com/owner/repo.git
  # - https://github.com/owner/repo
  if [[ "${remote}" =~ github.com[:/]([^/]+)/([^/.]+)(\.git)?$ ]]; then
    OWNER="${BASH_REMATCH[1]}"
    REPO="${BASH_REMATCH[2]}"
  else
    die "unsupported remote format: ${remote}"
  fi
}

parse_checks_csv() {
  local input="$1"
  local cleaned
  cleaned="$(echo "${input}" | tr ',' '\n' | sed 's/^ *//; s/ *$//' | sed '/^$/d')"
  [[ -n "${cleaned}" ]] || die "required checks cannot be empty"
  echo "${cleaned}"
}

# --- live protection readers (read-only) -------------------------------------

live_contexts_csv() {
  if [[ -z "${CURRENT_JSON}" || ! -f "${CURRENT_JSON}" ]]; then
    return 0
  fi
  jq -r '(.required_status_checks.contexts // []) | join(",")' "${CURRENT_JSON}"
}

# live_bool <jq filter with a default, e.g. '(.enforce_admins.enabled // false)'>
live_bool() {
  if [[ -z "${CURRENT_JSON}" || ! -f "${CURRENT_JSON}" ]]; then
    echo "false"
    return 0
  fi
  jq -r "$1 | tostring" "${CURRENT_JSON}" 2>/dev/null || echo "false"
}

live_number() {
  if [[ -z "${CURRENT_JSON}" || ! -f "${CURRENT_JSON}" ]]; then
    echo "0"
    return 0
  fi
  jq -r "$1 | tostring" "${CURRENT_JSON}" 2>/dev/null || echo "0"
}

# A boolean reaches us in one of two shapes: a plain boolean (what this script
# PUTs, and what GitHub accepts) or an object with "enabled" (what GET returns).
# Reading the wrong shape is a jq runtime error, so normalize before comparing.
JQ_ENABLED_HELPER='def enabled: if type == "boolean" then . elif type == "object" then (.enabled // false) else false end;'

# read_bool_state <json file> <jq filter that uses enabled>
read_bool_state() {
  local file="$1"
  local filter="$2"
  if [[ -z "${file}" || ! -f "${file}" ]]; then
    echo "false"
    return 0
  fi
  jq -r "${JQ_ENABLED_HELPER} ${filter} | tostring" "${file}" 2>/dev/null || echo "false"
}

apply_profile_defaults() {
  case "${PROFILE}" in
    derive)
      # Keep the live values for every managed setting: "no flags" means no change.
      if [[ "${STRICT_UP_TO_DATE_SET}" != true ]]; then
        STRICT_UP_TO_DATE="$(live_bool '(.required_status_checks.strict // false)')"
      fi
      if [[ "${REQUIRE_CONVERSATION_RESOLUTION_SET}" != true ]]; then
        REQUIRE_CONVERSATION_RESOLUTION="$(live_bool '(.required_conversation_resolution.enabled // false)')"
      fi
      if [[ "${ENFORCE_ADMINS_SET}" != true ]]; then
        ENFORCE_ADMINS="$(live_bool '(.enforce_admins.enabled // false)')"
      fi
      if [[ "${REQUIRED_REVIEWS_SET}" != true ]]; then
        REQUIRED_REVIEWS="$(live_number '(.required_pull_request_reviews.required_approving_review_count // 0)')"
      fi
      if [[ "${DISMISS_STALE_REVIEWS_SET}" != true ]]; then
        DISMISS_STALE_REVIEWS="$(live_bool '(.required_pull_request_reviews.dismiss_stale_reviews // false)')"
      fi
      if [[ "${REQUIRE_CODE_OWNER_REVIEWS_SET}" != true ]]; then
        REQUIRE_CODE_OWNER_REVIEWS="$(live_bool '(.required_pull_request_reviews.require_code_owner_reviews // false)')"
      fi
      ;;
    recommended)
      if [[ "${STRICT_UP_TO_DATE_SET}" != true ]]; then STRICT_UP_TO_DATE=true; fi
      if [[ "${REQUIRE_CONVERSATION_RESOLUTION_SET}" != true ]]; then REQUIRE_CONVERSATION_RESOLUTION=true; fi
      if [[ "${ENFORCE_ADMINS_SET}" != true ]]; then ENFORCE_ADMINS=false; fi
      if [[ "${REQUIRED_REVIEWS_SET}" != true ]]; then REQUIRED_REVIEWS=1; fi
      if [[ "${DISMISS_STALE_REVIEWS_SET}" != true ]]; then DISMISS_STALE_REVIEWS=true; fi
      if [[ "${REQUIRE_CODE_OWNER_REVIEWS_SET}" != true ]]; then REQUIRE_CODE_OWNER_REVIEWS=false; fi
      ;;
    strict)
      if [[ "${STRICT_UP_TO_DATE_SET}" != true ]]; then STRICT_UP_TO_DATE=true; fi
      if [[ "${REQUIRE_CONVERSATION_RESOLUTION_SET}" != true ]]; then REQUIRE_CONVERSATION_RESOLUTION=true; fi
      if [[ "${ENFORCE_ADMINS_SET}" != true ]]; then ENFORCE_ADMINS=true; fi
      if [[ "${REQUIRED_REVIEWS_SET}" != true ]]; then REQUIRED_REVIEWS=2; fi
      if [[ "${DISMISS_STALE_REVIEWS_SET}" != true ]]; then DISMISS_STALE_REVIEWS=true; fi
      if [[ "${REQUIRE_CODE_OWNER_REVIEWS_SET}" != true ]]; then REQUIRE_CODE_OWNER_REVIEWS=true; fi
      ;;
    relaxed)
      if [[ "${STRICT_UP_TO_DATE_SET}" != true ]]; then STRICT_UP_TO_DATE=false; fi
      if [[ "${REQUIRE_CONVERSATION_RESOLUTION_SET}" != true ]]; then REQUIRE_CONVERSATION_RESOLUTION=false; fi
      if [[ "${ENFORCE_ADMINS_SET}" != true ]]; then ENFORCE_ADMINS=false; fi
      if [[ "${REQUIRED_REVIEWS_SET}" != true ]]; then REQUIRED_REVIEWS=1; fi
      if [[ "${DISMISS_STALE_REVIEWS_SET}" != true ]]; then DISMISS_STALE_REVIEWS=false; fi
      if [[ "${REQUIRE_CODE_OWNER_REVIEWS_SET}" != true ]]; then REQUIRE_CODE_OWNER_REVIEWS=false; fi
      ;;
    *)
      die "unknown profile: ${PROFILE}"
      ;;
  esac
}

# Default required checks come from the live protection on the targeted branch.
# No live checks and no --checks => refuse (exit 5) instead of inventing a list.
resolve_required_checks() {
  if [[ -n "${RESTORE_FROM}" ]]; then
    return 0
  fi
  if [[ "${CHECKS_SET}" == true ]]; then
    return 0
  fi

  local live
  live="$(live_contexts_csv)"
  if [[ -z "${live}" ]]; then
    echo "[error] no live required status checks on ${OWNER}/${REPO}:${BRANCH} to derive from." >&2
    echo "        Pass --checks <a,b,c> explicitly (add --allow-unknown-checks if the names are new)." >&2
    exit "${EXIT_NO_LIVE_CHECKS}"
  fi
  CHECKS="${live}"
  CHECKS_DERIVED=true
}

read_current_protection() {
  local tmp
  tmp="$(mktemp)"
  if gh api "repos/${OWNER}/${REPO}/branches/${BRANCH}/protection" >"${tmp}" 2>/dev/null; then
    CURRENT_JSON="${tmp}"
    LIVE_REVIEWS_NULL="$(jq -r '(.required_pull_request_reviews == null) | tostring' "${tmp}" 2>/dev/null || echo false)"
  else
    CURRENT_JSON=""
    LIVE_REVIEWS_NULL=true
    rm -f "${tmp}"
  fi
}

# --- check-name probe (read-only) --------------------------------------------

# Job ids and job-level "name:" values from the workflows in this checkout.
# The context a GitHub Actions check reports is the job id, or the job "name:".
workflow_job_names() {
  local dir="${PROJECT_ROOT}/.github/workflows"
  [[ -d "${dir}" ]] || return 0

  local files=()
  local f
  while IFS= read -r f; do
    [[ -n "${f}" ]] || continue
    files+=("${f}")
  done < <(find "${dir}" -maxdepth 1 -type f \( -name '*.yml' -o -name '*.yaml' \) | sort)
  (( ${#files[@]} > 0 )) || return 0

  awk '
    /^[A-Za-z]/ { in_jobs = 0; next }
    /^jobs:[[:space:]]*$/ { in_jobs = 1; next }
    in_jobs && /^  [A-Za-z0-9_.-]+:[[:space:]]*$/ {
      job = $1
      sub(/:$/, "", job)
      print job
      next
    }
    in_jobs && /^    name:[[:space:]]*/ {
      label = $0
      sub(/^    name:[[:space:]]*/, "", label)
      gsub(/"/, "", label)
      print label
      next
    }
  ' "${files[@]}" 2>/dev/null || true
}

# Names of check runs already seen on the branch (best effort; read-only).
recent_check_run_names() {
  gh api "repos/${OWNER}/${REPO}/commits/${BRANCH}/check-runs?per_page=100" 2>/dev/null \
    | jq -r '(.check_runs // [])[]?.name' 2>/dev/null || true
}

collect_known_check_names() {
  local out="$1"
  : > "${out}"
  if [[ -n "${CURRENT_JSON}" && -f "${CURRENT_JSON}" ]]; then
    jq -r '(.required_status_checks.contexts // [])[]' "${CURRENT_JSON}" >> "${out}" 2>/dev/null || true
    jq -r '(.required_status_checks.checks // [])[]?.context' "${CURRENT_JSON}" >> "${out}" 2>/dev/null || true
  fi
  workflow_job_names >> "${out}" || true
  recent_check_run_names >> "${out}" || true
  local tmp="${out}.sorted"
  sed '/^[[:space:]]*$/d' "${out}" | sort -u > "${tmp}" || true
  mv "${tmp}" "${out}"
}

print_known_checks() {
  print_section "Known Check Names"
  echo "Names ${OWNER}/${REPO}:${BRANCH} can report (live contexts + workflow job names + recent check runs):"
  local known
  known="$(mktemp)"
  collect_known_check_names "${known}"
  cat "${known}"
  rm -f "${known}"
}

print_current_summary() {
  print_section "Current Configuration"
  if [[ -z "${CURRENT_JSON}" ]]; then
    echo "No existing branch protection found on ${OWNER}/${REPO}:${BRANCH}."
    return
  fi

  jq -r '
    "Branch: " + ($ENV.BRANCH) + "\n" +
    "Require up-to-date: " + ((.required_status_checks.strict // false)|tostring) + "\n" +
    "Required checks (" + (((.required_status_checks.contexts // []) | length) | tostring) + "): " +
      ((.required_status_checks.contexts // []) | join(", ")) + "\n" +
    "Required reviews: " + ((.required_pull_request_reviews.required_approving_review_count // 0)|tostring) + "\n" +
    "Dismiss stale reviews: " + ((.required_pull_request_reviews.dismiss_stale_reviews // false)|tostring) + "\n" +
    "Require code owner reviews: " + ((.required_pull_request_reviews.require_code_owner_reviews // false)|tostring) + "\n" +
    "Require conversation resolution: " + ((.required_conversation_resolution.enabled // false)|tostring) + "\n" +
    "Enforce admins: " + ((.enforce_admins.enabled // false)|tostring)
  ' "${CURRENT_JSON}"
}

prompt_bool() {
  local label="$1"
  local default="$2"
  local raw
  while true; do
    read -r -p "${label} [${default}]: " raw
    raw="${raw:-${default}}"
    if v="$(normalize_bool "${raw}" 2>/dev/null)"; then
      echo "${v}"
      return
    fi
    echo "Please enter true/false (or yes/no)."
  done
}

interactive_customize() {
  if [[ -n "${RESTORE_FROM}" ]]; then
    return
  fi

  print_section "Profile Selection"
  echo "1) maintain    - keep current live settings (default; nothing hardcoded)"
  echo "2) recommended - balanced governance (reviews=1, conversation resolution on)"
  echo "3) strict      - stronger controls, higher merge friction"
  echo "4) relaxed     - faster merge, higher governance risk"
  echo "5) custom      - guided custom configuration"

  local choice
  read -r -p "Select profile [1]: " choice
  choice="${choice:-1}"

  case "${choice}" in
    1) PROFILE="derive"; apply_profile_defaults ;;
    2) PROFILE="recommended"; apply_profile_defaults ;;
    3) PROFILE="strict"; apply_profile_defaults ;;
    4) PROFILE="relaxed"; apply_profile_defaults ;;
    5)
      PROFILE="derive"
      apply_profile_defaults
      local checks_default checks_in rev_in
      checks_default="$(live_contexts_csv)"
      read -r -p "Required checks (comma-separated) [${checks_default}]: " checks_in
      if [[ -n "${checks_in:-}" ]]; then
        CHECKS="${checks_in}"
        CHECKS_SET=true
      fi
      read -r -p "Required approving reviews (0..6) [${REQUIRED_REVIEWS}]: " rev_in
      if [[ -n "${rev_in:-}" ]]; then
        REQUIRED_REVIEWS="${rev_in}"
      fi
      STRICT_UP_TO_DATE="$(prompt_bool "Require branches up to date before merge" "${STRICT_UP_TO_DATE}")"
      REQUIRE_CONVERSATION_RESOLUTION="$(prompt_bool "Require conversation resolution" "${REQUIRE_CONVERSATION_RESOLUTION}")"
      ENFORCE_ADMINS="$(prompt_bool "Enforce for administrators" "${ENFORCE_ADMINS}")"
      DISMISS_STALE_REVIEWS="$(prompt_bool "Dismiss stale reviews on new push" "${DISMISS_STALE_REVIEWS}")"
      REQUIRE_CODE_OWNER_REVIEWS="$(prompt_bool "Require code owner reviews" "${REQUIRE_CODE_OWNER_REVIEWS}")"
      ;;
    *) die "invalid profile choice: ${choice}" ;;
  esac
}

load_restore_payload() {
  [[ -n "${RESTORE_FROM}" ]] || return
  [[ -f "${RESTORE_FROM}" ]] || die "restore snapshot not found: ${RESTORE_FROM}"

  local s_owner s_repo s_branch
  s_owner="$(jq -r '.owner // empty' "${RESTORE_FROM}")"
  s_repo="$(jq -r '.repo // empty' "${RESTORE_FROM}")"
  s_branch="$(jq -r '.branch // empty' "${RESTORE_FROM}")"

  [[ -n "${s_owner}" && -n "${s_repo}" && -n "${s_branch}" ]] || die "invalid snapshot: missing owner/repo/branch"

  if [[ -z "${OWNER}" ]]; then
    OWNER="${s_owner}"
  fi
  if [[ -z "${REPO}" ]]; then
    REPO="${s_repo}"
  fi
  if [[ -z "${BRANCH}" ]]; then
    BRANCH="${s_branch}"
  fi

  [[ "${OWNER}" == "${s_owner}" ]] || die "snapshot owner mismatch: expected ${OWNER}, got ${s_owner}"
  [[ "${REPO}" == "${s_repo}" ]] || die "snapshot repo mismatch: expected ${REPO}, got ${s_repo}"
  [[ "${BRANCH}" == "${s_branch}" ]] || die "snapshot branch mismatch: expected ${BRANCH}, got ${s_branch}"

  jq -e '.protection_exists == true and (.protection | type == "object")' "${RESTORE_FROM}" >/dev/null || die "snapshot does not contain restorable protection payload"

  PROPOSED_JSON="$(mktemp)"
  jq '.protection' "${RESTORE_FROM}" > "${PROPOSED_JSON}"
}

validate_inputs() {
  [[ "${REQUIRED_REVIEWS}" =~ ^[0-9]+$ ]] || die "required reviews must be integer"
  if (( REQUIRED_REVIEWS < 0 || REQUIRED_REVIEWS > 6 )); then
    die "required reviews out of range (0..6): ${REQUIRED_REVIEWS}"
  fi

  STRICT_UP_TO_DATE="$(normalize_bool "${STRICT_UP_TO_DATE}")"
  REQUIRE_CONVERSATION_RESOLUTION="$(normalize_bool "${REQUIRE_CONVERSATION_RESOLUTION}")"
  ENFORCE_ADMINS="$(normalize_bool "${ENFORCE_ADMINS}")"
  DISMISS_STALE_REVIEWS="$(normalize_bool "${DISMISS_STALE_REVIEWS}")"
  REQUIRE_CODE_OWNER_REVIEWS="$(normalize_bool "${REQUIRE_CODE_OWNER_REVIEWS}")"

  local lines
  lines="$(parse_checks_csv "${CHECKS}")"
  CHECKS="$(echo "${lines}" | paste -sd ',' -)"
}

build_proposed_payload() {
  if [[ -n "${RESTORE_FROM}" ]]; then
    load_restore_payload
    return
  fi

  local checks_json reviews_json
  checks_json="$(parse_checks_csv "${CHECKS}" | jq -R . | jq -s .)"
  if [[ "${LIVE_REVIEWS_NULL}" == "true" && "${REQUIRED_REVIEWS}" == "0" \
        && "${DISMISS_STALE_REVIEWS}" == "false" && "${REQUIRE_CODE_OWNER_REVIEWS}" == "false" ]]; then
    # Mirror the live shape: null means "no required PR reviews". Sending an
    # object with count 0 would switch "require a pull request" back on.
    reviews_json="null"
  else
    reviews_json="$(jq -n \
      --argjson count "${REQUIRED_REVIEWS}" \
      --argjson dismiss "${DISMISS_STALE_REVIEWS}" \
      --argjson codeowners "${REQUIRE_CODE_OWNER_REVIEWS}" \
      '{dismiss_stale_reviews: $dismiss, require_code_owner_reviews: $codeowners, required_approving_review_count: $count}')"
  fi

  PROPOSED_JSON="$(mktemp)"
  jq -n \
    --argjson checks "${checks_json}" \
    --argjson strict "${STRICT_UP_TO_DATE}" \
    --argjson enforce_admins "${ENFORCE_ADMINS}" \
    --argjson reviews "${reviews_json}" \
    --argjson require_conversation "${REQUIRE_CONVERSATION_RESOLUTION}" \
    '{
      required_status_checks: {
        strict: $strict,
        contexts: $checks
      },
      enforce_admins: $enforce_admins,
      required_pull_request_reviews: $reviews,
      restrictions: null,
      required_conversation_resolution: $require_conversation
    }' > "${PROPOSED_JSON}"
}

# --- fail-closed guards ------------------------------------------------------

GUARD_REMOVED=()
GUARD_WEAKENED=()

guard_against_downgrade() {
  GUARD_REMOVED=()
  GUARD_WEAKENED=()
  print_section "Downgrade Guard"

  if [[ -z "${CURRENT_JSON}" || ! -f "${CURRENT_JSON}" ]]; then
    echo "No existing branch protection on ${OWNER}/${REPO}:${BRANCH} - nothing to downgrade."
    return 0
  fi

  local live_ctx prop_ctx
  live_ctx="$(mktemp)"
  prop_ctx="$(mktemp)"
  jq -r '(.required_status_checks.contexts // [])[]' "${CURRENT_JSON}" 2>/dev/null | sed '/^[[:space:]]*$/d' > "${live_ctx}" || true
  jq -r '(.required_status_checks.contexts // [])[]' "${PROPOSED_JSON}" 2>/dev/null | sed '/^[[:space:]]*$/d' > "${prop_ctx}" || true

  local ctx
  while IFS= read -r ctx; do
    [[ -n "${ctx}" ]] || continue
    if ! grep -Fxq -- "${ctx}" "${prop_ctx}"; then
      GUARD_REMOVED+=("${ctx}")
    fi
  done < "${live_ctx}"
  rm -f "${live_ctx}" "${prop_ctx}"

  local live_strict prop_strict
  live_strict="$(read_bool_state "${CURRENT_JSON}" '(.required_status_checks.strict // false)')"
  prop_strict="$(read_bool_state "${PROPOSED_JSON}" '(.required_status_checks.strict // false)')"
  if [[ "${live_strict}" == "true" && "${prop_strict}" == "false" ]]; then
    GUARD_WEAKENED+=("required_status_checks.strict: true -> false (stale branches may merge without latest checks)")
  fi

  local live_admins prop_admins
  live_admins="$(read_bool_state "${CURRENT_JSON}" '(.enforce_admins | enabled)')"
  prop_admins="$(read_bool_state "${PROPOSED_JSON}" '(.enforce_admins | enabled)')"
  if [[ "${live_admins}" == "true" && "${prop_admins}" == "false" ]]; then
    GUARD_WEAKENED+=("enforce_admins: true -> false (admins can bypass the rules)")
  fi

  local live_conv prop_conv
  live_conv="$(read_bool_state "${CURRENT_JSON}" '(.required_conversation_resolution | enabled)')"
  prop_conv="$(read_bool_state "${PROPOSED_JSON}" '(.required_conversation_resolution | enabled)')"
  if [[ "${live_conv}" == "true" && "${prop_conv}" == "false" ]]; then
    GUARD_WEAKENED+=("required_conversation_resolution: true -> false (unresolved threads can merge)")
  fi

  local live_rev_type prop_rev_type
  live_rev_type="$(jq -r '(.required_pull_request_reviews | type)' "${CURRENT_JSON}")"
  prop_rev_type="$(jq -r '(.required_pull_request_reviews | type)' "${PROPOSED_JSON}")"
  if [[ "${live_rev_type}" == "object" && "${prop_rev_type}" == "null" ]]; then
    GUARD_WEAKENED+=("required_pull_request_reviews: enabled -> disabled")
  else
    local live_count prop_count
    live_count="$(jq -r '((.required_pull_request_reviews // {}).required_approving_review_count // 0)' "${CURRENT_JSON}")"
    prop_count="$(jq -r '((.required_pull_request_reviews // {}).required_approving_review_count // 0)' "${PROPOSED_JSON}")"
    if (( prop_count < live_count )); then
      GUARD_WEAKENED+=("required_approving_review_count: ${live_count} -> ${prop_count}")
    fi

    local live_dismiss prop_dismiss
    live_dismiss="$(read_bool_state "${CURRENT_JSON}" '((.required_pull_request_reviews // {}).dismiss_stale_reviews // false)')"
    prop_dismiss="$(read_bool_state "${PROPOSED_JSON}" '((.required_pull_request_reviews // {}).dismiss_stale_reviews // false)')"
    if [[ "${live_dismiss}" == "true" && "${prop_dismiss}" == "false" ]]; then
      GUARD_WEAKENED+=("dismiss_stale_reviews: true -> false (old approvals survive new pushes)")
    fi

    local live_co prop_co
    live_co="$(read_bool_state "${CURRENT_JSON}" '((.required_pull_request_reviews // {}).require_code_owner_reviews // false)')"
    prop_co="$(read_bool_state "${PROPOSED_JSON}" '((.required_pull_request_reviews // {}).require_code_owner_reviews // false)')"
    if [[ "${live_co}" == "true" && "${prop_co}" == "false" ]]; then
      GUARD_WEAKENED+=("require_code_owner_reviews: true -> false (domain owners may be skipped)")
    fi
  fi

  local live_restr prop_restr
  live_restr="$(jq -r '(.restrictions | type)' "${CURRENT_JSON}")"
  prop_restr="$(jq -r '(.restrictions | type)' "${PROPOSED_JSON}")"
  if [[ "${live_restr}" != "null" && "${prop_restr}" == "null" ]]; then
    GUARD_WEAKENED+=("restrictions: enabled -> disabled (push restrictions removed)")
  fi


  if (( ${#GUARD_REMOVED[@]} == 0 && ${#GUARD_WEAKENED[@]} == 0 )); then
    echo "No downgrade detected: the plan keeps every existing required check and weakens nothing."
    return 0
  fi

  if (( ${#GUARD_REMOVED[@]} > 0 )); then
    echo "Plan REMOVES ${#GUARD_REMOVED[@]} existing required status check(s):"
    printf '  - %s\n' "${GUARD_REMOVED[@]}"
  fi
  if (( ${#GUARD_WEAKENED[@]} > 0 )); then
    echo "Plan WEAKENS ${#GUARD_WEAKENED[@]} existing protection setting(s):"
    printf '  - %s\n' "${GUARD_WEAKENED[@]}"
  fi

  if [[ "${ALLOW_DOWNGRADE}" != true ]]; then
    echo >&2
    echo "[refused] this plan would downgrade existing branch protection (exit ${EXIT_DOWNGRADE})." >&2
    echo "          Re-run with --allow-downgrade only if the reduction is intentional." >&2
    exit "${EXIT_DOWNGRADE}"
  fi

  echo
  echo "[warning] --allow-downgrade is set: applying this plan will REDUCE protection."
}

validate_proposed_check_names() {
  print_section "Check Name Probe"

  local known
  known="$(mktemp)"
  collect_known_check_names "${known}"

  local unknown=()
  local ctx
  while IFS= read -r ctx; do
    [[ -n "${ctx}" ]] || continue
    if ! grep -Fxq -- "${ctx}" "${known}"; then
      unknown+=("${ctx}")
    fi
  done < <(jq -r '(.required_status_checks.contexts // [])[]' "${PROPOSED_JSON}" 2>/dev/null | sed '/^[[:space:]]*$/d')
  rm -f "${known}"

  if (( ${#unknown[@]} == 0 )); then
    echo "Every proposed required check matches a live context, a workflow job name, or a recent check run."
    return 0
  fi

  echo "Proposed required checks that no workflow job name, live context, or recent check run reports:"
  printf '  - %s\n' "${unknown[@]}"

  if [[ "${ALLOW_UNKNOWN_CHECKS}" != true ]]; then
    echo >&2
    echo "[refused] unknown required check name(s) would block every PR forever:" >&2
    echo "          GitHub waits for a context that never reports (exit ${EXIT_UNKNOWN_CHECKS})." >&2
    echo "          Run --list-known-checks to see the names this repo can report." >&2
    exit "${EXIT_UNKNOWN_CHECKS}"
  fi

  echo
  echo "[warning] --allow-unknown-checks is set: unknown names are applied anyway."
}

print_risk_guidance() {
  print_section "Options and Risk Guidance"
  if [[ -n "${RESTORE_FROM}" ]]; then
    echo "Restore mode: true"
    echo "Restore source: ${RESTORE_FROM}"
    echo
    echo "Potential risks by option:"
    echo "- restoring old snapshot may overwrite newer governance settings."
    echo "- restored required checks may not exist in current workflow names."
    echo "- restored review settings may increase or decrease merge friction unexpectedly."
    return
  fi

  echo "Selected profile: ${PROFILE}"
  if [[ "${CHECKS_DERIVED}" == true ]]; then
    echo "Required checks: derived from live branch protection (no hardcoded list)"
  else
    echo "Required checks: provided via --checks"
  fi
  echo "Required checks (${#GUARD_REMOVED[@]} removal(s) planned): ${CHECKS}"
  echo "Required reviews: ${REQUIRED_REVIEWS}"
  echo "Require up-to-date: ${STRICT_UP_TO_DATE}"
  echo "Require conversation resolution: ${REQUIRE_CONVERSATION_RESOLUTION}"
  echo "Enforce admins: ${ENFORCE_ADMINS}"
  echo "Dismiss stale reviews: ${DISMISS_STALE_REVIEWS}"
  echo "Require code owner reviews: ${REQUIRE_CODE_OWNER_REVIEWS}"

  echo
  echo "Potential risks by option:"
  [[ "${STRICT_UP_TO_DATE}" == "false" ]] && echo "- strict-up-to-date=false: stale branches may merge without latest checks."
  [[ "${REQUIRE_CONVERSATION_RESOLUTION}" == "false" ]] && echo "- conversation-resolution=false: unresolved review threads can be merged."
  [[ "${ENFORCE_ADMINS}" == "false" ]] && echo "- enforce-admins=false: admins can bypass branch rules."
  [[ "${DISMISS_STALE_REVIEWS}" == "false" ]] && echo "- dismiss-stale=false: old approvals remain valid after new pushes."
  [[ "${REQUIRE_CODE_OWNER_REVIEWS}" == "false" ]] && echo "- require-codeowners=false: domain owners may be skipped in review."
  if [[ "${ALLOW_UNKNOWN_CHECKS}" == true ]]; then
    echo "- allow-unknown-checks=true: a context nothing reports blocks every PR forever."
  fi
  return 0
}

print_diff_hint() {
  print_section "Proposed Payload"
  cat "${PROPOSED_JSON}" | jq .
}

create_snapshot_backup() {
  if [[ "${DRY_RUN}" == "true" ]]; then
    return
  fi

  mkdir -p "${BACKUP_DIR}"
  [[ -d "${BACKUP_DIR}" && -w "${BACKUP_DIR}" ]] || die "snapshot dir is not writable: ${BACKUP_DIR}"

  local ts
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  SNAPSHOT_JSON="${BACKUP_DIR}/${OWNER}-${REPO}-${BRANCH}-${ts}.json"

  if [[ -n "${CURRENT_JSON}" && -f "${CURRENT_JSON}" ]]; then
    jq -n \
      --arg generated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      --arg owner "${OWNER}" \
      --arg repo "${REPO}" \
      --arg branch "${BRANCH}" \
      --arg backup_type "pre-apply" \
      --slurpfile protection "${CURRENT_JSON}" \
      '{
        generated_at: $generated_at,
        owner: $owner,
        repo: $repo,
        branch: $branch,
        backup_type: $backup_type,
        protection_exists: true,
        protection: $protection[0]
      }' > "${SNAPSHOT_JSON}"
  else
    jq -n \
      --arg generated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      --arg owner "${OWNER}" \
      --arg repo "${REPO}" \
      --arg branch "${BRANCH}" \
      --arg backup_type "pre-apply" \
      '{
        generated_at: $generated_at,
        owner: $owner,
        repo: $repo,
        branch: $branch,
        backup_type: $backup_type,
        protection_exists: false,
        protection: null
      }' > "${SNAPSHOT_JSON}"
  fi

  if [[ ! -s "${SNAPSHOT_JSON}" ]]; then
    echo "[error] snapshot file is missing or empty: ${SNAPSHOT_JSON}" >&2
    exit "${EXIT_SNAPSHOT}"
  fi
  if ! jq -e . "${SNAPSHOT_JSON}" >/dev/null 2>&1; then
    echo "[error] snapshot file is not valid JSON: ${SNAPSHOT_JSON}" >&2
    exit "${EXIT_SNAPSHOT}"
  fi

  print_section "Snapshot Backup"
  echo "Saved current configuration snapshot to: ${SNAPSHOT_JSON}"
  echo "Restore later with: --restore-from ${SNAPSHOT_JSON}"
}

confirm_apply() {
  if [[ "${DRY_RUN}" == "true" ]]; then
    return
  fi

  if [[ "${INTERACTIVE}" == "true" ]]; then
    local phrase expected
    if [[ -n "${RESTORE_FROM}" ]]; then
      expected="RESTORE ${OWNER}/${REPO}:${BRANCH}"
    else
      expected="APPLY ${OWNER}/${REPO}:${BRANCH}"
    fi
    echo
    echo "Type this exact phrase to apply changes:"
    echo "  ${expected}"
    read -r -p "> " phrase
    [[ "${phrase}" == "${expected}" ]] || die "confirmation phrase mismatch; aborted"
  else
    [[ "${YES}" == "true" ]] || die "non-interactive apply requires --yes"
  fi
}

apply_changes() {
  if [[ "${DRY_RUN}" == "true" ]]; then
    print_section "Dry Run"
    echo "No changes were applied."
    echo "Only read-only GET calls were sent to the GitHub API; no PATCH/PUT/DELETE was issued."
    echo "To apply, run with: --apply"
    return
  fi

  print_section "Applying Branch Protection"
  # Snapshot first: a PUT replaces the whole protection payload, so the previous
  # state must be on disk before the call.
  create_snapshot_backup
  gh_api_write \
    -X PUT \
    -H "Accept: application/vnd.github+json" \
    "repos/${OWNER}/${REPO}/branches/${BRANCH}/protection" \
    --input "${PROPOSED_JSON}" >/dev/null

  echo "Applied successfully."
}

print_post_apply_summary() {
  if [[ "${DRY_RUN}" == "true" ]]; then
    return
  fi

  print_section "Post-Apply Summary"
  local after
  after="$(mktemp)"
  gh api "repos/${OWNER}/${REPO}/branches/${BRANCH}/protection" > "${after}"
  BRANCH="${BRANCH}" jq -r '
    "Branch: " + ($ENV.BRANCH) + "\n" +
    "Require up-to-date: " + ((.required_status_checks.strict // false)|tostring) + "\n" +
    "Required checks: " + ((.required_status_checks.contexts // []) | join(", ")) + "\n" +
    "Required reviews: " + ((.required_pull_request_reviews.required_approving_review_count // 0)|tostring) + "\n" +
    "Require conversation resolution: " + ((.required_conversation_resolution.enabled // false)|tostring) + "\n" +
    "Enforce admins: " + ((.enforce_admins.enabled // false)|tostring)
  ' "${after}"
  rm -f "${after}"

  if [[ -n "${SNAPSHOT_JSON}" ]]; then
    print_section "Rollback Hint"
    echo "To restore from snapshot:"
    echo "  jq '.protection' \"${SNAPSHOT_JSON}\" > /tmp/branch-protection-restore.json"
    echo "  gh api -X PUT -H \"Accept: application/vnd.github+json\" \"repos/${OWNER}/${REPO}/branches/${BRANCH}/protection\" --input /tmp/branch-protection-restore.json"
  fi

  if [[ -n "${RESTORE_FROM}" ]]; then
    echo
    echo "Applied from snapshot source: ${RESTORE_FROM}"
  fi
}

cleanup() {
  if [[ -n "${PROPOSED_JSON}" && -f "${PROPOSED_JSON}" ]]; then
    rm -f "${PROPOSED_JSON}"
  fi
  if [[ -n "${CURRENT_JSON}" && -f "${CURRENT_JSON}" ]]; then
    rm -f "${CURRENT_JSON}"
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
    --allow-downgrade) ALLOW_DOWNGRADE=true; shift ;;
    --allow-unknown-checks) ALLOW_UNKNOWN_CHECKS=true; shift ;;
    --list-known-checks) LIST_KNOWN_CHECKS=true; shift ;;
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

if [[ -z "${OWNER}" || -z "${REPO}" ]]; then
  infer_repo_from_git_remote
fi

# Read-only GET: needed before any profile resolution, because "derive" reads it.
read_current_protection

if [[ "${LIST_KNOWN_CHECKS}" == true ]]; then
  print_known_checks
  exit 0
fi

if [[ "${INTERACTIVE}" == "true" ]]; then
  interactive_customize
else
  apply_profile_defaults
fi

resolve_required_checks

if [[ -z "${RESTORE_FROM}" ]]; then
  validate_inputs
fi

BRANCH="${BRANCH}" print_current_summary
build_proposed_payload
guard_against_downgrade
validate_proposed_check_names
print_risk_guidance
print_diff_hint
confirm_apply
apply_changes
print_post_apply_summary

print_section "Done"
if [[ "${DRY_RUN}" == "true" ]]; then
  echo "Review the payload and risk notes above, then rerun with --apply when ready."
else
  echo "Branch protection updated for ${OWNER}/${REPO}:${BRANCH}."
fi
