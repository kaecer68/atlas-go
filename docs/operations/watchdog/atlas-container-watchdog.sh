#!/bin/bash
# ⛔ 已淘汰，勿用（2026-09-26 標記） — 本檔是**未被任何檔案引用**的舊副本
#   （盤查：`git grep 'operations/watchdog' origin/main` = 0 命中；本 PR 新增的說明行本身會命中，故須指定 ref）。
#   正本 = scripts/ops/imac-container-watchdog.sh（**唯一**被 Makefile target
#   `imac-watchdog-diff` / `imac-watchdog-install` 引用、也是文件指向的那一份）。
#   差異：
#     1. 本檔停在 2026-08-27 初版：**沒有 restart ledger**（正本 2026-09-12 起有，
#        見 #1901），因此無法回答「健康狀態下容器被重啟」這類問題。
#     2. `CONTAINERS` 仍是已退役的 `atlas-go-imac`。Mac Mini 上主 API 容器實名 = `atlas-go`
#        （`-imac` 後綴 2026-09-22 遷移後淘汰）→ 照本檔安裝只會每 60s 對不存在的容器
#        `docker start`，log 爆量且零復原能力。正本已於 2026-09-25 修正為 `atlas-go`。
#   安裝位置（Mac Mini）= ~/bin/atlas-container-watchdog.sh（實查存在，2026-09-25 11:25 版）。
#   保留此檔僅為歷史；現行說明見 docs/operations/local-deploy.md §容器守護腳本（版控正本）。
#
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

