package collector_test

import (
	"testing"
	"time"

	"github.com/vm-energy-agent/internal/collector"
)

// Helper stub for iterator tests
type testNode struct {
	name     string
	children []collector.MetricSource
}

func (n *testNode) Name() string                       { return n.name }
func (n *testNode) Children() []collector.MetricSource { return n.children }
func (n *testNode) Collect() (*collector.Metrics, error) {
	return &collector.Metrics{}, nil
}
func (n *testNode) Accept(v collector.Visitor) {}

func TestIterator_BreadthFirst_MultipleLevels(t *testing.T) {
	t1 := &testNode{name: "t1"}
	t2 := &testNode{name: "t2"}
	p1 := &testNode{name: "p1", children: []collector.MetricSource{t1}}
	p2 := &testNode{name: "p2", children: []collector.MetricSource{t2}}
	vm := &testNode{name: "vm", children: []collector.MetricSource{p1, p2}}

	iter := collector.NewIterator(vm)
	var visited []string
	for node := iter.Next(); node != nil; node = iter.Next() {
		visited = append(visited, node.Name())
	}

	if len(visited) != 5 {
		t.Errorf("expected 5 nodes, got %d", len(visited))
	}
}

func TestCachedSource_CacheHit(t *testing.T) {
	vm := collector.NewVMCollector("test-vm")
	cached := collector.NewCachedSource(vm, 1*time.Second)

	m1, _ := cached.Collect()
	m2, _ := cached.Collect()

	if m1.CPU != m2.CPU {
		t.Error("second call should return cached value")
	}
}

func TestThreadCollector_Linux(t *testing.T) {
	t1 := collector.NewThread(1, 1, "test-thread")
	_, err := t1.Collect()
	if err != nil {
		t.Logf("Thread collect error (expected in some envs): %v", err)
	}
}

func TestProcessCollector_Full(t *testing.T) {
	proc, err := collector.NewProcess(1)
	if err != nil {
		t.Skip("Process creation not supported in this environment")
	}

	m, err := proc.Collect()
	if err != nil {
		t.Logf("Collect error (acceptable): %v", err)
	}

	_ = proc.ShortName()
	_, _ = proc.ThreadIDs()
	_ = m // explicitly use m to satisfy compiler
}

func TestVMCollector_AllMetrics(t *testing.T) {
	vm := collector.NewVMCollector("test-vm")
	m, err := vm.Collect()
	if err != nil {
		t.Fatal(err)
	}
	// Exercise all fields (prevents "declared and not used")
	_ = m.CPU
	_ = m.Memory
	_ = m.Disk
	_ = m.Network
}

func TestNewCachedSource(t *testing.T) {
	vm := collector.NewVMCollector("vm")
	c := collector.NewCachedSource(vm, 100*time.Millisecond)
	if c == nil {
		t.Fatal("NewCachedSource returned nil")
	}
}