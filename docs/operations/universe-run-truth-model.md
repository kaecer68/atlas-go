# 母體(universe)執行的真值模型與判層

> **這份文件回答三個問題**：驗收母體時**該信哪一個訊號**、每個訊號**可能怎麼說謊**、
> 以及**壞了要怎麼定位到哪一層**。
>
> 對應工具：[`scripts/ops/verify-universe-run.sh`](../../scripts/ops/verify-universe-run.sh)
> （唯讀、可重跑、**會判層**；實作在 `scripts/ops/verify-universe-run.py`）。
> 對應告警：`monitoring/rules/atlas_universe_scoring_alerts.yml`（11 條；第 10/11 條於 2026-09-27 新增，
> 分別守兩個產物：`universe_snapshot.json` 與 `universe.json`）。
>
> **這份文件不是 runbook 的替代品**：`docs/operations/universe-scoring-ranked-zero-20260925.md`
> 是各條告警的 triage 步驟；本文件是**訊號之間的關係與可信度**（誰是主真值、衝突時信誰）。

---

## 0. 一句話結論（先看這個）

**主真值是「輸出」，不是「記帳」。** 判定的順序永遠是：

```
產物檔（data/state/universe_snapshot.json）
  → 輸出面 metric（atlas_universe_last_run_* / next_run）
    → 容器日誌（事件）→ counter（必須先驗標籤形狀）→ 告警狀態
```

counter 與告警狀態**不能**當真值：counter 可能是 legacy 錯標籤系列的殘留（[§3.3](#33-counter-記帳面a_universe_total)），
告警狀態只反映「規則有沒有載入、有沒有被評估」，不反映母體本身（[§3.5](#35-告警狀態-apiv1alerts)）。

---

## 1. 真值階層（衝突時信誰）

由強到弱。**上一層與下一層衝突時，永遠信上一層**，並把衝突本身當成缺陷訊號記錄下來。

| # | 真值來源 | 為什麼是這個位階 | 代表性失敗模式 | 衝突時怎麼辦 |
|---|----------|------------------|----------------|--------------|
| 1 | **產物檔案** `data/state/universe_snapshot.json`（`result.*` ＋ `ranked[]`；`mtime` 是獨立證據） | 唯一被下游（D6 watchlist、coverage check、人工驗收）實際讀取的東西；它同時是「產出」與「證據」 | 檔案沒更新（寫入失敗 / WorkDir 指錯 / 磁碟滿）⇒ 讀者看到**無聲過期**的母體 | 檔案舊 ⇒ 不管 verdict 多健康，都以檔案為準（`L5` 紅） |
| 2 | **輸出面 metric** `atlas_universe_last_run_*`、`atlas_universe_next_run_timestamp_seconds` | 由 `BuildUniverse` 的**單一 defer** 發佈、走獨立 sink（不共用 counter 的 wiring）⇒ 每條退出路徑都會留下痕跡 | **記憶體裡的宣稱**：它描述「算出了什麼」，不描述「寫進了什麼」；`valid == 0` 時 `finished` 是**啟動哨兵**而非執行 | `verdict` 說跑了但檔案沒動 ⇒ 兩者都記下來，`L1` 綠、`L5` 紅（這正是第 10 條的形狀） |
| 3 | **容器日誌事件**（`daily_refresh_start` / `*_ok` / `*_skip_*` / `snapshot_save_error`） | 唯一帶**人類可讀細節**（原因、參數）的來源；可跨 process 邊界（在被 `docker logs` 的保存範圍涵蓋時） | (a) 會被容器重啟截斷（`--since 72h` 實得 5h）；(b) `daily_skip_*` 自 2026-09-27 起是 `logging.Info`（原為 `Debug`，INFO 等級下看不見），且只在對齊視窗內輸出（≤3 行/日） | 只用來**佐證**或**解釋**，不用來單獨判定「沒跑」 |
| 4 | **counter** `atlas_universe_*_total` | 唯一有歷史視窗（`increase()`）的來源 | (a) legacy 標籤形狀（`{daily="failed"}`）會給出**假讀數**；(b) 重啟後歸零；(c) `snapshot_persisted_total` 在寫入失敗時**仍然 +1** | 先驗標籤形狀；形狀不對 ⇒ **整個記帳面不可用**（不是「數字偏大」而已） |
| 5 | **告警狀態** `/api/v1/alerts` | 回答「值班的人現在看到什麼」 | 規則沒載入 / interval / `for:` pending ⇒ 一切正常也沉默 | 只能當「規則面的健康度」，不能當母體真值 |

### 裁決規則（三條，實作在工具的 `overall_exit`）

1. **產物優先**：`L5`（產物面）判定為紅時，「輸出健康」的說法一律不被接受（`L2/L3/L4` 就算綠也不能覆蓋它）。
2. **輸出優先於記帳**：`L3`（評分面）與 `L4`（發射面）分歧時，以 `L3` 為準；`L4` 紅代表「指標發射壞了」，**不是**「母體壞了」。
3. **不確定不當結論**：證據不足（端點讀不到、日曆不可判定、artifact schema 太舊、重啟銷毀證據）⇒ `UNKNOWN/PENDING`，
   既不是綠也不是紅（exit code 2）。把「我不知道」報成「壞了」會浪費值班；報成「綠」會掩蓋缺口。

---

## 2. 判層（哪一層壞了）

| 層 | 名稱 | 判定式（可稽核） | 對應告警 |
|----|------|------------------|----------|
| `L0` | 傳輸面 transport | `/metrics` 可讀 **AND** `atlas_universe_last_run_valid` 存在 **AND** Prometheus 已載入規則群 `atlas_universe_scoring`（CORE 現行 9 條；第 10/11 條逐條比對 `/api/v1/rules` 的載入集合 ⇒ 已載入即視為已部署、未載入才 WARN） | 第 4 / 6 條 |
| `L1` | 排程面 schedule | 心跳 `next_run + grace < now` ⇒ **沒跑**；否則「`<= now` 的最大台灣交易日 `06:00Z`」之後有任何 stage `finished >= 該時刻`（或日誌 `*_start/*_ok`）⇒ **有跑**；兩者皆非、已過寬限、**且缺席可被否證** ⇒ **沒跑**；否則不可判定 | 第 8 條 |
| `L2` | 資料面 input | `snapshot.result.quotes_status == ok` **AND** `quotes_missing_fetch_error + quotes_missing_not_attempted == 0` | 第 3 條 |
| `L3` | 評分面 scoring | `symbols_built > 0` **AND** `symbols_filtered > 0` **AND** `ranked_trustworthy != false` **AND** `symbols_ranked > 0`（缺一即紅） | 第 1 / 2 / 5 / 7 條 |
| `L4` | 發射面 emission | ① counter 的每個系列都帶 `stage` label（不得出現 `{daily=...}`）**AND** ② **NOT**（輸出說 `ranked > 0` 且 `<6d` 內完成 **且** `increase(...[6d]) == 0`） | 第 9 條 |
| `L5` | 產物面 artifact | snapshot 存在 **AND** `mtime` 不比「宣稱完成時刻」舊 2 分鐘以上 **AND**（若已過寬限）不比最後一次應執行時刻舊 2 分鐘以上 **AND** `last_run_snapshot_persisted != 0`（`last_run_registry_persisted` 只**印出**，不改變本層判層：這一層的主詞是 `snapshot` 這一個檔；把第二個產物併進來會讓「哪一個檔案壞了」從判層讀不出來 —— 見 §7 第 8 條） | 第 10 條 ＋ 第 11 條（第 11 條守第二個產物 `universe.json`；2026-09-27 新增） |

### 判定空間是完整的、互斥的

以 `(valid, gathered, filtered, trustworthy, ranked, snapshot_persisted)` 為座標：

```
valid == 0（這個 process 還沒跑過）           ⇒ 第 8 條（沒跑）/ 工具 L1 負責
gathered == 0                                ⇒ 第 7 條 EmptyUniverse
gathered > 0, filtered == 0                  ⇒ 第 5 條 EmptyFiltered
filtered > 0, trustworthy == 0               ⇒ 第 3 條 QuotesMissing
trustworthy == 1, ranked == 0                ⇒ 第 1/2 條 RankedZero
trustworthy == 1, ranked > 0, snapshot_persisted == 0 ⇒ **第 10 條 SnapshotNotPersisted**
trustworthy == 1, ranked > 0, snapshot_persisted == 1, registry_persisted == 0
                                              ⇒ **第 11 條 RegistryNotPersisted**
trustworthy == 1, ranked > 0, 兩個產物都落地    ⇒ 健康（除非記帳面斷線 ⇒ 第 9 條）
```

> ⚠️ 第 10 條刻意要求 `trustworthy == 1` **且** `ranked > 0`：早期退出（母體空 / 篩選全滅 / 報價不可用）必然
> `trustworthy == 0`，所以那些路徑各自由第 7 / 5 / 3 條負責，**同一個根因不會有兩條規則 firing**。
> 唯一允許同時 firing 的是第 9 條（記帳面）——那是**不同層**的失敗。

### exit code（工具）

| code | 意義 |
|------|------|
| `0` | 全綠（可含 `WARN`：`WARN` = 「這個維度還沒上線」，不是「母體壞了」） |
| `1` | 至少一層成立「應告警」條件（與告警規則同一組判定式） |
| `2` | **無法判定**（證據不足） |
| `3` | 用法 / 設定錯誤 |

---

## 3. 逐訊號盤查（權威嗎／怎麼說謊／答不出什麼）

### 3.1 產物 `data/state/universe_snapshot.json` 的 `result.*`（**主真值**）

- **權威？** 是。它是下游唯一讀的東西，且與判層座標一一對應（`symbols_*`、`ranked_trustworthy`、`quotes_*`）。
- **可能怎麼說謊？**
  - **stale**：內容正確但**不是這一輪的**（`result.timestamp` 是寫檔當時的那一輪；`mtime` 才回答「什麼時候寫的」）。
  - **schema 落後**：2026-09-17 的 artifact 沒有 `quotes_status` / `ranked_trustworthy`
    ⇒ `symbols_ranked == 0` 分不出「市場判定」與「量測失敗」。工具對此回 `UNKNOWN`（`L2`）而不是綠。
  - `ranked[]` 與 `symbols_ranked` 不一致（同一個 artifact 裡的清單與它自己的計數互相矛盾）：
    2026-09-27 之前沒有任何規則或測試比對這一對。現在寫入端 `SaveUniverseSnapshot` 有不致命的守門
    （`os.Stat` 之外的另一條讀回：`RankedCountConflict` ⇒ WARN `universe_ranked_count_mismatch`，
    **仍然照寫**，因為檔案是下游唯一有的東西），並由 `TestRankedCountConflict` /
    `TestSaveUniverseSnapshot_MismatchIsReportedAndStillWritten` 釘住。工具本身仍只並列印出兩個讀數。
- **答不出什麼？** 「排程有沒有跑」（那是 `L1`）、「指標有沒有發射」（`L4`）。

### 3.2 輸出族 `atlas_universe_last_run_*` 與 `atlas_universe_next_run_timestamp_seconds`

- **權威？** 是（第二順位）。由 `BuildUniverse` 的**單一 defer** 發佈（每一條退出路徑都經過），走 `GaugeSink`，不共用 counter 的 wiring。
- **可能怎麼說謊？**
  - **暖機哨兵**：`last_run_valid{stage} == 0` 時，`last_run_finished_timestamp_seconds` 是 **process 啟動時刻**，不是任何一次執行
    （2026-09-27 生產實測：`finished = 1790479521` ≈ 容器 `StartedAt + 54s`）。**必須成對讀 `valid`**。
  - **記憶體宣稱**：它描述「算出了什麼」，**不**描述「寫進了什麼」⇒ 這是第 10 條存在的原因。
  - verdict 不會自己過期：週一跑完的 weekly verdict 會留到下一輪（刻意的）。
- **答不出什麼？** 「產物有沒有落地」（`L5`/第 10 條）、「上一次執行是不是來自排程」（可能是 CLI / 人工）。

### 3.3 counter（記帳面）`atlas_universe_*_total`

- **權威？** 只在**標籤形狀正確**時可用，且只能用 `increase()` 判零/非零（累積 counter 沒有上界，**不得**比絕對值）。
- **可能怎麼說謊？（2026-09-25 生產實例）**
  - legacy 形狀 `atlas_universe_symbols_screened_total{daily="failed"} 4906`：label 的**值**被當成 label 名，
    正確形狀 `{stage="daily"|"weekly"}` 的系列**全是 0**。⇒ 任何 `increase()` 讀數都是假的。
  - 重啟後歸零（`WarmUp` 之後全 0）。
  - `atlas_universe_snapshot_persisted_total` 在**寫入失敗時仍然 +1**（`Increment` 不在 `err == nil` 分支內）
    ⇒ 它**不能**回答「產物有沒有落地」。
- **答不出什麼？** 「產出什麼」（只看得到有沒有記帳）。

### 3.4 容器日誌（`universe_scheduler` 事件）

- **權威？** 佐證與解釋用。
- **可能怎麼說謊？**
  - `docker logs --since 72h` **會被容器啟動截斷**（2026-09-27 實測：要求 72h、實得 5h21m）
    ⇒ 「沒有 `daily_refresh_start`」**只有在日誌真的涵蓋那一刻時**才算否證。
  - `daily_skip_non_trading` / `daily_skip_monday` 自 2026-09-27 起是 `logging.Info`（`weekly_skip_holiday`
    本來就是 INFO）⇒ 休市／週一的 skip 有日誌證據。`weekly_skip_not_monday` 仍是 `Debug`（非週一的每一天都會碰到，
    升成 Info 會變成每日噪音）。
  - skip 日誌的**量**由閘門順序決定：兩個 daily 閘門現在跑在 `alignToTarget` 之後（與
    `NewWeeklyUniverseRebuildTask` 一致）⇒ 一天 ≤3 行；改回「閘門在前」會變成每分鐘一行（實測 1440 行/日，
    `TestDailySkip_InfoLevelAndDailyVolume` 會在超過 3 行時失敗）。
- **答不出什麼？** 「跑了但產物沒落地」除非 `snapshot_save_error` 剛好有印（2026-09-27 新增 `snapshot_not_persisted`）。

### 3.5 告警狀態 `/api/v1/alerts`

- **權威？** 否。它回答「規則有沒有被評估、有沒有 firing」，不回答母體是否健康。
- **可能怎麼說謊？** 規則檔沒載入（2026-08-27 事故：寫進錯的設定樹 ⇒ 27 天沒被評估）、`for:` 期間只是 pending、
  `absent()` 型規則在服務不健康時沉默（那是 container liveness 的轄區）。
- **答不出什麼？** 任何「規則沒覆蓋的形狀」——本輪的第 10 條就是為了補一個**完全沒有偵測器**的形狀。

### 3.6 其他納入盤查的訊號

| 訊號 | 權威性 | 備註 |
|------|--------|------|
| `data/state/universe.json`（agents.json 相容的 registry） | 次要 | 與 snapshot 同一步寫入；**2026-09-27 起有告警**（第 11 條 `AtlasUniverseRegistryNotPersisted`，與第 10 條互斥）。mtime 比 snapshot 舊 ⇒ 工具的 `L5` 仍會附帶 `⚠️ 次要矛盾`（作為第二個見證） |
| `data/state/universe_watchlist.json`（D6） | 次要 | 有 in-process 告警（`monitor.Alert`，`len > 20`），不是 Prometheus 規則 |
| `universe_coverage_check`（`cmd/atlas/main.go`） | 次要 | 讀 snapshot 的 `symbols_built`；2026-09-27 起有**兩個** finding：覆蓋率 `< 90%`（原有）與**產物過期**（`monitoring.AssessUniverseCoverage`，與最後一次應執行時刻比、含寬限與容差；訊息含 artifact 幾小時舊與該次應執行時刻）。仍**答不出**「artifact 不存在」（那是第 10 條與工具 L5 的轄區） |
| 台灣交易日曆 `internal/taiwanholidays` | 前提 | 排程判定式的另一半。工具**解析 Go 原始碼**的字面值（不硬編第二份表）；解析不到 ⇒ `UNKNOWN` |

---

## 4. 2026-09-27 找到的缺口與修法

### 缺口：「跑了、有產出，但產物沒有落地」

在 2026-09-27 之前，10 條規則裡的 9 條覆蓋不到這個形狀：

- 第 8 條（`RunOverdue`）只覆蓋「**沒跑**」；
- 第 1/2 條（`RankedZero`）覆蓋「跑了但 `ranked == 0`」；
- 第 9 條（`CounterEmissionMissing`）覆蓋「跑了、產出健康，但 counter 沒發射」；
- **沒有任何一條**覆蓋「跑了、verdict 健康、`next_run` 心跳照樣更新，但 `universe_snapshot.json` 停在上一輪」。

後果：讀 snapshot 的消費者（D6 watchlist 的 `loadPreviousRankedSymbols`、`universe_coverage_check`、人工驗收）
會拿到一份**無聲過期**的母體——它不會壞、不會報錯，只會繼續用上一次的排名字單。

### 修法（bounded）

1. **Go 端新增輸出訊號** `atlas_universe_last_run_snapshot_persisted{stage}`（`internal/monitoring/metrics/universe_run.go`）：
   由 `SnapshotPersistedOnDisk`（`internal/monitoring/universe_scheduler.go`）在寫入嘗試**之後讀回檔案**
   （`os.Stat` 比對 `mtime` 與本輪開始時刻）決定 ⇒ 它問的是「產物在不在、是不是這一輪的」，
   **不是**「write 有沒有回 nil」。暖機值是 `0`（不是 `1`）。
2. **規則端新增第 10 條** `AtlasUniverseSnapshotNotPersisted`，並把這個輸入族加進第 6 條的 `absent(...)` 清單
   （否則「把發佈路徑刪掉」會讓第 10 條變成零偵測）。
3. **測試**：Go 端（`universe_snapshot_persistence_test.go`，含「舊檔不算這一輪」的負向對照）、
   promtool case `T`（firing）/ `U`（`ranked == 0` ⇒ 不得 firing）/ `V`、`V2`（兩個產物訊號**各自**
   缺席 ⇒ 第 6 條 firing）、`W`（只有 registry 沒落地 ⇒ 第 11 條 firing），
   以及 `scripts/ci/promtool-mutation-negative-proof.sh`（四個外科式突變必須被咬住，仍只呼叫 promtool 三次）。
4. **本文件 ＋ 工具**：`scripts/ops/verify-universe-run.sh`（唯讀、判層、exit code）與
   `tests/scripts/test-verify-universe-run.sh`（20 個 fixture case ＋ 8 個判定式突變；2026-09-27 的
   「可觀測性誠實化」批次擴到 22 個 case ＋ 10 個突變 ＋ 7 個 note/evidence 斷言；2026-09-29 的
   #2019 批次再加 2 個 case（`excluded-reasons-present` / `excluded-reasons-absent`）＋ 6 個
   報告/JSON 附加欄位斷言，並把那兩個 case 的**期望判層向量與 `green` 寫成完全一樣** ——
   這是「細分是證據、不是判層」的機械化證明）。

### ⚠️ 部署時序（讀 PR body 的那一段）

**本輪的規則與 binary 一起部署，且排在 2026-09-29 06:00Z 的母體驗收之後**（不在驗收前改變告警行為）。
在「規則已上、binary 還沒上」的窗口內：`atlas_universe_last_run_snapshot_persisted` 不存在
⇒ 第 10 條沉默（`sum by (stage)` 取不到東西），而那個缺席由**第 6 條**回報（正確的訊號）。
診斷工具 `verify-universe-run.sh` **不依賴**這個新 metric：它的 `L5` 直接讀檔案的 `mtime`，
所以在部署前後講的是同一句話。

### 4.2 2026-09-27「可觀測性誠實化」批次（B/C/D/F/G，同一個 PR）

§4.1 補的是「跑了、有產出，但產物沒落地」。同一輪盤查另外找到五個**會說謊或沉默**的地方，
它們一起上（同一個 PR、同一批部署），每一項都附正反案例：

| 項 | 缺口（症狀） | 落地 |
|----|-------------|------|
| **B** | registry（`universe.json`）寫入失敗只有一行 WARN ⇒ 「registry 舊、snapshot 新」兩種消費者看不同母體，**零偵測** | 新訊號 `atlas_universe_last_run_registry_persisted{stage}` ＋ 第 11 條（與第 10 條互斥：它要求 `snapshot_persisted == 1`）；promtool `W`（fire）/ `A`（兩者都落地 ⇒ 沉默）/ `T`（兩者都失敗 ⇒ 只有第 10 條） |
| **C** | `universe_coverage_check` **覆蓋率半邊恆不成立、且完全不看年齡** ⇒ 39.7h 舊的 artifact 零告警（09-27 實況）。真因是**分母口徑**：分子 `symbols_built=1599` vs 分母 `TotalClassifiedSymbols()=27`（分類樹 12 段中 11 段的代表股）⇒ ≈5922%，`< 90` 不可滿足（**分母修正屬 #1944 I29，未在本批改**；舊敘述「99.9% 覆蓋」是測試 fixture 的 `TotalSymbols=1600`，非生產讀數） | `monitoring.AssessUniverseCoverage` ＋ `PreviousUniverseRun`（以交易日曆算最後一次應執行時刻，**不是**固定視窗）⇒ 過期時告警，訊息含 artifact 幾小時舊與該次應執行時刻 |
| **D** | `ranked[]` 的長度與 `symbols_ranked` 無人比對 | 寫入端 `RankedCountConflict` 守門（WARN、**不致命、不修改資料**）＋ Go 測試（含 mutation 自證） |
| **F** | `daily_skip_non_trading` / `daily_skip_monday` 是 `Debug` ⇒ INFO 的生產**看不到休市證據** | 升成 `Info`；同時把兩個 daily 閘門移到 `alignToTarget` **之後**（與 weekly 一致）⇒ 上限由「每分鐘一行」降為 ≤3 行/日（**實測**：舊順序 + Info = 1440 行/日，見 `TestDailySkip_InfoLevelAndDailyVolume`） |
| **G** | 檔頭要求「部署後把規則從 `NEW_RULES` 搬進 `CORE_RULES`」= 人工步驟，漏掉會誤導（搬了反而在部署前變 RED 假警報） | 工具**逐條比對** `/api/v1/rules` 的載入集合（`NEW_RULES` 也比）：已載入 ⇒ 靜默；未載入 ⇒ WARN 且保留「這條是新增的」語意；補上「部分載入」fixture 與兩個方向的 mutation |

> ⚠️ (C) 的新告警會在驗收前對**已知的過期 artifact** 亮紅 ⇒ 本批的部署**必須**排在
> 2026-09-29 06:00Z 的母體驗收之後（與 §4.1 的部署時序同一個窗口）。

---

## 5. 實例：2026-09-27T08:46Z 的生產讀數（工具的真實對照）

| 訊號 | 讀數 |
|------|------|
| 容器 | `atlas-go`（`atlas-atlas`），healthy，`18080` |
| 本輪 process 起點 | 2026-09-27T03:24:27Z（`last_run_valid{daily}=0`、`{weekly}=0`；`finished=1790479521` = 啟動哨兵） |
| 心跳 `next_run` | **2026-09-29T06:00:00Z**（+45h13m）⇒ `next_run + 2h < now` 為 false ⇒ `RunOverdue` 沉默（正確） |
| 產物 | `universe_snapshot.json`：`symbols_built=1599`、`symbols_ranked=150`、`quotes_status=ok`、`ranked_trustworthy=true`、`quotes_returned=1581`；`ranked` 陣列 150 檔；`mtime=2026-09-25T17:01:40Z`（**39h44m 前**） |
| 規則 | 規則群 `atlas_universe_scoring` 9 條全部載入、全部 `inactive`（**當時**；第 10/11 條是同日稍後才進 repo） |
| 告警 | `AtlasUniverse*` = **0 條**（唯一 active 的是 `CalibrationArtifactStale`，與母體無關） |
| 日誌 | 72h 內 `universe_scheduler` 事件 **0 行**（實際只涵蓋 5h21m） |

（上表是**用真實生產讀數**餵給工具驗證過的：`/metrics` 的 65 行、`/api/v1/rules` 的 9 條規則、
`universe_snapshot.json` 的 `result` 全部取自當次實查輸出。同一組讀數也順手抓到工具的一個**假陽性**
（標籤形狀檢查對合法的 `industry` / `error_type` label 誤報）—— 見 [§7.7](#7-已知限制誠實列出)。）

**工具怎麼判（誠實版）**：

- `L2/L3` 綠（artifact 的健康讀數是真的），`L4` 綠（verdict 全是暖機值 ⇒ 沒有「輸出說有產出」的矛盾），
- `L1` **UNKNOWN**：最後一次應執行時刻是 2026-09-24T06:00Z（09-25 中秋、09-28 教師節休市、中間夾週末），
  但 process 直到 09-27T03:24Z 才啟動、日誌也只涵蓋 5h ⇒ **那一輪是否跑過已不可查**（重啟銷毀證據）。
  這是刻意的：把「我不知道」報成「沒跑」會浪費一次值班。
- `L0` **WARN**：9 條現行規則都在；第 10/11 條尚未部署（部署排在驗收之後）。
  自 2026-09-27 起工具**逐條**比對 `/api/v1/rules` 的載入集合（`NEW_RULES` 也一起比）⇒ 部署完成後
  這個 WARN 會自己消失，**不需要**人工把規則從 `NEW_RULES` 搬進 `CORE_RULES`（搬了反而會在部署前
  變成 RED 的假警報）。
- 整體 exit = **2（無法判定）**，不是 0 也不是 1。

> **這就是「真值模型」的價值**：同一組讀數在舊的四道防線下是「全綠」（沒有任何告警 firing），
> 但真正的答案既不是「健康」也不是「壞了」，而是**「證據不足以回答那個問題」**——而那個答案
> 本身告訴你下一步該做什麼（看 09-29 的執行，而不是去查 pipeline）。

---

## 6. 怎麼跑（一行指令）

```bash
# 生產主機（Mac Mini）的 repo checkout 內，唯讀：
scripts/ops/verify-universe-run.sh            # 完整報告（逐層判定式 + 證據）
scripts/ops/verify-universe-run.sh --quiet    # 只看結論
scripts/ops/verify-universe-run.sh --json     # 機器可讀（exit code 同上）
echo $?                                       # 0 全綠 / 1 有層成立應告警條件 / 2 無法判定 / 3 用法錯誤
```

常用開關：

| 開關 | 用途 |
|------|------|
| `--expect-run none` | 已知休市 / 不預期有執行（覆寫日曆） |
| `--grace-seconds N` | 「已預告要跑但還沒跑完」的寬限（預設 7200；驗收當下剛過觸發時刻時會得到 `PENDING`） |
| `--offline-dir DIR` ＋ `--now EPOCH` | 用 fixture 離線重跑（`tests/scripts/test-verify-universe-run.sh` 就是這樣跑 24 個判層分支） |
| `--logs-file FILE` / `--no-logs` | 不呼叫 docker（唯讀環境或容器在別台時） |

### 附錄（證據，不參與判層）：排除原因細分

2026-09-29（issue #2019）起，完整報告多一節 `── 排除原因細分 ──`：把
`snapshot.result.symbols_excluded_reasons` 依階段（風控 Step 5／選股 Step 4／concentration cap）
印出來，並把兩條恆等式並排核對：

```
symbols_filtered = symbols_ranked + screener_total + concentration_cap
symbols_excluded = risk_total
```

* 這一節**不改任何 verdict、不改 exit code**（0/1/2/3 的語意完全不變）；缺項時印「無法核對」，不假設成立。
* 欄位不存在時只印 **`未提供（本次 run 未帶細分）`**，**不補 0** —— 0 是有效讀數（該階段跑過且未排除
  任何檔），「沒帶細分」是另一個世界（與 channel-health gauge 的 absent-vs-present-zero 同一條原則）。
* `--json` 同步附加 `excluded_reasons`（物件或 `null`）與 `excluded_reasons_present`（bool）；
  既有欄位一字不動（工具鏈相容）。
* 細分的權威定義在監控端：`internal/monitoring/universe_exclusion_reasons.go`。

**它不取代告警**：它不寫任何檔案、不改任何狀態，只是把「判層」變成可重跑的一件事。
告警是**持續性**的守門；本工具回答的是「現在這一刻、這一輪到底在哪一層」這種**一次性驗收**問題。

---

## 7. 已知限制（誠實列出）

1. **交易日曆是解析 Go 原始碼**（`internal/taiwanholidays/taiwan_holidays.go` 的 `time.Date` 字面值 ＋ `fixedHolidays`）。
   解析不到（或年份不在表內）⇒ `L1` 一律 `UNKNOWN`，**絕不猜**「那個交易日不存在」。
2. **重啟會銷毀證據**：觸發時刻之後才重啟時，「那一輪跑過沒有」不可查 ⇒ `L1 = UNKNOWN`。
   這是工具**刻意**的判定（不是紅燈），但同時也指出現行規則的結構性盲點：
   心跳在重啟時會被重新發佈到下一輪 ⇒ `AtlasUniverseRunOverdue` 不會 firing（工具會把這句印出來）。
3. **skip 日誌自 2026-09-27 起是 INFO**（`daily_skip_non_trading` / `daily_skip_monday`；改動前是 `Debug`，
   INFO 的生產看不到），且只在對齊視窗內輸出（≤3 行/日）。但 `L1` **仍不**把「日誌沒有 `*_start`」
   當成唯一證據：日誌會被容器重啟截斷（見第 2 條），所以「日誌缺席」有兩個成因，只有日曆能區分。
4. **`L4` 的增量需要 Prometheus 的 query API**；讀不到時該層 `UNKNOWN`（不是綠）。
5. **`universe.json`（registry）自 2026-09-27 起有告警**（第 11 條，與第 10 條互斥）；工具另外會把
   「registry 的 mtime 比 snapshot 舊」印成 `⚠️ 次要矛盾`，那份對照需要兩個檔案都在本機可見。
6. **診斷工具本身的覆蓋範圍**：`scripts/ops/verify-universe-run.py` 的判定式有 10 個突變測試釘住
   （`tests/scripts/test-verify-universe-run.sh`）；**新增判定式時必須同時加一個突變**，
   否則它會變成一份「永遠說沒問題」的檢查。「顯示契約」（note / evidence）另有 7 個斷言，
   但它們沒有對應的 mutation harness —— 這是一個**已知的**不對稱。
7. **`L5` 只判 `snapshot` 一個檔**：`atlas_universe_last_run_registry_persisted == 0` 會被 `L5` 印出
   （逐 stage），但**不改變判層**；registry 落地的判定屬於第 11 條規則，以及「`universe.json` 的 mtime
   與 `snapshot` 的 mtime 對照」那次附帶的 `⚠️ 次要矛盾`。要讓工具一起判，得先決定「同一次執行的兩個產物
   要不要共用一個判層」—— 在那之前不改，避免把兩個檔案混成一句「產物面 OK/紅」。
8. **標籤形狀檢查的精度**：它只盯兩個指紋 ——「規則讀的 counter 有沒有 `stage`」與
   「有沒有出現 `{daily=...}` 這種值當名字的 series」。刻意**不**做「label 名不在白名單裡就報警」：
   那會在 `{industry="all",stage="daily"}`（coverage 稽核）與 `{error_type="scrape_error",...}`
   （narrative 分類）上誤報。這個誤判是 2026-09-27 用**生產的真實 `/metrics`** 跑工具才發現的，
   regression 由 case `emission-legit-labels` 釘住。
