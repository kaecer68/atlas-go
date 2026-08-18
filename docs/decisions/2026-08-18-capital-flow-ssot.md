# capital_flow 單一真相源 (SSoT) 決策 — 2026-08-18

> **裁決 (用戶拍板)**: **PG `capital_flow` 表為 SSoT (主)。**

## 背景 (三處儲存現況)

| 儲存 | 位置 | 角色 |
|---|---|---|
| **PG `capital_flow` 表** | internal/repository/postgres_others.go (RecordCapitalFlow/QueryLatestCapitalFlow/QueryCapitalFlowRange), 經 DualWriteRepository 寫入 | **SSoT (主, 本決策)** |
| JSON snapshot | data/state/capital_flow/*.json (T86 三大法人日報), internal/capitalflow/history_import.go LoadT86CapitalFlow | 上游原始導入 (來源層) |
| rolling window JSONL | internal/capitalflow/rolling_store.go (BK-15, date-keyed atomic persistence), 七維錢潮 Z-score 參考 | 計算層快取 (可重建自 PG) |

## 決策內容

1. **SSoT = PG `capital_flow` 表** (用戶拍板, 2026-08-18)
2. **JSON snapshot (T86)** = 上游導入來源, 非 SSoT (只寫入 PG 後即完成使命)
3. **rolling_store JSONL** = 七維錢潮計算的持久化快取, **可重建自 PG** (Reconciler 對帳若發現漂移, 以 PG 修正)
4. **寫入者責任**:
   - 上游 T86 導入 → 寫 PG (現有 RecordCapitalFlow 路徑)
   - 七維錢潮計算 → 讀 PG (QueryCapitalFlowRange) 而非 rolling_store 為準
   - rolling_store 仍可寫 (快取), 但 Reconciler 以 PG 為基準對帳
5. **Reconciler 基準**: capital_flow 對帳以 PG 為權威端

## 影響

- 未來 capital_flow 相關修復/對帳都以 PG 為基準
- rolling_store JSONL 若與 PG 漂移 → 以 PG 重建 (非反向)
- 本決策覆蓋 v4-pro 規劃書「建議 rolling_store 為主」— 用戶裁決優先

## 執行

- 本文件為正式決策記錄; Reconciler v1 的 capital_flow 規則以 PG 為基準
- 若未來 PG 不可用 (fallback 情境), 讀取仍可退 rolling_store, 但標記 degraded
