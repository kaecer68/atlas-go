---
applyTo: "internal/live/**,cmd/atlas/**"
description: "適用於 atlas-go 的 live trading 協調路徑修改。強制 replay 優先安全策略、明確 TODO 邊界處理，以及保守風控整合變更。"
---

# Live Trading 守則

## 適用範圍

套用於 live orchestration 程式碼與 atlas 執行入口。詳細套件結構、職責、反模式見 `internal/live/AGENTS.md`。

## 目前可靠性邊界

- 將 replay 與 simulation 路徑視為可靠預設
- 除非任務明確要補齊，否則假設 `internal/live` 仍有部分整合 TODO

## 變更規則

- 若沒有測試證據，不得宣稱與 replay/sim 功能對等
- 維持 control 層意圖清晰：風險過濾與執行順序必須可稽核
- 優先小範圍、隔離式變更，並加入明確 feature flag 或 guard 條件
- 若觸及下單執行邏輯，需保留市場資料缺失時的 fail-safe 行為

## 驗證清單

```bash
go test ./internal/live/... ./internal/orchestrator/... ./internal/sim/...
go run ./cmd/atlas                        # 手動 smoke check
go run ./cmd/experimental/validate-broker # Broker 簽名格式驗證
```

## 參考文件

- `internal/apigateway/CONSTITUTION.md`：Live trading 中所有外部 API 必須透過 Gateway（6 條憲法 + 3 附錄：統一入口、限流、熔斷、背景任務排程、環境變數治理）
- `AGENTS.md`：倉庫層級邊界、21 模組路由、關鍵跨模組陷阱
- `docs/REFERENCE/TRAPS.md`：高危陷阱完整參考（含 live trading 安全旗標陷阱）
- `docs/operations_playbook.md`：操作流程期待
- `docs/architecture.md`：執行流程與分層邊界
- `internal/live/AGENTS.md`：套件內部約定、ANTI-PATTERNS、KEY TYPES
