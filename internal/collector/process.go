package collector

import (
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/mem"
	goproc "github.com/shirou/gopsutil/v3/process"
)

// ProcessCollector is a Composite node: it owns ThreadCollectors as children
// and also collects its own process-level CPU and memory metrics.
type ProcessCollector struct {
	mu                sync.Mutex
	pid               int32
	name              string
	threads           []MetricSource
	threadSet         map[int32]struct{}
	proc              *goproc.Process
	threadCacheTTL    time.Duration
	totalSystemMemory uint64 // cached from VMCollector / Agent to avoid N syscalls per cycle
}

func NewProcess(pid int32) (*ProcessCollector, error) {
	p, err := goproc.NewProcess(pid)
	if err != nil {
		return nil, err
	}
	name, _ := p.Name()
	return &ProcessCollector{
		pid:            pid,
		name:           name,
		proc:           p,
		threadSet:      make(map[int32]struct{}),
		threadCacheTTL: 0, // disabled by default; agent can enable it
	}, nil
}

func (p *ProcessCollector) ShortName() string        { return p.name }
func (p *ProcessCollector) Name() string             { return fmt.Sprintf("%s/pid:%d", p.name, p.pid) }
func (p *ProcessCollector) Children() []MetricSource { return p.threads }
func (p *ProcessCollector) Accept(v Visitor) { v.Visit(p) }
// AddThread appends a thread and records its TID directly in threadSet.
// Accepting tid explicitly avoids fragile string parsing of the thread name
// (which could contain ':' if the process name has special characters).
func (p *ProcessCollector) AddThread(t MetricSource, tid int32) {
	p.threads = append(p.threads, t)
	if p.threadSet == nil {
		p.threadSet = make(map[int32]struct{})
	}
	p.threadSet[tid] = struct{}{}
}

// SetThreadCacheTTL enables automatic wrapping of dynamically discovered threads
// with CachedSource using the given TTL.
func (p *ProcessCollector) SetThreadCacheTTL(ttl time.Duration) {
	p.threadCacheTTL = ttl
}

// SetTotalSystemMemory allows the agent to inject the total system RAM once per cycle
// so ProcessCollector doesn't need to call mem.VirtualMemory() repeatedly.
func (p *ProcessCollector) SetTotalSystemMemory(total uint64) {
	p.totalSystemMemory = total
}

// ThreadIDs returns the OS thread IDs (TIDs) for this process.
func (p *ProcessCollector) ThreadIDs() ([]int32, error) {
	threads, err := p.proc.Threads()
	if err != nil {
		return nil, err
	}
	ids := make([]int32, 0, len(threads))
	for tid := range threads {
		ids = append(ids, tid)
	}
	return ids, nil
}

// RefreshThreads discovers newly spawned threads for this process and adds them.
// Called periodically from the agent to support dynamic thread creation after startup.
func (p *ProcessCollector) RefreshThreads() {
	if p.proc == nil {
		return
	}
	current, err := p.ThreadIDs()
	if err != nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.threadSet == nil {
		p.threadSet = make(map[int32]struct{})
	}

	for _, tid := range current {
		if _, exists := p.threadSet[tid]; !exists {
			threadRaw := NewThread(p.pid, tid, p.name)
			var thread MetricSource = threadRaw
			if p.threadCacheTTL > 0 {
				thread = NewCachedSource(threadRaw, p.threadCacheTTL)
			}
			p.AddThread(thread, tid) // uses the safe explicit-TID version
		}
	}
}

// Collect returns process-level CPU% and memory%.
// Thread CPU is attributed separately via each ThreadCollector leaf.
func (p *ProcessCollector) Collect() (*Metrics, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	m := &Metrics{}
	if cpuPct, err := p.proc.CPUPercent(); err == nil {
		m.CPU = cpuPct
	}
	if memInfo, err := p.proc.MemoryInfo(); err == nil {
		total := p.totalSystemMemory
		if total == 0 {
			// Fallback (should rarely happen if agent sets it)
			if vmStat, err2 := mem.VirtualMemory(); err2 == nil {
				total = vmStat.Total
			}
		}
		if total > 0 {
			m.Memory = float64(memInfo.RSS) / float64(total) * 100.0
		}
	}
	return m, nil
}
