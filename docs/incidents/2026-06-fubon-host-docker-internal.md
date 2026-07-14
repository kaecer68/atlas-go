# fubon-proxy 容器網路隔離 — host.docker.internal 修復

**日期**: 2026-06-21
**嚴重性**: P1（atlas 容器無法連線至 macOS host 上的 fubon-proxy）
**狀態**: 已修復

---

## 症狀

atlas 容器啟動後,所有 fubon 通道的請求都回傳 `connection refused`:

```
fubon fetch: fubon proxy: http request: Get
"http://127.0.0.1:8081/quotes?symbols=2330%2C0050":
dial tcp 127.0.0.1:8081: connect: connection refused
```

Fugle、TWSE、FinMind 等其他通道正常；僅 fubon 通道故障。

## 根因

3 個 Go 檔案硬編碼 `127.0.0.1:8081` 作為 fubon-proxy 的連線位址：

| 檔案 | 用途 | 舊值 |
|------|------|------|
| `internal/marketdata/fubon_client.go` | FubonClient proxy URL 常數 | `http://127.0.0.1:8081` |
| `internal/marketdata/hybrid_provider.go` | HybridProvider 連線探測 | `127.0.0.1:8081` |
| `internal/apigateway/register_adapters.go` | Adapter 註冊前的連線探測 | `127.0.0.1:8081` |

**問題**: atlas 執行在 Docker 容器內。`127.0.0.1:8081` 指向容器的 loopback 介面，
而非 macOS host。fubon-proxy 是原生 Python 服務,執行在 macOS host 上 (不在容器內)。
容器內部無法透過 `127.0.0.1` 接觸到 host 的服務。

此問題自專案開始使用 Docker Compose 部署以來即存在,但此前可能因:
- 開發階段以原生方式執行 atlas (非 Docker),`127.0.0.1` 可正確連線
- 未在 Docker 部署環境中測試 fubon 通道

而未暴露。

## 修復

### A. Go 程式碼 (3 檔案 → `host.docker.internal:8081`)

`127.0.0.1:8081` → `host.docker.internal:8081`:

1. **`internal/marketdata/fubon_client.go`**: `const fubonProxyBaseURL`
2. **`internal/marketdata/hybrid_provider.go`**: `proxyAddr` (連線探測)
3. **`internal/apigateway/register_adapters.go`**: `net.DialTimeout` 目標位址

`host.docker.internal` 是 Docker Desktop (macOS/Windows) 的特殊 DNS 名稱,
由 Docker 內部 DNS resolver 自動解析為 host 的 gateway IP。
支援版本: Docker Desktop 4.13+。

### B. docker-compose.yml

在 atlas 服務加入 `extra_hosts`:

```yaml
extra_hosts:
  - "host.docker.internal:host-gateway"
```

`host-gateway` 是 Docker Compose 的特殊值,自動解析為 host 的 gateway IP。
此設定確保所有 Docker Desktop 版本 (含舊版) 都能正確解析。

### C. 不需修改的檔案

- **`internal/fubonproxy/manager.go`**: ProcessManager 在 macOS host 上原生執行,
  管理 Python fubon-proxy 的生命週期。使用 `127.0.0.1:8081` 正確 (host 自身的 loopback)。
- **`services/fubon-proxy/main.py`**: Python proxy 綁定 `0.0.0.0:8081`,已可接受來自
  任何介面的連線,無需修改。

## PR #572 防護測試 (未回歸)

`fubon_url_guard_test.go` 的 `TestFubon_URLGuard` 仍通過。
該測試防止以下兩個反模式復活:
1. `localhost:8081` 出現在 production .go 程式碼中
2. `os.Getenv("FUBON_PROXY_URL")` 環境變數覆寫

`host.docker.internal:8081` 不在 banned patterns 清單中,測試正常通過。

## 新增測試 (TDD)

| 測試 | 檔案 | 驗證 |
|------|------|------|
| `TestFubonClient_DefaultURL_UsesHostDockerInternal` | `fubon_client_test.go` | `NewFubonClient().proxyURL == "http://host.docker.internal:8081"` |
| `TestFubonClient_RejectsFUBONProxyURL` | `fubon_client_test.go` | 設定 `FUBON_PROXY_URL` 環境變數後,client URL 不受影響 (PR #572 回歸) |
| `TestDockerCompose_HasHostDockerInternalAlias` | `docker_compose_test.go` | docker-compose.yml atlas 服務包含 `host.docker.internal:host-gateway` |

## macOS Docker Desktop 需求

- Docker Desktop 4.13+ (2023 Q1+)
- `host.docker.internal` 由 Docker 內部 DNS 自動解析
- 無需手動設定 `/etc/hosts`

## 教訓

1. **容器內外網路隔離**: `127.0.0.1` 在容器內指向容器自身,不是 host。
   容器需要透過 `host.docker.internal` 或 Docker network 與 host 服務通訊。
2. **分離 host-side 與 container-side 程式碼**: `fubonproxy/manager.go` 是 host-side
   程式碼 (管理 Python proxy 生命週期),不需修改；`fubon_client.go` / 
   `hybrid_provider.go` / `register_adapters.go` 是 container-side 程式碼,
   需要 `host.docker.internal`。
3. **Docker Compose `extra_hosts`**: 確保跨 Docker Desktop 版本的相容性,
   `host-gateway` 特殊值自動解析 host IP。
