#!/usr/bin/env bash
# scripts/post-soak-cleanup.sh
#
# Day 7 結束後的歸檔+改名+降頻自動化 SOP,per production-rollout-runbook.md §4。
# 對應 Issue #1187 §5「成功後動作」,取代手動執行 §4.1–4.4。
#
# Phase 1: 文件歸檔 (runbook §4.1)
# Phase 2: Script 改名 + 推廣 (runbook §4.2)
# Phase 3: Cron/LaunchAgent 降頻 (runbook §4.3)
# Phase 4: .omo plans 移除 (runbook §4.4)
# Phase 5: Issue #1187 收尾驗證 (runbook §5)
#
# Dry-run 模式:`--dry-run`(預設只列出將執行的動作,不實際改動)
# Live 模式:`--live`(執行)
# 預設 dry-run 避免意外 commit/push。
#
# 前置:
# - staging 7-day soak Day 7 全部 6/6 PASS
# - Issue #1187 已 CLOSED
# - main HEAD 已 merge 所有 7-day 期間 fix

set -euo pipefail

MODE="${1:---dry-run}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATE_TAG=$(date -u +%Y-%m-%d)
LOG_DIR="${LOG_DIR:-$HOME/logs/atlas-soak/post-soak-cleanup}"
LAUNCHD_AGENT="$HOME/Library/LaunchAgents/com.atlas.soak-check.plist"
OMO_PLAN_DIR="$REPO_ROOT/.omo/plans/2026-07-15-capital-flow-audit-followup"
MAIN_PLAN_FILE="$REPO_ROOT/.omo/plans/Atlas 錢潮方向預測實作規劃.md"

mkdir -p "$LOG_DIR"
LOG_FILE="$LOG_DIR/$DATE_TAG.log"

log() {
    local tag="$1"
    shift
    echo "[$(date -u +%H:%M:%S)] [$tag] $*" | tee -a "$LOG_FILE"
}

run_cmd() {
    local desc="$1"
    shift
    if [[ "$MODE" == "--live" ]]; then
        log "RUN" "$desc: $*"
        "$@"
    else
        log "DRY" "$desc (would run): $*"
    fi
}

main() {
    log "PHASE" "==== post-soak-cleanup start (mode: $MODE) ===="

    # Phase 1: 文件歸檔 to docs/operations/archive/2026-07-15/
    log "PHASE" "1 — 文件歸檔"
    local archive_dir="$REPO_ROOT/docs/operations/archive/2026-07-15"
    run_cmd "create archive dir" mkdir -p "$archive_dir"
    run_cmd "git mv soak test" \
        git -C "$REPO_ROOT" mv \
            "docs/operations/2026-07-15-staging-soak-test.md" \
            "docs/operations/archive/2026-07-15/"
    run_cmd "git mv soak-day-counter" \
        git -C "$REPO_ROOT" mv \
            "docs/operations/soak-day-counter.md" \
            "docs/operations/archive/2026-07-15/"
    run_cmd "add Completed header to soak test" \
        bash -c 'sed -i "1i\# Completed '"$DATE_TAG"'\n" "$REPO_ROOT/docs/operations/archive/2026-07-15/2026-07-15-staging-soak-test.md"'
    run_cmd "add Completed header to soak-day-counter" \
        bash -c 'sed -i "1i\# Completed '"$DATE_TAG"'\n" "$REPO_ROOT/docs/operations/archive/2026-07-15/soak-day-counter.md"'

    # Phase 2: Script 改名 + 推廣
    log "PHASE" "2 — Script 改名"
    run_cmd "git mv staging-soak-check.sh → staging-deployment-health-check.sh" \
        git -C "$REPO_ROOT" mv \
            "scripts/staging-soak-check.sh" \
            "scripts/staging-deployment-health-check.sh"
    run_cmd "git mv install-soak-automation.sh → install-staging-health-check.sh" \
        git -C "$REPO_ROOT" mv \
            "scripts/install-soak-automation.sh" \
            "scripts/install-staging-health-check.sh"
    run_cmd "install new name to ~/bin" \
        install -m 0755 "$REPO_ROOT/scripts/staging-deployment-health-check.sh" "$HOME/bin/staging-deployment-health-check.sh"

    # Phase 3: 降頻 weekly Monday
    log "PHASE" "3 — LaunchAgent 降頻 weekly Monday"
    if [[ -f "$LAUNCHD_AGENT" ]]; then
        run_cmd "bootout existing" launchctl bootout "gui/$(id -u)/com.atlas.soak-check" 2>/dev/null || true
        run_cmd "edit plist (daily → weekly Monday 06:00 UTC)" \
            bash -c "sed -i.bak 's|<key>Hour</key><integer>6</integer>|<key>Weekday</key><integer>1</integer>|' '$LAUNCHD_AGENT'"
        run_cmd "reload launchd" \
            launchctl bootstrap "gui/$(id -u)" "$LAUNCHD_AGENT"
    else
        log "SKIP" "LaunchAgent $LAUNCHD_AGENT not found — manual launchd edit may be needed"
    fi

    # Phase 4: .omo plans 移除 (gitignored)
    log "PHASE" "4 — .omo plans 移除"
    if [[ -d "$OMO_PLAN_DIR" ]]; then
        run_cmd "rm audit-followup plan dir" rm -rf "$OMO_PLAN_DIR"
    else
        log "SKIP" "$OMO_PLAN_DIR not present"
    fi
    if [[ -f "$MAIN_PLAN_FILE" ]]; then
        run_cmd "rm main plan file" rm -f "$MAIN_PLAN_FILE"
    fi

    # Phase 5: Issue #1187 收尾驗證
    log "PHASE" "5 — Issue #1187 收尾驗證"
    if command -v gh >/dev/null 2>&1; then
        local state
        state=$(gh issue view 1187 --json state --jq '.state' 2>/dev/null || echo "UNKNOWN")
        log "ISSUE" "Issue #1187 state=$state (expect CLOSED)"
        if [[ "$state" != "CLOSED" ]]; then
            log "WARN" "Issue #1187 not CLOSED — manual close may be needed before final production deploy"
        fi
    else
        log "SKIP" "gh CLI not installed"
    fi

    # Final commit (live mode only)
    log "PHASE" "6 — Final commit"
    if [[ "$MODE" == "--live" ]]; then
        run_cmd "git add all changes" \
            git -C "$REPO_ROOT" add -A
        run_cmd "git commit chore(stage-8): post-7-day-soak cleanup automation" \
            bash -c "git -C '$REPO_ROOT' commit -m 'chore(stage-8): automated 7-day-soak cleanup (archive + rename + cron-downgrade + plan-removal)

$(cat <<'EOF'
Per docs/operations/production-rollout-runbook.md §4:
- §4.1 Archive 2 ops docs into docs/operations/archive/2026-07-15/
- §4.2 Rename scripts/staging-soak-check.sh → staging-deployment-health-check.sh
  + scripts/install-soak-automation.sh → install-staging-health-check.sh
- §4.3 LaunchAgent daily → weekly Monday 06:00 UTC
- §4.4 Remove .omo/plans/ (gitignored, post-task cleanup)
- §5 Issue #1187 already CLOSED — verify final state

Trigger: run --live only after staging Day 7 6/6 PASS per Issue #1187 §5.
Default mode is --dry-run to prevent accidental execution.
EOF
)'"
        run_cmd "git push origin main" \
            git -C "$REPO_ROOT" push origin main
    else
        log "DRY" "would git add -A && commit && push origin main"
    fi

    log "DONE" "==== post-soak-cleanup complete (mode: $MODE) ===="
    log "LOG" "full log: $LOG_FILE"
}

main "$@"
