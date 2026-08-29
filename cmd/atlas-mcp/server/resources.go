//go:generate go run ../docsgen

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerResources(mcpSrv *mcp.Server, s *server) {
	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://config/parameters",
		Name:        "atlas-go Parameters Config",
		Description: "Live ParametersConfig from atlas-go admin endpoint. Live state, not on-disk snapshot. Read-only. Equivalent tool (with audit + params): parameters_get.",
		MIMEType:    "application/json",
	}, s.handleResourceConfigParameters)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://tools/catalog",
		Name:        "atlas-mcp Tool Catalog",
		Description: "118 MCP tools grouped by area (120 with sampling/elicitation feature-gated on). Source: docs/reference/tool-catalog.md (embedded at build time; catalog doc lags until the doc PR lands). Use to enumerate capabilities without tools/list round-trip.",
		MIMEType:    "text/markdown",
	}, s.handleResourceToolsCatalog)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://workflows/catalog",
		Name:        "atlas-go Workflow Catalog",
		Description: "42 WA-XXX workflows in 7 layers. Source: docs/reference/workflow-map.md (embedded at build time). Helps an agent decide which Tool maps to a given intent.",
		MIMEType:    "text/markdown",
	}, s.handleResourceWorkflowsCatalog)

	// R-10: 內部開發知識資源（file-based，embedded，不需 backend）
	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://reference/architecture",
		Name:        "atlas-go Architecture",
		Description: "docs/architecture.md — 分層架構活地圖（入口層/模組/eventbus/orchestrator）。Internal introspection.",
		MIMEType:    "text/markdown",
	}, s.handleResourceArchitecture)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://reference/constitution",
		Name:        "atlas-go Constitution",
		Description: "docs/reference/constitution.md — 開發憲法。Internal introspection.",
		MIMEType:    "text/markdown",
	}, s.handleResourceConstitution)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://reference/traps",
		Name:        "atlas-go Traps Reference",
		Description: "docs/reference/traps.md — 跨模組陷阱權威清單。Internal introspection.",
		MIMEType:    "text/markdown",
	}, s.handleResourceTraps)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://reference/parameter-system",
		Name:        "atlas-go Parameter System",
		Description: "docs/reference/parameter-system.md — 參數系統設計。Internal introspection.",
		MIMEType:    "text/markdown",
	}, s.handleResourceParameterSystem)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://processes/catalog",
		Name:        "atlas-go Process Catalog",
		Description: "docs/reference/processes.yaml — 結構化 workflow/process metadata。Internal introspection.",
		MIMEType:    "text/yaml",
	}, s.handleResourceProcessesCatalog)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://docs/map",
		Name:        "atlas-go Documentation Map",
		Description: "docs/documentation-map.md — 文件地圖。Internal introspection.",
		MIMEType:    "text/markdown",
	}, s.handleResourceDocsMap)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://modules/index",
		Name:        "atlas-go Module Index",
		Description: "internal/AGENTS_INDEX.md — 59 模組索引與各模組 AGENTS.md 位置。Internal introspection.",
		MIMEType:    "text/markdown",
	}, s.handleResourceModulesIndex)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://modules/maturity",
		Name:        "atlas-go Module Maturity",
		Description: "internal/MATURITY.md — 模組成熟度評級（S/E/X/U）。Internal introspection.",
		MIMEType:    "text/markdown",
	}, s.handleResourceModulesMaturity)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://audit/recent",
		Name:        "Recent Audit Log Entries",
		Description: "Last 50 entries from the JSONL audit log (most recent first; subset fields ts/tool/status/duration_ms; source ATLAS_MCP_AUDIT_LOG). Useful for debugging recent agent activity without a separate log query.",
		MIMEType:    "application/json",
	}, s.handleResourceAuditRecent)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://strategies/active",
		Name:        "Active Strategy Definitions",
		Description: "Current active strategies in the production strategy set. Live data from /api/strategies/active. Equivalent tool (with audit + params): strategy_list_active.",
		MIMEType:    "application/json",
	}, s.handleResourceStrategiesActive)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://market/regime",
		Name:        "Latest Market Regime",
		Description: "Current market regime classification (RISK_ON / RISK_OFF / NEUTRAL / TRANSITIONAL). Live data from /api/regime/history. Equivalent tool (with audit + days param): regime_get_history.",
		MIMEType:    "application/json",
	}, s.handleResourceMarketRegime)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://events/today",
		Name:        "Today's Market Events",
		Description: "Upcoming and active Taiwan market events for today. Live data from /api/events/calendar. Equivalent tool (with audit): event_calendar.",
		MIMEType:    "application/json",
	}, s.handleResourceEventsToday)
}

func (s *server) handleResourceConfigParameters(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	var params map[string]any
	if err := s.cli.Get(ctx, "/api/parameters", nil, &params); err != nil {
		return nil, fmt.Errorf("resource config parameters: %w", err)
	}
	return resourceText("atlas://config/parameters", "application/json", mustJSON(map[string]any{
		"parameters": params,
	})), nil
}

func (s *server) handleResourceToolsCatalog(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return embeddedResource("tools/catalog", "atlas://tools/catalog", "text/markdown")
}

func (s *server) handleResourceWorkflowsCatalog(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return embeddedResource("workflows/catalog", "atlas://workflows/catalog", "text/markdown")
}

// handleResourceArchitecture 提供 docs/architecture.md（分層架構活地圖）。
func (s *server) handleResourceArchitecture(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return embeddedResource("reference/architecture", "atlas://reference/architecture", "text/markdown")
}

// handleResourceConstitution 提供 docs/reference/constitution.md（開發憲法）。
func (s *server) handleResourceConstitution(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return embeddedResource("reference/constitution", "atlas://reference/constitution", "text/markdown")
}

// handleResourceTraps 提供 docs/reference/traps.md（跨模組陷阱權威）。
func (s *server) handleResourceTraps(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return embeddedResource("reference/traps", "atlas://reference/traps", "text/markdown")
}

// handleResourceParameterSystem 提供 docs/reference/parameter-system.md（參數系統）。
func (s *server) handleResourceParameterSystem(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return embeddedResource("reference/parameter-system", "atlas://reference/parameter-system", "text/markdown")
}

// handleResourceProcessesCatalog 提供 docs/reference/processes.yaml（結構化 workflow metadata）。
func (s *server) handleResourceProcessesCatalog(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return embeddedResource("processes/catalog", "atlas://processes/catalog", "text/yaml")
}

// handleResourceDocsMap 提供 docs/documentation-map.md（文件地圖）。
func (s *server) handleResourceDocsMap(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return embeddedResource("docs/map", "atlas://docs/map", "text/markdown")
}

// handleResourceModulesIndex 提供 internal/AGENTS_INDEX.md（模組索引）。
func (s *server) handleResourceModulesIndex(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return embeddedResource("modules/index", "atlas://modules/index", "text/markdown")
}

// handleResourceModulesMaturity 提供 internal/MATURITY.md（模組成熟度評級）。
func (s *server) handleResourceModulesMaturity(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return embeddedResource("modules/maturity", "atlas://modules/maturity", "text/markdown")
}

// embeddedResource 回傳 build-time 內嵌的文件快照（由 cmd/atlas-mcp/docsgen
// 產生 resources_docs.gen.go）。不依賴 process CWD / 檔案系統。
func embeddedResource(key, uri, mime string) (*mcp.ReadResourceResult, error) {
	raw, ok := embeddedDocs[key]
	if !ok {
		return nil, fmt.Errorf("resource %s: embedded doc %q missing (run go generate ./cmd/atlas-mcp/...)", uri, key)
	}
	return resourceText(uri, mime, string(raw)), nil
}

func (s *server) handleResourceAuditRecent(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	entries, err := tailAuditLog(s.audit.path, 50)
	if err != nil {
		return nil, fmt.Errorf("resource audit recent: %w", err)
	}
	return resourceText("atlas://audit/recent", "application/json", mustJSON(map[string]any{
		"entries": entries,
		"count":   len(entries),
	})), nil
}

func resourceText(uri, mime, text string) *mcp.ReadResourceResult {
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: mime,
			Text:     text,
		}},
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// tailAuditLog reads up to the last 64KB of the audit log and returns the last
// n JSONL entries. Best-effort: the file is bounded by the retention period
// (default 30 days), and 64KB holds ~500 JSONL lines at ~120 bytes/line.
func tailAuditLog(path string, n int) ([]auditEntryView, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	const tailSize = 64 * 1024
	offset := max(info.Size()-tailSize, 0)
	if _, err := f.Seek(offset, 0); err != nil {
		return nil, err
	}

	buf := make([]byte, info.Size()-offset)
	if _, err := f.Read(buf); err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	entries := make([]auditEntryView, 0, len(lines))
	for _, line := range lines {
		var e AuditEntry
		if jErr := json.Unmarshal([]byte(line), &e); jErr != nil {
			continue
		}
		if e.TS == "" {
			continue
		}
		entries = append(entries, auditEntryView{
			TS:         e.TS,
			Tool:       e.Tool,
			Status:     e.Status,
			DurationMS: e.DurationMS,
		})
	}
	return entries, nil
}

type auditEntryView struct {
	TS         string `json:"ts"`
	Tool       string `json:"tool"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
}

func (s *server) handleResourceStrategiesActive(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	var out map[string]any
	if err := s.cli.Get(ctx, "/api/strategies/active", nil, &out); err != nil {
		return nil, fmt.Errorf("resource strategies active: %w", err)
	}
	return resourceText("atlas://strategies/active", "application/json", mustJSON(out)), nil
}

func (s *server) handleResourceMarketRegime(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	var out map[string]any
	if err := s.cli.Get(ctx, "/api/regime/history?limit=1", nil, &out); err != nil {
		return nil, fmt.Errorf("resource market regime: %w", err)
	}
	return resourceText("atlas://market/regime", "application/json", mustJSON(out)), nil
}

func (s *server) handleResourceEventsToday(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	var out map[string]any
	if err := s.cli.Get(ctx, "/api/events/calendar", nil, &out); err != nil {
		return nil, fmt.Errorf("resource events today: %w", err)
	}
	return resourceText("atlas://events/today", "application/json", mustJSON(out)), nil
}
