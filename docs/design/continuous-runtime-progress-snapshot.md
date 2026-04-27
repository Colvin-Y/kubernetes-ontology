# Continuous Runtime Progress Snapshot

> Historical snapshot: this document records the earlier continuous-runtime
> alpha state. For the open-source MVP implementation boundary and current
> status, use `docs/design/open-source-mvp-plan.md` and
> `docs/design/current-state-and-next-steps.md`.

## Purpose

This document is a restart-safe progress snapshot for the current repository state.

It summarizes what has already been implemented toward the continuous Kubernetes semantic kernel runtime, what has been verified, what is still missing, and what the next clean step should be after restarting the machine.

## Context

This snapshot builds on two repo-level design documents:
- `docs/design/kubernetes-semantic-kernel-evolution.md`
- `docs/design/continuous-runtime-technical-design.md`

The project direction remains the same:

> evolve from a one-shot diagnostic CLI into a continuous semantic kernel runtime, while staying graph-first and AI-agent-first.

## What has already been built

### 1. Runtime skeleton exists
The repository now has a real runtime layer, not just a one-shot CLI flow.

Key files:
- `internal/runtime/manager.go`
- `internal/runtime/bootstrap.go`
- `internal/runtime/status.go`

Current runtime capabilities:
- `Start(ctx)` for bootstrap full sync
- `Apply(ctx, event)` for update entrypoint
- `RunStream(ctx, stream)` so runtime can consume a change stream
- `Status()` for internal runtime state
- `RuntimeStatus()` for projected, user-facing runtime state
- `Facade()` for query access

### 2. CLI is now an adapter
The CLI has been thinned down substantially.

Key file:
- `cmd/kubernetes-ontology/main.go`

Current CLI role:
- parse flags
- build Kubernetes client
- start runtime
- run query or print status
- optionally keep polling for a bounded observation window via `--observe-duration`

The CLI is no longer the place that owns collect/build/query orchestration.

### 3. Query facade exists
Key file:
- `internal/query/facade.go`

Current facade responsibilities:
- entry lookup
- diagnostic query dispatch
- policy assembly from `diagnostic.DefaultExpansionPolicy()`
- builder evidence append
- runtime status projection storage/access

### 4. Runtime status and observability exist
The runtime now exposes meaningful status data.

Currently tracked:
- `Phase`
- `Cluster`
- `Ready`
- `NodeCount`
- `EdgeCount`
- `LastBootstrapAt`
- `LastAppliedChangeKind`
- `LastAppliedChangeNS`
- `LastAppliedChangeName`
- `LastAppliedChangeType`
- `LastAppliedChangeAt`
- `LastStrategy`
- `FullRebuildCount`
- `EventNarrowCount`
- `StorageNarrowCount`
- `ServiceNarrowCount`
- `PodNarrowCount`
- `WorkloadNarrowCount`
- `LastError`

This status is visible via:
- runtime projection
- query facade projection
- CLI `--status-only`

### 5. Polling stream exists
Key file:
- `internal/collect/k8s/stream.go`

The repo now contains a real polling-based stream implementation.

Current behavior:
- poll via existing collector
- compute category fingerprints for workload, pod, service, storage, event, and identity/security inputs
- include graph-affecting fields such as service selectors and pod labels in the fingerprint
- only emit synthetic `ChangeEvent` when a category fingerprint changes
- attach the best detected object namespace/name to the synthetic event when possible
- stay quiet when nothing changes

This is intentionally a low-risk bridge toward future informer-based streaming.

### 6. Change-event path exists
Update path is now real:

```text
Stream -> Manager.RunStream -> Manager.Apply -> planner -> reconcile -> status update
```

This is no longer theoretical architecture. The entrypoint and runtime path both exist in code.

### 7. Reconcile package is real
Key files:
- `internal/reconcile/full.go`
- `internal/reconcile/planner.go`

The reconcile package currently provides:
- `FullReconciler.Rebuild(...)`
- category-aware planner output
- scope-aware rebuild fallback shape

### 8. Category-aware strategy accounting exists
The reconcile layer is no longer blind to change category.

Currently implemented strategy categories:
- `event-narrow`
- `storage-narrow`
- `service-narrow`
- `pod-narrow`
- `workload-narrow`

Current execution truth:
- the planner classifies event/storage/service/pod/workload changes
- runtime status records `LastStrategy` and increments the matching strategy counter
- service, event, storage, pod, and workload updates use scoped graph mutation
- full rebuild remains the fallback for unsupported strategies and scoped mutation failures

That means kind observation runs now prove the long-lived update path, strategy accounting, and true partial graph mutation for these categories.

Other categories still use the `full-rebuild` strategy.

## What has already been verified

### A. Unit/integration validation
Repeated repository-wide test runs passed.

Stable command:
```bash
go test -p 1 ./...
```

Notes:
- plain `go test ./...` worked most of the time, but one run hit a system kill during `internal/collect/k8s` under higher parallelism
- `go test -p 1 ./...` was stable and green
- latest focused validation covers polling selector/label change detection and runtime strategy counter projection

### B. Real-cluster status-only smoke test
A real, read-only smoke test was run against an internal kubeconfig.

The actual path and cluster name are intentionally kept in ignored local config, not in repository docs.

Command used:
```bash
go run ./cmd/kubernetes-ontology \
  --kubeconfig "$KUBECONFIG" \
  --cluster "$CLUSTER" \
  --context-namespaces "default" \
  --status-only
```

This confirmed:
- bootstrap works on a large existing cluster
- status JSON renders correctly
- new observability fields are present
- no write operations were performed

### C. Polling stream is quiet when nothing changes
A short read-only observation run showed:
- bootstrap succeeded
- no synthetic event was emitted
- counters remained at zero

This is desirable.

It means the coarse fingerprint gate works and the runtime does not rebuild pointlessly when the observed state is unchanged.

### D. Kind cluster controlled-change test
A local kind cluster was used for a controlled long-lived observation experiment.

What was done:
1. created a minimal pod + service in kind
2. started one runtime process with `--status-only --observe-duration 24s --poll-interval 2s`
3. changed the service selector while the runtime process was still alive
4. let polling catch the change and emit a synthetic `service` event
5. inspected final runtime status from the same process

What was learned:
- the long-lived runtime saw the selector change in-process
- the stream classified the change as `service`
- runtime status populated `LastAppliedChange*`
- runtime status recorded `LastStrategy: service-narrow`
- `ServiceNarrowCount` incremented to `1`
- first run used full rebuild fallback for the update, proving bootstrap + update rebuild both happened
- follow-up implementation moved service changes to scoped graph mutation
- follow-up validation kept `FullRebuildCount` at `1`, proving the service selector update no longer triggered a second full rebuild

Observed final status after scoped service mutation:
```json
{
  "Phase": "ready",
  "Cluster": "kind-kind",
  "Ready": true,
  "NodeCount": 10,
  "EdgeCount": 7,
  "LastAppliedChangeKind": "service",
  "LastAppliedChangeNS": "ko-svc-scope-20260424163438",
  "LastAppliedChangeName": "ko-svc",
  "LastAppliedChangeType": "upsert",
  "LastStrategy": "service-narrow",
  "FullRebuildCount": 1,
  "ServiceNarrowCount": 1,
  "LastError": ""
}
```

That means continuous in-process update detection and true scoped graph mutation are now behaviorally validated for service selector changes.

### E. Kind cluster event scoped-mutation test
A second local kind experiment validated event-class scoped mutation.

What was done:
1. created a minimal pod in kind
2. started one runtime process with `--status-only --observe-duration 40s --poll-interval 2s`
3. created a core/v1 `Event` targeting that pod while the runtime was still alive
4. let polling catch the event and emit a synthetic `event` update
5. inspected final runtime status from the same process

What was learned:
- the long-lived runtime saw the Event creation in-process
- the stream classified the change as `event`
- runtime status populated `LastAppliedChange*`
- runtime status recorded `LastStrategy: event-narrow`
- `EventNarrowCount` incremented to `1`
- `FullRebuildCount` stayed at `1`, proving the Event update did not trigger a second full rebuild

Observed final status after scoped event mutation:
```json
{
  "Phase": "ready",
  "Cluster": "kind-kind",
  "Ready": true,
  "NodeCount": 10,
  "EdgeCount": 8,
  "LastAppliedChangeKind": "event",
  "LastAppliedChangeNS": "ko-event-scope-20260424170935",
  "LastAppliedChangeName": "ko-target.manual2",
  "LastAppliedChangeType": "upsert",
  "LastStrategy": "event-narrow",
  "FullRebuildCount": 1,
  "EventNarrowCount": 1,
  "LastError": ""
}
```

That means scoped graph mutation is now behaviorally validated for both service selector changes and pod-targeted event creation.

### F. Kind cluster storage scoped-mutation test
A third local kind experiment validated storage-class scoped mutation.

What was done:
1. started one runtime process against an empty test namespace with `--status-only --observe-duration 40s --poll-interval 2s`
2. created a static hostPath PV and matching PVC while the runtime was still alive
3. let polling catch the PVC/PV storage changes
4. inspected final runtime status from the same process

What was learned:
- the long-lived runtime saw the PV/PVC creation and binding in-process
- the stream classified the change as `storage`
- runtime status populated `LastAppliedChange*`
- runtime status recorded `LastStrategy: storage-narrow`
- `StorageNarrowCount` incremented to `2`, because polling observed two storage transitions
- `FullRebuildCount` stayed at `1`, proving the storage updates did not trigger a second full rebuild

Observed final status after scoped storage mutation:
```json
{
  "Phase": "ready",
  "Cluster": "kind-kind",
  "Ready": true,
  "NodeCount": 5,
  "EdgeCount": 1,
  "LastAppliedChangeKind": "storage",
  "LastAppliedChangeNS": "ko-storage-pure-20260424172500",
  "LastAppliedChangeName": "data",
  "LastAppliedChangeType": "upsert",
  "LastStrategy": "storage-narrow",
  "FullRebuildCount": 1,
  "StorageNarrowCount": 2,
  "LastError": ""
}
```

That means scoped graph mutation is now behaviorally validated for service, event, and storage updates.

### G. Kind cluster pod scoped-mutation test
A fourth local kind experiment validated pod-class scoped mutation.

What was done:
1. created a minimal pod + service in kind
2. waited for the pod to become Ready before starting observe, reducing status-noise
3. started one runtime process with `--status-only --observe-duration 40s --poll-interval 2s`
4. changed the pod label from `app=front` to `app=back` while the runtime was still alive
5. let polling catch the pod change and emit a synthetic `pod` update
6. inspected final runtime status from the same process

What was learned:
- the long-lived runtime saw the Pod label change in-process
- the stream classified the change as `pod`
- runtime status populated `LastAppliedChange*`
- runtime status recorded `LastStrategy: pod-narrow`
- `PodNarrowCount` incremented to `1`
- `FullRebuildCount` stayed at `1`, proving the Pod update did not trigger a second full rebuild

Observed final status after scoped pod mutation:
```json
{
  "Phase": "ready",
  "Cluster": "kind-kind",
  "Ready": true,
  "NodeCount": 10,
  "EdgeCount": 7,
  "LastAppliedChangeKind": "pod",
  "LastAppliedChangeNS": "ko-pod-scope-20260424173436",
  "LastAppliedChangeName": "ko-pod",
  "LastAppliedChangeType": "upsert",
  "LastStrategy": "pod-narrow",
  "FullRebuildCount": 1,
  "PodNarrowCount": 1,
  "LastError": ""
}
```

That means scoped graph mutation is now behaviorally validated for service, event, storage, and pod updates.

### H. Kind cluster workload scoped-mutation test
A fifth local kind experiment validated workload-class scoped mutation.

What was done:
1. created a temporary namespace
2. created a `Deployment` with `replicas=0`, avoiding ReplicaSet/Pod churn during the observation
3. started one runtime process with `--status-only --observe-duration 40s --poll-interval 2s`
4. changed a Deployment label while the runtime was still alive
5. let polling catch the workload change and emit a synthetic `workload` update
6. inspected final runtime status from the same process

What was learned:
- the long-lived runtime saw the Deployment label change in-process
- the stream classified the change as `workload`
- runtime status populated `LastAppliedChange*`
- runtime status recorded `LastStrategy: workload-narrow`
- `WorkloadNarrowCount` incremented to `1`
- `FullRebuildCount` stayed at `1`, proving the workload update did not trigger a second full rebuild

Observed final status after scoped workload mutation:
```json
{
  "Phase": "ready",
  "Cluster": "kind-kind",
  "Ready": true,
  "NodeCount": 4,
  "EdgeCount": 0,
  "LastAppliedChangeKind": "workload",
  "LastAppliedChangeNS": "ko-workload-scope-20260424174659",
  "LastAppliedChangeName": "ko-workload",
  "LastAppliedChangeType": "upsert",
  "LastStrategy": "workload-narrow",
  "FullRebuildCount": 1,
  "WorkloadNarrowCount": 1,
  "LastError": ""
}
```

That means scoped graph mutation is now behaviorally validated for service, event, storage, pod, and workload updates.

### I. Owner chain behavior
Owner-reference resolution is now recursive and shared by full rebuild, pod scoped mutation, and workload scoped mutation.

Current owner-chain behavior:
- controller ownerReferences are preferred when present
- if no controller marker exists, all ownerReferences are considered for backward compatibility with older tests and partial data
- lookup is UID-first, then namespace/kind/name fallback
- recursion is cycle guarded
- ReplicaSet changes are fingerprinted as workload changes and mapped to their owning workload when the owner chain can be resolved

## What is still missing

### 1. Real informer/watch source
Current stream source is polling-based.

Missing:
- watch/informer implementation
- reconnect logic
- resync policy
- event coalescing/backpressure

### 2. Broader partial graph mutation
Service, event, storage, pod, and workload changes now use scoped graph mutation.

Missing:
- scoped mutation application for identity/security categories
- edge/node deletion for those scoped updates
- consistency checks after partial mutation for those categories

### 3. More strategy categories
Currently present:
- event
- storage
- service
- pod
- workload

Still likely candidates:
- identity/security-first narrow reconcile

### 4. Smarter execution driven by planner scope
The system has crossed from “planner exists” to “planner drives scoped execution paths for service, event, storage, pod, and workload changes.”

There is still room to reduce full rebuild usage for identity/security changes.

### 5. Persistent backend, daemon hardening, and recovery
Now implemented:
- long-lived daemon binary
- network API surface for status, entity/relation queries, and diagnostic queries
- standalone CLI client mode through `--server`

Still missing:
- HTTP auth/TLS for anything beyond trusted local or controlled internal networks
- persistent backend
- restart recovery

These remain later-stage work.

## Current engineering truth

The repository is no longer a one-shot CLI prototype.

It is now best described as:

> a continuous Kubernetes ontology database alpha with a real runtime layer, an in-memory ontology backend, a daemon HTTP API, a standalone CLI client, a query facade, a polling-based update loop, category-aware planning, recursive owner-chain resolution, and validated scoped graph mutation for service, event, storage, pod, and workload updates.

That is a meaningful milestone.

## Most important next step after restart

Do **not** immediately add more architecture.

Do this first:

### Harden ontology backend contract and add identity/security scoped mutation
Goal:
- harden `internal/ontology.Backend` so external graph backends can implement it cleanly
- add the next strategy category for service accounts, role bindings, and cluster role bindings
- keep full rebuild fallback as the safety path
- prove node/edge counts and query results match a full rebuild after a controlled identity/security update

The service, event, storage, pod, and workload narrow paths are already validated. Identity/security changes are the remaining category-level source of avoidable full rebuilds.

## Safe commands to prefer after restart

For test stability:
```bash
go test -p 1 ./...
```

For real-cluster read-only status check:
```bash
make status
```

For daemon work:
```bash
make serve
make status-server
make list-entities-server ENTITY_KIND=Pod
```

For kind work:
- export a temporary kubeconfig rather than mutating your default config
- prefer controlled changes in kind, not existing large clusters

## Final summary

If you restart now, the key thing to remember is:

**The codebase has a real long-lived runtime path, daemon API, standalone CLI client mode, and kind validation proved polling can apply service, event, storage, pod, and workload changes in-process without a second full rebuild.**

The main open question is no longer “what should continuous runtime look like?”

It is:

> can the ontology backend contract stay clean enough for external graph stores while the system repeats the scoped-mutation pattern for identity/security updates?

That should be the first thing to build and validate next.
