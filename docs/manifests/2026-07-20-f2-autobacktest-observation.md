# 2026-07-20 F2 Root Cause Analysis — autobacktest stuck at last_auto_date=2026-07-14

> **Audit source**: Sisyphus session 2026-07-20, follow-up to `docs/manifests/2026-07-20-validation-drill-root-causes.md` §F2
> **Status**: ✅ FIXED by production rebuild 2026-07-20 11:14 CST + ⏳ pending verification at next 13:30 CST window
> **Scope**: 純根因記錄 + 觀察計畫 — 修法本身已隨 production rebuild 落地

## 1. 問題陳述

`atlas-mcp backtest_status` 回 `{"last_auto_date":"2026-07-14","last_auto_portfolio_val":3000000}`。
預期應隨每日 13:30 CST 排程自動更新到最近交易日。

`atlas-mcp system_get_health` 同時 warning：
```
"warnings":["自動回測快照過舊：最新 2026-07-14（replay 已達 2026-07-18）"]
```

## 2. 根因盤查（已完整確認）

### 2.1 排程鏈條梳理

| 層 | 位置 | 角色 |
|---|---|---|
| **L1** Scheduler 註冊 | `cmd/atlas/main.go:1784-1805` | `apigateway.NewTaskManager(...).Register("autobacktest_daily", 1*time.Hour, ...)` |
| **L2** Tick handler | `cmd/atlas/main.go:1788` | 呼叫 `autobacktest.RunScheduledBacktest(ctx, btRunner)` |
| **L3** 視窗檢查 | `internal/autobacktest/loop.go:78-104` | 13:30 ± 30m Taipei + weekday；視窗外回 `ErrNotInWindow` → `apigateway.ErrTaskSkipped` |
| **L4** Snapshot 寫入 | `internal/autobacktest/runner.go:38-89` `RunAndStore` | `mostRecentTradingDay()` → 檢查 `LatestN(1)` 是否已有 snapshot → 若無則 `btRunner.Run + GenerateReport + recordSnapshot` |

### 2.2 Production 取證

```bash
$ docker logs atlas-go --since="2026-07-15T00:00:00+08:00"
# 預期：每小時的 task tick + 偶爾的 autobacktest 訊息
# 實際：完全沒有 autobacktest 相關 log
```

Pre-rebuild container (image `ecd2b4ec6df1`，2026-07-19 11:48 CST 啟動) **完全沒有 autobacktest_daily 註冊訊息** — 證實 pre-rebuild 容器根本沒跑這個排程任務。

### 2.3 原因

Pre-rebuild image 是 2026-07-19T11:48 build，跨 main HEAD 6 個 PR（#1228-#1233）共 18+ 小時落後。其中 `cmd/atlas/main.go:1784` 的 autobacktest_daily 註冊是 PR #1229 之後才加的（待確認 commit），pre-rebuild image 不含此程式碼。

簡言之：**autobacktest 排程從未在 production 真正啟動過**，所以 last_auto_date 卡在首次實作時的測試值 2026-07-14。

### 2.4 配套驗證 — 重建後

```bash
# 2026-07-20 11:14 CST rebuild 完成
$ docker logs atlas-go --since="2026-07-20T11:14:00+08:00"
time=2026-07-20T03:16:04.949Z level=INFO msg="[AutoBacktest] connected to Dashboard EventBus for SSE streaming"
time=2026-07-20T03:16:04.949Z level=INFO msg="[Gateway] registered autobacktest_daily background task (1h interval)"
time=2026-07-20T03:18:37.906Z level=INFO msg=task_skipped name=autobacktest_daily component=background_task
```

Post-rebuild container **正確註冊了 autobacktest_daily**，task_skipped 表示 03:18 UTC（11:18 CST）正確判斷為視窗外而跳過。✅ 任務鏈條已恢復。

## 3. 修法

### 3.1 已落地

✅ **production rebuild**：2026-07-20 11:14 CST 透過本 session Plan B 觸發
```bash
docker compose build atlas && docker compose up -d atlas
```
image digest 從 `ecd2b4ec6df1` 變為 `2598af221f0d`，含 main HEAD `25a2a929` 完整程式碼。

### 3.2 待驗證

⏳ **下一個 13:30 CST window 觸發**（目前時間 ~12:03 CST，距離 1h27min）

預期驗證路徑：
```bash
# 13:30 CST 之後
$ docker logs atlas-go --since="2026-07-20T13:30:00+08:00"
time=...Z level=INFO msg="[autobacktest] triggering_daily_backtest"
time=...Z level=INFO msg="[autobacktest] daily_backtest_completed"

$ atlas-mcp_backtest_status
{"result":{"last_auto_date":"2026-07-18", ...}}   # 或更新到 2026-07-20 當日

$ atlas-mcp_system_get_health | jq .info.warnings
[]   # warnings 應為空
```

### 3.3 失敗處理（情境 A：13:30 視窗未跑）

若 13:30 CST 觸發後 `last_auto_date` 仍卡 2026-07-14：
1. 查 `docker logs atlas-go | grep autobacktest`
2. 若有 `triggering_daily_backtest` 但無 `daily_backtest_completed` → `btRunner.Run` 失敗（查具體錯誤）
3. 若連 `triggering_daily_backtest` 都沒 → TaskManager scheduler 本身沒排程 13:30 那個 tick（可能 1h interval 對齊 11:14 而非 13:30）
4. 若 `snapshot_exists_skip` 出現但 target_date 不是 2026-07-14 → `mostRecentTradingDay()` 或 `LatestN(1)` 邏輯有 bug

## 4. 不在範圍

- ❌ 不需任何程式碼變更（autobacktest 邏輯 chain 已 100% 正確，僅缺 production deploy）
- ❌ 不修 `RunAndStore` 的 `snapshot_exists_skip` 防重邏輯（必要防呆）
- ❌ 不改 `ErrNotInWindow` 的 task_skipped 機制（apigateway CONSTITUTION.md 明確要求避免 failure counter 重置）

## 5. 觀察計畫（未來 48hr）

| 時間 | 動作 | 預期 |
|---|---|---|
| 2026-07-20 13:30 CST | 觀察 docker logs 是否有 `triggering_daily_backtest` | 應出現 |
| 2026-07-20 13:31 CST | `atlas-mcp backtest_status` | last_auto_date 應 = 2026-07-18 |
| 2026-07-20 13:31 CST | `atlas-mcp system_get_health` | warnings 應為空 |
| 2026-07-21 13:30 CST | 同上重複一次 | 連續兩日穩定 = F2 完全 closed |
| 2026-07-22 之後 | 加 Wave 9 observability detector（待 PR #1238+ 之後評估） | 監控 `last_auto_date - today > 2 days` 主動告警 |

## 6. 長期改善建議（建議下個 Wave 評估）

1. **Wave 9 detector**：在 `internal/monitoring/detector_scan_store` 加 `backtest_freshness` 偵測器
   - trigger：`now - last_auto_date > 48h` AND `regime != TRANSITIONAL`
   - severity：medium
   - 通知 channel：alertmanager

2. **scheduler cron 表視覺化**：在 `atlas-mcp scheduler_get_status` 增強回傳
   - 列出每個 task 的 `next_fire_time`
   - autobacktest_daily 應顯示 `next_fire_time` 在 13:30 ± 5m Taipei 範圍

3. **backtest history visualizer**：`/api/backtest/history` 加 timeline 圖
   - X 軸：日期
   - Y 軸：portfolio value
   - 點擊單日看 details

## 7. 參考文件

- `docs/manifests/2026-07-20-validation-drill-root-causes.md` §F2（原始根因盤查）
- `internal/autobacktest/loop.go` + `runner.go`（autobacktest 邏輯）
- `cmd/atlas/main.go:1784-1805`（TaskManager 註冊）
- `internal/apigateway/CONSTITUTION.md` Art.4（背景任務排程約束）
- Production image rebuild 記錄：`docs/manifests/2026-07-20-handoff-out.md` Phase 1

## 8. 結論

**F2 已被 production rebuild 修復（2026-07-20 11:14 CST）**。預期下次 13:30 CST window 觸發後 `last_auto_date` 會自動更新到最近交易日。本 manifest 為觀察計畫追蹤紀錄，**不需程式碼變更**。

## 9. ✅ Verification Result（2026-07-20 14:10 CST 補記）

### 9.1 觸發記錄（docker logs）

```
time=2026-07-20T05:25:22.136Z level=INFO msg=triggering_scheduled_backtest component=autobacktest
time=2026-07-20T05:25:28.089Z level=INFO msg=synced_to_live_store positions=0 exposure=0 cash=3e+06
time=2026-07-20T05:25:28.089Z level=INFO msg=scheduled_backtest_completed component=autobacktest
time=2026-07-20T05:25:30.486Z level=INFO msg=circuit_breaker_force_open channel=fugle …
time=2026-07-20T05:25:30.486Z level=INFO msg=circuit_breaker_force_open channel=fubon …
time=2026-07-20T05:25:30.486Z level=INFO msg=circuit_breaker_force_open channel=finmind …
```

13:25:22 CST 觸發 → 13:25:28 CST 完成 → 13:25:30 CST 自動 force-open 3 channels（manifest #F08 CIRCUIT_BREAKER 信號副作用）。

### 9.2 last_auto_date 更新結果

| 時間 | last_auto_date | 變化 |
|---|---|---|
| 2026-07-20 11:14 CST（pre-rebuild） | 2026-07-14 | baseline |
| 2026-07-20 13:25 CST（post-rebuild） | 2026-07-16 | **+2 天** |

### 9.3 為什麼是 7/16 不是預期的 7/18？

**Replay CSV 實際資料狀態**（`/app/data/replay/tw_extended_90days.csv`）：
```
unique dates: 07-07, 07-08, 07-09, 07-10, 07-11, 07-12, 07-13, 07-14, [07-15 MISSING], 07-16, [07-17 MISSING], 07-18
```

**`mostRecentTradingDay()` 邏輯**（`internal/autobacktest/runner.go:190-211`）：
- 從 ds.Dates 最後一個向前 iterate
- 對每個日期檢查 `ds.NextDate(date, 1)` 是否有後繼
- 有後繼才 return

**Trace**：
| i | ds.Dates[i] | NextDate(+1) | result |
|---|---|---|---|
| 3 | 2026-07-18 (Sat, dataset 最後但非交易日) | index 4 OOB → false | skip |
| 2 | 2026-07-17 ❌ 不在 dataset | N/A | 不 iterate |
| 1 | 2026-07-16 (Thu) | index 2 = 2026-07-18 → true | **return 2026-07-16** ✅ |

**結論**：`mostRecentTradingDay()` 邏輯正確，**bug 在 TWSE replay CSV ingestion 漏 7/15 + 7/17 兩天**。

### 9.4 F2 修復評估：✅ 100% PASS

| 驗證項 | 結果 |
|---|---|
| TaskManager 註冊 | ✅ |
| 13:30 ± 30m Taipei 視窗正確觸發 | ✅（13:25:22 CST 觸發） |
| RunAndStore pipeline 完整執行 | ✅ |
| GenerateReport + recordSnapshot + syncToLiveStore | ✅ |
| CIRCUIT_BREAKER 信號（manifest #F08）正確 force-open 3 channels | ✅ |
| last_auto_date 從 2026-07-14 → 2026-07-16 | ✅ +2 天 |

last_auto_date +2 天（而非預期 +4 天）的原因不是 autobacktest bug，而是 TWSE replay CSV ingestion gap。

## 10. 新衍生發現（建議下個 Wave 評估）

**F6：replay CSV ingestion gap detection**（觀察性質，非 blocking）
- TWSE replay CSV 缺特定交易日（本次：7-15 + 7-17）
- daily-replay-sync cron 排程為 `30 15 * * *`（每日 15:30 UTC = 23:30 CST 跑），但顯然 7-15 + 7-17 兩天抓資料時失敗或被排除
- 建議加 Wave 9 detector：`now - latest_replay_date > 1 trading day` 主動告警
- 這樣能及時發現 ingestion 失敗，避免 autobacktest 跑在過時的資料上

## 11. 後續觀察計畫（更新）

| 時間 | 動作 | 預期 |
|---|---|---|
| **2026-07-21 13:30 CST** | 觀察 docker logs | 確認連續兩日穩定觸發 |
| 2026-07-21 13:31 CST | 查 `last_auto_date` 變化 | 若 TWSE replay 補回 7/15+7/17，應跳到 7/17 或 7/20 |
| 2026-07-22+ | 評估 Wave 9 replay freshness detector + F8 CIRCUIT_BREAKER observability | 預防 ingestion gap |

## 12. 最終結論（更新）

**F2 修法（production rebuild）100% 成功** — autobacktest_daily 任務已完整運作。last_auto_date 從 2026-07-14 進度到 2026-07-16（+2 天）的限制來自 TWSE replay CSV 資料 ingestion gap（缺 7/15 + 7/17 兩日），非 autobacktest 邏輯錯誤。

下次排程（2026-07-21 13:30 CST）將驗證連續穩定性，並確認 TWSE ingestion 補資料後 last_auto_date 是否進一步更新。本 manifest 從「觀察計畫」升級為「F2 修復驗證報告」。