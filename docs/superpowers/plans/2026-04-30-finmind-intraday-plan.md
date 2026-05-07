# FinMind 日內行情實作計畫

**日期**: 2026-04-30
**依據**: SPEC.md 調研結果
**Scope**: 方案 A — 大盤 5 秒指數資料整合（低成本可行方案）

---

## 目標

將 FinMind `TaiwanVariousIndicators5Seconds` 大盤 5 秒指數資料整合進 atlas-go，支援：
1. 日內大盤動能特徵計算
2. 敘事事件的當日觸發增強

---

## 檔案結構

```
internal/marketdata/
  finmind_intraday.go      # 新增：FinMind 5 秒指數抓取

data/replay/
  taiwan_5sec_index.jsonl  # 新增：5 秒大盤指數儲存

internal/portfolio/
  factor_engine.go         # 修改：整合大盤時序特徵

internal/narrative/
  ingestor.go              # 修改：增強日內事件偵測
```

---

## Task 1: FinMind 5 秒指數 Client

**檔案**:
- 建立: `internal/marketdata/finmind_intraday.go`
- 建立: `internal/marketdata/finmind_intraday_test.go`

### Step 1: 寫測試

```go
package marketdata

import (
    "testing"
    "time"
)

func TestFetchTaiwan5SecIndex_Success(t *testing.T) {
    // 測試能夠成功取得 5 秒指數
    // 注意：需要有效的 FINMIND_API_KEY
}

func TestParse5SecIndexResponse(t *testing.T) {
    raw := `{"msg":"success","status":200,"data":[
        {"date":"2026-04-29 09:00:00","TAIEX":39521.73},
        {"date":"2026-04-29 09:00:05","TAIEX":39521.50}
    ]}`

    resp, err := parseTaiwan5SecIndexResponse([]byte(raw))
    if err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    if len(resp.Data) != 2 {
        t.Fatalf("expected 2 records, got %d", len(resp.Data))
    }

    if resp.Data[0].TAIEX != 39521.73 {
        t.Fatalf("expected TAIEX 39521.73, got %f", resp.Data[0].TAIEX)
    }
}
```

### Step 2: 跑測試（預期 FAIL - 尚未實作）

### Step 3: 實作

```go
package marketdata

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

const (
    finmindBaseURL = "https://api.finmindtrade.com/api/v4"
)

// Taiwan5SecIndexBar 代表一筆 5 秒大盤指數
type Taiwan5SecIndexBar struct {
    Date  time.Time `json:"date"`
    TAIEX float64   `json:"taiex"`
}

// Taiwan5SecIndexResponse 是 FinMind API 回應格式
type Taiwan5SecIndexResponse struct {
    Msg    string `json:"msg"`
    Status int    `json:"status"`
    Data   []struct {
        Date  string  `json:"date"`
        TAIEX float64 `json:"taiex"`
    } `json:"data"`
}

// FetchTaiwan5SecIndex 抓取指定日期的 5 秒大盤指數
func (c *FinMindClient) FetchTaiwan5SecIndex(ctx context.Context, date string) ([]Taiwan5SecIndexBar, error) {
    url := fmt.Sprintf("%s/data?dataset=TaiwanVariousIndicators5Seconds&start_date=%s",
        finmindBaseURL, date)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }

    if c.apiKey != "" {
        req.Header.Set("Authorization", "Bearer "+c.apiKey)
    }

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("do request: %w", err)
    }
    defer resp.Body.Close()

    var result Taiwan5SecIndexResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("decode response: %w", err)
    }

    if result.Status != 200 {
        return nil, fmt.Errorf("API error: %s", result.Msg)
    }

    bars := make([]Taiwan5SecIndexBar, 0, len(result.Data))
    for _, item := range result.Data {
        t, err := time.Parse("2006-01-02 15:04:05", item.Date)
        if err != nil {
            continue
        }
        bars = append(bars, Taiwan5SecIndexBar{
            Date:  t,
            TAIEX: item.TAIEX,
        })
    }

    return bars, nil
}

func parseTaiwan5SecIndexResponse(data []byte) (*Taiwan5SecIndexResponse, error) {
    var resp Taiwan5SecIndexResponse
    if err := json.Unmarshal(data, &resp); err != nil {
        return nil, err
    }
    return &resp, nil
}
```

### Step 4: 驗證

```bash
go test ./internal/marketdata/... -run Taiwan5Sec -v
```

### Step 5: Commit

```bash
git add internal/marketdata/finmind_intraday.go internal/marketdata/finmind_intraday_test.go
git commit -m "feat(marketdata): add FinMind Taiwan 5-second index client"
```

---

## Task 2: 5 秒指數寫入 Ledger

**檔案**:
- 修改: `internal/marketdata/finmind_intraday.go`（加入 SaveToLedger 方法）
- 修改: `internal/ledger/ledger.go`（如需要新型別）

### Step 1: 寫入方法

```go
// Save5SecIndexToLedger 將 5 秒指數寫入 JSONL
func Save5SecIndexToLedger(bars []Taiwan5SecIndexBar, date string) error {
    dir := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "replay")
    path := filepath.Join(dir, "taiwan_5sec_index.jsonl")

    f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return fmt.Errorf("open file: %w", err)
    }
    defer f.Close()

    enc := json.NewEncoder(f)
    for _, bar := range bars {
        rec := map[string]interface{}{
            "date":  bar.Date.Format(time.RFC3339),
            "taiex": bar.TAIEX,
            "type":  "5sec_index",
        }
        if err := enc.Encode(rec); err != nil {
            return fmt.Errorf("encode record: %w", err)
        }
    }

    return nil
}
```

### Step 2: 驗證

```bash
go build ./internal/marketdata/...
```

### Step 3: Commit

```bash
git add internal/marketdata/finfinmind_intraday.go
git commit -m "feat(marketdata): save 5-sec index to ledger"
```

---

## Task 3: 日內大盤動能特徵（可選擴充）

**檔案**:
- 修改: `internal/portfolio/factor_engine.go`

### Step 1: 新增特徵

```go
// IntradayMomentumFeatures 計算日內大盤動能特徵
type IntradayMomentumFeatures struct {
    OpeningGap     float64 // 開盤缺口（相對於前日收盤）
    MorningMomentum float64 // 上午動能（9:00-12:00）
    AfternoonDrift  float64 // 下午 drift（12:00-13:30）
    CloseDirection float64 // 收盤方向
}

// ComputeIntradayMomentumFrom5Sec 使用 5 秒資料計算日內動能
func ComputeIntradayMomentumFrom5Sec(bars []Taiwan5SecIndexBar) *IntradayMomentumFeatures {
    if len(bars) < 100 {
        return nil
    }

    // 取上午（9:00-12:00）與下午（12:00-13:30）
    // 計算動能並回傳
    return &IntradayMomentumFeatures{...}
}
```

---

## Task 4: 敘事事件日內觸發增強（可選擴充）

**檔案**:
- 修改: `internal/narrative/ingestor.go`

在既有敘事事件偵測中加入 5 秒大盤資料輔助判斷。

---

## 驗證檢查清單

- [ ] `go build ./internal/marketdata/...` — PASS
- [ ] `go test ./internal/marketdata/...` — PASS
- [ ] 手動測試：成功寫入 `taiwan_5sec_index.jsonl`
- [ ] `go vet ./internal/marketdata/...` — PASS

---

## 時間預估

| Task | 預估工時 |
|------|----------|
| Task 1: 5 秒指數 Client | 1 小時 |
| Task 2: Ledger 寫入 | 30 分鐘 |
| Task 3: 動能特徵（可選）| 2 小時 |
| Task 4: 敘事增強（可選）| 2 小時 |

**最低可行版本（Task 1+2）**: ~1.5 小時

---

## 替代方案（Premium 訂閱）

若未來取得 FinMind Premium：

1. 評估 `TaiwanStockPriceTick` 與 `TaiwanStockKBar` 欄位
2. 設計個股日內資料模型
3. 更新 `marketdata.FinMindClient` 支援個股查詢

此計畫留待 Premium 取得後擴充。
