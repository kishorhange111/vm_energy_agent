package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vm-energy-agent/internal/agent"
	"github.com/vm-energy-agent/internal/config"
	"github.com/vm-energy-agent/internal/otel" // NEW
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// OpenTelemetry is a bonus observability feature.
	// If Jaeger / OTLP endpoint is unreachable, we degrade gracefully.
	var otelShutdown func(context.Context) error
	if shutdown, err := otel.Setup(ctx); err != nil {
		slog.Warn("OpenTelemetry tracing disabled (non-fatal)", "err", err)
		otelShutdown = func(context.Context) error { return nil }
	} else {
		otelShutdown = shutdown
	}
	defer func() {
		if shutdownErr := otelShutdown(context.Background()); shutdownErr != nil {
			slog.Error("otel shutdown error", "err", shutdownErr)
		}
	}()

	ag := agent.New(cfg)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		slog.Info("shutting down", "signal", sig.String())
		cancel()
	}()

	if err := ag.Run(ctx); err != nil {
		slog.Error("agent error", "err", err)
		os.Exit(1)
	}
}