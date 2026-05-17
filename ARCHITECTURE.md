# VM Energy Agent - Architecture

This document explains the architecture, design decisions, and data flow of the VM Energy Agent.

---

## 1. High-Level Architecture

The system follows a **clean, layered architecture** with strong separation of concerns.

```
┌─────────────────────────────────────────────────────────────┐
│                        main.go                              │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                     Agent (Facade)                          │
│  - Discovers all processes on the host/VM                   │
│  - Wires all components                                     │
│  - Starts collection ticker (every 5s)                      │
│  - Starts Prometheus HTTP server                            │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                  Collection Layer                           │
│  VMCollector → ProcessCollector → ThreadCollector           │
│  (Composite Pattern)                                        │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                  Traversal Layer                            │
│  TreeIterator (Breadth-First Traversal)                     │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                  Processing Layer                           │
│  ExportVisitor                                              │
│    ├── Collect metrics (via CachedSource)                   │
│    ├── Estimate Power in Watts (via Estimator)              │
│    └── Export to Prometheus                                 │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                  Observability Layer                        │
│  - Prometheus Exporter                                      │
│  - OpenTelemetry Tracing → Jaeger                           │
│  - Grafana Dashboards                                       │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. Core Design Principles

- **Separation of Concerns**: Collection, Estimation, and Export are decoupled.
- **Open/Closed Principle**: New collectors or estimators can be added without modifying existing code.
- **Performance First**: Heavy use of caching (`CachedSource`) to stay under 2% CPU.
- **Data Freshness**: Stale data is explicitly handled to avoid wrong power calculations.
- **Observability by Default**: Tracing and metrics are built-in.
- **Correct Units**: Memory is reported in **bytes**, and power is estimated in **watts**.

---

## 3. Key Components

### 3.1 Agent (Facade Pattern)

**Location:** `internal/agent/agent.go`

The `Agent` is the single entry point. It:
- Discovers **all processes** running on the host/VM and builds the full VM → Process → Thread hierarchy.
- Starts a ticker that triggers collection every 5 seconds.
- Starts the Prometheus HTTP server.
- Manages graceful shutdown.

**Important Note**:  
Process and thread metrics now reflect **actual workloads** on the VM, not just the agent itself. The agent’s own process receives additional dynamic thread discovery on every cycle.

**Why Facade?**  
External code only needs to call `New()` and `Run()`. All internal wiring is hidden.

---

### 3.2 Collection Layer (Composite Pattern)

**Components:**
- `VMCollector` — Host-wide metrics (CPU%, Memory in **bytes**, Disk I/O, Network)
- `ProcessCollector` — Per-process CPU and Memory (**bytes**). Manages child threads.
- `ThreadCollector` — Per-thread CPU usage (accurate on Linux).

All implement the `MetricSource` interface:
```go
type MetricSource interface {
    Name() string
    Collect() (*Metrics, error)
    Children() []MetricSource
    Accept(Visitor)
}
```

**Why Composite?**
- Treats individual objects (threads) and compositions (processes containing threads) uniformly.
- Easy to traverse the entire hierarchy using the Iterator.

---

### 3.3 Traversal Layer (Iterator Pattern)

**Location:** `internal/collector/iterator.go`

`TreeIterator` performs **Breadth-First Search (BFS)** over the process tree.

**Why Iterator?**
- Decouples traversal logic from the node structure.
- Enables clean iteration: `for node := iter.Next(); node != nil; ...`

---

### 3.4 Processing Layer (Visitor Pattern)

**Location:** `internal/visitor/export_visitor.go`

`ExportVisitor` is responsible for:
1. Collecting metrics from each node (via `CachedSource`)
2. Estimating power consumption in **watts**
3. Updating Prometheus gauges

**Why Visitor?**
- Adds new behavior (collection + export + estimation) **without modifying** the `MetricSource` structs.
- Follows the **Open/Closed Principle**.

---

### 3.5 Performance Optimization (Decorator Pattern)

**Location:** `internal/collector/decorator.go`

`CachedSource` wraps any `MetricSource` and caches results for 5 seconds.

**Key Behavior:**
- On success → Cache result
- On failure + fresh data available → Return cached data
- On failure + stale data → Return error (prevents bad power calculations)

**Why Decorator?**
- Adds caching transparently.
- Helps achieve the **<2% CPU** target by reducing expensive syscalls.

---

### 3.6 Power Estimation (Strategy Pattern)

**Location:** `internal/estimator/estimator.go`

The estimator converts resource usage into **watts**:

```go
Power (watts) = normalized(CPU, Memory_bytes, Disk, Network)
```

- Memory is **normalized** from bytes to a 0–100 score (using a 128GB ceiling).
- Disk and Network are also normalized.
- Final value is scaled to a configurable watt ceiling (default: 250W).

**Why Strategy?**
- Easy to swap or improve the power model in the future.
- Coefficients are configurable via environment variables.

---

### 3.7 Export Layer

**Location:** `internal/exporter/exporter.go`

- Exposes `/metrics` in Prometheus format.
- Registers gauges with rich labels (`instance`, `vm_name`, `level`, `node`).
- `vm_collection_errors_total` uses reduced labels to avoid high cardinality.
- HTTP handlers are auto-instrumented with OpenTelemetry.

---

### 3.8 Observability Layer

- **Prometheus**: Primary metrics system
- **OpenTelemetry + Jaeger**: Distributed tracing
- **Grafana**: Visualization dashboards

All major operations (`collection.cycle`, `visit.*`, `estimate.power`) emit traces.

---

## 4. Data Freshness & Error Handling

| Situation                        | Behavior                              | Reason |
|----------------------------------|---------------------------------------|--------|
| Collection succeeds              | Cache result                          | Performance |
| Collection fails + data fresh    | Return cached data                    | Stability |
| Collection fails + data stale    | Return error (no `-1` pushed)         | Prevent misleading metrics |
| Estimator sees stale data        | Return `0` watts                      | Avoid wrong power values |
| One node fails                   | Log + continue with others            | Resilience |

---

## 5. Platform Support

| Platform     | VM Level | Process Level | Thread Level     | Notes |
|--------------|----------|---------------|------------------|-------|
| **Linux**    | Full     | Full          | Full (accurate)  | Best support |
| **Windows**  | Good     | Good          | Limited          | Uses `gopsutil` |
| **macOS**    | Good     | Good          | Limited          | Fallbacks used |

---

## 6. Summary of Design Patterns

| Pattern      | Location                        | Benefit |
|--------------|----------------------------------|--------|
| **Facade**   | `Agent`                          | Simple external interface |
| **Composite**| Collector hierarchy              | Uniform treatment of VM/Process/Thread |
| **Visitor**  | `ExportVisitor`                  | Add behavior without modifying nodes |
| **Iterator** | `TreeIterator`                   | Clean, decoupled traversal |
| **Decorator**| `CachedSource`                   | Transparent caching + staleness handling |
| **Strategy** | `Estimator`                      | Swappable & configurable power model |

---

This architecture prioritizes **correctness** (proper units, fresh data), **performance** (caching), **maintainability** (design patterns), and **observability** (tracing + metrics).
