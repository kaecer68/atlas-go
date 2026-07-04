# 富邦證券通道反覆故障 — 根因事件紀錄

**日期**: 2026-06-16
**嚴重性**: P1(每週一次手動修復,阻斷 atlas 即時行情通道)
**狀態**: 已止血(A),根治排程中(B)

---

## 症狀

```
⚠ fubon fetch: fubon proxy: http request: Get
"http://localhost:8081/quotes?symbols=2330%2C0050":
dial tcp [::1]:8081: connect: connection refused
```

每 ~1 週發生一次,使用者反覆需要手動介入。

## 根因(單一)

`~/.config/atlas-go/.env` 第 41 行設定:

```bash
FUBON_PROXY_URL=http://localhost:8081
```

此環境變數透過 `internal/marketdata/fubon_client.go:98` 覆寫了程式碼內的
IPv4 硬編碼預設值:

```go
proxyURL := os.Getenv("FUBON_PROXY_URL")   // ← 讀到 localhost
if proxyURL == "" {
    proxyURL = fubonProxyBaseURL            // = "http://127.0.0.1:8081"(永遠走不到)
}
```

Go 的 `http.NewRequest` 對 `localhost` 觸發 DNS 解析,在 macOS / Linux
雙棧環境下 resolver 偏好回傳 IPv6 `[::1]`。Python fubon-proxy 雖已綁
`host="::"`,但若程序未運行(或 Python proxy 5 分鐘內被 ProcessManager
自動重啟失敗),就會回 `ECONNREFUSED` 而非 fallback。

## 為何 Fugle/TWSE 穩定而 Fubon 不穩定

| 維度 | Fugle | TWSE | Fubon |
|------|-------|------|-------|
| `os.Getenv` 用於 URL | 0 | 0 | **1** |
| URL 來源 | 硬編碼常數 | 硬編碼常數 | 環境變數覆寫優先 |
| 連線目標 | 外部 HTTPS | 外部 HTTPS | **本地** Python proxy |

Fubon 是三個 provider 中**唯一**允許外部輸入影響連線位址的,此反模式
造成 DNS 解析變因進入故障面。

## 已執行的修復(A 方案)

1. `~/.config/atlas-go/.env:41` → `FUBON_PROXY_URL=http://127.0.0.1:8081`
2. `.env_example:43` → `FUBON_PROXY_URL=http://127.0.0.1:8081`
   (同步修模板,避免新使用者照抄)

## B 方案(已實作於此 PR)

B 方案核心已直接實作在此 PR：移除 `fubon_client.go`
中的環境變數覆寫邏輯,讓 IPv4 硬編碼成為唯一真理來源,徹底消除 DNS
變因再次侵入的可能性。另修復 BackgroundTaskManager breaker deadlock、
以及 `auto_geopolitical` 繞過熔斷器的同型問題。

## git 歷史佐證(症狀修補模式)

自 2026 年起,以下 commit 都在不同層加保護,但都沒回到 `.env`:

- `fix(marketdata): hardcode 127.0.0.1 for fubonproxy connections (#495)`
- `feat(marketdata): extract generic providerBreaker, add Fubon circuit breaker (#492)`
- `feat(marketdata): add circuit breaker for Fubon provider`
- `fix(fubonproxy): decouple Start() from health check and fix Stop() double-Wait bug (#489)`
- `fix(boot): server-startup panic recovery + fubon-proxy fail-safe (#521)`
- `feat(fubonproxy): add port 8081 pre-flight probe to detect foreign holders (F9)`
- `fix: hybrid provider gracefully handles unreachable Fubon proxy`
- `fix(marketdata): add Fubon proxy probe in HybridProvider (#342)`
- `fix(apigateway): improve channel resilience — circuit breaker, non-trading guard, fubon probe (#339)`

每個 commit 都修在不同地方 — Go client、Python proxy、process
manager、circuit breaker、apigateway 通道 — 但**沒有人檢查 `.env`**。
這是「貼膏藥式修復」的典型指紋。

## 教訓

1. **不要讓環境變數覆寫安全預設值** — 如果某個值是「硬編碼防護」,
   它就應該是唯讀的,不應被外部輸入繞過。
2. **憑證齊全 ≠ 設定正確** — 故障時不要往「缺東西」方向假設,先
   檢查「設定指向哪」。
3. **症狀修補的警訊** — 同一通道出現 5+ 個獨立修復 commit,代表
   根本問題未被解決,需要回溯到第一個 commit 之前的設計意圖。

## Resolution (PR #943 — 2026-07-04)

三層韌性修復，從根源解決 fubon 通道脆弱性：

- **Layer 1**：`/health` 改為 fast process-only check；SDK 初始化延遲到第一個請求；in-memory quote cache（30s TTL, stale-while-revalidate）
- **Layer 2**：`FubonClient.healthClient`（2s timeout）隔離健康檢查；背景探測 goroutine（15s）；`IsHealthy()` + pre-flight fast-fail
- **Layer 3**：TCP pre-flight check — `socket.connect(neoapi.fbs.com.tw:443, timeout=5s)` 在 SDK init 前檢測上游是否可達，不可達時直接回 503 而不呼叫 C extension（C extension 在 macOS 上無法被 Python timeout 中斷）

此前 19 個 fubon 相關 PR 全屬 process management 修補，本 PR 是第一個處理**資料通道韌性**的修復。
