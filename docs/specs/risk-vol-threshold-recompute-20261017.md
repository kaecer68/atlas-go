# 風險門檻 `darwinian.risk_volatility_threshold` 重算驗證（2026-10-17）

> **一句話結論**：生產 overlay 的 **0.012323875595980305** 是
> `internal/calibration` 的 **p75 估計算出來的輸出**（指向 `energy-desk-01` 的日波動，
> 相對差 **1.4e-15**），**不是**手改或被注入的數字。
>
> **性質**：**唯讀**。只讀生產 PostgreSQL（`SELECT`）與唯讀檔（overlay / 日誌），
> 重算在本機執行。**未**寫入 overlay、**未**改任何參數、**未**重啟任何服務。

## 1. 待驗事實（唯讀取得）

| 事實 | 值 | 來源 |
|---|---|---|
| overlay 現值 | `0.012323875595980305` | `docker exec atlas-go grep -n risk_volatility_threshold -A 14 /app/data/state/parameters.calibrated.json` |
| 該 entry 的 `calibrated_at` | `2026-10-17T06:22:58.298457289Z` | 同上 |
| 該 entry 的 `before` / `ssot` | `0.08` / `0.08` | 同上 |
| 前一版 overlay 值 | `0.07265911146663549`（`calibrated_at 2026-10-13T23:38:07Z`） | `docker logs atlas-go --since 6h \| grep overlay_entry_applied`（06:21:14Z 兩筆） |
| SSOT（`configs/parameters.json`） | `0.08`（`configs/parameters.json:313-314`） | repo |
| 產生它的校準報告 | `after=0.012324 method=percentile_based confidence=0.75 n=13` | `docker logs atlas-go --since 6h \| grep -A 40 'completed binary=/app/calibrate-parameters'`（06:22:58Z） |

換算：`0.012323875595980305 / 0.08 = 0.154048`（−83.04% 對前一版；
比值 0.154 落在單步窗 `[1/3, 3]` **之外** ⇒ 這正是 `drift_unexplained` WARN
與 `CalibrationEffectiveDriftUnexplained` 告警的來源，見 §5）。

## 2. 公式：與程式碼逐行對應

生產校準（`/app/calibrate-parameters --module=darwinian`）走
`internal/calibration/calibrate.go` 的 darwinian 分支：

| 步驟 | 程式碼 |
|---|---|
| 樣本來源 | 注入的 ledger store（`darwinian_ledger_source source=injected-store store=*ledger.PostgresLedgerStore`）⇒ `LoadOutcomesFromSessions()` = `SELECT … FROM recommendation_outcomes`（`internal/ledger/postgres_ledger.go:146-160`） |
| 每個 agent 的日報酬序列 | `agentReturns[o.AgentID] = append(…, o.ForwardReturn)`；`ForwardReturn` 由 `metadata` JSON 反序列化（`recommendation.go:61`） |
| 樣本下限 | `len(returns) < cfg.Darwinian.SharpeMinSampleSize.Value ⇒ skip`（`calibrate.go:315`；SSOT `sharpe_min_sample_size.value = 30`，`configs/parameters.json:406-407`） |
| 報酬均值必須為正 | `if mean <= 0 { continue }`（`calibrate.go:323`） |
| 樣本標準差（n−1） | `stdDev = sqrt(Σ(r−mean)² / (n−1))`（`calibrate.go:331`） |
| 年化波動（排序用） | `agentVols = append(…, stdDev*sqrt(252))`（`calibrate.go:335`），`sort.Float64s(agentVols)`（`calibrate.go:341` 同區塊） |
| **百分位指標** | `riskVolRank = int(0.75 × len(agentVols))`；`dailyRiskVol = agentVols[riskVolRank] / sqrt(252)`（`calibrate.go:533-534`） |
| 寫入 | `Method: "percentile_based", Confidence: 0.75, SampleSize: len(agentVols)`（`calibrate.go:600`） |

⇒ 兩個 `sqrt(252)` 相消，**寫出的值就是某一個 agent 的日報酬樣本標準差（n−1）**。
`n = 13` ⇒ `riskVolRank = int(9.75) = 9` ⇒ 取排序後**第 10 小**的那個 agent。

## 3. 重算

### 3.1 用真程式重跑（今天、全量資料）

兩條獨立路徑都得到同一個數字：

| 路徑 | 結果 |
|---|---|
| 生產容器內 `docker exec atlas-go /app/calibrate-parameters --module=darwinian --dry-run`（**唯讀**，10:23:16Z） | `risk_volatility_threshold before=0.080000 after=0.012461 … n=15` |
| 本機 probe：`CalibrateDarwinianWithStore` ＋ 以 SQL dump 建的 stub `OutcomeStore`（同一支函式、同一份 SSOT） | `after=0.012460882961210222 … n=15` |

⇒ **部署中的程式與我重現的邏輯一致**（同一份 `configs/parameters.json`：兩邊
`before` 皆為 `0.080000`、`hit_rate_high before=0.725191`）。

### 3.2 06:22 那一次的樣本集（n=13）⇒ 指標指向誰

今天合格的 agent 是 **15** 個（`n≥30` 且 `mean>0` 且 `σ>0`），其中排序如下
（日波動 = 年化/√252）：

| # | agent | n | 日 σ |
|---|---|---|---|
| 0 | value-yield-01 | 3426 | 0.005901 |
| 1 | super-ack-01 | 2966 | 0.007021 |
| 2 | etf-rotation-01 | 18572 | 0.007836 |
| 3 | consumer-desk-01 | 196 | 0.009336 |
| 4 | mining-desk-01 | 40 | 0.010872 |
| 5 | earnings-quality-01 | 400 | 0.011194 |
| 6 | semi-desk-01 | 400 | 0.011194 |
| 7 | super-asc-01 | 282 | 0.011488 |
| 8 | super-bak-01 | 278 | 0.011570 |
| 9 | industrial-desk-01 | 976 | 0.012217 |
| **10** | **energy-desk-01** | **1944** | **0.012323875595980322** |
| 11 | super-dru-01 | 2266 | 0.012463 |
| 12 | technical-breakout-01 | 7234 | 0.013141 |
| 13 | shipping-desk-01 | 4480 | 0.014740 |
| 14 | electronics-desk-01 | 54 | 0.016391 |

`n=15` ⇒ rank 11 ⇒ `0.012463`（**今天**的答案，與 §3.1 一致）。

報告說 `n=13` ⇒ rank 9。**唯一兩個落在 `30 ≤ n < 100` 這個帶內的 agent 正是
`mining-desk-01`(40) 與 `electronics-desk-01`(54)**；把它們排除後集合恰好是 13 個，
且 `energy-desk-01` 的索引正好是 **9**：

```
rank = int(13 × 0.75) = 9  ⇒  agentVols[9] = energy-desk-01
dailyRiskVol = 0.012323875595980322
```

**與 overlay 的差**：`0.012323875595980305`（寫入值）vs `0.012323875595980322`（重算值）
⇒ 絕對差 `1.7e-17`、相對差 **`1.4e-15`**（浮點累加次序等級；Welford 累加得
`0.012323875595980307`，`math.fsum` 得 `…313`，全部落在 7 個 ULP 內）。
`energy-desk-01` 的最後一筆 outcome 是 `2026-09-26`、該 agent 今天沒有新列
⇒ 它的 σ 在 06:22 與現在**是同一組 1944 個數值**，所以這個比對是**同一份輸入**。

### 3.3 判定

* **是算出來的 ✓**：寫入值 = 該 agent 的樣本標準差，且該 agent 正好是
  `int(0.75×13)=9` 這個指標位置上的元素。要「剛好」撞上這個 15 位有效數字的機率為零。
* **為什麼值變得那麼激進**：p75 取的是一個**單一 agent** 的日波動。今天合格樣本裡
  有 3 個 agent 的 σ 明顯低（0.0059 / 0.0070 / 0.0078，因為它們的 `forward_return`
  多為 ±0 級的小值），p75 位置因此落在 0.0123 —— 與 SSOT 的 0.08（「8%/日 已是極端」，
  `configs/parameters.json:315`）差 **6.5×**。這是**估計量本身**的性質（單點百分位、
  n=13），不是這一輪的計算錯誤。

## 4. 對稽核敘述的三處更正（讀碼／實測結果，非照抄）

1. **樣本來源不是 `stock_signal_outcomes.forward_return`** ✗ —— darwinian 校準讀的是
   **`recommendation_outcomes`**，欄位是 `metadata->>'forward_return'`
   （`internal/ledger/postgres_ledger.go:153-160` 的 `SELECT`，加上
   `scanPGOutcomes` 把 `metadata` 反序列化回 `domain.RecommendationOutcome`）。
   `stock_signal_outcomes` 是 stockpicker 的訊號結果表，**不經過**這條路徑。
2. **`IsUsablePrice`（`close <= 0` 過濾）不影響這個樣本集** ✗ —— 它在
   `internal/calibration/load.go:117`，那是 **replay bars → 日報酬序列** 的路徑
   （garch / var / factor 模組用）。darwinian 只用 ledger outcomes，不經那一支。
3. **「13 個樣本可精確重現」→ 不可行，已改用可重現的近似** ✓ —— 表上沒有
   `created_at`（`\d recommendation_outcomes` 只有 `time`），06:22 之後表內容確實
   變了：同一支程式今天回報的 p75 命中率是 `0.714286`，而 06:22 的報告是
   `0.873016`（同一批 19 個 agent），**且 `SELECT` 只看得到現在的列**。
   因此「哪 13 個」是**集合一致性重建**（§3.2），而「值是那個 agent 的 σ」是
   **逐位元組可比對的事實**（§3.2 尾）。

## 5. 與其他觀測的交叉驗證（同一次部署）

* `drift_unexplained out_of_window_keys=darwinian.risk_volatility_threshold.value`
  （06:27:05Z 起每 5 分鐘一則）⇒ 比值 `0.154` 在單步窗 `[1/3, 3]` 之外，與 §1 的換算一致。
* `CalibrationEffectiveDriftUnexplained` 在生產 Prometheus 為 **firing**
  （`activeAt 2026-10-17T07:27:20Z`，Alertmanager 同一筆 active）
  ⇒ 這個偏離**已經**有規則在看，缺的是「哪個 key」的可查詢性（見
  `docs/specs/calibration-drift-unexplained-per-key-20261017.md`）。

## 6. 重現指令（唯讀；不含任何憑證）

```bash
# 1) 現值（唯讀檔）
ssh kmacmini "/usr/local/bin/docker exec atlas-go grep -n 'risk_volatility_threshold' -A 14 /app/data/state/parameters.calibrated.json"

# 2) 用同一支程式重算（--dry-run：不寫任何東西）
ssh kmacmini "/usr/local/bin/docker exec atlas-go /app/calibrate-parameters --module=darwinian --dry-run"

# 3) 每個 agent 的樣本數與日 σ（唯讀 SQL）
ssh kmacmini "/usr/local/bin/docker exec -i atlas-postgres psql -U atlas -d atlas" <<'SQL'
SELECT agent_id,
       count(*) AS n,
       avg(COALESCE((metadata->>'forward_return')::float8, 0)) AS mean,
       stddev_samp(COALESCE((metadata->>'forward_return')::float8, 0)) AS sd_daily
FROM recommendation_outcomes
GROUP BY agent_id
HAVING count(*) >= 30 AND avg(COALESCE((metadata->>'forward_return')::float8, 0)) > 0
ORDER BY sd_daily;
SQL
```

## 7. 已知限制

1. **歷史列集合不可重建**（無 `created_at`）；§3.2 的 13 個是「與 `n=13` 及
   rank-9 指標同時一致」的重建，不是日誌級的證據。
2. 重算用今天的表 ⇒ 得到的是**今天**的答案（`0.012461`，n=15），與 06:22 的
   `0.012324`（n=13）不同；此差異**來自資料**（§3.1 兩條路徑同值即證明非程式差異）。
3. 本文件**未**主張 0.0123 這個值「合理」；它只是把「是算出來的」證完。
   是否該用 p75-of-13 這種估計量（單點、無穩健化）是**政策**決定，見 FU-20260930-09。
