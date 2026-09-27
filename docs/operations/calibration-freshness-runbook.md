# 校準產物新鮮度與 drift 監控 runbook（`atlas_calibration_*`）

> **角色**：校準產物的**新鮮度**（權威的 effective 產物 ＋ image 內基準）與
> **image-vs-effective drift** 在生產上的觀測與處置。
> **建立**：2026-09-26（issue #1944 的 **I31 production 半邊**；PR #1991 的「未完成項 1」）。
> **改版**：2026-09-27（issue **#2007** 的兩個 bounded 子項：① 新鮮度檢查**改標的**
> ② 加 **image-vs-effective drift 偵測**）。改版前的第 7 條「只觀測 SSOT」已由本版解決。
> **相關**：[`docs/specs/industry-allocation-inert-audit-20260924.md`](../specs/industry-allocation-inert-audit-20260924.md)
> §10.2（CI 半邊與政策裁決）／§12（接線）；[`monitoring/rules/calibration_freshness_alerts.yml`](../../monitoring/rules/calibration_freshness_alerts.yml)；
> [`monitoring/tests/calibration_freshness_test.yml`](../../monitoring/tests/calibration_freshness_test.yml)；
> [`internal/monitoring/calibration_effective.go`](../../internal/monitoring/calibration_effective.go)。

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

⇒ 在接線之前，生產上「校準產物已不新鮮」是**零觀測**：不是值班忘了跑，而是沒有東西會跑。
現在三個觀察由背景任務 `calibration_freshness_metrics_export`
（`cmd/atlas/calibration_freshness_metrics_task.go`，每 5 分鐘、同一個時戳）執行，
結果變成 Prometheus 序列，由 `monitoring/rules/calibration_freshness_alerts.yml` 的四條規則判讀。

**沒有任何新的監控系統**：指標走既有的 `MetricsCollector` + `/metrics`
（`monitoring/prometheus.yml` 的 `atlas-go` job 已是 10s 抓取），規則進既有的
`monitoring/rules/`（權威樹，`scripts/ci/check_monitoring_single_source.sh` 會擋第二棵樹）。

### 1.1 三個觀察（#2007 之後；這是本 runbook 最重要的一張表）

| 觀察 | 產物 | artifact 標籤 | 回答什麼 |
|---|---|---|---|
| 基準新鮮度 | `configs/parameters.json`（image 內、受版控、經人工審查） | `parameters` | 「**基準**多久沒被換過」。**已無告警規則**（見 §1.2） |
| 權威產物新鮮度 | `data/state/parameters.calibrated.json`（校準 overlay，bind mount 內） | `parameters_overlay` | 「**runtime 校準**最後一次交出東西是多久以前」← 第 1、2 條的標的 |
| drift | 兩者的差 | `parameters_overlay` | 「生效值偏離憲章多少、偏離的樣子說不說得通」← 第 4 條的標的 |

### 1.2 為什麼改標的（#2007 的實證）

`config.ValidateCalibration`／新鮮度檢查原本只看 `configs/parameters.json`。那在
**#2013（FU-20260926-07）之前**是對的：校準迴圈會改寫它，所以「它多久沒被改寫」
＝「校準迴圈還有沒有在動」。**#2013 之後**，容器內校準的寫入改到 bind mount 上的
overlay，基準保持 pristine。生產實測（2026-09-27，容器已跑 ~33h）：

```
atlas_calibration_freshness_ok{artifact="parameters"}            0
atlas_calibration_freshness_run_ok{artifact="parameters"}        1
atlas_calibration_freshness_age_seconds{artifact="parameters"}   7206752   ≈ 83.4 天
```

83.4 天 = 基準的 `updated_at`（`2026-07-06`，＝ image 建置日）；而 overlay 是**當天**
11:27（localtime）寫的 32,113 bytes。**證明「基準自 image 之後沒被寫過」是可靠的**：
`ParametersConfig.SaveWithRollback`（`internal/config/parameters.go`）**每一次**寫入都會
`p.UpdatedAt = time.Now()`，所以「`updated_at` 停在建置日」等價於「這個檔沒被寫過」。

⇒ `CalibrationArtifactStale` 對 `artifact="parameters"` 是**永久誤報**：它監控的不是權威產物。
修法是**新增 artifact 標籤值**（`parameters_overlay`）而不是改既有序列的語意——
`artifact="parameters"` 仍然照舊輸出（可見、可查、可畫圖），只是不再有規則讀它，
因為它在生產的「不新鮮」是**預期狀態**。

## 2. 指標族（權威定義：`internal/monitoring/calibration_freshness.go` ＋ `calibration_effective.go`）

### 2.1 新鮮度（兩個 artifact 共用同一組名稱）

| 指標 | 型別 | 意義 |
|---|---|---|
| `atlas_calibration_freshness_ok{artifact}` | gauge | **1** = 新鮮；**0** = 其他一切情況（超過契約／從未記錄校準時間／檔案不存在／**檢查無法評估**）。0 刻意包含「不知道」⇒ fail-closed |
| `atlas_calibration_freshness_run_ok{artifact}` | gauge | **1** = 檢查能評估產物；**0** = 不能（此時 `_ok` 的 0 **不代表**「舊」，代表「未知」） |
| `atlas_calibration_freshness_age_seconds{artifact}` | gauge | **落後多少**（秒）。只在「可評估且有記錄時間」時輸出 |
| `atlas_calibration_last_calibrated_timestamp_seconds{artifact}` | gauge | **最後一次成功校準時間**（Unix 秒）。**0 = 從未記錄校準時間**（明確哨兵值，不是 1970 年） |
| `atlas_calibration_freshness_checked_timestamp_seconds{artifact="parameters"}` | gauge | 檢查最後一次執行的 Unix 秒（探針心跳）。**任務級**，不帶 artifact 的其他值（見 §3.3） |

兩個 artifact 的「新鮮」定義不同，這是刻意的：

| artifact | 判定 | 為什麼 |
|---|---|---|
| `parameters` | `updated_at` **與檔案 mtime** 都在 48h 內（＝ `config.ValidateCalibration`，與 CLI 同判） | 它是受審查的檔案：`cp -p` 之類的還原也要看得出來 |
| `parameters_overlay` | overlay 自己的 `updated_at` 在 48h 內 | 它不是 parameters 文件（沒有 `classification_tree` 可驗結構），而 `SaveCalibrationOverlay` 每次寫入都用 tmp→rename 蓋章 ⇒ 年齡＝校準活動的界 |

### 2.2 drift

| 指標 | 型別 | 意義 |
|---|---|---|
| `atlas_calibration_drift_run_ok{artifact="parameters_overlay"}` | gauge | **1** = 基準 vs 生效值的比較做完了；**0** = 沒做完（基準讀不到/解析失敗、overlay 解不開）⇒ 下面三個是**凍結值** |
| `atlas_calibration_drift_keys{artifact="parameters_overlay"}` | gauge | **現在**有幾個 key 的生效值與基準不同（**可見性**；> 0 是 intended 的正常狀態） |
| `atlas_calibration_drift_out_of_window_keys{artifact="parameters_overlay"}` | gauge | 其中 `生效值/基準值` 落在單步窗 `[1/3, 3]` 之外的數量（`config.RatioOutsideSingleStepWindow`，與 loader 的 WARN 同判定） |
| `atlas_calibration_drift_dropped_keys{artifact="parameters_overlay",reason}` | gauge | overlay 宣告了但**沒生效**的 entry 數量。`reason="ssot_moved"`＝基準已移動（設計行為，只呈現）；`reason="not_applicable"`＝參數/路徑不存在或值型別不可套用（**缺陷**） |

> 命名：drift 這一族刻意**沒有** `_total` 後綴——它們是 gauge（last-write-wins 的
> **現況**），不是單調累加的事件計數；加 `_total` 會讓使用者對它們做 `rate()`。

### 2.3 判讀順序（**照這個順序，不要跳**）

1. **先看 `_run_ok`**（`atlas_calibration_freshness_run_ok{artifact="parameters_overlay"}`）
   與 `atlas_calibration_drift_run_ok`。任一為 0 ⇒ 它下面的值都是**凍結樣本**，
   不代表現況：
   - `age` / `last_calibrated` / `drift_*` 只由「可評估/可比較」的輪次寫入；collector 是
     last-write-wins 且**不會移除序列**，所以 `/metrics` 上會留著最後一次的值。
   - 判定值 `_ok` 與 `drift_run_ok` **每一輪都被覆寫**，所以它們不會騙人。
2. **再看判定值**：`_ok`（新鮮度）與 `drift_out_of_window_keys` / `drift_dropped_keys`（異常）。
3. **最後**才把 `age` / `last_calibrated` / `drift_keys` 當人可讀補充。

> 沒有任何規則拿 `age` / `last_calibrated` / `drift_keys` 做**判定**（規則的 expr 只看
> `_ok` / `_run_ok` / 兩個異常計數）⇒ 凍結值不會製造誤報。

### 2.4 `_ok` 與 `age` 量的不是同一件事（只適用基準那一條）

`_ok{artifact="parameters"}` 是 CLI 的同一個判定（`updated_at` **與檔案 mtime** 都看）；
`age` 只反映 `updated_at`。以 `cp -p` 還原一份舊檔時，mtime 舊（`_ok` = 0）而 `updated_at` 新
（`age` 小）——**以 `_ok` 為準**。`MTIME_STALE` 由 Go 測試
`TestObserveCalibrationFreshness_MtimeStaleMatchesCLI` 釘住。

### 2.5 契約（48h）只有一個數字

`monitoring.CalibrationFreshnessContract` **就是** `config.DefaultCalibrationMaxAge`，
而 CLI 的 `--max-age` flag 預設值也引用它 ⇒ 全 repo 只有一個 48h
（Go 測試 `TestCalibrationFreshnessContractMatchesCLIAndRunbook` 讀 CLI 原始碼與本文件把這件事釘住）。
**兩個 artifact 共用這一個契約**（overlay 的判定沒有第二個門檻）。

**48h 的實測基線（不是照抄別的規則）**

| 基線 | 值 | 來源 |
|---|---|---|
| 生產 overlay「健康」 | 2026-09-27 03:27Z 寫入，量測時 age ≈ 1733s（~29 分鐘）；校準任務群 cadence 24h | 2026-09-27 唯讀盤查（本 PR 的 probe，見 §4） |
| 生產基準「不新鮮」 | 約 **83.4 天**（`updated_at=2026-07-06`）——**這是預期狀態**，不是故障 | 同上；也是 issue #1944 I30 的形狀（nightly backfill 的寫入被丟棄） |
| 校準任務 cadence | 主要 **24h**（17 個 top-level 任務；`auto_cycle_update` 6h、`regime_calibrate` 1h） | `cmd/atlas/calibration_tasks.go` |

48h = 兩個 24h 週期，可吸收一次失敗的週期與週末（`ValidateCalibration` 的既有註解即為此而設）。

## 3. 四條告警與 triage

| 告警 | severity | 條件 | for | 什麼意思 |
|---|---|---|---|---|
| `CalibrationArtifactStale` | warning | `freshness_ok{artifact="parameters_overlay"} == 0` **且** `freshness_run_ok{artifact="parameters_overlay"} == 1` | 1h | 權威產物（overlay）**真的不新鮮**（超過契約／從未記錄／檔案不存在） |
| `CalibrationFreshnessUnverifiable` | error | `freshness_run_ok{artifact="parameters_overlay"} == 0` | 15m | **未知**：檢查讀不到/解不開 overlay（fail-closed） |
| `CalibrationFreshnessExporterDown` | warning | `checked_timestamp_seconds` 缺席，或距今 > 1800s；以 `up{job="atlas-go"} == 1` 為閘門 | 15m | 任務不再更新 ⇒ 前三／四條的輸出此刻不可信 |
| `CalibrationEffectiveDriftUnexplained`（**新**） | warning | `drift_out_of_window_keys > 0` **或** `drift_dropped_keys{reason="not_applicable"} > 0`，且 `drift_run_ok == 1` | 1h | 生效值偏離基準，而且**偏離的樣子說不通**（超出單步窗／有套不上的 entry） |

四條是互斥的分工（`_ok==0 且 run_ok==1` / `run_ok==0` / 探針缺席 / drift 異常且可比較），
同一個根因不會有兩條規則各自 paging；服務掛掉時四條都沉默（由 `AtlasGoTargetDown` 負責）。

> **`artifact="parameters"`（image 內基準）沒有任何告警**，這是刻意的（§1.2）：
> 它在生產一定會超過 48h。它有資訊價值，但那由 drift 承擔。

### 3.1 `CalibrationArtifactStale`（warning）

先分辨三種子狀態（「舊」／「從未記錄」／「檔案不存在」）：

```bash
# 「從未記錄」⇒ 值是 null/空；「檔案不存在」⇒ ls 就失敗
docker exec atlas-go grep -o '"updated_at": *"[^"]*"' /app/data/state/parameters.calibrated.json | head -1
docker exec atlas-go ls -l /app/data/state/parameters.calibrated.json
# 落後多少（只在「舊」的情況有值）
docker exec atlas-go curl -s localhost:18080/metrics | grep 'atlas_calibration_freshness_age_seconds{artifact="parameters_overlay"}'
```

「檔案不存在」＝ 沒有任何 runtime 校準被持久化（#2013 要消滅的狀態）。先確認寫入路由：

```bash
docker logs atlas-go --since 24h 2>&1 | grep -E "calibration_writeback|overlay_written"
# 完全沒有這類日誌 ⇒ 校準沒有把結果持久化到 overlay（risk gate 等自適應值不會跨重建存活）
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
docker logs atlas-go --since 6h 2>&1 | grep -E "overlay_not_fresh|calibration_freshness"
```

### 3.2 `CalibrationFreshnessUnverifiable`（error，fail-closed）

「解析不到值／檢查失敗」**不得靜默通過**（#2009 的同族缺陷）。日誌上直接有原因：

```bash
docker logs atlas-go --since 2h 2>&1 | grep overlay_unverifiable
# 期待看到 WARN overlay_unverifiable ... code=OVERLAY_STAT_FAILED|OVERLAY_READ_FAILED|OVERLAY_INVALID_DOCUMENT
```

1. 檔案存在與權限：`docker exec atlas-go ls -l /app/data/state/parameters.calibrated.json`
2. 為什麼解析失敗（生產 image **沒有 python/jq/node**，用 grep/head/tail）：
   ```bash
   docker exec atlas-go head -c 200 /app/data/state/parameters.calibrated.json
   docker exec atlas-go tail -c 80 /app/data/state/parameters.calibrated.json
   ```
3. 檢查路徑是否指向別處（權威來源是製程註冊的 `config.GetCalibratedOverlayPath()`；
   `cmd/atlas/main.go` 啟動時註冊 `<workDir>/data/state/parameters.calibrated.json`）。
4. 這個狀態**不代表**產物舊：`age` / `last_calibrated`／`drift_*` 此刻都是**凍結值**（§2.3）。
5. `drift_run_ok` 必然也是 0（同一個根因）：不要把它讀成「沒有偏離」。

### 3.3 `CalibrationFreshnessExporterDown`（warning）

1. 指標是否還在／換名了：`docker exec atlas-go curl -s localhost:18080/metrics | grep '^atlas_calibration_'`
2. 任務接線回歸：
   ```bash
   grep -rn "calibration_freshness_metrics_export" cmd/atlas/
   docker logs atlas-go --since 6h 2>&1 | grep -E "calibration_freshness|calibration_drift|task_failed"
   ```
   啟動日誌應有 `[Gateway] registered calibration_freshness_metrics_export background task (5m interval)`；
   `scripts/ci/check_critical_tasks.sh` 也把這個 task 名列為關鍵任務（被 compiler DCE／變數遮蔽
   移除時 CI 會紅在 `strings` 檢查）。
3. 閘門邊界：本條 firing 時 `up{job="atlas-go"}` 必須是 1；若同時是 0，
   先看 `AtlasGoTargetDown`，不要查這裡。
4. 心跳是**任務級**的（三個觀察同一次呼叫、同一個時戳寫入，所以它**不帶** artifact 標籤）：
   這一條 firing 時，**兩個** artifact 與 drift 的序列全部不可信。

### 3.4 `CalibrationEffectiveDriftUnexplained`（warning，**新**）

**先讀懂這一條不是什麼**：它不是「有偏離就告警」。runtime 校準是 **intended**
（業主定案：持久化 overlay ＋ 漂移偵測），`atlas_calibration_drift_keys > 0` 是**健康**
系統的正常狀態，對它開告警就是重複我們剛修掉的永久誤報。它只在偏離**說不通**時 firing：

| 條件 | 意思 | 怎麼修 |
|---|---|---|
| `drift_out_of_window_keys > 0` | 有 key 的 `生效值/基準值` 落在單步窗 `[1/3, 3]` 外 ⇒ 與 `logOverlayApplication` 的既有 WARN 同判定 | 先判斷是不是**多輪累積**（窗是相對於憲章的累積比值）；目的正當 ⇒ 對齊/更新基準那條線；不正當 ⇒ 查該 calibrator |
| `drift_dropped_keys{reason="not_applicable"} > 0` | overlay 宣告了不存在的參數/路徑，或值型別不可套用 ⇒ 那些「已校準」的值**永遠不會生效** | 修正 overlay 的 key/型別或產生它的校準器；下次載入會把 entry 丟掉（本條 resolve，`ssot_moved` 那一格記一筆） |

```bash
# 全貌
docker exec atlas-go curl -s localhost:18080/metrics | grep -E "atlas_calibration_drift_(keys|out_of_window|dropped|run_ok)"
# 逐筆宣告（生產 image 沒有 jq，用 grep）
docker exec atlas-go grep -o '"path": *"[^"]*"' /app/data/state/parameters.calibrated.json | sort
docker exec atlas-go grep -o '"method": *"[^"]*"' /app/data/state/parameters.calibrated.json | sort | uniq -c
# 誰寫的、是不是一次野蠻的單步
docker logs atlas-go --since 72h 2>&1 | grep -E "overlay_entry_applied|overlay_entry_outside_single_step_window"
```

> 這一條與第 1 條的關係：overlay 不新鮮（校準沒在動）由第 1 條負責；本條只管
> 「**有東西在動**，但動的樣子說不通」。兩者可以同時 firing（不同根因）。

## 4. 驗收（部署後，root 執行）

```bash
# 1) 指標族存在，且「永遠應該存在」的序列都有值
docker exec atlas-go curl -s localhost:18080/metrics | grep '^atlas_calibration_' | sort

# 2) 改版的核心驗收：基準可以是 0（預期），**overlay 必須是 1**，drift 必須可比較
docker exec atlas-go curl -s localhost:18080/metrics | grep -E 'atlas_calibration_(freshness_ok|freshness_run_ok|drift_run_ok|drift_keys)'
# 期待（2026-09-27 生產實測；age 會隨時間變大）:
#   atlas_calibration_freshness_ok{artifact="parameters"}             0
#   atlas_calibration_freshness_ok{artifact="parameters_overlay"}     1
#   atlas_calibration_drift_run_ok{artifact="parameters_overlay"}     1
#   atlas_calibration_drift_keys{artifact="parameters_overlay"}       6
# ⇒ CalibrationArtifactStale 必須是 **inactive**（部署前它對 artifact="parameters" 永久 firing）

# 3) 規則已載入（Prometheus API）
curl -s localhost:9090/api/v1/rules | grep -o 'Calibration[A-Za-z]*' | sort -u

# 4) 反例（**不要**在生產上動參數檔；在 repo checkout 上驗證匯出器與 CLI 的判定一致）
go test ./internal/monitoring/ ./cmd/atlas/ -run 'Calibration' -v
# 出貨檔的實測值會印在 TestObserveCalibrationFreshness_ShippedArtifactIsEvaluable 的輸出。
# fixture 邊界與 fail-closed 由 TestObserveCalibrationFreshness_* /
# TestExportCalibrationMetrics_* / TestInspectCalibratedOverlayLayer_* 覆蓋。
go run ./cmd/calibration-validate --path=configs/parameters.json --format=json | head -20   # CLI 側
```

規則面本身有 `promtool` 單元測試（**16 個案例**，每一個都對四條規則斷言 firing 或沉默）：

```bash
docker run --rm -v "$PWD:/work" -w /work --entrypoint /bin/promtool \
  prom/prometheus:v3.14.0 test rules monitoring/tests/calibration_freshness_test.yml
```

**用真實資料重現 #2007 的樣本**（本 PR 的 probe：把生產 overlay 讀回來，套在出貨基準上）：

```bash
# 生產 overlay（唯讀取回；不進版控，也不寫回生產）
ssh kmacmini 'cat /Users/kaecer/workspace/atlas/data/state/parameters.calibrated.json' > /tmp/prod-overlay.json
PROD_OVERLAY=/tmp/prod-overlay.json go test ./cmd/atlas/ -run TestProbeProductionOverlayDrift -v
```

2026-09-27 實測輸出（生產 overlay 6 筆、全部來自 `calibrate_seasonal`）：
`overlay run_ok=true fresh=true age≈1733s`、`drift keys(6)`、
`out_of_window(0)`、`not_applicable(0)`、`ssot_moved(0)`。
⇒ 兩個驗收同時成立：**改標的後告警不再永久 firing**，而且**新的 drift 告警在真實資料上不誤報**。
（`#2007` 記載的 `risk/max_daily_loss_pct` 0.03 vs 0.0108 屬於 **#2013 之前的 SSOT 回寫世代**；
今日 overlay 沒有這一筆，該樣本以固定 fixture 重現在
`TestCalibrationOverlayDriftForRealProductionSample` 與
`TestExportCalibrationMetrics_OverlayFreshAndRealDriftVisible`。）

變異測試（**手跑、不進版控**；在 /tmp 的副本上做，改規則後確認測試紅燈 —— 8 項全部被咬住）：

| 變異 | 被咬住的案例 |
|---|---|
| 第 4 條 expr 改成 `drift_keys > 0`（把「有 drift 就 page」裝回去） | A、L（＋B/C/E/K/N 的沉默斷言） |
| 拿掉第 4 條的 `drift_run_ok == 1` 閘門 | O |
| 第 1 條標的改回 `artifact="parameters"`（把誤報裝回去） | A |
| 第 2 條標的改回 `artifact="parameters"` | D、O |
| 第 1 條 `for: 1h` → `0m` | I |
| 第 4 條 `for: 1h` → `0m` | P |
| 第 3 條 `1800` → `3600` | E |
| 第 3 條拿掉 `up` 閘門 | G |

## 5. 已知限制（誠實聲明）

1. **產物年齡只是「校準活動」的上界**：寫入是有變更才寫，因此已收斂的系統可以合法地
   超過 48h 不寫檔 ⇒ `CalibrationArtifactStale` 可能誤報。缺的是一個真正的
   「校準任務已執行」心跳指標（要接到 18 個校準任務），見
   [`FOLLOWUPS.md`](FOLLOWUPS.md) 的 FU-20260926-10。
2. **凍結樣本是刻意的行為**（§2.3）：`age` / `last_calibrated` / `drift_*` 在
   `run_ok = 0` 之後仍以最後一次可評估的值留在 `/metrics`。規則不被它騙
   （判定一律看 `_ok` / `_run_ok`），但**人**必須照 §2.3 的順序讀。
3. **第 1 條 → 第 2 條有 ≤15 分鐘的交接窗**（promtool 案例 K）。
4. **結構性 finding 沒有進監控**：`L1/L2_NO_REPRESENTATIVES` 之類由 CI 的
   `--policy=configs/calibration-validation-policy.json` 負責；本族只回答「新鮮度 + drift」。
5. **`_ok` 與 `age` 可能不一致**（§2.4，只適用基準）：以 `_ok` 為準。
6. **生產 image 沒有 `atlas-validate`**：這條政策命令需要在 image 外執行（或用
   `go build ./cmd/calibration-validate`）；是否有人這樣做過仍未查證。
7. **overlay 的新鮮度是「檔案級」的**：`atlas_calibration_drift_keys` 與 overlay 的
   `updated_at` 都無法分辨「所有 calibrator 都在跑」與「只有一個 calibrator 一直寫、
   其他都死了」——**per-entry 的 `calibrated_at` 才是那個問題的資料面**，目前沒有把它
   變成序列（避免 cardinality 與 `_total`/gauge 誤用）。要查時直接讀 overlay 的
   `calibrated_at`（見 §3.4 的逐筆命令）。
8. **單步窗是「相對於憲章的累積比值」**：一個合法的多輪同向校準（例如連續三輪各
   ×2/3）最後也可能落在窗外 ⇒ 第 4 條的 `out_of_window` arm 是「人該看一眼」的訊號，
   不是「一定有 bug」的證明。triage 的第一步永遠是判斷目的是否正當。
9. **基準不再有告警**（§1.2）：`artifact="parameters"` 的訊號只在 `/metrics` 上，
   沒有人會被它叫醒。若未來真的需要「基準也該被換了」的提醒（例如年度憲章複審），
   那是另一條規則與另一個門檻，不在本族。
10. **未驗證於生產**：本次只交付接線與本機/promtool/probe 證據；生產驗收（§4）待部署後執行。
