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
		Description: "Live ParametersConfig from atlas-go admin endpoint. Live state, not on-disk snapshot. Read-only.",
		MIMEType:    "application/json",
	}, s.handleResourceConfigParameters)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://tools/catalog",
		Name:        "atlas-mcp Tool Catalog",
		Description: "112 read-only MCP tools grouped by area. Source: docs/reference/tool-catalog.md on disk. Use to enumerate capabilities without tools/list round-trip.",
		MIMEType:    "text/markdown",
	}, s.handleResourceToolsCatalog)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://workflows/catalog",
		Name:        "atlas-go Workflow Catalog",
		Description: "42 WA-XXX workflows in 7 layers. Source: docs/workflow-map.md on disk. Helps an agent decide which Tool maps to a given intent.",
		MIMEType:    "text/markdown",
	}, s.handleResourceWorkflowsCatalog)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://audit/recent",
		Name:        "Recent Audit Log Entries",
		Description: "Last 50 entries from the JSONL audit log (most recent first). Useful for debugging recent agent activity without a separate log query.",
		MIMEType:    "application/json",
	}, s.handleResourceAuditRecent)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://strategies/active",
		Name:        "Active Strategy Definitions",
		Description: "Current active strategies in the production strategy set. Live data from /api/strategies/active.",
		MIMEType:    "application/json",
	}, s.handleResourceStrategiesActive)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://market/regime",
		Name:        "Latest Market Regime",
		Description: "Current market regime classification (RISK_ON / RISK_OFF / NEUTRAL / TRANSITIONAL). Live data from /api/regime/history.",
		MIMEType:    "application/json",
	}, s.handleResourceMarketRegime)

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "atlas://events/today",
		Name:        "Today's Market Events",
		Description: "Upcoming and active Taiwan market events for today. Live data from /api/events/calendar.",
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
	raw, err := os.ReadFile("docs/reference/tool-catalog.md")
	if err != nil {
		return nil, fmt.Errorf("resource tools catalog (docs/reference/tool-catalog.md): %w", err)
	}
	return resourceText("atlas://tools/catalog", "text/markdown", string(raw)), nil
}

func (s *server) handleResourceWorkflowsCatalog(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	raw, err := os.ReadFile("docs/workflow-map.md")
	if err != nil {
		return nil, fmt.Errorf("resource workflows catalog (docs/workflow-map.md): %w", err)
	}
	return resourceText("atlas://workflows/catalog", "text/markdown", string(raw)), nil
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
	offset := info.Size() - tailSize
	if offset < 0 {
		offset = 0
	}
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
