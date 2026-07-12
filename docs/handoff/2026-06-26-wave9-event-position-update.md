# Wave 9 — EventPositionUpdate 生產發布（PR #696）

> **文件角色**：Wave 9 功能發布 / 交接紀錄。  
> **來源**：原記載於 `internal/live/AGENTS.md`，因屬 PR 級發布說明與下游影響評估，搬遷至此。  
> **狀態**：已於 Wave 9（PR #696）發布並上線。

## 變更摘要

`internal/live/orchestrator.go` 成為 `EventPositionUpdate` 的生產呼叫者之一。此事件原本只在模擬/回測路徑產生，Wave 9 將其延伸至實盤編排器，以驅動 `BaselineTrigger` 與 `DriftDetector` 等觀測元件。

## 發布點

- 位於 `setupEventHandlers()` 中對 `EventMarketSnapshot` 的 **critical subscription**（`SubscribeCritical`）。
- 當收到市場快照且 `stateStore` 中存在對應部位時，呼叫：

  ```go
  o.eventBus.PublishPositionUpdate(payload.Symbol, position, "updated")
  ```

- 該 handler 同時更新部位市價並檢查停損/停利（當 `StopLossEnabled` / `TakeProfitEnabled` 為 true）。

## `changeType` 語意

| 值 | 意義 | 使用場景 |
|----|------|---------|
| `"added"` | 新增部位 | 模擬/回測推薦初次建立部位 |
| `"updated"` | 部位已存在，價格或數量更新 | live orchestrator 每次市場快照 |
| `"removed"` | 部位已平倉或清空 | 模擬/回測結算；`DriftDetector` 會將其從 snapshots 移除 |

> 注意：live orchestrator 目前只發布 `"updated"`；新增/移除部位的發布由模擬引擎與訂單成交路徑負責。

## 下游訂閱者

- `internal/baseline/trigger.go` — `BaselineTrigger` 執行期政策評估。
- `internal/monitoring/service/drift_detector.go` — v1/v2 drift 偵測累積 `symbol → snapshot`。

## 相關規則

`internal/live/AGENTS.md` 保留的決策性規則：

> live orchestrator 透過 `EventPositionUpdate` 發布部位更新，下游為 `BaselineTrigger` 與 `DriftDetector`。詳見 `docs/handoff/2026-06-26-wave9-event-position-update.md`。
