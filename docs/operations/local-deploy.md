# 部署設定（本機 dev + production 雙機）

> **文件角色**：部署的權威說明。涵蓋 MacBook 本機 dev 與 Mac Mini production 兩種情境。
> **雙機治理（2026-09-22 起）**：開發在 MacBook、production 在 **Mac Mini**（`kaecer@kmacmini`；iMac 已退役，`KiMac` guard 僅為 legacy 防護）。
> **跨機一律用 Tailscale 名稱 `kmacmini`**（MagicDNS，解析為 Tailscale IP）；LAN IP 僅在 MacBook 位於同一網段時可用，外出時失效。
> **Mac Mini 部署實走驗證**：2026-09-23（issue #1898）—— 步驟與 10 個實踩坑見下方 §Mac Mini production 部署。
> **跨設備總則**：`~/workspace/a2a-dev/docs/governance/雙機治理憲章.md`；iMac 運維手冊：`~/workspace/a2a-dev/docs/operations/iMac-RUNBOOK.md`。

## 平台架構（方案二真相）

```
MacBook (kaecer) = 唯一開發機           iMac (kk) = 唯一 production 部署機
  ├─ code 編輯 / 測試 / PR               ├─ atlas 11 容器 + litellm 2 容器
  ├─ git push → GitHub                   ├─ git pull（只讀 clone，不 push）
  └─ 本機 dev 驗證（可 build/run）        └─ docker build + compose up（hermes 運維）
```

- **映像來源（production）**：iMac **本地 build**（`atlas-atlas:latest`），**不是** ghcr.io pull。
- **部署流程**：MacBook push → iMac `git pull` → iMac `make rebuild-all`（或 hermes 代勞）。
- **本機 dev（MacBook）**：可用 `make rebuild-all` 起本地容器驗證（不影響 iMac production，不同機器）。

## 環境變數（統一由 `~/.config/atlas-go/.env` 載入）

`config.Load()` 自動讀取以下路徑（`internal/config/config.go:70-71`）：
1. `loadEnvFile(resolveEnvFilePath())` — 專案根 `.env`
2. `loadUserEnvFile()` — `~/.config/atlas-go/.env`（使用者統一管理入口）

| 變數 | 用途 | 備註 |
|------|------|------|
| `LLM_DEEPSEEK_API_KEY` | DeepSeek（canonical 模型 `deepseek-flash` = V4.1-Flash；可用 `LLM_DEEPSEEK_MODEL` 覆寫） | 從 https://platform.deepseek.com 取得 |
| `LLM_MINIMAX_API_KEY` | MiniMax M3（coding plan） | `sk-cp-` 前綴的 minimax-cn-coding-plan key；為敘事/解釋群組的 primary（DataClass 自 ADR-012 起不再擋 provider） |
| `LLM_ANNOTATOR_API_KEY` | **向後相容** — 早期 `KimiClient` 讀此變數 | 實際值等同 `LLM_MINIMAX_API_KEY`（Kimi K2.7 因 coding plan key 限制已移除） |
| `LLM_RATIONALE_TRANSLATION_ENABLED` | 啟用 `CapabilityRationaleGeneration` hook | default `false` |
| `LLM_PRISM_SCENARIO_ENABLED` | 啟用 `CapabilityScenarioSimulation` hook | default `false` |
| `LLM_NARRATIVE_EXPLAIN_ENABLED` | 啟用 `CapabilityRegimeExplanation` + `CapabilitySentimentExplanation` | default `false` |
| `LLM_RISK_FORENSICS_ENABLED` | 啟用 `CapabilityPerformanceForensics` | default `false` |
| `LLM_SECTOR_AGENTS_ENABLED` | 啟用 `SectorAgentLLM` Plan→ToolCall→Reflect loop（Issue #719 wired） | default `false` |

> **兩機 .env 分離（2026-08-28 修正）**：MacBook 與 iMac 的 `~/.config/atlas-go/.env` 都指向 **dev DB（`atlas_dev`）**；production DB（`atlas`）的 DSN **只存在 gateway 容器環境**（`docs/operations/docker-compose.prod.yml`），**不進 .env**——讓任何「source .env 的 CLI」永遠碰不到 prod DB。**不可互相覆蓋**。
>
> ⚠️ **陷阱**：source `.env` 跑任何會 migrate 的 CLI 前，先 `echo $DATABASE_URL` 確認目標 DB（曾發生 migration 19 誤套到 atlas_dev 的事件，2026-08-28）。

## 部署流程

### 情境 A：本機 dev 驗證（MacBook）

```bash
# 1. 確認 main HEAD 已是目標版本
git fetch origin main && git log --oneline origin/main -1

# 2. 本地 build + 起容器（完整 stack）
make rebuild-all

# 3. 驗證
docker compose ps
curl -fsS http://localhost:18080/health
```

### 情境 B：production 部署（Mac Mini）

> 2026-09-23 更新：production 自 2026-09-22 起為 **Mac Mini**（`kaecer@kmacmini`）；iMac（`kk@kimac`）已退役，舊指令一律失效。完整步驟/坑見本檔 §Mac Mini production 部署。

```bash
# 1. MacBook: push 你的修改
git push origin main

# 2. Mac Mini: 同步 + 重建 + 重啟（hermes 可代勞）
ssh kaecer@kmacmini
export PATH="$HOME/.orbstack/bin:/usr/local/bin:/opt/homebrew/bin:$PATH"
export GOPROXY=https://goproxy.cn,direct
cd ~/workspace/atlas && git fetch origin main && git checkout main && git merge --ff-only origin/main && make rebuild-all

# 3. Mac Mini: 驗證正式服務
curl -fsS http://localhost:18080/health && curl -s localhost:18080/api/version
```

> **hermes 代勞**：部署是 hermes（iMac 運維員）的職責。可用 hermes-dispatch skill 派她完成
> `git pull → make rebuild-all → 驗證 /health → 回報`。

> ⚠️ **Darwinian state 同步（2026-08-27 起）**：`data/state/darwinian_history.jsonl` 不是 git
> tracked，部署不會自動帶過去。任何會**重建/重啟 atlas-go 或 atlas-cron-darwinian 容器**的部署，
> 先跑 `~/workspace/atlas/scripts/sync-darwinian.sh`（union merge，只增不減），
> 並遵守「sync 前容器必須停」的硬性規定（避免 torn line）。完整章節見
> a2a-dev `~/workspace/a2a-dev/docs/deployment/IMAC-DEPLOY-RUNBOOK.md` §2.1。

## 部署驗證

兩個 endpoint 都必須通過：

```bash
# Liveness（基礎健康）
curl -fsS http://localhost:18080/health

# LLM Readiness（深度健康 — 含 Provider 狀態、Router 版本）
curl -fsS http://localhost:18080/api/llm/health
```

預期回傳：
- `/health`：JSON `{"status":"ok",...}`
- `/api/llm/health`：JSON `{"providers":{"deepseek":{...},"minimax":{...},"kimi":{...}},"router_version":"v2.2"}`（v2.2 = ADR-012 路由表；`kimi` 這個 key 目前承載 legacy annotator adapter）

### 部署後驗證腳本

```bash
#!/usr/bin/env bash
# scripts/verify_deploy.sh
set -e
echo "=== Liveness ==="
curl -fsS http://localhost:18080/health | jq .
echo "=== LLM Health ==="
curl -fsS http://localhost:18080/api/llm/health | jq .
echo "=== Container Status ==="
docker compose ps --format json | jq -s 'map({name, state, health})'
```

## Rollback

```bash
# 退回上一個 commit 並重啟（Mac Mini）
ssh kaecer@kmacmini 'export PATH="$HOME/.orbstack/bin:/usr/local/bin:$PATH"; cd ~/workspace/atlas && git checkout <previous-sha> && make rebuild-all'
```

> **注意**：Mac Mini 用本地 build image（`atlas-atlas:latest`），Rollback = checkout 舊 commit 重建。
> 已不使用 ghcr.io tag pinning（舊模式，ghcr 已被本地 build 取代）。


## iMac 容器守護腳本（版控正本）

**為什麼要進版控**：`atlas-container-watchdog.sh` 是 iMac 唯一的自動復原機制（容器死掉時把它拉起來），
但過去只存在於 iMac 的 `~/bin/`，無法回答「iMac 上跑的是哪一版、有沒有漂移」。

**正本位置**

| 檔案 | 用途 |
|---|---|
| `scripts/ops/imac-container-watchdog.sh` | 腳本正本（launchd 每 60s 執行） |
| `scripts/ops/launchd/com.goluck.atlas-container-watchdog.plist` | launchd job 正本（`StartInterval` = 60） |

**指令**

```bash
make imac-watchdog-diff      # 比對 repo 正本與 iMac 版 sha256（漂移檢查，不一致 exit 1）
make imac-watchdog-install   # 備份 iMac 現有版本 → scp 正本 → bash -n → 重載 launchd → 再驗 sha256
```

**腳本行為（2026-09-12 起）**

1. **restart ledger**：每 60s 比對 `RestartCount / StartedAt / FinishedAt / ExitCode / OOMKilled / Error`，
   只要變化就 append 到 `~/Library/Logs/atlas-container-restarts.log`，並記錄**變化前**的值。
   這解決了「健康狀態下容器被重啟卻無法歸因」的問題（docker 不保留重啟歷史；`docker compose up -d`
   會 recreate 容器並清掉舊 log）。判讀方式見 issue #1901。
2. **start-if-down**：容器不是 `running` 才 `docker start`（不 create，避免與 compose 打架），
   啟動後 5 秒再確認；crash loop 會記 WARN 而不無限重啟。
3. 所有動作寫入 `~/Library/Logs/atlas-watchdog.log`。

**注意**：本腳本**只**啟動已存在的容器，不負責部署。**現行部署入口 = `make rebuild-all`**（Mac Mini，見下方 §Mac Mini production 部署；2026-09-23 由 issue #1898 修好並實走驗證）。
歷史：`make rebuild-all` 曾在 **iMac** 被 dev-compose guard 擋下（guard 只在本機 hostname=KiMac 時觸發；iMac 已於 2026-09-22 退役），當時需手打 docker 指令。詳見 `docs/operations/pr-lifecycle.md` §5。

## Mac Mini production 部署（2026-09-23 實走驗證，issue #1898）

> 前置：`ssh kaecer@kmacmini`。以下每一步都是實測會踩到的點，照抄即可。

```bash
cd ~/workspace/atlas

# 0) 一次性環境（重開 shell/session 才需重做）
go env -w GOPROXY=https://goproxy.cn,direct                        # 見坑①
export PATH="$HOME/.orbstack/bin:/usr/local/bin:$PATH"             # 見坑②

# 1) 對齊 main（若 repo 停在舊 branch，ff-only 會直接失敗）
git fetch origin main && git checkout main && git merge --ff-only origin/main

# 2) repo 目錄 .env（gitignored）——Mac Mini 的對外埠契約
printf 'GRAFANA_PORT=3001\nATLAS_POSTGRES_PORT=55432\n' > .env && chmod 600 .env

# 3) 重建（host bin + atlas image + cron image + 全部容器）
make rebuild-all                                                   # 見坑③④

# 4) 驗收
curl -s localhost:18080/api/version        # commit 應等於當前 main
bash ~/bin/macmini-recover.sh              # 期望 48 OK / 0 WARN / 0 FAIL
make check-binaries                        # 見坑⑦
```

### 實踩的坑（依序）
1. **`proxy.golang.org` 被 MITM**：本機 router/HiNet 把它導到 `202.39.161.53` 並以 `safebrowsing.hinet.net` 憑證攔截 → 所有 go build 以 x509 失敗。解：`GOPROXY=https://goproxy.cn,direct`（`Dockerfile.cron` 的 `GOPROXY` build-arg 註解即為同款 workaround）。
2. **非互動 ssh 沒有 docker**：`docker` 不在 PATH（`~/.orbstack/bin` / `/usr/local/bin`）。
3. **`ATLAS_GIT_COMMIT` 是 compose 的 `:?` 必填**：`make` 目標會自帶；**裸跑** `docker compose up -d` 必須 `ATLAS_GIT_COMMIT=$(git rev-parse HEAD) docker compose up -d`。
4. **cron image 必須覆蓋 compose 的每個 build-only cron service**：`CRON_IMAGE_TAGS` 曾漏掉 `atlas-cron-darwinian`（2026-09-23 修）→ 症狀 `No such image: atlas-cron-darwinian:latest`。`tests/scripts/test-binary-freshness-guard.sh` 現在以 `docker-compose.yml` 為準逐一比對。
5. **`environment:` 的 `${VAR:-}` 會蓋掉 `env_file` 同名字**：曾讓 8 個服務的 `ATLAS_LIVENESS_TOKEN` 變空、cron 的 task_liveness ping 靜默死亡（#1921/#1922 修）。
6. **host port 契約**：`atlas-postgres` **55432**、`grafana` **3001**（3000 是 gitea）、`redis` 16379、`atlas` 18080、`fubon-proxy` 18081、`onepager` 18090。repo compose 的預設（5432/3000）是 dev 用，靠步驟 2 的 `.env` 覆寫。
7. **`make check-binaries` 的語意**：比對「binary buildinfo commit vs HEAD」，所以 compose/docs-only commit 也會報 STALE（非真漂移）；script 另有 `TEMP_FILES[@]` 空陣列在 bash 3.2 崩潰的 bug（#1923 修）。
8. **fresh worktree 缺 gitignored 前端 dist** → `ci-gate` 失敗（`embed: pattern all:dist`）：從主 worktree `cp -r admin_web/dist client_web/dist`。
9. **golangci-lint cache 會掃到已刪除的相鄰 worktree**（假 issue）→ `golangci-lint cache clean`。
10. **重開機後一鍵恢復**：`bash ~/bin/macmini-recover.sh`（`imac-recover.sh` 為相容 symlink；MacBook wrapper：`macmini-recover`）。launchd idle 排程 agent（watchdog/orbstack，`state = not running` 且 `last exit = 0`）屬正常。

### Rollback
```bash
cd ~/workspace/atlas && git log --oneline -3        # 找上一個已知良好 commit
git checkout <prev> && make rebuild-all             # 重建並重啟容器（data volume 不動）
# 只回滾 compose 層：git checkout <prev> -- docker-compose.yml && ATLAS_GIT_COMMIT=$(git rev-parse HEAD) docker compose up -d
```
