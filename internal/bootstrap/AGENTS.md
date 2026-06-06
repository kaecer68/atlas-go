# AGENTS.md — internal/bootstrap

**成熟度**: stable
**模組職責**: 系統初始化與依賴注入，負責資料庫、儲存、倉儲、儀表板路由的啟動順序。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|
| `Config` | `bootstrap.go` | `WorkDir`、`LedgerDir` 等啟動配置 |
| `Runtime` | `bootstrap.go` | 聚合所有初始化後的依賴實例 |
| `Stores` | `bootstrap.go` | AlertStore、MetricsStore、OutcomeStore 組合 |
| `BrokerOverrides` | `broker_config.go` | Broker 配置覆寫與驗證 |

## 資料流

```
InitMetrics() → InitDatabase() → InitStores() → InitRepository() → InitTaskManager()
     ↓
RegisterDashboardRoutes() [依序註冊各模組路由]
     ↓
ApplyBrokerConfig() [驗證 live mode 與簽名器配置]
```

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **初始化順序不可顛倒** | Repository 依賴 Stores，Stores 依賴 Ledger，顛倒會 panic |
| **DATABASE_URL 為空時 graceful degradation** | `InitDatabase` 回傳 `nil, nil`，後續邏輯需判斷 nil |
| **Broker live mode 需顯式 opt-in** | 必須同時設置 `AllowLiveBroker` + `AllowRealSigner` 等旗標 |
| **路由註冊順序影響 middleware 鏈** | 先註冊的路由先匹配，順序錯誤可能導致 middleware 未生效 |
| **ATLAS_STORE_BACKEND 直接讀 env** | `InitStores` 繞過 Gateway 直接讀環境變數，僅限啟動階段使用 |
| **Broker 預設為 dry-run** | 未覆寫時 mode=`dry-run`、adapter=`guarded`、signer=`placeholder` |
| **getLatestReplayDate 解析寬鬆** | 取 CSV 第一欄為日期，無效列直接 skip，可能拿到錯誤日期 |

## 測試

- `go test ./internal/bootstrap/...`
- `bootstrap_test.go`：初始化順序與 graceful degradation 測試
