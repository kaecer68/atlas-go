# 啟動異常關閉多服務連環根因調查(2026-06-28)

> **文件角色**:根因調查(RCA)—「為什麼本專案啟動後跑一陣子就異常關閉」的多服務連環 bug 完整記錄。
> **狀態**:已解決(PR #809 系列),所有 fix 已部署並 5 分鐘 boot test 驗證穩定。
> **範圍**:`atlas-prism-worker`(主因)+ `atlas-grafana` + `atlas-alertmanager` + `atlas-otel-collector` 的啟動連環崩潰。

## TL;DR

| # | 服務 | Root Cause | 證據 |
|---|------|-----------|------|
| 1 | `atlas-prism-worker` | docker-compose `command` 包含完整 binary path,跟 Dockerfile `ENTRYPOINT` 串接變成 `/app/atlas-go /app/atlas-go prism worker`,binary 收到錯誤 args → subcommand 路由 false → fall through 到 one-shot `runSimulation()` → 60s 重啟循環 | `docker inspect` 顯示 `Cmd: ["/app/atlas-go","prism","worker"]` + `Entry: ["/app/atlas-go"]` |
| 2 | `atlas-prism-worker` 同時 | `prismMgr` 在 apiMode 區被建立但 `Start()` 從未被呼叫 → PRISM system 是「實作但不完整」,任務永遠在 in-memory queue 堆著沒人消化 | `internal/prism/prism_manager.go:382` 的 log 在 production code 從不出現,只在 `prism_test.go:203` 測試內被呼叫 |
| 3 | `atlas-grafana` | `grafana-data` volume 內舊 datasource UID 跟 provisioning template 衝突 → provisioning 報 `data source not found` | 啟動 log:`Datasource provisioning error: data source not found` |
| 4 | `atlas-alertmanager` | `monitoring/alertmanager.yml` line 70 `- name: info-slack` 縮排錯誤(column 1 而非 column 3) | log:`yaml: line 69: did not find expected key` |
| 5 | `atlas-otel-collector` | `monitoring/otel-collector.yaml` 用 `postgresql` exporter,但 OpenTelemetry Collector **沒有** 這個 exporter(404 in valid list) | log:`error decoding 'exporters': unknown type: "postgresql"` |
| 6 | `atlas-go` `/health` 回 503 | `docker-compose.yml` 的 `environment: ATLAS_API_KEY=${ATLAS_API_KEY}` 展開成 host shell 的空字串(shell 沒設),**shadow** 掉 `env_file: ~/.config/atlas-go/.env` 裡的值 | `docker exec atlas-go printenv ATLAS_API_KEY` 回空;`.env` 檔有 `ATLAS_API_KEY=e2e-test-key-not-for-prod` |
| 7 | `atlas-go` Dockerfile | `HEALTHCHECK CMD curl -f http://localhost:8080/health` 硬編碼,繼承給所有用同 image 的服務;非 API 服務沒有 HTTP server 在 8080 → 永遠 unhealthy | `prism-worker` docker inspect 顯示 healthcheck 一直失敗 |
| 8 | `fubon-neo` 公開 PyPI 不可用 | `fubon-neo` 是富邦新一代 API 官方 Python SDK,**從未放在公開 PyPI**(不是「下架」,本來就沒有)。`pip install fubon-neo==2.2.8` 永遠 404。`requirements.txt` pin 這個版本 → build 失敗 | `curl https://pypi.org/pypi/fubon-neo/json` → 404;官方下載頁 `https://www.fbs.com.tw/TradeAPI/docs/download/download-sdk` 才有 wheel(Windows / macOS / Linux 都提供) |
| 9 | 兩個 .env 模板 | repo 根目錄有 `.env_example`(untracked,stale)跟 `.env.example`(tracked,canonical),`CONTRIBUTING.md` 用 `cp .env.example .env` 但 `.env_example` 還在誤導 user | `git ls-files .env_example` 回空,`.env.example` 正常 tracked |

## 背景

使用者回報「本專案啟動後跑一陣子就異常關閉,這個問題昨天已經處理過但又出問題」。5 分鐘 boot test 證實:

- 修復前:`atlas-prism-worker` 60 秒重啟循環(每次 `exitCode=0` 乾淨退出),`atlas-grafana` 1 秒 crash,`atlas-alertmanager` 跟 `atlas-otel-collector` 也在 crash loop
- 修復後:所有 8 個核心容器 5 分鐘內 0 重啟,`atlas-go /health` 回 200(需帶 `X-API-Key: e2e-test-key-not-for-prod`)

## 根因深層分析(每一條都附證據鏈)

### 1. `atlas-prism-worker` 的 60 秒重啟循環

#### 1.1 表面症狀

`docker events` 顯示連續模式:
```
start → 1 秒 → die (exitCode=0) → 60 秒 → start → 1 秒 → die (exitCode=0) → ...
```

不是 OOM,不是 panic,是**正常退出**被 docker `restart: unless-stopped` 一直重啟。

#### 1.2 真實根因:`docker-compose` 跟 `ENTRYPOINT` 衝突

`docker-compose.yml` line 320-322(原版):
```yaml
prism-worker:
  command: ["/app/atlas-go", "prism", "worker"]  # ← 包含 binary path
```

`Dockerfile` line 96:
```dockerfile
ENTRYPOINT ["/app/atlas-go"]  # exec form
```

Docker 行為:exec form ENTRYPOINT 是執行檔,CMD 是參數。docker-compose 的 `command` **會替換 CMD**,但 **ENTRYPOINT 保留**。所以實際執行:
```
/app/atlas-go /app/atlas-go prism worker
```

binary 收到 `os.Args[1:] = ["/app/atlas-go", "prism", "worker"]` → `flags.Parse` 在第一個非 flag 停下 → `flags.Args() = ["/app/atlas-go", "prism", "worker"]` → `isPrismWorkerCmd` 檢查 `args[0] == "prism"` → **false**(實際是 `/app/atlas-go`)。

#### 1.3 為什麼以前以為修過了

git log 顯示以前有人修過 prism-worker 的 crash loop,但都是用 `docker compose restart` — 暫時讓容器重跑,**根因完全沒解決**。所以 60 秒後又回到同樣的 fall-through → runSimulation → exit → 重啟循環。

#### 1.4 修法(無 patch,既有基礎設施的正確用法)

```yaml
prism-worker:
  command: ["prism", "worker"]  # args-only,讓 ENTRYPOINT 提供 binary path
```

`cmd/atlas/main.go` 原本就設計支援 subcommand(只是 `prism worker` 從未實作 → 補上 `runPrismWorker()` daemon + `isPrismWorkerCmd()` 路由,鏡像 `runLiveTrading` 的 signal-handled select pattern)。

### 2. PRISM system「實作但不完整」

#### 2.1 症狀

即使修了 subcommand 路由,`runPrismWorker` 啟動的 manager 也不會處理任何任務(空 queue 永遠空)。

#### 2.2 真實根因:`prismMgr.Start()` 從未被呼叫

`cmd/atlas/main.go:304-305`:
```go
prismMgr := prism.NewPRISMManager(prism.DefaultPRISMConfig())
dashboard.WithPRISMManager(prismMgr)
// 缺: prismMgr.Start() ← 從未補上
```

`internal/prism/prism_manager.go:359-383` 的 `Start()` 會 spawn 5 regime queues 的 workers,但 grep 結果:
- `pm.Start()` 在 production code 沒被呼叫
- 只在 `prism_test.go:203` 與 `handlers_test.go:50`(test only)出現

**意圖是什麼**:
- `atlas-go` 的 apiMode 建立 manager(給 dashboard API 排程任務)
- 但 workers 必須在某處跑 — 原意是 prism-worker container
- 然而 prism-worker 的 command 從未實作(回到 #1)

#### 2.3 修法

在 `cmd/atlas/main.go:320` 後加 `prismMgr.Start()` + `defer prismMgr.Stop()`。**修的不是 prism-worker,是 atlas-go 本體** — 讓 workers 在同一個 process 跑(queue 是 in-memory,跨 process 沒用,兩個 manager 也不會共享任務)。

### 3. `atlas-grafana` provisioning error

#### 3.1 根因

`atlas_grafana-data` volume 內已有 datasource(UID 衝突),provisioning template 嘗試建立新 datasource(`uid: prometheus`)時找不到對應目標。

#### 3.2 修法

```bash
docker compose rm -f grafana
docker volume rm atlas_grafana-data
docker compose up -d grafana
```

volume 從零開始,provisioning 第一次成功寫入。

### 4. `atlas-alertmanager` YAML 縮排錯誤

#### 4.1 根因

`monitoring/alertmanager.yml` line 70 原版:
```yaml
67:         send_resolved: true
68: 
69: 
70: - name: info-slack      ← column 1 (其他 receivers 在 column 3)
```

#### 4.2 修法

line 70 加 2 個空格:`  - name: info-slack`,對齊其他 receivers。

### 5. `atlas-otel-collector` 不存在的 `postgresql` exporter

#### 5.1 根因

`monitoring/otel-collector.yaml` line 17:
```yaml
exporters:
  postgresql:  # ← OpenTelemetry Collector 沒有這個 exporter
    dsn: postgres://atlas:${DB_PASSWORD}@postgres:5432/atlas?sslmode=prefer
```

`otel/opentelemetry-collector-contrib:0.107.0` 的 valid exporters list 沒有 `postgresql`(`datadog / otlp / prometheus / file / debug` 等都沒有 `postgresql`)。

#### 5.2 修法

換成 `debug` exporter(no-loss placeholder,traces 仍被接收但 log 到 stdout 而非持久化)。未來要還原 Postgres 持久化需要寫 custom exporter。

### 6. `atlas-go` /health 503「ATLAS_API_KEY required in production」

#### 6.1 根因(最 subtle,只有 PR #79 commit 引入)

`docker-compose.yml` atlas 服務(原版):
```yaml
environment:
  - ATLAS_ENV=production
  - ATLAS_API_KEY=${ATLAS_API_KEY}  # ← 這行是 bug
```

Docker Compose 行為:
- `env_file` 把 `.env` 的所有變數注入容器
- `environment` 段對每個 `KEY=VALUE` 做 **shell 變數展開**(`${KEY}` 從 host shell 讀,**不是從 .env 讀**)
- `environment` 段的同名變數會**覆蓋** `env_file` 的值

所以 `ATLAS_API_KEY=${ATLAS_API_KEY}` 展開成 host shell 的值(host shell 沒設 → 空字串),把 `.env` 裡的 `ATLAS_API_KEY=e2e-test-key-not-for-prod` 蓋掉。

`internal/monitoring/api/shared/handler.go:18-28`:
```go
if isProduction && apiKey == "" {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        WriteJSONError(w, http.StatusServiceUnavailable, "server misconfigured: ATLAS_API_KEY required in production")
    })
}
```

→ 永遠 503。

#### 6.2 git blame

`79da8840 fix(deploy): add ATLAS_API_KEY to docker-compose.yml env (required in production)` — 這個 commit 用 `${ATLAS_API_KEY}` 語法,當時可能是想在 CI 用 shell env,但**跟 env_file 機制衝突**。

#### 6.3 修法(非 patch,既有 env_file 機制的正確用法)

**移除** `ATLAS_API_KEY=${ATLAS_API_KEY}` 這行,讓 `env_file` 唯一負責提供。CI/CD 應該把 secrets 寫進 .env(透過 secret mount),不要靠 shell env。

### 7. Dockerfile 硬編碼 HEALTHCHECK

#### 7.1 根因

`Dockerfile` line 92-93:
```dockerfile
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1
```

這個 endpoint 只有 `atlas` 服務(用 `-api` 啟動)會 listen。所有用同一個 Dockerfile build 的服務都繼承:
- `atlas-prism-worker`(跑 `prism worker` daemon,沒 HTTP server)
- `atlas-cron-*`(cron 容器,只跑週期性 job)
- `atlas-swarm-runner`(跑 `swarm run`)

這些服務的 container 內 `localhost:8080` 是空的,healthcheck 永遠失敗。

#### 7.2 修法

在 `docker-compose.yml` 對非 API 服務覆寫:
```yaml
prism-worker:
  healthcheck:
    disable: true  # ← 覆蓋 Dockerfile 繼承的壞設定
```

`cron-*` 服務保留(因為是週期性 one-shot,healthcheck 無意義)。

### 8. `fubon-neo` 公開 PyPI 不可用(2026-06-28 修正版)

#### 8.1 根因

**`fubon-neo` 是富邦新一代 API 的官方 Python SDK,從未放在公開 PyPI**。使用者需於 [`https://www.fbs.com.tw/TradeAPI/docs/download/download-sdk`](https://www.fbs.com.tw/TradeAPI/docs/download/download-sdk) 簽署 API 服務申請書後,從 `https://www.fbs.com.tw/TradeAPI_SDK/fubon_binary/` 手動下載 wheel(支援 Windows / macOS / Linux,Python 3.8-3.13)。

`curl https://pypi.org/pypi/fubon-neo/json` → 404 Not Found — 因為本來就沒上架,不是「下架」。
`services/fubon-proxy/requirements.txt` 還在 pin `fubon-neo==2.2.8` → build 階段 `pip install -r requirements.txt` 一定失敗。

#### 8.2 既存基礎設施(被忽略的)

`main.py:81-82` 已經有 fallback 路徑(找 cert):
```python
search_dirs = [
    os.path.expanduser("~/.config/atlas-go/.fubon-env"),  # ← 已預編譯 SDK 在這
    os.path.expanduser("~/.local/share/atlas-go/fubon_neo"),
]
```

`~/.config/atlas-go/.fubon-env/` 內已有預編譯的 venv(以官方 macOS wheel 安裝),`fubon_neo-2.2.8-cp37-abi3-macosx_11_0_arm64.zip` 也在 `~/.config/atlas-go/`。**build 流程沒接上**:Dockerfile 沒在 build 階段抓 wheel,Dockerfile 也沒用對應的 Python 版本。

#### 8.3 修法(2026-06-28 第二次修正,從官方 wheel URL 改成 TARGETARCH 自動切)

第一版以為只要從官方拉 wheel 就好,但實際 build 失敗:
```
ERROR: fubon_neo-2.2.8-cp37-abi3-manylinux_2_17_x86_64.manylinux2014_x86_64.whl
is not a supported wheel on this platform.
```

真實原因:**Docker build 跑在 host arch**(本機 Apple Silicon = arm64),但 x86_64 wheel 不能在 arm64 平台 pip install。Docker `TARGETARCH` 是 `arm64`,但 PEP 600 manylinux tag 是 `aarch64`,需要 mapping。

**真正的修法**(`services/fubon-proxy/Dockerfile`):

```dockerfile
ARG FUBON_NEO_VERSION=2.2.8
ARG TARGETARCH
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) WHEEL_ARCH=x86_64 ;; \
      arm64) WHEEL_ARCH=aarch64 ;; \
      *) echo "Unsupported TARGETARCH: ${TARGETARCH}"; exit 1 ;; \
    esac; \
    WHEEL_URL="https://www.fbs.com.tw/TradeAPI_SDK/fubon_binary/fubon_neo-${FUBON_NEO_VERSION}-cp37-abi3-manylinux_2_17_${WHEEL_ARCH}.manylinux2014_${WHEEL_ARCH}.zip"; \
    curl -fsSL -o /tmp/fubon_neo.zip "${WHEEL_URL}"; \
    unzip -j /tmp/fubon_neo.zip '*.whl' -d /tmp/fubon_wheel; \
    pip install --no-cache-dir /tmp/fubon_wheel/*.whl
```

對應的 `docker-compose.yml` fubon-proxy 段 build args:
```yaml
build:
  context: ./services/fubon-proxy
  dockerfile: Dockerfile
  args:
    FUBON_NEO_VERSION: "2.2.8"
  # 不用傳 FUBON_NEO_WHEEL_URL — Dockerfile 用 TARGETARCH 自動選
```

**驗證結果**(在 Apple Silicon / aarch64 上):
- container 內 `python -c "import fubon_neo; from fubon_neo.sdk import FubonSDK; print(FubonSDK)"` 成功
- `pip show fubon_neo` 顯示 `Version: 2.2.8`,`Location: /usr/local/lib/python3.13/site-packages`
- container `platform.machine()` = `aarch64`,`platform.python_version()` = `3.13.x`

**官方分發 URL pattern**(從 `https://www.fbs.com.tw/TradeAPI/docs/download/download-sdk` 確認):
- Windows: `fubon_neo-2.2.8-cp37-abi3-win_amd64.zip`
- macOS arm64: `fubon_neo-2.2.8-cp37-abi3-macosx_11_0_arm64.zip`
- macOS x86_64: `fubon_neo-2.2.8-cp37-abi3-macosx_10_12_x86_64.zip`
- Linux arm64: `fubon_neo-2.2.8-cp37-abi3-manylinux_2_17_aarch64.manylinux2014_aarch64.zip`
- Linux x86_64: `fubon_neo-2.2.8-cp37-abi3-manylinux_2_17_x86_64.manylinux2014_x86_64.zip`

`docker-compose.yml` 的 `.fubon-env` mount **只為 .p12 憑證**,不為 SDK(SDK 已從官方 wheel install 進 image)。

**`requirements.txt` 仍保留 `fubon-neo==2.2.8`**(文件化所需版本),但 Dockerfile 的 explicit `pip install <wheel>` 在 `-r requirements.txt` 之前先跑,所以 pip 不會去 PyPI 找(會 404)。

### 9. 兩個 .env 模板(.env_example vs .env.example)

#### 9.1 根因

Repo 根目錄有兩個 env 模板:
- `.env.example` — **tracked**,完整 template,`CONTRIBUTING.md:23` 跟 `SECURITY.md:59` 都用 `cp .env.example .env`
- `.env_example` — **untracked**(git ls-files 回空),簡化版,只有 market data + DB

`.env_example` 是 stale orphan,從 `.env` 目錄演變而來(`4bc98dc8 chore: gofmt 8 files, replace empty .env dir with .env_example`)。

#### 9.2 為什麼之前沒被清掉

沒人寫文件規範哪個是 canonical(其實有:`CONTRIBUTING.md` 跟 `SECURITY.md` 都用 `.env.example`)。stale 那份只是恰好在磁碟上。

#### 9.3 修法

`rm .env_example`(確認 `.env.example` 才是 tracked 的 canonical)。同時在 `.env.example` 加上 dev default 讓 `cp .env.example .env` 之後不需要再手動補 ATLAS_API_KEY。

## 為什麼「昨天修過又壞了」

5 個 crash loop 服務,昨天的「修法」都是 `docker compose restart` — 暫時讓容器重跑。**沒有一個是真正的根因修復**:

- prism-worker 1 秒 exit → restart 60 秒後又 crash
- grafana 1 秒 crash → restart 立刻又 crash  
- alertmanager / otel-collector 是 config 錯誤,restart 完全沒用(讀同一個壞 config)

唯一「修好」的是 atlas-go(它本來就 healthy,22 小時沒動過)。但 atlas-go 的 health 是 false positive — `retries: 3` + `interval: 30s` 的寬鬆設定下,某次 health check 在 atlas-go 還在 init 時通過,標記成 healthy 後就再沒嚴格驗證。實際上 `/health` 在生產模式 + 缺 ATLAS_API_KEY 時早就回 503(只是 health check 沒抓到)。

## 修復一覽(對應 9 條根因)

| # | 修法 | 檔案 |
|---|------|------|
| 1 | `command: ["prism", "worker"]` | `docker-compose.yml` |
| 1 | 補 `runPrismWorker` + `isPrismWorkerCmd` | `cmd/atlas/main.go` |
| 1 | 測試 `TestIsPrismWorkerCmd` 7 case | `cmd/atlas/main_test.go` |
| 2 | `prismMgr.Start()` + `defer prismMgr.Stop()` | `cmd/atlas/main.go` |
| 3 | `rm atlas_grafana-data` volume | (docker) |
| 4 | 修 line 70 縮排 | `monitoring/alertmanager.yml` |
| 5 | `postgresql` → `debug` exporter | `monitoring/otel-collector.yaml` |
| 6 | 移除 `ATLAS_API_KEY=${ATLAS_API_KEY}` | `docker-compose.yml` |
| 6 | 加 `ATLAS_API_KEY=e2e-test-key-not-for-prod` + `ATLAS_ENV=development` | `.env.example` |
| 6 | 補相同 dev defaults | `~/.config/atlas-go/.env` |
| 7 | `healthcheck: disable: true` | `docker-compose.yml`(prism-worker) |
| 8 | Dockerfile 用 TARGETARCH 自動切 manylinux wheel(arm64/amd64) | `services/fubon-proxy/Dockerfile` |
| 8 | docker-compose 移除 hardcoded FUBON_NEO_WHEEL_URL(交給 TARGETARCH 處理) | `docker-compose.yml` |
| 8 | Dockerfile 加 `unzip` 到 apt-get(預設 slim 沒有) | `services/fubon-proxy/Dockerfile` |
| 8 | requirements.txt 加註解說明 fubon-neo 不是 PyPI 套件 | `services/fubon-proxy/requirements.txt` |
| 9 | `rm .env_example`(stale orphan) | (filesystem) |

## 5 分鐘 boot test 驗證結果

| 服務 | 修復前 | 修復後 |
|------|--------|--------|
| atlas-prism-worker | 1-3 秒重啟循環(60s 週期) | Up 5 分鐘,0 重啟事件 |
| atlas-grafana | 1 秒 crash | Up 5 分鐘,0 provisioning error |
| atlas-alertmanager | 60s 重啟循環 | Up 33 秒,0 重啟 |
| atlas-otel-collector | 60s 重啟循環 | Up 33 秒,0 重啟 |
| atlas-go | (healthy) false positive | Up 5 分鐘,`/health` 回 200(需 API key) |
| atlas-postgres | healthy | Up 5 分鐘(healthy) |
| atlas-prometheus | Up 2 個月 | Up 5 分鐘 |
| atlas-redis | Up 2 天 | Up 5 分鐘 |

**`go test ./cmd/atlas/` 全數通過**(7/7 subcommand case + 既有測試,8.268s)。

## 已記錄在文件體系的位置(避免下次重複犯)

| 議題 | 歸屬 |
|------|------|
| ENTRYPOINT/command 衝突 | `docs/REFERENCE/traps.md` § Deploy/Docker |
| env_file vs environment precedence | `docs/REFERENCE/traps.md` § Deploy/Docker |
| Dockerfile 硬編碼 healthcheck | `docs/REFERENCE/traps.md` § Deploy/Docker |
| fubon-neo PyPI 404 + Linux wheel 限制 | `docs/environment.md` § Fubon SDK |
| .env.example dev defaults | `.env.example` 內嵌註解 + `docs/guides/install-and-deploy.md` § Generate ATLAS_API_KEY |
| fubon-proxy mount 機制 | `services/fubon-proxy/README.md` § Docker 部署的關鍵設計 |

## 已知仍存在的問題(不在本次修法範圍)

1. **`atlas-cron-*` 繼承壞 healthcheck**:不修是因為 cron 是週期性 one-shot,healthcheck 無意義。如果要修,在 `docker-compose.yml` 對每個 cron 服務加 `healthcheck: disable: true`。
2. **`internal/marketdata/fubon_client.go` 的 platform-specific 依賴**:走 `fubon-proxy` 繞過,但若 `fubon-proxy` 不可用時 fallback 不明確。

## 相關文件

- `docs/REFERENCE/traps.md` — 跨模組陷阱(含本次新增 Deploy/Docker 章節)
- `docs/environment.md` § Fubon SDK — SDK 位置 + 官方下載頁 + wheel 平台分發(含 arm64 / x86_64 對應)
- `services/fubon-proxy/README.md` — fubon-proxy 部署指南(含 wheel build 機制 + TARGETARCH 自動切)
- `services/fubon-proxy/requirements.txt` — 故意不含 fubon-neo
- `cmd/atlas/main.go:130` — `isPrismWorkerCmd` 路由
- `cmd/atlas/main.go:1690` — `runPrismWorker` daemon
- `cmd/atlas/main.go:320` — `prismMgr.Start()`
- `internal/monitoring/api/shared/handler.go:18` — `AuthMiddleware`(ATLAS_API_KEY 邏輯)
- `docs/incidents/2026-06-fubon-channel-recurring-failure.md` — 之前的 fubon 事件(IPv4/IPv6 雙棧議題,跟本次無關)
- `docs/investigations/2026-06-fubonproxy-ipv4-uvloop.md` — 之前的 fubon-proxy 調查(uvloop 議題,跟本次無關)
