# 校準產物新鮮度監控 runbook（`atlas_calibration_freshness_*`）

> **角色**：`configs/parameters.json` 的**新鮮度**在生產上的觀測與處置。
> **建立**：2026-09-26（issue #1944 的 **I31 production 半邊**；PR #1991 的「未完成項 1」）。
> **相關**：[`docs/specs/industry-allocation-inert-audit-20260924.md`](../specs/industry-allocation-inert-audit-20260924.md)
> §10.2（CI 半邊與政策裁決）／§12（本次接線）；[`monitoring/rules/calibration_freshness_alerts.yml`](../../monitoring/rules/calibration_freshness_alerts.yml)；
> [`monitoring/tests/calibration_freshness_test.yml`](../../monitoring/tests/calibration_freshness_test.yml)。

---

## 1. 這個監控在回答什麼

CI 只看**結構**（checkout 帶進來的檔案結構上不可能新鮮），**新鮮度只有生產主機答得出來**
（檔案真的在那裡被刷新）。在本次接線之前，那條檢查只存在於「有人記得手跑」：

```bash
# production 的 freshness 命令（政策 SSOT；CI 用的是 --policy=configs/calibration-validation-policy.json）
atlas-validate --path=configs/parameters.json --max-age=48h --format=json
```

現在同一套判定（`config.ValidateCalibration`）由背景任務
`calibration_freshness_metrics_export`（`cmd/atlas/calibration_freshness_metrics_task.go`，
每 5 分鐘）執行，結果變成 Prometheus 序列，由
`monitoring/rules/calibration_freshness_alerts.yml` 的三條規則判讀。

**沒有任何新的監控系統**：指標走既有的 `MetricsCollector` + `/metrics`
（`monitoring/prometheus.yml` 的 `atlas-go` job 已是 10s 抓取），規則進既有的
`monitoring/rules/`（權威樹，`scripts/ci/check_monitoring_single_source.sh` 會擋第二棵樹）。

## 2. 指標族（權威定義：`internal/monitoring/calibration_freshness.go`）

| 指標 | 型別 | 意義 |
|---|---|---|
| `atlas_calibration_freshness_ok{artifact="parameters"}` | gauge | **1** = 新鮮（可評估，且有記錄的校準時間落在契約內）；**0** = 其他一切情況（超過契約／從未記錄校準時間／**檢查無法評估**）。0 刻意包含「不知道」⇒ fail-closed |
| `atlas_calibration_freshness_run_ok{artifact="parameters"}` | gauge | **1** = 檢查能評估產物（stat/讀取/JSON 都成功）；**0** = 不能（此時 `_ok` 的 0 **不代表**「舊」，代表「未知」） |
| `atlas_calibration_freshness_age_seconds{artifact="parameters"}` | gauge | **落後多少**（秒）：產物自述的最後校準時間（`updated_at`）距今。**只在有記錄時間時輸出** |
| `atlas_calibration_last_calibrated_timestamp_seconds{artifact="parameters"}` | gauge | **最後一次成功校準時間**（Unix 秒）。**0 = 從未記錄校準時間**（明確哨兵值，不是 1970 年） |
| `atlas_calibration_freshness_checked_timestamp_seconds{artifact="parameters"}` | gauge | 檢查最後一次執行的 Unix 秒（探針心跳） |

**合約（48h）只有一個數字**：Go 側 `monitoring.CalibrationFreshnessContract`
= 上面 production CLI 的 `--max-age=48h`。規則檔刻意**不重寫**這個門檻；Go 測試
`TestCalibrationFreshnessContractMatchesRunbook` 會讀本文件，若兩邊漂移即紅燈。

### 48h 是怎麼來的（實測基線，不是照抄別的規則）

| 基線 | 值 | 來源 |
|---|---|---|
| 生產「健康」值 | 容器啟動後約 **65 分鐘**就被校準任務改寫 | 2026-09-26 唯讀盤查：`Created=02:04:02Z` vs `updated_at=2026-09-26T03:08:44.90062791Z`（`FOLLOWUPS.md` FU-20260926-07） |
| 生產「壞掉」值 | **約 82 天**（image 內 `/app/configs/parameters.json` 的 `updated_at=2026-07-06`） | 同上；也正是 issue #1944 I30 的形狀（nightly backfill 的寫入被丟棄） |
| 校準任務 cadence | 主要 **24h**（17 個 top-level 任務；`auto_cycle_update` 6h、`regime_calibrate` 1h） | `cmd/atlas/calibration_tasks.go` |

48h 落在健康值與壞值之間（對健康值 ~48×、對壞值 ~40×），且 = 兩個 24h 週期，
可吸收一次失敗的週期與週末（`ValidateCalibration` 的既有註解即為此而設）。
⇒ 門檻與 CLI 契約一致，**監控與命令不會互相矛盾**。

## 3. 三條告警與 triage

| 告警 | severity | 條件 | 什麼意思 |
|---|---|---|---|
| `CalibrationArtifactStale` | warning | `atlas_calibration_freshness_ok == 0` **且** `..._run_ok == 1`，持續 1h | 產物**真的舊**（不是讀不到） |
| `CalibrationFreshnessUnverifiable` | error | `atlas_calibration_freshness_run_ok == 0`，持續 30m | **未知**：檢查讀不到/解不開產物（fail-closed） |
| `CalibrationFreshnessExporterDown` | warning | `atlas_calibration_freshness_checked_timestamp_seconds` 缺席，或距今 > 1800s；以 `up{job="atlas-go"} == 1` 為閘門，持續 15m | 這個檢查不再更新 ⇒ **前兩條的輸出此刻不可信** |

三條是互斥的分工（`_ok==0 且 run_ok==1` / `run_ok==0` / 探針缺席），同一個根因不會有
兩條規則各自 paging；服務掛掉時三條都沉默（由 `AtlasGoTargetDown` 負責）。

### 3.1 `CalibrationArtifactStale`（warning）

```
curl -s localhost:18080/metrics | grep '^atlas_calibration_'
```

先讀數字再決定方向（秒）：

- `atlas_calibration_freshness_age_seconds` = 落後多少。
- `atlas_calibration_last_calibrated_timestamp_seconds` = 0 ⇒ **從未記錄校準時間**
  （`updated_at` 為零）：這不是「很舊」，是「沒有記錄」——先查寫入者。

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

其他步驟：

1. 檔案在不在、時間戳對不對：
   ```bash
   docker exec atlas-go stat -c '%y %n' /app/configs/parameters.json
   docker exec atlas-go python3 -c "import json;print(json.load(open('/app/configs/parameters.json'))['updated_at'])"
   ```
2. 檢查心跳是否正常（若異常，先處理 `CalibrationFreshnessExporterDown`，本條的輸出不可信）：
   ```bash
   docker exec atlas-go curl -s localhost:18080/metrics | grep checked_timestamp
   ```
3. 載入器是否讀到同一份檔案（權威來源是應用自己的 `config.GetParametersConfigPath()`）：
   ```bash
   docker logs atlas-go 2>&1 | grep -i parameters | tail -5
   ```

### 3.2 `CalibrationFreshnessUnverifiable`（error，fail-closed）

「解析不到值／檢查失敗」**不得靜默通過**（#2009 的同族缺陷）。日誌上直接有原因：

```bash
docker logs atlas-go --since 2h 2>&1 | grep calibration_freshness
# 期待看到 WARN artifact_unverifiable ... code=PARAMS_STAT_FAILED|PARAMS_READ_FAILED|PARAMS_INVALID_JSON
```

1. 檔案存在與權限：`docker exec atlas-go ls -l /app/configs/parameters.json`
2. JSON 是否合法（不合法通常是半寫入或人為編輯）：
   `docker exec atlas-go python3 -c "import json;json.load(open('/app/configs/parameters.json'))"`
3. 檢查路徑是否指向別處（權威來源是應用自己的 `config.GetParametersConfigPath()`，
   不是硬編 `/app/configs`）。
4. 這個狀態**不代表**產物舊：`atlas_calibration_freshness_age_seconds` 此刻刻意
   **不輸出**（沒有可信時間就輸出 0 是假造量測值）。

### 3.3 `CalibrationFreshnessExporterDown`（warning）

1. 指標是否還在／換名了：`curl -s localhost:18080/metrics | grep '^atlas_calibration_'`
2. 任務接線回歸：
   ```bash
   grep -rn "calibration_freshness_metrics_export" cmd/atlas/
   docker logs atlas-go --since 6h 2>&1 | grep -E "calibration_freshness|task_failed"
   ```
   啟動日誌應有 `[Gateway] registered calibration_freshness_metrics_export background task (5m interval)`。
3. 閘門邊界：本條 firing 時 `up{job="atlas-go"}` 必須是 1；若同時是 0，
   先看 `AtlasGoTargetDown`，不要查這裡。

## 4. 驗收（部署後，root 執行）

```bash
# 1) 指標族存在，且三個「永遠應該存在」的序列都有值
curl -s localhost:18080/metrics | grep '^atlas_calibration_' | sort

# 2) 新鮮度為真（部署後先看這一格；生產實測應為 1 且 age 在數小時內）
curl -s localhost:18080/metrics | grep 'atlas_calibration_freshness_ok'

# 3) 規則已載入（Prometheus API；沒有這個端點就從 Prometheus 容器內查 /api/v1/rules）
curl -s localhost:9090/api/v1/rules | grep -o 'Calibration[A-Za-z]*' | sort -u

# 4) 負向案例（**不要**在生產上動參數檔；在本機用一份刻意過期的副本驗證匯出器）
cp configs/parameters.json /tmp/old-parameters.json
python3 - <<'PY'
import json, datetime
p = '/tmp/old-parameters.json'
d = json.load(open(p))
d['updated_at'] = (datetime.datetime.now(datetime.UTC) - datetime.timedelta(days=5)).isoformat()
json.dump(d, open(p, 'w'))
PY
go test ./internal/monitoring/ -run 'TestObserveCalibrationFreshness_Stale' -v   # 應 PASS（判定翻 0）
```

規則面本身有 `promtool` 單元測試（10 個案例，含 5 個負向對照與 5 個變異測試）：

```bash
docker run --rm -v "$PWD:/work" -w /work --entrypoint /bin/promtool \
  prom/prometheus:v3.14.0 test rules monitoring/tests/calibration_freshness_test.yml
```

## 5. 已知限制（誠實聲明）

1. **產物年齡只是「校準活動」的上界**：寫入是有變更才寫，因此已收斂的系統可以合法地
   超過 48h 不寫檔 ⇒ `CalibrationArtifactStale` 可能誤報。缺的是一個真正的
   「校準任務已執行」心跳指標（要接到 18 個校準任務），見
   [`FOLLOWUPS.md`](FOLLOWUPS.md) 的對應條目（FU-20260926-10）。
2. **結構性 finding 沒有進監控**：`L1/L2_NO_REPRESENTATIVES` 之類由 CI 的
   `--policy=configs/calibration-validation-policy.json` 負責；本族只回答「新鮮度」。
   生產端的結構漂移目前仍無自動訊號。
3. **`updated_at` 為單一時間來源**：mtime 不參與判定（生產兩者同一次寫入產生，
   而 `updated_at` 才是「校準記錄」）。CLI 仍會同時檢查 mtime，兩者在
   `cp -p` 還原這類人工操作下可能不一致。
4. **未驗證於生產**：本次只交付接線與本機/promtool 證據；生產驗收（§4）待部署後執行。
