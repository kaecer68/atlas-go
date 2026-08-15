# Sector Allocation Closure — 操作手冊

> **SA11.B dark launch operation runbook**

## Preflight Checklist

1. **Simulation sessions**: 確認 ≥20 valid simulation sessions 完成
2. **Benchmark data**: TAIEX daily returns 已載入 `FileTAIEXBenchmarkProvider`
3. **Legacy reads**: `sac.legacy.read` counter == 0
4. **Negative evidence**: `sa12-negative-evidence.sh` all 0 hits
5. **Mutation count**: Live mutation count == 0

## Promotion Gate

符合 L2.4 5-condition hard gate 模式（`docs/operations/l2-4-runbook.md` §3）：
- ✅ Feature flag `sector_allocation_closure_enabled` 預設 off
- ✅ SACMetrics 11 events 全部觀察中
- ✅ No live mutations during observation
- ✅ Rollback drill 通過
- ✅ Operator sign-off

## Rollback

```bash
# 關閉 feature flag
export SECTOR_ALLOCATION_CLOSURE_ENABLED=false
# 確認 legacy reader 正常運作
curl http://localhost:18080/api/dashboard/sector-allocation-plan
```

## Rollback Drills（SA11.B）

Rollback drills 在 production 觀察期前逐一驗證三個最可能阻斷服務的降級路徑。每次都必須保存原始 snapshot、benchmark 與 feature flag 狀態，演練後清除注入資料，確認沒有 side effect 殘留。可用以下端點確認 handler 與 legacy reader：

```bash
curl -i http://localhost:18080/api/dashboard/sector-allocation-plan
export SECTOR_ALLOCATION_CLOSURE_ENABLED=false
curl -i http://localhost:18080/api/dashboard/sector-allocation-plan
```

### Drill 1：Feature Flag Toggle

1. **觸發條件**：Sector Allocation Closure 新路徑上線後出現 handler `5xx`、response schema 錯誤，或 legacy 輸出已明確可用，需要判斷是否能以 feature flag 立即回滾。
2. **執行動作**：將 `SECTOR_ALLOCATION_CLOSURE_ENABLED` 設為 `false`，重新載入該 runtime 設定，再呼叫 sector-allocation-plan endpoint；記錄切換前後的 feature flag 與回應。
3. **驗收標準**：flag off 時 endpoint 由可重現的 `503`／錯誤回應恢復為 `200`，legacy `BaseWeights` 正常回傳且沒有 dependency 錯誤；確認後恢復原 flag 狀態。

**對應 rollback 情境**：新 closure reader、parser 或 service dependency 故障，必須停止讀新路徑並立即回到 legacy reader。

### Drill 2：Snapshot Data Corruption

1. **觸發條件**：需要確認 snapshot 無法解析或內容損毀時，新路徑會 fail closed，而不會把 corrupt data 當成有效配置。
2. **執行動作**：在受控環境將 sector allocation snapshot 替換為截斷或無效內容，保持 feature flag 開啟並呼叫 endpoint；完成後暫時關閉 flag，獨立確認 legacy reader。
3. **驗收標準**：新路徑回傳 `503` 且錯誤碼為 `snapshot_unavailable`；legacy `BaseWeights` endpoint 仍為 `200`。恢復原 snapshot 與 flag 後，相同查詢不再出現 `snapshot_unavailable`。

**對應 rollback 情境**：磁碟損壞、寫入中斷或 snapshot parser regression；rollback trigger 是 corrupt snapshot 已可能污染新路徑輸出，而不是等待一般 API 5xx 累積。

### Drill 3：TAIEX Data Missing

1. **觸發條件**：benchmark provider 暫時無資料或 TAIEX daily returns 未載入，需要驗證 warming-up 降級行為。
2. **執行動作**：在受控環境移除或遮蔽 benchmark data，呼叫 sector-allocation-plan endpoint；保存此次 response，再恢復資料並重查。
3. **驗收標準**：endpoint 為 `200`，回應 `status=warming_up`，`ranking` 為空 ranked list（`[]`），不可回傳上一期排名，也不可因缺資料產生 `5xx`；恢復資料後 response 能回到正常路徑。

**對應 rollback 情境**：TAIEX benchmark 中斷使新排名無法形成；warming-up response 是安全降級，若無法在服務恢復後回到正常路徑才升級為 rollback 事件。

### 與 dark launch 觀察期銜接

- 三個 drills 必須在 promotion 前、feature flag 預設 off 且 rollback 指令可用的 staging／受控 production 環境逐項通過；任一失敗都先保持 flag off，不得進場。
- 將日期、三個 trigger、執行動作、HTTP status／error code、`ranking` 長度與 legacy `BaseWeights` 結果寫入 SA11.B 私有 observation log。drill 產生的 snapshot、benchmark 變更與 flag 狀態必須復原。
- 通過後才進入至少 20 個 valid simulation sessions 的 dark launch 觀察期；期間持續檢查 11 個 `SACMetrics` events、`sa12-negative-evidence.sh` 為 0 hits，且 live mutation count 為 0。
- 觀察期間若任一已驗證情境回歸，依 Drill 1 立即關閉 flag 回到 legacy reader；修復後先重跑受影響 drill，再累積新的有效 sessions，不可直接 promotion。
- promotion gate 的 rollback drill 必須為「本輪」結果；由其他版本、staging 或未記錄 side effect 的舊紀錄不可替代。
