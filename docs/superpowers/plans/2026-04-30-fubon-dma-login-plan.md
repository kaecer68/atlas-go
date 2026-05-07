# PLAN.md — 富邦 DMA Login 整合執行計劃

## 階段一：Python Wrapper（隔離 Go 複雜度）

### 1.1 建立 Python wrapper

**目標**：`cmd/fubon-dma/wrapper.py` — 長期行程，JSONL stdin/stdout

```bash
# 位置
cmd/fubon-dma/wrapper.py
```

**Protocol：** JSONL（每行一個 JSON）

**Commands：**

| cmd | 輸入 | 輸出 |
|-----|------|------|
| `login` | `{"cmd": "login", "personal_id": "...", "api_key": "..."}` | `{"status": "ok", "accounts": [...]}` 或 `{"status": "error", "message": "..."}` |
| `submit_order` | `{"cmd": "submit_order", "account": "...", "symbol": "...", "side": "...", "price": ..., "quantity": ...}` | `{"status": "ok", "order_id": "...", "fill_price": ...}` 或 `{"status": "error", "code": ...}` |
| `logout` | `{"cmd": "logout"}` | `{"status": "ok"}` |

### 1.2 實作 wrapper.py

```python
#!/usr/bin/env python3
from fubon_neo.sdk import FubonSDK
import json
import sys

def main():
    sdk = FubonSDK()
    session = None

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        req = json.loads(line)
        cmd = req.get("cmd")

        if cmd == "login":
            result = sdk.login(
                sdk.apikey_dma_login(req["personal_id"], req["api_key"])
            )
            print(json.dumps({
                "status": "ok" if result.is_success else "error",
                "is_success": result.is_success,
                "message": result.message,
                "data": str(result.data) if result.data else None
            }))
            sys.stdout.flush()
            if result.is_success:
                session = result.data

        elif cmd == "submit_order":
            if session is None:
                print(json.dumps({"status": "error", "code": "NOT_LOGGED_IN"}))
            else:
                # TODO: submit via session
                print(json.dumps({"status": "ok", "order_id": "mock"}))
            sys.stdout.flush()

        elif cmd == "logout":
            print(json.dumps({"status": "ok"}))
            sys.stdout.flush()
            break

if __name__ == "__main__":
    main()
```

### 1.3 驗證點

- [ ] `python3 cmd/fubon-dma/wrapper.py` 正常啟動（stdin/stdout 可讀）
- [ ] 輸入 `{"cmd": "login", "personal_id": "M120628569", "api_key": "..."}` 有回應

---

## 階段二：Go DMA Adapter（core）

### 2.1 建立 `internal/live/fubon_dma.go`

**職責：** 啟動 Python wrapper，管理 session，實現 `LiveExecutionAdapter`

```go
type FubonDMAAdapter struct {
    personalID  string
    apiKey      string
    scriptPath  string
    proc       *exec.Cmd
    stdin       io.WriteCloser
    stdout      *bufio.Reader
    mu          sync.RWMutex
    connected   bool
}
```

**Methods:**
- `NewFubonDMAAdapter(personalID, apiKey, scriptPath)` — 啟動 subprocess
- `SubmitOrder(ctx context.Context, order domain.Order) (BrokerResult, error)`
- `Close() error`
- `Ping(ctx context.Context) error` — 健康檢查

### 2.2 建立 `cmd/fubon-dma/main.go`

CLI 工具，可用於手動登入/登出/下單測試

```bash
go run ./cmd/fubon-dma login
go run ./cmd/fubon-dma submit -symbol 2330 -side buy -price 600 -qty 1000
go run ./cmd/fubon-dma logout
```

---

## 階段三：整合進 atlas 生命週期

### 3.1 Broker 工廠修改

修改 `internal/live/orchestrator.go` 或工廠方法：

```go
func NewBroker(mode string) Broker {
    switch mode {
    case "dry-run":
        return NewDryRunBroker()
    case "fubon-dma":
        return NewGuardedLiveBroker(NewFubonDMAAdapter(
            os.Getenv("FUBON_DMA_PERSONAL_ID"),
            os.Getenv("FUBON_DMA_API_KEY"),
            "cmd/fubon-dma/wrapper.py",
        ))
    default:
        return NewDryRunBroker()
    }
}
```

### 3.2 環境變數驗證

啟動時檢查必要變數是否具備：

```go
func init() {
    if os.Getenv("ATLAS_BROKER_MODE") == "fubon-dma" {
        if os.Getenv("FUBON_DMA_PERSONAL_ID") == "" || os.Getenv("FUBON_DMA_API_KEY") == "" {
            log.Fatal("FUBON_DMA_PERSONAL_ID and FUBON_DMA_API_KEY required for fubon-dma mode")
        }
    }
}
```

---

## 階段四：驗證

### 4.1 整合測試

```bash
# 啟動（無需聲明書）
FUBON_DMA_PERSONAL_ID=M120628569 \
FUBON_DMA_API_KEY=F6049D5DD934EFFEDE91EDE4E337C32E5CAC3A0FDEC0D75CFEC46B94845A6AAA \
ATLAS_BROKER_MODE=fubon-dma \
go run ./cmd/atlas -allow-live-broker

# 預期：啟動成功，login 回應 is_success: false（尚未簽署但不 panic）
```

### 4.2 Smoke test

```bash
go test ./internal/live/...
go build ./...
gofmt -l .
```

---

## 執行順序

```
階段一 → 1.1 wrapper.py（Python 工程師視角，無 Go 複雜度）
       → 1.3 驗證 JSONL protocol

階段二 → 2.1 FubonDMAAdapter（核心 adapter 實作）
       → 2.2 CLI tool

階段三 → 3.1 Broker 工廠
       → 3.2 環境變數驗證

階段四 → 4.1/4.2 驗證
```

每個階段完成後應有可獨立驗證的產出。