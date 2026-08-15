---
applyTo: "internal/experiment/**,internal/evolution/**,internal/baseline/**,cmd/run-experiment/**,cmd/judge-experiment/**,cmd/promote-baseline/**,cmd/revert-baseline/**"
description: "適用於 atlas-go 的實驗執行、評估、baseline 升降級與 mutation 流程調整。強制 baseline 優先、replay 視窗就緒檢查、保守接受邏輯與可稽核變更。"
---

# 實驗流程守則

## 適用範圍

編輯實驗生命週期程式碼時套用：mutation → execute → judge → promote → revert。

## 安全關鍵規則

- **Baseline 優先**：實驗執行與評估前必須先載入 baseline policy
- **可稽核性**：mutation 決策與 promote 歷程需可追溯
- **避免靜默漂移**：若調整接受門檻或 judge profile，需在程式註解或 PR 說明原因
- **控制層順序**：除非有明確需求，control 過濾需維持在上游建議之後

## 資料與視窗就緒檢查

- 評估結果前，先確認 replay windows 存在且可用
- 視窗稀疏期間屬低信度證據，避免用極小樣本去過度調整接受門檻
- Replay 匯入格式維持 JSONL（每行一個 JSON 物件）

## 狀態與 Mutation 紀律

- 不要在多次 simulation run 間重用可變 recommendation slices
- 除非任務包含 migration，否則維持既有公開 struct 與 JSON 欄位名稱穩定
- baseline 與 experiment policy 優先採可回復、可疊加（additive）的調整

## 驗證清單

```bash
go test ./internal/experiment/...
go test ./internal/evolution/...
go test ./internal/baseline/...
```

若變更影響 orchestration 或 simulation 邊界，再跑 `go test ./internal/orchestrator/...` 與 `go test ./internal/sim/...`。若影響範圍較大，最後執行 `go test ./...`。

## 參考文件

- `internal/apigateway/CONSTITUTION.md`：實驗中涉及資料源操作須遵循 Gateway 規範
- `docs/reference/constitution.md`：深度開發憲法 — 涉及 optimizer / portfolio 數學驗證時的必要參考
- `docs/reference/parameter-system.md`：實驗門檻與判斷參數的權威來源
- `docs/operations-playbook.md`：日常 mutation 工作流程
- `docs/evolution-loop.md`：接受門檻與循環機制
- `.omo/audit/2026-06-15-experiment-baseline-report.md`：稀疏資料教訓與門檻調整背景（harness 私有）
- `docs/iteration-playbook.md`：mutation 策略模式
- `AGENTS.md`：倉庫層級建置/測試與架構邊界
