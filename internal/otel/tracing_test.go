package otel_test

import (
	"context"
	"testing"
	"time"

	"github.com/vm-energy-agent/internal/otel"
)

func TestSetupAndShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	shutdown, err := otel.Setup(ctx)
	if err != nil {
		t.Fatalf("otel.Setup failed: %v", err)
	}

	// Should shutdown cleanly
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("otel shutdown returned error: %v", err)
	}
}

func TestSetupMultipleTimes(t *testing.T) {
	ctx := context.Background()

	shutdown1, err := otel.Setup(ctx)
	if err != nil {
		t.Fatalf("first Setup failed: %v", err)
	}
	defer shutdown1(context.Background())

	// Second setup should still work (or be idempotent)
	shutdown2, err := otel.Setup(ctx)
	if err != nil {
		t.Fatalf("second Setup failed: %v", err)
	}
	shutdown2(context.Background())
}