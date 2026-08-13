package server

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRegisterPrompts_NoPanic(t *testing.T) {
	// Smoke test: confirm all 9 prompts register on a fresh *mcp.Server
	// without panicking. The SDK v1.6.1 has no public ListPrompts, so we
	// rely on the AddPrompt call not panicking as a registration signal.
	mcpSrv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0"}, &mcp.ServerOptions{})
	registerPrompts(mcpSrv)
}

func TestHandleDailyMarketBriefing_ReturnsInstructionText(t *testing.T) {
	res, err := handleDailyMarketBriefing(context.Background(), &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res == nil || len(res.Messages) != 1 {
		t.Fatalf("expected 1 message, got %+v", res)
	}
	tc, ok := res.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Messages[0].Content)
	}
	for _, want := range []string{
		"macro_get_snapshot_latest",
		"narrative_get_bundle",
		"risk_get_metrics",
		"system_get_metrics",
	} {
		if !strings.Contains(tc.Text, want) {
			t.Errorf("expected body to mention %q, got: %s", want, tc.Text)
		}
	}
}

func TestHandleRiskCheck_ReturnsInstructionText(t *testing.T) {
	res, err := handleRiskCheck(context.Background(), &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res == nil || len(res.Messages) != 1 {
		t.Fatalf("expected 1 message, got %+v", res)
	}
	tc := res.Messages[0].Content.(*mcp.TextContent)
	for _, want := range []string{
		"risk_get_metrics",
		"risk_get_correlation_matrix",
		"risk_get_drawdown",
		"risk_get_calibration",
	} {
		if !strings.Contains(tc.Text, want) {
			t.Errorf("expected body to mention %q, got: %s", want, tc.Text)
		}
	}
}

func TestHandleRegimeInterpretation_RISK_ON(t *testing.T) {
	res, err := handleRegimeInterpretation(context.Background(), &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Arguments: map[string]string{"regime": "RISK_ON"},
		},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res == nil || len(res.Messages) != 1 {
		t.Fatalf("expected 1 message, got %+v", res)
	}
	tc := res.Messages[0].Content.(*mcp.TextContent)
	for _, want := range []string{
		"The current regime is: RISK_ON",
		"narrative_get_bundle",
		"macro_get_snapshot_latest",
	} {
		if !strings.Contains(tc.Text, want) {
			t.Errorf("expected body to contain %q, got: %s", want, tc.Text)
		}
	}
}

func TestHandleRegimeInterpretation_AllFourRegimes(t *testing.T) {
	cases := []string{"RISK_ON", "RISK_OFF", "NEUTRAL", "TRANSITIONAL"}
	for _, regime := range cases {
		res, err := handleRegimeInterpretation(context.Background(), &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Arguments: map[string]string{"regime": regime},
			},
		})
		if err != nil {
			t.Errorf("regime=%s: %v", regime, err)
			continue
		}
		tc := res.Messages[0].Content.(*mcp.TextContent)
		want := "The current regime is: " + regime
		if !strings.Contains(tc.Text, want) {
			t.Errorf("regime=%s not substituted, got: %s", regime, tc.Text)
		}
	}
}

func TestHandleRegimeInterpretation_MissingArgument(t *testing.T) {
	_, err := handleRegimeInterpretation(context.Background(), &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{},
	})
	if err == nil {
		t.Fatal("expected error when 'regime' arg is missing")
	}
	if !strings.Contains(err.Error(), "regime") {
		t.Errorf("error should mention 'regime', got: %v", err)
	}
}

func TestHandleRegimeInterpretation_EmptyArgument(t *testing.T) {
	_, err := handleRegimeInterpretation(context.Background(), &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Arguments: map[string]string{"regime": ""},
		},
	})
	if err == nil {
		t.Fatal("expected error when 'regime' arg is empty string")
	}
}

func TestHandleRegimeInterpretation_NilParams(t *testing.T) {
	_, err := handleRegimeInterpretation(context.Background(), &mcp.GetPromptRequest{
		Params: nil,
	})
	if err == nil {
		t.Fatal("expected error when params is nil")
	}
	if !strings.Contains(err.Error(), "regime") {
		t.Errorf("error should mention 'regime', got: %v", err)
	}
}

func TestHandleSystemIntrospection_ReturnsInstructionText(t *testing.T) {
	res, err := handleSystemIntrospection(context.Background(), &mcp.GetPromptRequest{Params: &mcp.GetPromptParams{}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res == nil || len(res.Messages) != 1 {
		t.Fatalf("expected 1 message, got %+v", res)
	}
	tc := res.Messages[0].Content.(*mcp.TextContent)
	for _, want := range []string{"atlas://tools/catalog", "atlas://workflows/catalog", "audit_state", "system_get_health", "system_get_maturity"} {
		if !strings.Contains(tc.Text, want) {
			t.Errorf("expected body to mention %q, got: %s", want, tc.Text)
		}
	}
}

func TestHandleMcpObservabilityReview_ReturnsInstructionText(t *testing.T) {
	res, err := handleMcpObservabilityReview(context.Background(), &mcp.GetPromptRequest{Params: &mcp.GetPromptParams{}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	tc := res.Messages[0].Content.(*mcp.TextContent)
	for _, want := range []string{"mcp_get_call_stats", "mcp_get_session_topology", "mcp_get_top_slow_tools", "mcp_anomaly_get_recent"} {
		if !strings.Contains(tc.Text, want) {
			t.Errorf("expected body to mention %q, got: %s", want, tc.Text)
		}
	}
}

func TestHandleConstitutionAuditWalkthrough_ReturnsInstructionText(t *testing.T) {
	res, err := handleConstitutionAuditWalkthrough(context.Background(), &mcp.GetPromptRequest{Params: &mcp.GetPromptParams{}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	tc := res.Messages[0].Content.(*mcp.TextContent)
	for _, want := range []string{"audit_state", "ATLAS_CONSTITUTION_AUDIT.md"} {
		if !strings.Contains(tc.Text, want) {
			t.Errorf("expected body to mention %q, got: %s", want, tc.Text)
		}
	}
}
