package collector

import (
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	gnet "github.com/shirou/gopsutil/v3/net"
)

// VMCollector is the root Composite node.
// It collects system-wide metrics and owns ProcessCollectors as children.
type VMCollector struct {
	mu          sync.Mutex
	name        string
	processes   []MetricSource
	prevDisk    uint64
	prevNet     uint64
	prevNetRecv uint64
	prevNetSent uint64
	prevTime    time.Time
	initialized bool
}

func NewVMCollector(name string) *VMCollector {
	return &VMCollector{name: name}
}

func (v *VMCollector) Name() string              { return v.name }
func (v *VMCollector) Children() []MetricSource  { return v.processes }
func (v *VMCollector) Accept(vis Visitor) { vis.Visit(v) }
func (v *VMCollector) AddProcess(p MetricSource)  { v.processes = append(v.processes, p) }

// Collect returns host-wide CPU%, memory%, disk MB/s, and network MB/s.
func (v *VMCollector) Collect() (*Metrics, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	m := &Metrics{}

	// Use a small positive interval (100ms) instead of 0.
	// Passing 0 makes the first call return usage since process start (unreliable on startup).
	if pcts, err := cpu.Percent(100*time.Millisecond, false); err == nil && len(pcts) > 0 {
		m.CPU = pcts[0]
	}
	if vmStat, err := mem.VirtualMemory(); err == nil {
		m.Memory = float64(vmStat.Used) // bytes (not percent) — follows Prometheus convention
	}

	elapsed := now.Sub(v.prevTime).Seconds()

	// Disk I/O: We intentionally combine ReadBytes + WriteBytes into a single
	// blended metric (vm_disk_io_mb_per_sec). While reads and writes can have
	// different power draw characteristics, the assignment specifies collecting
	// "Disk I/O" as one metric, so we keep the model simple with a combined value.
	if diskStats, err := disk.IOCounters(); err == nil {
		var totalBytes uint64
		for _, d := range diskStats {
			totalBytes += d.ReadBytes + d.WriteBytes
		}
		if v.initialized && totalBytes >= v.prevDisk && elapsed > 0 {
			m.Disk = float64(totalBytes-v.prevDisk) / 1024 / 1024 / elapsed
		}
		v.prevDisk = totalBytes
	}

	if netStats, err := gnet.IOCounters(false); err == nil {
		// When pernic=false, gopsutil returns a record with Name=="" that
		// represents the aggregate across all interfaces. We must select it
		// explicitly instead of assuming netStats[0].
		var total, recv, sent uint64
		for _, ns := range netStats {
			if ns.Name == "" {
				recv = ns.BytesRecv
				sent = ns.BytesSent
				total = recv + sent
				break
			}
		}
		// Fallback
		if total == 0 && len(netStats) > 0 {
			recv = netStats[0].BytesRecv
			sent = netStats[0].BytesSent
			total = recv + sent
		}

		// Total (for power model)
		if v.initialized && total >= v.prevNet && elapsed > 0 {
			m.Network = float64(total-v.prevNet) / 1024 / 1024 / elapsed
		}
		v.prevNet = total

		// Separate directions (better observability)
		if v.initialized && recv >= v.prevNetRecv && elapsed > 0 {
			m.NetworkRecv = float64(recv-v.prevNetRecv) / 1024 / 1024 / elapsed
		}
		v.prevNetRecv = recv

		if v.initialized && sent >= v.prevNetSent && elapsed > 0 {
			m.NetworkSent = float64(sent-v.prevNetSent) / 1024 / 1024 / elapsed
		}
		v.prevNetSent = sent
	}

	v.prevTime = now
	v.initialized = true

	return m, nil
}
