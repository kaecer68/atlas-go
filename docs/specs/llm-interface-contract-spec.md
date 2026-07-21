# LLM 統一介面合約（Unified Interface Contract）

> **文件角色**：atlas-go LLM 整合的介面定義 + 既有模組接入範例（架構藍圖 §4.2-4.5 抽離）。
> **設計權威**：`docs/llm-integration-strategy-framework.md`（v2.1）
> **Maturity 規則**：`internal/MATURITY.md` LLM 相關條目

---

### 4.2 介面定義（提案）

> 以下為**新增**的介面合約（會以新檔案落地，例如 `internal/llm/provider.go`）；**不修改** `internal/llm_annotator/doc.go:75-80` 的既有 `Annotator`。

```go
// internal/llm/provider.go（提案；尚未實作）
package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// Capability 定義 LLM 可提供的「能力」而非「模型」。
// 新增 capability 必須同步 4 個位置：provider.go 常數、capabilities/ handler、
// docs §3 capability taxonomy、metrics label set。
type Capability string

const (
	CapabilityFailureAttribution     Capability = "failure_attribution"     // ADR-002 中樞
	CapabilityRationaleGeneration     Capability = "rationale_generation"
	CapabilityStrategySummary         Capability = "strategy_summary"
	CapabilityPromptLint              Capability = "prompt_lint"
	CapabilityScenarioSimulation      Capability = "scenario_simulation"
	CapabilityRiskSurfaceExtraction   Capability = "risk_surface_extraction"
	CapabilityRegimeExplanation       Capability = "regime_explanation"
	CapabilityPerformanceForensics    Capability = "performance_forensics"
	CapabilityCodeReviewAnnotation    Capability = "code_review_annotation"
)

// DataClass 標記 capability 輸出資料的合規分級。
// DataClass 在每次 Request 中由呼叫端標明，Router 依此拒絕違規 provider。
type DataClass string

const (
	DataClassPublic    DataClass = "public"     // 公開資料；任何 provider 可接收
	DataClassInternal  DataClass = "internal"   // 內部；non-hosted 偏好但 hosted 可
	DataClassRegulated DataClass = "regulated"  // 受規範金融資料；hosted M3 禁止
	DataClassSecret    DataClass = "secret"     // 營業秘密；強制 self-host
)

// ProviderImpl 是所有 provider client 必須實作的介面。
// ADR-001：此介面包進既有 Annotator，不取代。
type ProviderImpl interface {
	// Name 回傳 provider 的穩定識別字串。
	// 命名約定：見 `provider.go` 既有 Provider 常數區段。
	Name() Provider

	// Supports 檢查此 provider 是否可服務給定 capability。
	// 例：K2.7 對 CapabilityFailureAttribution 必須回 false（ADR-009）。
	Supports(cap Capability) bool

	// Call 執行一次 LLM 呼叫。
	// 必須遵守：DataClass 閘門、circuit breaker、rate limit、timeout。
	// 回傳 error 時需區分 transient（可重試）vs permanent（不可重試）。
	Call(ctx context.Context, req Request) (Response, error)

	// Health 回傳此 provider 當前健康度。
	// 觸發條件見 `specs/llm-routing.md` §6.5。
	Health() HealthStatus
}

// Request 是 capability-agnostic 的輸入。
type Request struct {
	Capability Capability         `json:"capability"`
	DataClass  DataClass          `json:"data_class"`
	Payload    json.RawMessage    `json:"payload"`
	Options    RequestOptions     `json:"options"`
	Metadata   map[string]string  `json:"metadata,omitempty"`
}

// RequestOptions 提供 per-call 行為控制。
type RequestOptions struct {
	// ForceProvider 用於測試 / sticky routing。
	// 若 provider.Supports(req.Capability) == false，Router 仍會拒絕（見 §4.4）。
	ForceProvider Provider `json:"force_provider,omitempty"`

	// Trace 開啟時，Router 會寫 raw response 到 trace store（預設關閉，見 ADR-007）。
	Trace bool `json:"trace,omitempty"`

	// TimeoutMS 覆寫 default timeout。
	TimeoutMS int `json:"timeout_ms,omitempty"`
}

// Response 是 capability-agnostic 的輸出。
type Response struct {
	Provider   Provider          `json:"provider"`
	Capability Capability        `json:"capability"`
	Payload    json.RawMessage   `json:"payload"`
	Usage      Usage             `json:"usage"`
	AttemptedProviders []Provider `json:"attempted_providers,omitempty"` // ADR-007 v2.0 補充
	DataClass  DataClass         `json:"data_class"`                     // ADR-007 v2.0 補充
	LatencyMS  int64             `json:"latency_ms"`
}

// Usage 記錄 token 與計費資訊。
type Usage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostPer1k    float64 `json:"cost_per_1k,omitempty"`
}

// HealthStatus 為 health 端點的最小資訊集。
type HealthStatus struct {
	Provider   Provider `json:"provider"`
	Healthy    bool     `json:"healthy"`
	CircuitOpen bool    `json:"circuit_open"`
	LatencyP95 int64    `json:"latency_p95_ms"`
	LastError  string   `json:"last_error,omitempty"`
}

// Router 是 capability-based 的統一入口。
// 實作見 `internal/llm/router.go`（v2.0 spec）。
type Router interface {
	// Call 處理一次 LLM 請求，依 capability + 動態健康度自動選擇 provider。
	// ADR-005：依 §3.2 決策表 + 4 級 fallback chain。
	Call(ctx context.Context, req Request) (Response, error)

	// Health 暴露所有 provider 的 health（見 `/api/llm/health`）。
	Health() map[Provider]HealthStatus
}

// Provider 是 provider 的穩定識別字串（避免在 metric label 用 model id）。
type Provider string

const (
	ProviderKimi        Provider = "kimi"
	ProviderMiniMax     Provider = "minimax"
	ProviderDeepSeek    Provider = "deepseek"
	ProviderOpenCodeGo  Provider = "opencode-go"
	ProviderOpenCodeZen Provider = "opencode-zen"
	ProviderMock        Provider = "mock"

	// DEPRECATED：v1.0 路線保留以避免破壞既有 config；新 capability 不得使用。
	ProviderOpenAI Provider = "openai"
)

// Capabilities 是 Capability 常數的 runtime 列表，供 metric label discovery。
func Capabilities() []Capability {
	return []Capability{
		CapabilityFailureAttribution,
		CapabilityRationaleGeneration,
		CapabilityStrategySummary,
		CapabilityPromptLint,
		CapabilityScenarioSimulation,
		CapabilityRiskSurfaceExtraction,
		CapabilityRegimeExplanation,
		CapabilityPerformanceForensics,
		CapabilityCodeReviewAnnotation,
	}
}

// 編譯期檢查：所有 ProviderImpl 實作必須 compile-pass。
var _ ProviderImpl = (ProviderImpl)(nil)
var _ Router = (Router)(nil)
```

> **重要**：介面**故意保持小**（4 個 method）。新增 method 需走 ADR；不要為了「方便」加 getter / setter。

### 4.3 CapabilityRequest 強型別範例

> Capability 對應的 payload 用 typed struct（見 `internal/llm/schemas/`），而非 `map[string]any`。
> 理由：編譯期檢查、IDE 自動完成、JSON schema 易生成。

**範例：Failure Attribution 請求**

```go
// internal/llm/schemas/failure_attribution.go
package schemas

import "time"

// FailureAttributionRequest 是 CapabilityFailureAttribution 的 typed payload。
type FailureAttributionRequest struct {
	// 必要欄位
	StrategyFrameID string                 `json:"strategy_frame_id" validate:"required"`
	Outcome         StrategyOutcome        `json:"outcome" validate:"required"`
	MarketContext   MarketContextSnapshot  `json:"market_context" validate:"required"`

	// 選擇性欄位
	HistoricalMatches []HistoricalMatch     `json:"historical_matches,omitempty"`
	MaxLength         int                   `json:"max_length,omitempty"`
}

type StrategyOutcome struct {
	Symbol       string  `json:"symbol"`
	Return       float64 `json:"return"`
	HoldDays     int     `json:"hold_days"`
	EntryDate    time.Time `json:"entry_date"`
	ExitDate     time.Time `json:"exit_date"`
}

type MarketContextSnapshot struct {
	Regime    string  `json:"regime"`     // bull / bear / high_vol
	Volatility float64 `json:"volatility"`
	Volume    int64   `json:"volume"`
}

type HistoricalMatch struct {
	StrategyFrameID string    `json:"strategy_frame_id"`
	Similarity      float64   `json:"similarity"`
	Outcome         string    `json:"outcome"` // success / failure
}
```

**範例：對應的 Response payload**

```go
// FailureAttributionResponse 是 CapabilityFailureAttribution 的 typed 回傳。
type FailureAttributionResponse struct {
	RootCause       string  `json:"root_cause"`       // 主因
	ContributingFactors []string `json:"contributing_factors"`
	Confidence      float64 `json:"confidence"`       // 0-1
	Severity        string  `json:"severity"`         // low / medium / high
	SuggestedFix    string  `json:"suggested_fix,omitempty"`
}
```

> **未來擴充**：typed schema 可由 `internal/llm/schemas/` 編譯時自動產生 Zod-compatible JSON Schema，供前端與 `cmd/lint-prompts` 驗證。

### 4.4 既有 Annotator 如何接入

> 目標：把現有 `internal/llm_annotator.Annotator` 包進新 `Router`，**不改** `Annotator` 簽章。

```go
// internal/llm/adapters/annotator_adapter.go
package adapters

import (
	"context"
	"encoding/json"

	"atlas-go/internal/llm"
	"atlas-go/internal/llm_annotator"
)

// AnnotatorAdapter 把 llm_annotator.Annotator 包成 llm.ProviderImpl。
// ADR-001：保留既有投資，不改 Annotator 簽章。
type AnnotatorAdapter struct {
	provider   llm.Provider
	annotator  *llm_annotator.Annotator
	capability llm.Capability // 這個 adapter 對應的 capability
}

func NewAnnotatorAdapter(p llm.Provider, a *llm_annotator.Annotator, cap llm.Capability) *AnnotatorAdapter {
	return &AnnotatorAdapter{provider: p, annotator: a, capability: cap}
}

func (a *AnnotatorAdapter) Name() llm.Provider { return a.provider }

func (a *AnnotatorAdapter) Supports(cap llm.Capability) bool {
	// K2.7 對敘事 capability 必須回 false（ADR-009）
	if a.provider == llm.ProviderKimi && isNarrativeCapability(cap) {
		return false
	}
	return cap == a.capability
}

func (a *AnnotatorAdapter) Call(ctx context.Context, req llm.Request) (llm.Response, error) {
	// 把 typed Request 轉成 Annotator 既有 input shape
	annotatorInput := a.translateRequest(req)

	// 委派給既有 Annotator
	annotatorOutput, err := a.annotator.Annotate(ctx, annotatorInput)
	if err != nil {
		return llm.Response{}, fmt.Errorf("annotator: %w", err)
	}

	// 把既有 output 包成新 Response
	return a.translateResponse(req, annotatorOutput), nil
}

func (a *AnnotatorAdapter) Health() llm.HealthStatus {
	// 既有 Annotator 沒有獨立 health 概念
	// 預設 healthy；circuit breaker 由 Router 層判斷
	return llm.HealthStatus{Provider: a.provider, Healthy: true}
}

func isNarrativeCapability(cap llm.Capability) bool {
	switch cap {
	case llm.CapabilityFailureAttribution,
		llm.CapabilityRationaleGeneration,
		llm.CapabilityStrategySummary,
		llm.CapabilityRegimeExplanation,
		llm.CapabilityPerformanceForensics,
		llm.CapabilityScenarioSimulation,
		llm.CapabilityRiskSurfaceExtraction:
		return true
	}
	return false
}

// translateRequest 與 translateResponse 省略（type-specific mapping）
```

> **為何保留 Annotator**：既有 metrics（`llm_annotator_requests_total`）與 alert 規則依賴既有介面；adapter pattern 讓我們在不改 production wiring 的前提下接通新架構（ADR-001）。

### 4.5 既有模組如何改寫呼叫端

> 兩個代表性範例：rationale corpus 與 PRISM training executor。

#### 4.5.1 既有 `monitoring/api/strategies/handlers.go:283-287`（rationale corpus miss → 直接 passthrough）

**Before**：

```go
// 既有邏輯
if !corpus.Has(reason) {
    return reason, nil // 直接 passthrough
}
```

**After**：

```go
// 改寫後：corpus miss 時嘗試 LLM fallback
if !corpus.Has(reason) {
    // corpus miss → 試 LLM fallback
    router := getLLMRouter() // 從 wiring 取得
    resp, err := router.Call(ctx, llm.Request{
        Capability: llm.CapabilityRationaleGeneration,
        DataClass:  llm.DataClassRegulated, // rationale 屬於 regulated
        Payload:    marshal(reason),
    })
    if err == nil {
        return parseRationaleResponse(resp.Payload), nil
    }
    // LLM 失敗：log + 降級到 passthrough
    log.Warn("rationale fallback failed", "err", err)
    return reason, nil
}
```

> **不變 invariant**：`internal/narrative/rationale_corpus.go` 本身**無變更**（ADR-003）；LLM fallback 發生在呼叫端（pipeline handlers）。

#### 4.5.2 既有 `orchestrator/prism_executor.go:94-102`（PRISM 訓練結案 → 不寫 insight）

**Before**：

```go
// 既有：訓練結案後直接 return，無 insight
func (e *PRISMTrainingExecutor) Run(ctx context.Context, req TrainingRequest) (TrainingResult, error) {
    // ... 訓練邏輯 ...
    return TrainingResult{
        Status: "completed",
        Metrics: metrics,
        // 無 insight 欄位
    }, nil
}
```

**After**：

```go
// 改寫後：訓練結案後加 insight 欄位（router 失敗時空字串）
type PRISMTrainingExecutor struct {
    // ... 既有欄位 ...
    insightRouter llm.Router // 可選注入
}

func NewPRISMTrainingExecutor(...) *PRISMTrainingExecutor {
    return &PRISMTrainingExecutor{
        // ... 既有初始化 ...
        insightRouter: nil, // 預設 nil → fallback 行為
    }
}

// SetInsightRouter 允許從 cmd/atlas/main.go 注入 router
func (e *PRISMTrainingExecutor) SetInsightRouter(r llm.Router) {
    e.insightRouter = l
}

func (e *PRISMTrainingExecutor) Run(ctx context.Context, req TrainingRequest) (TrainingResult, error) {
    // ... 既有訓練邏輯 ...
    result := TrainingResult{
        Status: "completed",
        Metrics: metrics,
    }

    // 嘗試 LLM insight
    if e.insightRouter != nil {
        insight, err := e.insightRouter.Call(ctx, llm.Request{
            Capability: llm.CapabilityScenarioSimulation,
            DataClass:  llm.DataClassRegulated, // PRISM 是 regulated
            Payload:    marshal(metrics),
        })
        if err == nil {
            result.Insight = parseInsight(insight.Payload)
        } else {
            log.Warn("insight generation failed", "err", err)
            // 不寫 insight：保留既有 behavior（failure silent）
        }
    }

    return result, nil
}
```

> **不變 invariant**：`Run` method signature **不變**；`TrainingResult.Insight` 為**新欄位**（向後相容，前端 optional 顯示）。
