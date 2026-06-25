package adapters

import (
	"context"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm_annotator"
)

// mockRouter implements llm.Router for testing RouterAnnotator.
// It returns a configurable output and error, and records the last request
// for assertion in tests.
type mockRouter struct {
	output  string
	err     error
	lastReq *llm.Request
}

func (m *mockRouter) Call(_ context.Context, req llm.Request) (llm.Response, error) {
	m.lastReq = &req
	return llm.Response{Output: m.output}, m.err
}

func (m *mockRouter) Health() map[llm.Provider]llm.HealthStatus {
	return nil
}

func (m *mockRouter) Register(llm.ProviderImpl) error {
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestRouterAnnotator_Success verifies that a successful router call returns
// the expected output string.
func TestRouterAnnotator_Success(t *testing.T) {
	expectedOutput := "外資賣超導致策略未觸發"
	mock := &mockRouter{output: expectedOutput}
	ra := NewRouterAnnotator(mock)

	fc := llm_annotator.FailureContext{
		FrameID:   "frame-1",
		FrameName: "趨勢跟隨",
		Layer:     "signal",
	}
	ctx := context.Background()
	got, err := ra.Annotate(ctx, fc)

	if err != nil {
		t.Fatalf("Annotate() unexpected error: %v", err)
	}
	if got != expectedOutput {
		t.Errorf("Annotate() = %q, want %q", got, expectedOutput)
	}
}

// TestRouterAnnotator_RouterError verifies that a router error is propagated.
func TestRouterAnnotator_RouterError(t *testing.T) {
	expectedErr := errors.New("upstream failure")
	mock := &mockRouter{err: expectedErr}
	ra := NewRouterAnnotator(mock)

	fc := llm_annotator.FailureContext{FrameID: "f1"}
	ctx := context.Background()
	_, err := ra.Annotate(ctx, fc)

	if err == nil {
		t.Fatal("Annotate() expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Annotate() error = %v, want %v", err, expectedErr)
	}
}

// TestRouterAnnotator_EmptyResponse verifies that an empty output from the
// router is returned as an empty string without error.
func TestRouterAnnotator_EmptyResponse(t *testing.T) {
	mock := &mockRouter{output: ""}
	ra := NewRouterAnnotator(mock)

	fc := llm_annotator.FailureContext{FrameID: "f1"}
	ctx := context.Background()
	got, err := ra.Annotate(ctx, fc)

	if err != nil {
		t.Fatalf("Annotate() unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("Annotate() = %q, want %q", got, "")
	}
}

// TestRouterAnnotator_NilRouter verifies that a nil router returns ("", nil).
func TestRouterAnnotator_NilRouter(t *testing.T) {
	ra := NewRouterAnnotator(nil)

	fc := llm_annotator.FailureContext{FrameID: "f1"}
	ctx := context.Background()
	got, err := ra.Annotate(ctx, fc)

	if err != nil {
		t.Fatalf("Annotate() unexpected error with nil router: %v", err)
	}
	if got != "" {
		t.Errorf("Annotate() = %q, want %q", got, "")
	}
}

// TestRouterAnnotator_Name verifies the name format.
func TestRouterAnnotator_Name(t *testing.T) {
	ra := NewRouterAnnotator(nil)
	expected := "router(minimax→deepseek→mock)"
	if got := ra.Name(); got != expected {
		t.Errorf("Name() = %q, want %q", got, expected)
	}
}

// TestRouterAnnotator_CorrectCapability verifies that the request dispatched
// to the router has CapabilityFailureAttribution and the FailureContext payload.
func TestRouterAnnotator_CorrectCapability(t *testing.T) {
	mock := &mockRouter{output: "ok"}
	ra := NewRouterAnnotator(mock)

	fc := llm_annotator.FailureContext{
		FrameID:   "frame-42",
		FrameName: "均值回歸",
		Layer:     "signal",
	}
	ctx := context.Background()
	_, err := ra.Annotate(ctx, fc)
	if err != nil {
		t.Fatalf("Annotate() unexpected error: %v", err)
	}

	if mock.lastReq == nil {
		t.Fatal("Annotate() did not call router.Call")
	}
	if mock.lastReq.Capability != llm.CapabilityFailureAttribution {
		t.Errorf("request.Capability = %q, want %q",
			mock.lastReq.Capability, llm.CapabilityFailureAttribution)
	}
	if mock.lastReq.DataClass != llm.DataClassNonRegulated {
		t.Errorf("request.DataClass = %v, want %v",
			mock.lastReq.DataClass, llm.DataClassNonRegulated)
	}

	reqFC, ok := mock.lastReq.Payload.(llm_annotator.FailureContext)
	if !ok {
		t.Fatalf("request.Payload type = %T, want llm_annotator.FailureContext", mock.lastReq.Payload)
	}
	if reqFC.FrameID != fc.FrameID {
		t.Errorf("request.Payload.FrameID = %q, want %q", reqFC.FrameID, fc.FrameID)
	}
	if reqFC.FrameName != fc.FrameName {
		t.Errorf("request.Payload.FrameName = %q, want %q", reqFC.FrameName, fc.FrameName)
	}
}
