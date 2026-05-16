package estimator

import (
	"context"

	"github.com/vm-energy-agent/internal/collector"
	"github.com/vm-energy-agent/internal/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// maxDiskMBps / maxNetMBps define the normalisation ceiling.
// Values at or above the ceiling map to a normalised score of 100.
const (
	maxDiskMBps = 1000.0
	maxNetMBps  = 1000.0
)

// Estimator applies a linear power model:
//
//	Power = a*CPU% + b*Mem% + c*DiskNorm(0-100) + d*NetNorm(0-100)
type Estimator struct {
	cpuCoeff  float64
	memCoeff  float64
	diskCoeff float64
	netCoeff  float64
}

func NewEstimator(cfg config.Config) *Estimator {
	return &Estimator{
		cpuCoeff:  cfg.CPUCoeff,
		memCoeff:  cfg.MemoryCoeff,
		diskCoeff: cfg.DiskCoeff,
		netCoeff:  cfg.NetworkCoeff,
	}
}

// Estimate returns a power score in the range [0, 100].
func (e *Estimator) Estimate(ctx context.Context, m *collector.Metrics) float64 {
	if ctx == nil {
		ctx = context.Background()
	}
	_, span := otel.Tracer("vm-energy-agent").Start(ctx, "estimate.power")
	defer span.End()

	// If metrics are nil (stale from CachedSource or other failure), return 0 power
	if m == nil {
		span.SetAttributes(attribute.Float64("power.score", 0))
		span.SetAttributes(attribute.Bool("data.stale", true))
		return 0
	}

	result := (e.cpuCoeff * m.CPU) +
		(e.memCoeff * m.Memory) +
		(e.diskCoeff * clamp(m.Disk/maxDiskMBps*100, 0, 100)) +
		(e.netCoeff * clamp(m.Network/maxNetMBps*100, 0, 100))

	span.SetAttributes(attribute.Float64("power.score", result))
	return result
}

func clamp(v, lo, hi float64) float64 {
	if v < 0 {
		return 0 // treat negative (stale) as 0
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}