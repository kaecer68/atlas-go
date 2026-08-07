package server

// feat/20260807-m1-m4-period-strategy-tools — M4: 策略適用時期 MCP 工具。
//
// 對應 §附錄 F M4（策略適用時期 MCP 工具公開）：
//   - 目標：把「給定市場時期 → 回傳該時期適用策略清單」透過 MCP 對外
//     公開，讓外部 agent 直接回答「這個時期應該配置什麼策略」。
//   - 先前狀態：⚠️ partial — `get_recommendations` / `strategy_list_active`
//     間接可用，但 `GetApplicableStrategies(regime)` 無獨立工具。
//   - 實作方式：直接載入 `configs/methodology_rules.yaml`（與後端
//     MethodologyAdvisor 同源），呼叫 `MethodologyRules.GetAllowedStrategies()`
//     與 `GetStrategiesWithCategory()` — 無需 HTTP passthrough，純函數、
//     無副作用、零網路依賴。
//   - 資料源對齊：`internal/config/methodology_config.go` 的
//     `TryLoadMethodologyRules`（YAML 缺失時 fallback 到 default rules）。
//
// 工具名：strategy_for_period
// 參數：period（market period ID，見 internal/domain/shared/shared.go
//       MarketPeriod 常數：downturn / turnaround_up / bull / plateau /
//       consolidation / turnaround_down / black_swan）
// 輸出：StrategyForPeriodOutput（period 名稱 + allowed strategy IDs +
//       strategies brief 清單含 category/priority）

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StrategyForPeriodInput 是 strategy_for_period 工具的請求 schema。
type StrategyForPeriodInput struct {
	Period string `json:"period" jsonschema:"market period ID (downturn / turnaround_up / bull / plateau / consolidation / turnaround_down / black_swan)"`
}

// StrategyBriefOutput 是 strategy_for_period 輸出中的單一策略描述。
type StrategyBriefOutput struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"` // defensive / aggressive / tactical
	Priority string `json:"priority"` // primary / secondary
}

// StrategyForPeriodOutput 是 strategy_for_period 工具的回應 payload。
type StrategyForPeriodOutput struct {
	Period       string                `json:"period"`
	PeriodNameZH string                `json:"period_name_zh"`
	PeriodNameEN string                `json:"period_name_en"`
	Allowed      []string              `json:"allowed"`
	Strategies   []StrategyBriefOutput `json:"strategies"`
	KnownPeriods []string              `json:"known_periods"` // 支援的七時期清單
	Source       string                `json:"source"`
}

// registerStrategyForPeriodTool 註冊 strategy_for_period 工具。
func registerStrategyForPeriodTool(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "strategy_for_period",
		Description: autoDescOr("strategy_for_period", "Return the strategies allowed for a given market period (downturn / turnaround_up / bull / plateau / consolidation / turnaround_down / black_swan). Reads configs/methodology_rules.yaml (same source as MethodologyAdvisor). Use to answer 'what strategy fits this regime' without calling the recommender. Read-only; no side effects."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStrategyForPeriod)
}

// handleStrategyForPeriod 回傳指定時期適用的策略清單。
func (s *server) handleStrategyForPeriod(ctx context.Context, _ *mcp.CallToolRequest, in StrategyForPeriodInput) (*mcp.CallToolResult, StrategyForPeriodOutput, error) {
	period := in.Period
	rules := config.TryLoadMethodologyRules("configs/methodology_rules.yaml")

	// 驗證時期 ID；未知時期回 400 語意（含 known_periods 供 agent 修正）。
	out := StrategyForPeriodOutput{
		Period:       period,
		KnownPeriods: knownMarketPeriodIDs(),
		Source:       "configs/methodology_rules.yaml (MethodologyAdvisor 同源)",
	}
	if period == "" {
		return nil, out, errPeriodRequired
	}

	// period 名稱（zh / en）
	for _, reg := range rules.Regimes {
		if reg.ID == period {
			out.PeriodNameZH = reg.Name
			out.PeriodNameEN = reg.NameEn
			break
		}
	}
	if out.PeriodNameZH == "" {
		// 未知時期：回 400，附 known_periods
		return nil, out, &unknownPeriodError{period: period}
	}

	out.Allowed = rules.GetAllowedStrategies(period)
	for _, sb := range rules.GetStrategiesWithCategory(period) {
		out.Strategies = append(out.Strategies, StrategyBriefOutput{
			ID:       sb.ID,
			Name:     sb.Name,
			Category: sb.Category,
			Priority: sb.Priority,
		})
	}
	return nil, out, nil
}

// knownMarketPeriodIDs 回傳支援的七時期 ID（供未知時期錯誤訊息用）。
func knownMarketPeriodIDs() []string {
	periods := []shared.MarketPeriod{
		shared.PeriodDownturn,
		shared.PeriodTurnaroundUp,
		shared.PeriodBull,
		shared.PeriodPlateau,
		shared.PeriodConsolidation,
		shared.PeriodTurnaroundDown,
		shared.PeriodBlackSwan,
	}
	out := make([]string, 0, len(periods))
	for _, p := range periods {
		out = append(out, string(p))
	}
	return out
}

// errPeriodRequired 是 period 參數缺漏的錯誤。
var errPeriodRequired = &mcpError{Code: "PERIOD_REQUIRED", Message: "strategy_for_period: period is required (downturn / turnaround_up / bull / plateau / consolidation / turnaround_down / black_swan)"}

// unknownPeriodError 是未知 period ID 的錯誤。
type unknownPeriodError struct {
	period string
}

func (e *unknownPeriodError) Error() string {
	return "strategy_for_period: unknown period " + e.period
}

// mcpError 是帶 code 的錯誤（供 MCP client 分支）。
type mcpError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *mcpError) Error() string { return e.Message }
