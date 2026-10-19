# channel-health runbook（通道資料陳舊 / 抓取延遲 / 健康狀態 / 錯誤率）

> **告警（4 條規則、7 個 `runbook_url` 指到本檔）**：
> `ChannelDataStale`、`ChannelFetchLatencyHigh`、`ChannelHealthStatusError`（4 條，依探測週期分 5m／1h／6h／24h）、
> `ChannelHighErrorRatePerChannel`。
> 規則檔：`monitoring/rules/channel_health_latent_staleness.yml`（6 處）＋
> `monitoring/rules/wave9_channel_individual_health.yml`（1 處）。
> 指標由背景任務 `channel_health_metrics_export`（5 分鐘一輪）從
> `data/state/channel_health.json` 匯出。
>
> **建立背景（2026-10-16）**：這 7 個 `runbook_url` 原本指向
> `https://wiki.internal/runbooks/...`，而 `wiki.internal` 是 **DNS NXDOMAIN**
> ⇒ 事件當下點進去是死的（生產實證見 §9）。

## 1. 這個家族量的是什麼

資料流只有一條（三個消費者必須共用同一個判定，見
[channel-health-status-single-truth-spec.md](../specs/channel-health-status-single-truth-spec.md)）：

```
adapter 抓取 → ChannelHealthStore.Record 寫 data/state/channel_health.json
      → apigateway.DeriveChannelStatus(record, contract, now)   ← 唯一判定點
      → channel_health_metrics_export（5m）匯出 4 個 gauge
      → Prometheus 規則（本檔的 4 條）
```

匯出的 gauge（`cmd/atlas/channel_health_metrics_task.go`）：

| gauge | 意義 | 讀它的規則 |
|---|---|---|
| `atlas_channel_health_status` | 判定後狀態：0=ok、1=warn（含 stale／degraded）、2=error、3=inactive／retired、4=other | `ChannelHealthStatusError`（**只匹配 `== 2`**） |
| `atlas_channel_staleness_overage_seconds` | 資料陳舊**超出契約窗**的秒數（窗內 = 0，永遠輸出） | `ChannelDataStale`（`> 0`） |
| `atlas_channel_data_staleness_seconds` | 原始資料齡（人可讀補充；**沒有規則拿它做判定**） | 無 |
| `atlas_channel_fetch_latency_seconds` | 最後一次抓取耗時（秒；`LatencyMs > 0` 才有序列） | `ChannelFetchLatencyHigh`（`> 90`） |
| `atlas_channel_consecutive_failures` | 連續失敗次數（`> 0` 才有序列） | 無（給人看「正在往 error 升級」） |
| `atlas_channel_health_errors_total` | 健康檢查每輪對 `status != "ok"` 的通道加 1 | `ChannelHighErrorRatePerChannel`（`rate(...[5m]) > 0.1`） |

三個**必須先知道**的語意（否則會把正常當故障）：

1. **`overage` 是契約感知的，不是牆鐘差**。`ChannelContract.StalenessOverageSeconds`
   對宣告 `PublishCalendar=tw_trading_day` 的通道改用**交易日曆**（上游只在交易日發布
   ⇒ 週末／連假不算過期），未宣告的通道維持原牆鐘語意。
2. **`overage` 永遠輸出**（窗內寫 0）。若「條件消失就跳過輸出」，`MetricsCollector`
   的 last-write-wins gauge 會把舊樣本永久凍結 ⇒ 告警永遠 firing 不會 resolve
   （2026-09-05 實證；改動前請先看 §9 的「不可回退」）。
3. **known-issue 與 derived 通道不輸出 staleness／latency**。`monitoring/known_issues.go`
   登記的通道（上游已永久移除／退役）與 `provenance=derived` 的衍生記錄只保留
   `atlas_channel_health_status`，其餘 gauge 不出序列 ⇒ **看不到 `overage` 不等於健康**。

## 2. 共同診斷（任何一條先做這三步）

```bash
# 1. 目前值（全部通道；先看哪個 channel label 在動）
docker exec atlas-go curl -s localhost:18080/metrics \
  | grep -E 'atlas_channel_(health_status|consecutive_failures|staleness_overage_seconds|fetch_latency_seconds|health_errors_total)'

# 2. 這個通道的原始記錄（last_error 是抓取路徑寫的原文）
docker exec atlas-go grep -o '"<channel>":[^}]*}' /app/data/state/channel_health.json

# 3. 判定後的人可讀視圖（與 gauge 同一個函式；known_issue badge 也在這裡）
curl -s -H "X-API-Key: $ATLAS_API_KEY" http://localhost:18080/api/dashboard/channel-health | head -c 2000
```

**第 2、3 步的差異就是本家族最常見的誤判來源**：檔案裡是「最後一次寫入的狀態」，
gauge 與 API 是「現在重新判定後的狀態」。只看檔案會讀到凍結值。

## 3. `ChannelDataStale`（warning；`overage > 0` 持續 15m）

**症狀**：`atlas_channel_staleness_overage_seconds{channel="X"} > 0` 持續 15 分鐘。

**影響**：該通道的下游計算會用到過期資料（不是服務中斷；warning 不 page）。

**判讀順序**：

1. 看**超出多少**：`overage` 1–2 小時多半是排程漂移或上游晚發布；`overage` 以「天」計
   才是真的斷。
2. 排除**契約／曆法**造成的假陽性：
   - 通道宣告 `PublishCalendar=tw_trading_day` ⇒ 週末與連假的過窗由曆法吸收，
     若仍在週末 firing，先確認 `internal/taiwanholidays` 有沒有漏掉該假日。
   - 上游資料時間戳**合法落後**抓取時間的通道（實證：`us10y`／`vix` 的 data-as-of 落後約 27h）
     ⇒ 契約窗要涵蓋這個落差，否則日日誤報。
3. 確認**排程有沒有跑**：`docker logs atlas-go --since 24h 2>&1 | grep -iE 'channel_health|<channel>'`。
4. 確認**上游是否停用**：若上游報告已下架／更名，走 `monitoring/known_issues.go` 的
   known-issue 或退役路徑（**不要**用放寬窗口或 silence 掩蓋；#2138 的要求是「有替代輸入就退役或修」）。
5. **已知的假樣本形狀**（2026-09-05 已修，僅供辨識舊版本部署）：`overage` 不再輸出 0
   ⇒ `> 0` 永久 firing 且重啟後仍在。若看到「值與 `overage=0` 條件同時存在」，先確認部署版本。

**緩解**：`overage` 只反映資料齡，**沒有東西可以「清掉」**。要嘛補資料（後端排程／backfill），
要嘛修契約窗／曆法，要嘛把通道退役。三種都不是 silence。

**升級**：`overage > 1 天`、或同一輪多個通道同時過窗（共因：資料源或排程器）
⇒ 依「本 repo 的告警 → 值班」升級路徑，並在
[local-deploy.md](local-deploy.md) 的重啟／回滾程序前先確認不是資料源問題。

## 4. `ChannelFetchLatencyHigh`（warning；`> 90s` 持續 15m）

**症狀**：`atlas_channel_fetch_latency_seconds{channel="X"} > 90`。

**影響**：抓取變慢會延後資料落地；持續變慢會連帶觸發 §3。

**判讀**：

1. 90 秒的門檻是**降噪後**的值（2026-09-03）：`day_trading`／`market_volume`／
   `geopolitical_taiwan` 這類**多來源外部抓取**，30–50 秒是常態而非故障。
   所以先問「這個通道是不是多來源抓取」——是的話 90–150 秒仍屬可接受。
2. 區分三種原因（`last_error` 通常會直說）：
   - **上游 API 變慢**：同一個通道的其他來源正常 ⇒ 等或換來源。
   - **rate limit**：錯誤字串含 429／quota。
   - **網路不穩**：容器內直接測該來源（照 `wave9-runbook.md` §3.3 的 us_yahoo 測法）。
3. 只有 `LatencyMs > 0` 才有序列 ⇒ **序列消失**代表該輪沒有抓取（可能是 known-issue／
   退役通道），不是「延遲歸零」。

**緩解**：降載（加大間隔／縮小來源集合）或修來源；**不要**直接拉高門檻，
除非同時更新 `monitoring/rules/` 內的理由註解（門檻與理由必須一起改）。

## 5. `ChannelHealthStatusError`（**error，會 page**；`status == 2`）

**症狀**：`atlas_channel_health_status{channel="X"} == 2`，持續時間依探測週期
（5m 週期等 5m、1h 等 1h、6h 等 6h、24h 等 24h）。

**這條與 §3／§4 的關係**：`ChannelDataStale` 是「資料舊」，本條是「通道壞」。
`degraded`（抓取成功但 payload 空／舊／殘缺）在**超過契約窗之後會升級成 `error`**
（E29-3）⇒ 同一根因可能同時點亮兩條，先修本條。

**判讀**：

1. `consecutive_failures` 與 `last_error`：連續失敗 ≥ 契約窗的代表結構性損壞。
2. 檢查該通道是否**應該**被排除／退役：第一條規則的 matcher 是「不在排除清單內」，
   排除清單（`exchange_rate|finmind|fubon|fugle|geopolitical_taiwan|taiex_index|taifex_daily|tsmc_revenue|twse_insider|twse_replay`）
   與其 `for` 值是**校驗配對**的（`cmd/check-channel-consistency` 會比對排程週期）；
   改 matcher 或 `for` 必須同時改另一邊。
3. `status == 2` 但**沒有告警**：檢查 gauge 是否還在輸出（`grep '^atlas_channel_health_status'`）。
   若整個 family 消失，代表 `channel_health_metrics_export` 沒在跑 ⇒ 先修任務，
   不要從「沒有告警」推論「通道健康」。
4. **不要**用 `status` 的原始值代替判定：`DeriveChannelStatus` 才是 SSOT（rule 0 契約退役 →
   rule 1 passthrough → rule 2b degraded 升級 → rule 3 stale → rule 4 空時間戳視為 ok）。

**緩解**：修抓取路徑（憑證／來源／schema）為正解。**回滾**：若為部署引入，
走 [local-deploy.md](local-deploy.md) 的回滾程序並確認回滾後 gauge 重新輸出。
退役路線（有替代輸入時）見 `monitoring/known_issues.go` 的治理欄位
（`UpstreamRemovedAt`／`ReplacementInput`／`ActionBy`／`RetiredAt`）。

## 6. `ChannelHighErrorRatePerChannel`（info；`rate(...[5m]) > 0.1`）

**症狀**：某通道 5 分鐘錯誤率 > 0.1/s。這是**早期訊號**（`severity: info`，不 page）。

**為什麼會單獨存在**：它在 Wave 9.1 定義、2026-07-05 才啟用（PR #950），
`wave9-runbook.md` §3.3 是它的原始 triage（以 `us_yahoo` 為例：rate limit、
網路不穩、circuit breaker 三種原因，由高到低機率）。

**判讀**：與 §5 同一組證據（`last_error`、`consecutive_failures`）；
本條與 §5 的差別只有「嚴重度」與「是否已過契約窗」。

## 7. 重啟 / 回滾（共同）

- 指標本身不落盤，**重啟 atlas-go 會清空 app 指標家族**，直到 `channel_health_metrics_export`
  下一輪（≤5 分鐘）才重新輸出 ⇒ 重啟後馬上看 `/metrics` 得到「空」是預期行為，
  不是修好了也不是壞掉了。
- 容器重啟、程式回滾與部署驗證：見 [local-deploy.md](local-deploy.md)（含 Mac Mini production 實走路徑）。
- Wave 9 自體（detector／事件流）的緊急處置：見 [wave9-runbook.md](wave9-runbook.md) §8。

## 8. 升級路徑

1. 值班先做 §2 三步，把 `channel`、`overage`／`latency`／`status` 現值、`last_error` 原文記下來。
2. 通道層級：`internal/apigateway` 的抓取路徑擁有者。
3. 上游永久不可用：走 `monitoring/known_issues.go` 的治理判準（退役或修，期限由 `ActionBy` 管）。
4. 指標家族整體消失或規則沒載入：`internal/monitoring` ＋ `monitoring/`（權威樹；
   `scripts/ci/check_monitoring_single_source.sh` 會擋第二棵樹）。

## 9. 生產實證（2026-10-16，唯讀）

| 查詢 | 結果 |
|---|---|
| `count(atlas_channel_health_status)` | `46` 條序列（家族活著） |
| `atlas_channel_staleness_overage_seconds > 0` | `{channel="tej"} 6930.9`（約 1h55m 過窗） |
| `atlas_channel_health_status == 2` | 空集合（沒有通道處於 error） |
| `ALERTS{...}` | `ChannelDataStale{channel="tej"}` **firing** |
| 該 firing 告警的 `runbook_url` annotation | `https://wiki.internal/runbooks/channel-data-stale`（**NXDOMAIN**） |

最後一列就是本 runbook 的存在理由：事件當下值班點到的連結是死的。

## 10. 已知限制（刻意列舉）

- 本檔涵蓋的 4 條規則**不含** `atlas_channel_governance_overdue`（#2138 的治理訊號，
  目前**沒有任何規則**讀它，見 `cmd/atlas/channel_health_metrics_task.go`）。
- `monitoring/tests/*_test.yml` 的 `runbook_url` **不在** `scripts/ci/check_monitoring_single_source.py` 的 R4 範圍內
  （R4 只掃 `monitoring/rules/`）。⚠️ **但 promtool `test rules` 會逐字比對 alert 的 annotation（含 `runbook_url`）**
  —— 有 fixture 的家族，改 URL 必須與 fixture 同 commit 同步，否則 RC=1
  （2026-10-19 在 replay 家族實測；本家族現況**沒有** fixture）。
- `overage` 的曆法判定依賴 `internal/taiwanholidays` 的表；新假日未入表時，
  該通道的過窗判定會偏移一天（長假後第一個交易日請人工對照交易所行事曆）。
- 本檔的門檻數字（`> 0`、`> 90`、`== 2`、`> 0.1`）**以規則檔為準**；規則改了本檔就要跟著改。

## 11. 相關文件

- [wave9-runbook.md](wave9-runbook.md) — Wave 9 observability 操作手冊（§3.3 是 `atlas_channel_health_errors_total` 的原始 triage）
- [channel-health-status-single-truth-spec.md](../specs/channel-health-status-single-truth-spec.md) — 「狀態只有一個判定點」的規格
- [local-deploy.md](local-deploy.md) — 部署／重啟／回滾
- [traps.md](../reference/traps.md) — 跨模組陷阱
