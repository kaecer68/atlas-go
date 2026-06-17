# Generated Skills — 程式碼參考索引

此目錄包含 **21 個自動生成**的技能檔案，對應 `internal/` 下的 Go 模組。所有內容由程式碼符號索引工具自動產生，**不含領域知識或決策指引**。

## 來源

這些 SKILL.md 透過程式碼符號分析工具掃描 `internal/` 各模組的 Go 原始碼後自動生成，記錄了：

- 每個模組的關鍵符號（函數、類型、方法）及其所在檔案
- 模組間的依賴關係與呼叫關聯
- 模組內聚度（cohesion）指標

## 使用方式

### ✅ 適用情境：程式碼導航

- 查找某個模組有哪些公開符號
- 確認某個函數/類型位於哪個檔案
- 了解模組間的呼叫關係

### ❌ 不適用情境

這些技能**不應**用於：

- 領域知識查詢（例如「portfolio 模組如何運作」）
- 決策指引（例如「應該使用哪個引擎」）
- AI Coding 時的 context 載入（會產生誤導性建議）

**需要領域知識時，請查閱對應的 `internal/<模組>/AGENTS.md` 或手寫技能。**

## 技能清單

| 技能 | 對應模組 |
|------|----------|
| `apigateway` | `internal/apigateway/` |
| `atlas` | `internal/atlas/` |
| `baseline` | `internal/baseline/` |
| `eventbus` | `internal/eventbus/` |
| `experiment` | `internal/experiment/` |
| `industry` | `internal/industry/` |
| `janus` | `internal/janus/` |
| `ledger` | `internal/ledger/` |
| `live` | `internal/live/` |
| `marketdata` | `internal/marketdata/` |
| `monitoring` | `internal/monitoring/` |
| `narrative` | `internal/narrative/` |
| `orchestrator` | `internal/orchestrator/` |
| `portfolio` | `internal/portfolio/` |
| `prism` | `internal/prism/` |
| `realtime` | `internal/realtime/` |
| `risk` | `internal/risk/` |
| `service` | `internal/service/` |
| `sim` | `internal/sim/` |
| `spawning` | `internal/spawning/` |
| `tax` | `internal/tax/` |

## 載入策略

所有生成技能均標記 `load_policy: "manual_only"`。AI 編碼助手應**跳過**這些技能，除非使用者明確要求進行程式碼導航查詢。

## 維護

當 `internal/` 模組有重大結構變更時（新增/刪除/重新命名檔案、新增/刪除公開符號），應重新執行符號索引工具來重新生成這些檔案。

```bash
# 重新生成所有 generated skills
go run cmd/code-symbol-indexer/main.go --output .claude/skills/generated/
```
