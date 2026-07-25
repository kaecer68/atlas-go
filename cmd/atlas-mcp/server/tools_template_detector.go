package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTemplateDetectorTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "template_detector_status",
		Description: autoDescOr("template_detector_status", "Recent template trigger detector scan results from ledger. Returns the most recent DetectionResult rows (newest first), each with theme, severity, confidence, detected_at, source, and scan_batch_id. limit param defaults to 100."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleTemplateDetectorStatus)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "detector_registry_list",
		Description: autoDescOr("detector_registry_list", "All 24 template trigger detectors registered in the narrative.DetectorRegistry with current enable/disable state. Use to inspect which detectors are active or to verify registry wiring."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleDetectorRegistryList)
}

type templateDetectorStatusInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"max number of recent scans to return; default 100 if omitted or 0"`
}

func (s *server) handleTemplateDetectorStatus(ctx context.Context, _ *mcp.CallToolRequest, in templateDetectorStatusInput) (*mcp.CallToolResult, map[string]any, error) {
	path := "/api/detector/scan/status"
	if in.Limit > 0 {
		path = fmt.Sprintf("%s?limit=%d", path, in.Limit)
	}
	// Backend returns top-level JSON array; decode into slice then wrap.
	type scanRow struct {
		ScanID      int64   `json:"scan_id"`
		ScanBatchID string  `json:"scan_batch_id"`
		Theme       string  `json:"theme"`
		Severity    string  `json:"severity"`
		Confidence  float64 `json:"confidence"`
		DetectedAt  string  `json:"detected_at"`
		Source      string  `json:"source"`
	}
	var rows []scanRow
	if err := s.withAudit(ctx, "template_detector_status", []string{"limit"}, func() error {
		return s.cli.Get(ctx, path, nil, &rows)
	}); err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"scans": rows, "total": len(rows)}, nil
}

func (s *server) handleDetectorRegistryList(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	// Backend returns top-level JSON array; decode into slice then wrap.
	type detectorEntry struct {
		Theme   string `json:"theme"`
		Enabled bool   `json:"enabled"`
	}
	var detectors []detectorEntry
	if err := s.withAudit(ctx, "detector_registry_list", nil, func() error {
		return s.cli.Get(ctx, "/api/detector/registry/list", nil, &detectors)
	}); err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"detectors": detectors, "total": len(detectors)}, nil
}
