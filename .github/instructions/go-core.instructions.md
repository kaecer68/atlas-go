---
applyTo: "internal/**/*.go,cmd/**/*.go"
description: "適用於 atlas-go 的 Go 程式碼修改。涵蓋 import 分組、錯誤包裝、介面邊界與 internal/cmd 套件的測試優先驗證。"
---

# Go 核心守則

## 適用範圍

修改 internal/ 與 cmd/ 下的 Go 程式碼時套用。

## 程式撰寫規則

- 介面保持小而聚焦，優先沿用既有 executor 風格：`Supports` 加上一個操作方法。
- 優先使用 early-return 流程以降低巢狀層級。
- 回傳錯誤請補上操作脈絡，使用 `fmt.Errorf("operation: %w", err)`。
- import 分組維持：標準庫、第三方、內部模組。
- 既有 enum 與領域狀態維持字串型別。

## 安全規則

- 除非任務明確包含 migration，否則維持既有公開 API 簽名。
- 避免跨層洩漏：domain 型別留在 internal/domain；協調邏輯留在 internal/orchestrator。
- 不要引入全域可變狀態做執行期協調。

## 測試規則

- 新增或更新測試時，使用同目錄同 package 的 *_test.go。
- 優先跑聚焦套件測試，再視需要擴大測試範圍。

## 驗證清單

```bash
# 格式檢查
test -z "$(gofmt -l .)"

# 聚焦測試（依變更套件調整）
go test ./internal/orchestrator/...
go test ./internal/sim/...
```

若影響範圍較大：

```bash
go test ./...
```

## 參考文件

- `agents.md`：倉庫層級邊界與預設指令
- `internal/apigateway/CONSTITUTION.md`：數據源治理規範 — 禁止直接 `os.Getenv`/`&http.Client{}`，強制 Gateway 模式
- `docs/PARAMETER_SYSTEM.md`：參數管理系統 — 禁止硬編碼 magic number，強制使用 `ParametersConfig`
- `docs/architecture.md`：分層設計原則
- `docs/ai_agent_architecture.md`：代理協調細節
