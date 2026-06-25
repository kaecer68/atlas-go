// Package otel provides OpenTelemetry SDK initialization for atlas-go.
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is set in the environment, spans are
// exported via OTLP HTTP to the collector (the OTel SDK reads this env
// var natively). Otherwise, falls back to stdouttrace for local
// development and CI.
//
// Maturity: evolving
//
// Telemetry egress is out of Data Source Constitution scope (which
// governs data ingestion only); OTel does NOT route through apigateway.
package otel

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	TracerName = "github.com/kaecer68/atlas-go/internal/observability"

	// Span payload limits guard against malformed huge attributes that
	// could OOM the batch processor.
	spanAttributeValueLengthLimit = 4096
	spanAttributeCountLimit       = 128

	// BatchSpanProcessor bounds; spans beyond max queue are dropped
	// with a metric increment rather than growing unbounded.
	maxExportQueueSize = 2048
	maxExportBatchSize = 512
	exportBatchTimeout = 5 * time.Second

	// otlpEndpointEnv is the OpenTelemetry SDK standard env var; atlas
	// code checks it only to decide exporter choice (OTLP vs stdout).
	otlpEndpointEnv = "OTEL_EXPORTER_OTLP_ENDPOINT"
)

// Init bootstraps the OpenTelemetry TracerProvider.
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is set, uses OTLP HTTP exporter (SDK
// reads endpoint from env). Otherwise falls back to stdouttrace for
// local development and CI.
//
// Returns a shutdown function that flushes pending spans. Caller must
// invoke it before process exit.
func Init(ctx context.Context) (func(context.Context) error, error) {
	if os.Getenv(otlpEndpointEnv) != "" {
		return initOTLPExporter(ctx)
	}
	return initStdoutExporter()
}

func initOTLPExporter(ctx context.Context) (func(context.Context) error, error) {
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("otel: create OTLP exporter: %w", err)
	}
	return setupTracerProvider(ctx, exporter)
}

func initStdoutExporter() (func(context.Context) error, error) {
	exporter, err := stdouttrace.New(
		stdouttrace.WithWriter(os.Stdout),
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: create stdout exporter: %w", err)
	}
	return setupTracerProvider(context.Background(), exporter)
}

func setupTracerProvider(ctx context.Context, exporter sdktrace.SpanExporter) (func(context.Context) error, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("atlas-go"),
			semconv.ServiceVersion("0.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: create resource: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithMaxQueueSize(maxExportQueueSize),
			sdktrace.WithMaxExportBatchSize(maxExportBatchSize),
			sdktrace.WithBatchTimeout(exportBatchTimeout),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSpanLimits(sdktrace.SpanLimits{
			AttributeValueLengthLimit: spanAttributeValueLengthLimit,
			AttributeCountLimit:       spanAttributeCountLimit,
		}),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown, nil
}
