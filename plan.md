# ATLAS 策略進化系統修復與優化迭代計劃

**來源**: analysis_report.md
**日期**: 2026-04-25
**版本**: 1.0

---

## 計劃概述

本計劃將 analysis_report.md 中的 7 個問題轉為可執行的修復任務，按優先順序分三波執行：

| 波次 | 時間框架 | 任務 | 風險 |
|------|---------|------|------|
| **Wave 1** | 本日 (P0) | P6 靜默夾制加日誌 | 無風險，純新增日誌 |
| **Wave 2** | 本週 (P1) | P1 文件對齊、P2 死碼標記 | 低風險，文件與標記 |
| **Wave 3** | 下週 (P2) | P5 動態命中率、P3 Sharpe、P4 OOS | 中風險，功能變更 |
| **Wave 4** | 下月 (P3) | P7 MetaLearner 決策 | 高風險，架構決策 |

---

## Wave 1: 本日 — P6 靜默夾制加日誌

### 任務 W1-1: 在 constrainWeight() 中加入夾制日誌

**檔案**: `internal/portfolio/darwinian_weights.go`
**行號**: 310-319
**修改類型**: 新增日誌記錄

**目前程式碼**:
```go
func (m *DarwinianWeightManager) constrainWeight(weight float64) float64 {
    if weight < DarwinianWeightMin {
        return DarwinianWeightMin
    }
    if weight > DarwinianWeightMax {
        return DarwinianWeightMax
    }
    return weight
}
```

**修改後程式碼**:
```go
func (m *DarwinianWeightManager) constrainWeight(weight float64, agentID string) float64 {
    if weight < DarwinianWeightMin {
        log.Printf("[Darwinian] Weight clamped to MIN: agent=%s raw=%.4f clamped=%.4f",
            agentID, weight, DarwinianWeightMin)
        return DarwinianWeightMin
    }
    if weight > DarwinianWeightMax {
        log.Printf("[Darwinian] Weight clamped to MAX: agent=%s raw=%.4f clamped=%.4f",
            agentID, weight, DarwinianWeightMax)
        return DarwinianWeightMax
    }
    return weight
}
```

**呼叫點更新**:
- `PerformDailyAdjustment()` 中所有 `constrainWeight(rawWeight)` 改為 `constrainWeight(rawWeight, w.AgentID)`
- 預計影響 3 處呼叫（top/middle/bottom tier 各一處）

**驗證**:
- [ ] `go build ./internal/portfolio/...` 成功
- [ ] `go test ./internal/portfolio/...` 通過
- [ ] 手動檢查日誌格式正確

---

### 任務 W1-2: 在 PerformDailyAdjustment() 回傳值中加入夾制事件

**檔案**: `internal/portfolio/darwinian_weights.go`
**行號**: 197-294（PerformDailyAdjustment 函式）

**新增結構體**（建議放在檔案頂部，與 DarwinianAgentWeight 相鄰）:
```go
// ClampingEvent records when a weight was clamped to boundary.
type ClampingEvent struct {
    AgentID   string    `json:"agent_id"`
    RawWeight float64   `json:"raw_weight"`
    FinalWeight float64 `json:"final_weight"`
    Boundary  string    `json:"boundary"` // "min" or "max"
    Timestamp time.Time `json:"timestamp"`
}
```

**修改 PerformDailyAdjustment 回傳簽名**:
```go
// 從:
func (m *DarwinianWeightManager) PerformDailyAdjustment() map[string]float64

// 改為:
func (m *DarwinianWeightManager) PerformDailyAdjustment() (adjustments map[string]float64, clampingEvents []ClampingEvent)
```

**驗證**:
- [ ] 所有呼叫者更新（`executors.go`, `system.go`, `plugin_control.go` 等）
- [ ] `go build ./...` 成功
- [ ] `go test ./internal/portfolio/...` 通過

---

## Wave 2: 本週 — P1 文件對齊 + P2 死碼標記

### 任務 W2-1: 更新技能文件門檻至實際值

**檔案**: `.claude/skills/atlas-strategy-evolution/SKILL.md`
**行號**: 93-98

**修改內容**:
將「觀測值門檻提高：level_1: 3→8, level_2: 8→15, level_3: 12→25」改為：

```markdown
### 目前門檻（實際程式碼值）

| 成熟度等級 | 最少觀測值 | 最少檢查數 |
|-----------|-----------|-----------|
| `level_1_exploratory` | **3** | 2 |
| `level_2_window_validated` | **8** | 3 |
| `level_3_regime_aware` | **12** | 4 |

> ⚠️ 技能文件 v2.0 規劃提升至 8/15/25，但尚未實作。
> 實際值定義於 `internal/experiment/judge.go:340-363`。
```

**驗證**:
- [ ] 文件語法正確
- [ ] 與 `judge.go` 實際值一致

---

### 任務 W2-2: 標記 AgentWeightManager 為已棄用

**檔案**: `internal/portfolio/agent_weights.go`
**行號**: 1-10

**修改內容**:
在 package 聲明後加入：
```go
// DEPRECATED: AgentWeightManager is not used in production.
// The active weight management system is DarwinianWeightManager in darwinian_weights.go.
// This file is retained for reference but should not be used for new code.
// See analysis_report.md (P2) for details.
```

**驗證**:
- [ ] `go build ./...` 成功（註解不影響編譯）
- [ ] 文件可讀性良好

---

### 任務 W2-3: 標記 MetaLearner 為已棄用

**檔案**: `internal/metalearning/metalearner.go`
**行號**: 1-15

**修改內容**:
在 package 聲明後加入：
```go
// DEPRECATED: MetaLearner is not integrated into the production experiment flow.
// It is not called by experiment/executor.go, orchestrator/system.go, or any other production code.
// See analysis_report.md (P7) for integration plan or removal decision.
```

**驗證**:
- [ ] `go build ./...` 成功

---

## Wave 3: 下週 — P5 動態命中率 + P3 Sharpe + P4 OOS

### 任務 W3-1: 在 InvestmentModel 中加入 HitRate 欄位

**檔案**: `internal/narrative/types.go`
**行號**: 48-59

**修改內容**:
```go
type InvestmentModel struct {
    ID            string   `json:"id"`
    Name          string   `json:"name"`
    ActiveThemes  []string `json:"active_themes"`
    FavoredSectors []string `json:"favored_sectors"`
    AvoidedSectors []string `json:"avoided_sectors"`
    Weight        float64  `json:"weight"`
    RecentError   float64  `json:"recent_error"`
    HitRate       float64  `json:"hit_rate"`        // 新增
    Description   string   `json:"description"`
}
```

**驗證**:
- [ ] `go build ./internal/narrative/...` 成功
- [ ] `go test ./internal/narrative/...` 通過

---

### 任務 W3-2: 在 EvaluateModels() 中動態更新 HitRate

**檔案**: `internal/narrative/knowledge_base.go`
**行號**: 266-313

**修改內容**:
在 `EvaluateModels()` 的結尾，於更新 `RecentError` 後加入：
```go
// Update HitRate based on RecentError (complementary metric)
model.HitRate = 1.0 - model.RecentError
if model.HitRate < 0 {
    model.HitRate = 0
}
if model.HitRate > 1 {
    model.HitRate = 1
}
```

**驗證**:
- [ ] `go test ./internal/narrative/...` 通過
- [ ] 手動驗證 HitRate 計算正確

---

### 任務 W3-3: 建立 Sharpe 穩定性檢查模組

**新檔案**: `internal/experiment/sharpe_stability.go`

**內容**:
```go
package experiment

import (
    "math"
    "fmt"
)

// SharpeStabilityCheck evaluates whether a Sharpe ratio series is statistically stable.
// Returns true if the standard error of the Sharpe is below the threshold.
func SharpeStabilityCheck(sharpeSeries []float64, threshold float64) (stable bool, stderr float64, err error) {
    if len(sharpeSeries) < 2 {
        return false, 0, fmt.Errorf("sharpe series must have at least 2 observations, got %d", len(sharpeSeries))
    }
    
    // Calculate mean Sharpe
    var sum float64
    for _, s := range sharpeSeries {
        sum += s
    }
    mean := sum / float64(len(sharpeSeries))
    
    // Calculate standard deviation
    var variance float64
    for _, s := range sharpeSeries {
        diff := s - mean
        variance += diff * diff
    }
    stddev := math.Sqrt(variance / float64(len(sharpeSeries)-1))
    
    // Standard error = stddev / sqrt(n)
    stderr = stddev / math.Sqrt(float64(len(sharpeSeries)))
    
    stable = stderr < threshold
    return stable, stderr, nil
}

// DefaultSharpeStabilityThreshold is the recommended threshold from the strategy evolution skill.
const DefaultSharpeStabilityThreshold = 0.5
```

**整合點**: `internal/experiment/judge.go`
在 `passesAcceptance()` 中，於 `improve_sharpe_like` 閘門後加入：
```go
// Sharpe stability check (if enough observations)
if result.CandidateObservations >= 10 {
    sharpeSeries := extractSharpeSeries(result) // 需實作
    stable, stderr, err := SharpeStabilityCheck(sharpeSeries, DefaultSharpeStabilityThreshold)
    if err != nil {
        log.Printf("[Judge] Sharpe stability check failed: %v", err)
    } else if !stable {
        log.Printf("[Judge] Sharpe unstable: stderr=%.4f >= threshold=%.4f", stderr, DefaultSharpeStabilityThreshold)
        // 可選：將 unstable 記錄為警告但不拒絕實驗
    }
}
```

**驗證**:
- [ ] `go test ./internal/experiment/...` 通過
- [ ] 新增 `sharpe_stability_test.go` 測試邊界條件

---

### 任務 W3-4: 建立 Out-of-Sample 驗證模組

**新檔案**: `internal/experiment/oos_validator.go`

**內容**:
```go
package experiment

import (
    "fmt"
    "time"
)

// OOSValidator performs out-of-sample validation on experiment candidates.
type OOSValidator struct {
    replayDataPath string
    store          *ledger.Store
}

// OOSResult contains the validation outcome.
type OOSResult struct {
    Passed           bool    `json:"passed"`
    BaselineScore    float64 `json:"baseline_score"`
    CandidateScore   float64 `json:"candidate_score"`
    Improvement      float64 `json:"improvement"`
    WindowDays       int     `json:"window_days"`
    Observations     int     `json:"observations"`
}

// Validate performs OOS validation using a non-overlapping time window.
func (v *OOSValidator) Validate(candidate ExperimentCandidate, baseline ExperimentResult, primaryWindowEnd time.Time) (*OOSResult, error) {
    // Use 30-day non-overlapping window after primary window
    oosStart := primaryWindowEnd.AddDate(0, 0, 1)
    oosEnd := oosStart.AddDate(0, 0, 30)
    
    // Score baseline on OOS window
    baselineOOS, err := v.scoreOnWindow(baseline, oosStart, oosEnd)
    if err != nil {
        return nil, fmt.Errorf("baseline OOS scoring failed: %w", err)
    }
    
    // Score candidate on OOS window
    candidateOOS, err := v.scoreOnWindow(candidate, oosStart, oosEnd)
    if err != nil {
        return nil, fmt.Errorf("candidate OOS scoring failed: %w", err)
    }
    
    improvement := candidateOOS - baselineOOS
    
    return &OOSResult{
        Passed:         improvement > 0,
        BaselineScore:  baselineOOS,
        CandidateScore: candidateOOS,
        Improvement:    improvement,
        WindowDays:     30,
        Observations:   v.countObservations(oosStart, oosEnd),
    }, nil
}

func (v *OOSValidator) scoreOnWindow(result interface{}, start, end time.Time) (float64, error) {
    // Delegate to existing replay_compare scoring logic
    // TODO: Implement using scorePromptWindowWithObservations or scoreConstraintWindowWithObservations
    return 0, fmt.Errorf("not yet implemented")
}

func (v *OOSValidator) countObservations(start, end time.Time) int {
    // TODO: Count actual trading days in window
    return 0
}
```

**整合點**: `internal/experiment/judge.go`
在 `passesAcceptance()` 中，於主視窗評估後加入 OOS 閘門：
```go
// Out-of-sample validation (level_2+ only)
if maturityLevel == "level_2_window_validated" || maturityLevel == "level_3_regime_aware" {
    oosValidator := NewOOSValidator(j.replayDataPath, j.store)
    oosResult, err := oosValidator.Validate(candidate, baseline, primaryWindowEnd)
    if err != nil {
        log.Printf("[Judge] OOS validation error: %v", err)
    } else if !oosResult.Passed {
        return false, fmt.Sprintf("OOS validation failed: improvement=%.6f", oosResult.Improvement)
    }
}
```

**驗證**:
- [ ] `go test ./internal/experiment/...` 通過
- [ ] 新增 `oos_validator_test.go`

---

## Wave 4: 下月 — P7 MetaLearner 決策

### 任務 W4-1: 決策 — 整合或移除 MetaLearner

**選項 A: 整合進實驗流程**
- 在 `experiment/executor.go` 的 `Execute()` 中，於實驗完成後呼叫 `metaLearner.SubmitSwarmData()`
- 在 `orchestrator/system.go` 的 `NewSystem()` 中初始化 `MetaLearner`
- 預估工作量: 8 小時

**選項 B: 標記並移除**
- 已在 W2-3 中標記為 deprecated
- 下月確認無人使用後，刪除 `internal/metalearning/` 目錄
- 預估工作量: 1 小時

**決策依據**:
- 檢查是否有任何外部系統（如 dashboard、API）引用 metalearning
- 檢查是否有文件描述其用途
- 若無，建議選項 B（移除）

---

## 執行檢查清單

### 通用驗證（每個 Wave 結束時執行）

```bash
# 格式檢查
test -z "$(gofmt -l .)"

# 建置
go build ./...

# 測試
go test ./...

# 品質檢查
go vet ./...
staticcheck ./...
```

### Wave 1 完成標準
- [ ] `darwinian_weights.go` 編譯成功
- [ ] 日誌格式正確（手動檢查）
- [ ] 所有現有測試通過
- [ ] 新測試覆蓋夾制場景

### Wave 2 完成標準
- [ ] 技能文件更新並與程式碼一致
- [ ] 死碼標記清晰可見
- [ ] 無編譯錯誤

### Wave 3 完成標準
- [ ] 新模組（sharpe_stability, oos_validator）有獨立測試
- [ ] 整合測試驗證閘門邏輯
- [ ] 文件更新說明新功能

### Wave 4 完成標準
- [ ] MetaLearner 決策記錄於文件中
- [ ] 若移除，確認無遺留引用

---

## 風險緩解計劃

| 風險 | 緩解措施 |
|------|---------|
| 修改 judge.go 影響既有實驗 | 先備份 `data/state/baseline_policy.json`，在 staging 環境驗證 |
| 新模組引入效能問題 | 使用 benchmark 測試，確保 Sharpe/OOS 檢查 < 100ms |
| 動態命中率不穩定 | 加入 EMA 平滑（alpha=0.3），避免劇烈跳動 |
| 移除死碼影響未來整合 | 標記 deprecated 保留 1 個版本，而非立即刪除 |

---

## 附錄：檔案對照表

| 問題 | 修改檔案 | 新檔案 | 測試檔案 |
|------|---------|--------|---------|
| P6 靜默夾制 | `darwinian_weights.go` | — | `darwinian_weights_test.go` |
| P1 文件脫節 | `SKILL.md` | — | — |
| P2 死碼標記 | `agent_weights.go`, `metalearner.go` | — | — |
| P5 動態命中率 | `types.go`, `knowledge_base.go` | — | `knowledge_base_test.go` |
| P3 Sharpe | `judge.go` | `sharpe_stability.go` | `sharpe_stability_test.go` |
| P4 OOS | `judge.go` | `oos_validator.go` | `oos_validator_test.go` |
| P7 MetaLearner | — | — | — |

---

## P-Infra: 基礎設施優化計劃（未來）

**觸發時機**：數據庫升級（JSONL → SQLite/PostgreSQL）時一併啟動
**與數據庫升級綁定原因**：Data Access Layer 的 interface 設計正好支援 JSONL → SQLite 的無縫切換，兩者一起做減少重複重構

---

### P-Infra 1: Structured Logging

**問題**：360 處 `log.Printf`/`fmt.Printf`，零 structured logging，production 問題難排查

**現況**：
```
48 個檔案使用 log.Printf/fmt.Printf
零 zap/zerolog/slog 引入
```

**實作目標**：
- 選擇 `slog`（標準庫，Go 1.21+）或 `zap`
- 建立 `internal/logging/logger.go`：package-level logger，結構化欄位
- 核心路徑先遷：orchestrator、marketdata、ledger
- 格式：`component`, `event`, `symbol`, `session_id`, `duration_ms`

**驗證**：
```bash
# 確認無 log.Printf 在核心路徑
grep -r "log\.Printf" internal/orchestrator/ internal/marketdata/ internal/ledger/
# 確認 slog/zap 已引入
grep -r "slog\.\|zap\." internal/
```

---

### P-Infra 2: Context Propagation

**問題**：127 處 `context.Background()`，graceful shutdown 不完整，goroutine 可能洩漏

**現況**：
- Phase 2 已修復核心管線（executors.go → collectRecommendations → ScreenDetailed）
- `system.go` 初始化（line 102, 112）仍有 2 處 `context.Background()`
- 所有 entry point（main.go）和測試檔案有 ~90+ 處

**實作目標**：
- 審計所有 internal/ 下的 `context.Background()` 使用
- 區分「entry point 必須用」vs「有 parent context 卻沒傳遞」
- 修復第二類（有機會被 parent 取消的 operation）
- 不修改：entry point、測試檔案

**Context 傳遞審計清單**（待審計）：
```
internal/orchestrator/system.go         — engine.WithContext(context.Background())   [需要？]
internal/orchestrator/system.go         — ctx: context.Background()                  [需要？]
internal/marketdata/*.go               — 各 provider GetQuotes() 呼叫
internal/ledger/*.go                    — 各 Store 方法
```

**驗證**：
```bash
# 確認 context 傳遞鍊完整
go test -race ./internal/orchestrator/...  # 無 context race
```

---

### P-Infra 3: Data Access Layer

**問題**：無 interface 抽象，直接操作 JSONL，未來換 DB 成本高

**現況 — PostgreSQL 和 Redis 已在使用中**：

```
PostgreSQL (pgx + migrate)
├── ✅ 連接池初始化  → db.Init() [cmd/atlas/main.go]
├── ✅ Migration 系統 → 可運作，sql/migrations/ 有 1 個 migration
├── ✅ channel_health 表 → Dashboard API 監控用
└── ❌ Ledger 數據  → 仍用 JSONL（這是真正的 gap）

Redis (go-redis)
├── ✅ NonceReplayStore → 可選，需 BrokerNonceStore: "redis"
└── ❌ 主要儲存     → 不是預設

JSONL (ledger package)
└── ✅ 主要儲存     → outcomes, sessions, scorecards
```

**已驗證的可用基礎設施**：
- `db.Init()` — pgxpool 建立 + migrate up，已在 `cmd/atlas/main.go` 正常運作
- `ChannelHealthStore` 模式 — 示範了「interface + PostgreSQL pool」的使用方式，可以復用

**真正的 gap**：
```go
// 現有（無 interface）
type Store struct { baseDir string }  // 直接操作 JSONL

// 缺口：ledger 層沒有 OutcomeStore / SessionStore interface
// 對比 ChannelHealthStore 的模式：
//   type ChannelHealthStore { pool *pgxpool.Pool }
//   → 已有 Pool 但 ledger 層沒有抽象成 interface
```

**實作目標**：
```go
// OutcomeStore — 推薦結果
type OutcomeStore interface {
    Record(ctx context.Context, outcomes []RecommendationOutcome) error
    Load(ctx context.Context) ([]RecommendationOutcome, error)
    LoadSession(ctx context.Context, sessionID string) ([]RecommendationOutcome, error)
}

// SessionStore — Session 元數據
type SessionStore interface {
    Save(ctx context.Context, session ReplaySession, summary *SessionSummary) error
    Load(ctx context.Context, sessionID string) (*ReplaySession, *SessionSummary, error)
}

// QuoteStore — 行情快取
type QuoteStore interface {
    Put(ctx context.Context, quotes []Quote) error
    Get(ctx context.Context, symbols []string, asOf time.Time) ([]Quote, error)
}

// JSONLStore 實作（現在）
type JSONLStore struct { ... }

// SQLiteStore 實作（未來）
type SQLiteStore struct { ... }

// PostgreSQLStore 實作（最終目標）
type PostgreSQLStore struct { pool *pgxpool.Pool }  // 復用既有的 db.Init()
```

**與 DB 升級的關聯**：
- JSONLStore → SQLiteStore → PostgreSQLStore 為同一 interface 的三實作
- 迁移过程：先建立 interface → 實作 SQLiteStore → 雙實作並行 → PostgreSQLStore → 確認後切換
- 降低風險：DB 升級不需要改變上層呼叫
- **可直接復用**：既有的 `db.Init()` 和 `sql/migrations/` 在過渡期間保持可用

**驗證**：
```bash
go build ./...
go test ./internal/ledger/...  # interface 不改變行為
```

---

## P-Infra 執行策略

### 觸發條件（任一滿足即啟動）

| 條件 | 緊迫度 |
|------|--------|
| JSONL 單日檔案超過 100MB | 高 |
| 需要多 service 共用 ledger 資料 | 高 |
| Production 需要 structured logging alert | 中 |
| Live mode graceful shutdown 仍有 goroutine 洩漏 | 高 |

### 執行順序

```
Phase 1: 建立 logging package
         ↓
Phase 2: 實作 OutcomeStore interface + JSONLStore
         ↓
Phase 3: 遷移 ledger 核心方法到 interface
         ↓
Phase 4: Context propagation 審計與修復
         ↓
Phase 5: Structured logging 全面遷移（非核心路徑）
         ↓
Phase 6: SQLiteStore 實作（與 DB 升級同步）
         ↓
Phase 7: 切換並移除 JSONLStore
```

---

## 原始計劃完成記錄

*計劃產出時間: 2026-04-25*
*來源: analysis_report.md*
