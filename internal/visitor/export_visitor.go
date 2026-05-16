// Package visitor implements the Visitor pattern.
// ExportVisitor adds collect-and-export behaviour to each node in the
// composite tree WITHOUT modifying any node struct (open/closed principle).
package visitor

import (
	"context"
	"errors"
	"log/slog"

	"github.com/vm-energy-agent/internal/collector"
	"github.com/vm-energy-agent/internal/config"
	"github.com/vm-energy-agent/internal/estimator"
	"github.com/vm-energy-agent/internal/exporter"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ExportVisitor collects metrics from a node, estimates power, and updates
// the Prometheus exporter — all without the nodes knowing any of this happens.
type ExportVisitor struct {
	est *estimator.Estimator
	exp *exporter.Exporter
	cfg config.Config
	ctx context.Context // set per collection cycle for proper span parenting
}

func NewExportVisitor(est *estimator.Estimator, exp *exporter.Exporter, cfg config.Config) *ExportVisitor {
	return &ExportVisitor{est: est, exp: exp, cfg: cfg}
}

// SetContext is called by the agent before each collection cycle so that
// visit.* spans become children of collection.cycle instead of root spans.
func (v *ExportVisitor) SetContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	v.ctx = ctx
}

func (v *ExportVisitor) Visit(node collector.MetricSource) {
	if node == nil {
		return
	}

	// Unwrap CachedSource so we always Collect() through the decorator (enables caching)
	// and can still detect the real level for labeling.
	realNode := node
	level := "unknown"
	for {
		if cached, ok := realNode.(*collector.CachedSource); ok {
			realNode = cached.Inner()
			continue
		}
		break
	}

	switch realNode.(type) {
	case *collector.VMCollector:
		level = "vm"
	case *collector.ProcessCollector:
		level = "process"
	case *collector.ThreadCollector:
		level = "thread"
	}

	parentCtx := v.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	_, span := otel.Tracer("vm-energy-agent").Start(parentCtx, "visit."+level,
		trace.WithAttributes(attribute.String("node", node.Name())))
	defer span.End()

	m, err := node.Collect() // IMPORTANT: call on the (possibly wrapped) node → caching works
	if err != nil {
		if errors.Is(err, collector.ErrStaleMetrics) {
			// Stale data from CachedSource — do NOT push -1 values to Prometheus.
			// Just count it as a collection error for alerting.
			slog.Debug(level + " collect skipped (stale data)")
			v.exp.IncCollectionError(level, node.Name(), v.cfg)
			return
		}
		slog.Warn(level+" collect failed", "name", node.Name(), "err", err)
		span.RecordError(err)
		v.exp.IncCollectionError(level, node.Name(), v.cfg)
		return
	}
	v.exp.UpdateNode(level, node.Name(), m, v.est.Estimate(parentCtx, m), v.cfg)
}