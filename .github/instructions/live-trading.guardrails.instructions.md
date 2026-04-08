---
applyTo: "internal/live/**,cmd/atlas/**"
description: "適用於 atlas-go 的 live trading 協調路徑修改。強制 replay 優先安全策略、明確 TODO 邊界處理，以及保守風控整合變更。"
---

# Live Trading 守則

## 適用範圍

套用於 live orchestration 程式碼與 atlas 執行入口。

## 目前可靠性邊界

- 將 replay 與 simulation 路徑視為可靠預設。
- 除非任務明確要補齊，否則假設 internal/live 仍有部分整合 TODO。

## 變更規則

- 若沒有測試證據，不得宣稱與 replay/sim 功能對等。
- 維持 control 層意圖清晰：風險過濾與執行順序必須可稽核。
- 優先小範圍、隔離式變更，並加入明確 feature flag 或 guard 條件。
- 若觸及下單執行邏輯，需保留市場資料缺失時的 fail-safe 行為。

## 驗證清單

```bash
go test ./internal/live/...
go test ./internal/orchestrator/...
go test ./internal/sim/...
```

手動 smoke check：

```bash
go run ./cmd/atlas
```

## 參考文件

- `AGENTS.md`：架構邊界與常見陷阱
- `docs/operations-playbook.md`：操作流程期待
- `docs/architecture.md`：執行流程與分層邊界
