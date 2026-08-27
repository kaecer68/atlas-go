#!/bin/bash
# atlas-container-watchdog.sh — 檢查 atlas 核心容器是否存活，不跑就啟動
# 由 launchd com.goluck.atlas-container-watchdog 每 60s 觸發
# 2026-08-27 建立（PR #1695 事件後 32h silent 的教訓：容器死了沒人拉）
#
# 設計原則：
# - 只負責「start 已存在的 container」，不 create（避免跟 compose 打架）
# - crash-loop 保護：啟動後 5s 內又死 → 記錄 WARN 並跳過，不無限重啟
# - 所有動作寫 log，供後續調查

DOCKER=/usr/local/bin/docker
LOG="$HOME/Library/Logs/atlas-watchdog.log"

# 要監控的容器（space-separated）
CONTAINERS="atlas-go-imac atlas-postgres"

log() { echo "$(date '+%Y-%m-%d %H:%M:%S') $1" >> "$LOG"; }

for name in $CONTAINERS; do
    state=$("$DOCKER" inspect -f '{{.State.Status}}' "$name" 2>/dev/null)
    if [ "$state" = "running" ]; then
        continue
    fi
    log "WATCH: $name state=$state -> starting"
    if "$DOCKER" start "$name" >> "$LOG" 2>&1; then
        # crash-loop 保護：等 5 秒確認還活著
        sleep 5
        state2=$("$DOCKER" inspect -f '{{.State.Status}}' "$name" 2>/dev/null)
        if [ "$state2" != "running" ]; then
            log "WARN: $name failed to stay up (state=$state2 after start) — crash loop? manual intervention needed"
        else
            log "OK: $name is running again"
        fi
    else
        log "ERROR: docker start $name failed"
    fi
done

