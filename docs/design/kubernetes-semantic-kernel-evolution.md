# Kubernetes Semantic Kernel Evolution

## Status

Approved macro evolution guidance.

This document defines the repo-level evolution path from the current one-shot diagnostic CLI into a continuous Kubernetes semantic kernel.

## Context

The repository has already validated the right foundation.

It should **not** pivot toward "full ontology database first." It should continue along a **graph-first, AI-agent-first** path and upgrade the current implementation from a single-run diagnostic pipeline into a continuously maintained semantic kernel.

Today the repository already contains the minimum viable kernel shape:
- Read-only collection: `internal/collect/k8s/collector.go`
- Canonical identity and typed model: `internal/model/identity.go`, `internal/model/node.go`, `internal/model/edge.go`
- Relation recovery and inference: `internal/graph/builder.go`, `internal/resolve/**/*`
- Machine-consumable contract: `internal/api/types.go`, `AI_CONTRACT.md`
- AI-oriented query path: `internal/service/diagnostic/service.go`

The main gap is not product direction.

The gap is runtime shape.

The current system is still a one-shot CLI orchestrator in `cmd/kubernetes-ontology/entry.go`: collect, build, store, query, exit. That is good for phase-1 validation, but it does not match the long-term goal:

> continuously maintain memory dependency state and return contract-shaped data when clients request it.

## North Star

Build a **continuous semantic kernel for Kubernetes** that:
- maintains a provenance-aware dependency memory for a cluster
- serves AI-agent-oriented graph queries on demand
- cleanly separates operational facts from later semantic mapping and reasoning
- allows richer human-facing tooling to grow on top, not beside it

This means the project should evolve from:
- one-shot CLI diagnosis

to:
- long-lived kernel runtime
- query-serving contract layer
- semantic mapping hooks
- optional later persistent backend and reasoning pipeline

## Recommended Direction

**Keep the existing graph/kernel/contract spine. Refactor runtime shape first. Do not prioritize full RDF/OWL materialization yet.**

The near-term goal is to make the current implementation trustworthy as a long-lived fact kernel.

Then grow:
1. richer diagnostic query families
2. ontology mapping / export
3. reasoning hooks
4. non-memory backend support, including Reef-class adapters

## Why the Current Trajectory Is Reasonable

### Already correct
- `AI_CONTRACT.md` is the right kind of stable consumer boundary.
- `internal/model/identity.go` provides the right long-term identity spine.
- `internal/graph/builder.go` already separates explicit facts from inferred relations.
- `internal/store/store.go` already creates an adapter boundary for future backends.

### Currently limiting
- `cmd/kubernetes-ontology/entry.go` is too central, because it hardwires the whole product into a single-run orchestration flow.
- `internal/collect/k8s/collector.go` is snapshot-oriented, not continuous.
- `internal/service/diagnostic/service.go` behaves like one query implementation, not a general query facade.
- `internal/store/memory/store.go` is still treated like temporary per-run storage rather than long-lived kernel state.

## Target Runtime Shape

```text
Kubernetes API (read-only list/watch)
        -> Collector Runtime
        -> Normalization / Relation Resolution
        -> Continuous Graph Kernel
        -> Query Contract Layer
        -> Clients (AI agents / tools / future human UX)
                         \
                          -> Ontology Mapping / Export / Reasoning Hooks
```

The kernel should not rebuild state from scratch every time a client asks a question.

Instead:
- the kernel continuously maintains cluster memory dependency state
- clients send entry/query/policy requests
- the service returns stable contract-shaped subgraphs, evidence, and extensible semantic views

## Architecture Phases

### Phase 1: Stabilize the semantic kernel contract
Goal: turn the current implementation from "works now" into "safe to build on."

Key work:
- stabilize node / edge / provenance semantics in `AI_CONTRACT.md`
- define the query surface as a family of kernel queries, not just one diagnostic helper
- tighten API / model / runtime boundaries so the CLI no longer defines the product
- explicitly separate stable semantics from best-effort evidence

Preserve:
- `internal/api/types.go`
- `internal/model/*`
- `internal/resolve/**/*`
- `internal/graph/builder.go`

Refactor early:
- `cmd/kubernetes-ontology/entry.go`, reducing it from product entrypoint to CLI adapter / debug tool

### Phase 2: Upgrade from snapshot CLI to continuous in-memory kernel
Goal: evolve from one-shot graph construction to continuously maintained graph state.

Key work:
- add a runtime layer that starts collectors and reconciles graph updates
- evolve collection from `Collect(ctx) -> Snapshot` toward continuous event or delta-driven updates
- support local graph mutations without full rebuild for every query
- treat memory store as long-lived process state

Likely focal areas:
- `internal/collect/k8s/collector.go`
- `internal/graph/kernel.go`
- `internal/store/store.go`
- `internal/store/memory/store.go`

### Phase 3: Introduce a service query surface
Goal: let clients query a running kernel instead of shelling out to a CLI.

Key work:
- split query serving from CLI execution
- preserve the CLI as a client / debug adapter
- widen the query layer beyond the current diagnostic subgraph path

Natural preservation point:
- `internal/service/diagnostic/service.go` remains valuable, but becomes one query service implementation inside a broader contract layer

### Phase 4: Add semantic mapping layer
Goal: make the graph truly semantic-ready without forcing ontology storage into the hot path.

Key work:
- define explicit graph node / edge to ontology class / property mappings
- separate operational facts from semantic abstractions
- support export or sync hooks for downstream semantic systems

Rule:

**The semantic layer grows on top of the kernel. It must not distort the kernel’s operational fact model.**

### Phase 5: Add persistent backend and reasoning hooks
Goal: evolve from continuous in-memory kernel to recoverable, extensible semantic infrastructure.

Key work:
- add non-memory store adapters
- support restart recovery, state rebuild, and versioned schema/mapping evolution
- only then consider RDF/OWL materialization, rule execution, and deeper reasoning workflows as first-class runtime concerns

This is the phase where Reef-class or ontology database integrations should become central.

## What Must Be Preserved

These are not temporary phase-1 details. They are the durable spine of the system.

- **Canonical identity spine**: `internal/model/identity.go`
- **Typed node/edge model**: `internal/model/node.go`, `internal/model/edge.go`
- **Resolver-based relation recovery**: `internal/resolve/**/*`
- **Provenance-aware query contract**: `internal/api/types.go`, `AI_CONTRACT.md`
- **Diagnostic subgraph as a killer query**: `internal/service/diagnostic/service.go`

## What Should Change Early

1. **Reduce the CLI to an adapter**
   - `cmd/kubernetes-ontology/entry.go` currently acts like the whole product
2. **Move from snapshot collection to continuous update model**
   - `internal/collect/k8s/collector.go` is the largest short-term architectural bottleneck
3. **Evolve builder logic beyond full rebuilds**
   - `internal/graph/builder.go` should eventually support initial build plus incremental reconciliation concepts
4. **Expand from one query implementation to query facade**
   - `internal/service/diagnostic/service.go` is too narrow to remain the only contract layer

## Risks to Avoid

### 1. Ontology-database-first drift
This would overinvest in representation format before the fact layer is trustworthy.

### 2. Diagnosis-tool-only drift
If the project keeps expanding diagnosis features without upgrading runtime shape, it will become a strong troubleshooting tool but not a semantic kernel.

### 3. Runtime / kernel / query entanglement
These layers must separate cleanly, or later serviceization will become painful.

### 4. No boundary between stable semantics and best-effort evidence
If AI clients cannot distinguish strong facts from weaker explanation evidence, they will over-trust the graph.

## Milestone Sequence

Recommended sequence:

1. Define the continuous kernel runtime boundary
2. Split CLI orchestration away from core kernel behavior
3. Upgrade collect/build/store toward a continuous update model
4. Serviceize the current diagnostic API
5. Add ontology mapping / export layer
6. Only then add persistent backend and reasoning engine concerns

## Verification Guidance

The near-term verification question is not “did we add features?”

It is “does the evolution path preserve the right assets while changing the runtime shape?”

Use these checks:
- compare against `README.md` and `AI_CONTRACT.md` to ensure the phase-1 consumer contract survives
- compare against `cmd/kubernetes-ontology/entry.go` to ensure the CLI becomes an adapter instead of the system core
- compare against `internal/collect/k8s/collector.go` to ensure the next stage introduces continuous updates
- compare against `internal/graph/builder.go` and `internal/service/diagnostic/service.go` to ensure they are preserved and elevated, not discarded
- evolve tests such as `internal/service/diagnostic/service_test.go`, `internal/graph/contract_test.go`, and `internal/collect/k8s/collector_test.go` from single-run result checks toward incremental update and long-lived state consistency checks

## Bottom Line

The repository is on the right path.

The next important move is not “switch to ontology.”

It is:

**upgrade the current diagnostic graph core from a one-shot CLI into a continuously maintained semantic kernel runtime.**
