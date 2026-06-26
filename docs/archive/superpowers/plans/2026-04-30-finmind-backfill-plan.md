# FinMind Backfill System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立三個獨立的 backfill CLI 命令（month-revenue、institutional-investors、financial-statements），透過 FinMind API 補足歷史數據並建立監控框架。

**Architecture:** 三個獨立 Go CLI 命令，共用統一的 health record 寫入機制與 rate limit 處理。所有輸出寫入 `data/replay/*.jsonl`，以 `date|symbol` 去重。

**Tech Stack:** Go stdlib (`net/http`, `encoding/json`, `flag`, `time`) + FinMind API v4

---

## File Structure

```
cmd/
  backfill-month-revenue/main.go          # Create
  backfill-institutional-investors/main.go # Create
  backfill-financial-statements/main.go    # Create

internal/
  monitoring/channel_health.go            # Create (shared health record writer)
  marketdata/finmind_backfill.go           # Create (shared FinMind client + rate limiter)
```

---

## Task 1: Shared FinMind Backfill Client (`internal/marketdata/finmind_backfill.go`)

**Files:**
- Create: `internal/marketdata/finmind_backfill.go`
- Test: `internal/marketdata/finmind_backfill_test.go`

- [ ] **Step 1: Write the failing test**

```go
package marketdata

import (
    "testing"
    "time"
)

func TestRateLimiter_WaitAndRecord(t *testing.T) {
    limiter := newRateLimiter(600) // 600 req/hr
    ctx := context.Background()

    // Should not block on first call
    err := limiter.Wait(ctx)
    if err != nil {
        t.Fatalf("first wait failed: %v", err)
    }

    remaining := limiter.Remaining()
    if remaining >= 600 {
        t.Fatalf("remaining should decrease after wait, got %d", remaining)
    }
}

func TestRateLimiter_DecrementsOnUse(t *testing.T) {
    limiter := newRateLimiter(600)
    limiter.RecordUse()
    if limiter.Remaining() != 599 {
        t.Fatalf("expected 599 remaining, got %d", limiter.Remaining())
    }
}

func TestRateLimiter_429Handling(t *testing.T) {
    limiter := newRateLimiter(600)
    // Simulate hitting 429 - should compute correct wait time
    resetAt := time.Now().Add(30 * time.Second)
    waitDuration := limiter.WaitForReset(resetAt)
    if waitDuration < 29*time.Second || waitDuration > 31*time.Second {
        t.Fatalf("expected ~30s wait, got %v", waitDuration)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/marketdata/... -run TestRateLimiter -v`
Expected: FAIL - undefined functions/types

- [ ] **Step 3: Write minimal implementation**

```go
package marketdata

import (
    "context"
    "net/http"
    "sync"
    "time"

    "github.com/kaecer68/atlas-go/internal/monitoring"
)

type rateLimiter struct {
    mu         sync.Mutex
    remaining  int
    resetAt    time.Time
    limit      int
    requests   []time.Time
    window     time.Duration
}

func newRateLimiter(limit int) *rateLimiter {
    return &rateLimiter{
        limit:  limit,
        remaining: limit,
        window: time.Hour,
        requests: make([]time.Time, 0),
    }
}

func (r *rateLimiter) Wait(ctx context.Context) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    now := time.Now()
    cutoff := now.Add(-r.window)
    var validRequests []time.Time
    for _, t := range r.requests {
        if t.After(cutoff) {
            validRequests = append(validRequests, t)
        }
    }
    r.requests = validRequests

    if len(r.requests) >= r.limit {
        sleepDuration := r.window - now.Sub(r.requests[0])
        r.mu.Unlock()
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(sleepDuration):
            r.mu.Lock()
            return nil
        }
    }
    return nil
}

func (r *rateLimiter) RecordUse() {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.requests = append(r.requests, time.Now())
    if r.remaining > 0 {
        r.remaining--
    }
}

func (r *rateLimiter) Remaining() int {
    r.mu.Lock()
    defer r.mu.Unlock()
    return r.remaining
}

func (r *rateLimiter) WaitForReset(resetAt time.Time) time.Duration {
    return time.Until(resetAt) + time.Second
}

// FinMindAPIError classifies API errors
type FinMindAPIError struct {
    StatusCode int
    Message    string
    RetryAfter time.Duration
}

func (e *FinMindAPIError) IsRateLimit() bool {
    return e.StatusCode == 429
}

func (e *FinMindAPIError) IsServerError() bool {
    return e.StatusCode >= 500
}

// FetchWithRetry performs HTTP GET with rate limiting and retry
func FetchWithRetry(ctx context.Context, client *http.Client, url string, apiKey string, limiter *rateLimiter, maxRetries int) ([]byte, error) {
    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
    if apiKey != "" {
        req.Header.Set("Authorization", "Bearer "+apiKey)
    }

    for attempt := 0; attempt <= maxRetries; attempt++ {
        if err := limiter.Wait(ctx); err != nil {
            return nil, err
        }

        resp, err := client.Do(req)
        if err != nil {
            if attempt < maxRetries {
                time.Sleep(time.Duration(attempt+1) * time.Second)
                continue
            }
            return nil, err
        }

        body, _ := io.ReadAll(resp.Body)
        resp.Body.Close()

        if resp.StatusCode == 429 {
            retryAfter := resp.Header.Get("Retry-After")
            var waitTime time.Duration
            if retryAfter != "" {
                waitTime, _ = time.ParseDuration(retryAfter + "s")
            }
            if waitTime == 0 {
                waitTime = time.Minute
            }
            time.Sleep(waitTime)
            continue
        }

        if resp.StatusCode >= 500 && attempt < maxRetries {
            time.Sleep(time.Duration(attempt+1) * 5 * time.Second)
            continue
        }

        limiter.RecordUse()
        return body, nil
    }
    return nil, &FinMindAPIError{StatusCode: 0, Message: "max retries exceeded"}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/marketdata/... -run TestRateLimiter -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/marketdata/finmind_backfill.go internal/marketdata/finmind_backfill_test.go
git commit -m "feat(marketdata): add shared FinMind backfill client with rate limiter"
```

---

## Task 2: Health Record Writer (`internal/monitoring/channel_health.go`)

**Files:**
- Create: `internal/monitoring/channel_health.go`
- Modify: `internal/monitoring/service/macro.go:1-10` (add import if needed)

- [ ] **Step 1: Write the failing test**

```go
package monitoring

import (
    "encoding/json"
    "os"
    "testing"
    "time"
)

func TestRecordChannelFetch(t *testing.T) {
    tmpDir := t.TempDir()
    healthPath := tmpDir + "/channel_health.json"

    RecordChannelFetch(healthPath, "test_channel", "ok", "", 100, 50)

    data, err := os.ReadFile(healthPath)
    if err != nil {
        t.Fatalf("failed to read health file: %v", err)
    }

    var result map[string]ChannelHealthRecord
    if err := json.Unmarshal(data, &result); err != nil {
        t.Fatalf("failed to unmarshal: %v", err)
    }

    record, ok := result["test_channel"]
    if !ok {
        t.Fatal("test_channel not found in health record")
    }
    if record.Status != "ok" {
        t.Fatalf("expected status 'ok', got %s", record.Status)
    }
}

func TestRecordChannelFetch_MultipleChannels(t *testing.T) {
    tmpDir := t.TempDir()
    healthPath := tmpDir + "/channel_health.json"

    RecordChannelFetch(healthPath, "channel_a", "ok", "", 100, 50)
    RecordChannelFetch(healthPath, "channel_b", "error", "rate limit exceeded", 0, 0)

    data, _ := os.ReadFile(healthPath)
    var result map[string]ChannelHealthRecord
    json.Unmarshal(data, &result)

    if len(result) != 2 {
        t.Fatalf("expected 2 channels, got %d", len(result))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/monitoring/... -run TestRecordChannelFetch -v`
Expected: FAIL - undefined function

- [ ] **Step 3: Write minimal implementation**

```go
package monitoring

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"
)

type ChannelHealthRecord struct {
    Status             string    `json:"status"`
    LastRun            time.Time `json:"last_run"`
    RecordsFetched     int       `json:"records_fetched"`
    SymbolsProcessed   int       `json:"symbols_processed"`
    Errors             []string  `json:"errors"`
    RateLimitRemaining int       `json:"rate_limit_remaining"`
    LatencyMs          int64     `json:"latency_ms"`
}

var healthMu sync.RWMutex

func RecordChannelFetch(stateDir, channel, status, errMsg string, rateRemaining int, latencyMs int64) {
    filepath := filepath.Join(stateDir, "channel_health.json")

    healthMu.Lock()
    defer healthMu.Unlock()

    var records map[string]ChannelHealthRecord
    if data, err := os.ReadFile(filepath); err == nil {
        json.Unmarshal(data, &records)
    }
    if records == nil {
        records = make(map[string]ChannelHealthRecord)
    }

    record := ChannelHealthRecord{
        Status:             status,
        LastRun:            time.Now(),
        RecordsFetched:     0,
        SymbolsProcessed:   0,
        RateLimitRemaining: rateRemaining,
        LatencyMs:          latencyMs,
    }

    if errMsg != "" {
        record.Errors = []string{errMsg}
    }

    records[channel] = record

    data, _ := json.MarshalIndent(records, "", "  ")
    os.WriteFile(filepath, data, 0644)
}

func RecordChannelFetchWithPool(stateDir, channel, status, errMsg string, pool interface{}) {
    filepath := filepath.Join(stateDir, "channel_health.json")

    healthMu.Lock()
    defer healthMu.Unlock()

    var records map[string]ChannelHealthRecord
    if data, err := os.ReadFile(filepath); err == nil {
        json.Unmarshal(data, &records)
    }
    if records == nil {
        records = make(map[string]ChannelHealthRecord)
    }

    record := ChannelHealthRecord{
        Status: status,
        LastRun: time.Now(),
    }
    if errMsg != "" {
        record.Errors = []string{errMsg}
    }

    records[channel] = record

    data, _ := json.MarshalIndent(records, "", "  ")
    os.WriteFile(filepath, data, 0644)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/monitoring/... -run TestRecordChannelFetch -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/monitoring/channel_health.go internal/monitoring/channel_health_test.go
git commit -m "feat(monitoring): add ChannelHealthRecord writer for backfill monitoring"
```

---

## Task 3: `backfill-month-revenue` Command

**Files:**
- Create: `cmd/backfill-month-revenue/main.go`
- Modify: `internal/marketdata/finmind_client.go:246-256` (add helper method)

- [ ] **Step 1: Write the failing test**

```go
package main

import (
    "testing"
)

func TestMonthRevenueBackfill_Validation(t *testing.T) {
    // Test that date parsing works correctly
    start := parseDate("2024-01-01")
    end := parseDate("2026-04-30")
    if end.Before(start) {
        t.Fatal("end date should be after start date")
    }
}

func TestMonthRevenueBackfill_Deduplication(t *testing.T) {
    existing := map[string]struct{}{
        "2024-01|2330": {},
        "2024-02|2330": {},
    }
    key := "2024-01|2330"
    if _, ok := existing[key]; ok {
        // Should skip this
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go build ./cmd/backfill-month-revenue 2>&1`
Expected: FAIL - directory does not exist

- [ ] **Step 3: Create the command**

```go
package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "time"

    "github.com/kaecer68/atlas-go/internal/marketdata"
    "github.com/kaecer68/atlas-go/internal/monitoring"
)

const (
    finmindBaseURL = "https://api.finmindtrade.com/api/v4"
    rateLimit      = 600
    pacingSeconds  = 6
)

var (
    startDate  = flag.String("start", "2024-01-01", "backfill start date (YYYY-MM-DD)")
    endDate    = flag.String("end", "2026-04-30", "backfill end date (YYYY-MM-DD)")
    symbolsArg = flag.String("symbols", "", "comma-separated stock IDs (or use fundamentals.json)")
    dryRun     = flag.Bool("dry-run", false, "print what would be added without writing")
)

type FinMindResponse struct {
    Msg    string `json:"msg"`
    Status int    `json:"status"`
    Data   []struct {
        Date        string  `json:"date"`
        StockID     string  `json:"stock_id"`
        Revenue     float64 `json:"revenue"`
        RevenueMonth int    `json:"revenue_month"`
        RevenueYear  int    `json:"revenue_year"`
    } `json:"data"`
}

func main() {
    flag.Parse()

    stateDir := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "state")
    os.MkdirAll(stateDir, 0755)

    apiKey := os.Getenv("FINMIND_API_KEY")
    if apiKey == "" {
        fmt.Fprintf(os.Stderr, "FINMIND_API_KEY not set\n")
        os.Exit(1)
    }

    symbols := loadSymbols(*symbolsArg)
    fmt.Printf("Backfill month revenue for %d symbols from %s to %s\n", len(symbols), *startDate, *endDate)

    outputPath := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "replay", "month_revenue.jsonl")
    existing := loadExistingRecords(outputPath)

    client := &http.Client{Timeout: 30 * time.Second}
    limiter := marketdata.NewRateLimiter(rateLimit)

    var newRecords int
    startTime := time.Now()

    for i, symbol := range symbols {
        fmt.Printf("[%d/%d] Fetching %s...\n", i+1, len(symbols), symbol)

        url := fmt.Sprintf("%s/data?dataset=TaiwanStockMonthRevenue&data_id=%s&start_date=%s&end_date=%s",
            finmindBaseURL, symbol, *startDate, *endDate)

        body, err := marketdata.FetchWithRetry(context.Background(), client, url, apiKey, limiter, 3)
        if err != nil {
            fmt.Fprintf(os.Stderr, "[%s] failed: %v\n", symbol, err)
            continue
        }

        var resp FinMindResponse
        if err := json.Unmarshal(body, &resp); err != nil {
            fmt.Fprintf(os.Stderr, "[%s] parse error: %v\n", symbol, err)
            continue
        }

        if resp.Status != 200 {
            fmt.Fprintf(os.Stderr, "[%s] API error: %s\n", symbol, resp.Msg)
            continue
        }

        for _, item := range resp.Data {
            key := fmt.Sprintf("%s|%s", item.Date, item.StockID)
            if _, ok := existing[key]; ok {
                continue
            }

            if !*dryRun {
                fmt.Printf("%s|%s|%.0f\n", item.Date, item.StockID, item.Revenue)
            }
            existing[key] = struct{}{}
            newRecords++
        }

        time.Sleep(time.Duration(pacingSeconds) * time.Second)
    }

    elapsed := time.Since(startTime)
    latencyMs := elapsed.Milliseconds()

    monitoring.RecordChannelFetch(stateDir, "backfill_month_revenue", "ok", "", limiter.Remaining(), latencyMs)

    if *dryRun {
        fmt.Printf("\nDry run: would add %d new records for %d symbols\n", newRecords, len(symbols))
    } else {
        fmt.Printf("\nBackfill complete: added %d new records\n", newRecords)
    }

    fmt.Printf("Time elapsed: %v, rate limit remaining: %d\n", elapsed, limiter.Remaining())
}

func loadSymbols(symbolsArg string) []string {
    if symbolsArg != "" {
        parts := strings.Split(symbolsArg, ",")
        return parts
    }

    fundamentalsPath := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "fundamentals.json")
    data, err := os.ReadFile(fundamentalsPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "failed to read fundamentals.json: %v\n", err)
        return nil
    }

    var fund map[string]interface{}
    json.Unmarshal(data, &fund)

    var symbols []string
    for id := range fund {
        symbols = append(symbols, id)
    }
    sort.Strings(symbols)
    if len(symbols) > 100 {
        symbols = symbols[:100]
    }
    return symbols
}

func loadExistingRecords(path string) map[string]struct{} {
    existing := make(map[string]struct{})
    f, err := os.Open(path)
    if err != nil {
        return existing
    }
    defer f.Close()

    br := io.LimitReader(f, 100<<20)
    dec := json.NewDecoder(br)
    for dec.More() {
        var rec map[string]interface{}
        if err := dec.Decode(&rec); err != nil {
            continue
        }
        date, _ := rec["date"].(string)
        stockID, _ := rec["stock_id"].(string)
        if date != "" && stockID != "" {
            existing[date+"|"+stockID] = struct{}{}
        }
    }
    return existing
}
```

- [ ] **Step 4: Build and verify**

Run: `go build ./cmd/backfill-month-revenue && echo "BUILD SUCCESS"`
Expected: BUILD SUCCESS

- [ ] **Step 5: Commit**

```bash
git add cmd/backfill-month-revenue/main.go
git commit -m "feat(cmd): add backfill-month-revenue command for FinMind month revenue history"
```

---

## Task 4: `backfill-institutional-investors` Command

**Files:**
- Create: `cmd/backfill-institutional-investors/main.go`

- [ ] **Step 1: Write the failing test (build test)**

Run: `go build ./cmd/backfill-institutional-investors 2>&1`
Expected: FAIL - directory does not exist

- [ ] **Step 2: Create the command**

```go
package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "net/http"
    "path/filepath"
    "sort"
    "strings"
    "time"

    "github.com/kaecer68/atlas-go/internal/marketdata"
    "github.com/kaecer68/atlas-go/internal/monitoring"
)

const (
    finmindBaseURL = "https://api.finmindtrade.com/api/v4"
    rateLimit      = 600
    pacingSeconds  = 6
)

var (
    symbolsArg = flag.String("symbols", "", "comma-separated stock IDs (or use fundamentals.json)")
    date       = flag.String("date", "", "specific date (YYYY-MM-DD), default: yesterday")
    dryRun     = flag.Bool("dry-run", false, "print what would be added without writing")
)

type InstitutionalInvestorRow struct {
    Date     string  `json:"date"`
    StockID  string  `json:"stock_id"`
    Name     string  `json:"name"`
    Buy      float64 `json:"buy"`
    Sell     float64 `json:"sell"`
    Net      float64 `json:"net"`
}

func main() {
    flag.Parse()

    stateDir := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "state")
    os.MkdirAll(stateDir, 0755)

    apiKey := os.Getenv("FINMIND_API_KEY")
    if apiKey == "" {
        fmt.Fprintf(os.Stderr, "FINMIND_API_KEY not set\n")
        os.Exit(1)
    }

    symbols := loadSymbols(*symbolsArg)

    targetDate := *date
    if targetDate == "" {
        yesterday := time.Now().AddDate(0, 0, -1)
        targetDate = yesterday.Format("2006-01-02")
    }

    fmt.Printf("Backfill institutional investors for %d symbols on %s\n", len(symbols), targetDate)

    outputPath := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "replay", "institutional_investors.jsonl")
    existing := loadExistingRecords(outputPath)

    client := &http.Client{Timeout: 30 * time.Second}
    limiter := marketdata.NewRateLimiter(rateLimit)

    var newRecords int
    startTime := time.Now()

    for i, symbol := range symbols {
        fmt.Printf("[%d/%d] Fetching %s...\n", i+1, len(symbols), symbol)

        reqURL := fmt.Sprintf("%s/data?dataset=TaiwanStockInstitutionalInvestorsBuySell&data_id=%s&start_date=%s&end_date=%s",
            finmindBaseURL, symbol, targetDate, targetDate)

        body, err := marketdata.FetchWithRetry(context.Background(), client, reqURL, apiKey, limiter, 3)
        if err != nil {
            fmt.Fprintf(os.Stderr, "[%s] failed: %v\n", symbol, err)
            continue
        }

        var resp struct {
            Msg    string `json:"msg"`
            Status int    `json:"status"`
            Data   []struct {
                Date    string  `json:"date"`
                StockID string  `json:"stock_id"`
                Name    string  `json:"name"`
                Buy     float64 `json:"buy"`
                Sell    float64 `json:"sell"`
            } `json:"data"`
        }
        if err := json.Unmarshal(body, &resp); err != nil {
            fmt.Fprintf(os.Stderr, "[%s] parse error: %v\n", symbol, err)
            continue
        }

        if resp.Status != 200 {
            fmt.Fprintf(os.Stderr, "[%s] API error: %s\n", symbol, resp.Msg)
            continue
        }

        for _, item := range resp.Data {
            key := fmt.Sprintf("%s|%s|%s", item.Date, item.StockID, item.Name)
            if _, ok := existing[key]; ok {
                continue
            }

            if !*dryRun {
                fmt.Printf("%s|%s|%s|buy=%.0f|sell=%.0f|net=%.0f\n",
                    item.Date, item.StockID, item.Name, item.Buy, item.Sell, item.Buy-item.Sell)
            }
            existing[key] = struct{}{}
            newRecords++
        }

        time.Sleep(time.Duration(pacingSeconds) * time.Second)
    }

    elapsed := time.Since(startTime)
    monitoring.RecordChannelFetch(stateDir, "backfill_institutional_investors", "ok", "", limiter.Remaining(), elapsed.Milliseconds())

    if *dryRun {
        fmt.Printf("\nDry run: would add %d new records\n", newRecords)
    } else {
        fmt.Printf("\nBackfill complete: added %d new records\n", newRecords)
    }
}

func loadSymbols(symbolsArg string) []string {
    if symbolsArg != "" {
        return strings.Split(symbolsArg, ",")
    }

    fundamentalsPath := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "fundamentals.json")
    data, err := os.ReadFile(fundamentalsPath)
    if err != nil {
        return nil
    }

    var fund map[string]interface{}
    json.Unmarshal(data, &fund)

    var symbols []string
    for id := range fund {
        symbols = append(symbols, id)
    }
    sort.Strings(symbols)
    if len(symbols) > 100 {
        symbols = symbols[:100]
    }
    return symbols
}

func loadExistingRecords(path string) map[string]struct{} {
    existing := make(map[string]struct{})
    f, err := os.Open(path)
    if err != nil {
        return existing
    }
    defer f.Close()

    br := io.LimitReader(f, 100<<20)
    dec := json.NewDecoder(br)
    for dec.More() {
        var rec map[string]interface{}
        if err := dec.Decode(&rec); err != nil {
            continue
        }
        date, _ := rec["date"].(string)
        stockID, _ := rec["stock_id"].(string)
        name, _ := rec["name"].(string)
        if date != "" && stockID != "" {
            existing[date+"|"+stockID+"|"+name] = struct{}{}
        }
    }
    return existing
}
```

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/backfill-institutional-investors && echo "BUILD SUCCESS"`
Expected: BUILD SUCCESS

- [ ] **Step 4: Commit**

```bash
git add cmd/backfill-institutional-investors/main.go
git commit -m "feat(cmd): add backfill-institutional-investors command"
```

---

## Task 5: `backfill-financial-statements` Command

**Files:**
- Create: `cmd/backfill-financial-statements/main.go`

- [ ] **Step 1: Write the failing test (build test)**

Run: `go build ./cmd/backfill-financial-statements 2>&1`
Expected: FAIL - directory does not exist

- [ ] **Step 2: Create the command**

```go
package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "net/http"
    "path/filepath"
    "sort"
    "strings"
    "time"

    "github.com/kaecer68/atlas-go/internal/marketdata"
    "github.com/kaecer68/atlas-go/internal/monitoring"
)

const (
    finmindBaseURL = "https://api.finmindtrade.com/api/v4"
    rateLimit      = 600
    pacingSeconds  = 6
)

var (
    startDate  = flag.String("start", "2024-01-01", "backfill start date (YYYY-MM-DD)")
    endDate    = flag.String("end", "2026-04-30", "backfill end date (YYYY-MM-DD)")
    symbolsArg = flag.String("symbols", "", "comma-separated stock IDs (or use fundamentals.json)")
    dryRun     = flag.Bool("dry-run", false, "print what would be added without writing")
)

func main() {
    flag.Parse()

    stateDir := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "state")
    os.MkdirAll(stateDir, 0755)

    apiKey := os.Getenv("FINMIND_API_KEY")
    if apiKey == "" {
        fmt.Fprintf(os.Stderr, "FINMIND_API_KEY not set\n")
        os.Exit(1)
    }

    symbols := loadSymbols(*symbolsArg)
    fmt.Printf("Backfill financial statements for %d symbols from %s to %s\n", len(symbols), *startDate, *endDate)

    outputPath := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "replay", "financial_statements.jsonl")
    existing := loadExistingRecords(outputPath)

    client := &http.Client{Timeout: 30 * time.Second}
    limiter := marketdata.NewRateLimiter(rateLimit)

    var newRecords int
    startTime := time.Now()

    for i, symbol := range symbols {
        fmt.Printf("[%d/%d] Fetching %s...\n", i+1, len(symbols), symbol)

        reqURL := fmt.Sprintf("%s/data?dataset=TaiwanStockFinancialStatements&data_id=%s&start_date=%s&end_date=%s",
            finmindBaseURL, symbol, *startDate, *endDate)

        body, err := marketdata.FetchWithRetry(context.Background(), client, reqURL, apiKey, limiter, 3)
        if err != nil {
            fmt.Fprintf(os.Stderr, "[%s] failed: %v\n", symbol, err)
            continue
        }

        var resp struct {
            Msg    string `json:"msg"`
            Status int    `json:"status"`
            Data   []struct {
                Date        string  `json:"date"`
                StockID     string  `json:"stock_id"`
                OriginName  string  `json:"origin_name"`
                Value       float64 `json:"value"`
            } `json:"data"`
        }
        if err := json.Unmarshal(body, &resp); err != nil {
            fmt.Fprintf(os.Stderr, "[%s] parse error: %v\n", symbol, err)
            continue
        }

        if resp.Status != 200 {
            fmt.Fprintf(os.Stderr, "[%s] API error: %s\n", symbol, resp.Msg)
            continue
        }

        for _, item := range resp.Data {
            key := fmt.Sprintf("%s|%s|%s", item.Date, item.StockID, item.OriginName)
            if _, ok := existing[key]; ok {
                continue
            }

            if !*dryRun {
                fmt.Printf("%s|%s|%s|%.2f\n", item.Date, item.StockID, item.OriginName, item.Value)
            }
            existing[key] = struct{}{}
            newRecords++
        }

        time.Sleep(time.Duration(pacingSeconds) * time.Second)
    }

    elapsed := time.Since(startTime)
    monitoring.RecordChannelFetch(stateDir, "backfill_financial_statements", "ok", "", limiter.Remaining(), elapsed.Milliseconds())

    if *dryRun {
        fmt.Printf("\nDry run: would add %d new records\n", newRecords)
    } else {
        fmt.Printf("\nBackfill complete: added %d new records\n", newRecords)
    }
}

func loadSymbols(symbolsArg string) []string {
    if symbolsArg != "" {
        return strings.Split(symbolsArg, ",")
    }

    fundamentalsPath := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "fundamentals.json")
    data, err := os.ReadFile(fundamentalsPath)
    if err != nil {
        return nil
    }

    var fund map[string]interface{}
    json.Unmarshal(data, &fund)

    var symbols []string
    for id := range fund {
        symbols = append(symbols, id)
    }
    sort.Strings(symbols)
    if len(symbols) > 100 {
        symbols = symbols[:100]
    }
    return symbols
}

func loadExistingRecords(path string) map[string]struct{} {
    existing := make(map[string]struct{})
    f, err := os.Open(path)
    if err != nil {
        return existing
    }
    defer f.Close()

    br := io.LimitReader(f, 100<<20)
    dec := json.NewDecoder(br)
    for dec.More() {
        var rec map[string]interface{}
        if err := dec.Decode(&rec); err != nil {
            continue
        }
        date, _ := rec["date"].(string)
        stockID, _ := rec["stock_id"].(string)
        originName, _ := rec["origin_name"].(string)
        if date != "" && stockID != "" && originName != "" {
            existing[date+"|"+stockID+"|"+originName] = struct{}{}
        }
    }
    return existing
}
```

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/backfill-financial-statements && echo "BUILD SUCCESS"`
Expected: BUILD SUCCESS

- [ ] **Step 4: Commit**

```bash
git add cmd/backfill-financial-statements/main.go
git commit -m "feat(cmd): add backfill-financial-statements command"
```

---

## Task 6: Integration - Add RateLimiter to finmind_backfill.go

**Files:**
- Modify: `internal/marketdata/finmind_backfill.go` (add NewRateLimiter export)

- [ ] **Step 1: Add NewRateLimiter export**

```go
// NewRateLimiter creates a rate limiter with the given requests per hour limit
func NewRateLimiter(limit int) *rateLimiter {
    return newRateLimiter(limit)
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./cmd/backfill-month-revenue ./cmd/backfill-institutional-investors ./cmd/backfill-financial-statements && echo "ALL BUILD SUCCESS"`
Expected: ALL BUILD SUCCESS

- [ ] **Step 3: Verify tests**

Run: `go test ./internal/marketdata/... ./internal/monitoring/... && echo "ALL TESTS PASS"`
Expected: ALL TESTS PASS

- [ ] **Step 4: Commit**

```bash
git add internal/marketdata/finmind_backfill.go
git commit -m "fix(marketdata): export NewRateLimiter for backfill commands"
```

---

## Task 7: Initial Data Fetch Test

**Files:**
- (No file changes - just run commands to verify)

- [ ] **Step 1: Test month revenue backfill with dry-run**

Run: `cd /Users/kaecer/workspace/atlas && go run ./cmd/backfill-month-revenue --start 2024-01-01 --end 2024-03-31 --dry-run 2>&1 | head -30`
Expected: Output showing records to be fetched for 2330, etc.

- [ ] **Step 2: Test institutional investors backfill with dry-run**

Run: `cd /Users/kaecer/workspace/atlas && go run ./cmd/backfill-institutional-investors --dry-run 2>&1 | head -20`
Expected: Output showing yesterday's data for top symbols

- [ ] **Step 3: Commit health record update**

Run: `cat /Users/kaecer/workspace/atlas/data/state/channel_health.json`
Expected: Shows health records with last_run timestamps

---

## Verification Checklist

After all tasks:
- [ ] `go build ./cmd/backfill-month-revenue` — PASS
- [ ] `go build ./cmd/backfill-institutional-investors` — PASS
- [ ] `go build ./cmd/backfill-financial-statements` — PASS
- [ ] `go test ./internal/marketdata/... ./internal/monitoring/...` — ALL PASS
- [ ] Dry-run produces expected output
- [ ] Health records written to `data/state/channel_health.json`