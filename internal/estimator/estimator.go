package estimator

import (
	"context"

	"github.com/vm-energy-agent/internal/collector"
	"github.com/vm-energy-agent/internal/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// Normalisation ceilings.
// Values at or above the ceiling are clamped to 100 before weighting.
const (
	maxDiskMBps = 1000.0
	maxNetMBps  = 1000.0
	maxMemBytes = 128.0 * 1024 * 1024 * 1024 // 128 GB ceiling for normalisation
	maxWatts    = 250.0                      // realistic ceiling for a typical server VM
)

// Estimator applies a linear power model:
//
//	Power = a*CPU% + b*MemNorm(0-100) + c*DiskNorm(0-100) + d*NetNorm(0-100)
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

// Estimate returns estimated power consumption in **watts** (0 .. maxWatts).
// It first computes a 0-100 score from the linear model, then scales it
// to a realistic watt value using maxWatts.
func (e *Estimator) Estimate(ctx context.Context, m *collector.Metrics) float64 {
	if ctx == nil {
		ctx = context.Background()
	}
	_, span := otel.Tracer("vm-energy-agent").Start(ctx, "estimate.power")
	defer span.End()

	// If metrics are nil (stale from CachedSource or other failure), return 0 watts
	if m == nil {
		span.SetAttributes(attribute.Float64("power.watts", 0))
		span.SetAttributes(attribute.Bool("data.stale", true))
		return 0
	}

	score := (e.cpuCoeff * m.CPU) +
		(e.memCoeff * clamp(m.Memory/maxMemBytes*100, 0, 100)) + // normalize bytes → 0-100
		(e.diskCoeff * clamp(m.Disk/maxDiskMBps*100, 0, 100)) +
		(e.netCoeff * clamp(m.Network/maxNetMBps*100, 0, 100))

	watts := clamp(score, 0, 100) / 100.0 * maxWatts

	span.SetAttributes(attribute.Float64("power.watts", watts))
	return watts
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