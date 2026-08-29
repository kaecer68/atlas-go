package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kaecer68/atlas-go/internal/alerting"
)

// RootsConfig configures the read-only roots filesystem boundary.
type RootsConfig struct {
	AllowedRoots []string // static allow-list of file:// roots (fallback when client does not declare roots)
	ReadSizeCap  int64    // maximum bytes a single mcp_roots_read_file may read (default 1MB)
}

const defaultRootsReadSizeCap = 1 * 1024 * 1024

func (c RootsConfig) readSizeCap() int64 {
	if c.ReadSizeCap > 0 {
		return c.ReadSizeCap
	}
	return defaultRootsReadSizeCap
}

// MCPRootsListOutput is the response schema for mcp_roots_list.
type MCPRootsListOutput struct {
	Roots []string `json:"roots"`
}

// MCPRootsReadFileInput is the request schema for mcp_roots_read_file.
type MCPRootsReadFileInput struct {
	Path string `json:"path" jsonschema:"file:// URI to read, must be under a declared root"`
}

// MCPRootsReadFileOutput is the response schema for mcp_roots_read_file.
type MCPRootsReadFileOutput struct {
	Content string `json:"content"`
}

func registerRootsTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_roots_list",
		Description: autoDescOr("mcp_roots_list", "List the file:// roots declared by the connected MCP client."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleMCPRootsList)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_roots_read_file",
		Description: autoDescOr("mcp_roots_read_file", "Read a file under a declared file:// root. Read-only, path-traversal hardened, and audited."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleMCPRootsReadFile)
}

func (s *server) handleRootsListChanged(ctx context.Context, req *mcp.RootsListChangedRequest) error {
	if req == nil || req.Session == nil {
		return errors.New("roots_list_changed: missing session")
	}
	err := s.refreshRootsCache(ctx, req.Session)
	s.publishRootsChangedAlert(ctx, err)
	return err
}

func (s *server) refreshRootsCache(ctx context.Context, ss *mcp.ServerSession) error {
	res, err := ss.ListRoots(ctx, nil)
	if err != nil {
		return fmt.Errorf("roots_list_changed: list roots: %w", err)
	}
	uris := make([]string, 0, len(res.Roots))
	for _, r := range res.Roots {
		uris = append(uris, r.URI)
	}
	s.rootsMu.Lock()
	s.rootsCache = uris
	s.rootsMu.Unlock()
	return nil
}

func (s *server) publishRootsChangedAlert(ctx context.Context, refreshErr error) {
	if s.alerter == nil {
		return
	}
	severity := alerting.SeverityInfo
	var message string
	annotations := map[string]string{}
	if refreshErr != nil {
		severity = alerting.SeverityWarning
		message = "MCP roots list_changed refresh failed"
		annotations["error"] = refreshErr.Error()
	} else {
		uris := s.cachedRoots()
		message = fmt.Sprintf("MCP roots list_changed applied: %d root(s)", len(uris))
		if len(uris) > 0 {
			annotations["root_uri"] = uris[0]
		}
		annotations["root_count"] = strconv.Itoa(len(uris))
	}
	alert := alerting.Alert{
		Type:        alerting.EventSecurityRootsChanged,
		Severity:    severity,
		Source:      "atlas-mcp",
		Message:     message,
		Labels:      map[string]string{"alertname": "security_roots_changed"},
		Annotations: annotations,
		Timestamp:   time.Now().UTC(),
	}
	_ = s.alerter.Publish(ctx, alert)
}

func (s *server) handleMCPRootsList(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, MCPRootsListOutput, error) {
	if req == nil || req.Session == nil {
		return nil, MCPRootsListOutput{}, errors.New("mcp_roots_list: missing server session")
	}
	if !clientSupportsRoots(req.Session.InitializeParams()) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "unsupported"}},
		}, MCPRootsListOutput{}, nil
	}

	res, err := req.Session.ListRoots(ctx, nil)
	if err != nil {
		return nil, MCPRootsListOutput{}, fmt.Errorf("mcp_roots_list: list roots: %w", err)
	}
	uris := make([]string, 0, len(res.Roots))
	for _, r := range res.Roots {
		uris = append(uris, r.URI)
	}
	_ = s.refreshRootsCache(ctx, req.Session)
	return nil, MCPRootsListOutput{Roots: uris}, nil
}

func (s *server) handleMCPRootsReadFile(ctx context.Context, req *mcp.CallToolRequest, in MCPRootsReadFileInput) (*mcp.CallToolResult, MCPRootsReadFileOutput, error) {
	if req == nil || req.Session == nil {
		return nil, MCPRootsReadFileOutput{}, errors.New("mcp_roots_read_file: missing server session")
	}
	ss := req.Session
	if !clientSupportsRoots(ss.InitializeParams()) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "unsupported"}},
		}, MCPRootsReadFileOutput{}, nil
	}

	path, err := fileURIToPath(in.Path)
	if err != nil {
		return nil, MCPRootsReadFileOutput{}, fmt.Errorf("mcp_roots_read_file: %w", err)
	}
	if strings.ContainsRune(in.Path, '?') || strings.ContainsRune(in.Path, '#') {
		return nil, MCPRootsReadFileOutput{}, errors.New("mcp_roots_read_file: write flags or fragments are not allowed")
	}

	roots, err := s.resolveRoots(ctx, ss)
	if err != nil {
		return nil, MCPRootsReadFileOutput{}, fmt.Errorf("mcp_roots_read_file: resolve roots: %w", err)
	}

	absPath, err := filepath.Abs(filepath.FromSlash(path))
	if err != nil {
		return nil, MCPRootsReadFileOutput{}, fmt.Errorf("mcp_roots_read_file: abs path: %w", err)
	}
	if !isUnderRoots(absPath, roots) {
		return nil, MCPRootsReadFileOutput{}, fmt.Errorf("mcp_roots_read_file: path outside declared roots")
	}

	var content []byte
	var readErr error
	extraFn := func() map[string]any {
		return map[string]any{
			"path":       in.Path,
			"size_bytes": len(content),
			"ts":         time.Now().UTC().Format(time.RFC3339Nano),
			"tenant_id":  TenantIDFromContext(ctx),
		}
	}

	if err := s.withAuditExtra(ctx, "mcp_roots_read_file", []string{"path"}, extraFn, func() error {
		f, openErr := readFileNoFollow(absPath)
		if openErr != nil {
			return fmt.Errorf("open: %w", openErr)
		}
		defer func() { _ = f.Close() }()

		info, statErr := f.Stat()
		if statErr != nil {
			return fmt.Errorf("stat: %w", statErr)
		}
		if !info.Mode().IsRegular() {
			return errors.New("not a regular file")
		}
		capBytes := s.cfg.Roots.readSizeCap()
		if info.Size() > capBytes {
			return fmt.Errorf("file size %d exceeds read cap %d", info.Size(), capBytes)
		}

		lr := io.LimitReader(f, capBytes)
		content, readErr = io.ReadAll(lr)
		if readErr != nil {
			return fmt.Errorf("read: %w", readErr)
		}
		return nil
	}); err != nil {
		return nil, MCPRootsReadFileOutput{}, err
	}

	return nil, MCPRootsReadFileOutput{Content: string(content)}, nil
}

func clientSupportsRoots(ip *mcp.InitializeParams) bool {
	return ip != nil && ip.Capabilities != nil && ip.Capabilities.RootsV2 != nil
}

func (s *server) resolveRoots(ctx context.Context, ss *mcp.ServerSession) ([]string, error) {
	res, err := ss.ListRoots(ctx, nil)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{})
	for _, r := range res.Roots {
		p, perr := fileURIToPath(r.URI)
		if perr != nil {
			continue
		}
		abs, aerr := filepath.Abs(filepath.FromSlash(p))
		if aerr != nil {
			continue
		}
		set[abs] = struct{}{}
	}
	for _, r := range s.cfg.Roots.AllowedRoots {
		p, perr := fileURIToPath(r)
		if perr != nil {
			continue
		}
		abs, aerr := filepath.Abs(filepath.FromSlash(p))
		if aerr != nil {
			continue
		}
		set[abs] = struct{}{}
	}
	roots := make([]string, 0, len(set))
	for r := range set {
		roots = append(roots, r)
	}
	return roots, nil
}

func fileURIToPath(uri string) (string, error) {
	const prefix = "file://"
	if !strings.HasPrefix(uri, prefix) {
		return "", fmt.Errorf("URI %q is not file://", uri)
	}
	return filepath.FromSlash(strings.TrimPrefix(uri, prefix)), nil
}

func isUnderRoots(target string, roots []string) bool {
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return false
	}
	realTarget = filepath.Clean(realTarget)
	for _, r := range roots {
		realR, err := filepath.EvalSymlinks(r)
		if err != nil {
			continue
		}
		realR = filepath.Clean(realR)
		if realTarget == realR {
			return true
		}
		sep := string(filepath.Separator)
		if strings.HasPrefix(realTarget+sep, realR+sep) {
			return true
		}
	}
	return false
}
