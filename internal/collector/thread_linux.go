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

	"golang.org/x/sys/unix"
)

// clkTck is the number of clock ticks per second (usually 100, 250 or 1000).
// We read it at init time using sysconf(_SC_CLK_TCK) for correctness on
// non-standard kernels.
var clkTck = func() float64 {
	ticks, err := unix.Sysconf(unix.SC_CLK_TCK)
	if err != nil || ticks <= 0 {
		return 100 // safe fallback
	}
	return float64(ticks)
}()

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
	// Guard against uint64 underflow on counter resets.
	if t.initialized && user >= t.prevUser && sys >= t.prevSys {
		if elapsed := now.Sub(t.prevTime).Seconds(); elapsed > 0 {
			delta := float64((user - t.prevUser) + (sys - t.prevSys))
			// jiffies (CLK_TCK from sysconf) → percent. This works correctly
			// even on kernels compiled with HZ=250 or HZ=1000.
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
	// Fields after ')': state ppid pgrp session tty tpgid flags
	//                   minflt cminflt majflt cmajflt utime(13) stime(14) ...
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