# Kubernetes Ontology Current State and Next Steps

## Purpose

This is the restart entrypoint for the project.

It records the current architecture, what has already been implemented, how to use the repository, what has been validated, and what remains open.

Related design documents:
- `docs/design/open-source-mvp-plan.md`
- `docs/design/kubernetes-semantic-kernel-evolution.md`
- `docs/design/continuous-runtime-technical-design.md`
- `docs/design/continuous-runtime-progress-snapshot.md`

## Current Position

`kubernetes-ontology` has moved beyond a one-shot diagnostic CLI.

It is now best described as:

> a read-only Kubernetes ontology database alpha with an in-memory backend, runtime manager, informer-first change detection with polling fallback, scoped graph mutation, recursive owner-chain resolution, daemon HTTP API, standalone CLI client, diagnostic query facade, and local graph viewer.

For the open-source MVP, this should now be read as:

> a local, read-only, in-memory Kubernetes topology service with daemon-backed AI-agent queries, informer-first continuous updates, polling fallback, RBAC topology for ServiceAccount bindings, focused diagnostics, and a lightweight human inspection viewer.

The binary still runs locally and reads from a user-provided kubeconfig. It does not mutate cluster state.

The product goal is not only fault diagnosis. Fault diagnosis is the first MVP query family on top of a general Kubernetes ontology database for AI-agent resource relationship queries.

## Runtime Architecture

Current flow:

```text
Kubernetes API
  |
  v
Read-only Collector
  |
  +-- bootstrap snapshot -------------------+
  |                                         |
  +-- informer/watch stream --------------- | ----------------+
  |                                         |                 |
  +-- polling fallback -------------------- | ----------------+
                                            v                 |
                                     Runtime Manager          |
                                            |                 |
                                            v                 |
                                    Reconcile Planner         |
                                            |                 |
                                            v                 |
                            Full rebuild or scoped mutation   |
                                            |                 |
                                            v                 |
                                      Graph Kernel            |
                                            |                 |
                                            v                 |
                                     Query Facade            |
                                            |
                                            +----> Ontology HTTP API
                                            |
                                            v
                                  Diagnostic Subgraph JSON
```

Key packages:
- `internal/collect/k8s`: read-only Kubernetes collection, informer stream, and polling fallback
- `internal/runtime`: runtime lifecycle, bootstrap, status, and stream application
- `internal/ontology`: backend abstraction for entity/relation storage and graph queries
- `internal/server`: HTTP API for runtime status, ontology queries, and diagnostics
- `internal/reconcile`: full rebuild plus scoped mutation reconcilers
- `internal/graph`: graph builder, kernel, and reverse index
- `internal/query`: facade above diagnostic services
- `internal/service/diagnostic`: diagnostic subgraph query implementation
- `tools/visualize`: local HTML graph viewer

## Implemented Capabilities

### Collection
- read-only snapshot collection from Kubernetes
- context namespace collection scope, with empty scope meaning all namespaces
- normalized resources for workloads, ReplicaSets, pods, services, configmaps, secrets, service accounts, role bindings, PVCs, PVs, events, and webhook configs
- ownerReferences retain the Kubernetes `controller` marker

### Graph Semantics
- workload nodes
- pod nodes
- service selector edges
- pod-to-node scheduling edges
- pod-to-configmap, pod-to-secret, pod-to-serviceaccount edges
- serviceaccount-to-rolebinding and serviceaccount-to-clusterrolebinding edges
- pod-to-image edges with lightweight OCI parsing
- pod-to-PVC and PVC-to-PV edges
- event report edges
- admission webhook evidence edges
- CSI evidence edges from configured CSI component rules

### Recursive Owner Chain
Owner resolution is now recursive and shared by:
- full graph build
- `pod-narrow`
- `workload-narrow`

Current owner-chain rules:
- prefer controller ownerReferences when present
- if no controller marker exists, consider all ownerReferences for compatibility with partial or synthetic data
- resolve by UID first
- fall back to namespace/kind/name
- stay namespace-scoped for namespaced owner objects
- guard against cycles

This covers ordinary `Pod -> ReplicaSet -> Deployment` ownership and deeper controller chains.

### Runtime
The runtime layer supports:
- bootstrap full sync
- runtime status projection
- query facade wiring
- daemon HTTP serving
- standalone CLI client mode through `--server`
- bounded observe mode via CLI flags
- informer-based stream application with polling fallback

Important CLI flags:
- `--status-only`
- `--observe-duration`
- `--stream-mode`
- `--poll-interval`

### Scoped Mutation
The planner and runtime currently support these scoped strategies:
- `service-narrow`
- `event-narrow`
- `storage-narrow`
- `identity/security-narrow`
- `pod-narrow`
- `workload-narrow`

Unsupported categories still fall back to full rebuild.

### Runtime Status
The status object currently includes:
- phase and readiness
- cluster name
- node and edge counts
- last bootstrap timestamp
- last applied change kind, namespace, name, type, and timestamp
- last strategy
- full rebuild count
- narrow strategy counters
- last error

## Validation Completed

Stable local test command:

```bash
go test -p 1 ./...
```

The `-p 1` form is preferred because it has been stable under local resource pressure.

Kind-cluster validation has proved these in-process update paths without a second full rebuild:
- service selector change -> `service-narrow`
- core/v1 Event creation -> `event-narrow`
- PV/PVC creation and binding -> `storage-narrow`
- ServiceAccount / RoleBinding / ClusterRoleBinding changes -> `identity/security-narrow`
- Pod label change -> `pod-narrow`
- Deployment label change -> `workload-narrow`

For each kind validation, `FullRebuildCount` stayed at `1` after bootstrap and the relevant narrow counter incremented.

## How to Use

Build:

```bash
make build
```

Run tests:

```bash
make test
```

Optional local defaults:

```bash
cp local/kubernetes-ontology.yaml.example local/kubernetes-ontology.yaml
```

Edit `local/kubernetes-ontology.yaml` with local kubeconfig, cluster, context
namespaces, server URL, stream mode, and polling fallback defaults. This file is
ignored by git and loaded automatically by `make`.

Run daemon:

```bash
make serve \
  KUBECONFIG="$HOME/.kube/config" \
  CLUSTER="kind-kind" \
  CONTEXT_NAMESPACES="default,kube-system"
```

Query daemon:

```bash
make status-server SERVER_URL="http://127.0.0.1:18080"
make list-entities-server SERVER_URL="http://127.0.0.1:18080" ENTITY_KIND=Pod NAMESPACE=default
```

Read-only runtime status:

```bash
make status \
  KUBECONFIG="$HOME/.kube/config" \
  CLUSTER="kind-kind" \
  CONTEXT_NAMESPACES="default,kube-system"
```

Observe for a bounded window:

```bash
make observe-status \
  KUBECONFIG="$HOME/.kube/config" \
  CLUSTER="kind-kind" \
  CONTEXT_NAMESPACES="default,kube-system" \
  OBSERVE_DURATION=40s \
  POLL_INTERVAL=2s
```

Diagnose a pod:

```bash
make diagnose-pod \
  KUBECONFIG="$HOME/.kube/config" \
  CLUSTER="kind-kind" \
  NAMESPACE="default" \
  NAME="my-pod" \
  > /tmp/pod-graph.json
```

Diagnose a workload:

```bash
make diagnose-workload \
  KUBECONFIG="$HOME/.kube/config" \
  CLUSTER="kind-kind" \
  NAMESPACE="default" \
  NAME="my-deployment" \
  > /tmp/workload-graph.json
```

View live topology or a diagnostic graph:

```bash
make visualize
```

Open `http://127.0.0.1:8765`, then click `Load topology` or load the JSON output path.

## Visualization

The viewer is intentionally local and lightweight:
- static HTML
- tiny Python server for local file loading and ontology-server proxying
- no frontend build system
- no browser-side cluster access

It can load:
- live topology from `kubernetes-ontologyd`
- live pod/workload diagnostic graphs
- a selected JSON file
- dragged-and-dropped JSON
- an absolute file path through the local server
- `?file=/tmp/graph.json` URL hints

The viewer is useful for quick inspection of topology shape, nodes, edges, provenance, connected relations, and explanation evidence.

## Remaining Work

Highest-value next work:
1. extend informer and scoped-reconcile coverage for future topology categories while keeping the same `ChangeEvent` sink contract
2. add HTTP auth/TLS and internal soak tests for daemon mode
3. harden diagnostic output schema and evidence ranking for downstream AI-agent consumers
4. expand real-world failure fixtures and viewer QA cases
5. keep persistent stores and external graph adapters as post-MVP research, not open-source MVP work

## Non-Goals for the Current Slice

Still intentionally out of scope:
- writing objects back to Kubernetes
- CRD installation
- cluster-side agents
- full RBAC reasoning
- ontology database materialization
- persistent graph storage or external graph adapters
- restart recovery beyond rebuilding the in-memory graph from Kubernetes
- final public multi-tenant API protocol
