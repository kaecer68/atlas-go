package descgen

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// schemaMap returns ToolDesc.InputSchema as a map[string]any, or empty map if nil/empty.
func schemaMap(t *testing.T, d ToolDesc) map[string]any {
	t.Helper()
	if d.InputSchema == nil {
		return map[string]any{}
	}
	m, ok := d.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("InputSchema is %T, expected map[string]any", d.InputSchema)
	}
	return m
}

// goSrc is a helper to embed Go source code with backtick-containing struct tags.
// It uses ${BT} as a placeholder that gets replaced with backtick.
func goSrc(s string) string {
	return strings.ReplaceAll(s, "${BT}", "`")
}

func writeTestGoFile(t *testing.T, name string, content string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return dir, path
}

func TestExtract_ManualOverrideSkipped(t *testing.T) {
	src := goSrc(`package server

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTestTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "auto_tool",
		Description: "should be auto-generated",
	}, s.handleAutoTool)

	// gen:manual-override
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "manual_tool",
		Description: "this one is manual",
	}, s.handleManualTool)
}

// gen:manual-override — kept as manual, not auto-generated
func (s *server) handleManualTool(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}

func (s *server) handleAutoTool(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}
`)
	dir, _ := writeTestGoFile(t, "tools_testgen.go", src)
	result, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if _, ok := result["manual_tool"]; ok {
		t.Error("manual_tool should be skipped (gen:manual-override)")
	}
	if _, ok := result["auto_tool"]; !ok {
		t.Error("auto_tool should be included")
	}
}

func TestExtract_OmitemptyNotRequired(t *testing.T) {
	src := goSrc(`package server

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTestTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "omitempty_test",
		Description: "test omitempty",
	}, s.handleOmitemptyTest)
}

type omitemptyTestInput struct {
	Name  string ${BT}json:"name"${BT}
	Debug bool   ${BT}json:"debug,omitempty"${BT}
}

func (s *server) handleOmitemptyTest(ctx context.Context, _ *mcp.CallToolRequest, in omitemptyTestInput) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}
`)
	dir, _ := writeTestGoFile(t, "tools_testgen.go", src)
	result, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	desc, ok := result["omitempty_test"]
	if !ok {
		t.Fatal("omitempty_test not found in result")
	}

	schema := schemaMap(t, desc)
	required, _ := schema["required"].([]string)
	for _, r := range required {
		if r == "debug" {
			t.Error("debug should NOT be in required (has omitempty)")
		}
	}
	if !contains(required, "name") {
		t.Error("name should be in required (no omitempty)")
	}
}

func TestExtract_EnumStructType(t *testing.T) {
	src := goSrc(`package server

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTestTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "enum_test",
		Description: "test enum",
	}, s.handleEnumTest)
}

type enumTestInput struct {
	Direction string ${BT}json:"direction" enum:"buy,sell,hold"${BT}
}

func (s *server) handleEnumTest(ctx context.Context, _ *mcp.CallToolRequest, in enumTestInput) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}
`)
	dir, _ := writeTestGoFile(t, "tools_testgen.go", src)
	result, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	desc, ok := result["enum_test"]
	if !ok {
		t.Fatal("enum_test not found in result")
	}

	schema := schemaMap(t, desc)
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties is not a map")
	}
	dirProp, ok := props["direction"].(map[string]any)
	if !ok {
		t.Fatal("direction property not found")
	}
	enumVals, ok := dirProp["enum"].([]string)
	if !ok || len(enumVals) != 3 {
		t.Fatalf("direction enum: got %v, want [buy sell hold]", dirProp["enum"])
	}
	if enumVals[0] != "buy" || enumVals[1] != "sell" || enumVals[2] != "hold" {
		t.Errorf("direction enum values: got %v, want [buy sell hold]", enumVals)
	}
}

func TestExtract_NestedStruct(t *testing.T) {
	src := goSrc(`package server

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTestTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "nested_test",
		Description: "test nested",
	}, s.handleNestedTest)
}

type nestedInner struct {
	Value string ${BT}json:"value"${BT}
}

type nestedTestInput struct {
	Inner nestedInner ${BT}json:"inner"${BT}
	Flag  bool        ${BT}json:"flag,omitempty"${BT}
}

func (s *server) handleNestedTest(ctx context.Context, _ *mcp.CallToolRequest, in nestedTestInput) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}
`)
	dir, _ := writeTestGoFile(t, "tools_testgen.go", src)
	result, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	desc, ok := result["nested_test"]
	if !ok {
		t.Fatal("nested_test not found in result")
	}

	schema := schemaMap(t, desc)
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties is not a map")
	}
	inner, ok := props["inner"].(map[string]any)
	if !ok {
		t.Fatal("inner property not found or not a map")
	}
	if inner["type"] != "object" {
		t.Errorf("inner type: got %v, want object", inner["type"])
	}
	innerProps, ok := inner["properties"].(map[string]any)
	if !ok {
		t.Fatal("inner.properties not found")
	}
	if _, ok := innerProps["value"]; !ok {
		t.Error("inner should have field 'value'")
	}
}

func TestExtract_PointerVsValue(t *testing.T) {
	src := goSrc(`package server

import (
	"context"
	"time"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTestTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "pointer_test",
		Description: "test pointer vs value",
	}, s.handlePointerTest)
}

type pointerTestInput struct {
	Timeout    *time.Duration ${BT}json:"timeout,omitempty" jsonschema:"timeout in seconds"${BT}
	RetryDelay time.Duration  ${BT}json:"retry_delay" jsonschema:"delay between retries"${BT}
}

func (s *server) handlePointerTest(ctx context.Context, _ *mcp.CallToolRequest, in pointerTestInput) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}
`)
	dir, _ := writeTestGoFile(t, "tools_testgen.go", src)
	result, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	desc, ok := result["pointer_test"]
	if !ok {
		t.Fatal("pointer_test not found in result")
	}

	schema := schemaMap(t, desc)
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties is not a map")
	}

	timeoutProp, ok := props["timeout"].(map[string]any)
	if !ok {
		t.Fatal("timeout property not found")
	}
	if timeoutProp["type"] != "number" {
		t.Errorf("timeout type: got %v, want number (time.Duration → seconds)", timeoutProp["type"])
	}

	retryProp, ok := props["retry_delay"].(map[string]any)
	if !ok {
		t.Fatal("retry_delay property not found")
	}
	if retryProp["type"] != "number" {
		t.Errorf("retry_delay type: got %v, want number", retryProp["type"])
	}

	required, _ := schema["required"].([]string)
	if contains(required, "timeout") {
		t.Error("timeout should NOT be in required (pointer + omitempty)")
	}
	if !contains(required, "retry_delay") {
		t.Error("retry_delay should be in required (no omitempty)")
	}
}

func contains(s []string, v string) bool {
	return slices.Contains(s, v)
}

func TestExtract_DescriptionFromContext(t *testing.T) {
	// Existing human-written Description from mcp.Tool{} literal is preserved.
	src := goSrc(`package server

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTestTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "desc_test",
		Description: "human-written description kept as-is",
	}, s.handleDescTest)
}

// handleDescTest returns the current market regime assessment.
func (s *server) handleDescTest(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}
`)
	dir, _ := writeTestGoFile(t, "tools_testgen.go", src)
	result, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	desc, ok := result["desc_test"]
	if !ok {
		t.Fatal("desc_test not found in result")
	}

	if desc.Description != "human-written description kept as-is" {
		t.Errorf("description should preserve human-written value, got: %q", desc.Description)
	}
}

func TestExtract_NoInputStruct(t *testing.T) {
	src := goSrc(`package server

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTestTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "no_input_test",
		Description: "test no input",
	}, s.handleNoInput)
}

func (s *server) handleNoInput(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}
`)
	dir, _ := writeTestGoFile(t, "tools_testgen.go", src)
	result, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	desc, ok := result["no_input_test"]
	if !ok {
		t.Fatal("no_input_test not found in result")
	}

	schema := schemaMap(t, desc)
	if schema["type"] != "object" {
		t.Errorf("type: got %v, want object", schema["type"])
	}
}

func TestExtract_GoTypesToSchema(t *testing.T) {
	src := goSrc(`package server

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTestTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "types_test",
		Description: "test types",
	}, s.handleTypesTest)
}

type typesTestInput struct {
	Count  int     ${BT}json:"count" jsonschema:"number of items"${BT}
	Label  string  ${BT}json:"label" jsonschema:"display label"${BT}
	Active bool    ${BT}json:"active,omitempty"${BT}
	Values []int   ${BT}json:"values,omitempty"${BT}
}

func (s *server) handleTypesTest(ctx context.Context, _ *mcp.CallToolRequest, in typesTestInput) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}
`)
	dir, _ := writeTestGoFile(t, "tools_testgen.go", src)
	result, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	desc, ok := result["types_test"]
	if !ok {
		t.Fatal("types_test not found in result")
	}

	schema := schemaMap(t, desc)
	props, _ := schema["properties"].(map[string]any)

	if p, ok := props["count"].(map[string]any); ok {
		if p["type"] != "integer" {
			t.Errorf("count type: got %v, want integer", p["type"])
		}
		if p["description"] != "number of items" {
			t.Errorf("count description: got %v, want 'number of items'", p["description"])
		}
	}

	if p, ok := props["label"].(map[string]any); ok {
		if p["type"] != "string" {
			t.Errorf("label type: got %v, want string", p["type"])
		}
	}

	if p, ok := props["active"].(map[string]any); ok {
		if p["type"] != "boolean" {
			t.Errorf("active type: got %v, want boolean", p["type"])
		}
	}

	if p, ok := props["values"].(map[string]any); ok {
		if p["type"] != "array" {
			t.Errorf("values type: got %v, want array", p["type"])
		}
	}
}

// TestExtract_AtLeast60AutoGenerated verifies that Extract can describe
// at least 60 of the 74 production MCP tools from the real server source.
func TestExtract_AtLeast60AutoGenerated(t *testing.T) {
	result, err := Extract("../../server")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(result) < 60 {
		t.Errorf("Extract produced %d tools, want >= 60", len(result))
	}
	t.Logf("Extract produced %d tools", len(result))
}

// TestHasManualOverride_CommentOnFuncDecl verifies that a "gen:manual-override"
// doc comment on the handler function itself causes the tool to be excluded.
func TestHasManualOverride_CommentOnFuncDecl(t *testing.T) {
	src := goSrc(`package server

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTestTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "funcdecl_tool",
		Description: "should be skipped",
	}, s.handleFuncDeclTool)
}

// gen:manual-override
func (s *server) handleFuncDeclTool(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}
`)
	dir, _ := writeTestGoFile(t, "tools_testgen.go", src)
	result, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if _, ok := result["funcdecl_tool"]; ok {
		t.Error("funcdecl_tool should be excluded (gen:manual-override on func decl)")
	}
}

// TestHasManualOverride_CommentOnCallExpr verifies that a "gen:manual-override"
// comment immediately before the mcp.AddTool call causes the tool to be excluded.
func TestHasManualOverride_CommentOnCallExpr(t *testing.T) {
	src := goSrc(`package server

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTestTools(mcpSrv *mcp.Server, s *server) {
	// gen:manual-override
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "callexpr_tool",
		Description: "should be skipped",
	}, s.handleCallExprTool)
}

func (s *server) handleCallExprTool(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}
`)
	dir, _ := writeTestGoFile(t, "tools_testgen.go", src)
	result, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if _, ok := result["callexpr_tool"]; ok {
		t.Error("callexpr_tool should be excluded (gen:manual-override on call expr)")
	}
}

// TestHasManualOverride_NoOverride verifies that tools have comment
// that does NOT contain "gen:manual-override" are included.
func TestHasManualOverride_NoOverride(t *testing.T) {
	src := goSrc(`package server

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTestTools(mcpSrv *mcp.Server, s *server) {
	// This is a normal comment — not a manual override.
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "normal_tool",
		Description: "should be included",
	}, s.handleNormalTool)
}

func (s *server) handleNormalTool(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return nil, nil, nil
}
`)
	dir, _ := writeTestGoFile(t, "tools_testgen.go", src)
	result, err := Extract(dir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if _, ok := result["normal_tool"]; !ok {
		t.Error("normal_tool should be included (no gen:manual-override)")
	}
}
