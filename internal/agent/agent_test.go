package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/vm-energy-agent/internal/agent"
	"github.com/vm-energy-agent/internal/config"
)

func TestAgent_New(t *testing.T) {
	cfg := config.Config{
		InstanceName: "test",
		VMName:       "vm-test",
	}
	a := agent.New(cfg)
	if a == nil {
		t.Fatal("expected non-nil agent")
	}
}

func TestAgent_Run_WithCancel(t *testing.T) {
	cfg := config.Config{InstanceName: "test", VMName: "vm-test"}
	a := agent.New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := a.Run(ctx)
	if err != nil {
		t.Logf("Run returned error (expected on short timeout): %v", err)
	}
}
func TestAgent_NewWithDifferentConfigs(t *testing.T) {
	cases := []config.Config{
		{InstanceName: "a", VMName: "b"},
		{InstanceName: "", VMName: ""},
	}

	for _, cfg := range cases {
		a := agent.New(cfg)
		if a == nil {
			t.Errorf("New() returned nil for config %+v", cfg)
		}
	}
}

func TestAgent_Run_ContextCancellation(t *testing.T) {
	cfg := config.Config{InstanceName: "test", VMName: "vm"}
	a := agent.New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_ = a.Run(ctx) // should return quickly
}