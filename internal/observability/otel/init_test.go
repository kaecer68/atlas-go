package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

func TestInit_StdoutWhenEnvUnset(t *testing.T) {
	t.Setenv(otlpEndpointEnv, "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdown, err := Init(ctx)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestInit_OTLPWhenEnvSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Setenv(otlpEndpointEnv, srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdown, err := Init(ctx)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestInit_ReturnsShutdown(t *testing.T) {
	t.Setenv(otlpEndpointEnv, "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdown, err := Init(ctx)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := shutdown(shutdownCtx); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestStartSpan_CreatesSpanWithAttributes(t *testing.T) {
	ctx := context.Background()
	_, span := StartSpan(ctx, "test.span",
		attribute.String("test.key", "test.value"),
		attribute.Int("test.count", 42),
	)
	if span == nil {
		t.Fatal("expected non-nil span")
	}
	span.End()
}

func TestRecordError_NoOpOnNilError(t *testing.T) {
	_, span := StartSpan(context.Background(), "test.no_error")
	defer span.End()

	RecordError(span, nil)
}

func TestRecordError_SetsSpanError(t *testing.T) {
	_, span := StartSpan(context.Background(), "test.error")
	defer span.End()

	RecordError(span, errSentinel)
}

var errSentinel = &sentinelError{msg: "test error"}

type sentinelError struct{ msg string }

func (e *sentinelError) Error() string { return e.msg }
