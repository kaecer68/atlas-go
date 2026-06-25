package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestSafeInvokeHandler_NormalCall verifies that a non-panicking handler
// returns its value through the safe wrapper unchanged.
func TestSafeInvokeHandler_NormalCall(t *testing.T) {
	want := json.RawMessage(`{"ok":true}`)
	tool := Tool{
		Name: "echo",
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			return want, nil
		},
	}
	got, err := SafeInvokeHandler(context.Background(), &tool, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("SafeInvokeHandler: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestSafeInvokeHandler_HandlerError verifies that an error from the
// handler propagates through the wrapper unchanged.
func TestSafeInvokeHandler_HandlerError(t *testing.T) {
	sentinel := errors.New("handler failed")
	tool := Tool{
		Name: "broken",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return nil, sentinel
		},
	}
	_, err := SafeInvokeHandler(context.Background(), &tool, json.RawMessage(`{}`))
	if !errors.Is(err, sentinel) {
		t.Errorf("got err=%v, want errors.Is(_, sentinel)=true", err)
	}
}

// TestSafeInvokeHandler_PanicRecovered verifies that a handler panic is
// caught, converted to an error, and does not propagate up.
func TestSafeInvokeHandler_PanicRecovered(t *testing.T) {
	tool := Tool{
		Name: "panicker",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			panic("boom")
		},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SafeInvokeHandler should not propagate panic, got: %v", r)
		}
	}()
	result, err := SafeInvokeHandler(context.Background(), &tool, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error from panicking handler, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on panic, got %s", result)
	}
	if !strings.Contains(err.Error(), "panicker") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error message should mention tool name and panic value, got: %v", err)
	}
}

// TestSafeInvokeHandler_NilHandler verifies that a nil Handler is handled
// gracefully (it panics with a clear nil-deref, which the wrapper recovers).
func TestSafeInvokeHandler_NilHandler(t *testing.T) {
	tool := Tool{Name: "nilhandler", Handler: nil}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SafeInvokeHandler should recover nil-handler panic, got: %v", r)
		}
	}()
	_, err := SafeInvokeHandler(context.Background(), &tool, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error from nil handler, got nil")
	}
}

// TestBindTypedArgs_RoundTrip verifies that a typed handler receives a
// properly unmarshaled struct and its result is re-marshaled.
func TestBindTypedArgs_RoundTrip(t *testing.T) {
	type weatherArgs struct {
		City string `json:"city"`
	}
	type weatherResult struct {
		Temp int `json:"temp"`
	}
	factory := func() weatherArgs { return weatherArgs{} }
	handler := func(_ context.Context, args weatherArgs) (weatherResult, error) {
		if args.City != "taipei" {
			t.Errorf("got city=%q, want taipei", args.City)
		}
		return weatherResult{Temp: 30}, nil
	}
	wrapped := BindTypedArgs("get_weather", factory, handler)
	got, err := wrapped(context.Background(), json.RawMessage(`{"city":"taipei"}`))
	if err != nil {
		t.Fatalf("wrapped: %v", err)
	}
	var r weatherResult
	if err := json.Unmarshal(got, &r); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if r.Temp != 30 {
		t.Errorf("got temp=%d, want 30", r.Temp)
	}
}

// TestBindTypedArgs_UnmarshalError verifies that malformed JSON produces
// a wrapped error mentioning the tool name.
func TestBindTypedArgs_UnmarshalError(t *testing.T) {
	type args struct {
		X int `json:"x"`
	}
	wrapped := BindTypedArgs("broken_tool", func() args { return args{} },
		func(_ context.Context, _ args) (args, error) { return args{}, nil })
	_, err := wrapped(context.Background(), json.RawMessage(`{not json}`))
	if err == nil {
		t.Fatal("expected error from malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "broken_tool") {
		t.Errorf("error should mention tool name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("error should mention unmarshal, got: %v", err)
	}
}
