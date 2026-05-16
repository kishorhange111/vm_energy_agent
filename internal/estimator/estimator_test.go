package estimator_test

import (
	"testing"

	"github.com/vm-energy-agent/internal/collector"
	"github.com/vm-energy-agent/internal/config"
	"github.com/vm-energy-agent/internal/estimator"
)

func cfg(cpu, mem, disk, net float64) config.Config {
	return config.Config{CPUCoeff: cpu, MemoryCoeff: mem, DiskCoeff: disk, NetworkCoeff: net}
}

func TestEstimate_CPUOnly(t *testing.T) {
	est := estimator.NewEstimator(cfg(0.5, 0.3, 0.1, 0.1))
	if got := est.Estimate(nil, &collector.Metrics{CPU: 80}); got != 40.0 {
		t.Errorf("got %.4f, want 40.0", got)
	}
}

func TestEstimate_DiskClamped(t *testing.T) {
	est := estimator.NewEstimator(cfg(0, 0, 1.0, 0))
	// 5000 MB/s exceeds ceiling → normalised to 100
	if got := est.Estimate(nil, &collector.Metrics{Disk: 5000}); got != 100.0 {
		t.Errorf("got %.4f, want 100.0", got)
	}
}

func TestEstimate_AllZero(t *testing.T) {
	est := estimator.NewEstimator(cfg(0.5, 0.3, 0.1, 0.1))
	if got := est.Estimate(nil, &collector.Metrics{}); got != 0.0 {
		t.Errorf("got %.4f, want 0.0", got)
	}
}

func TestEstimate_FullLoad(t *testing.T) {
	est := estimator.NewEstimator(cfg(0.5, 0.3, 0.1, 0.1))
	got := est.Estimate(nil, &collector.Metrics{CPU: 100, Memory: 100, Disk: 1000, Network: 1000})
	if got != 100.0 {
		t.Errorf("got %.4f, want 100.0", got)
	}
}
