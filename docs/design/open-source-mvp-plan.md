# Open Source MVP Implementation Plan

## Purpose

This plan turns the current continuous runtime alpha into an open-source MVP for
read-only Kubernetes topology recovery and AI-agent-assisted diagnostics.

The MVP should be useful without requiring any cluster-side installation, graph
database, persistent store, or write access to Kubernetes.

## MVP Decisions

### In Scope

- Keep the runtime backend in memory only.
- Rebuild topology from Kubernetes state after process restart.
- Complete RBAC topology for ServiceAccounts, RoleBindings, and
  ClusterRoleBindings.
- Use informer/watch as the primary continuous update path.
- Keep polling as a fallback stream for environments where informers are not
  available or are explicitly disabled.
- Make the query API and CLI friendly for AI agents:
  - stable entity and relation JSON
  - bounded expansion
  - focused diagnostics
  - machine-readable errors
  - clear status and freshness metadata
- Document agent-facing CLI recipes so downstream agents can invoke the CLI and
  consume ontology output consistently without shipping local agent state.
- Mature the viewer UX while keeping it intentionally focused on human-assisted
  inspection of live topology and diagnostic subgraphs.

### Out of Scope

- Persistent graph storage.
- External graph adapters.
- RDF/OWL materialization.
- Runtime recovery beyond rebuilding in memory after restart.
- Cluster-side agents, CRDs, controllers, or mutating Kubernetes resources.
- Full Kubernetes RBAC authorization reasoning.
- Public multi-tenant API hardening.

## Current Baseline

The repository already has:

- in-memory graph kernel
- runtime manager and daemon
- CLI server-client mode
- polling stream
- scoped mutation for service, event, storage, pod, and workload changes
- diagnostic subgraph query
- generic entity, relation, neighbor, and expand endpoints
- local graph viewer
- YAML configuration
- custom workload resource collection and controller display rules

## Workstreams

### 1. RBAC Topology

Goal: make identity and security relationships visible in the graph.

Tasks:

- Add RoleBinding and ClusterRoleBinding nodes during full graph build.
- Add edges from ServiceAccount subjects to RoleBinding and
  ClusterRoleBinding nodes.
- Preserve provenance with `binding_resolution` and resolver
  `rbac-binding/v1`.
- Add an `identity/security-narrow` reconcile strategy.
- Update polling and informer category mapping so identity/security changes use
  scoped mutation rather than full rebuild.
- Cover ServiceAccount, RoleBinding, and ClusterRoleBinding add/update/delete
  behavior with tests.

Acceptance:

- Full rebuild output includes RBAC nodes and binding edges.
- Scoped identity/security mutation matches full rebuild results for RBAC graph
  facts.
- Diagnostic traversal can include RBAC boundary nodes without exploding by
  default.

### 2. Informer Runtime

Goal: use Kubernetes watch/informer as the primary continuous update source.

Tasks:

- Implement a `Stream` backed by shared informers or watches.
- Emit normalized `ChangeEvent` values using the existing sink contract.
- Cover workload, pod, service, storage, event, and identity/security resource
  categories.
- Keep `PollingStream` available as fallback.
- Add CLI and daemon flags/config for stream mode:
  - `--stream-mode informer|polling`
  - fallback to polling when informer setup fails only when configured to do so
- Preserve current polling behavior for tests and constrained environments.

Acceptance:

- Daemon defaults to informer mode for continuous refresh.
- Tests validate informer event to `ChangeEvent` classification without a real
  cluster.
- Polling fallback remains available and tested.

### 3. AI-Agent Friendly Query Surface

Goal: make the API and CLI easier and safer for agents to consume.

Tasks:

- Normalize response envelopes for CLI-accessible query families where useful.
- Add machine-readable error shape for CLI server calls.
- Add status/freshness fields to graph query responses when available.
- Document stable agent workflows in README and AI contract.
- Add CLI affordances for common agent flows:
  - resolve entity
  - expand entity
  - diagnose pod/workload
  - list filtered relations
- Document reusable agent-facing commands in `AI_CONTRACT.md` and `README.md`.

Acceptance:

- Agent workflows can be performed with documented CLI commands.
- Agent-facing instructions are read-only, stable, and point to daemon-backed queries.
- Existing JSON output remains backward compatible unless explicitly versioned.

### 4. Focused Viewer UX

Goal: improve the viewer into a polished, focused human-assist tool.

Tasks:

- Keep scope narrow: live topology, pod/workload diagnostics, expand/collapse,
  filtering, details, export.
- Improve first-screen workflow for connecting to the daemon and loading a graph.
- Improve empty, loading, error, and timeout states.
- Improve graph readability for common Kubernetes topology:
  - node grouping
  - relation labels
  - selected-node details
  - focused filters
- Avoid adding broad dashboard/product analytics features.

Acceptance:

- Viewer remains static HTML plus tiny Python proxy.
- `make visualize-check` passes.
- UX supports manual inspection without requiring users to understand raw JSON.

### 5. Open Source Packaging

Goal: make the MVP understandable and safe for external users.

Tasks:

- Update README and QUICKSTART for the MVP boundaries.
- Make it explicit that MVP storage is in-memory only.
- Remove persistent backend and external graph adapter from MVP next steps.
- Clarify restart behavior: daemon rebuilds topology from Kubernetes on start.
- Keep safety/read-only model prominent.
- Ensure local config examples do not include internal paths or assumptions.

Acceptance:

- New users can build, run daemon, query via CLI, and open viewer from docs.
- The README does not imply unimplemented production recovery or external graph
  backends are MVP goals.

## Implementation Ownership

The implementation can be split across independent ownership areas:
- RBAC topology and identity/security scoped reconcile.
- Informer stream and stream-mode configuration.
- AI-agent query ergonomics and CLI docs.
- Focused viewer UX and viewer tests.

Each owner should avoid reverting changes outside their area and accommodate
parallel edits.

## Integration Order

1. Land this plan.
2. Run workers in parallel on disjoint ownership areas.
3. Review and merge worker changes.
4. Resolve integration seams in runtime config, status, docs, and tests.
5. Run:
   - `go test -p 1 ./...`
   - `make visualize-check`
   - `make build build-daemon`
6. Update this plan or a progress snapshot with what shipped and what remains.

## Quality Gates

- All changes must be read-only with respect to Kubernetes.
- No Kubernetes object create/update/delete/patch paths may be introduced.
- In-memory remains the only runtime backend for MVP.
- Informer and polling must share the existing `ChangeEvent` sink contract.
- Agent-facing JSON must remain stable and machine-readable.
- Viewer changes must stay lightweight and require no frontend build system.
- All tests must pass before the work is considered complete.

## Implementation Status

This branch implements the MVP plan:

- RBAC topology for ServiceAccount subjects and RoleBinding /
  ClusterRoleBinding nodes is implemented.
- `identity/security-narrow` scoped reconciliation is implemented.
- Informer-backed streaming is implemented as the default daemon stream mode.
- Polling remains available as `streamMode: polling` / `--stream-mode polling`
  and as informer fallback.
- AI-agent CLI aliases, additive freshness metadata, structured error payloads,
  and public agent-facing command docs are implemented.
- The viewer remains static HTML plus a small Python proxy and has a focused UX
  pass for loading, errors, filters, selection details, and graph readability.

Verification commands for this implementation:

```bash
go test -p 1 ./...
make visualize-check
make build build-daemon
```
