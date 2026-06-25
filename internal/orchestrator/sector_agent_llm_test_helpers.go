package orchestrator

import (
	"context"
	"fmt"
)

// MockLLMDriver is a test helper that returns canned responses
// from PlanComplete / ReflectComplete without invoking a real
// LLM. It satisfies both PlanDriver and ReflectDriver (the
// new split from Issue #711 #10) so it can be assigned to
// either SectorAgentLLM field.
//
// File-suffix _test_helpers.go means this file is ONLY compiled
// in test builds (per plan v2 C4 fix: MockLLMDriver must live
// in a test-only file, not in production code paths).
//
// Typical usage in PR5b E2E tests:
//
//	mock := NewMockLLMDriver().
//	    WithPlanResponse([]PlanStep{{Kind: "tool", ToolName: "get_factor_weight"}}).
//	    WithReflectResponse(Reflection{Continue: false, FinalConviction: 75})
//	agent := &SectorAgentLLM{PlanDriver: mock, ReflectDriver: mock, Tools: TestTools()}
type MockLLMDriver struct {
	planResp    []PlanStep
	reflectResp Reflection
	planErr     error
	reflectErr  error

	// Recorded calls (for test assertions)
	planCalls    []mockCall
	reflectCalls []mockCall
}

type mockCall struct {
	Skill  string
	Symbol string
	Extra  string // tool result for reflect calls
}

// NewMockLLMDriver constructs a MockLLMDriver with empty (zero-value)
// canned responses. Call WithPlanResponse / WithReflectResponse to
// configure before passing to SectorAgentLLM.
func NewMockLLMDriver() *MockLLMDriver {
	return &MockLLMDriver{
		planResp:    nil,
		reflectResp: Reflection{},
	}
}

// WithPlanResponse sets the canned plan response.
func (m *MockLLMDriver) WithPlanResponse(steps []PlanStep) *MockLLMDriver {
	m.planResp = steps
	return m
}

// WithReflectResponse sets the canned reflect response.
func (m *MockLLMDriver) WithReflectResponse(r Reflection) *MockLLMDriver {
	m.reflectResp = r
	return m
}

// WithPlanError makes PlanComplete return the given error.
func (m *MockLLMDriver) WithPlanError(err error) *MockLLMDriver {
	m.planErr = err
	return m
}

// WithReflectError makes ReflectComplete return the given error.
func (m *MockLLMDriver) WithReflectError(err error) *MockLLMDriver {
	m.reflectErr = err
	return m
}

// PlanComplete satisfies PlanDriver.
func (m *MockLLMDriver) PlanComplete(_ context.Context, skill, symbol string) ([]PlanStep, error) {
	m.planCalls = append(m.planCalls, mockCall{Skill: skill, Symbol: symbol})
	if m.planErr != nil {
		return nil, m.planErr
	}
	if m.planResp == nil {
		return nil, fmt.Errorf("MockLLMDriver.PlanComplete: no canned response configured (call WithPlanResponse first)")
	}
	return m.planResp, nil
}

// ReflectComplete satisfies ReflectDriver.
func (m *MockLLMDriver) ReflectComplete(_ context.Context, skill, symbol, toolResult string) (Reflection, error) {
	m.reflectCalls = append(m.reflectCalls, mockCall{Skill: skill, Symbol: symbol, Extra: toolResult})
	if m.reflectErr != nil {
		return Reflection{}, m.reflectErr
	}
	return m.reflectResp, nil
}

// PlanCallCount returns the number of times PlanComplete was called.
// Useful for assertions in tests.
func (m *MockLLMDriver) PlanCallCount() int { return len(m.planCalls) }

// ReflectCallCount returns the number of times ReflectComplete was called.
func (m *MockLLMDriver) ReflectCallCount() int { return len(m.reflectCalls) }

// LastPlanCall returns the most recent PlanComplete call args, or
// (zero, false) if none.
func (m *MockLLMDriver) LastPlanCall() (mockCall, bool) {
	if len(m.planCalls) == 0 {
		return mockCall{}, false
	}
	return m.planCalls[len(m.planCalls)-1], true
}

// LastReflectCall returns the most recent ReflectComplete call args,
// or (zero, false) if none.
func (m *MockLLMDriver) LastReflectCall() (mockCall, bool) {
	if len(m.reflectCalls) == 0 {
		return mockCall{}, false
	}
	return m.reflectCalls[len(m.reflectCalls)-1], true
}
