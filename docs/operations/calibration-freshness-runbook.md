# 校準產物新鮮度監控 runbook（`atlas_calibration_freshness_*`）

> **角色**：`configs/parameters.json` 的**新鮮度**在生產上的觀測與處置。
> **建立**：2026-09-26（issue #1944 的 **I31 production 半邊**；PR #1991 的「未完成項 1」）。
> **相關**：[`docs/specs/industry-allocation-inert-audit-20260924.md`](../specs/industry-allocation-inert-audit-20260924.md)
> §10.2（CI 半邊與政策裁決）／§12（本次接線）；[`monitoring/rules/calibration_freshness_alerts.yml`](../../monitoring/rules/calibration_freshness_alerts.yml)；
> [`monitoring/tests/calibration_freshness_test.yml`](../../monitoring/tests/calibration_freshness_test.yml)。

---

## 1. 這個監控在回答什麼

CI 只看**結構**（checkout 帶進來的檔案結構上不可能新鮮），**新鮮度只有生產主機答得出來**
（檔案真的在那裡被刷新）。政策上的 production 命令是：

```bash
# production 的 freshness 命令（政策 SSOT；CI 用的是 --policy=configs/calibration-validation-policy.json）
atlas-validate --path=configs/parameters.json --max-age=48h --format=json
```

⚠️ **但這條命令沒有隨 image 出貨**。`cmd/calibration-validate` 是 CI 現場 build 的
（`.github/workflows/nightly-refresh.yml`：`go build -o atlas-validate ./cmd/calibration-validate`），
本 repo 的 Dockerfile 沒有把它放進 `/app`。實查（2026-09-26，本機用同一份 Dockerfile 建出的 image）：

```bash
docker run --rm --entrypoint /bin/sh atlas-atlas:latest -c 'ls /app'
# atlas-go  atlas-mcp  calibrate-seasonal  daily-replay-sync  configs  data  logs  prompts  reports  scripts  sql

# ⚠️ 逐個查，不要寫成 `command -v python3 jq node`（dash 的 builtin 對多個名字只回第一個，
#    會把「有 python3」誤判成「有」）。
docker run --rm --entrypoint /bin/sh atlas-atlas:latest -c 'command -v python3 || echo python3-MISSING'
# python3-MISSING（jq / node 同樣 MISSING）
docker run --rm --entrypoint /bin/sh atlas-atlas:latest -c 'command -v grep || echo grep-MISSING'
# /bin/grep（stat / head / tail / curl / sed / awk / cat / ls 都在）
```

⇒ 在本次接線之前，生產上「`configs/parameters.json` 已不新鮮」是**零觀測**：
不是值班忘了跑，而是沒有東西會跑。（若這條命令曾在生產被執行，那必然是 image 外的路徑
—— 本 PR 未能查證，明示為未驗證。）

現在同一套判定（`config.ValidateCalibration`）由背景任務
`calibration_freshness_metrics_export`（`cmd/atlas/calibration_freshness_metrics_task.go`，每 5 分鐘）
執行，結果變成 Prometheus 序列，由 `monitoring/rules/calibration_freshness_alerts.yml` 的三條規則判讀。

**沒有任何新的監控系統**：指標走既有的 `MetricsCollector` + `/metrics`
（`monitoring/prometheus.yml` 的 `atlas-go` job 已是 10s 抓取），規則進既有的
`monitoring/rules/`（權威樹，`scripts/ci/check_monitoring_single_source.sh` 會擋第二棵樹）。

## 2. 指標族（權威定義：`internal/monitoring/calibration_freshness.go`）

| 指標 | 型別 | 意義 |
|---|---|---|
| `atlas_calibration_freshness_ok{artifact="parameters"}` | gauge | **1** = 新鮮（沒有任何 freshness finding）；**0** = 其他一切情況（超過契約／從未記錄校準時間／**檢查無法評估**）。0 刻意包含「不知道」⇒ fail-closed |
| `atlas_calibration_freshness_run_ok{artifact="parameters"}` | gauge | **1** = 檢查能評估產物（stat/讀取/JSON 都成功）；**0** = 不能（此時 `_ok` 的 0 **不代表**「舊」，代表「未知」） |
| `atlas_calibration_freshness_age_seconds{artifact="parameters"}` | gauge | **落後多少**（秒）：產物 `updated_at` 距今。只在有記錄時間時輸出 |
| `atlas_calibration_last_calibrated_timestamp_seconds{artifact="parameters"}` | gauge | **最後一次成功校準時間**（Unix 秒）。**0 = 從未記錄校準時間**（明確哨兵值，不是 1970 年） |
| `atlas_calibration_freshness_checked_timestamp_seconds{artifact="parameters"}` | gauge | 檢查最後一次執行的 Unix 秒（探針心跳） |

### 2.1 判讀順序（**照這個順序，不要跳**）

1. **先看 `_run_ok`**。它是 0 ⇒ 下面三個都是**凍結樣本**，不代表現況：
   - `age` / `last_calibrated` 只由「可評估」的輪次寫入；collector 是 last-write-wins
     且**不會移除序列**，所以 `/metrics` 上會留著最後一次可評估時的值。
   - 判定值 `_ok` **每一輪都被覆寫**（未知 ⇒ 0），所以 `_ok` 不會騙人。
   - 這個狀態由 `CalibrationFreshnessUnverifiable`（error）單獨負責。
2. **再看 `_ok`**：這是唯一被規則使用的判定值。
3. **最後**才把 `age` / `last_calibrated` 當「最後一次可評估時」的人可讀補充。

> 沒有任何規則拿 `age` / `last_calibrated` 做**判定**（規則的 expr 只看 `_ok` / `_run_ok`）
> ⇒ 凍結值不會製造誤報；它只是會被值班的人讀到，所以在此寫明。

### 2.2 `_ok` 與 `age` 量的不是同一件事

`_ok` 是 CLI 的同一個判定（`updated_at` **與檔案 mtime** 都看）；`age` 只反映 `updated_at`。
以 `cp -p` 還原一份舊檔時，mtime 舊（`_ok` = 0）而 `updated_at` 新（`age` 小）——
**以 `_ok` 為準**（與 CLI 一致）。`MTIME_STALE` 由 Go 測試
`TestObserveCalibrationFreshness_MtimeStaleMatchesCLI` 釘住（gauge 必須與 CLI 同判）。

### 2.3 契約（48h）只有一個數字

`monitoring.CalibrationFreshnessContract` **就是** `config.DefaultCalibrationMaxAge`，
而 CLI 的 `--max-age` flag 預設值也引用它 ⇒ 全 repo 只有一個 48h
（Go 測試 `TestCalibrationFreshnessContractMatchesCLIAndRunbook` 讀 CLI 原始碼與本文件把這件事釘住）。

**48h 的實測基線（不是照抄別的規則）**

| 基線 | 值 | 來源 |
|---|---|---|
| 生產「健康」 | 容器啟動後約 **65 分鐘**就被校準任務改寫 | 2026-09-26 唯讀盤查：`Created=02:04:02Z` vs `updated_at=2026-09-26T03:08:44.90062791Z`（`FOLLOWUPS.md` FU-20260926-07） |
| 生產「壞掉」 | 約 **82.8 天**（image 內 `/app/configs/parameters.json` 的 `updated_at=2026-07-06`；本 PR 對出貨檔實跑 ≈82.8 天，`age_seconds` 隨時間增加，重跑測試會印當下的值） | 同上；也正是 issue #1944 I30 的形狀（nightly backfill 的寫入被丟棄） |
| 校準任務 cadence | 主要 **24h**（17 個 top-level 任務；`auto_cycle_update` 6h、`regime_calibrate` 1h） | `cmd/atlas/calibration_tasks.go` |

48h 落在兩者之間（對健康值 ~44×、對壞值 ~41×），且 = 兩個 24h 週期，
可吸收一次失敗的週期與週末（`ValidateCalibration` 的既有註解即為此而設）。

## 3. 三條告警與 triage

| 告警 | severity | 條件 | for | 什麼意思 |
|---|---|---|---|---|
| `CalibrationArtifactStale` | warning | `_ok == 0` **且** `_run_ok == 1` | 1h | 產物**真的不新鮮**（超過契約 **或** 從未記錄校準時間） |
| `CalibrationFreshnessUnverifiable` | error | `_run_ok == 0` | 15m | **未知**：檢查讀不到/解不開產物（fail-closed） |
| `CalibrationFreshnessExporterDown` | warning | `_checked_timestamp_seconds` 缺席，或距今 > 1800s；以 `up{job="atlas-go"} == 1` 為閘門 | 15m | 這個檢查不再更新 ⇒ 前兩條的輸出此刻不可信 |

三條是互斥的分工（`_ok==0 且 run_ok==1` / `run_ok==0` / 探針缺席），同一個根因不會有
兩條規則各自 paging；服務掛掉時三條都沉默（由 `AtlasGoTargetDown` 負責）。

### 3.1 `CalibrationArtifactStale`（warning）

先分辨「舊」還是「從未記錄」：

```bash
# 「從未記錄」⇒ 值是 null/空；此時 age 序列**不會**有值，別去 grep 它
docker exec atlas-go grep -o '"updated_at": *"[^"]*"' /app/configs/parameters.json | head -1
# 檔案時間戳（mtime 是 _ok 判定的一部分；cp -p 之類的還原會讓它變舊）
docker exec atlas-go stat -Lc '%y %n' /app/configs/parameters.json
# 落後多少（只在「舊」的情況有值）
docker exec atlas-go curl -s localhost:18080/metrics | grep atlas_calibration_freshness_age_seconds
```

> ⚠️ **假陽性第一順位（先查這個，不要直奔資料源）**
> 校準寫入是**有變更才寫**：`internal/risk/self_calibrate.go` 只有
> `len(report.Changes) > 0` 才呼叫 `LockedSaveWithRollback`。一個**已收斂**的系統
> 可能連續多輪 `verdict=stable` 而合法地超過 48h 不改寫檔案。
>
> ```bash
> docker logs atlas-go --since 72h 2>&1 | grep -E "self_calibrate|verdict"
> ```
>
> 看到連續多輪 `stable` / `no adjustments needed` ⇒ 產物沒有變舊的理由，
> 要查的是「校準任務到底有沒有在跑」而不是資料源。**目前沒有「校準任務已執行」
> 的心跳指標**（見 §5），所以這一格只能靠日誌。

心跳與日誌（確認這個判定本身可信）：

```bash
docker exec atlas-go curl -s localhost:18080/metrics | grep atlas_calibration_freshness_checked
docker logs atlas-go --since 6h 2>&1 | grep -E "artifact_not_fresh|calibration_freshness"
```

### 3.2 `CalibrationFreshnessUnverifiable`（error，fail-closed）

「解析不到值／檢查失敗」**不得靜默通過**（#2009 的同族缺陷）。日誌上直接有原因：

```bash
docker logs atlas-go --since 2h 2>&1 | grep artifact_unverifiable
# 期待看到 WARN artifact_unverifiable ... code=PARAMS_STAT_FAILED|PARAMS_READ_FAILED|PARAMS_INVALID_JSON findings=N
```

1. 檔案存在與權限：`docker exec atlas-go ls -l /app/configs/parameters.json`
2. 為什麼解析失敗（生產 image **沒有 python/jq/node**，用 grep/head/tail）：
   ```bash
   docker exec atlas-go head -c 200 /app/configs/parameters.json
   docker exec atlas-go tail -c 80 /app/configs/parameters.json
   ```
3. 檢查路徑是否指向別處（權威來源是應用自己的 `config.GetParametersConfigPath()`，
   不是硬編 `/app/configs`）。
4. 這個狀態**不代表**產物舊：`age` / `last_calibrated` 此刻是**凍結值**（§2.1）。

### 3.3 `CalibrationFreshnessExporterDown`（warning）

1. 指標是否還在／換名了：`docker exec atlas-go curl -s localhost:18080/metrics | grep '^atlas_calibration_'`
2. 任務接線回歸：
   ```bash
   grep -rn "calibration_freshness_metrics_export" cmd/atlas/
   docker logs atlas-go --since 6h 2>&1 | grep -E "calibration_freshness|task_failed"
   ```
   啟動日誌應有 `[Gateway] registered calibration_freshness_metrics_export background task (5m interval)`；
   `scripts/ci/check_critical_tasks.sh` 也把這個 task 名列為關鍵任務（被 compiler DCE／變數遮蔽
   移除時 CI 會紅在 `strings` 檢查）。
3. 閘門邊界：本條 firing 時 `up{job="atlas-go"}` 必須是 1；若同時是 0，
   先看 `AtlasGoTargetDown`，不要查這裡。

## 4. 驗收（部署後，root 執行）

```bash
# 1) 指標族存在，且三個「永遠應該存在」的序列都有值
docker exec atlas-go curl -s localhost:18080/metrics | grep '^atlas_calibration_' | sort

# 2) 新鮮度為真（部署後先看這一格；生產實測的 _ok 應為 1，age 在數小時內）
docker exec atlas-go curl -s localhost:18080/metrics | grep atlas_calibration_freshness_ok
docker exec atlas-go curl -s localhost:18080/metrics | grep atlas_calibration_freshness_age_seconds

# 3) 規則已載入（Prometheus API）
curl -s localhost:9090/api/v1/rules | grep -o 'Calibration[A-Za-z]*' | sort -u

# 4) 反例（**不要**在生產上動參數檔；在 repo checkout 上驗證匯出器與 CLI 的判定一致）
go test ./internal/monitoring/ -run 'CalibrationFreshness' -v
# 出貨檔的實測值會印在 TestObserveCalibrationFreshness_ShippedArtifactIsEvaluable 的輸出，
# 例如（2026-09-26 量測；age_seconds 隨時間增加，數字不是固定值）：
#   fresh=false freshness_code="UPDATED_AT_STALE" age_seconds≈7150619 last_calibrated=2026-07-05T17:43:01Z
# fixture 的邊界與 fail-closed 案例由 TestObserveCalibrationFreshness_ContractBoundary /
# _StaleArtifactFlipsOK / _MissingFileIsUnverifiable / _UnverifiableFreezesLastKnownSeries 覆蓋。
go run ./cmd/calibration-validate --path=configs/parameters.json --format=json | head -20   # CLI 側
```

規則面本身有 `promtool` 單元測試（**11 個案例**，含 6 個負向對照與 1 個「已知交接窗」）：

```bash
docker run --rm -v "$PWD:/work" -w /work --entrypoint /bin/promtool \
  prom/prometheus:v3.14.0 test rules monitoring/tests/calibration_freshness_test.yml
```

變異測試（**手跑、不進版控**；在 /tmp 的副本上做，改規則後確認測試紅燈 —— 8 項全部被咬住）：
拿掉 `and _run_ok == 1`／拿掉 `up` 閘門／拿掉 `absent()` arm／第 2 條 `== 0` 改 `== 1`／
第 1 條 `for: 1h` → `0m`／第 3 條 `1800` → `3600`／第 2 條 `for: 15m` → `30m`／
規則側加回 `or vector(0)`。逐項對應的紅燈案例見 PR 說明。

## 5. 已知限制（誠實聲明）

1. **產物年齡只是「校準活動」的上界**：寫入是有變更才寫，因此已收斂的系統可以合法地
   超過 48h 不寫檔 ⇒ `CalibrationArtifactStale` 可能誤報。缺的是一個真正的
   「校準任務已執行」心跳指標（要接到 18 個校準任務），見
   [`FOLLOWUPS.md`](FOLLOWUPS.md) 的 FU-20260926-10。
2. **凍結樣本是刻意的行為**（§2.1）：`age` / `last_calibrated` 在 `_run_ok = 0` 之後
   仍以最後一次可評估的值留在 `/metrics`（collector 不移除序列）。沒有任何規則被它騙
   （判定一律看 `_ok` / `_run_ok`），但**人**必須照 §2.1 的順序讀。此行為由
   `TestObserveCalibrationFreshness_UnverifiableFreezesLastKnownSeries` 釘住。
3. **第 1 條 → 第 2 條有 ≤15 分鐘的交接窗**（promtool 案例 K）：`_run_ok` 由 1 翻 0 時
   第 1 條立刻 resolve，第 2 條要累積 15m 才 firing。這是「同一個根因不重複 paging」的
   取捨；窗內那兩條都不 firing。若要把窗縮小，改第 2 條的 `for`（並同步案例 K）。
4. **結構性 finding 沒有進監控**：`L1/L2_NO_REPRESENTATIVES` 之類由 CI 的
   `--policy=configs/calibration-validation-policy.json` 負責；本族只回答「新鮮度」。
   生產端的結構漂移目前仍無自動訊號。
5. **`_ok` 與 `age` 可能不一致**（§2.2）：`_ok` 同時看 mtime，`age` 只看 `updated_at`。
   以 `_ok` 為準。
6. **生產 image 沒有 `atlas-validate`**：這條政策命令需要在 image 外執行（或用
   `go build ./cmd/calibration-validate`）；本 PR 未查證是否有人這樣做過。
7. **未驗證於生產**：本次只交付接線與本機/promtool 證據；生產驗收（§4）待部署後執行。
