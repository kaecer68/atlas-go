#!/usr/bin/env bash
# scripts/ops/imac-container-watchdog.sh — iMac production 容器守護腳本（版控正本）
#
# 為什麼在 repo 裡：這支腳本是 iMac 唯一的自動復原機制，先前只存在於
# iMac 的 ~/bin/（未版控），造成「改了哪一版、有沒有漂移」無法回答。
#
# 安裝（iMac）：
#   make imac-watchdog-install          # scp 到 iMac 的 ~/bin 並重載 launchd
#   make imac-watchdog-diff             # 比對 repo 版與 iMac 版的 sha256（漂移檢查）
#
# 由 launchd com.goluck.atlas-container-watchdog 每 60s 觸發
# （plist 正本：scripts/ops/launchd/com.goluck.atlas-container-watchdog.plist）
#
# 歷史：
# - 2026-08-27 建立（PR #1695 事件後 32h silent 的教訓：容器死了沒人拉）
# - 2026-09-12 追加 restart ledger（#1901）：docker 不保留重啟歷史、recreate 會清掉舊 log，
#   導致健康狀態下的重啟無法歸因。本版每 60s 比對容器狀態變化並落到
#   ~/Library/Logs/atlas-container-restarts.log（含「變化前」的值 = 上一次結束的線索）。
# atlas-container-watchdog.sh — 檢查 atlas 核心容器是否存活，不跑就啟動
# 由 launchd com.goluck.atlas-container-watchdog 每 60s 觸發
# 2026-08-27 建立（PR #1695 事件後 32h silent 的教訓：容器死了沒人拉）
#
# 設計原則：
# - 只負責「start 已存在的 container」，不 create（避免跟 compose 打架）
# - crash-loop 保護：啟動後 5s 內又死 → 記錄 WARN 並跳過，不無限重啟
# - 所有動作寫 log，供後續調查
#
# 2026-09-12 追加（#1901 事後檢討）：
# - **restart ledger**：docker 不保留重啟歷史，而 `docker compose up -d` 會 recreate 容器並清掉舊 log，
#   導致「容器在健康狀態下被重啟」完全無法歸因（2026-09-11 於 40 分鐘內發生 4 次，exit=0、無 shutdown log）。
#   本版每 60s 比對 RestartCount / StartedAt / FinishedAt / ExitCode / OOMKilled / Error，
#   只要任一改變就 append 一行到 $RESTART_LEDGER（同時記「變化前」的值 = 上一次結束的線索）。
# - ledger 只增不刪，供日後比對 app log 的 boot marker（server_startup_ok / dashboard api listening）。

DOCKER=/usr/local/bin/docker
LOG="$HOME/Library/Logs/atlas-watchdog.log"
STATE_DIR="$HOME/Library/Logs/atlas-watchdog-state"
RESTART_LEDGER="$HOME/Library/Logs/atlas-container-restarts.log"

# 要監控的容器（space-separated）
#
# 2026-09-25 修正（監控缺口調查 任務 E；報告 Q3-5(a)）：原值 `atlas-go-imac` 是 iMac
# 時代的容器名。Mac Mini 上主 API 容器實名為 **`atlas-go`**（`-imac` 後綴在 2026-09-22
# 遷移後退役），於是本腳本在 Mac Mini 上對主容器完全無效：
#   docker inspect atlas-go-imac     → 空 → state="" （restart ledger 靜默跳過）
#   docker start   atlas-go-imac     → Error response from daemon: No such container
# 實測後果（2026-09-25 11:33）：每 60 秒一行 `WATCH: atlas-go-imac state= -> starting` /
# `ERROR: docker start atlas-go-imac failed`，各 2,554 行（該 log 共 10,219 行）、
# 從 2026-09-23T13:37+0800 起持續 45 小時以上。而它是 PR #1695(32h 沉默) 之後唯一的
# 自動復原機制 → 主 API 容器其實一直沒有復原保護。
# 對照證據：同一個 60 秒迴圈對 `atlas-postgres`（名字正確）在 ledger 有正常紀錄，
# 且 `/usr/local/bin/docker` 在 Mac Mini 存在 → 失敗原因就是這個名字，不是缺 docker。
#
# 檔名 `imac-container-watchdog.sh` 屬歷史債（launchd label / 安裝路徑 / Makefile
# target 都指向它）→ **不改檔名**，只在此註明 Mac Mini 的實名。
# 若要新增受監控容器，請用 `docker ps --format '{{.Names}}'` 的實名。
CONTAINERS="atlas-go atlas-postgres"

log() { echo "$(date '+%Y-%m-%d %H:%M:%S') $1" >> "$LOG"; }

mkdir -p "$STATE_DIR"

# ── 1) restart ledger（健康狀態下的重啟也要留下證據）────────────────────────
for name in $CONTAINERS; do
    info=$("$DOCKER" inspect -f '{{.RestartCount}}|{{.State.StartedAt}}|{{.State.FinishedAt}}|{{.State.ExitCode}}|{{.State.OOMKilled}}|{{.State.Error}}' "$name" 2>/dev/null) || continue
    [ -z "$info" ] && continue
    f="$STATE_DIR/$name.state"
    prev=""
    [ -f "$f" ] && prev=$(cat "$f")
    if [ -z "$prev" ]; then
        printf '%s BASELINE %s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$name" "$info" >> "$RESTART_LEDGER"
    elif [ "$info" != "$prev" ]; then
        printf '%s RESTART-DETECT %s new=%s prev=%s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$name" "$info" "$prev" >> "$RESTART_LEDGER"
        log "RESTART-DETECT $name new=$info prev=$prev"
    fi
    echo "$info" > "$f"
done

# ── 2) start-if-down（原本行為，未改變）─────────────────────────────────────
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
