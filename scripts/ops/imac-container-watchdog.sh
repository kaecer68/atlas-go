#!/usr/bin/env bash
# scripts/ops/imac-container-watchdog.sh — production 容器守護腳本（版控正本）
#
# 為什麼在 repo 裡：這支腳本是 production 唯一的自動復原機制，先前只存在於
# 主機的 ~/bin/（未版控），造成「改了哪一版、有沒有漂移」無法回答。
#
# 安裝（production = **Mac Mini**，2026-09-26 更新；檔名與 Makefile target 的 `imac-`
# 前綴屬歷史債，不改）：
#   make imac-watchdog-install          # scp 到 Mac Mini 的 ~/bin 並重載 launchd
#   make imac-watchdog-diff             # 比對 repo 版與主機版的 sha256（漂移檢查）
#   ✅ 2026-09-26 起可用（PR #2041）：Makefile 的 `IMAC_HOST` 已由退役的 `kk@kimac`
#      改指 `kaecer@kmacmini`，`WATCHDOG_DST` 改指 Mac Mini 的實查安裝路徑
#      `/Users/kaecer/bin/atlas-container-watchdog.sh` ⇒ 上面兩個 target 都可直接跑。
#      等價手動指令另見 docs/operations/local-deploy.md §容器守護腳本（版控正本）。
#   安裝位置（Mac Mini）= ~/bin/atlas-container-watchdog.sh（實查存在，2026-09-25 版）。
#
# 由 launchd com.goluck.atlas-container-watchdog 每 60s 觸發
# （plist 正本：scripts/ops/launchd/com.goluck.atlas-container-watchdog.plist）
#
# 歷史：
# - 2026-08-27 建立（PR #1695 事件後 32h silent 的教訓：容器死了沒人拉）
# - 2026-09-12 追加 restart ledger（#1901）：docker 不保留重啟歷史、recreate 會清掉舊 log，
#   導致健康狀態下的重啟無法歸因。本版每 60s 比對容器狀態變化並落到
#   ~/Library/Logs/atlas-container-restarts.log（含「變化前」的值 = 上一次結束的線索）。
# - 2026-09-30 追加「計畫性 vs 異常」分類（FU-20260930-10 方案②）：先前**每次部署**都發一則
#   warning（hermes QC 每日檢查的 `[FIRING] watchdog.restart-detect`），因為計畫性部署與
#   異常重啟長得一樣。本版只改**分類**、不改**偵測**（偵測閾值與條件一字不動）：
#     RESTART-DETECT   = 同映像重啟（在部署窗口外）⇒ 維持 warning ✓（崩潰迴圈不得靜音）
#     RESTART-EXPECTED = 計畫性重啟 ⇒ expected／INFO ✓ **仍寫 ledger**（重啟歷史不消失 ✓）
#
# 【分類規則（兩條獨立證據，任一成立即 planned）】
#   ① 映像變更：本輪 `docker inspect {{.Image}}` 的短 ID ≠ ledger 上一筆同容器行的 `image=`。
#      基準取自 ledger（不是 state 檔）：ledger 是跨版本、跨重啟、跨 state 重建都還在的證據鏈。
#   ② 部署窗口標記：`MARKER` 檔存在 ⇒ 窗內重啟＝planned。涵蓋「沒換映像」的部署
#      （例：`docker compose restart` 只為載入新 config）——此時 image 不變，① 不會成立。
#      MARKER 由**部署方**在窗口內建立、窗口結束後刪除（本腳本唯讀它，不建立、不刪除）。
#      ⚠️ 殘留的 MARKER 會讓窗內所有重啟（含崩潰迴圈）都被歸為 planned ⇒ 部署 wrapper 有責清除。
# ★ 保守預設（業主指定）：ledger 舊行**沒有** `image=` 欄位 ⇒ 映像視為**未知**。未知**不是**
#   變更 ⇒ 不因映像而降級，該次維持 RESTART-DETECT／warning（`why=image-unknown`）。把「我不知道」
#   降級成 INFO，等於給崩潰迴圈一條靜音路徑 ⇒ 這條界線由 tests/scripts/test-watchdog-restart-classify.sh
#   的 case 4 釘住。升級後第一筆事件因此**可能仍是 warning** —— 那是刻意設計，不是漏改。
#   例外：②（窗口標記）是**獨立的**明確訊號，它成立時即使映像未知也照 planned 處理（`why=deploy-window`）。
#   若 ①② 同時成立 ⇒ `why=image-change`（較具體者勝）。
#
# 【ledger 行格式（欄位契約）】
#   舊： `<時間> <日期> RESTART-DETECT <name> new=<6 欄狀態> prev=<6 欄狀態>`
#   新： `<時間> <日期> <kind> <name> new=<6 欄狀態> prev=<6 欄狀態> image=<image 短 ID>
#          image_name=<可讀標籤> prev_image=<上一筆 image 短 ID 或空> marker=<0|1> why=<原因>`
#   - `kind` 仍是**第 3 欄**、容器名仍在 kind 之後 ⇒ 既有的「以第 3 欄當 kind、以行內第一個
#     `atlas-…` token 當容器名」解析器不受影響（見下方消費者的相容性說明）。
#   - `image=` / `prev_image=` 為**空**代表未知：刻意**不**寫字面 `unknown`（那會變成一個永遠
#     比不中的值 ⇒ 之後每次比較都被判成「變更」⇒ 所有重啟都被歸為 planned）。語意由 `why=` 承載。
#   - `why=` ∈ {image-change, deploy-window, same-image, image-unknown}；
#     `BASELINE` 行同樣追加 `image=` / `image_name=`（作為後續比對的第一個基準）。
#
# 【既有消費者的相容性（改動前已實查）】
#   通知橋 = a2a-dev `scripts/ops/watchdog-ledger-notify.sh`（Mac Mini ~/bin/），它對 restart ledger：
#     - `kind = $3`（行內第 3 欄）⇒ 新欄位一律接在後面，不影響；
#     - 容器名 = 行內**第一個** `atlas-[a-z0-9-]+` ⇒ 容器名仍排在 `image_name=`（例 atlas-atlas:v2）
#       **之前** ⇒ 不會被映像標籤搶走；
#     - 預設 `WLN_RESTART_PATTERN=RESTART-DETECT` ⇒ `RESTART-EXPECTED` 預設**不通知**
#       （= 計畫性重啟降為 INFO／只進 ledger）；要連 INFO 一起收就設
#       `WLN_RESTART_PATTERN='RESTART-DETECT|RESTART-EXPECTED'`（opt-in，不必改它的程式）。
#   本腳本的 `$LOG` 另外多寫一行 `INFO: …`：主 ledger 的 kind 過濾式是 `WATCH|OK|ERROR|WARN`
#   ⇒ `INFO` 不在其中，同樣不會多發一則通知（`RESTART-EXPECTED` 也不在其中）。
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

# 部署窗口標記（分類規則 ②；可由部署方覆寫路徑）。**唯讀**：本腳本不建立、不刪除。
MARKER="${A2A_DEPLOY_MARKER:-/tmp/atlas-deploy-window}"

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

# ── 0) 分類輔助（見檔頭「分類規則」；唯讀、無副作用）────────────────────────
# `docker inspect` 的 image ID（`sha256:<64hex>`）正規化成 `docker ps` 的 12 碼短 ID。
# 取不到 ⇒ 印空字串＝**未知**（未知不可當成「變更」，見檔頭 ★）。
image_short() {
    local raw
    raw=$("$DOCKER" inspect -f '{{.Image}}' "$1" 2>/dev/null) || return 0
    raw=${raw#sha256:}
    [ -n "$raw" ] && printf '%s' "${raw:0:12}"
    return 0
}

# 可讀的映像標籤（`{{.Config.Image}}`，例 atlas-atlas:v2）。**只給人看**，不參與判定。
image_name_of() {
    "$DOCKER" inspect -f '{{.Config.Image}}' "$1" 2>/dev/null || true
}

# ledger 中**最後一筆**提及該容器那行的 `image=` 值；沒有（含舊格式行）⇒ 空＝未知。
# 只用 grep/awk（macOS 的 bash 3.2 沒有 tac；ledger 每日行數為個位數，整檔掃描成本可忽略）。
ledger_last_image() {
    [ -f "$RESTART_LEDGER" ] || return 0
    grep -F " $1 " "$RESTART_LEDGER" 2>/dev/null | awk '
        { img = ""; for (i = 1; i <= NF; i++) if ($i ~ /^image=/) img = substr($i, 7) }
        img != "" { last = img }
        END { if (last != "") print last }'
    return 0
}

# ── 1) restart ledger（健康狀態下的重啟也要留下證據）────────────────────────
for name in $CONTAINERS; do
    info=$("$DOCKER" inspect -f '{{.RestartCount}}|{{.State.StartedAt}}|{{.State.FinishedAt}}|{{.State.ExitCode}}|{{.State.OOMKilled}}|{{.State.Error}}' "$name" 2>/dev/null) || continue
    [ -z "$info" ] && continue
    # image 用**獨立**的 inspect 取（刻意不併進上面的 info）：info 是 state 檔的比較鍵，
    # 一旦它的字串改變，升級當下每個容器都會多發一筆「假重啟」⇒ 不能共用同一條 format。
    cur_image=$(image_short "$name")
    cur_image_name=$(image_name_of "$name")
    prev_image=$(ledger_last_image "$name")
    marker=0
    [ -e "$MARKER" ] && marker=1
    f="$STATE_DIR/$name.state"
    prev=""
    [ -f "$f" ] && prev=$(cat "$f")
    if [ -z "$prev" ]; then
        printf '%s BASELINE %s %s image=%s image_name=%s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$name" "$info" "$cur_image" "$cur_image_name" >> "$RESTART_LEDGER"
    elif [ "$info" != "$prev" ]; then
        planned=0
        why=same-image
        if [ -z "$cur_image" ] || [ -z "$prev_image" ]; then
            why=image-unknown
        elif [ "$cur_image" != "$prev_image" ]; then
            planned=1
            why=image-change
        fi
        if [ "$marker" = 1 ]; then
            planned=1
            [ "$why" = "image-change" ] || why=deploy-window
        fi
        if [ "$planned" = 1 ]; then
            kind=RESTART-EXPECTED
        else
            kind=RESTART-DETECT
        fi
        printf '%s %s %s new=%s prev=%s image=%s image_name=%s prev_image=%s marker=%s why=%s\n' \
            "$(date '+%Y-%m-%d %H:%M:%S')" "$kind" "$name" "$info" "$prev" "$cur_image" "$cur_image_name" "$prev_image" "$marker" "$why" >> "$RESTART_LEDGER"
        log "$kind $name new=$info prev=$prev image=$cur_image image_name=$cur_image_name prev_image=$prev_image marker=$marker why=$why"
        if [ "$planned" = 1 ]; then
            log "INFO: $name restart is planned (why=$why) - recorded in ledger as RESTART-EXPECTED; not notified as warning"
        fi
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
