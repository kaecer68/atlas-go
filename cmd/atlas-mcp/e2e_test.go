package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestStdioJSONRPC_HappyPath starts the compiled atlas-mcp binary in a child
// process with stdio transport, sends a real JSON-RPC `initialize` and
// `tools/list`, and asserts that the 5 Phase 1 tools come back.
//
// Requirements for the test:
//   - go binary in PATH (uses `go run ./cmd/atlas-mcp`)
//   - ATLAS_BASE_URL points to a local mock server
//   - ATLAS_MCP_TOKEN unset (dev mode)
func TestStdioJSONRPC_HappyPath(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary required")
	}

	mockAtlas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer mockAtlas.Close()

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repoRoot := mustRepoRoot(t)
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/atlas-mcp")
	cmd.Dir = repoRoot
	cmd.Env = append(
		os.Environ(),
		"ATLAS_BASE_URL="+mockAtlas.URL,
		"ATLAS_MCP_AUDIT_LOG="+auditPath,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Send `initialize` (per JSON-RPC over MCP): sends empty {} as params.
	if err := sendJSONRPC(stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "e2e", "version": "v0"},
		},
	}); err != nil {
		t.Fatalf("send initialize: %v", err)
	}
	initResp, err := readJSONRPC(stdout)
	if err != nil {
		t.Fatalf("read initialize: %v", err)
	}
	if _, ok := initResp["result"]; !ok {
		t.Fatalf("initialize response missing result: %v", initResp)
	}

	// `tools/list` (no params needed in MCP).
	if err := sendJSONRPC(stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	}); err != nil {
		t.Fatalf("send tools/list: %v", err)
	}
	listResp, err := readJSONRPC(stdout)
	if err != nil {
		t.Fatalf("read tools/list: %v", err)
	}
	result, ok := listResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list result is not object: %v", listResp)
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list.tools is not array: %v", result)
	}
	gotNames := make([]string, 0, len(tools))
	for _, x := range tools {
		if obj, ok := x.(map[string]any); ok {
			if n, ok := obj["name"].(string); ok {
				gotNames = append(gotNames, n)
			}
		}
	}
	wantSet := map[string]bool{
		"regime_get_history":        true,
		"strategy_list_active":      true,
		"experiment_judge":          true,
		"alert_list_unacknowledged": true,
		"system_get_health":         true,
	}
	gotSet := make(map[string]bool, len(gotNames))
	for _, n := range gotNames {
		gotSet[n] = true
	}
	for want := range wantSet {
		if !gotSet[want] {
			t.Fatalf("missing tool %q (got %v)", want, gotNames)
		}
	}

	// Clean shutdown: close stdin to allow the child to exit cleanly.
	_ = stdin.Close()
}

// --- helpers -----------------------------------------------------------------

func sendJSONRPC(w io.Writer, msg map[string]any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// mustRepoRoot returns the absolute path of the atlas-go repo root by walking
// up two directories from the test file's location
// (cmd/atlas-mcp/e2e_test.go → repo root).
func mustRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	abs, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return abs
}

func readJSONRPC(r io.Reader) (map[string]any, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			return nil, fmt.Errorf("decode %q: %w", line, err)
		}
		return m, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// guard the import of strings.
var _ = strings.NewReplacer
