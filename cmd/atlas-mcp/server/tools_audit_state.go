package server

// feat/20260807-mcp-audit-state-tool — M6: 憲章審計狀態 MCP 工具。
//
// 對應 §附錄 F M6（憲章審計狀態 MCP 工具公開）：
//   - 目標：把 `docs/ATLAS_CONSTITUTION_AUDIT.md` §附錄 D（22 個審計項目
//     追蹤表）與 §附錄 F（F1-F5 / M1-M6 / X1-X3 治理追蹤表）以結構化
//     JSON 透過 MCP 對外公開，讓外部 agent（Hermes / Opus / Claude Desktop）
//     可以 self-audit「憲章對齊狀態」而不必讀取整份 markdown。
//   - 實作方式：embedded snapshot（Go struct），非 runtime 讀檔。
//     原因：
//       1. atlas-mcp 是獨立 binary，部署容器內不一定有 docs/ 目錄；
//       2. 審計狀態是「事實性」資料，隨 binary release 更新即可；
//       3. X3（nightly scan 自動漂移警報）是未來方向，與本工具的
//          embedded snapshot 不同層次——本工具提供「當前已知狀態」，
//          X3 提供「自動偵測漂移」。
//   - 更新紀律：修改 §附錄 D / §附錄 F 時，必須同步更新本檔
//     auditSnapshot 常數；新增 audit drift 檢查會加在 ci-gate 中。
//
// 工具名：audit_state
// 參數：無
// 輸出：ConstitutionAuditState（metadata + audit items + governance items + 統計）

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AuditItemStatus 是審計項目的狀態列舉（與 docs 表格的 ✅/⚠️/⬜ 對照）。
type AuditItemStatus string

const (
	AuditStatusDone     AuditItemStatus = "done"      // ✅ 完成
	AuditStatusPartial  AuditItemStatus = "partial"   // ⚠️ 部分完成
	AuditStatusNotStart AuditItemStatus = "not_start" // ⬜ 未啟動
	AuditStatusAligned  AuditItemStatus = "aligned"   // 已對齊（不需修改）
)

// AuditItem 是 §附錄 D 追蹤表中的一行。
type AuditItem struct {
	ID       string          `json:"id"`                 // e.g. "A1"
	Title    string          `json:"title"`              // e.g. "七時期判斷（DetectPeriod）"
	Level    string          `json:"level"`              // P0 / P1 / P2 / aligned
	Status   AuditItemStatus `json:"status"`             // done / partial / not_start / aligned
	PRs      string          `json:"prs,omitempty"`      // 修復 PR 編號（逗號分隔）
	Progress string          `json:"progress,omitempty"` // 補充說明（partial 時使用）
}

// GovernanceItem 是 §附錄 F 中的一行（F1-F5 / M1-M6 / X1-X3）。
type GovernanceItem struct {
	ID        string          `json:"id"`                  // e.g. "M3"
	Group     string          `json:"group"`               // fmx / mcp / enforce
	Title     string          `json:"title"`               // e.g. "因果鏈 tracing MCP 工具公開"
	Status    AuditItemStatus `json:"status"`              // done / partial / not_start
	Note      string          `json:"note,omitempty"`      // 備註
	Related   string          `json:"related,omitempty"`   // 對應 MCP 工具 / 文件
	Versioned string          `json:"versioned,omitempty"` // 從 v1.0 → v1.1 的狀態變化
}

// AuditStats 是彙總統計。
type AuditStats struct {
	Total              int `json:"total"`
	Done               int `json:"done"`
	Partial            int `json:"partial"`
	NotStart           int `json:"not_start"`
	Aligned            int `json:"aligned"`
	P0Done             int `json:"p0_done"`  // P0 中已完成數量
	P0Total            int `json:"p0_total"` // P0 總數量
	GovernanceDone     int `json:"governance_done"`
	GovernancePartial  int `json:"governance_partial"`
	GovernanceNotStart int `json:"governance_not_start"`
}

// ConstitutionAuditState 是 audit_state 工具的回應 payload。
type ConstitutionAuditState struct {
	DocVersion string           `json:"doc_version"` // e.g. "v1.1"
	UpdatedAt  string           `json:"updated_at"`  // e.g. "2026-08-07"
	HeadCommit string           `json:"head_commit"` // 對應 commit SHA
	Source     string           `json:"source"`      // 資料來源文件
	AuditItems []AuditItem      `json:"audit_items"` // §附錄 D 22 行
	Governance []GovernanceItem `json:"governance"`  // §附錄 F 14 行
	Stats      AuditStats       `json:"stats"`
}

// registerAuditStateTool 註冊 audit_state 工具。此工具無參數、無副作用，
// 回傳憲章審計追蹤表快照。
func registerAuditStateTool(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "audit_state",
		Description: autoDescOr("audit_state", "Return the constitution audit tracking snapshot (ATLAS_CONSTITUTION_AUDIT.md §附錄 D + §附錄 F). Use to self-audit 憲章對齊狀態: 22 audit items + F1-F5/M1-M6/X1-X3 governance progress, with P0/P1/P2 stats. Read-only; no side effects."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleAuditState)
}

// handleAuditState 回傳憲章審計快照。
func (s *server) handleAuditState(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ConstitutionAuditState, error) {
	return nil, auditSnapshot, nil
}

// auditSnapshot 是憲章審計追蹤表的 embedded 快照。
// 資料來源：docs/ATLAS_CONSTITUTION_AUDIT.md（v1.1, 2026-08-07, commit fb25ecca）
//
// 更新紀律：修改憲章審計文件時同步更新此處；或在 PR 中跑
// `go generate ./cmd/atlas-mcp/...` 檢查 drift。
var auditSnapshot = ConstitutionAuditState{
	DocVersion: "v1.1",
	UpdatedAt:  "2026-08-07",
	HeadCommit: "fb25ecca",
	Source:     "docs/ATLAS_CONSTITUTION_AUDIT.md",
	AuditItems: []AuditItem{
		{ID: "A1", Title: "七時期判斷（DetectPeriod）", Level: "P0", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "A2", Title: "七時期→三態向下相容", Level: "P0", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "A3", Title: "三套 regime 系統統一", Level: "P0", Status: AuditStatusDone, PRs: "#1372 (PeriodToRegime 映射)"},
		{ID: "B1", Title: "管線重排（MacroFlow 前置）", Level: "P0", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "B2", Title: "每層輸出強制影響下一層", Level: "P0", Status: AuditStatusDone, PRs: "#1381"},
		{ID: "B4", Title: "VIX key 修復 + macro evidence 注入", Level: "P0", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "C1", Title: "七大勢力數據源（壽險/公司派/散戶）", Level: "P0", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "C2", Title: "公股行庫自動化通道", Level: "P0", Status: AuditStatusDone, PRs: "#1372 + #1421 + #1424 + #1437"},
		{ID: "C3", Title: "orchestrator PrimaryFlow 改用 capitalflow", Level: "P0", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "D1", Title: "detector 時期敏感度", Level: "P0", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "D2", Title: "YAML consumer", Level: "P0", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "D3", Title: "推薦引擎按時期過濾", Level: "P0", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "D5", Title: "RegimeAllocator 六策略×七時期", Level: "P0", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "A4", Title: "macroflow RiskLevel 自動推導", Level: "P1", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "B3", Title: "MacroDataSnapshot 補漏指標", Level: "P1", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "C4", Title: "capitalflow 4-layer Assessment 消費鏈", Level: "P1", Status: AuditStatusDone, PRs: "#1378"},
		{ID: "E3", Title: "API 輸出時期結構化欄位", Level: "P1", Status: AuditStatusDone, PRs: "#1406 + #1426"},
		{ID: "E4", Title: "前端七時期 UI 卡片", Level: "P1", Status: AuditStatusDone, PRs: "#1397, #1398 + #1408"},
		{ID: "E5", Title: "策略類別三分類", Level: "P1", Status: AuditStatusDone, PRs: "#1404"},
		{ID: "C5", Title: "QualityScore 公式 + cfScore 動態權重", Level: "P2", Status: AuditStatusDone, PRs: "#1372"},
		{ID: "B5", Title: "Causal chain tracing", Level: "aligned", Status: AuditStatusAligned, PRs: "#1372 + MCP trace_get_*"},
		{ID: "C6", Title: "散戶反向指標統一口徑", Level: "aligned", Status: AuditStatusAligned, PRs: "#1372"},
	},
	Governance: []GovernanceItem{
		// F1-F5 DeepSeek 方法論覆核（群組 fmx）
		{ID: "F1", Group: "fmx", Title: "外資雙重動機模型（結構性 vs 投機性分流）", Status: AuditStatusNotStart, Versioned: "⬜ → ⬜", Note: "仍在 backlog；期間無相關 PR"},
		{ID: "F2", Group: "fmx", Title: "自營商大小分流（大型可納宏觀，小型用 AI 分點）", Status: AuditStatusNotStart, Versioned: "⬜ → ⬜", Note: "同上"},
		{ID: "F3", Group: "fmx", Title: "投信主動 vs 被動分流（ETF 被動買盤 vs 主動基金）", Status: AuditStatusNotStart, Versioned: "⬜ → ⬜", Note: "同上"},
		{ID: "F4", Group: "fmx", Title: "公股分點追蹤作為 BK-13 替代方案", Status: AuditStatusPartial, Versioned: "⬜ → 啟動為主要方案", Note: "TWSE CAPTCHA 啟用，F4 由 fallback 升級為主要方案"},
		{ID: "F5", Group: "fmx", Title: "選股層策略庫設計（Phase 4）", Status: AuditStatusNotStart, Versioned: "⬜ → ⬜", Note: "仍待 T27"},
		// M1-M6 MCP 工具對齊（群組 mcp）
		{ID: "M1", Group: "mcp", Title: "時期判斷 MCP 工具公開", Status: AuditStatusPartial, Versioned: "⬜ → ⚠️ partial", Note: "macro_get_snapshot_latest 等已暴露 7 時期欄位，但無獨立 PeriodDetector MCP", Related: "macro_get_snapshot_latest / macro_get_snapshot_history / macro_get_stress_index_current"},
		{ID: "M2", Group: "mcp", Title: "資金流品質分數 MCP 工具公開", Status: AuditStatusDone, Versioned: "⬜ → ✅", Note: "capital_flow_summary / capital_flow_daily / macro_get_capital_flow_latest 暴露 QualityScore", Related: "capital_flow_summary / capital_flow_daily"},
		{ID: "M3", Group: "mcp", Title: "因果鏈 tracing MCP 工具公開", Status: AuditStatusDone, Versioned: "⬜ → ✅", Related: "trace_get_decision_chain / trace_get_reasoning / trace_get_sim_latest / narrative_get_chains"},
		{ID: "M4", Group: "mcp", Title: "策略適用時期 MCP 工具公開", Status: AuditStatusPartial, Versioned: "⬜ → ⚠️ partial", Note: "get_recommendations 間接可用，但 GetApplicableStrategies(regime) 未獨立公開", Related: "get_recommendations / strategy_list_active / strategy_ranker"},
		{ID: "M5", Group: "mcp", Title: "壓力指數元件 MCP 工具公開", Status: AuditStatusDone, Versioned: "⬜ → ✅", Related: "macro_get_stress_index_current / narrative_stress_index_thresholds"},
		{ID: "M6", Group: "mcp", Title: "審計狀態 MCP 工具公開", Status: AuditStatusDone, Versioned: "⬜ → ✅（本工具）", Note: "audit_state 工具（本 PR 實作）", Related: "audit_state"},
		// X1-X3 憲章強制執行機制（群組 enforce）
		{ID: "X1", Group: "enforce", Title: "PR 合併前憲章對齊檢查（CI gate）", Status: AuditStatusPartial, Versioned: "⬜ → ⚠️ partial", Note: "#1423 提速 25× 後秒級可跑，但尚未強制為 mandatory gate", Related: "make ci-gate / check_constitution.sh"},
		{ID: "X2", Group: "enforce", Title: "方法論變更強制更新追蹤表", Status: AuditStatusPartial, Versioned: "⬜ → ⚠️ partial", Note: "#1464 ACI hook soft reminder，未升級為 PR template 強制檢查", Related: "PreToolUse soft reminder"},
		{ID: "X3", Group: "enforce", Title: "憲章漂移自動警報（nightly scan）", Status: AuditStatusPartial, Versioned: "⬜ → ⚠️ partial", Note: "known-issue badge + traps.md 為手動版；nightly scan 未建", Related: "traps.md / known-issue badge"},
	},
	Stats: AuditStats{
		Total:              22,
		Done:               22,
		Partial:            0,
		NotStart:           0,
		Aligned:            2,
		P0Done:             13,
		P0Total:            13,
		GovernanceDone:     3,
		GovernancePartial:  6,
		GovernanceNotStart: 5,
	},
}
