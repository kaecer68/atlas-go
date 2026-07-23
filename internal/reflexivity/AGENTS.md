# AGENTS.md — reflexivity（自反性價格動態引擎）

> 參考格式：`internal/capitalflow/AGENTS.md`

---

## 模組職責

模型化 **Soros 自反性理論（Theory of Reflexivity）**：市場參與者的 bias（認知偏誤）與市場現實之間的雙向反饋迴路。

> 官方定位：`doc.go` 明確標示 `Status: experimental`。底層 model 仍在開發中，**請勿從 stable 模組依賴本模組**。

核心概念：
- **Bias（偏誤）**：agents 對市場條件的認知（overweight optimism / panic selling / herding）
- **Reality（現實）**：實際市場狀態（價格、成交量、基本面偏離）
- **FeedbackLoop（反饋迴路）**：bias → 交易行為 → 價格移動 → reality 更新 → 下一輪 bias（正向或負向強化）
- **ConcreteRules**：具體的經驗法則（rule-based），在 `Engine.Run` 之前對 `[]Recommendation` 進行 conviction 調整

---

## 公開 API

### Engine（核心引擎）

| 符號 | 簽名 | 說明 |
|------|------|------|
| `NewReflexivityEngine` | `() *ReflexivityEngine` | 工廠函式，建立引擎實例 |
| `RegisterBias` | `(bias *MarketBias) error` | 註冊市場偏誤（來自 agent recommendations） |
| `UpdateReality` | `(reality *MarketReality)` | 更新市場現實狀態 |
| `GetActiveLoops` | `() []FeedbackLoop` | 回傳所有活躍的反饋迴路 |
| `GetLoopsByTarget` | `(target string) []FeedbackLoop` | 查詢特定標的之反饋迴路 |
| `PredictLoopOutcome` | `(loopID string) (string, float64)` | 預測迴路走向與信心度 |
| `UpdateLoopStatus` | `(loopID string, status LoopStatus)` | 更新迴路生命周期狀態 |
| `GetReflexivityReport` | `() *ReflexivityReport` | 產生完整自反性分析報告 |
| `ProcessRecommendations` | `(recs []domain.Recommendation)` | 批次分析 recommendations 中的自反性模式 |
| `ApplyReflexivityAdjustment` | `(recs []domain.Recommendation) []domain.Recommendation` | 根據反饋迴路調整 recommendation conviction |

### FeedbackLoop 相關型別

| 型別 | 說明 |
|------|------|
| `FeedbackLoop` | 反饋迴路實體，含 `LoopDirection`（positive/negative）、`LoopStatus`（forming/active/dissipating） |
| `LoopDirection` | `LoopPositiveFeedback` / `LoopNegativeFeedback` |
| `LoopStatus` | `StatusForming` / `StatusActive` / `StatusDissipating` / `StatusComplete` |
| `MarketBias` | 參與者認知偏誤，含 `BiasType`（overweight_optimism / panic_selling / herding / anchoring / confirmation） |
| `MarketReality` | 市場實際狀態，含偏離幅度與趨勢 |
| `ReflexivityReport` | 完整分析報告，含各迴路詳細狀態 |

### ConcreteRules（規則引擎，實驗性）

實作 `Rule` 介面：

```go
type Rule interface {
    Apply(recs []domain.Recommendation, state domain.SimulationState, quotes map[string]domain.Quote) []domain.Recommendation
}
```

| 規則 | 觸發條件 | 效果 |
|------|----------|------|
| `PriceToFundamentalsRule{}` | 任一標的日內跌幅 > 15% | 對其他所有標的降低 conviction（credit risk premium） |
| `PnLBehaviorRule{}` | 組合回撤 > 10% | 降低新倉位 conviction（規避風險行為） |
| `NarrativeFlowsRule{Threshold: N}` | ≥ N 個 agents 同時推薦同一標的 | 視為共識擁擠，降低該標的 conviction |
| `MarketPolicyRule{Threshold: f}` | 大盤日內跌幅 > f（如 3%） | 提升防御性標的 conviction |
| `ReversalDetectionRule` | 同一標的連續 ≥ 5 天同向推薦 | 標記為自反性極端，降低 conviction |

工廠：`NewReversalDetectionRule() *ReversalDetectionRule`（其餘為零值可用）

---

## 關鍵陷阱

| 陷阱 | 說明 |
|------|------|
| **Model 仍在開發中** | `doc.go` 明確標示 `experimental`；feedback loop 檢測演算法與 bias 權重計算可能變動。請勿從 stable 模組依賴本模組。 |
| **已被 orchestrator + sim 使用** | `ReflexivityEngine`（`composition.go`）與 `ConcreteRules`（`composition.go` + `sim/engine.go`）已實際整合至 production 路徑。Model 變更需同步這些消費者。 |
| **ConcreteRules 是 rule-based 近似** | 並非完整的自反性動態模型，而是經驗法則；`PriceToFundamentalsRule` 的 15% 閾值、`PnLBehaviorRule` 的 10% 回撤閾值均為 hardcoded，需有依據地調整。 |
| **迴路檢測滯後** | `detectLoop` 依賴 `UpdateReality` 推送 reality 資料；日內即時情境可能落後於市場變化。 |
| **ReversalDetectionRule 有狀態** | 內部 `streaks map[string]int` 跨呼叫保留；`Engine` 重啟或 simulation reset 時需一併重置，否則 streak 計數錯誤。 |
| **無持久化** | `ReflexivityEngine` 狀態在記憶體中，engine 重啟後 feedback loops 全部丟失；目前 sim runtime 在每輪 `sim.Run` 重新初始化 engine。 |

---

## 依賴關係

```
internal/domain          — Recommendation, SimulationState, Quote
internal/orchestrator    — 透過 composition.go 注入 ReflexivityEngine + ConcreteRules
internal/sim/engine.go   — 透過 WithReflexivityRules 附加 ConcreteRules
```

**路由註冊**：`cmd/atlas/main.go` 無獨立 HTTP handler；`ReflexivityEngine` 由 `orchestrator.Composition` 工廠方法持有，透過 `Phase3Controller` 整合進 simulation pipeline。

---

## 整合速查

- `orchestrator/composition.go`：`WithReflexivityRules(PriceToFundamentalsRule{}, PnLBehaviorRule{}, NarrativeFlowsRule{Threshold: 3}, MarketPolicyRule{Threshold: 0.03}, NewReversalDetectionRule())`
- `orchestrator/factory.go`：`reflexivity.NewReflexivityEngine()` → 傳入 `NewPhase3Controller`
- `sim/engine.go`：`Engine.WithReflexivityRules(...rules)` — 附加規則後在 `Run()` 第 0 步套用（先於交易決策）
- `phase3_metrics.go`：追蹤 `ReflexivityActiveLoops` 數量

---

## 測試

- `reflexivity_test.go`：FeedbackLoop 檢測、Bias 註冊與 merge、`ApplyReflexivityAdjustment`、迴路狀態轉換、`GetReflexivityReport`
- `concrete_rules_test.go`：各 Rule 的 threshold 邊界行為
