# VM Energy Agent

A lightweight, production-oriented Go application that monitors **VMs, Processes, and Threads**, estimates power consumption, and provides rich observability through **Prometheus**, **Grafana**, and **OpenTelemetry (Jaeger)**.

---

## 📌 Project Overview

The VM Energy Agent collects system metrics every 5 seconds and calculates estimated power usage using a configurable linear model. It is designed to be **lightweight** (<2% CPU, <100MB memory) and follows clean software architecture principles.

**Key Goals:**
- Hierarchical monitoring of **all processes** on the host/VM (VM → Process → Thread)
- Accurate power estimation with proper unit normalization
- Low resource consumption
- Full observability (Metrics + Tracing)
- Cross-platform support (Linux primary)

---

## 🏗️ Architecture & Components

### High-Level Flow

```
main.go
   └── Agent (Facade)
         ├── VMCollector (Composite Root)          ← host-wide CPU/Mem/Disk/Net
         │     └── ProcessCollector (ALL processes on the VM)
         │           └── ThreadCollector (per-process threads)
         ├── TreeIterator (BFS Traversal)
         ├── ExportVisitor
         │     ├── Collect metrics (via CachedSource)
         │     ├── Estimate Power (Strategy)
         │     └── Export to Prometheus
         └── OpenTelemetry Tracing
```

**Process/Thread coverage**: The agent discovers **every process** on the host/VM at startup
and snapshots their threads. The agent's own process additionally gets dynamic thread
discovery on every collection cycle (new threads spawned by the agent are picked up live).

### Component Breakdown

| Component                  | Package                          | Responsibility |
|---------------------------|----------------------------------|----------------|
| **Agent (Facade)**        | `internal/agent`                 | Wires all components. Starts ticker and HTTP server. |
| **VMCollector**           | `internal/collector/vm.go`       | Collects system-wide CPU, Memory, Disk I/O, Network metrics. |
| **ProcessCollector**      | `internal/collector/process.go`  | Collects per-process CPU & Memory. Manages child threads. |
| **ThreadCollector**       | `thread_linux.go` + `thread_windows.go` | Per-thread CPU collection (accurate on Linux, limited on Windows). |
| **CachedSource**          | `internal/collector/decorator.go`| **Key optimization**. Caches results for 5s. Returns `-1` if data is stale. |
| **TreeIterator**          | `internal/collector/iterator.go` | Breadth-First traversal of the tree. |
| **ExportVisitor**         | `internal/visitor/export_visitor.go` | Visits nodes, collects data, estimates power, updates Prometheus. |
| **Estimator**             | `internal/estimator/estimator.go` | Calculates power. Returns `0` if data is stale (`-1`). |
| **Exporter**              | `internal/exporter/exporter.go`  | Prometheus server + gauges. OpenTelemetry auto-instrumentation. |
| **OpenTelemetry**         | `internal/otel/tracing.go`       | Initializes tracing and exports to Jaeger. |
| **Config**                | `internal/config/config.go`      | Loads settings from `.env`. |

---

## 🔄 How It Works (Every 5 Seconds)

1. `Agent` triggers collection.
2. `TreeIterator` walks VM → Process → Thread tree.
3. `ExportVisitor` visits each node:
   - Calls `Collect()` (protected by `CachedSource`)
   - Calls `Estimator.Estimate()`
   - Pushes metrics to Prometheus
4. If data is older than 5s → `CachedSource` returns `-1`.
5. `Estimator` sees `-1` → returns **power = 0** (prevents wrong calculations).
6. All steps are traced with OpenTelemetry.

---

## 🚀 How to Run

### Using Docker Compose (Recommended)

```bash
git clone <repo-url>
cd vm_energy_agent

cp .env.example .env
make docker-run
```

**Access Points:**
- Agent Metrics: `http://localhost:8080/metrics`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (admin/admin)
- Jaeger: `http://localhost:16686`

### Run Locally

```bash
make setup
make run
```

---

## ⚙️ Configuration (`.env`)

```env
AGENT_PORT=8080
AGENT_INSTANCE_NAME=instance-01
AGENT_VM_NAME=vm-linux

AGENT_CPU_COEFFICIENT=0.5
AGENT_MEMORY_COEFFICIENT=0.3
AGENT_DISK_COEFFICIENT=0.1
AGENT_NETWORK_COEFFICIENT=0.1

OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4318
```

---

## 📊 Observability

### Prometheus Metrics
- `vm_cpu_usage`, `vm_memory_usage_bytes` (bytes)
- `vm_disk_io_mb_per_sec`, `vm_network_mb_per_sec` (total), `vm_network_recv_mb_per_sec`, `vm_network_sent_mb_per_sec`
- `vm_power_watts` (estimated power in watts)

Labels: `instance`, `vm_name`, `level`, `node`

### Self-Monitoring Queries
```promql
# Memory in MB
process_resident_memory_bytes / 1024 / 1024

# CPU usage
rate(process_cpu_seconds_total[1m])
```

### Tracing (Jaeger)
Key spans: `collection.cycle`, `visit.vm`, `visit.process`, `visit.thread`, `estimate.power`

---

## 🛡️ Error Handling & Data Freshness

- Collection errors are logged but do not crash the agent.
- `CachedSource` returns **`-1`** when data is stale (>5s old).
- `Estimator` returns **power = 0** on stale data.
- This prevents misleading energy calculations from old values.

---

## 🪟 Windows Support

- **VM & Process Level**: Good support via `gopsutil`.
- **Thread Level**: Limited. Returns `CPU: 0` (accurate per-thread CPU is hard on Windows).
- Dedicated `thread_windows.go` exists.

---

## 📈 Performance

- Target: **<2% CPU**, **<100MB Memory**
- Achieved mainly via `CachedSource` (avoids repeated expensive syscalls).
- Agent is stateless and lightweight → Easy to deploy at scale.

---

## 🧪 Design Patterns

| Pattern     | Used For |
|-------------|----------|
| Facade      | `Agent` hides complexity |
| Composite   | VM → Process → Thread tree |
| Visitor     | Collection + Export logic |
| Iterator    | Tree traversal |
| Decorator   | `CachedSource` for caching |
| Strategy    | Power estimation model |

---

## 📁 Project Structure

```
internal/
├── agent/          # Facade
├── collector/      # VM, Process, Thread, CachedSource, Iterator
├── estimator/      # Power calculation
├── exporter/       # Prometheus + OTEL
├── otel/           # Tracing setup
└── visitor/        # ExportVisitor
```

---

## 🔮 Future Work

- Improve Windows thread CPU accuracy
- Kubernetes DaemonSet deployment
- Remote write support
- mTLS
- Integration tests

---

**Built with Go + OpenTelemetry + Prometheus + Grafana**
