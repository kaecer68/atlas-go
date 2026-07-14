# 數據源憲法整改計畫

**日期**: 2026-05-13（初版）→ 2026-05-22（更新）  
**狀態**: 部分完成（見下表）  
**原始違規數**: 97 處（清查報告見 `docs/audit/2026-05-13-constitution-violations.md`）  
**目前憲法檢查**: 1 advisory 警告（`autobacktest/loop.go` — 已文件化為合法例外）

---

## 整改進度

### 🔴 P0: 立即整改（高風險）

| # | 文件 | 違規類型 | 整改方式 | 狀態 | 備註 |
|---|------|---------|---------|------|------|
| 1 | `cmd/atlas/main.go` | env_direct | API Key 改為 Gateway 注入 | ✅ **已解決** | 已透過 `configs/allowed_env_vars.md` 白名單管理，`os.Getenv` 均在中 |
| 2 | `internal/marketdata/*.go` (6個) | http_direct | HTTP Client 統一管理 | ⏳ | 架構性重構，需統一到 Gateway。新違規已被 constitution.yml CI 阻斷 |
| 3 | `internal/bootstrap/background.go` | goroutine_direct | 改用 BackgroundTaskManager | ✅ **已解決** | 經審查此檔案僅含輔助函數(`getLatestReplayDate`, `recoverPanic`)，無獨立 goroutine |

### 🟡 P1: 短期整改（中風險）

| # | 文件 | 違規類型 | 整改方式 | 狀態 | 備註 |
|---|------|---------|---------|------|------|
| 4 | `internal/narrative/*.go` (3個) | http_direct + goroutine | 改用 Gateway | ⏳ | 需 Narrative 專用 Gateway 或共用現有 Gateway |
| 5 | `internal/monitoring/notifier.go` | http_direct | HTTP Client 統一管理 | ⏳ | 低優先級，notifier 非核心資料流 |
| 6 | `internal/live/*.go` | http_direct | HTTP Client 統一管理 | ⏳ | live mode 有獨立架構，需另行規劃 |

### 🟢 P2: 中期整改（低風險）

| # | 範圍 | 違規類型 | 整改方式 | 狀態 | 備註 |
|---|------|---------|---------|------|------|
| 7 | 47 處 provider_direct | provider_direct | 逐步遷移至 Gateway | ⏳ | 最大單項違規，需批次重構 |
| 8 | 散落式 config 讀取 | config_direct | 統一到 Gateway | ⏳ | 憲法檢查已可自動偵測新違規 |

---

## 治理改善完成項目（2026-05-22）

除上述違規修復外，已完成以下治理基礎建設：

### 執行閉環（Phase 1）

| # | 項目 | 說明 |
|---|------|------|
| ✅ | `configs/allowed_env_vars.md` | 47 個環境變數白名單（基礎設施/Broker/TWSE/資料源） |
| ✅ | `scripts/ci/check_constitution.sh` | 跨平台憲法合規檢查腳本（4 條規則） |
| ✅ | `.github/workflows/constitution.yml` | PR 自動觸發憲法檢查 |
| ✅ | `.github/copilot-instructions.md` 更新 | 加入 constitution.md + parameter-system.md 至 Core Files |
| ✅ | 3 個 `instructions/*.md` 更新 | 補充足跡規範引用 |

### 權威階層（Phase 2）

| # | 項目 | 說明 |
|---|------|------|
| ✅ | `docs/reference/guidelines-index.md` | 規範總索引，5 階層優先級鏈 + 使用情境路由表 |
| ✅ | `constitution.md` 第四條擴充 | BackgroundTaskManager 操作指引（例外判斷、註冊流程、DI 模式、命名規範、故障處理） |
| ✅ | [`internal/domain/AGENTS.md`](../specs/domain-types.md) 精簡（後續於 Wave 11 整併） | 從 49 行減至 27 行，移除重複內容 |

### 結構修復（Phase 3）

| # | 項目 | 說明 |
|---|------|------|
| ✅ | 移除 `sysMetrics.Start()` goroutine | 完全冗餘（`metrics_snapshot` task 已涵蓋） |
| ✅ | 4 個新 `AGENTS.md` | `config/`、`risk/`、`sim/`、`screener/` 模組特有陷阱文件化 |
| ✅ | `HealthChecker` 完成接線 | 確認為遺漏工作，清理後註冊為 `health_check` TaskManager task |
| ✅ | `constitution.md` 4.5.2 例外表格 | autobacktest + RuleEngine live 模式正式文件化 |

---

## 剩餘待辦

| 優先級 | 項目 | 說明 | 預估工時 |
|--------|------|------|---------|
| **高** | DashboardAPI Gateway 注入 | 新增 `SetGateway()` + 18 處 provider 改為 `gateway.Fetch()` + 3 個新 channel | ~6h |
| **中** | 47 處 provider_direct 修復（已排除 simulation/DI 路徑） | 實際 4 處 main.go 已完成，simulation 路徑與 DI 路徑標記為例外。餘下 DashboardAPI 為主 | ~6h |
| **中** | `data_channels.go` health check → 改為讀取 UnifiedHealthStore | 3 處 provider 呼叫已備 TODO 註解 | ~2h |
| **低** | PRISMManager BTM 遷移 | 需先暴露 System getter | ~2h |
| **低** | SpawningManager 死碼清理 | `Start()`/`Stop()`/`runLoop()` 從未被呼叫，可刪除 | ~1h |
| **低** | 季度審計流程 | 與現有 quality.yml/daily-maintenance.yml 整合 | ~1h |

---

## 驗收標準

- [x] `go build ./...` 通過
- [x] `go vet ./...` 通過
- [x] `scripts/ci/check_constitution.sh` exit 0
- [ ] `grep -r "os.Getenv.*API_KEY" --include="*.go" .` 無違規（白名單除外）
- [ ] 47 處 provider_direct 違規遷移至 Gateway
