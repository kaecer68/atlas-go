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
