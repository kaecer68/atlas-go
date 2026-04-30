# SPEC.md — 富邦 DMA Login 整合

## 1. 目標

將富邦新一代 API（`fubon_neo` Python SDK）的 `apikey_dma_login()` 整合進 atlas-go，成為可切換的實盤下單介面。整合完成後，signaled account 可直接透過 DMA 線路執行訂單。

## 2. 現況

| 元件 | 狀態 | 說明 |
|------|------|------|
| 富邦報價 API | ✅ 已整合 | `FubonProvider` / `FubonClient` in `internal/marketdata/` |
| Broker 抽象 | ✅ 存在 | `DryRunBroker` / `GuardedLiveBroker` + `LiveExecutionAdapter` |
| `apikey_dma_login()` | 🔒 待開通 | 需要「API使用風險暨聲明書」簽署完成才可成功 |
| Python SDK | ✅ 已就緒 | `fubon_neo-2.2.8-cp37-abi3-macosx_11_0_arm64.zip` 在 worktree |

## 3. 整合架構

```
cmd/atlas (with -allow-live-broker)
  └── internal/live/orchestrator.go  →  GuardedLiveBroker
                                        └── FubonDMAAdapter  (NEW)
                                              └── Python subprocess (fubon_neo SDK)
```

### 新增元件

| 檔案 | 職責 |
|------|------|
| `internal/live/fubon_dma.go` | `FubonDMAAdapter` — 實現 `LiveExecutionAdapter` |
| `internal/live/fubon_session.go` | Session 管理（login logout lifecycle） |
| `cmd/fubon-dma/` | DMA 指令集（login, logout, submit, status） |

### 環境變數

```bash
FUBON_DMA_PERSONAL_ID=M120628569
FUBON_DMA_API_KEY=F6049D5DD934EFFEDE91EDE4E337C32E5CAC3A0FDEC0D75CFEC46B94845A6AAA
ATLAS_BROKER_MODE=fubon-dma   # 替換 dry-run
```

## 4. Python subprocess 設計

Go 無法直接呼叫 Python SDK，透過以下方式整合：

```
┌─────────────┐     stdin JSON      ┌────────────────────┐
│ Go process  │ ────────────────→  │ Python wrapper      │
│ (fubon_dma) │                     │ (fubon_neo SDK)     │
│             │  ← stdout JSON     │                     │
└─────────────┘                    └────────────────────┘
```

**Protocol:**
- 啟動時建立一個長期 Python 行程（subprocess）
- 溝通格式：JSONL（每行一個 JSON 物件）
- Commands: `{ "cmd": "login" }`, `{ "cmd": "submit_order", ... }`, `{ "cmd": "logout" }`

## 5. FubonDMAAdapter 介面

```go
// internal/live/fubon_dma.go
type FubonDMAAdapter struct {
    personalID string
    apiKey     string
    session    *DMAClientSession  // Python subprocess handle
    mu         sync.RWMutex
}

func (a *FubonDMAAdapter) SubmitOrder(ctx context.Context, order domain.Order) (BrokerResult, error)
```

**SubmitOrder 轉譯邏輯：**
1. `domain.Order` → Python order dict
2. 送至 subprocess stdin
3. 解析 stdout JSON 回應
4. 回傳 `BrokerResult`

## 6. Session 管理

```go
type DMAClientSession struct {
    process   *exec.Cmd
    connected bool
    accountNo string
}

func (s *DMAClientSession) Login(ctx context.Context, personalID, apiKey string) error
func (s *DMAClientSession) SubmitOrder(ctx context.Context, order map[string]any) (map[string]any, error)
func (s *DMAClientSession) Logout(ctx context.Context) error
```

**Login 驗證：**
- API 回應 `is_success: false`（尚未簽署）仍視為連線成功（網路可達）
- 真正下單時若仍是 `is_success: false`，則回傳明確錯誤 `ErrAccountNotEnabled`

## 7. 錯誤處理

| 錯誤情境 | 回應 |
|----------|------|
| Python subprocess 未啟動 | `ErrDMANotConnected` |
| 尚未簽署 API 聲明書 | `ErrAccountNotEnabled`（含聲明書 URL） |
| 下單失敗（API 錯誤） | 解析回傳 error code，回傳結構化 `BrokerResult` |
| 超時（10s） | `ErrDMARequestTimeout` |

## 8. Feature Flag / Guard

`ATLAS_BROKER_MODE=fubon-dma` 時才啟動 DMA adapter。
其他模式（dry-run / mock）維持現有行為。

## 9. 驗收標準

- [ ] `go build ./cmd/fubon-dma/...` 成功
- [ ] `FUBON_DMA_API_KEY=... ATLAS_BROKER_MODE=fubon-dma go run ./cmd/atlas -allow-live-broker` 啟動不 panic
- [ ] `apikey_dma_login()` 連線成功（即便 `is_success: false`）
- [ ] 簽署完成後，`SubmitOrder` 可成功送出並回傳 `BrokerResult`
- [ ] `go test ./internal/live/...` 全數通過

## 10. 參考文獻

- [富邦API文件](https://www.fubon.com.tw/) — 新一代 API 規格
- `internal/live/broker.go` — 既有的 adapter 模式
- `internal/marketdata/fubon_client.go` — 富邦 API call 慣例