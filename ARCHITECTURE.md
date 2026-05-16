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
│    ├── Estimate Power (via Estimator)                       │
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
- **Data Freshness**: Stale data is explicitly marked (`-1`) to avoid wrong power calculations.
- **Observability by Default**: Tracing and metrics are built-in, not added later.

---

## 3. Key Components

### 3.1 Agent (Facade Pattern)

**Location:** `internal/agent/agent.go`

The `Agent` is the single entry point. It:
- Creates the VM → Process → Thread hierarchy
- Starts a ticker that triggers collection every 5 seconds
- Starts the Prometheus HTTP server
- Manages graceful shutdown

**Why Facade?**  
External code only needs to call `New()` and `Run()`. All internal wiring is hidden.

---

### 3.2 Collection Layer (Composite Pattern)

**Components:**
- `VMCollector`
- `ProcessCollector`
- `ThreadCollector`

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
- Treats individual objects (threads) and compositions (VMs containing processes) uniformly.
- Easy to traverse the entire hierarchy.

---

### 3.3 Traversal Layer (Iterator Pattern)

**Location:** `internal/collector/iterator.go`

`TreeIterator` performs **Breadth-First Search (BFS)** over the tree.

**Why Iterator?**
- Decouples traversal logic from the node structure.
- Easy to change traversal strategy (BFS vs DFS) later.
- Clean `for node := iter.Next(); node != nil; ...` loop in `Agent`.

---

### 3.4 Processing Layer (Visitor Pattern)

**Location:** `internal/visitor/export_visitor.go`

`ExportVisitor` is responsible for:
1. Collecting metrics from each node
2. Estimating power consumption
3. Updating Prometheus gauges

**Why Visitor?**
- Adds new behavior (export + estimation) **without modifying** the `MetricSource` structs.
- Follows the **Open/Closed Principle**.

---

### 3.5 Performance Optimization (Decorator Pattern)

**Location:** `internal/collector/decorator.go`

`CachedSource` wraps any `MetricSource` and caches results for 5 seconds.

**Key Behavior:**
- On success → Cache result
- On failure + data < 5s old → Return cached data
- On failure + data > 5s old → Return `-1` (instead of stale data)

**Why Decorator?**
- Adds caching behavior transparently.
- Directly helps achieve the **<2% CPU** target by reducing syscalls.

---

### 3.6 Power Estimation (Strategy Pattern)

**Location:** `internal/estimator/estimator.go`

```go
Power = a*CPU + b*Memory + c*Disk + d*Network
```

- Coefficients are loaded from environment variables.
- If any metric is `-1` (stale), it returns `0` power.

**Why Strategy?**
- Easy to swap estimation algorithms in the future.
- Coefficients are configurable at runtime.

---

### 3.7 Export Layer

**Location:** `internal/exporter/exporter.go`

- Exposes `/metrics` endpoint in Prometheus format.
- Registers custom gauges with rich labels.
- Auto-instruments HTTP handlers with OpenTelemetry.

---

### 3.8 Observability Layer

- **Prometheus**: Metrics collection
- **OpenTelemetry + Jaeger**: Distributed tracing
- **Grafana**: Visualization

All collection, estimation, and export steps emit traces.

---

## 4. Data Freshness & Error Handling Strategy

This is one of the most important design decisions:

| Situation                        | Behavior                                      | Reason |
|----------------------------------|-----------------------------------------------|--------|
| Collection succeeds              | Cache result                                  | Performance |
| Collection fails + data < 5s     | Return cached data                            | Stability |
| Collection fails + data > 5s     | Return `-1`                                   | Avoid misleading old data |
| Estimator sees `-1`              | Return power = `0`                            | Prevent wrong energy calculations |
| One node fails                   | Log warning + continue with other nodes       | Resilience |

This design ensures that **stale data does not corrupt power estimation**.

---

## 5. Platform Support

| Platform     | VM Level | Process Level | Thread Level     | Notes |
|--------------|----------|---------------|------------------|-------|
| **Linux**    | Full     | Full          | Full (accurate)  | Best support |
| **Windows**  | Good     | Good          | Limited (CPU=0)  | Uses `gopsutil` |
| **macOS**    | Good     | Good          | Limited          | Falls back to stub |

---

## 6. Scalability Considerations

**Current Strengths:**
- Agent is very lightweight.
- Stateless design.
- Rich labeling strategy.
- Standard Prometheus + OpenTelemetry integration.

**Future Needs for Large Scale (10k+ VMs):**
- Kubernetes DaemonSet + PodMonitor
- Remote write to scalable TSDB (VictoriaMetrics / Thanos)
- Hierarchical Prometheus federation
- mTLS
- Record rules for aggregation

---

## 7. Summary of Design Patterns

| Pattern      | Location                        | Benefit |
|--------------|----------------------------------|--------|
| **Facade**   | `Agent`                          | Simple external interface |
| **Composite**| Collector hierarchy              | Uniform treatment of VM/Process/Thread |
| **Visitor**  | `ExportVisitor`                  | Add behavior without modifying nodes |
| **Iterator** | `TreeIterator`                   | Clean, decoupled traversal |
| **Decorator**| `CachedSource`                   | Transparent caching + staleness handling |
| **Strategy** | `Estimator`                      | Swappable power model |

---

This architecture prioritizes **correctness** (fresh data), **performance** (caching), **maintainability** (patterns), and **observability** (tracing + metrics).