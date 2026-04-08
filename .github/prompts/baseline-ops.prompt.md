---
description: "為 atlas-go 準備安全的 baseline promote 或 revert 決策，包含檢查、證據摘要與精確下一步指令。"
name: "Baseline 操作決策"
argument-hint: "動作與範圍，例如 promote 最新實驗或 revert 到上一版 baseline"
agent: "agent"
---

你正在處理 atlas-go 的 baseline 操作。

任務：
- 將使用者需求判定為 promote、revert 或 assess-only 其中之一。
- 在建議任何會改變狀態的指令前，先執行最小安全檢查。
- 產出可稽核且明確標示風險的決策摘要。

守則：
- Baseline 優先：決策前先確認 baseline policy 狀態可讀。
- 優先依賴 replay/simulation 證據，不靠假設。
- 若證據不足（視窗稀疏、缺 outcomes、缺實驗結果），應阻擋 promote 並回報缺哪些資料。
- 信心不足時，建議採保守決策。

建議檢查順序：
1. 確認目前 baseline 與 data/state 內最近的實驗產物。
2. 執行與 baseline/experiment 流程相關的聚焦驗證指令。
3. 若檢查通過且證據充分，只提出一個精確的下一步指令。

預設驗證指令：
```bash
go test ./internal/baseline/...
go test ./internal/experiment/...
go test ./internal/evolution/...
```

若懷疑影響 orchestration：
```bash
go test ./internal/orchestrator/...
go test ./internal/sim/...
```

輸出格式：
- Requested action: promote | revert | assess-only
- Evidence found
- Blocking risks（若有）
- Decision: proceed | hold
- Exact next command
- Rollback note

若動作是 promote，補上一段簡短 why-not-revert 說明。
若動作是 revert，補上一段簡短 blast-radius 說明。
