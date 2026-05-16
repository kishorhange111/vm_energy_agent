//go:build linux

package collector

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// clkTck is the number of clock ticks per second.
// We use a safe default (100) which works on the vast majority of Linux systems.
// Dynamic detection was removed to avoid persistent Docker build issues with CGO_ENABLED=0.
const clkTck = 100.0

// ThreadCollector is a Leaf in the Composite pattern.
// It reads per-thread CPU time from /proc/<pid>/task/<tid>/stat.
type ThreadCollector struct {
	mu          sync.Mutex
	pid         int32
	tid         int32
	name        string
	prevUser    uint64
	prevSys     uint64
	prevTime    time.Time
	initialized bool
}

func NewThread(pid, tid int32, name string) *ThreadCollector {
	return &ThreadCollector{pid: pid, tid: tid, name: name}
}

func (t *ThreadCollector) Name() string             { return fmt.Sprintf("%s/tid:%d", t.name, t.tid) }
func (t *ThreadCollector) Children() []MetricSource { return nil }
func (t *ThreadCollector) Accept(v Visitor) { v.Visit(t) }

func (t *ThreadCollector) Collect() (*Metrics, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	user, sys, err := readThreadStat(t.pid, t.tid)
	if err != nil {
		return &Metrics{}, err
	}

	var cpuPct float64
	if t.initialized && user >= t.prevUser && sys >= t.prevSys {
		if elapsed := now.Sub(t.prevTime).Seconds(); elapsed > 0 {
			delta := float64((user - t.prevUser) + (sys - t.prevSys))
			cpuPct = (delta / clkTck) / elapsed * 100.0
		}
	}

	t.prevUser = user
	t.prevSys = sys
	t.prevTime = now
	t.initialized = true

	return &Metrics{CPU: cpuPct}, nil
}

// readThreadStat reads utime and stime from /proc/<pid>/task/<tid>/stat.
func readThreadStat(pid, tid int32) (user, sys uint64, err error) {
	path := fmt.Sprintf("/proc/%d/task/%d/stat", pid, tid)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	idx := bytes.LastIndexByte(data, ')')
	if idx < 0 {
		return 0, 0, fmt.Errorf("malformed stat: no closing paren in %s", path)
	}
	fields := strings.Fields(string(data[idx+1:]))
	if len(fields) < 13 {
		return 0, 0, fmt.Errorf("%s: need >= 13 fields after ')', got %d", path, len(fields))
	}
	user, err = strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return
	}
	sys, err = strconv.ParseUint(fields[12], 10, 64)
	return
}