# 模組成熟度標記系統

所有 `internal/*/` Go package 必須透過 `doc.go` 標記成熟度，並與本文件保持一致。

## 成熟度層級

| Tier | 標記 | 含義 |
|------|------|------|
| S | `stable` | 穩定生產 — API 穩定，breaking change 需 migration plan |
| E | `evolving` | 演進中 — API 可能調整，可能晉升為 stable |
| X | `experimental` | 實驗中 — 研究性質，不應被其他模組依賴 |
| U | `utility` | 輔助工具 — CLI 工具/資料轉換，非 runtime |

## AI Agent 工作流程

### 新建 `internal/` 模組
1. 建立 `doc.go`，加入 `// Maturity: <tier>` 標記
2. 更新 `internal/MATURITY.md`，將新模組加入對應層級的表格
3. 執行 `bash scripts/ci/check_maturity.sh` 確認通過

### 變更成熟度
1. 修改 `doc.go` 中的 Maturity 標記
2. 同步更新 `internal/MATURITY.md`
3. X→E 或 E→S 視為晉升，需 PR review；S→E 或任何降級需 migration plan

### 本地驗證
```bash
bash scripts/ci/check_maturity.sh
go run ./cmd/check-maturity
```

CI 的 `quality.yml` 中 `maturity` job 會在每個 PR 自動執行檢查，不一致會導致 CI 失敗。
