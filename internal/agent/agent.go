// Package agent implements the Facade pattern.
// It is the single entry point that wires Collector, Estimator, and Exporter
// together. Callers only see New() and Run() — all subsystem complexity is hidden.
package agent

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/vm-energy-agent/internal/collector"
	"github.com/vm-energy-agent/internal/config"
	"github.com/vm-energy-agent/internal/estimator"
	"github.com/vm-energy-agent/internal/exporter"
	"github.com/vm-energy-agent/internal/visitor"
	"go.opentelemetry.io/otel"
)

// Agent is the Facade that wires all subsystems together.
type Agent struct {
	cfg      config.Config
	vm       collector.MetricSource // root, wrapped with CachedSource
	selfProc *collector.ProcessCollector
	est      *estimator.Estimator
	exp      *exporter.Exporter
	visitor  *visitor.ExportVisitor
	iter     *collector.TreeIterator
	interval time.Duration

	// Cached total system memory to avoid redundant mem.VirtualMemory() calls
	// from every ProcessCollector during collection.
	totalSystemMemory atomic.Uint64
}

// New constructs the agent, discovers ALL processes on the host/VM (plus their threads),
// and wires the composite hierarchy: VM → Process → Threads.
// Only the agent's own process gets dynamic thread refresh on every cycle.
func New(cfg config.Config) *Agent {
	vmRaw := collector.NewVMCollector(cfg.VMName)

	// Discover ALL running processes on the host/VM so that level=process and level=thread
	// metrics reflect actual workloads, not just the agent itself.
	// We still specially track the agent's own process (selfProc) for dynamic thread refresh
	// in collect() cycles. Threads for other processes are snapshotted once at startup.
	var selfProc *collector.ProcessCollector
	self := int32(os.Getpid())

	allProcs, err := process.Processes()
	if err != nil {
		slog.Warn("failed to list processes, falling back to agent self only", "err", err)
		// Fallback: at least monitor ourselves
		if procRaw, err := collector.NewProcess(self); err == nil {
			selfProc = procRaw
			procRaw.SetThreadCacheTTL(5 * time.Second)
			if tids, err := procRaw.ThreadIDs(); err == nil {
				for _, tid := range tids {
					threadRaw := collector.NewThread(self, tid, procRaw.ShortName())
					thread := collector.NewCachedSource(threadRaw, 5*time.Second)
					procRaw.AddThread(thread, tid)
				}
			}
			proc := collector.NewCachedSource(procRaw, 5*time.Second)
			vmRaw.AddProcess(proc)
		}
	} else {
		for _, p := range allProcs {
			pid := p.Pid
			procRaw, err := collector.NewProcess(pid)
			if err != nil {
				continue // skip processes we can't inspect (zombies, permission, etc.)
			}
			procRaw.SetThreadCacheTTL(5 * time.Second)

			// Snapshot threads at startup for every process
			if tids, err := procRaw.ThreadIDs(); err == nil {
				for _, tid := range tids {
					threadRaw := collector.NewThread(pid, tid, procRaw.ShortName())
					thread := collector.NewCachedSource(threadRaw, 5*time.Second)
					procRaw.AddThread(thread, tid)
				}
			}

			proc := collector.NewCachedSource(procRaw, 5*time.Second)
			vmRaw.AddProcess(proc)

			if pid == self {
				selfProc = procRaw
			}
		}
	}

	// Wrap the VM root so top-level metrics (CPU/Mem/Disk/Net) are cached
	vm := collector.NewCachedSource(vmRaw, 5*time.Second)

	est := estimator.NewEstimator(cfg)
	exp := exporter.NewExporter()
	return &Agent{
		cfg:      cfg,
		vm:       vm,
		selfProc: selfProc,
		est:      est,
		exp:      exp,
		visitor:  visitor.NewExportVisitor(est, exp, cfg),
		iter:     collector.NewIterator(nil), // will Reset() each cycle
		interval: 5 * time.Second,
	}
}

// Run starts the collection ticker and the Prometheus HTTP server.
// Blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.collect(ctx)
			}
		}
	}()

	return a.exp.Start(ctx, a.cfg.Port)
}

func (a *Agent) collect(ctx context.Context) {
	ctx, span := otel.Tracer("vm-energy-agent").Start(ctx, "collection.cycle")
	defer span.End()

	// Dynamic thread discovery: pick up threads spawned after startup
	if a.selfProc != nil {
		a.selfProc.RefreshThreads()
	}

	// Refresh total system memory once per cycle (cheap).
	// We only inject it into selfProc (the agent's own process) as an optimization.
	// All other processes fall back to calling mem.VirtualMemory() inside their
	// Collect() when totalSystemMemory == 0. This is acceptable for an MVP.
	if vmStat, err := mem.VirtualMemory(); err == nil && vmStat.Total > 0 {
		a.totalSystemMemory.Store(vmStat.Total)
	}
	if a.selfProc != nil {
		a.selfProc.SetTotalSystemMemory(a.totalSystemMemory.Load())
	}

	// Reuse iterator (cursor-based implementation → zero alloc after first cycle)
	a.iter.Reset(a.vm)

	// Give visitor the current context so child spans are properly parented
	a.visitor.SetContext(ctx)

	for node := a.iter.Next(); node != nil; node = a.iter.Next() {
		node.Accept(a.visitor)
	}
	slog.Debug("collection cycle complete")
}