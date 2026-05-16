//go:build windows

package collector

import (
	"fmt"
	"log/slog"

	"github.com/shirou/gopsutil/v3/process"
)

// ThreadCollector for Windows
type ThreadCollector struct {
	pid  int32
	tid  int32
	name string
}

func NewThread(pid, tid int32, name string) *ThreadCollector {
	tc := &ThreadCollector{pid: pid, tid: tid, name: name}
	tc.tryFetchThreadInfo()
	return tc
}

func (t *ThreadCollector) Name() string {
	return fmt.Sprintf("%s/tid:%d", t.name, t.tid)
}

func (t *ThreadCollector) Children() []MetricSource {
	return nil
}

func (t *ThreadCollector) Accept(v Visitor) {
	v.Visit(t)
}

func (t *ThreadCollector) Collect() (*Metrics, error) {
	// On Windows, we try to use gopsutil, but accurate per-thread
	// CPU usage is still limited. So we return CPU as 0 for now.
	//
	// TODO: Improve thread CPU calculation on Windows in future if needed.
	return &Metrics{CPU: 0}, nil
}

// tryFetchThreadInfo tries to get thread information using gopsutil
func (t *ThreadCollector) tryFetchThreadInfo() {
	p, err := process.NewProcess(t.pid)
	if err != nil {
		return
	}

	// Try to get thread list from gopsutil
	threads, err := p.Threads()
	if err != nil {
		slog.Debug("Windows: Failed to get threads", "pid", t.pid, "err", err)
		return
	}

	// We successfully got thread info from gopsutil.
	// Currently we only use Thread ID. CPU calculation is limited on Windows.
	_ = threads
}