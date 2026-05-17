package visitor_test

import (
	"testing"

	"github.com/vm-energy-agent/internal/collector"
	"github.com/vm-energy-agent/internal/config"
	"github.com/vm-energy-agent/internal/estimator"
	"github.com/vm-energy-agent/internal/exporter"
	"github.com/vm-energy-agent/internal/visitor"
)

func newVisitor(t *testing.T) *visitor.ExportVisitor {
	t.Helper()
	cfg := config.Config{
		InstanceName: "test-instance",
		VMName:       "test-vm",
		CPUCoeff:     0.5,
		MemoryCoeff:  0.3,
		DiskCoeff:    0.1,
		NetworkCoeff: 0.1,
	}
	return visitor.NewExportVisitor(
		estimator.NewEstimator(cfg),
		exporter.NewExporter(),
		cfg,
	)
}

func TestExportVisitor_VisitVM(t *testing.T) {
	v := newVisitor(t)
	vm := collector.NewVMCollector("test-vm")
	v.Visit(vm) // Fixed: was VisitVM
}

func TestExportVisitor_VisitProcess(t *testing.T) {
	v := newVisitor(t)
	proc, err := collector.NewProcess(1)
	if err != nil {
		t.Skip("Process collector not available in this environment")
	}
	v.Visit(proc)
}

func TestExportVisitor_MultipleVisits(t *testing.T) {
	v := newVisitor(t)
	vm := collector.NewVMCollector("vm-1")

	for i := 0; i < 3; i++ {
		v.Visit(vm)
	}
}

func TestExportVisitor_ErrorHandling(t *testing.T) {
	v := newVisitor(t)
	vm := collector.NewVMCollector("")
	v.Visit(vm) // Fixed: was VisitVM
}

func TestExportVisitor_VisitWithZeroMetrics(t *testing.T) {
	v := newVisitor(t)
	vm := collector.NewVMCollector("zero-vm")
	v.Visit(vm) // Fixed: was VisitVM
}

func TestExportVisitor_Creation(t *testing.T) {
	cfg := config.Config{InstanceName: "test", VMName: "vm"}
	v := visitor.NewExportVisitor(
		estimator.NewEstimator(cfg),
		exporter.NewExporter(),
		cfg,
	)
	if v == nil {
		t.Fatal("NewExportVisitor returned nil")
	}
}

func TestExportVisitor_VisitZeroValue(t *testing.T) {
	v := newVisitor(t)
	vm := collector.NewVMCollector("")
	v.Visit(vm)
}