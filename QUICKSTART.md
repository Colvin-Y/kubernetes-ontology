# Quick Start

[Project README](README.md) | [中文说明](README.zh-CN.md)

This guide gets `kubernetes-ontology` running in its recommended MVP mode:

- `kubernetes-ontologyd` runs as the ontology server.
- `kubernetes-ontology` runs as the CLI client.
- The server reads Kubernetes objects and maintains an in-memory graph.
- The client queries status, entities, relations, neighbors, and diagnostic
  subgraphs through the server.

The daemon is read-only with respect to the Kubernetes resources it observes:
it does not create, update, patch, delete, or annotate workloads and related
objects. Helm installation does create this project's own Deployment, Service,
ServiceAccount, ConfigMap, and read-only RBAC. The MVP stores graph state in
memory only: restarting the daemon rebuilds the graph from the Kubernetes API.

## Choose a Path

| Path | Best for | What runs where |
| ---- | -------- | --------------- |
| Helm + release CLI | Users who want to try the project without compiling Go code | Server and viewer run in Kubernetes. CLI runs on your workstation. |
| Source build | Contributors and local development | Server, CLI, and viewer run from this repository. |

If you are unsure, use the Helm path first. Use the source path when changing
code or testing local patches.

## Contents

- [Prerequisites](#prerequisites)
- [No-Compile Path: Helm + Release CLI](#no-compile-path-helm--release-cli)
- [Source Path](#source-path)
- [Query Examples](#query-examples)
- [Topology Viewer](#topology-viewer)
- [Verification Flow For Changes](#verification-flow-for-changes)
- [Troubleshooting](#troubleshooting)

## Prerequisites

- A kubeconfig with read access to the target cluster.
- Network access from your machine to the Kubernetes API server.
- For the no-compile path: `kubectl`, `helm`, and a downloaded
  `kubernetes-ontology` CLI binary from GitHub Releases.
- For local development from source: Go installed locally.

## No-Compile Path: Helm + Release CLI

This path runs the server in Kubernetes from a published container image and
uses `kubectl port-forward` plus the release CLI binary from your machine.

Set the version and image namespace you want to use:

```bash
export KO_VERSION=v0.1.2
export KO_IMAGE=ghcr.io/colvin-y/kubernetes-ontology
```

Use the `KO_VERSION` value for the release tag you want to install. If you
publish a fork or a different package namespace, replace `KO_IMAGE` with your
image reference.

Install the Helm chart:

```bash
helm upgrade --install kubernetes-ontology ./charts/kubernetes-ontology \
  --namespace kubernetes-ontology \
  --create-namespace \
  --set image.repository="${KO_IMAGE}" \
  --set image.tag="${KO_VERSION}" \
  --set cluster="your-logical-cluster" \
  --set contextNamespaces='{default,kube-system}'
```

The chart installs the project server, viewer, ServiceAccount, ConfigMap, and
read-only RBAC. The daemon uses those in-cluster credentials only for
`get`/`list`/`watch` collection. Inside the pod, the server listens on `:18080`
rather than `0.0.0.0:18080` so Kubernetes IPv4, IPv6, and dual-stack networking
can use the wildcard listener supported by the runtime. By default the chart
grants `secrets` `get`/`list`/`watch` permission so Secret nodes and
`uses_secret` edges can be collected. To run without Secret collection:

```bash
helm upgrade --install kubernetes-ontology ./charts/kubernetes-ontology \
  --namespace kubernetes-ontology \
  --reuse-values \
  --set rbac.readSecrets=false
```

Wait for the server:

```bash
kubectl -n kubernetes-ontology rollout status deploy/kubernetes-ontology
```

Expose the in-cluster server on your workstation:

```bash
kubectl -n kubernetes-ontology port-forward svc/kubernetes-ontology 18080:18080
```

Download the CLI from GitHub Releases in another terminal. Example for macOS
Apple Silicon:

```bash
curl -LO "https://github.com/Colvin-Y/kubernetes-ontology/releases/download/${KO_VERSION}/kubernetes-ontology_${KO_VERSION}_darwin_arm64.tar.gz"
tar -xzf "kubernetes-ontology_${KO_VERSION}_darwin_arm64.tar.gz"
sudo install "kubernetes-ontology_${KO_VERSION}_darwin_arm64/kubernetes-ontology" /usr/local/bin/kubernetes-ontology
```

Use `linux_amd64`, `linux_arm64`, `darwin_amd64`, `darwin_arm64`, or
`windows_amd64.zip` for other machines.

Query the port-forwarded server:

```bash
kubernetes-ontology --server "http://127.0.0.1:18080" --status
```

```bash
kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --list-entities \
  --entity-kind Pod \
  --namespace default \
  --limit 20
```

Open the dependency-free viewer. The Helm chart deploys it by default:

```bash
kubectl -n kubernetes-ontology port-forward svc/kubernetes-ontology-viewer 8765:8765
```

Then open:

```text
http://127.0.0.1:8765
```

You can also run the viewer locally from the GitHub Release archive without
Python:

```bash
kubernetes-ontology-viewer --server "http://127.0.0.1:18080"
```

## Source Path

Use this path when you want to run from a local checkout.

### 1. Build

```bash
make build build-daemon
```

### 2. Create Local Config

Create a machine-local YAML config:

```bash
cp local/kubernetes-ontology.yaml.example local/kubernetes-ontology.yaml
```

Edit `local/kubernetes-ontology.yaml`:

```yaml
kubeconfig: /absolute/path/to/kubeconfig.yaml
cluster: your-logical-cluster
namespace: default
contextNamespaces:
  - default
  - kube-system

workloadResources:
  - group: apps.kruise.io
    version: v1beta1
    resource: statefulsets
    kind: StatefulSet
    namespaced: true
  - group: redis.io
    version: v1beta1
    resource: clusters
    kind: Cluster
    namespaced: true

controllerRules:
  - apiVersion: apps.kruise.io/*
    kind: "*"
    namespace: kruise-system
    controllerPodPrefixes:
      - kruise-controller-manager
    nodeDaemonPodPrefixes:
      - kruise-daemon

server:
  addr: 127.0.0.1:18080
  url: http://127.0.0.1:18080
bootstrapTimeout: 2m
streamMode: informer
pollInterval: 5s
```

`server.addr` is used by `kubernetes-ontologyd`. `server.url` is documentation
for local tooling; the CLI only queries a daemon when `--server` or a
`*-server` make target is used.
`bootstrapTimeout` bounds the initial full snapshot sync. Large clusters or
slow API servers usually need more than the old 30 second default.

`local/kubernetes-ontology.yaml` is ignored by git. Put local kubeconfig paths,
private cluster names, namespaces, collection resources, display rules, and
scratch query defaults there. Make targets automatically use this file when it
exists. Use `CONFIG=other.yaml` for a different config file.

`NAMESPACE` is the default namespace used by client queries and diagnostic
entrypoints. `contextNamespaces` is the server collection scope.

The context namespace list is only a collection scope. It does not mark pods as
infrastructure, business, system, or any other ontology role.

`workloadResources` tells the collector which CRD-like workload resources should
be collected so ownerReference chains can resolve through them. `controllerRules`
adds display-only controller ownership, such as Kruise workloads being served by
`kruise-controller-manager` and node-local `kruise-daemon` pods.
For `workloadResources.kind`, use the actual Kubernetes ownerReference `kind`
such as `StatefulSet` for Kruise ASTS, not a local nickname.
The example OpenKruise and Redis resources are optional. On a clean kind
cluster without those CRDs installed, the server logs the unavailable custom
resources and skips their informers; remove those entries or install the CRDs
when you want them collected.

The equivalent CLI flags still exist for one-off overrides:

```bash
--context-namespaces "default,kube-system"
--workload-resources "apps.kruise.io/v1beta1/statefulsets/StatefulSet,redis.io/v1beta1/clusters/Cluster"
--controller-rules "apiVersion=apps.kruise.io/*;kind=*;namespace=kruise-system;controller=kruise-controller-manager;daemon=kruise-daemon"
```

The Makefile still accepts `KUBECONFIG=...`, `CLUSTER=...`, and
`CONTEXT_NAMESPACES=...` for one-off runs when `CONFIG` is not used.

### 3. Start the Server

In terminal 1:

```bash
make serve
```

The daemon performs an initial full snapshot sync, then refreshes the in-memory
ontology graph with Kubernetes informers. If informer startup fails, it logs the
failure and falls back to polling. Set `streamMode: polling` or pass
`--stream-mode polling` to force polling. Stop it with `Ctrl+C`.

Equivalent direct command:

```bash
go run ./cmd/kubernetes-ontologyd --config local/kubernetes-ontology.yaml
```

For a one-shot bootstrap API without continuous refresh:

```bash
go run ./cmd/kubernetes-ontologyd \
  --config local/kubernetes-ontology.yaml \
  --disable-polling
```

### 4. Check Server Health

In terminal 2:

```bash
curl -s http://127.0.0.1:18080/healthz
```

Expected shape:

```json
{"cluster":"your-logical-cluster","ok":true,"phase":"ready"}
```

Read full runtime status through the CLI client:

```bash
make status-server
```

Direct CLI:

```bash
./bin/kubernetes-ontology --server "http://127.0.0.1:18080" --status
```

## Query Examples

### Query Ontology Entities

List pods:

```bash
make list-entities-server ENTITY_KIND=Pod NAMESPACE=default LIMIT=20
```

Get one entity by Kubernetes identity:

```bash
make get-entity-server ENTITY_KIND=Pod NAMESPACE=default NAME=my-pod
```

The response contains `entity.entityGlobalId`. Use that ID for relation and
neighbor queries.

Example direct CLI command:

```bash
go run ./cmd/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --list-entities \
  --entity-kind Pod \
  --namespace default \
  --limit 20
```

Resolve one entity with the agent-friendly alias:

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --resolve-entity \
  --entity-kind Pod \
  --namespace default \
  --name my-pod
```

### Query Relations and Neighbors

List outgoing neighbors for an entity:

```bash
make neighbors-server \
  ENTITY_ID='your/entityGlobalId' \
  DIRECTION=out \
  LIMIT=50
```

Filter by relation kind:

```bash
make neighbors-server \
  ENTITY_ID='your/entityGlobalId' \
  RELATION_KIND=scheduled_on \
  DIRECTION=out
```

List relations from one entity:

```bash
make list-relations-server \
  FROM_ID='your/entityGlobalId' \
  RELATION_KIND=scheduled_on
```

The direct CLI alias for filtered relation listing is:

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --list-filtered-relations \
  --from 'your/entityGlobalId' \
  --relation-kind scheduled_on \
  --limit 50
```

Common relation kinds include:

- `controlled_by`
- `owns_pod`
- `scheduled_on`
- `selects_pod`
- `uses_config_map`
- `uses_secret`
- `uses_service_account`
- `bound_by_role_binding`
- `mounts_pvc`
- `bound_to_pv`
- `reported_by_event`
- `affected_by_webhook`
- `managed_by_csi_controller`
- `served_by_csi_node_agent`

### Query Diagnostic Subgraphs

Diagnose a pod:

```bash
make diagnose-pod-server NAMESPACE=default NAME=my-pod
```

Diagnose a workload:

```bash
make diagnose-workload-server NAMESPACE=default NAME=my-deployment
```

Direct CLI:

```bash
go run ./cmd/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --diagnose-pod \
  --namespace default \
  --name my-pod
```

For workloads:

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --diagnose-workload \
  --namespace default \
  --name my-deployment
```

The diagnostic response is a focused subgraph intended for the MVP
fault-diagnosis workflow and downstream AI-agent consumption.

Pod-centered diagnostic queries keep shared nodes bounded by default. For
example, a pod's `ServiceAccount` is shown, but the traversal does not continue
through that ServiceAccount to every other pod using it. Use
`terminalKinds=...` or `expandTerminalNodes=true` on HTTP queries when you need
that deeper fan-out.

### Use HTTP Directly

The CLI is a convenience wrapper over the HTTP API:

```bash
curl -s 'http://127.0.0.1:18080/status'
curl -s 'http://127.0.0.1:18080/entities?kind=Pod&namespace=default&limit=20'
curl -s 'http://127.0.0.1:18080/entity?kind=Pod&namespace=default&name=my-pod'
curl -s 'http://127.0.0.1:18080/neighbors?entityGlobalId=your/entityGlobalId&direction=out'
curl -s 'http://127.0.0.1:18080/expand?entityGlobalId=your/entityGlobalId&depth=1'
curl -s 'http://127.0.0.1:18080/diagnostic/pod?namespace=default&name=my-pod'
curl -s 'http://127.0.0.1:18080/diagnostic/pod?namespace=default&name=my-pod&expandTerminalNodes=true'
```

Graph and list responses include the original fields plus additive `freshness`
metadata from the daemon runtime status. Error responses include the historical
`error` string plus `code`, `message`, `status`, `retryable`, and `source`.

For agent workflows that need machine-readable stderr on failures:

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --machine-errors \
  --resolve-entity \
  --entity-kind Pod \
  --namespace default \
  --name missing-pod
```

## Topology Viewer

With `make serve` still running, start the local viewer in another terminal:

```bash
make visualize
```

Open:

```text
http://127.0.0.1:8765
```

Click `Load topology` to read live entities and relations from `SERVER_URL`.
Use `Auto refresh` for continuous polling, or load a focused pod/workload
diagnostic graph from the same page.

Select a node and use `Expand 1 hop` to fetch the next layer from the daemon.
The CLI equivalent is:

```bash
make expand-node-server ENTITY_ID='your/entityGlobalId' EXPAND_DEPTH=1
```

Direct CLI:

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --expand-entity \
  --entity-id 'your/entityGlobalId' \
  --expand-depth 1
```

After expanding, select the same node and use `Collapse 1 hop` to remove that
expansion layer. Double-click toggles the same expand/collapse behavior. For an
agent workflow, export the viewer state and collapse the same node locally:

```bash
make -s collapse-node-graph GRAPH_FILE=/tmp/kubernetes-ontology-visible-topology.json ENTITY_ID='your/entityGlobalId'
```

## Verification Flow For Changes

After every code change, run the fixed local verification flow:

```bash
make verify
make serve
make visualize
make live-check NAMESPACE=default NAME=my-pod
```

Run `make serve` and `make visualize` in separate terminals. `make live-check`
then exercises the daemon, viewer topology proxy, and viewer diagnostic proxy.
That makes the timeout-prone visualization path part of the normal check, not a
manual afterthought.

## Troubleshooting

If `make serve` fails with `KUBECONFIG is required`, check
`local/kubernetes-ontology.yaml`, or pass `CONFIG=...` / `KUBECONFIG=...` on
the command line.

If `/healthz` returns `ok: false`, inspect:

```bash
make status-server
```

If entity lists are empty, check that `NAMESPACE` matches the target workload.
Also check that `contextNamespaces` includes the namespace you want the server
to collect. An empty `contextNamespaces` list means collect all namespaces.

If a diagnostic query returns `entry not found`, verify the exact pod or
workload name:

```bash
make list-entities-server ENTITY_KIND=Pod NAMESPACE=default
make list-entities-server ENTITY_KIND=Workload NAMESPACE=default
```

The default backend is in-memory. Restarting `kubernetes-ontologyd` rebuilds
the graph from Kubernetes.
