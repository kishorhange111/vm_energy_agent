// internal/otel/tracing.go
package otel

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func Setup(ctx context.Context) (func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error

	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			if fnErr := fn(ctx); fnErr != nil {
				err = errors.Join(err, fnErr)
			}
		}
		return err
	}

	// Standard propagator
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(prop)

	// Resource attributes
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("vm-energy-agent"),
			semconv.ServiceVersion("v2.1.0-otel"),
			semconv.DeploymentEnvironment(os.Getenv("AGENT_ENV")),
		),
	)
	if err != nil {
		slog.Warn("otel resource creation failed", "err", err)
		res = resource.Default()
	}

	// FIXED: Correct endpoint for Jaeger (no "http://" prefix when using WithInsecure)
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint("jaeger:4318"),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	shutdownFuncs = append(shutdownFuncs, exporter.Shutdown)

	// Tracer provider with batching
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
	)
	shutdownFuncs = append(shutdownFuncs, tp.Shutdown)
	otel.SetTracerProvider(tp)

	slog.Info("OpenTelemetry tracing initialized successfully")
	return shutdown, nil
}