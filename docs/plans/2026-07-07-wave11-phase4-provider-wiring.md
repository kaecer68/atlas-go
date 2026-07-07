# Wave 11 Phase 4: 後端資料提供者串接（停掉 stub）

> 目標：清理 PR #971–#975 重構後遺留的後端資料提供者 stub / legacy 路徑，確保生產環境使用單一、可觀測的 Gateway-backed 資料流。

## 背景

Phase 3 完成後，前端欄位合約已與後端對齊。但後端仍有兩條並行的 macro 資料路徑：

1. **Legacy `NewDashboardAPI`**：直接組裝 `CompositeMacroProvider`，繞過 `apigateway.Gateway`。
2. **Gateway-backed `NewDashboardAPIWithGateway`**：透過 `macroDataGatewayAdapter` 從 Gateway 取得資料，是生產主推路徑。

`NewDashboardAPI` 已被註解標示為 legacy，但下列生產程式碼仍使用它：

- `cmd/backtest-window/main.go`（serve mode）
- `internal/bootstrap/dashboard.go`
- `cmd/atlas/bootstrap_helpers.go` 的 `defaultAppDeps` fallback（Gateway 初始化失敗時）

此外，`internal/marketdata/etf_nav_scraper.go` 仍有一處明確 stub：TWSE ETF NAV 無免費 REST API。

## 階段範圍

**屬於 Phase 4**：

1. 明確標記 `NewDashboardAPI` 為 deprecated，禁止新程式碼使用。
2. 在 `internal/bootstrap/dashboard.go` 提供 `NewDashboardAPIWithGateway` wrapper，方便無法直接引入 `monitoring` 的套件使用。
3. 將 `cmd/backtest-window/main.go` 的 serve mode 遷移到 `NewDashboardAPIWithGateway` + `monitoring.NoopFetcher()`（該模式主要展示 backtest 報告，不依赖 macro 資料）。
4. 更新 `cmd/atlas/bootstrap_helpers.go` 的 fallback：Gateway 初始化失敗時改用 `NoopFetcher` 並記錄明確警告，避免默默回到 legacy CompositeMacroProvider。
5. 評估 `etf_nav_scraper.go` stub：若短期無法實作，改以文件/TODO 明確標示缺口與替代方案。
6. commit → push → PR #979

**不屬於 Phase 4**（留待 Phase 5+）：

- 新增頁面功能
- 安全強化
- 大規模重構 DashboardAPI 內部實作

## 實作步驟

### 4.1 標記 legacy constructor

在 `internal/monitoring/dashboard_api.go` 的 `NewDashboardAPI` 上加註：

```go
// Deprecated: production code should use NewDashboardAPIWithGateway with a
// monitoring.DataFetcher. This constructor assembles CompositeMacroProvider
// directly and bypasses apigateway.Gateway observability and caching.
```

### 4.2 新增 Gateway wrapper

在 `internal/bootstrap/dashboard.go` 新增：

```go
func NewDashboardAPIWithGateway(workDir, ledgerDir string, collector *monitoring.MetricsCollector, fetcher monitoring.DataFetcher) *monitoring.DashboardAPI {
    return monitoring.NewDashboardAPIWithGateway(workDir, ledgerDir, collector, fetcher)
}
```

### 4.3 遷移 backtest-window serve mode

`cmd/backtest-window/main.go` 改為：

```go
dashboard := monitoring.NewDashboardAPIWithGateway(cfg.WorkDir, cfg.LedgerDir, nil, monitoring.NoopFetcher())
```

並在 AGENTS.md / 計畫文件說明：serve mode 僅用於 backtest 報告展示，macro 資料由 NoopFetcher 提供空快照。

### 4.4 更新 atlas fallback

在 `cmd/atlas/bootstrap_helpers.go` 的 `defaultAppDeps` 中，將 `newDashboardAPI` fallback 改為回傳使用 `NoopFetcher` 的 `NewDashboardAPIWithGateway`，並記錄警告：

```go
newDashboardAPI: func(workDir, ledgerDir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
    logging.Warn("bootstrap", "gateway_unavailable_using_noop_fetcher")
    return monitoring.NewDashboardAPIWithGateway(workDir, ledgerDir, collector, monitoring.NoopFetcher())
},
```

### 4.5 ETF NAV stub 評估

- 檢查 `internal/marketdata/etf_nav_scraper.go` 的 Tier-1 stub 是否已有替代方案（如 QuoteFetcher fallback）。
- 若無法在 Phase 4 實作，於檔案頂端加入 `// TODO(#980): ...` 並更新相關 AGENTS.md。

## 驗收標準

- [ ] `go vet ./...` 無錯誤
- [ ] `go test ./internal/monitoring/... ./cmd/atlas/... ./cmd/backtest-window/...` 通過
- [ ] 生產程式碼不再新增 `NewDashboardAPI` 呼叫
- [ ] PR #979 已開出

## 風險與注意

- `NoopFetcher` 會讓 macro snapshot 為空；`backtest-window` serve mode 需確認不因此影響報告展示。
- `cmd/atlas` 的 Gateway fallback 改為 NoopFetcher 後，若 Gateway 初始化失敗，dashboard 將無 macro 資料；這比 silently 使用 legacy provider 更透明，但需確認 UI 能妥善處理空資料。
- 修改前先用 GitNexus 確認 `NewDashboardAPI` 的 upstream/downstream 影響範圍。
