# quotes 新鮮度 runbook（F54 phase 1）

> 告警：`QuotesDataStale` / `QuotesFreshnessUnverifiable` / `QuotesFreshnessExporterDown`
> （`monitoring/rules/quotes_freshness_alerts.yml`，指標由背景任務
> `quotes_freshness_metrics_export` 每 5 分鐘匯出）。
> 建立背景：2026-10-02 實損 —— quotes 無排程結構性保障，universe build 僅 2/117
> 有 quotes 而無人發現（F54）。

## 1. 這個檢查量的是什麼

`quotes` 表（postgres migration 000012）的 `max(date)` 與「期望交易日」的比較。

期望交易日（Go 側 `monitoring.ExpectedQuoteDate`，Asia/Taipei 日曆）：

- **交易日 18:00 起** ⇒ 要求**當日** quotes 已落地。
- **非交易日 / 交易日 18:00 前** ⇒ 要求「之前最近一個交易日」（含假日，
  `internal/taiwanholidays`，2021–2040 實測表）。

判讀順序（**永遠先看 run_ok**）：

1. `atlas_quotes_freshness_run_ok` = 0 ⇒ 新鮮度未知（查詢失敗），`max_date` 是凍結值，
   走 §3。
2. `atlas_quotes_freshness_ok` = 0 ⇒ 真的落後或空表，走 §2。
3. `atlas_quotes_max_date_timestamp_seconds` 只是人可讀補充，**沒有規則拿它做判定**。

## 2. QuotesDataStale（warning）triage

1. 看落後多少：
   `docker exec atlas-go curl -s localhost:18080/metrics | grep -E 'atlas_quotes_'`
2. **先排除已知假陽性**（known_issue，不當故障回報）：
   - 交易日 **18:00–19:00** 之間短暫 firing 後自行 resolve ⇒ backfill 落地時刻漂移，
     對照 `docker logs atlas-go --since 24h 2>&1 | grep -E 'auto_quote_backfill|quote_backfill'`。
   - 連假結束後第一個交易日的 firing ⇒ 多半是**真實落後**（連假 backfill 沒跑），
     週一早晨 universe build 前必須補資料 —— 這正是本告警的主要保護場景。
3. 真實落後的處置：先查 backfill 任務為何沒寫入（日誌、FinMind quota、資料源），
   補資料後告警自行 resolve。禁止「直接跳過告警去跑 universe build」。

## 3. QuotesFreshnessUnverifiable（error）triage

查詢本身失敗（資料庫連線 / migration 缺表）：

1. `docker logs atlas-go --since 1h 2>&1 | grep -E 'quotes_freshness|query_failed|postgres'`
2. 資料庫可達性與 `quotes` 表存在性（migration 000012）。
3. 與 AtlasGoTargetDown / 資料庫連線告警對照同一根因，不要重複 paging。

## 4. QuotesFreshnessExporterDown（warning）triage

1. `docker logs atlas-go --since 1h 2>&1 | grep 'registered quotes_freshness_metrics_export'`
   —— 沒有這行 ⇒ 看是否有 skipped log：
   - `quote store unavailable`：quote store 初始化失敗（另案）。
   - `does not implement ledger.QuoteMaxDater`：**JSONL 後端不支援此檢查**
     （已知限制；生產 backend=postgres 不受影響）。
2. 本條 firing 時，另外兩條的輸出不可信（全部凍結），先修本條。

## 5. 已知限制（刻意取捨，phase 1）

- **Cutoff 18:00 是固定簡化門檻**，不是 backfill 落地時刻觀測；`for: 1h` 吸收抖動。
- JSONL / SQLite-without-shared-db 等後端：只有 postgres、sqlite 實作
  `ledger.QuoteMaxDater`；JSONL 部署**沒有此檢查**（註冊時跳過）。
- 假日表缺新假日時：硬護欄（max(date) 比今天舊 >7 日曆日 ⇒ 不新鮮）限制誤判窗口，
  但仍可能在「新假日未入表」當天誤判新鮮 —— 長假後第一個交易日請人工對照
  交易所行事曆。
- 刻意**不加 silence**：連假形狀由交易日感知判定正確處理，剩餘假陽性走 §2 判讀。

## 6. 寫入端拒收（`quote_write_rejected` WARN）— 源頭阻擋，**不是告警**

> 本節記錄**寫入端守門**（`FU-20260930-03` 的最小處置①）。它**沒有** Prometheus 指標、
> 也**沒有**告警：告警化改由 **a2a-dev 的 postgres_exporter extended query** 負責
> （純設定、直接查 `quotes` 現況值、能涵蓋**既有**壞列）。本 repo 刻意**不**再放第二個
> 來源，避免重複計數／重複告警。

## 6.1 守的是什麼

`RecordQuotes`（`internal/ledger/quote_write_guard.go`）在**寫入前**擋掉壞值：

| 欄位 | 守什麼 | 為什麼 |
|------|--------|--------|
| `close` | 非正（<= 0）或非有限（NaN/±Inf） | `Close=0` 是 `FU-20260930-03` 的既有壞值；任何未守門的 return/volatility 消費者都會讀成 **−100%** |
| `open` / `high` / `low` | **只守非有限** | `Open/High/Low = 0` 而 `close` 可用是合法列（`cmd/cron-quote-backfill` 只填 `Close`/`Volume`）；擋掉它等於丟掉可用的收盤價 |

被拒收的列**不入庫**，每列一則 WARN（`ledger_quote_write_guard` / `quote_write_rejected`，
帶 `backend`／`reason`／`symbol`／`date`／`source`）。`reason ∈ non_positive｜non_finite`。
每 process 上限 **100 行**，第 101 列起只留一則 `quote_write_rejection_suppressed`
（被拒收的列數不再遞增記錄，所以**不以 log 行數當指標**）。

**`RecordQuotes` 的錯誤語意**：單列壞值**不會**讓整批失敗（好列照寫）；
但若一批**全部**被拒收，回傳 `*ledger.QuoteRejectionsError`
（`errors.Is(err, ledger.ErrAllQuotesRejected)`）—— 呼叫端不會誤以為該 symbol 已落地。

## 6.2 triage（看到 `quote_write_rejected` 時）

1. 找出被擋的具體列（每 process 上限 100 行）：
   `docker logs atlas-go --since 2h 2>&1 | grep quote_write_rejected | tail -20`
2. 是否同一個 backfill 任務造成（`auto_quote_backfill` 24h interval；停牌股常有 `Last=0`）：
   `docker logs atlas-go --since 2h 2>&1 | grep -E 'quote_backfill|quote_write_rejected'`
3. 表內**既有**壞值列（守門只擋新列，不修舊列）：
   `docker exec atlas-postgres psql -U atlas -d atlas -c "select count(*) from quotes where close <= 0"`
   —— 這個數字與其趨勢由 a2a-dev 的 exporter 查詢曝露，不是本 repo 的指標。
4. 處置：資料源／provider 修正，或由 operator 修資料（`FU-20260930-03` 的資料面）。

## 6.3 已知限制

- 只覆蓋走 `QuoteStore.RecordQuotes` 的寫入；直接下 SQL 的工具（`cmd/migrate-data`）
  不受保護。
- 既有 109 列 `close<=0`／10 倍價／垃圾列**不因本守門消失**；本守門只保證**不再新增**。
- 拒收是**缺值**語意：該 `(symbol, date)` 沒有列，下游走既有「無資料」路徑。
