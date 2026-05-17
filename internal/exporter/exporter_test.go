package exporter_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vm-energy-agent/internal/collector"
	"github.com/vm-energy-agent/internal/config"
	"github.com/vm-energy-agent/internal/exporter"
)

func TestExporter_HealthEndpoint(t *testing.T) {
	exp := exporter.NewExporter()
	cfg := config.Config{Port: "19090", InstanceName: "test", VMName: "vm-test"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = exp.Start(ctx, cfg.Port) }()
	time.Sleep(80 * time.Millisecond)

	resp, err := http.Get("http://localhost:19090/health")
	if err != nil { t.Fatalf("request failed: %v", err) }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}
}

func TestExporter_UpdateNode_NoPanic(t *testing.T) {
	exp := exporter.NewExporter()
	cfg := config.Config{InstanceName: "test", VMName: "vm-test"}
	m := &collector.Metrics{CPU: 45, Memory: 4 * 1024 * 1024 * 1024, Disk: 10, Network: 2} // 4GB realistic bytes
	exp.UpdateNode("vm",      "vm-test",      m, 36.5, cfg)
	exp.UpdateNode("process", "agent/pid:1",  m, 25.0, cfg)
	exp.UpdateNode("thread",  "agent/tid:2",  m, 22.5, cfg)
}
