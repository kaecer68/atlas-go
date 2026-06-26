# AGENTS.md — internal/logging

**成熟度**: stable
**模組職責**: 統一結構化日誌介面，包裝 `log/slog` 並支援 context 傳播與欄位輔助函式。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|
| 型別 / 符號 | 檔案 | 功能 |
|------|------|------|
| `logger`（package-level） | `logger.go` | 底層 `*slog.Logger`，`mu` 保護 |
| `ctxKey` | `context.go` | context 中儲存 logger 的鍵型別 |
| `Component` / `Event` / `Symbol` / `SessionID` / `AgentID` / `DurationMs` | `logger.go` | 結構化欄位輔助函式，回傳 `slog.Attr` |
| `FStr` / `FInt` / `FFloat64` / `FBool` | `logger.go` | 泛用 slog 欄位輔助函式 |
| `Err` | `logger.go` | 包裝 error 為 slog 欄位，`nil` 時回傳 `nil` |

## 資料流

```
init() → text handler on stderr (Info level)
Init() → 可切換為 JSON handler 或調整 level
WithLogger(ctx, l) → context 傳播
FromContext(ctx) → 取出 logger（fallback 至 slog.Default()）
```

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **絕不回傳 nil logger** | `FromContext` 找不到時 fallback 至 `slog.Default()`，保證可用 |
| **Info/Error 非 context-aware** | 預設函式讀取 package-level logger，要用 context 請用 `InfoContext`/`ErrorContext` |
| **Critical 使用自訂 level 12** | 高於 Error 但非標準 slog level，外部解析器可能不認識 |
| **Err(nil) 回傳 nil** | 不會產生 `"error":"nil"` 字串，直接無操作 |
| **SetLogContext 為 package-level** | 影響全域，非 per-request；per-request 請用 `WithLogger` |
| **LegacyLog 僅供相容** | 輸出為 info level，欄位名為 `component` + `message`，新程式碼避免使用 |

## 測試

- `go test ./internal/logging/...`
- `logging_test.go`：context 傳播、欄位輔助、nil 安全測試
