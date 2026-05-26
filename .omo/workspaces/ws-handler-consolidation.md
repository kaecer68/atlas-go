# Workspace: 小型 handler 套件整併

## 目標

將 `internal/monitoring/api/` 下的小型 handler 套件整併到相近的套件中，減少目錄數量、降低維護成本。

## 整併計畫

| 來源套件 | 端點數 | 整併至 | 理由 |
|----------|--------|--------|------|
| `api/health/` | 3 端點 | `api/system/` | 健康檢查本質上屬於系統狀態 |
| `api/report/` | 3 端點 | `api/pipeline/` | 報告與 pipeline 高度相關 |
| `api/swagger/` | 1 端點 | `api/system/` | Swagger 屬於系統設施 |

## ⚠️ 影響範圍分析（必須先做）

整併 handler 套件會影響：

1. **`internal/monitoring/dashboard_api.go`**：
   - import 路徑變更（3 個 import 改名）
   - 建構式呼叫變更（如 `apihealth.HandleDataIntegrity` → `apisystem.HandleDataIntegrity`）
   - Route 註冊變更

2. **`cmd/atlas/main.go`**：
   - 可能直接引用了被整併的 handler（需確認）

3. **`internal/monitoring/api/` 目錄結構**：
   - 移除 3 個目錄
   - 相關檔案遷移

4. **測試檔案**：
   - 被整併套件的測試需同步遷移

## 執行步驟

### Phase 1: 影響分析（唯讀）

```bash
# 確認所有引用點
grep -rn "api/health" internal/ cmd/ --include="*.go" | grep -v "_test.go"
grep -rn "api/report" internal/ cmd/ --include="*.go" | grep -v "_test.go"
grep -rn "api/swagger" internal/ cmd/ --include="*.go" | grep -v "_test.go"
```

### Phase 2: 逐個整併

#### 2.1 health → system

- 搬移 `api/health/*.go` 到 `api/system/`
- 更新 package 宣告為 `package system`
- 更新所有 import 路徑
- 更新 `dashboard_api.go` 中的引用

#### 2.2 report → pipeline

- 搬移 `api/report/*.go` 到 `api/pipeline/`
- 更新 package 宣告為 `package pipeline`
- 更新所有 import 路徑
- 確認 pipeline handler 中無名稱衝突

#### 2.3 swagger → system

- 搬移 `api/swagger/*.go` 到 `api/system/`
- 更新 package 宣告為 `package system`

### Phase 3: 驗證

```bash
go build ./... && go vet ./... && go test ./...
```

## 風險提示

| 風險 | 緩解措施 |
|------|----------|
| import cycle | 先 dry-run：手動畫依賴圖確認無循環 |
| 名稱衝突 | 整併前檢查目標套件中無同名函數 |
| 測試斷裂 | 遷移後立即跑套餐件測試 |
| API 路徑變更 | 不變更路由路徑，僅變更內部結構 |

## 不可碰

- 不變更任何 API 路由路徑（URL 必須保持一致）
- 不變更 API 回應格式
- 不變更 `internal/portfolio/`、`internal/orchestrator/`、`internal/live/`、`internal/eventlogic/`

## 驗收標準

- [ ] 3 個來源目錄已移除
- [ ] 所有 API 端點仍可正常存取（URL 不變）
- [ ] `go build ./...` ✅
- [ ] `go test ./...` ✅
- [ ] `gofmt -l .` ✅
- [ ] import cycle 檢查通過
