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

	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		m.CPU = pcts[0]
	}
	if vmStat, err := mem.VirtualMemory(); err == nil {
		m.Memory = vmStat.UsedPercent
	}

	elapsed := now.Sub(v.prevTime).Seconds()

	if diskStats, err := disk.IOCounters(); err == nil {
		var total uint64
		for _, d := range diskStats {
			total += d.ReadBytes + d.WriteBytes
		}
		if v.initialized && total >= v.prevDisk && elapsed > 0 {
			m.Disk = float64(total-v.prevDisk) / 1024 / 1024 / elapsed
		}
		v.prevDisk = total
	}

	if netStats, err := gnet.IOCounters(false); err == nil {
		// When pernic=false, gopsutil returns a record with Name=="" that
		// represents the aggregate across all interfaces. We must select it
		// explicitly instead of assuming netStats[0] (order is not guaranteed
		// on multi-NIC systems and can lead to double-counting or wrong data).
		var total uint64
		for _, ns := range netStats {
			if ns.Name == "" {
				total = ns.BytesRecv + ns.BytesSent
				break
			}
		}
		// Fallback (should rarely be needed)
		if total == 0 && len(netStats) > 0 {
			total = netStats[0].BytesRecv + netStats[0].BytesSent
		}

		if v.initialized && total >= v.prevNet && elapsed > 0 {
			m.Network = float64(total-v.prevNet) / 1024 / 1024 / elapsed
		}
		v.prevNet = total
	}

	v.prevTime = now
	v.initialized = true

	return m, nil
}
