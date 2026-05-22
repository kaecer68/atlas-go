# Atlas-Go 接手指示 — 剩餘大型任務

> **用途**: 給新的 AI Agent 工作視窗，無縫接手 Phase C6 + C8 + D1-D5 的實作。
> **來源會話**: 已完成 Phase A+B+C1-C5+C7+C9（PR #75, branch `feat/portfolio-audit-phase6-10`）
> **日期**: 2026-05-22

---

## 一、專案導航

| 項目 | 值 |
|------|-----|
| **GitHub** | `github.com/kaecer68/atlas-go` |
| **實作分支** | `feat/portfolio-audit-phase6-10` |
| **Go 版本** | 1.25.0 |
| **模組** | `github.com/kaecer68/atlas-go` |
| **工作目錄** | `/Users/kaecer/workspace/atlas` |
| **關鍵文件** | `AGENTS.md`（根目錄守則）、`internal/apigateway/CONSTITUTION.md`（數據源憲法） |
| **CI 指令** | `go build ./... && go test ./internal/... && go vet ./... && test -z "$(gofmt -l .)"` |

### 第一次啟動
```bash
# 若分支已刪除，從 main 重建
git checkout main && git pull origin main
git checkout -b feat/portfolio-audit-phase6-10

# 若分支還在
git checkout feat/portfolio-audit-phase6-10
git pull origin feat/portfolio-audit-phase6-10
```

---

## 二、已完成的工作（不要重做）

### Phase A — 止血修復（7 項）
| 檔案 | 變更 |
|------|------|
| `internal/monitoring/service/pipeline.go` | 新增 WithNarrativeProvider / WithCycleProvider |
| `internal/monitoring/dashboard_api.go` | 注入 NarrativeEngine + CycleTracker 到 PipelineService |
| `web/static/js/pages/pipeline.js:645` | URL 組裝 & → ? 修正 |
| `internal/monitoring/service/pipeline.go` | Legacy PascalCase JSONL fallback |
| `internal/autobacktest/signals.go` | NewSignalEngine 改為 (*, error) 消除 panic |
| `internal/autobacktest/runner.go` | 同上，更新呼叫者 |
| `internal/monitoring/api/backtest/handlers.go` | 同上 |
| `web/static/js/names.js` | leo_satellite: '低軌衛星' |
| `internal/monitoring/api/control/handlers.go` | approve/reject 儲存 agent_id:symbol 組合鍵 |
| `internal/monitoring/service/control.go` | CreateIntervention 解析組合鍵 |

### Phase B — 閉環實作（8 項）
| 檔案 | 變更 |
|------|------|
| `internal/risk/decision.go` | **新**: RiskPhase, Verdict, ActionType, RiskDecision, RuleResult, OrderIntent, PortfolioState 型別 |
| `internal/risk/pre_trade.go` | **新**: PreTradeGate 含 4 項規則 (max_position/sector_exposure/VaR/cash_buffer) |
| `internal/risk/gate.go` | **新**: RiskGate 入口含 PreTradeCheck/InTradeCheck/PostTradeCheck + SetMode |
| `internal/orchestrator/system.go` | applyHumanOverrides 新增 approve/reject 過濾 + set_model_weight 接線 |
| `internal/portfolio/darwinian_weights.go` | 新增 SetWeight() 方法 |
| `internal/config/parameters.go` | 新增 RiskGateParameters 含 PreTrade/InTrade/PostTrade 設定 |
| `internal/config/parameters_defaults.go` | 對應預設值 |
| `web/static/index.html` | 新增 riskGatePanel 容器 |
| `web/static/js/pages/portfolio.js` | 新增 renderRiskGatePanel 接線 |
| `web/static/js/components/risk-gate-panel.js` | **新**: Risk Gate 面板 UI |
| `internal/reporting/performance.go` | 新增 CalculateSharpeRatio/CalculateBeta/CalculateAlpha/CalculateTrackingError/CalculateInfoRatio |
| `internal/monitoring/api/live/benchmark.go` | 改為呼叫 reporting 函數，移除重複的 mean/stdDev/sessionDateFromID |

### Phase C1 — InTradeGate
| 檔案 | 變更 |
|------|------|
| `internal/risk/in_trade.go` | **新**: InTradeGate 含 stop-loss, take-profit, trailing-stop (ATR 2x), volatility spike (3x), circuit breaker |
| `internal/risk/gate.go` | 新增 InTradeCheck() 方法 + inTrade 欄位 |

### Phase C2 — PostTradeGate
| 檔案 | 變更 |
|------|------|
| `internal/risk/post_trade.go` | **新**: PostTradeGate 含 drawdown halt(20%)/defensive(10%), rolling sharpe<0, consecutive losses≥5 |
| `internal/risk/gate.go` | 新增 PostTradeCheck() 方法 + postTrade 欄位 |

### Phase C3 — HumanIntervention 到期失效
| 檔案 | 變更 |
|------|------|
| `internal/domain/recommendation/recommendation.go` | ExpiresAt + TTLHours 欄位 + IsExpired() 方法 |
| `internal/monitoring/service/control.go` | CreateIntervention 設 TTL (pause=24h, sector=24h, approve=48h, reject=48h, model=72h) |
| `internal/monitoring/service/control.go` | GetActiveOverrides 跳過過期項 |
| `internal/orchestrator/system.go` | applyHumanOverrides 跳過過期項 |

### Phase C4 — RBAC
| 檔案 | 變更 |
|------|------|
| `internal/monitoring/api/shared/handler.go` | 新增 RequireAdmin() + AdminPost() |
| `internal/monitoring/api/control/handlers.go` | pause/resume/sector-ban/set-model-weight 改為 AdminPost |
| `internal/monitoring/api/parameters/handlers.go` | POST/sweep/rollback/reload 改為 AdminPost |
| `internal/monitoring/api/backtest/handlers.go` | POST /api/backtest/run 改為 AdminPost |
| `internal/monitoring/api/experiment/handlers.go` | promote/revert/judge 改為 AdminPost |
| `internal/monitoring/api/scheduler/handlers.go` | toggle 改為 AdminPost |
| `internal/monitoring/api/circuitbreaker/handlers.go` | reset 改為 AdminPost |
| `internal/monitoring/api/macro/handlers.go` | ingest 改為 AdminPost |

### Phase C5 — sector-ban 參數化
| 檔案 | 變更 |
|------|------|
| `internal/config/parameters.go` | IndustryParameters 新增 SkillToIndustries (map[string][]string) |
| `internal/config/parameters_defaults.go` | 對應預設值 |
| `internal/orchestrator/system.go` | isRecommendationInBannedSector 改為讀 config |

### Phase C7 — 參數熱更新
| 檔案 | 變更 |
|------|------|
| `internal/config/parameters.go` | 新增 ReloadParametersConfig() |
| `internal/monitoring/api/parameters/handlers.go` | 新增 POST /api/parameters/reload 端點 |
| `internal/monitoring/api/parameters/handlers.go` | POST /api/parameters 存檔後自動觸發 reload |
| `internal/monitoring/api/parameters/handlers.go` | rollback 後自動觸發 reload |

### Phase C9 — sessionDateFromID 統一
| 檔案 | 變更 |
|------|------|
| `internal/domain/session.go` | 新增 SessionDateFromID() |
| 7 個檔案, 24 處引用 | 全部改為 domain.SessionDateFromID() |
| 3 份重複實作 | 已移除 |

---

## 三、剩餘任務（請接手）

### 🔴 C6: Gateway 遷移（高優先級）

**目標**: 將 `dashboard_api.go` 中直接呼叫 HTTP 或 `os.Getenv` 的資料提供者遷移到統一的 `apigateway` 通道。

**參考文件**: `internal/apigateway/CONSTITUTION.md`

**關鍵檔案**:
- `internal/monitoring/dashboard_api.go` — 內含多個 data sources 和 providers
- `internal/apigateway/gateway.go` — Gateway 入口
- `internal/apigateway/channel_adapters.go` — 已註冊的通道配接器
- `cmd/atlas/main.go` — `RegisterChannelAdapters()` 呼叫點

**現有通道**（已註冊）:
- `fugle` (FugleChannelAdapter)
- TWSE / FinMind 等其他通道

**需要做的事**:
1. 檢查 `dashboard_api.go` 中所有直接建立 HTTP client 或呼叫 `os.Getenv` 的程式碼
2. 為未遷移的資料源建立 ChannelAdapter
3. 在 `main.go` 中註冊新通道
4. 更新 `dashboard_api.go` 改為呼叫 `gateway.Fetch(channelID)`
5. 執行憲法 CI 檢查: `scripts/ci/check_constitution.sh`

**架構提示**:
```go
// 現有模式（需遷移）:
func (a *DashboardAPI) someProvider() {
    apiKey := os.Getenv("SOME_KEY")
    client := &http.Client{}
    resp, _ := client.Get("https://api.example.com/...")
}

// 目標模式:
result, err := a.gateway.Fetch(ctx, "channel_id")
```

---

### 🟡 C8: 風控回測驗證（中優先級）

**目標**: 用歷史回測數據驗證 Risk Gate 的有效性。

**關鍵檔案**:
- `internal/risk/gate.go` — RiskGate 入口
- `internal/risk/pre_trade.go` — PreTradeGate
- `internal/risk/in_trade.go` — InTradeGate
- `internal/risk/post_trade.go` — PostTradeGate
- `internal/risk/decision.go` — 型別定義

**驗證方式**:
1. 從過去的 session 數據載入 PortfolioState
2. 對每筆歷史推薦執行 PreTradeCheck
3. 驗證: 超過風險門檻的訂單是否被攔截
4. 驗證: 安全範圍內的訂單是否被放行
5. 產生 effectiveness report（攔截率、誤攔截率、漏攔截率）

**依賴**: 需要歷史回測數據在 `data/state/` 或可執行的回測 CLI

---

### 🟢 D1: 參數自動調優（可選）

- 基於 Bayesian Optimization 的自動參數調校
- 需要 `internal/config/parameters.go` 中的 `ParameterMetadata` 結構
- 可參考現有的 `InferenceEngine` (`internal/config/inference.go`)

### 🟢 D2–D5: 壓力測試 / 成本建模 / 排程統一 / 自我學習

- D2: 場景式壓力測試框架
- D3: 交易成本、滑價、市場衝擊模型
- D4: 所有 ticker 改為 BackgroundTaskManager 統一排程
- D5: 策略自我學習閉環

---

## 四、架構地圖

```
cmd/atlas/main.go                    ← HTTP server, gateway init
internal/
├── apigateway/                      ← 數據源統一入口（憲法強制）
│   ├── CONSTITUTION.md              ← 數據源憲法（必讀）
│   ├── gateway.go                   ← Gateway.Fetch()
│   ├── channel_adapters.go          ← 已註冊通道
│   ├── background.go                ← BackgroundTaskManager
│   └── circuitbreaker.go
├── risk/                            ← 風險管理（Phase B+C 核心）
│   ├── decision.go                  ← RiskDecision, Verdict 等型別
│   ├── pre_trade.go                 ← PreTradeGate（4 規則）
│   ├── in_trade.go                  ← InTradeGate（5 規則）
│   ├── post_trade.go                ← PostTradeGate（3 規則）
│   ├── gate.go                      ← RiskGate 統一入口
│   ├── var_calculator.go            ← VaR/CVaR 計算
│   └── approval_workflow.go         ← 人工審批工作流
├── orchestrator/system.go           ← SystemCore + applyHumanOverrides
├── monitoring/
│   ├── dashboard_api.go             ← 核心 API 聚合器（C6 遷移目標）
│   └── api/
│       ├── control/handlers.go      ← 人工干預端點
│       ├── parameters/handlers.go   ← 參數管理端點（含熱更新）
│       ├── risk/handlers.go         ← 風險 API
│       └── pipeline/handlers.go     ← Pipeline API
├── config/
│   ├── parameters.go                ← ParametersConfig + ParameterMetadata
│   └── parameters_defaults.go       ← 預設值
├── reporting/performance.go         ← 效能指標計算（權威來源）
├── portfolio/darwinian_weights.go   ← DarwinianWeightManager
└── domain/
    ├── session.go                   ← SessionDateFromID() + SessionSummary
    └── recommendation/              ← HumanIntervention, Recommendation
web/static/
├── index.html
├── js/pages/portfolio.js
├── js/components/risk-panel.js      ← 風險分析面板（展示）
└── js/components/risk-gate-panel.js ← 風控閘道面板（操作）
```

---

## 五、開發注意事項

1. **分支策略**: `feat/portfolio-audit-phase6-10` → PR #75 → main。**禁止直接 push main**。
2. **CI 流程**: `go build ./...` → `go test ./internal/...` → `go vet ./...` → `gofmt -l .`
3. **憲法遵守**: 所有新資料源必須通過 Gateway，禁止直接 `os.Getenv` + `http.Client`。
4. **參數管理**: 所有 magic number 必須使用 `ParameterMetadata[T]` + config 管理。
5. **Risk Gate 整合**: 新的閘道邏輯應放在 `internal/risk/`，透過 `gate.go` 統一入口暴露。
6. **Pre-commit hook**: 修改 domain struct 時，`go generate .` 會自動同步前端型別定義。
7. **已知陷阱**: 見 `AGENTS.md` 中的「高危陷阱」章節。

---

## 六、快速啟動

```bash
# 1. 檢出分支
cd /Users/kaecer/workspace/atlas
git checkout feat/portfolio-audit-phase6-10

# 2. 確認編譯
go build ./...
go test ./internal/risk/... ./internal/config/...

# 3. 從這裡開始
# 優先級: C6 (Gateway 遷移) → C8 (風控驗證) → D1-D5 (卓越)
```
