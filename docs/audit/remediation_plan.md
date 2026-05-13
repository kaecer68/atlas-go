# 數據源憲法整改計畫

**日期**: 2026-05-13  
**狀態**: 進行中  
**總違規數**: 97 處（清查報告見 `docs/audit/constitution_violations_2026-05-13.md`）

---

## 整改優先級

### 🔴 P0: 立即整改（高風險）

| # | 文件 | 違規類型 | 整改方式 | 狀態 |
|---|------|---------|---------|------|
| 1 | `cmd/atlas/main.go` | env_direct | API Key 改為 Gateway 注入 | ⏳ |
| 2 | `internal/marketdata/*.go` (6個) | http_direct | HTTP Client 統一管理 | ⏳ |
| 3 | `internal/bootstrap/background.go` | goroutine_direct | 改用 BackgroundTaskManager | ⏳ |

### 🟡 P1: 短期整改（中風險）

| # | 文件 | 違規類型 | 整改方式 | 狀態 |
|---|------|---------|---------|------|
| 4 | `internal/narrative/*.go` (3個) | http_direct + goroutine | 改用 Gateway | ⏳ |
| 5 | `internal/monitoring/notifier.go` | http_direct | HTTP Client 統一管理 | ⏳ |
| 6 | `internal/live/*.go` | http_direct | HTTP Client 統一管理 | ⏳ |

### 🟢 P2: 中期整改（低風險）

| # | 範圍 | 違規類型 | 整改方式 | 狀態 |
|---|------|---------|---------|------|
| 7 | 47 處 provider_direct | provider_direct | 逐步遷移至 Gateway | ⏳ |
| 8 | 散落式 config 讀取 | config_direct | 統一到 Gateway | ⏳ |

---

## 整改原則

1. **漸進遷移**: 不刪除舊代碼，先加 `// Deprecated:` 標記
2. **功能優先**: 確保整改過程中系統功能不受影響
3. **測試驅動**: 每處整改後執行 `go test`
4. **文檔同步**: 更新 `docs/data_sources.md`

---

## 驗收標準

- [ ] `go build ./...` 通過
- [ ] `go test ./...` 通過
- [ ] `go vet ./...` 通過
- [ ] `gofmt -l .` 無輸出
- [ ] `grep -r "os.Getenv.*API_KEY" --include="*.go" .` 無違規（白名單除外）
- [ ] `grep -r "go func" --include="*.go" internal/` 數量減少 50%+
