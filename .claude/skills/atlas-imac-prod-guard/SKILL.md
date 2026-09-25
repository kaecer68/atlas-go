---
name: atlas-imac-prod-guard
description: "[已退役 / RETIRED 2026-09-22 — iMac 已出售，本流程一律不可照做；改用 docs/operations/MACMINI-RECOVER.md + a2a-dev scripts/prod-guard.sh] 歷史記錄：iMac (KiMac) 上的 docker compose / docker rm / make rebuild 前置護欄（比對 container_name / image / port / password 與 docs/operations/docker-compose.prod.yml）。Triggers: iMac rebuild, docker rm atlas-*, make rebuild-all, make rebuild-atlas, docker compose up -d, PR #1695 follow-up"
---

# atlas-imac-prod-guard (2026-08-27) — ⛔ 已退役（2026-09-22）

> ## ⛔ 本 skill 已退役，內容**不可照做**
>
> **退役理由**：iMac (KiMac) 已於 **2026-09-22 出售退役**；生產主機改為 **Mac Mini**
> （`ssh kaecer@kmacmini`／`KMacMini.local`）。本檔所有指令都以 iMac 為前提，現在**全部失效**：
>
> | 本檔寫的 | 現況 |
> |---|---|
> | `ssh kk@kimac`、`/Users/kk/...` | ✗ 該帳號/主機不存在；現為 `kaecer@kmacmini`、`/Users/kaecer/...` |
> | `LocalHostName == KiMac` 為觸發條件 | ✗ 現為 `KMacMini.local` |
> | `make rebuild-all` / `make rebuild-atlas` / `Makefile.prod` | ✗ 已移除 |
> | `docs/operations/iMac-RUNBOOK.md`、`IMAC-STARTUP-SOP.md` | ✗ 檔案已刪除（iMac 退役時一併移除） |
> | live 容器由 `docker-compose.prod.yml` 起 | ✗ 實際由 repo 根的 `docker-compose.yml` 起（`docker inspect` 可查證） |
>
> **現行對應做法（改用這些）**：
> - 生產主機護欄：a2a-dev `scripts/prod-guard.sh`（偵測 `~/.a2a/PRODUCTION_HOST`；
>   **不要在生產機跑 CI/重建**）。
> - 復原與重開機：`docs/operations/MACMINI-RECOVER.md`、a2a-dev `scripts/macmini-recover.sh`。
> - 部署：a2a-dev `docs/deployment/MACMINI-DEPLOY-RUNBOOK.md`。
> - 監控設定樹：**只有一棵權威樹 = repo 的 `monitoring/`** ✗ `~/workspace/atlas-monitoring/`
>   （歷史殘留）；詳見 `.claude/skills/atlas-monitoring-observability/`。
>
> 本檔保留為 **歷史事故記錄**（PR #1695 的 postgres 覆蓋事件，是真實教訓：compose 檔共用
> `container_name` 且沒帶 `-f` ⇒ 用 dev 預設值覆蓋 prod 容器）。**不要刪除本檔的歷史敘述**。

## 問題背景

2026-08-26 01:19-02:20 UTC，PR #1695（Warp agent）在 iMac (KiMac) 上跑 `make rebuild-all` 修 redis port collision，結果：

1. `make rebuild-atlas` 內部 `docker compose up -d atlas` 沒帶 `-f`，預設走 dev `docker-compose.yml`
2. dev compose 的 `atlas` service `depends_on: postgres`，自動重建 `atlas-postgres` 容器
3. 重建用 dev 預設值（`POSTGRES_PASSWORD=atlas`、port `5432:5432`），覆蓋 prod `atlas-postgres`（密碼 `atlas_prod_pwd_2026`、port `55432:5432`）
5. `atlas-go-imac` 用 prod compose DATABASE_URL 連不到新 postgres，crash loop → 502 bad gateway on atlas.goluck.uk
6. 32+ 小時 silent failure（alert 斷鏈是另一個 issue）

**根本**：`docker-compose.yml` 與 `docker-compose.prod.yml` 共用 `container_name: atlas-postgres`，沒有 SSOT 驗證。

## 何時呼叫

任何時候在 iMac (KiMac) 上要：

- 跑 `make rebuild-all` / `make rebuild-atlas` / `make rebuild-cron`
- 跑 `docker compose up -d`
- 跑 `docker rm` / `docker compose down` / `docker compose rm`
- 編輯 `docker-compose.yml` / `docker-compose.prod.yml` / `~/.config/atlas-go/.env`
- 評估任何 PR 是否影響 prod 狀態

## 操作流程（5 步）

### 1. 確認當前在哪台機器

```bash
hostname
scutil --get LocalHostName   # 已退役：iMac 應回 "KiMac"（該機已出售）
```

**`LocalHostName != KiMac`** → 跳過 skill（MacBook / Linux / CI 不適用）。
**2026-09-22 起一律結果都是「跳過」**——本 skill 已退役，見頂部警示。

### 2. 抓 SSOT

```bash
# 文件化 SSOT
docker compose -f docker-compose.prod.yml config 2>&1 | grep -A 1 DATABASE_URL
cat ~/.config/atlas-go/.env  # 對應欄
```

### 3. 抓現況

```bash
/usr/local/bin/docker inspect atlas-postgres --format '{{.Config.Env}} {{.HostConfig.PortBindings}}' 2>&1
/usr/local/bin/docker inspect atlas-go-imac --format '{{.Config.Env}}' 2>&1 | grep -E 'DATABASE_URL|ATLAS_ENV'
```

### 4. 比對 4 個不變量

| 不變量 | SSOT（prod compose） | 現況 | 偏差處置 |
|---|---|---|---|
| `container_name` | `atlas-postgres` | `docker ps --filter name=atlas-postgres` | 名稱一致但密碼/port 漂移 |
| `image` | `atlas-atlas:latest` | `docker inspect atlas-go-imac --format '{{.Config.Image}}'` | image 不一致 = 跑錯 binary |
| `port` | `55432:5432` | `docker inspect atlas-postgres --format '{{.HostConfig.PortBindings}}'` | port 不是 55432 = 走 ssh tunnel 或 dev compose |
| `password` | `atlas_prod_pwd_2026` | `docker exec -e PGPASSWORD=atlas_prod_pwd_2026 atlas-postgres psql -h 127.0.0.1 -U atlas -d atlas -tAc "SELECT 1;"` | auth failed → ALTER USER 修正並記錄 |

### 5. 決策樹

```
跑 'make rebuild-all'?
├─ 是 → 用 ALLOW_DEV_REBUILD_ON_IMAC=1 (Makefile 已加 guard)
└─ 否 → 繼續

跑 'docker rm atlas-postgres'?
├─ 是 → 先確認 atlas-postgres 的 image/port/password 與 prod compose 一致
│        不一致時該 rm = 移除 prod → 拒絕並查 SSOT
└─ 否 → 繼續

跑 'docker compose up -d postgres'?
├─ 是 → 確認 -f docker-compose.prod.yml (prod 才有 postgres service 的 SSOT 設定)
│        沒指定 -f = dev compose → 拒絕
└─ 否 → 繼續

跑 'docker run ... atlas-postgres'?
├─ 是 → 用 prod compose 的 -p 55432:5432 + -v atlas-postgres-data:... (注意 volume 是 atlas-postgres-data 不是 atlas_postgres-data)
└─ 否 → 繼續
```

## 失敗模式 (red flags)

| 症狀 | 紅燈 |
|---|---|
| `atlas-postgres` port 是 `5432:5432` 而非 `55432:5432` | ⚠️ 走 dev compose 或上次 prod compose 沒完整套用 |
| `atlas-postgres` Mount 用 `atlas_postgres-data` (底線) 而非 `atlas-postgres-data` | ⚠️ 裝到 dev volume；prod 573MB 資料被隔離 |
| `atlas-go-imac` DATABASE_URL 是 `host.docker.internal:55432` 但連不上 | ⚠️ ssh tunnel PID 1008 佔 55432 → `kill 1008` 先 |
| `atlas-postgres` 沒有 `POSTGRES_PASSWORD` env 或密碼不對 | ⚠️ 直接 ALTER USER（POSTGRES_PASSWORD 只在 initdb 時生效）|
| `atlas-go-imac` RestartCount=13+ 還在 crash loop | ⚠️ 不要無限重啟，先 root cause |

## 復原 SOP（⛔ 歷史記錄 — iMac 專用，已失效；現行走 `docs/operations/MACMINI-RECOVER.md`）

> 下列步驟帶 `/Users/kk/...` 與 `ssh kk@kimac`，在 2026-09-22 之後**無法執行**。
> 保留原樣只為還原當時處置（教訓是「用 prod compose 明確指定 `-f` 才 `up -d`」）。

1. `ssh kk@kimac "ps -p 1008 -o command"` 確認 ssh tunnel 是否 rogue（若是 `ssh -L 55432:...`，`kill 1008`）
3. `cd /Users/kk/workspace/atlas && /usr/local/bin/docker compose -f docker-compose.prod.yml stop postgres`
4. `/usr/local/bin/docker rm -f atlas-postgres`（volume 保留）
5. `/usr/local/bin/docker run -d --name atlas-postgres --restart unless-stopped -e POSTGRES_USER=atlas -e POSTGRES_PASSWORD=atlas_prod_pwd_2026 -e POSTGRES_DB=atlas -p 55432:5432 -v atlas-postgres-data:/var/lib/postgresql/data --health-cmd "pg_isready -U atlas -d atlas" timescale/timescaledb:2.26.4-pg15`
6. 驗密碼：`docker exec -e PGPASSWORD=atlas_prod_pwd_2026 atlas-postgres psql -h 127.0.0.1 -U atlas -d atlas -tAc "SELECT 1;"`
7. `/usr/local/bin/docker start atlas-go-imac`
8. `curl -s -o /dev/null -w '%{http_code}\n' https://atlas.goluck.uk/health`（期望 200）
9. 驗 prod 資料新鮮度：`docker exec atlas-postgres psql -U atlas -d atlas -tAc "select max(date) from quotes;"`

## 相關文件

- ✗ `docs/operations/iMac-RUNBOOK.md`（**已刪除**，iMac 退役）
- ✗ `docs/operations/IMAC-STARTUP-SOP.md`（**已刪除**，iMac 退役）
- `docs/operations/MACMINI-RECOVER.md`（**現行**：Mac Mini 復原）
- `docs/operations/docker-compose.prod.yml`（prod **文件化參考**；live 定義是 repo 根 `docker-compose.yml`）
- AGENTS.md（高頻陷阱）
- `.claude/skills/atlas-monitoring-observability/`（alert chain 健康 + monitoring 單一設定樹）

## 配套 PR

- `fix(infra/makefile): fail-closed when make rebuild-atlas runs on iMac` (#1703)
- `feat(monitoring): atlas-go target-down alert rule`
- `chore(mmemory): atlas_prod_state_drift_sop`
