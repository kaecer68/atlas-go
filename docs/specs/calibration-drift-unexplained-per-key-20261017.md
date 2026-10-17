# `drift_unexplained` 的**逐 key** 可觀測性（2026-10-17）

> **一句話**：`drift_unexplained` 那則 WARN 的**內容**（哪一個參數 key、為什麼）
> 原本只存在於日誌，Prometheus 看不到；本票把它變成
> `atlas_calibration_drift_unexplained_total{artifact,key,reason}` 這個 counter。
>
> **性質**：只新增指標（純觀測面）。**未**改任何判定、門檻、告警條件或風控行為。

## 1. 根因（實測，不是推論）

* WARN 只在日誌裡：`cmd/atlas/calibration_freshness_metrics_task.go:147`
  `logging.Warn("calibration_drift", "drift_unexplained", …)`，
  生產每 5 分鐘一則（`2026-10-17T06:27:05Z` 起，`drift_unexplained
  … out_of_window_keys=darwinian.risk_volatility_threshold.value`）。
* **稽核敘述「沒有 rule／alert」不成立**：`monitoring/rules/calibration_freshness_alerts.yml`
  第 4 條 `CalibrationEffectiveDriftUnexplained` **已存在**（#2076 落地），
  而且生產 Prometheus 此刻是 **firing**（`/api/v1/rules`：`state=firing`，
  `activeAt 2026-10-17T07:27:20Z`；Alertmanager `/api/v2/alerts` 同一筆 active）。
* 真正的缺口是**粒度**：既有 gauge 只回答「**幾個** key」
  （`atlas_calibration_drift_out_of_window_keys`、`…_dropped_keys{reason}`），
  而「**哪一個** key」只在日誌裡 ⇒ 值班被叫醒後仍要 `docker logs … | grep` 才知道對象。

## 2. 新增的序列

| 序列 | 型別 | 標籤 | 語意 |
|---|---|---|---|
| `atlas_calibration_drift_unexplained_total` | **counter** | `artifact="parameters_overlay"`、`key="<參數路徑>"`、`reason="out_of_window" \| "not_applicable"` | 每一輪匯出（5m）只要該 key 仍「無法解釋」就 **+1**，與「每一輪寫一則 `drift_unexplained` WARN」一一對應 |

* 條件與 WARN **完全同一份判定**（同一個 `CalibrationDriftObservation`：
  `OutOfWindowKeys` / `UnknownKeys`），所以日誌與序列不會說不同的話。
* `reason="ssot_moved"`（受審查的憲章編輯優先，**設計行為**）**不計數**。
* `drift_run_ok == 0`（基準讀不到／overlay 解不開）時**不輸出**：
  那時「無法解釋」是**未知**，不是事實（fail-closed，與同族其他序列同契約）。
* **目前有幾個 key** 仍請看 gauge（counter 單調遞增，不可讀成現況數量）；
  想知道「最近有沒有出現」用 `increase(atlas_calibration_drift_unexplained_total[15m]) > 0`。
* 為何是 counter 而非 gauge：它替代的是**事件**（一則 WARN），不是狀態；
  狀態已由 `…_out_of_window_keys` / `…_dropped_keys` 兩個 gauge 表示。

實作：`internal/monitoring/calibration_effective.go`
（`MetricCalibrationDriftUnexplainedTotal` + `recordCalibrationDriftUnexplained`），
在 `emitCalibrationDriftGauges` 內與 gauge 同一輪輸出。

## 3. 送達通道（**本票不改這裡**）

本 repo 目前**刻意沒有**以 Prometheus rule 讀這個 counter。告警／通知的接線在
**Mac Mini 的 `~/bin/notify.sh` watcher**，由 **a2a-dev** 擁有與接線：

```
--event  atlas.drift_unexplained
--source atlas-calibration
--severity warning
```

並由 watcher 內建 **6h 去重**。⇒ 本 repo 的責任只到「把事實變成可查詢的序列」與
「在文件寫明這條通道與 event 名」；**watcher 本身不在本 repo**，也不由本票修改。

## 4. 測試（同 commit）

| 測試 | 釘住的行為 |
|---|---|
| `internal/monitoring/calibration_drift_unexplained_test.go` | ① 標籤 `key`/`reason` 逐 key 出現；② 兩輪 ⇒ 值 2（counter 累加，不是 gauge 覆蓋）；③ 型別是 counter；④ `ssot_moved` 不計；⑤ 乾淨一輪 ⇒ 序列**不存在**；⑥ `run_ok=0` ⇒ 不輸出；⑦ 名稱前綴與 `_total` 後綴 |
| `cmd/atlas/calibration_effective_metrics_task_test.go`（新增 2 個案例） | 端到端 `/metrics`：真基準檔 + 真 overlay ⇒ 兩個 key 各一行（含 `# TYPE … counter`）；乾淨一輪 ⇒ 不得出現 |

## 5. 已知限制

1. `key` 標籤是**參數路徑**（基數 = overlay 的 entry 數，實測 14）⇒ 安全；
   但若未來 overlay 的 key 變成使用者輸入（多租戶），需要重新評估基數。
2. counter 每 5 分鐘 +1 ⇒ 它的**絕對值**沒有意義（只讀 `increase()`／`rate()`）。
3. 這個 counter **不會**讓任何人被通知；通知仍取決於 §3 的 watcher 是否已接線。
4. 舊的 WARN 日誌**保留不動**（日誌是第二條可見路徑，也是 runbook 的 grep 依據）。
