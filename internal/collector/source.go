// Package collector implements the Composite pattern.
// MetricSource is the Component interface. VM, Process, and Thread nodes all
// satisfy it — callers iterate and collect without knowing the concrete type.
package collector

// Metrics holds resource usage at any granularity level: thread, process, or VM.
type Metrics struct {
	CPU     float64 // percentage 0–100
	Memory  float64 // percentage 0–100
	Disk    float64 // MB/s  (meaningful at VM level)
	Network float64 // MB/s  (meaningful at VM level)
}

// Visitor is declared here (not in the visitor package) to break the import cycle.
// Using a single Visit(MetricSource) makes decorators (CachedSource) transparent:
// the visitor receives the wrapper and can call Collect() on it to get caching.
type Visitor interface {
	Visit(MetricSource)
}

// MetricSource is the Component interface for the Composite pattern.
// Leaves (Thread) and composites (Process, VM) all implement this.
type MetricSource interface {
	Name()     string
	Collect()  (*Metrics, error)
	Children() []MetricSource
	Accept(Visitor)
}
