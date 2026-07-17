package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm_annotator"
)

// ---------------------------------------------------------------------------
// Supports
// ---------------------------------------------------------------------------

// TestAnnotatorAdapter_Supports_FailureAttribution verifies that a non-K2.7
// adapter supports CapabilityFailureAttribution.
func TestAnnotatorAdapter_Supports_FailureAttribution(t *testing.T) {
	// Given: a healthy adapter wrapping a MockAnnotator with a non-K2.7 model
	model := "moonshot-v1-8k"
	mock := llm_annotator.NewMock("reasonable explanation")
	adapter := NewAnnotatorAdapter(mock, model)

	// When: Supports is called with CapabilityFailureAttribution
	result := adapter.Supports(llm.CapabilityFailureAttribution)

	// Then: it returns true
	if !result {
		t.Errorf("Supports(CapabilityFailureAttribution) = false, want true for model %q", model)
	}
}

// TestAnnotatorAdapter_Supports_K2_7Rejection verifies the ADR-009 guard:
// kimi-k2.7 is rejected for non-code capabilities, including
// CapabilityFailureAttribution.
func TestAnnotatorAdapter_Supports_K2_7Rejection(t *testing.T) {
	// Given: an adapter with the kimi-k2.7 model
	mock := llm_annotator.NewMock("code review result")
	adapter := NewAnnotatorAdapter(mock, "kimi-k2.7")

	// When: Supports is called with CapabilityFailureAttribution
	result := adapter.Supports(llm.CapabilityFailureAttribution)

	// Then: it returns false per ADR-009
	if result {
		t.Error("Supports(CapabilityFailureAttribution) = true for kimi-k2.7, want false (ADR-009)")
	}
}

// TestAnnotatorAdapter_Supports_K2_7CodeCapabilities verifies that kimi-k2.7
// is allowed for code-review and prompt-lint capabilities per ADR-009.
func TestAnnotatorAdapter_Supports_K2_7CodeCapabilities(t *testing.T) {
	// Given: an adapter with the kimi-k2.7 model
	mock := llm_annotator.NewMock("code annotation")
	adapter := NewAnnotatorAdapter(mock, "kimi-k2.7")

	t.Run("CodeReviewAnnotation", func(t *testing.T) {
		// When: Supports is called with CapabilityCodeReviewAnnotation
		result := adapter.Supports(llm.CapabilityCodeReviewAnnotation)
		// Then: it returns true
		if !result {
			t.Error("Supports(CapabilityCodeReviewAnnotation) = false for kimi-k2.7, want true")
		}
	})

	t.Run("PromptLint", func(t *testing.T) {
		// When: Supports is called with CapabilityPromptLint
		result := adapter.Supports(llm.CapabilityPromptLint)
		// Then: it returns true
		if !result {
			t.Error("Supports(CapabilityPromptLint) = false for kimi-k2.7, want true")
		}
	})

	t.Run("NonCodeCapabilityAlsoRejected", func(t *testing.T) {
		// When: Supports is called with a non-code, non-attribution capability
		result := adapter.Supports(llm.CapabilityStrategySummary)
		// Then: it returns false
		if result {
			t.Error("Supports(CapabilityStrategySummary) = true for kimi-k2.7, want false")
		}
	})
}

// TestAnnotatorAdapter_Supports_OtherCapabilities verifies that a non-K2.7
// adapter only supports CapabilityFailureAttribution and nothing else.
func TestAnnotatorAdapter_Supports_OtherCapabilities(t *testing.T) {
	// Given: a healthy adapter with a non-K2.7 model
	mock := llm_annotator.NewMock("annotation")
	adapter := NewAnnotatorAdapter(mock, "moonshot-v1-8k")

	// When/Then: only CapabilityFailureAttribution is supported
	caps := []llm.Capability{
		llm.CapabilityCodeReviewAnnotation,
		llm.CapabilityPromptLint,
		llm.CapabilityRationaleGeneration,
		llm.CapabilityStrategySummary,
		llm.CapabilityRiskSurfaceExtraction,
		llm.CapabilityRegimeExplanation,
		llm.CapabilityScenarioSimulation,
		llm.CapabilityPerformanceForensics,
		llm.CapabilityContraAttribution,
	}
	for _, cap := range caps {
		if adapter.Supports(cap) {
			t.Errorf("Supports(%q) = true, want false (only failure_attribution is supported)", cap)
		}
	}
}

// TestAnnotatorAdapter_Supports_DisabledAdapter verifies that a nil annotator
// (disabled adapter) returns false for all capabilities.
func TestAnnotatorAdapter_Supports_DisabledAdapter(t *testing.T) {
	// Given: a disabled adapter (nil annotator)
	adapter := NewAnnotatorAdapter(nil, "any-model")

	// When/Then: all capabilities return false
	allCaps := []llm.Capability{
		llm.CapabilityFailureAttribution,
		llm.CapabilityCodeReviewAnnotation,
		llm.CapabilityPromptLint,
	}
	for _, cap := range allCaps {
		if adapter.Supports(cap) {
			t.Errorf("Supports(%q) = true for disabled adapter, want false", cap)
		}
	}
}

// ---------------------------------------------------------------------------
// Call
// ---------------------------------------------------------------------------

// TestAnnotatorAdapter_Call_Success verifies a successful Call returns a
// Response with the annotation, ProviderKimi, and zero Usage fields.
func TestAnnotatorAdapter_Call_Success(t *testing.T) {
	// Given: a healthy adapter wrapping a MockAnnotator that returns a known string
	expectedAnnotation := "外資賣超 + 台幣貶值，導致策略未觸發"
	mock := llm_annotator.NewMock(expectedAnnotation)
	adapter := NewAnnotatorAdapter(mock, "moonshot-v1-8k")

	fc := llm_annotator.FailureContext{
		FrameID:   "frame-1",
		FrameName: "趨勢跟隨",
		Layer:     "signal",
		Label:     "test-label",
	}
	req := llm.Request{
		Capability: llm.CapabilityFailureAttribution,
		Payload:    fc,
	}

	// When: Call is invoked
	ctx := context.Background()
	resp, err := adapter.Call(ctx, req)
	// Then: no error, output matches, provider is Kimi
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}
	if resp.Output != expectedAnnotation {
		t.Errorf("Call().Output = %q, want %q", resp.Output, expectedAnnotation)
	}
	if resp.Provider != llm.ProviderKimi {
		t.Errorf("Call().Provider = %q, want %q", resp.Provider, llm.ProviderKimi)
	}
	if resp.Latency < 0 {
		t.Error("Call().Latency should not be negative")
	}
}

// TestAnnotatorAdapter_Call_Disabled verifies that a nil annotator returns
// the sentinel error ErrAnnotatorDisabled.
func TestAnnotatorAdapter_Call_Disabled(t *testing.T) {
	// Given: a disabled adapter (nil annotator)
	adapter := NewAnnotatorAdapter(nil, "any-model")

	req := llm.Request{
		Capability: llm.CapabilityFailureAttribution,
		Payload:    llm_annotator.FailureContext{},
	}

	// When: Call is invoked
	ctx := context.Background()
	_, err := adapter.Call(ctx, req)

	// Then: it returns ErrAnnotatorDisabled
	if err == nil {
		t.Fatal("Call() expected error for disabled adapter, got nil")
	}
	if !errors.Is(err, ErrAnnotatorDisabled) {
		t.Errorf("Call() error = %v, want ErrAnnotatorDisabled", err)
	}
}

// TestAnnotatorAdapter_Call_WrongPayloadType verifies that passing a non-
// FailureContext payload returns an error.
func TestAnnotatorAdapter_Call_WrongPayloadType(t *testing.T) {
	// Given: a healthy adapter
	mock := llm_annotator.NewMock("something")
	adapter := NewAnnotatorAdapter(mock, "moonshot-v1-8k")

	req := llm.Request{
		Capability: llm.CapabilityFailureAttribution,
		Payload:    "this is not a FailureContext",
	}

	// When: Call is invoked
	ctx := context.Background()
	_, err := adapter.Call(ctx, req)

	// Then: it returns an error
	if err == nil {
		t.Fatal("Call() expected error for wrong payload type, got nil")
	}
}

// TestAnnotatorAdapter_Call_RecordsCall verifies that the adapter delegates
// to the underlying annotator and increments the mock's call count.
func TestAnnotatorAdapter_Call_RecordsCall(t *testing.T) {
	// Given: a healthy adapter wrapping a MockAnnotator
	mock := llm_annotator.NewMock("annotation")
	adapter := NewAnnotatorAdapter(mock, "moonshot-v1-8k")

	fc := llm_annotator.FailureContext{FrameID: "f1"}
	req := llm.Request{
		Capability: llm.CapabilityFailureAttribution,
		Payload:    fc,
	}

	// When: Call is invoked twice
	ctx := context.Background()
	_, _ = adapter.Call(ctx, req)
	_, _ = adapter.Call(ctx, req)

	// Then: the mock's call count is 2 (safe to read directly in a
	// sequential, non-concurrent test after Call has returned).
	if mock.Calls != 2 {
		t.Errorf("MockAnnotator.Calls = %d, want 2", mock.Calls)
	}
}

// TestAnnotatorAdapter_Call_ReturnsError verifies that an annotator error is
// propagated to the caller.
func TestAnnotatorAdapter_Call_ReturnsError(t *testing.T) {
	// Given: a healthy adapter wrapping a MockAnnotator that always errors
	mock := &llm_annotator.MockAnnotator{Err: errors.New("upstream failure")}
	adapter := NewAnnotatorAdapter(mock, "moonshot-v1-8k")

	req := llm.Request{
		Capability: llm.CapabilityFailureAttribution,
		Payload:    llm_annotator.FailureContext{},
	}

	// When: Call is invoked
	ctx := context.Background()
	_, err := adapter.Call(ctx, req)

	// Then: the error is propagated
	if err == nil {
		t.Fatal("Call() expected error from mock, got nil")
	}
	if err.Error() != "upstream failure" {
		t.Errorf("Call() error = %q, want %q", err.Error(), "upstream failure")
	}
}

// TestAnnotatorAdapter_Call_LatencyIsSet verifies that Latency is set on the
// Response after a successful Call.
func TestAnnotatorAdapter_Call_LatencyIsSet(t *testing.T) {
	// Given: a healthy adapter
	mock := llm_annotator.NewMock("response")
	adapter := NewAnnotatorAdapter(mock, "moonshot-v1-8k")

	req := llm.Request{
		Capability: llm.CapabilityFailureAttribution,
		Payload:    llm_annotator.FailureContext{},
	}

	// When: Call is invoked
	start := time.Now()
	ctx := context.Background()
	resp, err := adapter.Call(ctx, req)
	// Then: response has a non-negative Latency within reasonable bounds
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}
	if resp.Latency < 0 {
		t.Error("Call().Latency should not be negative")
	}
	if resp.Latency > time.Since(start)+time.Second {
		t.Errorf("Call().Latency = %v, suspiciously large", resp.Latency)
	}
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

// TestAnnotatorAdapter_Health_Healthy verifies that a non-nil annotator
// reports healthy status with ProviderKimi.
func TestAnnotatorAdapter_Health_Healthy(t *testing.T) {
	// Given: a healthy adapter
	mock := llm_annotator.NewMock("annotation")
	adapter := NewAnnotatorAdapter(mock, "moonshot-v1-8k")

	// When: Health is called
	status := adapter.Health()

	// Then: reports healthy with ProviderKimi
	if !status.Healthy {
		t.Error("Health().Healthy = false, want true for non-nil annotator")
	}
	if status.Provider != llm.ProviderKimi {
		t.Errorf("Health().Provider = %q, want %q", status.Provider, llm.ProviderKimi)
	}
	if status.LastError != "" {
		t.Errorf("Health().LastError = %q, want empty", status.LastError)
	}
}

// TestAnnotatorAdapter_Health_Disabled verifies that a nil annotator reports
// unhealthy status with a "disabled" error message.
func TestAnnotatorAdapter_Health_Disabled(t *testing.T) {
	// Given: a disabled adapter (nil annotator)
	adapter := NewAnnotatorAdapter(nil, "any-model")

	// When: Health is called
	status := adapter.Health()

	// Then: reports unhealthy with "disabled" error
	if status.Healthy {
		t.Error("Health().Healthy = true, want false for nil annotator")
	}
	if status.Provider != llm.ProviderKimi {
		t.Errorf("Health().Provider = %q, want %q", status.Provider, llm.ProviderKimi)
	}
	if status.LastError != "disabled" {
		t.Errorf("Health().LastError = %q, want %q", status.LastError, "disabled")
	}
}

// TestAnnotatorAdapter_Health_AfterSuccessCall verifies LastSuccess is updated
// after a successful Call.
func TestAnnotatorAdapter_Health_AfterSuccessCall(t *testing.T) {
	// Given: a healthy adapter
	mock := llm_annotator.NewMock("annotation")
	adapter := NewAnnotatorAdapter(mock, "moonshot-v1-8k")

	// When: a successful Call is made
	ctx := context.Background()
	_, err := adapter.Call(ctx, llm.Request{
		Capability: llm.CapabilityFailureAttribution,
		Payload:    llm_annotator.FailureContext{},
	})
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}

	// Then: Health reports a recent LastSuccess
	status := adapter.Health()
	if status.LastSuccess.IsZero() {
		t.Error("Health().LastSuccess is zero after successful call")
	}
	age := time.Since(status.LastSuccess)
	if age > time.Second {
		t.Errorf("Health().LastSuccess is %v in the past, suspiciously old", age)
	}
}
