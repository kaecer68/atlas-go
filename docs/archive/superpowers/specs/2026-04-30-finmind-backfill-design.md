# FinMind Backfill System Design

## 1. Overview

建立三個獨立的 backfill CLI 命令，透過 FinMind API 補足系統所需的歷史數據（ 月營收、財報、三大法人）。

**範圍**：Top 50-100 檔股票，2024-01 起

---

## 2. Architecture

```
cmd/
  backfill-month-revenue/main.go          # 月營收（每月更新）
  backfill-institutional-investors/main.go  # 三大法人（每日更新）
  backfill-financial-statements/main.go   # 財報（季更）
```

每個命令：
- 獨立執行，不互相依賴
- 可透過 cron / systemd 排程
- 共用相同的 logging 與監控框架

---

## 3. Shared Monitoring Layer

### Health Record Format

寫入 `data/state/channel_health.json`，結構：

```json
{
  "backfill_month_revenue": {
    "status": "ok",
    "last_run": "2026-04-30T10:00:00Z",
    "records_fetched": 150,
    "symbols_processed": 50,
    "errors": [],
    "rate_limit_remaining": 420,
    "latency_ms": 850
  }
}
```

### Monitored Metrics

| 目標 | 實作 |
|------|------|
| **數據品質** | OHLC valid range、價格變動 `%`、`null` 欄位檢查 |
| **通信品質** | HTTP status code、latency、retry 次數 |
| **失敗分類** | `429 rate_limit` / `500 server_error` / `timeout` / `empty_response` |

### Rate Limit Tracking

- 每次 API 回 call 用完後，从 response header 或返回的 limit 值更新 remaining count
- 逼近（< 50 req）時输出 WARNING
- 達到 0 時自動 sleep 直到下一小時

---

## 4. Data Output

各 backfill 命令寫入以下 JSONL 檔案：

| 命令 | 輸出檔案 |
|------|---------|
| `backfill-month-revenue` | `data/replay/month_revenue.jsonl` |
| `backfill-institutional-investors` | `data/replay/institutional_investors.jsonl` |
| `backfill-financial-statements` | `data/replay/financial_statements.jsonl` |

格式：
```jsonl
{"date":"2024-01","stock_id":"2330","revenue":335003568000,...}
{"date":"2024-02","stock_id":"2330","revenue":401255128000,...}
```

每次執行前先檢查現有檔案，避免重複寫入同一筆資料（用 `date|stock_id` 作為 key）。

---

## 5. Backfill Commands Detail

### 5.1 `backfill-month-revenue`

- **dataset**: `TaiwanStockMonthRevenue`
- **參數**: `--start 2024-01-01 --end 2026-04-30 --symbols 50`
- **邏輯**:
  1. 如果 `--symbols` 未指定，讀取 `data/fundamentals.json` 的前 50 檔
  2. 每次迴圈對每個 symbol 發送一次 API call（date range = 整個 backfill 期間，一次拉完）
  3. 限速：每 request 後 sleep 6 秒（600 req/hr ÷ 50 symbols ≈ 12 秒/pacing，但我們保守用 6 秒等於半速）
  4. 遇到 429 → 讀 response header 的 `X-RateLimit-Reset`，sleep 到那個時間
  5. 遇到 500 → 等 5 秒 retry，最多 3 次

### 5.2 `backfill-institutional-investors`

- **dataset**: `TaiwanStockInstitutionalInvestorsBuySell`
- **邏輯**:
  1. date range = 每次執行時的前一天（daily catch-up）
  2. 一次迴圈對每個 symbol 發送一次 API call
  3. 限速：每 request 後 sleep 6 秒
  4. 寫入時以 `date|symbol|type` 為 key 做去重

### 5.3 `backfill-financial-statements`

- **dataset**: `TaiwanStockFinancialStatements`
- **邏輯**:
  1. 抓 2024-Q1 ~ 2025-Q4 的財報（每季一個 request per symbol）
  2. date range = 2024-01-01 ~ 2026-04-30（一次拉完）
  3. 限速：每 request 後 sleep 6 秒

---

## 6. Error Handling Strategy

| 錯誤類型 | 處理方式 |
|---------|---------|
| `429 Too Many Requests` | 解讀 `X-RateLimit-Reset` 或 `Retry-After` header，sleep 後重試 |
| `500 Internal Server Error` | 等 5 秒 retry，最多 3 次 |
| `timeout / network error` | 等 3 秒 retry，最多 2 次 |
| `empty response (status 200)` | 視為「該期間無資料」，log 後 continue |
| `401 Unauthorized` | 立即失敗，log "Invalid API key"，不 retry |

---

## 7. Execution Examples

```bash
# 月營收一次性 backfill
go run ./cmd/backfill-month-revenue --start 2024-01-01 --end 2026-04-30

# 三大法人每日 catch-up（cron job 建議 16:00 執行）
go run ./cmd/backfill-institutional-investors

# 財報一次性 backfill（季更）
go run ./cmd/backfill-financial-statements --start 2024-01-01 --end 2026-04-30

# 所有 dry-run 模式（不寫入）
go run ./cmd/backfill-month-revenue --dry-run
```

---

## 8. Verification

每次執行後：
1. 確認寫入記錄數 > 0
2. 確認 rate limit 未被卡死
3. 確認 health record 寫入成功

CI 检查：
- `go build ./cmd/backfill-*`
- `go vet ./cmd/backfill-*`