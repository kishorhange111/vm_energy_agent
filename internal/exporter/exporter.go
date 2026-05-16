package exporter

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/vm-energy-agent/internal/collector"
	"github.com/vm-energy-agent/internal/config"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Exporter is the Prometheus HTTP server and gauge registry.
// It uses a private registry so standard process_* and go_* metrics
// are also exposed — allowing Grafana to verify the agent stays within
// the <2% CPU and <100 MB RAM runtime constraints.
type Exporter struct {
	registry   *prometheus.Registry
	cpuGauge               *prometheus.GaugeVec
	memGauge               *prometheus.GaugeVec
	diskGauge              *prometheus.GaugeVec
	netGauge               *prometheus.GaugeVec
	powerGauge             *prometheus.GaugeVec
	collectionErrorCounter *prometheus.CounterVec
}

func NewExporter() *Exporter {
	reg := prometheus.NewRegistry()
	// Self-monitoring: exposes process_resident_memory_bytes,
	// process_cpu_seconds_total, go_goroutines, etc.
	reg.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
	)

	labels := []string{"instance", "vm_name", "level", "node"}
	e := &Exporter{
		registry:   reg,
		cpuGauge:   newGaugeVec(reg, "vm_cpu_usage",          "CPU usage %",          labels),
		memGauge:   newGaugeVec(reg, "vm_memory_usage",       "Memory usage %",       labels),
		diskGauge:  newGaugeVec(reg, "vm_disk_io_mb_per_sec", "Disk IO MB/s",         labels),
		netGauge:   newGaugeVec(reg, "vm_network_mb_per_sec", "Network IO MB/s",      labels),
		powerGauge: newGaugeVec(reg, "vm_power_score",        "Estimated power score (0-100, dimensionless)", labels),
		// New: allows alerting on repeated collection failures
		collectionErrorCounter: newCounterVec(reg, "vm_collection_errors_total", "Total collection failures", labels),
	}
	return e
}

func newCounterVec(reg *prometheus.Registry, name, help string, labels []string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	reg.MustRegister(c)
	return c
}

func newGaugeVec(reg *prometheus.Registry, name, help string, labels []string) *prometheus.GaugeVec {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels)
	reg.MustRegister(g)
	return g
}

// UpdateNode pushes metrics for a single tree node (vm / process / thread).
// The "level" label lets Grafana fan out by granularity independently.
func (e *Exporter) UpdateNode(level, node string, m *collector.Metrics, power float64, cfg config.Config) {
	l := prometheus.Labels{
		"instance": cfg.InstanceName,
		"vm_name":  cfg.VMName,
		"level":    level,
		"node":     node,
	}
	e.cpuGauge.With(l).Set(m.CPU)
	e.memGauge.With(l).Set(m.Memory)
	e.diskGauge.With(l).Set(m.Disk)
	e.netGauge.With(l).Set(m.Network)
	e.powerGauge.With(l).Set(power)
}

// IncCollectionError increments vm_collection_errors_total for alerting.
func (e *Exporter) IncCollectionError(level, node string, cfg config.Config) {
	l := prometheus.Labels{
		"instance": cfg.InstanceName,
		"vm_name":  cfg.VMName,
		"level":    level,
		"node":     node,
	}
	e.collectionErrorCounter.With(l).Inc()
}

// Start runs the HTTP server with graceful shutdown on ctx cancellation.
func (e *Exporter) Start(ctx context.Context, port string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(e.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// === NEW: OpenTelemetry auto-instrumentation ===
	handler := otelhttp.NewHandler(mux, "http.server")

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
