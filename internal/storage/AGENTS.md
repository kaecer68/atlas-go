# AGENTS.md — internal/storage

**成熟度**: stable
**模組職責**: 檔案生命週期管理，依保留政策自動清理過期資料，支援 dry-run 模式。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|
| `LifecycleManager` | `lifecycle.go` | 檔案生命週期管理器 |
| `RetentionPolicy` | `lifecycle.go` | 目錄級清理規則：路徑、天數、檔名模式、排除檔案 |
| `CleanupReport` | `lifecycle.go` | 跨政策彙總清理結果 |
| `PolicyReport` | `lifecycle.go` | 單一政策清理明細 |

## 資料流

```
LifecycleManager.Run(ctx, dryRun)
  → 迭代所有 RetentionPolicy
  → 讀取目錄 → filepath.Match 篩選
  → 排除清單比對（exact match）
  → modTime vs cutoff 判斷
  → 刪除或保留
  → 彙總 CleanupReport
```

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **非正規化寫入後清理** | 本模組不負責寫入時的原子性，僅清理已存在檔案；原子寫入由 `ledger.SessionWriter` 處理 |
| **filepath.Match 為 glob 非 regex** | 模式如 `20*.json` 是 glob 語法，複雜匹配需預先測試 |
| **排除清單為 exact match** | `isExcluded` 做字串完全比對，非 pattern match |
| **首個政策錯誤即中斷** | `Run` 遇到第一個政策錯誤就 return，後續政策不會執行 |
| **刪除錯誤僅印 stderr** | 單檔刪除失敗 `fmt.Fprintf(os.Stderr, ...)`，不影響整體回傳 |
| **Missing directory 回傳 0** | 政策目錄不存在時 graceful 回傳 0 筆，不報錯 |
| **LastReport() 回傳 any** | 需 type assertion 為 `CleanupReport`，無泛型保護 |

## 測試

- `go test ./internal/storage/...`
- `lifecycle_test.go`：標準單元測試
- `lifecycle_integration_test.go`：`//go:build integration`
