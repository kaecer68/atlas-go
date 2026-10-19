# replay runbook（replay 資料異常：資料日落點／JSONL 落後／CSV 停滯／匯出任務／sync 未落地）

> **告警（5 條規則、5 個 `runbook_url` 指到本檔）**：
> `AtlasReplaySessionDateInvalid`、`AtlasReplayJsonlBehindCsv`、`AtlasReplayCsvStale`、
> `AtlasReplayFreshnessExporterDown`、`AtlasReplaySyncNotLanding`。
> 規則檔：`monitoring/rules/atlas_replay_alerts.yml`（5 處 `runbook_url`，皆在本檔）。
> 指標由背景任務 `replay_freshness_metrics_export`（5 分鐘一輪，`cmd/atlas/replay_freshness_metrics_task.go`）
> 與既有任務 `channel_health_metrics_export`（5 分鐘一輪）匯出。
>
> **建立背景（2026-10-19，`FU-20260927-03`）**：這 5 個 `runbook_url` 原本全部指向
> `docs/operations/local-deploy.md`，而該檔是「部署與 `.env` 分工」文件，
> **完全不含 replay 內容**（`grep -ic replay docs/operations/local-deploy.md` ⇒ `0`）
> ⇒ 值班在告警當下點進去拿不到判讀步驟。連結本身可達（#2066 已修死鏈），但指錯了對象。
> 本檔的處置步驟**逐字取自這 5 條規則的 `description` annotation**（規則作者寫的原文），
> 沒有新增任何 repo 內找不依據的步驟。

## 1. 這個家族量的是什麼（兩個資料來源，不發明指標）

```
每日 15:30 UTC（台北 23:30） cron-replay-sync 抓 TWSE MI_INDEX
      → data/replay/tw_extended_90days.csv            （交易資料 CSV）
      → auto_backfill（24h 週期）做 CSV→JSONL 轉檔
      → data/replay/tw_extended_90days.jsonl          （FactorEngine 的正式輸入）

channel_health_metrics_export（5m）→ atlas_channel_health_status{channel="twse_replay_sync"}
replay_freshness_metrics_export（5m）→ atlas_replay_csv_latest_date_timestamp_seconds
                                      atlas_replay_jsonl_latest_date_timestamp_seconds
                                      atlas_replay_jsonl_behind_trading_days
                                      atlas_replay_freshness_checked_timestamp_seconds
```

| 序列 | 意義 | 讀它的規則 |
|---|---|---|
| `atlas_replay_csv_latest_date_timestamp_seconds` | CSV 內最新**資料日**（正規化為該日 UTC 00:00 的 Unix 秒） | `AtlasReplaySessionDateInvalid`、`AtlasReplayJsonlBehindCsv`（缺席 arm）、`AtlasReplayCsvStale` |
| `atlas_replay_jsonl_latest_date_timestamp_seconds` | 轉檔後 JSONL 內最新資料日（讀不到**不輸出**，不寫 0） | `AtlasReplayJsonlBehindCsv`（`unless` arm） |
| `atlas_replay_jsonl_behind_trading_days` | JSONL 落後 CSV 幾個**交易日**（穩態＝**1**，不是 0） | `AtlasReplayJsonlBehindCsv`（`>= 3`） |
| `atlas_replay_freshness_checked_timestamp_seconds` | 匯出任務的心跳（最後一次執行的 Unix 秒） | `AtlasReplayFreshnessExporterDown`（`absent` 或 `> 1800`） |
| `atlas_channel_health_status{channel="twse_replay_sync"}` | 抓取層判定：0=ok、1=warn/stale/**degraded**、2=error、3=inactive、4=其他 | `AtlasReplaySyncNotLanding`（`> 0`） |

**這個家族存在的理由（規則檔頭註解，實查）**：2026-08-24~09-24 CSV→JSONL 轉檔停擺一個月，
期間生產 Prometheus 上與 replay 有關的序列**全部是正常值** ——
`atlas_channel_health_status{channel="twse_replay_sync"}` 與 `{channel="twse_replay"}` 都是 `0`，
`atlas_channel_data_staleness_seconds` 只有 385.6 秒（那是 `last_fetch_at`，最後一次**嘗試抓取**，
不是資料本身的日期）⇒ **抓取層訊號在結構上看不到資料內容落後**。

## 2. 判讀前提（先看這個再看任何數字）

1. **第 1~3 條看的 gauge 是匯出任務的 last-write-wins 樣本**：任務若停止，樣本會**凍結**在最後一個值。
   `AtlasReplayFreshnessExporterDown` 就是那個獨立護欄 ⇒ 看到第 1~3 條 firing 時，
   **先確認第 4 條是 inactive**，否則你讀的是歷史快照。
2. **第 1/2/3 條（資料內容）與第 5 條（抓取記錄）是兩個不同的觀測面**，可能同時 firing。
   那是互補證據（檔案內容 vs 抓取記錄），不是同一根因重複 paging。
3. **門檻都用交易日／應用既有常數校準過**，看到 firing 不代表門檻錯：
   - CSV 停滯門檻取 **14 天**（不是 3d／72h），因為規則端拿不到交易日曆，長假（春節 9-11 天）
     會結構性誤報；快速偵測交給第 2 條（JSONL 落後）與第 5 條（sync 狀態）。
   - JSONL 落後門檻取 **3 個交易日**（不是日曆日）：穩態是 **1**（CSV 有當日、JSONL 有前一日），
     單一輪沒跑＝2、連續兩輪沒跑＝3。**不要把穩態的 1「修」成 0**。

## 3. 共同診斷（任何一條先做這三步）

```bash
# 1. 現值（app 指標家族；先確認第 4 條的心跳在動）
docker exec atlas-go curl -s localhost:18080/metrics | grep atlas_replay_

# 2. 兩個檔案的最新資料日（CSV 第一欄＝日期；JSONL 每行含日期）
docker exec atlas-cron-replay-sync tail -n 1 /app/data/replay/tw_extended_90days.csv | cut -d, -f1
docker exec atlas-cron-replay-sync tail -n 1 /app/data/replay/tw_extended_90days.jsonl

# 3. 抓取層的原始記錄（twse_replay_sync）
docker exec atlas-go grep -o '"twse_replay_sync":[^}]*}' /app/data/state/channel_health.json
```

> 步驟 1、2 的指令全部取自這 5 條規則的 annotation 原文（見 §4~§8）；
> 步驟 3 的形狀沿用 [channel-health-runbook.md](channel-health-runbook.md) §2 第 2 步（同一份檔案、同一種 JSON）。

**最常見的誤判來源**：只讀 `channel_health.json` 的 `status` 就下結論。那是「最後一次寫入」的記錄，
gauge 才是重新判定後的結果；而 gauge 本身也可能是凍結樣本（§2 第 1 點）。

## 4. `AtlasReplaySessionDateInvalid`（warning；`for: 30m`；`severity: warning`）

**症狀**：`atlas_replay_csv_latest_date_timestamp_seconds` 的日期落在**週六／週日**
（`day_of_week()` == 0 或 6），或**晚於當前時刻**（`> time()`）。
值本身就是「最新資料日」的 Unix 秒（規則刻意用 `and` 保留左邊樣本，所以 annotation 不會印成星期數字）。

**門檻依據（規則檔註解）**：`for: 30m` ＝ 6 個連續新樣本（gauge 每 5m 重寫）；幻影列一旦寫入，
錯誤狀態會持續整個週末／假日（≥24h）⇒ 30m 不會漏，只濾掉重新載入／單輪抖動。

**可能原因**（依 `description` 的背景段，皆有 repo 依據）：

1. **寫入端用 `time.Now()` 蓋章**（2026-08-29~09-25 實際發生：396 列幻影，weekend=352／holiday=44，每交易日 44 檔）。
   寫入端已由 #2057 根治（帶日期的 MI_INDEX + 回應日期守門 + 休市日 guard）
   ⇒ 若在**已部署該修正**的環境仍 firing，優先查部署版本與資料來源形狀。
2. **回填／人工寫入**走同一條 `IsTaiwanTradingDay` 閘門，正常不會產生週末列（見下方原文第 3 步）。

**立即檢查（`description` annotation 原文，逐字）**：

```text
replay CSV 的最新資料日 = {{ $value | humanizeTimestamp }}（UTC）,
落在週末或晚於當前時刻 ⇒ 這一列不可能是交易所產生的事實。
背景:2026-08-29~09-25 曾以 time.Now() 蓋章寫入 396 列幻影,
寫入端已由 #2057 根治（帶日期的 MI_INDEX + 回應日期守門 + 休市日 guard）。
立即檢查:
  1. 最新幾天的資料日與列數（每交易日 44 檔）:
     tail -n 88 /app/data/replay/tw_extended_90days.csv | cut -d, -f1 | sort | uniq -c
  2. 寫入端最近的判定結果（是否出現 data-date guard / 非交易日 guard 的 log）:
     docker logs atlas-cron-replay-sync --since 72h 2>&1 | grep -E "DailySync|refus|not a Taiwan trading day"
  3. 是否為**人工/回填**寫入（backfill 走同一條 IsTaiwanTradingDay 閘門,不該產生週末列）:
     docker logs atlas-cron-replay-sync --since 72h 2>&1 | grep -E "backfill"
注意:本條只判「週末」與「未來日」;**假日（週間休市）**的幻影列需要交易日曆,
目前無法在 Prometheus 端判別（見 PR body 的建議票）。
```

**補救**：依上方第 1、2 步定位後，處置方向是**修寫入端**（`cmd/daily-replay-sync`）而不是修資料檔；
若是既有幻影列，需人工剔除並重跑該日回填（回填同一條閘門，不該再產生週末列）。

**升級**：出現**假日（週間休市）**的幻影列時 —— 本條在 Prometheus 端**判不出來**（需要交易日曆），
屬已知限制（§10），需轉 `internal/taiwanholidays` 或 `internal/replay` 的擁有者。

## 5. `AtlasReplayJsonlBehindCsv`（warning；`for: 1h`）

**症狀**：兩個 arm 任一成立 ——
① `atlas_replay_jsonl_behind_trading_days >= 3`（值＝落後的**交易日數**）；
② CSV 序列存在但 JSONL 序列**不存在**（`unless`，值＝CSV 資料日的 Unix 秒）。

**門檻依據（規則檔註解）**：舊制是「> 2 個**日曆日**」，#2145 改成「3 個**交易日**」——
日曆日在長連假會結構性誤報（JSONL 停在 09-24、CSV 到 09-29，日曆差 5 天但交易日只差 1）。
門檻 3 ＝ 完整重現舊意圖「單一輪沒跑不 page、連續兩輪沒跑才 page」。
⚠️ **穩態是 1，不是 0**；`atlas_replay_jsonl_behind_trading_days` 的端點約定是
(JSONL 最新日, CSV 最新日]（含 CSV 端點）。

**可能原因**：

1. **CSV→JSONL 轉檔停擺**（實際事故：2026-08-24~09-24 一個月）。
2. **沒有缺口的日子結構上不轉檔**（#2057 之前的已知缺陷：`auto_backfill` 在
   「CSV 最大日+1 > 今天」時**提前 return**，而轉檔在 return 之後）。
3. **JSONL 被刪或搬到非設定路徑** ⇒ 第二個 arm（`unless`）成立。讀不到**不輸出**序列是刻意設計
   （寫 0 會被讀成 1970 年，見 issue #1995）。
4. **部署順序**：`atlas_replay_jsonl_behind_trading_days` 只在**第一 arm** ⇒ 規則與 binary 必須同批上；
   只上規則時第一 arm 會靜默，缺席 arm 不受影響。

**立即檢查（`description` annotation 原文，逐字）**：

```text
JSONL（FactorEngine 的正式輸入）落後 CSV **{{ $value }}**（數值 arm 的值＝落後的
**交易日數**,門檻 ≥ 3;第二個 arm 缺席時,值＝CSV 資料日的 Unix 秒）。
交易日由台灣假日曆判定（taiwanholidays.IsTradingDay）⇒ **長連假不會誤報**:
例如 JSONL 停在 09-24（中秋節前最後資料日）、CSV 已到 09-29（中間 09-25／09-28 休市）
只算落後 1 個交易日,不會 firing。
歷史實例:2026-08-24~09-24 轉檔停擺一個月,期間沒有任何告警。
立即檢查（轉檔在 auto_backfill 內執行,不是獨立 cron）:
  1. 兩個檔案的最新資料日:
     docker exec atlas-cron-replay-sync tail -n 1 /app/data/replay/tw_extended_90days.csv | cut -d, -f1
     docker exec atlas-cron-replay-sync tail -n 1 /app/data/replay/tw_extended_90days.jsonl
  2. 轉檔是否被執行過（#2057 之前的已知缺陷:auto_backfill 在
     「CSV 最大日+1 > 今天」時**提前 return**,而轉檔在 return 之後,
     所以沒有缺口的日子結構上不會轉檔）:
     docker logs atlas-go --since 72h 2>&1 | grep -E "backfill gap detected|backfill CSV→JSONL conversion"
     沒有 backfill gap detected 是**正常的**（沒有缺口就該沒有）,但
     「JSONL 落後」時要確認 conversion 那行是否出現過。
  3. 補救（單次,冪等;CSV 不會被改動）:重啟 atlas-go 讓 auto_backfill
     重新排程,或在主機上手動跑該轉檔（importer.ImportTWOpenDataCSVToJSONL
     的入口是 auto_backfill 任務;沒有獨立 CLI,所以優先用前者）。
```

> **註（2026-10-19 實查，規則檔未改）**：上方第 3 步的「沒有獨立 CLI」**已不成立** ——
> repo 內存在 `cmd/import-replay/main.go`，它呼叫同一個 `importer.ImportTWOpenDataCSVToJSONL`（`-source`／`-target`）。
> 兩條路都可行；本檔不因此改寫 annotation（改規則檔不在 `FU-20260927-03` 的範圍）。

**補救**：優先「重啟 atlas-go 讓 `auto_backfill` 重新排程」（原文第 3 步）；
單次手動轉檔是等價且冪等的替代（CSV 不會被改動）。

**升級**：重啟後下一輪（24h 週期）仍未收斂，或 `backfill CSV→JSONL conversion` 反覆失敗
⇒ 轉 `internal/importer` / `cmd/atlas/operations_tasks.go` 的擁有者。

## 6. `AtlasReplayCsvStale`（warning；`for: 1h`）

**症狀**：`time() - atlas_replay_csv_latest_date_timestamp_seconds > 14 * 86400`。

**門檻依據（規則檔註解）**：14 天 ＝ 應用自己對 replay 資料日的 **error** 門檻
（`adapter_twse.HealthCheck` 與 `monitoring/service.checkReplayHealth` 都是 <3d ok / <14d warn / 其後 error）。
為什麼不更緊：規則端沒有交易日曆，長假（春節 9-11 天）不會有新列；
2026-09-27 實測「健康狀態下 CSV 已 3.4 天沒動」（09-25 中秋、09-26/27 週末）。
本條是**backstop**：第 2 條只看「兩檔不一致」，若 CSV 與 JSONL **一起**停住，只有本條會響。

**可能原因**：

1. **寫入端掛掉**／cron 容器沒跑（排程每日 `30 15 * * *`，見 `docs/operations/docker-compose.crons.yml`）。
2. **休市日 guard 誤判成長期休市**（`internal/taiwanholidays` 已知缺 9 個真實休市日 —— 原文第 2 步）。
3. **磁碟滿或寫入權限**（原文第 3 步）。

**立即檢查（`description` annotation 原文，逐字）**：

```text
replay CSV 最新資料日 = {{ $value | humanizeDuration }} 前（門檻 14 天）。
14 天不是憑感覺:應用自己對 replay 資料日的判定就是 <3d ok / <14d warn /
其後 error（adapter_twse.HealthCheck 與 monitoring/service.checkReplayHealth）。
立即檢查:
  1. cron 容器與最近一次執行結果（排程為每日 15:30 UTC）:
     docker ps -a --filter name=atlas-cron-replay-sync
     docker logs atlas-cron-replay-sync --since 7d 2>&1 | tail -40
  2. 是否為「日曆被誤判成休市」（internal/taiwanholidays 已知缺 9 個真實休市日）:
     docker logs atlas-cron-replay-sync --since 7d 2>&1 | grep "not a Taiwan trading day"
  3. 磁碟空間與寫入權限:
     docker exec atlas-cron-replay-sync df -h /app/data/replay
```

**補救**：先把 cron 容器救回來（重啟／修正排程），再用回填補缺口；
**不要**放寬 14 天門檻來讓它閉嘴（門檻與理由必須一起改，見 §9）。

**升級**：`df -h` 顯示寫滿、或休市日判定錯誤造成長期停工 ⇒ 轉 `internal/taiwanholidays`／
本機磁碟與部署（[local-deploy.md](local-deploy.md)）的擁有者。

## 7. `AtlasReplayFreshnessExporterDown`（warning；`for: 15m`）

**症狀**：`atlas_replay_freshness_checked_timestamp_seconds` **缺席**（`absent`）
或已停止更新 > 1800 秒（6 個匯出週期），且 `up{job="atlas-go"} == 1`（服務本身還活著）。

**為什麼獨立成條（規則檔註解）**：第 1~3 條的 gauge 是 last-write-wins，匯出任務若停止
（panic、任務被移除、指標被改名），序列會**凍結**在最後一個值 ⇒ 那三條會以「看起來正常」的方式靜默。
`and on() (up{job="atlas-go"} == 1)` 閘門避免與 `AtlasGoTargetDown` 對同一根因重複 paging。

**⚠️ 這條 firing 時，第 1~3 條的值一律不能當現況 —— 先修這條。**

**可能原因**：

1. **剛啟動**：指標需要一次 5m 週期才會出現；部署後 5 分鐘內 absent 是**預期行為**。
2. **任務未註冊／未執行**：`replay_freshness_metrics_export` 由 `registerReplayFreshnessMetricsTask`
   註冊（`cmd/atlas/replay_freshness_metrics_task.go`），看 background_task log。
3. **檔案被搬到非設定路徑**：任務讀 `cfg.ReplayDataPath`（`internal/config/config.go`）；
   讀不到時任務**刻意回 nil**（缺席是被觀測的狀態，不是任務錯誤）⇒ 不會有 error log 可找。

**立即檢查（`description` annotation 原文，逐字）**：

```text
`replay_freshness_metrics_export`（5m 週期）的心跳序列
`atlas_replay_freshness_checked_timestamp_seconds` 缺席或已停止 >30 分鐘。
⚠️ 這代表 AtlasReplaySessionDateInvalid / AtlasReplayJsonlBehindCsv /
AtlasReplayCsvStale 看到的都是**凍結樣本**,不能當現況 —— 先修這條。
立即檢查:
  1. 服務是否剛啟動（指標需要一次 5m 週期才會出現;部署後 5 分鐘內 absent 是預期）:
     docker exec atlas-go curl -s localhost:18080/metrics | grep atlas_replay_
  2. 任務是否註冊/執行:
     docker logs atlas-go --since 30m 2>&1 | grep -E "replay_freshness|background_task"
  3. 檔案是否被搬到非設定路徑（任務讀 cfg.ReplayDataPath）:
     docker exec atlas-go ls -la /app/data/replay/
```

**補救**：修匯出任務（重啟、確認註冊、把資料檔放回設定路徑）。修好後等**一次** 5m 週期才看得到序列。

**升級**：任務反覆缺席／`Register` 之後仍無序列 ⇒ 轉 `cmd/atlas/replay_freshness_metrics_task.go`
與 `internal/apigateway`（`BackgroundTaskManager`）的擁有者。

## 8. `AtlasReplaySyncNotLanding`（warning；`for: 1h`）

**症狀**：`atlas_channel_health_status{channel="twse_replay_sync"} > 0`
（1=warn/stale/**degraded**、2=error、3=inactive、4=其他）。

**門檻依據（規則檔註解）**：既有規則**只**比對 `== 2`（error），而「跑了、但資料沒落地」在寫入端記的是
**degraded**；#2064 之後 degraded 在 gauge 上是 **1（warn）**，且要等資料年齡超過契約窗才自升級為 error ——
`twse_replay_sync` 未登記契約 ⇒ 窗口取預設 48h ⇒ **最多安靜 48 小時**。
本條以 `> 0` 在 1 小時內就把非 ok 變可見（不影響之後的 error 升級）。
`for: 1h` ＝ 12 個匯出週期；刻意**不用** 25h —— 記錄會被下一次每日 run 重置（間隔 24h），25h 會結構性漏掉單日事件。
該通道是 derived 通道，`DeriveChannelStatus` 不會對它產生 "stale" ⇒ 不會因「兩次 run 之間」誤報。

**可能原因**：

1. **回應日期與請求日不符** ⇒ 寫入端拒絕寫入並記 degraded（log：`refusing to write a mis-dated row`，
   `cmd/daily-replay-sync/main.go:242`）。
2. **MI_INDEX 回傳的檔都對不上 target symbol 集合**（log：`none matched the`，`main.go:280`）。
3. **非交易日被誤判／誤判為休市**（log：`not a Taiwan trading day`，`main.go:187`）。

三種在寫入端都記 degraded 且**刻意不推進 `LastSuccessAt`**（避免「沒資料卻顯示成功」）。

**立即檢查（`description` annotation 原文，逐字）**：

```text
`twse_replay_sync` 的通道狀態非 ok（gauge 值 {{ $value }}:
1=warn/stale/**degraded**、2=error、3=inactive、4=其他）。
「degraded」在寫入端有明確語意:**跑了但沒有資料落地**
（回應日期與請求日不符 ⇒ 拒絕寫入;或 MI_INDEX 回傳的檔都對不上 target 集合）,
且刻意**不推進 LastSuccessAt**（避免「沒資料卻顯示成功」的謊言）。
立即檢查:
  1. 最後一次執行的判定:
     docker logs atlas-cron-replay-sync --since 48h 2>&1 | tail -30
     關鍵字:`refusing to write a mis-dated row`、`none matched the`、
     `not a Taiwan trading day`、`appended`
  2. 記錄內容（status / last_error / last_success_at）:
     python3 -c "import json;d=json.load(open('/Users/kaecer/workspace/atlas/data/state/channel_health.json'));print(json.dumps(d['channels']['twse_replay_sync'],ensure_ascii=False,indent=1))"
  3. 補救:下一次每日 run 會自動 gap backfill（5 個日曆日的視窗）;
     若是上游 MI_INDEX 形狀變了,需要人工確認 symbol 對應。
```

> **註**：上方第 2 步的檔案路徑是 annotation 原文（筆電開發路徑）；
> 容器內對應檔案是 `/app/data/state/channel_health.json`（見 §3 第 3 步）。

**補救**：下一次每日 run 會自動 gap backfill（5 個日曆日的視窗）；
上游 MI_INDEX 形狀變了需人工確認 symbol 對應（原文第 3 步）。

**升級**：連續兩個交易日的 run 都是非 ok、或 `none matched the` 反覆出現（上游形狀變更）⇒
轉 `cmd/daily-replay-sync` 與 `internal/apigateway`（MI_INDEX 解析／symbol 對應）的擁有者；
通道治理面（known-issue／契約登記）見 [channel-health-runbook.md](channel-health-runbook.md)。

## 9. 防漂移：本檔與規則檔 `annotation` 的關係

**現況（刻意如此）**：判讀步驟同時存在兩個地方 ——
`monitoring/rules/atlas_replay_alerts.yml` 的 `description`（值班第一眼看到的那份）與本檔（§4~§8 逐字引用）。
這樣做的理由：告警 annotation 是**唯一在事件當下不需要連線就能看到的文字**（Prometheus UI／Alertmanager），
把它清空只留連結，等於把一份「離線可讀的判讀」換成一次網路跳轉。

**代價**：兩份文字未來會漂移。降風險的做法（本 PR 內已套用／未來的界線）：

1. **唯一真相寫進本檔 §9 這一段**：門檻數字（`30m`／`1h`／`14d`／`15m`／`>= 3`／`> 0`）**以規則檔為準**；
   本檔只解釋「為什麼是這個數字」，不重新定義它。規則改了，本檔必須跟著改（§10 最後一條同義）。
2. **annotation 只保留「事件當下必須看到的三步」**，長篇背景留在本檔。
   現況 annotation 已含大量設計背景（例如第 2 條的 #2145 交易日推導）⇒ **未來要縮的是 annotation，不是本檔**。
3. **機器守門只做到「連結不是死的」**：`scripts/ci/check_monitoring_single_source.py` 的 R4 會 fail-closed 檢查
   in-repo `/blob/<ref>/<path>` 目標**存在**（本檔），但**不檢查內容是否對應**。
   （另有一道會釘住 **URL 字面值**的守門：`monitoring/tests/atlas_replay_alerts_test.yml` 的 annotation 逐字比對，
   見 §10 ⇒ 改 URL 時 fixture 必須同 commit 同步。）
   ⇒ 「兩份文字的**內文**是否一致」目前**沒有自動守門**，靠 review。寫規則 PR 時請同時看本檔對應 §。
4. 反向也成立：改本檔的步驟時，若與 annotation 衝突，**先修 annotation**（那是值班實際看到的原文），再同步本檔。

## 10. 已知限制（刻意列舉）

- **假日幻影列判不出來**（§4）：Prometheus 端只有 `day_of_week()`，沒有台灣交易日曆
  ⇒ 只有「週末列」與「未來日」會 alert；**週間休市日**的幻影列需要 `internal/taiwanholidays` 才能判。
- **第 1~3 條是凍結樣本風險**（§2 第 1 點）：匯出任務停止時，值不變、規則照樣不響 ⇒ 只有第 4 條會響。
- **promtool 會比對 annotation（含 `runbook_url`）**：`monitoring/tests/atlas_replay_alerts_test.yml`
  的 expectation 是「labels + annotations 逐字」完整比對（見該檔檔頭紀律），
  ⇒ 改本家族的 `runbook_url` **必須與 fixture 同一個 commit 同步**，否則 `promtool test rules` RC=1。
  （2026-10-19 實測：只改 5 處 `runbook_url`、未同步 8 處 fixture 期望值 ⇒ RC=1；同步後 ⇒ `SUCCESS` RC=0。）
  ⇒ 這代表「URL 指到別處／被改壞」在本家族**有機器守門**；但「annotation 與本檔的**內文**是否一致」仍無機器檢查（§9 第 3 點）。
- **部署順序**：`atlas_replay_jsonl_behind_trading_days` 只有規則的第一 arm 用；規則與 binary 必須同批上，
  否則第一 arm 靜默（缺席 arm 不受影響）。
- **抓取面只涵蓋 `twse_replay_sync`**：第 5 條只看每日 sync 的通道狀態；
  hourly 的 `twse_replay` 通道不在本家族（它的 staleness 訊號屬 channel-health 家族，
  見 [channel-health-runbook.md](channel-health-runbook.md)）。
- **本檔的門檻數字以規則檔為準**：`monitoring/rules/atlas_replay_alerts.yml` 改了門檻或 `for`，
  §4~§8 的敘述就要跟著改（§9 第 1 點）。

## 11. 相關文件

- [local-deploy.md](local-deploy.md) — 部署／重啟／回滾（本家族 5 條規則**原本**錯誤指向的檔案）
- [channel-health-runbook.md](channel-health-runbook.md) — 通道層（`atlas_channel_health_status`）的完整 triage；
  第 5 條的 `twse_replay_sync` 就是這個家族的一員
- [wave9-runbook.md](wave9-runbook.md) — Wave 9 observability 操作手冊
- [docker-compose.crons.yml](docker-compose.crons.yml) — `cron-replay-sync` 的排程（`CRON_SCHEDULE=30 15 * * *`）
- [`cmd/atlas/replay_freshness_metrics_task.go`](../../cmd/atlas/replay_freshness_metrics_task.go) — 本家族的指標接線（4 個 gauge、5m 週期）
- [traps.md](../reference/traps.md) — 跨模組陷阱
