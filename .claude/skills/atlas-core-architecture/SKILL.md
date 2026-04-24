# Atlas Core Architecture Skill

**版本**: 1.0  
**日期**: 2026-04-23  
**職責**: 理解 Atlas-Go 的整體架構、資料流、模組邊界  

---

## 系統概覽

Atlas-Go 是一套**模擬優先、稽核導向**的台股投資研究系統。

- **語言**: Go 1.25.0
- **主要依賴**: `golang.org/x/time`, `golang.org/x/text`, `github.com/redis/go-redis/v9`
- **資料庫**: PostgreSQL 15（持久化）、Redis 7（快取）
- **CI 工具**: `gofmt`, `go vet`, `staticcheck`, `golangci-lint`, `gosec`

---

## 分層架構

```
┌─────────────────────────────────────────────────────────────┐
│  資料層 (Data Layer)                                         │
│  ├── Market Data: TWSE OpenAPI, Fugle, Hybrid               │
│  ├── Macro Data: 美元、美債、日圓、匯率、商品、地緣政治       │
│  └── Replay Data: JSONL 格式（每行獨立 JSON 物件）           │
├─────────────────────────────────────────────────────────────┤
│  協調層 (Orchestration Layer)                                │
│  ├── SystemCore: 系統核心協調                                 │
│  ├── PluginHost: 插件管理                                     │
│  └── Executors: Sector/Style/Superinvestor 執行器            │
├─────────────────────────────────────────────────────────────┤
│  領域層 (Domain Layer)                                       │
│  ├── Regime, Recommendation, Position 等字串 enum            │
│  └── SimulationConstraints, Order, DayResult 等結構          │
├─────────────────────────────────────────────────────────────┤
│  引擎層 (Engine Layer)                                       │
│  ├── Simulator: 模擬引擎與部位狀態轉換                        │
│  ├── FactorEngine: 動能/價值/品質多因子計算                   │
│  └── Screener: 宣告式個股篩選                                 │
├─────────────────────────────────────────────────────────────┤
│  持久層 (Persistence Layer)                                  │
│  ├── Ledger: JSONL append-only 持久化                        │
│  └── Experiment Store: 實驗結果儲存                           │
└─────────────────────────────────────────────────────────────┘
```

---

## 核心資料流

```
Market Data → Orchestrator (context → screener → sector/style/superinvestor → control)
    ↓
Simulator → Ledger (JSONL append-only)
    ↓
Portfolio (Darwinian weights) → Dashboard
```

---

## 關鍵模組

| 目錄 | 職責 | 關鍵檔案 |
|------|------|---------|
| `internal/domain/` | 領域型別 | `types.go`, `simulation.go` |
| `internal/orchestrator/` | 流程協調 | `system.go`, `plugin_host.go`, `executors.go` |
| `internal/sim/` | 模擬引擎 | `engine.go` |
| `internal/experiment/` | 實驗生命週期 | `executor.go`, `judge.go`, `replay_compare.go` |
| `internal/portfolio/` | 權重管理 | `darwinian_weights.go`, `factor_engine.go` |
| `internal/narrative/` | 宏觀敘事 | `ingestor.go`, `knowledge_base.go` |
| `internal/janus/` | 盤勢偵測 | `detector.go` |
| `internal/prism/` | 分層訓練 | `regime.go` |
| `internal/ledger/` | 持久化 | `store.go` |

---

## 設計原則

1. **介面風格**: 小而聚焦，`Supports(...) bool` + 一個操作方法
2. **Early return**: 優先使用，減少巢狀縮排
3. **錯誤包裝**: 一律 `fmt.Errorf("context: %w", err)`
4. **Import 順序**: 標準庫 → 外部套件 → `github.com/kaecer68/atlas-go/...`
5. **領域 enum**: 維持字串型別（方便 JSON roundtrip）
6. **禁止**: 全域可變狀態、跨層洩漏

---

## 高危陷阱

| 陷阱 | 說明 | 預防 |
|------|------|------|
| Enabled agent 缺少 prompt | `configs/agents.json` 中每個 `enabled: true` 需對應 `prompts/agents/<name>.md` | 自動檢查腳本 |
| Darwinian 權重靜默夾制 | 權重限制 [0.3, 2.5]，超界靜默正規化 | 新增透明度記錄 |
| Baseline 未載入 | 實驗執行前必須確認 `data/state/baseline_policy.json` | 啟動檢查 |
| Replay 格式錯誤 | JSONL（每行獨立 JSON），非 JSON array | 格式驗證 |
| Session 日期不可信 | `RecordedAt` 是計算完成時間，排序以 `SessionID` 為準 | 時間解析函式 |

---

*技能版本: 1.0*  
*最後更新: 2026-04-23*
