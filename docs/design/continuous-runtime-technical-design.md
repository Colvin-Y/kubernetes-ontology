# Continuous Runtime Technical Design

## Status

Approved technical design for the first runtime-focused implementation slice.

This document defines how `kubernetes-ontology` should evolve from a one-shot CLI into a long-lived, continuously maintained semantic kernel runtime.

## Background

The repo-level evolution document `docs/design/kubernetes-semantic-kernel-evolution.md` already defines the macro direction:
- keep the graph/kernel/contract spine
- refactor runtime shape first
- do not lead with ontology database materialization

This document narrows that into the first concrete technical design.

The immediate target is not “build the final daemon.”

It is:

**separate runtime lifecycle, state ingestion, graph reconciliation, and query serving so the current implementation can become a long-lived kernel process.**

## Problem in the Current Implementation

Today the executable in `cmd/kubernetes-ontology/main.go` does the whole product in one pass:
1. build kube client
2. run read-only collection
3. build a full graph from a snapshot
4. load it into memory
5. run one diagnostic query
6. print JSON
7. exit

That works for phase-1 validation. It does not work as a continuous kernel.

### Current structural bottlenecks

#### 1. No runtime lifecycle
`cmd/kubernetes-ontology/main.go` owns the entire orchestration flow. There is no long-lived runtime manager, no readiness concept, and no kernel state beyond one process invocation.

#### 2. Collection is snapshot-only
`internal/collect/k8s/collector.go` exposes a single `Collect(ctx) (Snapshot, error)` flow. Good for bootstrap. Not enough for continuous state maintenance.

#### 3. Graph construction is full-build only
`internal/graph/builder.go` only supports `Build(snapshot)`. There is no explicit place for partial recomputation, scoped rebuild, or event-driven mutation planning.

#### 4. Query serving is too narrow
`internal/service/diagnostic/service.go` is a useful query implementation, but it behaves like the entire query surface. The repo needs a query facade above it.

#### 5. Memory store is treated as temporary
`internal/store/memory/store.go` can already hold long-lived state, but the current runtime treats it as scratch storage for a single command.

## Design Goals

### 1. Preserve the current semantic spine
The design must preserve and build on:
- `internal/model/identity.go`
- `internal/model/node.go`
- `internal/model/edge.go`
- `internal/graph/kernel.go`
- `internal/service/diagnostic/service.go`
- `AI_CONTRACT.md`

### 2. Separate runtime from query logic
The runtime should own lifecycle and state freshness. Query services should consume a ready kernel, not build one.

### 3. Keep bootstrap simple
The first continuous version can still start from a full snapshot and a full graph build.

That is fine.

The important change is process shape, not algorithmic perfection.

### 4. Create an explicit path to continuous updates
The design must make room for watch/informer-driven updates without requiring them in the first implementation slice.

### 5. Avoid locking protocol or backend too early
This design should not force early decisions about:
- HTTP vs gRPC
- Reef vs another persistent backend
- RDF/OWL sync strategy

## Non-Goals

This document does **not** design:
- ontology mapping internals
- RDF/OWL materialization pipeline
- Reef adapter details
- final public network API protocol
- reasoning engine orchestration
- multi-cluster coordination

## Target Runtime Architecture

```text
Kubernetes API
   |
   +--> Snapshot Collector ------------------+
   |                                         |
   +--> Future Change Stream --------------- | -------------------+
                                             v                    |
                                     Runtime Manager              |
                                   (lifecycle + status)          |
                                             |                    |
                                             v                    |
                                       Reconcile Layer <----------+
                                  (full build now, scoped updates later)
                                             |
                                             v
                                       Graph Kernel
                                 (store + index + typed facts)
                                             |
                                             v
                                        Query Facade
                                   /                         \
                                  v                           v
                     Diagnostic Query Service        Future Query Services
```

## Layer Responsibilities

### Runtime Layer
Purpose: own process lifecycle and kernel readiness.

Responsibilities:
- start bootstrap sync
- hold references to collector, reconcile, kernel, and query facade
- expose runtime status
- own shutdown behavior
- decide when queries are allowed

This layer should be new.

Suggested package:
- `internal/runtime`

### Collect Layer
Purpose: convert Kubernetes state into normalized inputs for the kernel.

Two sub-modes are needed.

#### A. Snapshot collection
Keep the current `Collect(ctx) (Snapshot, error)` flow in `internal/collect/k8s/collector.go`.

Role:
- initial bootstrap source of truth

#### B. Future change stream
Add a second concept for normalized changes from watch/informer sources.

Role:
- continuous updates after bootstrap

This must be separate from the existing snapshot collector so the current implementation can survive intact.

### Reconcile Layer
Purpose: translate collected state or change events into kernel mutations.

This layer should sit between collection and kernel.

Responsibilities:
- perform full rebuild at bootstrap time
- later perform scoped recomputation or partial rebuild on affected graph regions
- centralize “what part of the graph needs updating?”

Important rule:
- `internal/graph/builder.go` should remain the full-build implementation
- reconcile should call into it at first, not replace it immediately

### Query Layer
Purpose: provide a stable, extensible boundary for kernel queries.

Responsibilities:
- route request types to specific query services
- hold a reference to the ready kernel
- isolate future protocol adapters from internal query implementations

The current diagnostic query remains important, but it should sit under this facade.

## Proposed Package Layout

```text
cmd/
  kubernetes-ontology/           # CLI adapter / debug entry
  kubernetes-ontologyd/          # long-lived daemon entrypoint

internal/runtime/
  manager.go
  bootstrap.go
  status.go
  loop.go

internal/collect/k8s/
  collector.go                   # existing snapshot collector
  stream.go                      # future change stream entrypoint
  events.go                      # future normalized change events

internal/reconcile/
  full.go                        # full snapshot -> kernel apply
  planner.go                     # future affected-scope planning
  applier.go                     # future mutation application

internal/query/
  facade.go

internal/ontology/
  backend.go                     # entity/relation backend abstraction
  types.go

internal/server/
  http.go                        # daemon HTTP API

internal/service/diagnostic/
  service.go                     # existing diagnostic query implementation
```

This layout intentionally preserves current files and adds new orchestration layers around them.

## Interface Sketch

These are design-level interfaces, not final code.

### Runtime manager
```go
type Manager interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Status() Status
    QueryFacade() *query.Facade
}
```

### Runtime status
```go
type Phase string

const (
    PhaseStarting     Phase = "starting"
    PhaseBootstrapping Phase = "bootstrapping"
    PhaseReady        Phase = "ready"
    PhaseDegraded     Phase = "degraded"
    PhaseStopping     Phase = "stopping"
)

type Status struct {
    Phase           Phase
    LastBootstrapAt *time.Time
    LastSyncAt      *time.Time
    LastError       string
}
```

### Snapshot collector
The existing concept remains valid.

```go
type SnapshotCollector interface {
    Collect(ctx context.Context) (k8s.Snapshot, error)
}
```

### Future change stream
```go
type ChangeStream interface {
    Run(ctx context.Context, sink ChangeSink) error
}

type ChangeSink interface {
    Apply(ctx context.Context, event ChangeEvent) error
}
```

### Reconcile boundary
```go
type Reconciler interface {
    ApplySnapshot(ctx context.Context, snapshot k8s.Snapshot) error
    ApplyChange(ctx context.Context, event ChangeEvent) error
}
```

### Query facade
```go
type Facade struct {
    Diagnostic *diagnostic.Service
}
```

The first version can stay simple. The point is architectural placement, not abstraction theater.

## State Model

The runtime should have explicit lifecycle states.

### starting
Process has initialized but has not begun bootstrap.

Query behavior:
- reject with “runtime not ready”

### bootstrapping
Snapshot collect and full graph load are in progress.

Query behavior:
- reject by default
- optional future mode: allow only if a prior ready snapshot exists

### ready
Kernel is loaded and queryable.

Query behavior:
- allow all supported query families

### degraded
Runtime is alive but sync/update health is impaired.

Examples:
- change stream failed
- collector encountered transient errors after a previously successful bootstrap

Query behavior:
- allow reads against last known good kernel state
- surface degraded metadata in runtime status

### stopping
Shutdown in progress.

Query behavior:
- reject new queries

## First Implementation Slice

The first implementation should be deliberately boring.

That is good.

### Scope
Build a long-lived runtime **without** watch/informer support yet.

Behavior:
- start process
- perform full snapshot collection
- perform full graph build
- load kernel into long-lived memory store
- expose query access through internal facade
- keep process alive

This already converts the project from:
- per-request rebuild

to:
- long-lived in-memory semantic kernel

That is the real first milestone.

### Why this slice first
Because the main bottleneck today is shape, not update sophistication.

If the project jumps directly to watch/informer logic before separating runtime and query boundaries, the code will just become a more complicated one-shot tool.

## Migration Plan

### Step 1: Introduce runtime package
Add `internal/runtime` with manager lifecycle and explicit status model.

### Step 2: Move bootstrap orchestration out of CLI main
Refactor the logic currently in `cmd/kubernetes-ontology/main.go` into runtime bootstrap code.

The CLI should become a consumer of runtime behavior, not the definition of the system.

### Step 3: Introduce reconcile boundary
Move snapshot -> graph -> kernel load orchestration behind a reconciler concept.

At first, `ApplySnapshot` can simply:
- run `builder.Build(snapshot)`
- replace or reload kernel state

### Step 4: Add query facade
Wrap `internal/service/diagnostic/service.go` with a query facade that hangs off a ready runtime.

### Step 5: Add long-lived entrypoint
Introduce a daemon-style command, likely `cmd/kubernetes-ontologyd`, while keeping the existing CLI as a debug adapter.

### Step 6: Add future change stream and incremental reconcile
Only after the above shape exists should the repo add:
- watch/informer inputs
- normalized change events
- scoped graph updates

## Impact on Existing Files

### `cmd/kubernetes-ontology/main.go`
Will stop being the system core.

Future role:
- CLI adapter / debug entrypoint
- possibly connecting to an in-process or external runtime

### `cmd/kubernetes-ontology/entry.go`
Its `findEntryID` helper is still useful, but it should no longer depend on fresh snapshot ownership at query time.

Future direction:
- resolve entry identity from the maintained kernel where possible

### `internal/collect/k8s/collector.go`
Preserve as snapshot bootstrap collector.

Do not overload it with both full snapshot and long-lived stream semantics in one interface.

### `internal/graph/builder.go`
Preserve as full-build logic.

Do not prematurely force it into partial-reconcile responsibility.

### `internal/graph/kernel.go`
Remains the core in-memory graph facade.

Longer term, it may need higher-level operations for state replacement or scoped mutation support, but its central role stays intact.

### `internal/store/memory/store.go`
Same implementation can power the first long-lived runtime.

The key change is operational role, not immediate backend replacement.

### `internal/service/diagnostic/service.go`
Preserve as the first query service implementation under a query facade.

## Testing Strategy

### Unit tests
Add tests for:
- runtime state transitions
- bootstrap success/failure handling
- query availability by runtime phase
- reconcile snapshot application

### Integration tests
Add tests for:
- bootstrap from fake Kubernetes client into long-lived runtime
- query after runtime reaches ready state
- degraded mode after simulated collector/update failure

### Future consistency tests
When change streams exist, add tests for:
- event -> reconcile -> kernel update correctness
- last known good state behavior under transient update failures
- no contract drift in `AI_CONTRACT.md` output semantics

## Open Questions

These are intentionally left open for the next design layer, not this one.

1. Should the first long-lived runtime expose an internal API only, or immediately ship an external HTTP/gRPC surface?
2. Should bootstrap load replace the whole kernel atomically, or mutate an existing one in place?
3. How should entry lookup evolve once queries are no longer backed by a fresh snapshot?
4. What minimal runtime metadata should be exposed to clients alongside graph query results?

## Bottom Line

Do not start by wrapping the current CLI in a server.

Do not start by wiring ontology storage into the hot path.

Start by creating a real runtime.

The first technical win is:

**turn the current collect/build/query pipeline into a long-lived runtime with a reusable query boundary and a future reconcile path.**
